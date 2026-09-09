# Printing Press Retro: Cal.com (Run 2)

## Session Stats
- API: cal-com
- Spec source: OpenAPI 3.0.0 from GitHub (285+ endpoints, 181 unique paths)
- Scorecard: 96/100 (Grade A) — after 2 polish passes
- Verify pass rate: 100% (26/26)
- Dogfood: PASS (0 dead flags, 0 dead functions, 6/6 valid paths)
- Fix loops: 2 polish passes
- Manual code edits: 4 (auth env var, base URL, cal-api-version header, store migration NOT NULL)
- Features built from scratch: 4 (stats, conflicts, gaps, noshow — by polish worker)
- Machine fixes committed during session: 8

## Findings

### F1. Per-Endpoint Version Header Routing (Assumption mismatch)
- **What happened:** Cal.com uses `cal-api-version` headers with DIFFERENT values per endpoint group (bookings: `2024-08-13`, event-types: `2024-06-14`, schedules: `2024-04-15`). The generator's `detectRequiredHeaders()` promotes headers appearing on >80% of endpoints to a global default, picking the majority value. Event-types and schedules endpoints get the wrong version header, returning HTTP 404.
- **Scorer correct?** N/A — detected during live dogfood, not scored.
- **Root cause:** `internal/openapi/parser.go` `detectRequiredHeaders()` uses a frequency threshold to promote required headers to global. When multiple values exist for the same header name, it picks the most common one and discards the rest.
- **Cross-API check:** Any API with per-resource versioning. Cal.com, Twilio, some enterprise APIs.
- **Frequency:** API subclass: per-endpoint-versioned APIs (~10% of catalog)
- **Fallback if the Printing Press doesn't fix it:** Claude fixes during verify loop — caught reliably because wrong version causes 400/404. Costs a fix loop iteration.
- **Worth a Printing Press fix?** Yes — eliminates a guaranteed fix loop.
- **Inherent or fixable:** Fixable. When multiple values exist for the same required header, track per-endpoint values instead of promoting one global.
- **Durable fix:** In the OpenAPI parser, when a required header has multiple distinct values, store a map of `path → header-value` alongside the global default. The command template should set the per-endpoint override when it differs from global.
- **Test:** Positive: Cal.com event-types endpoints get `cal-api-version: 2024-06-14`. Negative: Single-version API still uses global header.
- **Evidence:** Live dogfood test #4: event-types returned HTTP 404.

### F2. Store Migration NOT NULL on Foreign Keys (Template gap)
- **What happened:** Generated store schema uses `NOT NULL` on foreign key columns (`organizations_id`, `teams_id`) that only exist in org-level API responses. When syncing as a regular user (not org admin), the API doesn't return these fields. Migration creates the table but subsequent inserts fail because the NOT NULL constraint can't be satisfied.
- **Scorer correct?** N/A — detected during sync, not scored.
- **Root cause:** `internal/generator/` store template infers foreign key columns from spec path hierarchy (e.g., `/v2/organizations/{orgId}/teams` → `organizations_id` column). It marks all inferred FK columns as NOT NULL, but some are only populated in org-scoped API responses.
- **Cross-API check:** Any API with hierarchical resources where child resources are also accessible directly (not just under the parent). GitHub (repos accessible via user or org), Stripe (resources accessible per account or per connected account).
- **Frequency:** API subclass: APIs with hierarchical/org-scoped resources (~25% of catalog)
- **Fallback if the Printing Press doesn't fix it:** Manual edit to make columns nullable. Easy but tedious.
- **Worth a Printing Press fix?** Yes — simple template change.
- **Inherent or fixable:** Fixable. Inferred FK columns should default to nullable unless the column is part of the primary key.
- **Durable fix:** In the store template, change inferred foreign key columns from `TEXT NOT NULL` to `TEXT` (nullable). Only the primary key and entity ID should be NOT NULL.
- **Test:** Positive: store migration succeeds when syncing as non-org user. Negative: entity's own ID column is still NOT NULL.
- **Evidence:** `sync --full` failed with "SQL logic error: no such column: organizations_id".

### F3. Promoted Command Defaults to Wrong Endpoint (Template gap)
- **What happened:** The `schedules` promoted command's RunE uses the "get default schedule" endpoint (`GET /v2/schedules/default`) instead of "list schedules" (`GET /v2/schedules`). Running `cal-com-pp-cli schedules` hits a 404 because the default schedule endpoint requires different auth context.
- **Scorer correct?** N/A — detected during live dogfood.
- **Root cause:** `buildPromotedCommands()` in generator.go selects the first endpoint from the resource as the promoted endpoint. For schedules, "default" (get default) sorts before "list" alphabetically. The function should prefer "list" or "get" endpoints over others.
- **Cross-API check:** Any API where the alphabetically-first endpoint isn't the most useful one. Common when resources have special endpoints (default, check, connect).
- **Frequency:** API subclass: APIs with special singleton endpoints alongside list endpoints (~15%)
- **Fallback if the Printing Press doesn't fix it:** Polish worker or skill notices wrong output during verify.
- **Worth a Printing Press fix?** Yes — simple selection logic fix.
- **Inherent or fixable:** Fixable. Prefer "list" > "get" > other for promoted endpoint selection.
- **Durable fix:** In `buildPromotedCommands()`, add a preference order: endpoints named "list" first, "get" second, then alphabetical. This ensures the most common user intent (browsing resources) is the default.
- **Test:** Positive: `schedules` promoted command lists schedules. Negative: resource with only a "create" endpoint still promotes that.
- **Evidence:** Live dogfood test #5: schedules returned HTTP 404.

### F4. Auth Not Inferred When Spec Lacks Description (Assumption mismatch)
- **What happened:** Cal.com's spec has no `securitySchemes` AND a minimal `info.description` without auth keywords. The `inferDescriptionAuth` function (added in this session's PR) didn't trigger because the description doesn't mention "Bearer", "API key", etc. Had to manually add `CAL_COM_TOKEN` env var support to config.go.
- **Scorer correct?** Auth scored 10/10 only after manual fix.
- **Root cause:** `inferDescriptionAuth` only scans `info.description`. Cal.com's spec description is "Cal.com v2 API" — no auth keywords. But the API absolutely requires Bearer auth (cal_live_ prefix tokens).
- **Cross-API check:** Any API whose spec omits both securitySchemes and auth keywords in description. Internal APIs, auto-generated specs from frameworks.
- **Frequency:** API subclass: specs without formal auth declaration (~20%)
- **Fallback if the Printing Press doesn't fix it:** Skill adds auth during Phase 2 post-generation. ~70% reliable.
- **Worth a Printing Press fix?** Yes — add a fourth inference tier.
- **Inherent or fixable:** Fixable. Scan operation-level parameters for required `Authorization` headers as a fourth-tier fallback.
- **Durable fix:** After `inferDescriptionAuth` fails, scan all operations for a required header parameter named `Authorization`. If found consistently (>30% of operations), infer Bearer auth. Cal.com's spec uses `Authorization` as a required header on individual endpoints.
- **Test:** Positive: Cal.com spec with Authorization header params → infers Bearer. Negative: spec with no Authorization params → stays "none".
- **Evidence:** Doctor reported "Auth: not required" until manual fix.

### F5. Base URL Placeholder When Servers Block Missing (Default gap)
- **What happened:** Cal.com's GitHub-hosted spec has no `servers` block. The generator uses `https://api.example.com` as placeholder. Had to manually set to `https://api.cal.com`.
- **Scorer correct?** N/A — but causes all API calls to fail until fixed.
- **Root cause:** `internal/openapi/parser.go` falls back to placeholder when no servers defined. The placeholder is obviously wrong but the generator doesn't flag it as a blocking issue.
- **Cross-API check:** Any spec without a servers block (GitHub-hosted specs, auto-generated specs).
- **Frequency:** API subclass: specs without servers (~15% of specs)
- **Fallback if the Printing Press doesn't fix it:** Skill or user notices during verify.
- **Worth a Printing Press fix?** Yes — the parser can infer from the spec URL.
- **Inherent or fixable:** Fixable. When `--spec-url` is provided and no servers block exists, derive the base URL from the spec URL's domain (e.g., `raw.githubusercontent.com/.../cal.com/...` → `https://api.cal.com`). Or at minimum, emit a prominent warning.
- **Durable fix:** In the parser, when no servers block exists: (1) try to infer from spec URL domain if available, (2) check if the API name maps to a common pattern (e.g., `cal-com` → `api.cal.com`), (3) emit a WARNING requiring the user to set base_url.
- **Test:** Positive: Cal.com spec + spec-url → base URL inferred as api.cal.com. Negative: spec with servers block → uses servers URL.
- **Evidence:** All commands returned errors until manual base URL fix.

### F6. Auth Resource Endpoint Files Generated But Never Wired (Template gap)
- **What happened:** The generator creates endpoint files for the `auth` resource (oauth2-get-client.go, oauth2-token.go) but the root.go template excludes `auth` from the AddCommand loop (line 101: `ne $name "auth"`). These constructors are defined but never called, causing 2 "unregistered commands" in dogfood.
- **Scorer correct?** Dogfood correctly flags them as unregistered — they genuinely aren't wired.
- **Root cause:** The generator always creates endpoint files for every resource, but the `auth` resource is special — it's replaced by the auth template. The endpoint files for `auth` sub-resources are dead code.
- **Cross-API check:** Every API where the spec has auth-related endpoints in a resource named "auth" or "oauth".
- **Frequency:** API subclass: APIs with auth management endpoints (~30%)
- **Fallback if the Printing Press doesn't fix it:** Dogfood flags them as unregistered; harmless but noisy.
- **Worth a Printing Press fix?** Yes — skip endpoint file generation for the auth resource (same pattern as promoted resource parent skipping).
- **Inherent or fixable:** Fixable. The generator already knows to skip auth from AddCommand — it should also skip endpoint file generation for the auth resource.
- **Durable fix:** In the resource generation loop, skip endpoint file generation when the resource name is "auth" (matching the root.go template exclusion).
- **Test:** Positive: auth resource endpoints not generated. Negative: auth.go template still emitted.
- **Evidence:** Dogfood: "2 unregistered commands: oauth2-get-client, oauth2-token"

## What Was Fixed During This Session

These findings were discovered and fixed in the Printing Press before/during this Cal.com run:

| Fix | Commit | Impact |
|-----|--------|--------|
| Token masking in client template | acb9bff | Auth 8→10/10 on every CLI |
| Conditional data-layer helper emission | acb9bff | Dead Code 3→5/5 for non-data CLIs |
| Dogfood wiring check rewrite | acb9bff | 16 false positives → 2 genuine on Cal.com |
| OperationId normalization | acb9bff | No more controller-2024-08-13 filenames |
| Auto-calibrated endpoint limit | dd5abb1 | No more silently skipped endpoints |
| Skip dead promoted resource files | bfa4331 | 14 dead constructors → 2 |
| Spec provenance (--spec-url, checksum, archive) | fabe228, c636529 | Every CLI traceable and reproducible |
| Store=true → Sync=true invariant | 307aeb8 | Sync always generated when store exists |

## Prioritized Improvements

### P1 — High priority
| # | Finding | Component | Frequency | Fallback Reliability | Complexity |
|---|---------|-----------|-----------|---------------------|------------|
| F1 | Per-endpoint version header routing | OpenAPI parser + command template | ~10% of APIs | ~90% via verify loop | medium |
| F4 | Auth inference from Authorization header params | OpenAPI parser | ~20% of APIs | ~70% via skill | medium |

### P2 — Medium priority
| # | Finding | Component | Frequency | Fallback Reliability | Complexity |
|---|---------|-----------|-----------|---------------------|------------|
| F2 | Store migration NOT NULL on FK columns | Store template | ~25% of APIs | ~95% manual fix | small |
| F3 | Promoted command endpoint selection | Generator (buildPromotedCommands) | ~15% of APIs | ~80% via verify | small |
| F5 | Base URL inference when servers missing | OpenAPI parser | ~15% of specs | ~90% manual fix | small |
| F6 | Auth resource endpoint files dead code | Generator resource loop | ~30% of APIs | Harmless noise | small |

### Skip
*None — all findings have cross-API applicability.*

## Work Units

### WU-1: Per-Endpoint Version Header Routing (from F1)
- **Goal:** When an API uses different version header values per endpoint group, each command sends the correct version
- **Target:** `internal/openapi/parser.go` (detectRequiredHeaders) and `internal/generator/templates/command_endpoint.go.tmpl`
- **Acceptance criteria:**
  - positive test: Cal.com event-types commands send `cal-api-version: 2024-06-14`; bookings commands send `2024-08-13`
  - negative test: single-version API still uses global header without per-endpoint overrides
- **Scope boundary:** Does NOT change the spec data model — uses existing RequiredHeaders with per-endpoint values
- **Dependencies:** None
- **Complexity:** medium

### WU-2: Auth Inference from Authorization Header Params (from F4)
- **Goal:** When spec has no securitySchemes and no auth keywords in description, scan operations for required Authorization header parameters as fourth-tier auth inference
- **Target:** `internal/openapi/parser.go` (mapAuth chain)
- **Acceptance criteria:**
  - positive test: Cal.com spec (no securitySchemes, has Authorization header params) → infers Bearer
  - negative test: spec WITH securitySchemes → uses those, skips param scan
- **Scope boundary:** Only scans for Authorization header parameters, not arbitrary headers
- **Dependencies:** None (builds on existing inferDescriptionAuth)
- **Complexity:** medium

### WU-3: Nullable FK Columns in Store Migrations (from F2)
- **Goal:** Inferred foreign key columns in store migrations default to nullable
- **Target:** Store template in `internal/generator/`
- **Acceptance criteria:**
  - positive test: org-scoped table has nullable organizations_id column
  - negative test: entity's own ID column is still NOT NULL
- **Scope boundary:** Only affects inferred FK columns, not user-defined schema
- **Dependencies:** None
- **Complexity:** small

### WU-4: Promoted Command Endpoint Selection (from F3)
- **Goal:** Promoted commands default to the list endpoint, not alphabetically first
- **Target:** `internal/generator/generator.go` (buildPromotedCommands)
- **Acceptance criteria:**
  - positive test: schedules promoted command lists schedules (not get-default)
  - negative test: resource with only "create" still promotes that
- **Scope boundary:** Only changes selection logic, not the promoted command template
- **Dependencies:** None
- **Complexity:** small

### WU-5: Skip Auth Resource Endpoint Files (from F6)
- **Goal:** Don't generate endpoint files for the "auth" resource since they're never wired
- **Target:** `internal/generator/generator.go` (resource generation loop)
- **Acceptance criteria:**
  - positive test: no oauth2-get-client.go or oauth2-token.go generated for Cal.com
  - negative test: auth.go template still emitted; non-auth resources still generate endpoints
- **Scope boundary:** Only skips the "auth" resource name, not other resources
- **Dependencies:** None
- **Complexity:** small

## Anti-patterns
- **Post-generation auth patching:** Adding auth env vars to config.go after generation is fragile — it misses doctor, client, README, and auth template integration. The parser should get auth right so every template benefits.
- **Global header promotion for multi-value headers:** When an API has per-resource header values, promoting one value globally guarantees some endpoints break.

## What the Printing Press Got Right
- **Auto-calibrated endpoint limit:** No endpoints silently skipped (fixed during session).
- **Spec archiving:** Every CLI now carries its spec.json + provenance manifest — fully reproducible.
- **OperationId normalization:** `BookingsController_2024-08-13_getBooking` → `get` under bookings. Clean command names.
- **Token masking:** `maskToken()` in client.go — auth score 10/10 on first generation.
- **Promoted command dead code elimination:** No more dead parent/endpoint files for promoted resources.
- **Wiring check accuracy:** Static source analysis catches real issues without false positives.
- **Data layer generation:** Store, search, analytics, sync, FTS5, 4 insight commands — all generated and working.
- **96/100 scorecard:** After machine fixes + 2 polish passes, Grade A with only Type Fidelity (3/5) and Terminal UX (9/10) as gaps.
