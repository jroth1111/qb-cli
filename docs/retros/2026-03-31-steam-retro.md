# Printing Press Retro: Steam Web API

## Session Stats
- API: Steam Web API
- Spec source: Zuplo/Steam-OpenAPI (158 operations, OpenAPI 3.0)
- Scorecard: 71/100 (Grade B)
- Verify pass rate: 67% (50/75 passed, after polish; 44% pre-polish)
- Fix loops: 1 (verify) + 1 (polish)
- Manual code edits: 12 (description rewrite, env var support, vanity resolver, dry-run guard, response parsing fixes, 16 verify arg fixes, dead code removal)
- Features built from scratch: 22 (10 wrapper commands + 6 transcendence + vanity resolver + API key injection + response unwrapper + 3 shared helpers)

## Findings

### 1. Verify command name mismatch for numeric suffixes (Scorer bug)

- **Scorer correct?** No. The CLI works correctly — all 25 commands with numeric suffixes run fine at runtime. Verify fails them because it derives the expected command name from the Go function name (`camelToKebab("IEconItems440")` → `iecon-items440`) instead of reading the actual cobra `Use:` field (`iecon-items-440`). The bug is in `internal/pipeline/runtime.go:265` where `discoverCommands` calls `camelToKebab(m[1])`. `camelToKebab` (runtime.go:537) only inserts hyphens before uppercase letters, not before digits — so `Items440` becomes `items440` not `items-440`. But the generator's OpenAPI parser (parser.go:1380) converts `IEconItems_440` via `toSnakeCase` → `_` to `-` → `i-econ-items-440`, preserving the hyphen before the digit.
- **What happened:** 25 commands scored 0/3 in verify. Verify looks for `iecon-items440` but the actual command is `iecon-items-440`.
- **Root cause:** Verify tool (`internal/pipeline/runtime.go:265` `discoverCommands`) derives command names from Go function names via `camelToKebab`, which loses hyphen-before-digit information. The generator creates the name from the OpenAPI resource path, which preserves it.
- **Frequency:** API subclass — APIs with numeric segments in resource names (gaming APIs, versioned interfaces).
- **Recommendation: Fix verify, not the generator.** The generator's naming (`iecon-items-440`) is correct and readable. The verify tool should use ground-truth command names.
- **Durable fix:** In `discoverCommands()` (runtime.go), run `<binary> --help` once, parse the top-level command names from help output, and use those. This is ground-truth. Alternatively, parse the `Use:` field from each command's Go source file.
- **Test:** Generate from Steam spec → verify discovers `iecon-items-440` and passes. Generate from Stripe → verify still works (no numeric suffixes = no behavior change).
- **Impact if not fixed:** 25 false verify failures on this API alone. Every API with numeric resource names would see similar inflation.

### 2. Dogfood dead-flag detection skips root.go (Scorer bug)

- **Scorer correct?** No. The 6 "dead flags" (`agent`, `noCache`, `noInput`, `rateLimit`, `timeout`, `yes`) are all used. `flags.agent` is read at root.go:69 in `PersistentPreRunE`. `flags.noCache` is read at root.go:176 via `c.NoCache = f.noCache` in `newClient()`. `flags.timeout` is read at root.go:175 via `client.New(cfg, f.timeout, f.rateLimit)`. Dogfood reports them as dead because `internal/pipeline/dogfood.go:385` explicitly skips root.go: `if filepath.Base(file) == "root.go" { continue }`. The skip was likely added to avoid counting flag *declarations* (`&flags.agent`) as "usage", but it also skips genuine *reads* (`if flags.agent {`).
- **What happened:** Dogfood reports 6 dead flags on every generated CLI. All 6 are false positives.
- **Root cause:** Dogfood's dead-flag detector (dogfood.go:365-405) extracts flag names from `PersistentFlags().Var(&flags.<name>, ...)` calls, then searches all `.go` files for `flags.<name>` usage — but skips `root.go` at line 385.
- **Frequency:** Every API. These 6 flags exist in every generated CLI.
- **Recommendation: Fix dogfood, not the generator.** The flags are correctly used. The detection logic needs to include root.go reads while excluding root.go declarations.
- **Durable fix:** In dogfood.go, change the search to include root.go but filter out lines matching the declaration pattern `&flags\.<name>`. Lines like `if flags.agent {` and `c.NoCache = f.noCache` would be correctly counted as usage.
- **Test:** Run dogfood on any generated CLI → `agent`, `noCache`, `noInput`, `rateLimit`, `timeout`, `yes` should NOT appear as dead. Add a genuinely unused flag → dogfood should still catch it.
- **Impact if not fixed:** 6 false warnings per CLI. Noise that obscures real dead flags.

### 3. Root command description copies spec boilerplate (Generator bug — scorer is correct)

- **Scorer correct?** Yes. The root `Short:` field was literally "Get your API key from [here](https://steamcommunity.com/dev/apikey)" — a markdown link copied from the spec's `info.description`. This is genuinely bad UX.
- **What happened:** Had to manually rewrite to "Query Steam player profiles, game libraries, achievements, and friends from the terminal".
- **Root cause:** `internal/generator/templates/root.go.tmpl:43` uses `{{oneline .Description}}` which copies the spec description verbatim. Spec descriptions describe the API, not what the CLI does.
- **Frequency:** Every API. Spec descriptions are always API-centric.
- **Recommendation: Fix the generator template.** Change the default `Short:` to `"Manage {{.Name}} resources via the {{.Name}} API"`. Claude can still improve it during generation, but the floor is higher than raw spec text.
- **Durable fix:** In `root.go.tmpl`, replace `{{oneline .Description}}` with a template that produces a generic but correct CLI description. The skill instruction already tells Claude to rewrite it (Phase 2: "REQUIRED: Rewrite the CLI description"), so this is defense-in-depth.
- **Test:** Generate from any spec → root `Short:` should not contain markdown links, raw API descriptions, or auth instructions.

### 4. Commands with required positional args fail verify dry-run (Both — scorer partially right, generator should improve)

- **Scorer correct?** Partially. Verify is correct that `<binary> <command> --help` should work with no positional args — that's reasonable behavior. But verify's test approach (`<binary> <command> --dry-run` with no args) fails because `Args: cobra.ExactArgs(N)` rejects before `RunE` runs. The generator's `Args:` pattern is valid cobra — it's just incompatible with verify's testing method. However, the help-guard pattern (`if len(args) == 0 { return cmd.Help() }`) is objectively better UX anyway — the user sees help instead of an error. So the generator should adopt it regardless of verify.
- **What happened:** 16 wrapper commands scored 1/3 pre-polish. Fixed by replacing `Args: cobra.ExactArgs(N)` with help-guard.
- **Root cause:** Generator emits `Args: cobra.ExactArgs(N)` for commands with required positional args. Verify runs commands with no args and gets an error exit.
- **Frequency:** Every API that gets wrapper/promoted commands.
- **Recommendation: Fix the generator template (better UX), and the verify score improvement is a side effect.** The help-guard pattern is better for users and agents. Don't frame this as "fix to pass verify" — frame it as "better UX that also happens to pass verify."
- **Durable fix:** In the promoted command template, emit the help-guard pattern instead of `Args:`. For commands needing N args: `if len(args) < N { return cmd.Help() }`.
- **Test:** Generate a CLI → all commands with positional args show help when invoked with no args (exit 0, not exit 2).

### 5. Query-param auth not auto-detected from spec (Generator enhancement — scorer is N/A)

- **Scorer correct?** N/A — the scorecard gave Auth 8/10, which is reasonable. The Zuplo Steam spec has no `securitySchemes` section at all — auth is only expressed as a `key` query parameter on 47/158 operations. The generator correctly saw no auth scheme because the spec didn't declare one. This is a spec quality issue, not a generator bug.
- **What happened:** Had to manually add `APIKey` field to config and `steamAPIKey()` helper.
- **Root cause:** The spec lacks `securitySchemes`. The generator's `client.go.tmpl` already supports query-param auth (`Auth.In == "query"`), but the OpenAPI parser had nothing to detect.
- **Frequency:** API subclass — APIs with undeclared auth (~20% have missing or incomplete security declarations).
- **Recommendation: Generator enhancement (not a bug fix).** Add a heuristic: if >30% of operations have a parameter named `key` or `api_key` in query position, infer query-param auth. This is an improvement, not a correction.
- **Durable fix:** In the OpenAPI parser, after security scheme detection, run a fallback: scan all operations for common auth param names (`key`, `api_key`, `apikey`, `access_token`) in query position. If found on >30% of operations, set `Auth.In = "query"` and `Auth.Header = <param_name>`.
  - **Guard:** Don't override explicit bearer/OAuth auth if already detected.
- **Test:** Parse the Steam public spec → auth detected as `in: query, header: key`. Parse Stripe spec → bearer auth still detected (guard works).

### 6. Generic sync doesn't work for non-standard URL patterns (Generator bug — scorer is correct)

- **Scorer correct?** Yes. Sync genuinely doesn't work for Steam. `sync --resources isteam_apps` returns 404 because sync builds the path as `/isteam_apps` but the actual endpoint is `/ISteamApps/GetAppList/v2/`.
- **What happened:** `sync --resources isteam_apps` fails with HTTP 404.
- **Root cause:** `defaultSyncResources()` returns resource names derived from the spec, and `syncResource()` builds the API path as `"/" + resource`. This assumes REST-style paths but Steam uses `/{Interface}/{Method}/v{version}/`.
- **Frequency:** API subclass — non-REST APIs (~10-15%). Most APIs follow REST where resource name = path segment.
- **Recommendation: Fix the generator (profiler + sync template).** The profiler should store the full endpoint path alongside the resource name.
- **Durable fix:** In `internal/profiler/profiler.go`, when adding to `SyncableResources`, store `{Name, Path, Params}` instead of just the name string. In `sync.go.tmpl`, use the stored path.
  - **Guard:** For standard REST APIs, the stored path is `"/resource"` — same behavior as today.
- **Test:** Generate from Steam spec → `sync --resources isteam-apps` calls `/ISteamApps/GetAppList/v2/`. Generate from Stripe → sync still uses `/customers`.

### 7. Unconditionally emitted helper functions create dead code (Generator bug — scorer is correct)

- **Scorer correct?** Yes. 5 genuinely dead functions were present. `classifyDeleteError` should have been conditional on `HasDelete` (it's guarded by `{{if .HasDelete}}` in the template, but was emitted anyway — possible flag computation bug). `replacePathParam` and `usageErr` are emitted unconditionally.
- **What happened:** Had to manually delete `classifyDeleteError`, `firstNonEmpty`, `printOutputFiltered`, `replacePathParam`, `usageErr`.
- **Root cause:** `helpers.go.tmpl` emits some functions unconditionally. The `HelperFlags` struct has `HasDelete` but may not compute it correctly for all specs.
- **Frequency:** Every API gets some dead helpers (the specific set varies).
- **Recommendation: Fix the generator.** Add more conditional flags (`HasPathParams`, `HasUsageErrors`) and verify the `HasDelete` flag computation.
- **Durable fix:** Two-part: (1) Add conditional flags in generator.go for each helper. (2) Add a `printing-press polish --remove-dead-code` post-processing step as safety net.
- **Test:** Generate from spec with no DELETE endpoints → `classifyDeleteError` not emitted. Generate from spec with no path params → `replacePathParam` not emitted.

### 8. Dry-run responses poison cache (Generator bug — not a scoring issue)

- **Scorer correct?** N/A — this is a runtime bug, not flagged by any scorer. Discovered during live testing.
- **What happened:** Running `bans 76561197960287930 --dry-run` cached the dry-run stub `{"dry_run": true}`. The next real call returned the cached stub, making it look like the command was broken. Had to use `--no-cache` to get real data.
- **Root cause:** `client.go.tmpl`'s `Get()` method caches responses based on path+params but doesn't check `c.DryRun`. Dry-run responses are synthetic but get written to cache.
- **Frequency:** Every API. Any command with dry-run poisons the cache.
- **Recommendation: Fix the generator template.** Add `!c.DryRun` guards to cache read and write in `Get()`.
- **Durable fix:** In `client.go.tmpl`, skip cache when `c.DryRun`:
  ```go
  if !c.NoCache && !c.DryRun && c.cacheDir != "" { ... }
  ```
- **Test:** Run `<cli> <cmd> --dry-run` then `<cli> <cmd> --json` → second call returns real data.

### 9. Steam response envelope format not handled by generated commands (Generator enhancement)

- **Scorer correct?** N/A — not directly flagged by a scorer. The generated raw commands pass JSON through, which is technically correct for raw API access. The issue only surfaces when building wrapper commands that need to extract specific fields.
- **What happened:** Steam wraps most responses in `{"response": {...}}` but some use `{"players": [...]}`. Had to write `extractResponse()` and `extractPlayers()` helpers for wrapper commands.
- **Root cause:** The generator doesn't detect or handle API-specific response envelope patterns.
- **Frequency:** Most APIs (>80% use some envelope pattern — `data`, `results`, `response`, `items`).
- **Recommendation: Generator enhancement for promoted commands.** During spec analysis, detect the response envelope key from response schemas. Emit an unwrap call in promoted command templates. Raw API commands should continue passing through raw JSON (that's correct behavior for raw access).
- **Durable fix:** In profiler, detect common envelope keys. In the promoted command template, emit unwrapping.
  - **Guard:** Only for promoted commands. Raw commands pass through as-is.

### 10. Global achievement percentage field type mismatch (Skip — API quirk)

- **Scorer correct?** N/A — not flagged by a scorer. Discovered during live smoke testing.
- **What happened:** Steam returns `percent` as string `"11.4"` not float. Generated parsing crashed.
- **Root cause:** Spec says number but API returns string. Spec-reality gap.
- **Frequency:** Common (~30% of APIs have at least one type mismatch), but hard to fix generically.
- **Recommendation: Skip for generator. The verify fix loop should catch and auto-fix type errors when testing against the live API.** Adding defensive parsing to every field adds complexity for a minority of cases.

## Prioritized Improvements

### Fix the Scorer (scoring tool bugs — highest priority)
| # | Scorer | Bug | Impact (false failures/points) | Fix target |
|---|--------|-----|-------------------------------|------------|
| 1 | Verify | `discoverCommands` derives names from Go function names via `camelToKebab`, loses hyphen-before-digit | 25 false 0/3 failures (33% of verify total) | `internal/pipeline/runtime.go:265` |
| 2 | Dogfood | Dead-flag detection skips `root.go` where flags are consumed | 6 false dead-flag warnings per CLI | `internal/pipeline/dogfood.go:385` |

### Do Now
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 8 | Dry-run cache poisoning | `client.go.tmpl` | Every API | Ships broken — users hit stale cache | Small | None needed |
| 3 | Root Short copies spec boilerplate | `root.go.tmpl` | Every API | Claude usually rewrites (skill says REQUIRED) | Small | None |
| 4 | Help-guard instead of Args: for promoted commands | Command templates | Every API | Claude fixes during polish — reliable but mechanical | Small | None |

### Do Next (needs design/planning)
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 6 | Sync should use actual endpoint paths | Profiler + sync template | Non-REST APIs | Sync ships broken | Medium | REST APIs unaffected |
| 9 | Auto-detect response envelope for promoted commands | Profiler + command template | Most APIs | Claude writes helpers — medium reliability | Medium | Raw commands unaffected |
| 7 | More conditional helper functions | `helpers.go.tmpl` + generator `HelperFlags` | Every API | Claude deletes — reliable | Medium | None |
| 5 | Infer auth from query param names | OpenAPI parser | ~20% of APIs | Claude adds manually | Medium | Don't override explicit auth |

### Skip
| # | Fix | Why unlikely to recur / why skip |
|---|-----|--------------------------------|
| 10 | Defensive parsing for type mismatches | Common problem but wrong fix level. The verify fix loop (live testing) is where type errors should be caught and auto-fixed. Generator-level defensive parsing adds complexity to every field. |

## Work Units

### WU-1: Fix verify command name derivation (finding #1)
- **Goal:** Verify uses actual cobra command names instead of names derived from Go function names
- **Target files:** `internal/pipeline/runtime.go` (function `discoverCommands`, line 251)
- **Acceptance criteria:**
  - Generate from Steam spec (has `IEconItems_440`) → verify discovers `iecon-items-440`, all 25 previously-failing commands pass
  - Generate from Stripe spec → verify still discovers all commands correctly (negative test)
  - Total verify pass rate for Steam CLI jumps from 67% to ~95%
- **Scope boundary:** Only changes command discovery. Does not change how the generator names commands.
- **Complexity:** Medium (1 file, needs `--help` output parsing or `Use:` field extraction)

### WU-2: Fix dogfood dead-flag false positives (finding #2)
- **Goal:** Dogfood correctly identifies dead flags without false positives for cobra-bound flags
- **Target files:** `internal/pipeline/dogfood.go` (line 385, the `root.go` skip)
- **Acceptance criteria:**
  - Run dogfood on any generated CLI → `agent`, `noCache`, `noInput`, `rateLimit`, `timeout`, `yes` NOT reported dead
  - Add a genuinely unused flag to a test CLI → dogfood catches it (negative test)
- **Scope boundary:** Only changes dead-flag detection. Dead-function detection unchanged.
- **Complexity:** Small (1 file, adjust the root.go skip to exclude declarations only)

### WU-3: Fix dry-run cache poisoning (finding #8)
- **Goal:** Dry-run responses never written to or read from cache
- **Target files:** `internal/generator/templates/client.go.tmpl`
- **Acceptance criteria:**
  - Run `<cli> <cmd> --dry-run` then `<cli> <cmd> --json` → second call returns real API data
  - Run `<cli> <cmd> --json` twice → second call returns cached data (cache still works)
- **Scope boundary:** Only changes cache guards. Dry-run behavior itself unchanged.
- **Complexity:** Small (1 file, 2-line guard addition)

### WU-4: Generator template improvements (findings #3, #4, #7)
- **Goal:** Better defaults: root description, help-guard args, conditional helpers
- **Target files:**
  - `internal/generator/templates/root.go.tmpl` (Short field)
  - `internal/generator/templates/command_endpoint.go.tmpl` (Args handling)
  - `internal/generator/templates/helpers.go.tmpl` (conditional functions)
  - `internal/generator/generator.go` (HelperFlags struct)
- **Acceptance criteria:**
  - Generated root Short is "Manage <API> resources via the <API> API", not spec boilerplate
  - Generated promoted commands show help when invoked with no args (exit 0, not exit 2)
  - `replacePathParam` only emitted when spec has path params; `usageErr` only when needed
- **Scope boundary:** Template changes only. Profiler, parser, and verify unchanged.
- **Complexity:** Medium (4 files, mostly mechanical)

### WU-5: Sync path resolution for non-REST APIs (finding #6)
- **Goal:** Sync uses actual API endpoint paths instead of resource-name-as-path
- **Target files:**
  - `internal/profiler/profiler.go` (SyncableResources struct)
  - `internal/generator/templates/sync.go.tmpl` (use stored path)
  - `internal/spec/spec.go` (struct changes if needed)
- **Acceptance criteria:**
  - Generate from Steam spec → `sync --resources isteam-apps` calls `/ISteamApps/GetAppList/v2/`
  - Generate from Stripe spec → sync still calls `/customers` (negative test — REST unaffected)
- **Scope boundary:** Sync data flow only. Does not change resource naming in CLI help.
- **Dependencies:** None
- **Complexity:** Medium (3 files, struct change with ripple effects)

## Anti-patterns

- **Nuking and rewriting README instead of improving in-place:** Dropped scored sections (Agent Usage, Troubleshooting) unnecessarily. The scorer checks for named sections. The right approach is additive: keep all scored sections, replace bad content with good content. Score went 7→5→7 when we could have gone 7→8+.
- **Calling verify failures "unfixable" without tracing the root cause:** The naming mismatch was traced to a specific function (`camelToKebab` at runtime.go:537 vs actual cobra `Use:` field from the OpenAPI parser). What initially looked like "Steam's weird URL pattern" turned out to be a straightforward verify bug with a one-function fix.
- **Accepting "the scorer is wrong" as a terminal diagnosis:** The correct response to "the scorer is wrong" is to fix the scorer, not to shrug and publish with a bad score. Scorer bugs are the highest-priority retro findings because they distort every future CLI's quality signal.

## What the Machine Got Right

- **Spec selection:** Zuplo OpenAPI spec with 158 endpoints was comprehensive. Web search found two maintained community specs without needing catalog entry.
- **7/7 quality gates on first generation:** All static checks passed immediately.
- **Adaptive rate limiter:** Generated client rate-limits correctly against Steam's undocumented limits.
- **Agent-native output:** `--json`, `--compact`, `--select`, `--agent` all worked from generation. No fixes needed.
- **Query-param auth template:** `client.go.tmpl` already had the `Auth.In == "query"` conditional — the template supports query auth, just the detection was missing for this spec.
- **Profiler-derived sync resources:** `defaultSyncResources()` was computed from spec analysis, not hardcoded. The sync issue was about URL construction, not resource discovery.
- **Response caching:** 5-minute GET cache reduced API calls during development (except for the dry-run poisoning bug).
