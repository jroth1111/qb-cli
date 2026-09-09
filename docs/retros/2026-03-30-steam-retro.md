# Printing Press Retro: Steam API

## Session Stats
- API: Steam Web API (player profiles, game libraries, achievements, friends, news)
- Spec source: Crowd-sniff (v2, with auto-discovered params) — zero OpenAPI spec
- Scorecard: 73 (v1, manual params) → 70 (v2, auto params + generator improvements)
- Verify pass rate: 31% (8/26 pass; 9 parent commands fail, 9 positional-arg commands fail in mock)
- Fix loops: 1
- Manual code edits: 4 (auth config, resource restructuring, root description, input_json wrapping)
- Features built from scratch: 13 (10 top-level commands + 3 sync subcommands)
- Time to ship: ~20 min (v2 run, reusing v1 research)

## Findings

### 1. Crowd-sniff produces flat Interface/Method resource names with slashes (bug)
- **What happened:** Crowd-sniff output had resource names like `ISteamUser/GetPlayerSummaries` which contain `/`. The generator tried to create directories matching these names, causing `open /path/ISteamUser/GetPlayerSummaries.go: no such file or directory`.
- **Root cause:** `internal/crowdsniff/specgen.go` — the spec builder uses the raw endpoint path components as resource names without grouping methods under their interface.
- **Cross-API check:** Standard REST (Stripe): endpoints are `/v1/charges`, `/v1/customers` — crowd-sniff would produce `v1/charges` with slashes. Sniffed API: same pattern if paths have segments. This hits ANY API where crowd-sniff discovers paths with more than one segment.
- **Frequency:** Most APIs. Any API with path-structured endpoints (which is almost all of them).
- **Fallback if machine doesn't fix it:** Claude runs a Python script to restructure the spec (~3 min). But if forgotten, the generator crashes — **critical fallback**.
- **Tradeoff:** freq(3) × fallback(4) / effort(1) + risk(0) = 12.0. High priority.
- **Inherent or fixable:** Fixable. The spec builder should group endpoints by their first path segment (the interface/resource name) and use the method as the endpoint key within that resource.
- **Durable fix:** In `specgen.go`, when building the resource map, split the endpoint path and use the first significant segment as the resource name. Group all methods sharing that prefix under one resource. For Steam: `/ISteamUser/GetPlayerSummaries/v2` → resource `ISteamUser`, endpoint `get_player_summaries`.
  - Condition: Always active — resource names should never contain `/`
  - Guard: None needed — this is a universal fix
- **Test:** Generate a crowd-sniff spec from Steam → resource names have no slashes. Generate from a standard REST API → resources are clean path segments.
- **Evidence:** Generator crash on first attempt; Python restructuring script needed.

### 2. Crowd-sniff doesn't detect auth patterns from SDK code (missing scaffolding)
- **What happened:** Crowd-sniff set `auth.type: none` for Steam even though every npm SDK passes `key=XXX` as a query parameter. The auth config had to be manually edited to `type: api_key`, `in: query`, `header: key`.
- **Root cause:** `internal/crowdsniff/npm.go` — the npm analyzer extracts endpoint paths and (now) params, but doesn't look for authentication patterns in the SDK code.
- **Cross-API check:** Standard REST (Stripe): SDKs use `Authorization: Bearer sk_...` — detectable. Sniffed API: auth varies. Most npm SDKs set auth in their constructor or per-request headers. This applies to most APIs that need auth.
- **Frequency:** Most APIs. The crowd-sniff param discovery doc (`manuscripts/postman-explore/20260330-105847/proofs/2026-03-30-crowd-sniff-param-discovery-gap.md`) already identified auth pattern detection as a gap.
- **Fallback if machine doesn't fix it:** Claude edits 4 lines in the spec YAML (~1 min). Low cost per edit, but if missed the CLI generates with no auth wiring and every API call returns 401 — **critical if missed**.
- **Tradeoff:** freq(3) × fallback(3) / effort(2) + risk(0) = 4.5. Medium priority.
- **Inherent or fixable:** Fixable. SDK code reveals auth patterns: look for constructor params like `new SteamAPI(key)`, header setting like `headers: { 'X-Api-Key': key }`, or query param patterns like `params.key = this.key`. The param discovery infrastructure (multi-line brace scanner) already exists to extract these.
- **Durable fix:** In `npm.go`, after extracting endpoints and params, scan the SDK source for auth patterns:
  1. Constructor: look for `constructor(apiKey)` or `new Client(key)` → detect key param name
  2. Header setting: look for `headers[...] = ` patterns → detect header name and format
  3. Query param: look for `params.key = ` or `{ key: this.apiKey }` → detect `in: query`
  4. Write detected auth into the spec's `auth` section
  - Condition: Only when auth patterns are detected in SDK source
  - Guard: Don't override if the user passed `--auth` explicitly
  - Frequency estimate: ~80% of APIs need auth; ~60% of npm SDKs have extractable auth patterns
- **Test:** Run crowd-sniff for Steam → spec has `auth.type: api_key`, `auth.in: query`. Run for a bearer-token API → spec has `auth.type: bearer_token`.
- **Evidence:** Manual edit to spec; every npm SDK for Steam passes `key` as query param.

### 3. IPlayerService "Service interface" needs input_json wrapping (assumption mismatch)
- **What happened:** Steam's "Service" interfaces (IPlayerService, IGameServersService) require params wrapped as `input_json={"steamid":"xxx"}` URL-encoded, not as regular query params. Every top-level command calling these interfaces needed manual `input_json` wrapping code.
- **Root cause:** The spec has no way to express "this endpoint requires input_json wrapping" — it's a Steam-specific convention for Valve's "Service" interfaces.
- **Cross-API check:** Standard REST (Stripe): no. Sniffed APIs: unlikely. This is specific to Steam's Service interface pattern. Potentially other Valve APIs (Dota 2, CS2) use the same pattern.
- **Frequency:** This API only (Steam/Valve family). The `input_json` convention is Valve-specific.
- **Fallback if machine doesn't fix it:** Claude writes wrapper code for each command (~5 min total, medium cost). Not critical — the commands work, just with a different param format.
- **Tradeoff:** freq(1) × fallback(2) / effort(2) + risk(1) = 0.67. Low priority — too narrow.
- **Inherent or fixable:** Inherent to the Steam API's design. Not worth a machine fix for a single API family. The skill instruction to handle Service interfaces during Phase 3 is the right level.
- **Durable fix:** Skip machine fix. Document in the skill as a known Steam API quirk that Claude handles during Phase 3. If Valve APIs become a significant fraction of the catalog, revisit.
- **Test:** N/A — this is a skip recommendation.
- **Evidence:** All IPlayerService commands needed manual input_json wrapping.

### 4. dogfood/verify reject Python-restructured YAML specs (tool limitation)
- **What happened:** After restructuring the spec with Python's `yaml.dump`, dogfood and verify returned "at least one resource is required" — they couldn't parse the resources. The generator and scorecard (via JSON conversion) accepted the same spec.
- **Root cause:** Python's `yaml.dump` produces slightly different YAML formatting than Go's YAML libraries expect. The spec parser in `internal/spec/reader.go` is stricter about some YAML conventions (possibly indentation or string quoting) than the generator's spec loading path.
- **Cross-API check:** This affects any spec modified by non-Go tools (Python, JS). Since crowd-sniff produces specs via Go, the issue only arises when Python post-processes the spec (as we did for resource restructuring). If finding #1 is fixed (no restructuring needed), this issue goes away.
- **Frequency:** API subclass: crowd-sniff specs that need post-processing. Currently 100% of crowd-sniff specs need restructuring, so effectively "most APIs using crowd-sniff."
- **Fallback if machine doesn't fix it:** Convert to JSON for dogfood/verify (~30 sec). Low cost.
- **Tradeoff:** freq(2) × fallback(1) / effort(1) + risk(0) = 2.0. Low priority — especially if finding #1 eliminates the need for Python restructuring.
- **Inherent or fixable:** Mostly eliminated by fixing #1. If specs never need Python restructuring, the Go-generated YAML always works. Residual fix: make the spec parser more lenient about YAML style variations.
- **Durable fix:** Primary: fix #1 (resource naming in specgen.go). Residual: add a YAML normalization pass in `reader.go` that canonicalizes the YAML before parsing.
- **Test:** Run dogfood with a crowd-sniff-generated spec → passes without JSON conversion.
- **Evidence:** dogfood/verify rejected the restructured YAML; scorecard accepted JSON version.

### 5. Crowd-sniff misses params for endpoints not in popular SDKs (missing scaffolding)
- **What happened:** GetNewsForApp, GetOwnedGames, and several other endpoints had `params: []` even after v2 param discovery. The `steamapi` npm package doesn't expose all Steam endpoints — less popular endpoints aren't covered.
- **Root cause:** `internal/crowdsniff/npm.go` — param discovery only works for endpoints that appear in the npm packages' source code. Endpoints not covered by any SDK get no params.
- **Cross-API check:** Standard REST (Stripe): Stripe's npm SDK is comprehensive, covering ~95% of endpoints. Less popular APIs with smaller SDKs will have more gaps. This affects APIs where community coverage is patchy.
- **Frequency:** Most APIs using crowd-sniff. The coverage depends on SDK quality.
- **Fallback if machine doesn't fix it:** Claude adds missing params manually from docs (~5 min). Medium cost.
- **Tradeoff:** freq(3) × fallback(2) / effort(3) + risk(1) = 1.5. Low priority — the fallback is manageable and the fix is complex (would need a secondary source like Steamworks docs).
- **Inherent or fixable:** Partially inherent — crowd-sniff's value is community-driven discovery, and coverage gaps are expected. The fix is to add a secondary param source: scrape the official docs page to fill gaps. This is a significant new capability.
- **Durable fix:** Add a `--docs-fallback <url>` flag to crowd-sniff that scrapes official API docs to fill param gaps for endpoints where npm/GitHub produced no params. Lower priority than the resource naming fix.
- **Test:** Run crowd-sniff for Steam with docs fallback → GetNewsForApp has `appid` param.
- **Evidence:** GetNewsForApp `--appid 570` failed because the flag didn't exist.

### 6. Dead code scores 0/5 on every generation (recurring friction)
- **What happened:** Scorecard dead_code: 0/5. The generator emits helpers like `classifyDeleteError`, `firstNonEmpty`, `printOutputFiltered` that no command calls.
- **Root cause:** `internal/generator/templates/helpers.go.tmpl` emits all utility functions unconditionally.
- **Cross-API check:** Every API. This is the same finding as the postman-explore retro (#7).
- **Frequency:** Every API. 0/5 dead code score on both postman-explore and steam.
- **Fallback if machine doesn't fix it:** Claude deletes 3-5 functions (<2 min). Low cost.
- **Tradeoff:** freq(4) × fallback(1) / effort(2) + risk(1) = 1.3. Same as before.
- **Inherent or fixable:** Same analysis as postman-explore retro: partially inherent, mitigated by `printing-press polish --dead-code` or conditional emission.
- **Durable fix:** Already planned in the postman-explore retro (Tier 3, backlog). No new action needed.
- **Evidence:** 0/5 dead code on both postman-explore (85/100) and steam (70/100).

### 7. Top-level commands always built from scratch (template gap)
- **What happened:** 10 top-level commands (player, games, friends, achievements, news, players, resolve, search, compare, completionist) were written from scratch. The generator only produces `ISteamUser get_player_summaries` nested commands.
- **Root cause:** Same as postman-explore retro finding #1 — the generator derives command structure from spec paths, not user-facing UX patterns.
- **Cross-API check:** Every API. Same finding, same analysis as postman-explore retro.
- **Frequency:** Every API. Confirmed across two different APIs now (Postman Explore and Steam).
- **Fallback if machine doesn't fix it:** Claude writes 5-10 command files (~15-20 min). High cost.
- **Tradeoff:** freq(4) × fallback(3) / effort(3) + risk(2) = 2.4. Same score as before, but now confirmed on a second API.
- **Inherent or fixable:** Same analysis. The "command promotion" approach from the postman-explore retro plan is the right fix. Now validated on a second API.
- **Durable fix:** Already planned as Tier 2 in the postman-explore retro.
- **Evidence:** 10 commands written from scratch for Steam, 7 for Postman Explore — same pattern.

## Prioritized Improvements

### Tier 1: Do Now
| # | Fix | Component | Frequency | Fallback Cost | Effort | Guards |
|---|-----|-----------|-----------|--------------|--------|--------|
| 1 | Fix crowd-sniff resource naming (no slashes) | `internal/crowdsniff/specgen.go` | most | critical (generator crash) | 4 hrs | none — universal |

### Tier 2: Plan
| # | Fix | Component | Frequency | Fallback Cost | Effort | Guards |
|---|-----|-----------|-----------|--------------|--------|--------|
| 2 | Auth pattern detection in crowd-sniff | `internal/crowdsniff/npm.go` | most | critical if missed, medium per edit | 2 days | only when auth patterns detected |
| 7 | Top-level command promotion | `internal/generator/` | every | high (10+ files from scratch) | 3 days | keep `api` group as escape hatch |

### Tier 3: Backlog
| # | Fix | Component | Frequency | Fallback Cost | Effort | Guards |
|---|-----|-----------|-----------|--------------|--------|--------|
| 5 | Docs fallback for missing params | `internal/crowdsniff/` | most (crowd-sniff) | medium (manual param adds) | 1 week | `--docs-fallback` flag |
| 6 | Dead code emission | `internal/generator/templates/helpers.go.tmpl` | every | low (delete 3 funcs) | 1 day | conditional emission |
| 4 | YAML format normalization | `internal/spec/reader.go` | subclass: crowd-sniff | low (JSON convert) | 4 hrs | none |

### Skip
| # | Fix | Reason |
|---|-----|--------|
| 3 | IPlayerService input_json | Steam/Valve-only quirk. Not worth machine complexity for one API family |

## Work Units

### WU-1: Fix crowd-sniff resource naming (finding #1)
- **Goal:** Crowd-sniff outputs specs with clean resource names (no slashes) by grouping methods under their interface/resource prefix.
- **Target files:**
  - `internal/crowdsniff/specgen.go` — resource grouping logic
  - `internal/crowdsniff/specgen_test.go` — tests
- **Acceptance criteria:**
  - Run crowd-sniff for Steam → resource names are `ISteamUser`, `IPlayerService` (no slashes)
  - Each resource has multiple endpoints grouped under it
  - Generator accepts the spec without Python restructuring
  - dogfood/verify accept the spec directly (no JSON conversion)
  - Run crowd-sniff for a standard REST API → resources are clean path segments (e.g., `users`, `charges`)
- **Scope boundary:** Does NOT include auth detection or param gap filling
- **Effort:** 4 hours

### WU-2: Auth pattern detection in crowd-sniff (finding #2)
- **Goal:** Crowd-sniff detects authentication patterns from npm SDK source code and populates the spec's auth section.
- **Target files:**
  - `internal/crowdsniff/npm.go` — auth extraction alongside endpoint/param extraction
  - `internal/crowdsniff/types.go` — add auth pattern type
  - `internal/crowdsniff/aggregate.go` — merge auth patterns across sources
  - `internal/crowdsniff/specgen.go` — write auth into spec
  - `internal/crowdsniff/npm_test.go` — tests
- **Acceptance criteria:**
  - Run crowd-sniff for Steam → spec has `auth.type: api_key`, `auth.in: query`, `auth.header: key`, `auth.env_vars: [STEAM_API_KEY]`
  - Run crowd-sniff for a bearer-token API → spec has `auth.type: bearer_token`
  - Run crowd-sniff for a no-auth API → spec has `auth.type: none` (unchanged)
- **Scope boundary:** Does NOT include OAuth flow detection (complex multi-step). Focuses on simple patterns: API key (query/header), bearer token, basic auth.
- **Dependencies:** None
- **Effort:** 2 days

## Anti-patterns

- **"Python yaml.dump is interchangeable with Go yaml"** — It's not. Go's strict YAML parser rejects some valid YAML that Python produces. Avoid round-tripping specs through Python when Go tools need to consume them. Fix the Go code to produce correct specs from the start.
- **"Crowd-sniff is done when it finds endpoints"** — Endpoints without params, auth, and clean resource names aren't usable. Crowd-sniff needs to be a complete spec builder, not just an endpoint discoverer.
- **"The generator improvements from the postman-explore retro are validated"** — They are! Pagination-aware sync, batch upsert, DB path consolidation, and query param auth all worked correctly on the Steam run. But top-level command promotion and dead code fixes are still unresolved — confirmed on a second API.

## What the Machine Got Right

- **Crowd-sniff v2 param discovery** — Discovered 25 params across 17 endpoints from npm SDK source code. Eliminated 100% of manual param editing (vs 24 manual params in v1). The multi-line brace scanner and function signature cross-referencing worked as designed.
- **Generator improvements from retro** — All five fixes from the postman-explore retro worked on first try: pagination-aware sync generated correctly, batch upsert methods generated, single defaultDBPath(), query param auth (`in: query`) wired correctly, proxy route propagation verified.
- **Prior research reuse** — Phase 0 found manuscripts from the v1 run and reused the brief, absorb manifest, and crowd-sniff spec. Research phase took ~0 minutes instead of ~10.
- **Manuscript archiving (Phase 5.5)** — The fix we made earlier (archive unconditionally after shipcheck) worked — v1 manuscripts were available for v2 to reuse.
- **Live API verification** — 6/6 live tests passed. The auth, params, and endpoint routing all worked against the real Steam Web API with Gabe Newell's profile as test data.
