# Printing Press Retro: Cal.com

## Session Stats
- API: cal-com
- Spec source: catalog (OpenAPI 3.0.0, 285 endpoints, 181 unique paths)
- Scorecard: 83/100 (Grade A)
- Verify pass rate: 97% (32/33)
- Fix loops: 2
- Manual code edits: 7 (SQL reserved words, event-types version, busy command, search, tail, stale dry-run)
- Features built from scratch: 0 (all from generator + data layer template)

## Findings

### 1. Verbose OperationId-Derived Filenames (Template gap)
- **What happened:** Cal.com's OpenAPI spec includes version dates and controller class names in operationIds (e.g., `BookingsController_2024-08-13_getBooking`). The generator preserves these verbatim, producing 365 Go files with filenames up to 102 characters (`organizations_attributes_organizations-options-controller-get-organization-assigned-options-by-slug.go`). Command names in the CLI are equally verbose (`bookings controller-2024-08-13-get-bookinguid`).
- **Scorer correct?** N/A — no direct score penalty, but affects usability and DX.
- **Root cause:** `internal/openapi/parser.go` derives endpoint names directly from operationIds without stripping version date patterns (`2024-08-13`), controller class suffixes (`-controller`), or redundant resource prefixes.
- **Cross-API check:** Any API with versioned operationIds or verbose controller naming conventions. Cal.com, Stripe, and many auto-generated OpenAPI specs from NestJS/FastAPI include controller class names.
- **Frequency:** API subclass: versioned-operationId APIs
- **Fallback if the Printing Press doesn't fix it:** Claude strips names during polish, but 280+ files makes this unreliable. 30% chance some verbose names survive.
- **Worth a Printing Press fix?** Yes. The generated command tree should have human-friendly names.
- **Inherent or fixable:** Fixable. The parser can detect and strip common patterns: `*-controller-*`, date patterns like `YYYY-MM-DD`, and redundant resource name prefixes.
- **Durable fix:** OpenAPI parser (`internal/openapi/parser.go`) should normalize operationIds: (1) strip `-controller` and class name prefixes, (2) strip version date patterns when they appear in the middle of names, (3) deduplicate resource name from endpoint name when the endpoint is already nested under that resource.
- **Test:** Positive: Cal.com's `BookingsController_2024-08-13_getBooking` → `get` (under bookings). Negative: `getBookingByUid` stays as-is (no controller/version pattern).
- **Evidence:** 365 generated files, 20+ filenames exceeding 80 characters.

### 2. Wiring Check False Positives on Deeply Nested Commands (Scorer bug + Template gap)
- **What happened:** Dogfood reports 16 unregistered commands out of 284 defined (6%). Many are sub-commands at depth 2-3 under `organizations` (e.g., `attributes`, `ooo`, `roles`, `routing-forms`, `transcripts`). The `organizations` parent IS registered in root.go, and these commands ARE wired as children. The wiring check's help-output parsing can't reliably detect them.
- **Scorer correct?** Partially. The wiring check has genuine false positives: commands that are properly wired but not detected by the `--help` text-scraping approach. However, some may be genuine wiring gaps — the `organizations > organizations` naming collision (parent and child share the same `Use:` name) creates ambiguity.
- **Root cause:** (Scorer) `internal/pipeline/dogfood.go` lines 949-1066 — `extractCommandNames()` and `commandFoundInHelp()` use regex-based help output scraping with 4-level recursion. Short sub-command names like `ooo`, `roles` either don't appear distinctly in help text or the recursion doesn't traverse all branches reliably. (Template) The generator can produce parent/child commands with identical `Use:` names (both called `organizations`), which confuses help-based detection.
- **Cross-API check:** Any API with 3+ levels of resource nesting. Organizations, teams, and projects are common. Cal.com, GitHub, Stripe all have deep hierarchies.
- **Frequency:** Most APIs with nested resources (>50% of catalog APIs have 3+ nesting levels)
- **Fallback if the Printing Press doesn't fix it:** Manual verification. Unreliable — 16 out of 284 is noise.
- **Worth a Printing Press fix?** Yes — both the scorer and the generator.
- **Inherent or fixable:** Fixable. The wiring check should use the compiled binary's cobra command tree (via a `__complete` or hidden introspection subcommand) instead of parsing `--help` text.
- **Durable fix:** (Scorer) Replace help-text scraping with compiled-binary introspection: build the CLI, run a hidden `__commands` subcommand that walks the cobra tree and prints all registered commands. (Template) When parent and child would share the same `Use:` name, suffix the child (e.g., `managed` instead of `organizations`).
- **Test:** Positive: 284 defined = 284 registered for cal.com. Negative: a genuinely unwired command (deleted AddCommand line) is still caught.
- **Evidence:** Dogfood wiring_check shows 16 unregistered commands; manual inspection confirms `organizations` parent IS registered and children ARE wired.

### 3. Token Masking Missing from Client Template (Template gap)
- **What happened:** Auth scored 8/10. The scorer checks `client.go` for token masking patterns (`mask`, `***`, `last 4`, `Authorization` with substring). Cal.com's client.go has no masking — tokens could appear in debug/error output unredacted.
- **Scorer correct?** Yes. The client.go genuinely lacks token masking.
- **Root cause:** `internal/generator/templates/client.go.tmpl` doesn't include a `maskToken()` helper or apply masking when logging requests or surfacing auth errors.
- **Cross-API check:** Every generated CLI will be missing token masking.
- **Frequency:** Every API.
- **Fallback if the Printing Press doesn't fix it:** Claude adds masking during polish ~50% of the time. Inconsistent.
- **Worth a Printing Press fix?** Yes — security concern + 2 scorecard points on every CLI.
- **Inherent or fixable:** Fixable. Simple template addition.
- **Durable fix:** Add a `maskToken(token string) string` function to `client.go.tmpl` that shows only the last 4 characters (e.g., `cal_****chow`). Use it in any error message or debug output that includes the auth header.
- **Test:** Positive: `maskToken("cal_live_abc123xyz")` → `cal_****xyz"`. Negative: empty token returns "".
- **Evidence:** Auth scored 8/10; grep for `mask|***|last 4` in client.go returns 0 matches.

### 4. Dead Helper Functions in Generated Code (Template gap)
- **What happened:** Dead Code scored 3/5. The scorecard detected 2 helper functions in `helpers.go` that are defined but never called from any other file. The helpers template emits all 43+ utility functions regardless of which ones the specific API's commands actually use.
- **Scorer correct?** Yes. The functions exist and are uncalled. Template infrastructure, but still dead code.
- **Root cause:** `internal/generator/templates/helpers.go.tmpl` emits a fixed set of helper functions. Some are only needed by certain API patterns (e.g., `classifyDeleteError` only needed when delete commands exist, `camelToKebab` only needed for header normalization).
- **Cross-API check:** Every generated CLI will have some dead helpers — the exact count varies by API shape.
- **Frequency:** Every API (1-5 dead functions depending on which helpers the API's commands actually use).
- **Fallback if the Printing Press doesn't fix it:** Dead code is harmless to functionality but costs 1-2 scorecard points.
- **Worth a Printing Press fix?** Yes, but lower priority. The fix is conditional emission based on what the generated commands actually import.
- **Inherent or fixable:** Fixable but medium complexity. The generator needs to track which helpers are referenced by the generated command files and only emit those.
- **Durable fix:** During generation, build a set of used helper functions by scanning generated command file content. In the helpers template, wrap each function in a conditional: `{{if .UsesHelper "classifyDeleteError"}}...{{end}}`. Alternatively, post-generation `go vet` + dead-code removal as a binary post-processing step.
- **Test:** Positive: CLI with no delete commands → `classifyDeleteError` not emitted. Negative: CLI with delete commands → `classifyDeleteError` IS emitted.
- **Evidence:** scorecard Dead Code 3/5; 43 functions in helpers.go, ~2 unused in cal.com context.

### 5. Per-Endpoint API Version Routing Lost by Global Promotion (Assumption mismatch)
- **What happened:** Cal.com uses `cal-api-version` headers with DIFFERENT values per endpoint group (bookings: `2024-08-13`, event-types: `2024-06-14`). The generator's `detectRequiredHeaders()` promotes headers appearing on >80% of endpoints to a global default, picking the majority value. Event-types endpoints then get the wrong version header. Required manual fix.
- **Scorer correct?** N/A — detected during runtime verify, not a scorecard dimension.
- **Root cause:** `internal/openapi/parser.go` `detectRequiredHeaders()` (lines 433-512) uses a frequency threshold to promote required headers to global. When multiple values exist for the same header name, it picks the most common one and discards the rest.
- **Cross-API check:** Any API with per-resource versioning. Cal.com, Twilio, and some enterprise APIs use this pattern.
- **Frequency:** API subclass: per-endpoint-versioned APIs (~10% of catalog)
- **Fallback if the Printing Press doesn't fix it:** Claude fixes during verify loop — caught reliably because the wrong version causes 400 errors. But it costs a fix loop iteration.
- **Worth a Printing Press fix?** Yes — eliminates a guaranteed fix loop for affected APIs.
- **Inherent or fixable:** Fixable. When multiple values exist for the same required header, track per-endpoint values instead of promoting one global.
- **Durable fix:** In the OpenAPI parser, when a required header has multiple distinct values, store a map of `path → header-value` alongside the global default. The command template should set the per-endpoint override when it differs from global.
- **Test:** Positive: Cal.com event-types endpoints get `cal-api-version: 2024-06-14`. Negative: A single-version API still uses the global header without per-endpoint overrides.
- **Evidence:** Build log: "Fixed event-types API endpoint: uses cal-api-version: 2024-06-14 not 2024-08-13"

### 6. Verify Cannot Test Data-Layer-Dependent Commands (Missing scaffolding)
- **What happened:** `stale` fails both dry-run and runtime in verify (1/3 score). `tail` fails runtime (2/3). These commands depend on populated local SQLite data that verify's mock mode doesn't provide. Verify creates a mock HTTP server but never seeds the local store.
- **Scorer correct?** N/A — verify failures, not scorecard.
- **Root cause:** `internal/pipeline/verify.go` — the verify infrastructure has `verifyInfraFiles` exemptions for some commands (tail, sync, analytics) but doesn't seed the local store for data-layer commands that query it (stale, today, no-show, busy).
- **Cross-API check:** Every CLI with a data layer will have data-dependent commands that fail verify.
- **Frequency:** Every API with a data layer (every CLI with sync/search/transcendence commands).
- **Fallback if the Printing Press doesn't fix it:** These failures get documented as "expected" and excluded from pass rate. But they inflate false failure counts and mask real issues.
- **Worth a Printing Press fix?** Yes — reduces noise in verify results and catches real bugs in data-layer commands.
- **Inherent or fixable:** Fixable. Verify can seed the store with synthetic data before testing data-dependent commands.
- **Durable fix:** Add a `--seed-store` mode to verify that: (1) runs `sync` against the mock server to populate the store, (2) THEN tests data-layer commands against the populated store. Alternatively, add `stale`, `today`, `noshow`, `busy` to `verifyInfraFiles` exemption list as a quick fix.
- **Test:** Positive: `stale --days 3` passes verify when store is seeded with bookings older than 3 days. Negative: `stale` still fails correctly when there's a genuine code bug (e.g., wrong SQL query).
- **Evidence:** Verify results: stale 1/3, tail 2/3. Shipcheck notes: "needs populated local DB, fails in mock mode (works with real DB)."

### 7. Generator 50-Endpoint Limit per Resource (Default gap)
- **What happened:** Organizations.teams exceeded the 50-endpoint limit, causing ~20 endpoints to be skipped. This is a hard-coded safety limit in the generator.
- **Scorer correct?** N/A — not scored, just missing coverage.
- **Root cause:** Hard-coded limit in `internal/generator/generator.go` designed to prevent runaway generation from badly structured specs.
- **Cross-API check:** Large APIs with deeply nested resources. Cal.com organizations has 50+ endpoints. Shopify Admin API would also hit this.
- **Frequency:** API subclass: APIs with >50 endpoints per resource (~15% of catalog, growing)
- **Fallback if the Printing Press doesn't fix it:** Claude notices missing commands during the absorb audit and adds them manually. Reliable but costs time.
- **Worth a Printing Press fix?** Yes — the limit should be configurable, not hard-coded.
- **Inherent or fixable:** Fixable. Make the limit a flag or config option.
- **Durable fix:** Change the 50-endpoint limit to a flag (`--max-endpoints-per-resource`, default 50). When the limit is hit, log a warning listing skipped endpoints so the user can raise it.
- **Test:** Positive: `--max-endpoints-per-resource 100` generates all Cal.com organizations.teams endpoints. Negative: default 50 still prevents runaway generation for a malformed 500-endpoint resource.
- **Evidence:** Build log: "50-endpoint limit per resource (hit for organizations.teams - ~20 endpoints skipped)"

### 8. Auth Protocol Inference from Spec Missing Security Schemes (Assumption mismatch)
- **What happened:** Cal.com's spec doesn't declare auth via OpenAPI `securitySchemes`. Instead, it uses a required `Authorization` header parameter on individual endpoints. The generator can't infer the auth type, reporting "unknown" in doctor and costing potential auth score points.
- **Scorer correct?** Yes for the auth score itself — the CLI genuinely doesn't declare the protocol. But the root cause is the spec, not the CLI.
- **Root cause:** `internal/openapi/parser.go` reads `securitySchemes` from the spec for auth type detection. When absent, it falls back to "unknown". It doesn't scan for Authorization header parameters as a fallback signal.
- **Cross-API check:** APIs with auto-generated or minimal specs that declare auth as a header parameter rather than via securitySchemes. Common in internal APIs and some SaaS vendors.
- **Frequency:** API subclass: APIs without proper securitySchemes (~20% of catalog specs)
- **Fallback if the Printing Press doesn't fix it:** Claude sets auth type during the research/brief phase based on API docs. Reliable but adds a manual step.
- **Worth a Printing Press fix?** Yes — simple heuristic improvement.
- **Inherent or fixable:** Fixable. When `securitySchemes` is empty, scan for required `Authorization` header parameters and infer Bearer/Basic from the parameter description or default value.
- **Durable fix:** In `internal/openapi/parser.go`, after the securitySchemes check, add a fallback: scan all operations for a required header named `Authorization`. If found, check its description or schema for "Bearer" or "Basic" keywords to infer the auth type.
- **Test:** Positive: Cal.com spec (no securitySchemes, has Authorization header with "Bearer" in description) → infers Bearer. Negative: spec WITH securitySchemes → uses those, doesn't fall back.
- **Evidence:** Doctor output: "Auth: inferred (configured — verify header and env var are correct)". Shipcheck notes: "Auth protocol shows 'unknown': spec doesn't declare Bearer scheme explicitly."

## Prioritized Improvements

### Fix the Scorer
| # | Scorer | Bug | Impact | Fix target |
|---|--------|-----|--------|------------|
| 2 | Dogfood wiring check | Help-text scraping misses deeply nested commands — 16 false positives on cal.com | Inflates unregistered count on every API with 3+ nesting levels | `internal/pipeline/dogfood.go` lines 949-1066 |

### Do Now
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 3 | Token masking in client template | Generator templates (`client.go.tmpl`) | Every API | ~50% | small | None needed |
| 4 | Conditional helper function emission | Generator templates (`helpers.go.tmpl`) | Every API | Harmless but -1-2 pts | medium | Track used helpers during generation |
| 6 | Verify store seeding for data-layer commands | Pipeline (`verify.go`) | Every API with data layer | Documented as "expected" | medium | Only seed when CLI has sync command |

### Do Next
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 1 | OperationId normalization (strip controller names, version dates) | OpenAPI parser (`parser.go`) | Versioned-operationId APIs (~20%) | ~70% via polish | medium | Only strip patterns matching `*-controller-*` or `YYYY-MM-DD` within names |
| 5 | Per-endpoint API version header routing | OpenAPI parser (`parser.go`) | Per-endpoint-versioned APIs (~10%) | ~90% via verify loop | medium | Only when multiple values exist for same header |
| 7 | Configurable endpoint-per-resource limit | Generator (`generator.go`) | Large APIs (~15%) | ~80% via manual add | small | Flag with default 50 |
| 8 | Auth inference from Authorization header parameters | OpenAPI parser (`parser.go`) | APIs without securitySchemes (~20%) | ~90% via brief | small | Only when securitySchemes is empty |

### Skip
| # | Fix | Why unlikely to recur |
|---|-----|----------------------|
| — | SQL reserved words in schema | Already fixed — `safeSQLName()` in `schema_builder.go` and `{{safeName}}` in `store.go.tmpl` now handle this. Verify on next generation. |
| — | README template missing sections | Already fixed — template now includes Agent Usage, Cookbook, Health Check, Troubleshooting. Verify on next generation. |

## Work Units

### WU-1: Client Token Masking (from finding #3)
- **Goal:** Every generated CLI masks auth tokens in error messages and debug output
- **Target:** `internal/generator/templates/client.go.tmpl`
- **Acceptance criteria:**
  - positive test: generated client.go contains `maskToken` function; auth error messages show `****` with last 4 chars
  - negative test: empty/nil token returns empty string, not panic
- **Scope boundary:** Only masks in client output — does not affect config file storage
- **Dependencies:** None
- **Complexity:** small

### WU-2: Conditional Helper Emission (from finding #4)
- **Goal:** Helpers template only emits functions that generated commands actually use, eliminating dead code penalty
- **Target:** `internal/generator/templates/helpers.go.tmpl` and `internal/generator/generator.go` (to track used helpers)
- **Acceptance criteria:**
  - positive test: CLI without delete commands → no `classifyDeleteError` in helpers.go
  - negative test: CLI with delete commands → `classifyDeleteError` IS emitted; all existing CLIs still compile
- **Scope boundary:** Does NOT restructure helpers into separate files — just conditional blocks in the existing template
- **Dependencies:** None
- **Complexity:** medium

### WU-3: Verify Store Seeding (from finding #6)
- **Goal:** Verify can test data-layer commands (stale, today, no-show, busy) by seeding the local store with synthetic data
- **Target:** `internal/pipeline/verify.go`
- **Acceptance criteria:**
  - positive test: `stale --days 3` passes verify when store is seeded with old unconfirmed bookings
  - negative test: commands without data-layer dependency still work without seeding; genuinely buggy data-layer commands still fail
- **Scope boundary:** Does NOT replace mock HTTP server — seeding runs alongside it
- **Dependencies:** None
- **Complexity:** medium

### WU-4: Wiring Check Introspection (from finding #2)
- **Goal:** Dogfood wiring check uses compiled binary introspection instead of help-text scraping, eliminating false positives
- **Target:** `internal/pipeline/dogfood.go` lines 949-1066
- **Acceptance criteria:**
  - positive test: Cal.com's 284 defined commands = 284 registered (0 false positives)
  - negative test: a genuinely unwired command (deleted AddCommand) is still caught
- **Scope boundary:** Only changes the detection method — does not change the wiring_check output format
- **Dependencies:** None
- **Complexity:** medium

### WU-5: OperationId Normalization (from finding #1)
- **Goal:** Generated filenames and command names are human-friendly — no controller class names or embedded version dates
- **Target:** `internal/openapi/parser.go` (endpoint name derivation)
- **Acceptance criteria:**
  - positive test: `BookingsController_2024-08-13_getBooking` → command name `get` under `bookings` parent; filename `bookings_get.go`
  - negative test: `getBookingByUid` (no controller/version pattern) → stays as `get-booking-by-uid`
- **Scope boundary:** Only normalizes operationId-derived names — does not change the command hierarchy or API paths
- **Dependencies:** WU-4 (wiring check should work before testing new naming)
- **Complexity:** medium

### WU-6: Per-Endpoint Version Header Routing (from finding #5)
- **Goal:** When an API uses different version header values per endpoint, each command sends the correct version
- **Target:** `internal/openapi/parser.go` (header detection) and `internal/generator/templates/command_endpoint.go.tmpl` (header emission)
- **Acceptance criteria:**
  - positive test: Cal.com event-types commands send `cal-api-version: 2024-06-14`; bookings commands send `2024-08-13`
  - negative test: single-version API still uses global header without per-endpoint overrides
- **Scope boundary:** Does NOT change the spec data model — uses existing RequiredHeaders with per-endpoint values
- **Dependencies:** None
- **Complexity:** medium

## Anti-patterns
- **Scorecard-before-polish:** The scorecard was captured mid-generation, not after all fixes. Result: README showed 5/10 when the final README would score 10/10. The shipcheck should re-score as the last step after all fixes are applied.
- **Same-name parent/child commands:** `organizations > organizations` creates confusion for both the wiring check and end users. The generator should disambiguate when parent and child would share the same `Use:` name.

## What the Printing Press Got Right
- **Data layer generation:** The SQLite schema, sync, FTS5, and store methods were generated correctly. 7 transcendence commands (health, stale, today, noshow, busy, sql, search) worked with generated data layer code.
- **Agent-native flags:** `--json`, `--select`, `--dry-run`, `--compact`, `--agent`, `--data-source`, `--stdin`, `--yes`, `--no-cache` all generated and working.
- **Breadth coverage:** 284 commands generated from 285 endpoints — near-complete API coverage from a single spec.
- **Output mode flexibility:** Table, JSON, CSV, compact, plain, quiet modes all generated correctly.
- **Quality gates:** All 7 static gates passed on first build (go mod tidy, go vet, go build, binary, --help, version, doctor).
- **Verify auto-fix:** Loop 1 → Loop 2 improved pass rate from 94% → 97% automatically.
- **SQL reserved word quoting:** The `safeSQLName()` function and `{{safeName}}` template function handle SQL reserved words — this was fixed in the machine and works for future CLIs.
- **Per-endpoint versioning detection:** The generator correctly detected `cal-api-version` as a required header from the spec. The issue was only with per-endpoint VALUE variation, not detection itself.
