# Printing Press Retro: hackernews

## Session Stats
- API: Hacker News (Firebase + Algolia dual-API)
- Spec source: Internal YAML
- Scorecard: 87/100 (Grade A)
- Verify pass rate: 95%
- Fix loops: 1 (shipcheck + polish)
- Manual code edits: 1 (manifest api_name fix)
- Features built from scratch: 11 (algolia helper + 10 transcendence commands)

## Findings

### F1. lock promote doubles -pp-cli suffix in manifest (bug)
- **What happened:** After `lock promote --cli hackernews-pp-cli`, the manifest had `api_name: "hackernews-pp-cli"` and `cli_name: "hackernews-pp-cli-pp-cli"`. Had to manually fix before publishing.
- **Scorer correct?** N/A — not a scoring issue. Publishing validation caught it.
- **Root cause:** `internal/pipeline/state.go` line 309: `NewMinimalState(cliName, workingDir)` sets `APIName = cliName`. The skill passes the full CLI name (`hackernews-pp-cli`), which becomes the APIName. Then `writeCLIManifestForPublish` in `publish.go` line 200 does `CLIName: naming.CLI(state.APIName)` which appends `-pp-cli` again → `hackernews-pp-cli-pp-cli`.
- **Cross-API check:** Affects every CLI where the skill uses `lock promote --cli <name>-pp-cli` and the promote path goes through `NewMinimalState`.
- **Frequency:** Every CLI generation via the skill.
- **Fallback:** Claude catches it at publish validation (the doubled name fails). But requires a manual manifest edit every time — unreliable at scale.
- **Worth a Printing Press fix?** Yes — this fails on every run.
- **Inherent or fixable:** Fixable. `NewMinimalState` should strip the `-pp-cli` suffix: `APIName: naming.TrimCLISuffix(cliName)`.
- **Durable fix:** In `state.go` `NewMinimalState`, change `APIName: cliName` to `APIName: naming.TrimCLISuffix(cliName)`. This is a one-line fix. The naming package already has `TrimCLISuffix`.
- **Test:** Positive: `NewMinimalState("hackernews-pp-cli", dir)` → `APIName = "hackernews"`. Negative: `NewMinimalState("hackernews", dir)` → `APIName = "hackernews"` (no double strip).
- **Evidence:** `{"api_name": "hackernews-pp-cli", "cli_name": "hackernews-pp-cli-pp-cli"}` in published manifest.

### F2. Phase 5 auto-skips for no-auth APIs — zero live testing (skill instruction gap)
- **What happened:** Phase 5 (Dogfood Testing) was completely skipped because the skill says "When no API key is available, Phase 5 auto-skips." But HN requires NO auth — every endpoint is free and public. The CLI shipped with zero live testing until the user explicitly asked for it.
- **Scorer correct?** N/A — not a scoring issue.
- **Root cause:** The Phase 5 gate equates "no API key" with "can't test." For no-auth APIs, this is wrong — they're the MOST testable APIs.
- **Cross-API check:** Every no-auth API: ESPN, HN, CoinGecko free tier, public weather APIs, Postman Explore.
- **Frequency:** Every no-auth CLI (at least 5 of 17 current library CLIs).
- **Fallback:** User manually asks for testing. But if they don't ask, the CLI ships untested. The ESPN CLI shipped with zero testing in its original run for this same reason.
- **Worth a Printing Press fix?** Yes — this is the highest-impact skill fix. No-auth APIs are the easiest to test and the most embarrassing to ship broken.
- **Inherent or fixable:** Fixable. Change one sentence in the skill.
- **Durable fix:** In `skills/printing-press/SKILL.md` Phase 5, change the auto-skip logic:
  - **Current:** "When no API key is available, Phase 5 auto-skips"
  - **Proposed:** "Phase 5 auto-skips ONLY when the API requires auth AND no key is available. For APIs with `auth.type: none`, Phase 5 is MANDATORY."
- **Test:** Positive: HN (no auth) → Phase 5 runs automatically. Negative: Stripe (auth required, no key) → Phase 5 still skips.
- **Evidence:** User had to say "make sure you dogfood and test appropriately" before any live testing happened.

### F3. Dual-API CLIs need a second HTTP client pattern (template gap)
- **What happened:** HN uses Firebase (base_url in spec) AND Algolia (separate base URL). All 10 transcendence features use Algolia. Had to create `algolia.go` with a separate `algoliaGet()` helper using raw `net/http` because the generated client only knows one base URL.
- **Scorer correct?** N/A.
- **Root cause:** The spec format supports one `base_url`. The generated client template produces one `Client` struct with one base URL. There's no concept of a secondary API.
- **Cross-API check:** Same pattern as movie-goat (TMDb + OMDb). Any combo CLI needs this.
- **Frequency:** API subclass: multi-source CLIs. Growing pattern.
- **Fallback:** Claude builds the secondary client by hand. ~40 lines for a basic HTTP helper. Works but is boilerplate.
- **Worth a Printing Press fix?** P3 — interesting but low frequency. The hand-built approach is small enough.
- **Inherent or fixable:** Partially fixable. Could add an `enrichment_apis` section to the spec, but the complexity outweighs the frequency.
- **Durable fix:** Skip for now. Note as future direction. Same finding as movie-goat retro F8.

## Prioritized Improvements

### P1 — High priority
| Finding | Title | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---------|-------|-----------|-----------|---------------------|------------|--------|
| F1 | Double -pp-cli suffix in manifest | `internal/pipeline/state.go` NewMinimalState | Every CLI | Low — manual fix needed | Small | naming.TrimCLISuffix already exists |
| F2 | Phase 5 skips no-auth APIs | `skills/printing-press/SKILL.md` Phase 5 | Every no-auth CLI (~30% of library) | Low — user must manually request | Small | Check auth.type in spec |

### Skip
| Finding | Title | Why |
|---------|-------|-----|
| F3 | Dual-API client pattern | Same as movie-goat F8. Low frequency, small manual effort. |

## Work Units

### WU-1: Fix double -pp-cli suffix in NewMinimalState (from F1)
- **Goal:** `lock promote --cli hackernews-pp-cli` produces correct manifest with `api_name: "hackernews"` and `cli_name: "hackernews-pp-cli"`
- **Target:** `internal/pipeline/state.go` line 309
- **Acceptance criteria:**
  - positive: `NewMinimalState("hackernews-pp-cli", dir).APIName == "hackernews"`
  - positive: `NewMinimalState("notion-pp-cli", dir).APIName == "notion"`
  - negative: `NewMinimalState("notion", dir).APIName == "notion"` (no over-strip)
- **Scope boundary:** Only changes NewMinimalState. Does not change NewState or other state constructors.
- **Dependencies:** None
- **Complexity:** Small (one line + test)

### WU-2: Fix Phase 5 auto-skip for no-auth APIs (from F2)
- **Goal:** No-auth APIs get mandatory live dogfood testing instead of auto-skip
- **Target:** `skills/printing-press/SKILL.md` Phase 5 section
- **Acceptance criteria:**
  - positive: HN (auth.type: none) → Phase 5 runs without user asking
  - positive: Stripe (auth required, no key) → Phase 5 still auto-skips
  - positive: Stripe (auth required, key available) → Phase 5 runs
- **Scope boundary:** Only changes the auto-skip condition. Does not change test structure or depth options.
- **Dependencies:** None
- **Complexity:** Small (skill text change)

## Anti-patterns
- **Don't equate "no API key" with "can't test."** Auth and testability are different axes. No-auth APIs are the most testable.
- **Don't set APIName from a CLI name.** CLI names have the `-pp-cli` suffix. API slugs don't. Always strip before storing as APIName.

## What the Printing Press Got Right
- **Dual-API spec worked cleanly.** Firebase endpoints generated correctly from the internal spec. Algolia was hand-built but the generated foundation (client, store, helpers, root.go) provided solid scaffolding.
- **Write-through caching worked out of the box.** The template fix from the movie-goat retro was already in place — HN benefits automatically.
- **Sync page ceiling present.** No runaway pagination risk.
- **10 transcendence commands built and verified live.** `since`, `pulse`, `hiring`, `controversial`, `repost`, `my`, `tldr`, `comments`, `show --hot`, `ask --topic` all work against the real API.
- **87/100 Grade A on first shipcheck.** Strong baseline from the generator.
