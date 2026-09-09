# Printing Press Retro: trigger-dev

## Session Stats
- API: Trigger.dev
- Spec source: internal YAML (hand-crafted from official docs, no OpenAPI spec available)
- Scorecard: 88/100 (Grade A)
- Verify pass rate: 95% (21/22), polished to 100%
- Fix loops: 1
- Manual code edits: 2 (usageErr missing, unused variable)
- Features built from scratch: 8 (all transcendence features)

## Findings

### F1. usageErr function missing from generated helpers.go (Bug)
- **What happened:** Generated CLI failed to build because `usageErr` was called in envvars and batches commands but not defined in helpers.go.
- **Scorer correct?** N/A - this was a build failure, not a scoring issue.
- **Root cause:** The helpers.go.tmpl template DOES include `usageErr` (line 98). The binary used was v1.2.1-dirty, which may have been built from a state before the function was added to the template. Alternatively, the template rendering conditionally excluded it.
- **Cross-API check:** Would recur on any API with path parameters on promoted commands.
- **Frequency:** Every API with promoted endpoints that have required positional args.
- **Fallback if the Printing Press doesn't fix it:** Claude catches the build error and adds the function manually. Reliable but wastes time.
- **Worth a Printing Press fix?** Yes - this is a build-breaking template bug.
- **Inherent or fixable:** Fixable. The function is already in the template; ensure it renders unconditionally.
- **Durable fix:** Verify that `usageErr` is always included in helpers.go output regardless of which endpoints are generated. Add a test case with a spec that has positional args.
- **Test:** Generate a CLI from any spec with required positional args. Verify `usageErr` is in helpers.go and the CLI builds.
- **Evidence:** Build error on first `go build` attempt, had to manually add the function.

### F2. Unused variable `data` in generated config.go (Bug)
- **What happened:** `data, err := os.ReadFile(path)` where `data` was declared but never used, causing `go vet` failure.
- **Scorer correct?** N/A - build/vet failure.
- **Root cause:** Generator template for config.go reads the config file but doesn't use the contents (presumably intended for JSON parsing that wasn't wired up).
- **Cross-API check:** Would recur on every API.
- **Frequency:** Every API.
- **Fallback:** Claude fixes the vet error. Reliable but adds a fix loop.
- **Worth a Printing Press fix?** Yes.
- **Inherent or fixable:** Fixable. Either parse the JSON config data or use `_, err :=`.
- **Durable fix:** Fix the config.go template to either (a) parse the config file JSON into the Config struct, or (b) use `_, _ = os.ReadFile(path)` if the file read is just a presence check.
- **Test:** Generate any CLI, run `go vet ./...`, verify no unused variable errors in config.go.
- **Evidence:** `go vet` failure after generation, line 45 of config.go.

### F3. Dogfood/verify/scorecard can't parse internal YAML specs (Scorer bug)
- **What happened:** All three tools (dogfood, verify, scorecard) failed with "at least one resource is required" when given the internal YAML spec that the generator successfully consumed.
- **Scorer correct?** No - the scorer is wrong. The spec is valid (the generator parsed it fine), but the validation tools use a stricter parser that rejects the same spec.
- **Root cause:** The dogfood/verify/scorecard validation path runs a spec parse that expects resources in a format the internal YAML parser doesn't require. The generator's spec parser is more lenient than the validator's spec parser.
- **Cross-API check:** Would recur on any CLI generated from an internal YAML spec (not OpenAPI).
- **Frequency:** Every API that uses internal YAML format instead of OpenAPI.
- **Fallback:** Run verify/scorecard without --spec flag (works, but loses spec-dependent checks like path validity and auth protocol).
- **Worth a Printing Press fix?** Yes - this blocks three verification tools from providing full coverage.
- **Inherent or fixable:** Fixable. The spec parser used by the verification tools should accept the same format the generator accepts.
- **Durable fix:** Ensure the spec validation in dogfood/verify/scorecard uses the same parser (internal/spec/) as the generator, or add a fallback path that skips spec validation gracefully when the spec format is internal YAML.
- **Test:** Run `printing-press dogfood --dir <cli> --spec <internal-yaml-spec>`. Should not error on "at least one resource is required."
- **Evidence:** All three tools errored with the same message during shipcheck.

### F4. defaultSyncResources only syncs 4 of 9 resources (Template gap)
- **What happened:** The spec defines 9 resources (runs, tasks, schedules, envvars, queues, deployments, batches, waitpoints, query) but defaultSyncResources only syncs 4 (queues, runs, schedules, waitpoints). Missing: tasks, batches, envvars, deployments, query.
- **Scorer correct?** Yes - Data Pipeline Integrity scored 7/10, partly because of incomplete sync coverage.
- **Root cause:** The generator's sync template selects resources for sync but uses heuristics that exclude resources without list endpoints or with complex path parameters (envvars requires projectRef). The "query" resource is an execute-only endpoint, not a list-and-sync resource.
- **Cross-API check:** Would recur on APIs where some resources have complex list paths.
- **Frequency:** Most APIs - many APIs have resources with compound paths.
- **Fallback:** Claude adds sync resources manually. Unreliable - easy to forget.
- **Worth a Printing Press fix?** Partially. Some resources (envvars with projectRef, query) genuinely can't be synced with the standard pattern. But deployments and batches could be synced.
- **Inherent or fixable:** Partially fixable. The generator should sync all resources that have a standard GET list endpoint. Resources with compound paths (envvars/{projectRef}/{env}) need special handling.
- **Durable fix:** In the sync template, include all resources whose list endpoint matches the pattern `GET /api/v*/<resource>` (no path params). For resources with path params, generate a sync path that requires the user to provide the param via config or flag.
- **Test:** Generate a CLI from a spec with 5+ resources, verify defaultSyncResources includes all resources with simple list endpoints.
- **Evidence:** `defaultSyncResources()` returns only 4 entries despite 9 resources in the spec.

### F5. Per-entity tables are JSON blobs, not typed columns (Template gap)
- **What happened:** All entity tables store data as `id TEXT, data JSON, synced_at DATETIME`. No typed columns for status, taskIdentifier, costInCents, etc. This forces JSON extraction for queries and scores low on Type Fidelity (3/5).
- **Scorer correct?** Yes - Type Fidelity 3/5 is appropriate for JSON-blob-only tables.
- **Root cause:** The generator creates per-entity tables but doesn't extract response fields into typed columns. The spec provides response_fields but the store template ignores them.
- **Cross-API check:** Every API.
- **Frequency:** Every API.
- **Fallback:** Claude adds typed columns manually. Unreliable and time-consuming.
- **Worth a Printing Press fix?** Yes - high impact. Typed columns improve query performance, enable better FTS indexing, and improve scorecard.
- **Inherent or fixable:** Fixable. The spec's response fields can drive column generation.
- **Durable fix:** When the spec defines response fields for a resource's list endpoint, generate typed columns in the entity table for the top 5-8 fields (id, status, name, created_at, etc.). Keep the `data JSON` column as a catch-all for the full response.
- **Test:** Generate a CLI from a spec with response fields. Verify entity tables have typed columns beyond just id/data/synced_at.
- **Evidence:** All tables in store.go use the same 3-column schema regardless of entity type. Scorecard Type Fidelity 3/5.

### F6. Search uses generic Search only, no per-entity FTS (Recurring friction)
- **What happened:** Dogfood pipeline check noted "search uses generic Search only." The generic `resources_fts` table is used for all search, but per-entity tables have no FTS indexes.
- **Scorer correct?** Yes - this is correctly identified as a pipeline integrity gap.
- **Root cause:** The generator creates per-entity tables but only creates one FTS index on the generic `resources` table. Per-entity FTS would require knowing which text fields to index.
- **Cross-API check:** Every API.
- **Frequency:** Every API.
- **Fallback:** Works but search quality is limited.
- **Worth a Printing Press fix?** Medium priority. The generic search works but per-entity FTS would produce better results.
- **Inherent or fixable:** Fixable if spec response fields are available. The generator can identify text/string fields and create FTS indexes on per-entity tables.
- **Durable fix:** When generating per-entity tables with typed columns (F5), also create FTS5 virtual tables for entities with text fields (e.g., `runs_fts` indexing taskIdentifier, status, tags).
- **Test:** Generate a CLI, run `search "test"`, verify results come from per-entity FTS tables.
- **Evidence:** Dogfood pipeline_check shows "search uses generic Search only."

## Prioritized Improvements

### P1 - High priority
| Finding | Title | Component | Frequency | Fallback Reliability | Complexity |
|---------|-------|-----------|-----------|---------------------|------------|
| F3 | Dogfood/verify/scorecard can't parse internal YAML specs | Scorer (dogfood, verify, scorecard) | Every internal YAML API | Low - blocks three tools entirely | Medium |
| F2 | Unused variable in config.go | Generator templates | Every API | High - Claude fixes it, but wastes a fix loop | Small |

### P2 - Medium priority
| Finding | Title | Component | Frequency | Fallback Reliability | Complexity |
|---------|-------|-----------|-----------|---------------------|------------|
| F5 | Per-entity tables are JSON blobs | Generator templates (store.go.tmpl) | Every API | Low - manual column addition is error-prone | Medium |
| F4 | defaultSyncResources misses resources | Generator templates (sync.go.tmpl) | Most APIs | Medium - Claude sometimes catches it | Small |

### P3 - Low priority
| Finding | Title | Component | Frequency | Fallback Reliability | Complexity |
|---------|-------|-----------|-----------|---------------------|------------|
| F6 | Search uses generic Search only | Generator templates (store.go.tmpl, search.go.tmpl) | Every API | Medium - generic search works | Medium |

### Skip
| Finding | Title | Why unlikely to recur |
|---------|-------|----------------------|
| F1 | usageErr missing from helpers.go | Template already has it (line 98). Likely a stale binary issue (v1.2.1-dirty). If it recurs, promote to P1. |

## Work Units

### WU-1: Fix internal YAML spec validation in scorer tools (from F3)
- **Goal:** Make dogfood, verify, and scorecard accept the same internal YAML spec format the generator accepts
- **Target:** Scorer tools - spec validation path in dogfood, verify, and scorecard commands
- **Acceptance criteria:**
  - positive: `printing-press dogfood --spec trigger-dev-spec.yaml` runs without "at least one resource" error
  - positive: `printing-press verify --spec trigger-dev-spec.yaml` produces command-level results
  - negative: Invalid YAML specs still fail validation
- **Scope boundary:** Does not change the internal YAML spec format itself
- **Dependencies:** None
- **Complexity:** Medium

### WU-2: Fix unused variable in config.go template (from F2)
- **Goal:** Generated config.go passes `go vet` without manual fixes
- **Target:** Generator template `internal/generator/templates/config.go.tmpl`
- **Acceptance criteria:**
  - positive: Generate any CLI, `go vet ./...` passes on first run
  - negative: Config file loading still works when a config file exists
- **Scope boundary:** Does not change config loading behavior
- **Dependencies:** None
- **Complexity:** Small

### WU-3: Add typed columns to per-entity tables (from F5, F6)
- **Goal:** Per-entity tables have typed columns from spec response fields, improving query performance and Type Fidelity score
- **Target:** Generator templates for store.go and related sync/search code
- **Acceptance criteria:**
  - positive: Entity tables have top 5-8 typed columns from spec response fields
  - positive: Type Fidelity scores 4/5 or higher
  - negative: CLIs generated from specs without response fields still work (JSON-blob fallback)
- **Scope boundary:** Does not add per-entity FTS (that's a follow-up)
- **Dependencies:** None
- **Complexity:** Medium

### WU-4: Improve defaultSyncResources coverage (from F4)
- **Goal:** All resources with simple list endpoints are included in default sync
- **Target:** Generator template for sync.go
- **Acceptance criteria:**
  - positive: Resources with `GET /api/v*/<resource>` list endpoints are synced by default
  - negative: Resources with complex paths (compound params) are excluded with a comment explaining why
- **Scope boundary:** Does not add parameter-aware sync for compound-path resources
- **Dependencies:** None
- **Complexity:** Small

## Anti-patterns
- Running with a dirty/stale binary version. The v1.2.1-dirty build may have caused the usageErr issue.
- Writing internal YAML specs manually instead of using the generator's spec discovery. For well-documented APIs like Trigger.dev, a `--docs` generation from the official docs site would have been faster and more complete.

## What the Printing Press Got Right
- Generated 40+ commands from a hand-written internal YAML spec with correct path routing, auth headers, and pagination
- SQLite store with per-entity tables, FTS5 search, and sync cursor management all worked out of the box
- Agent-native flags (--json, --dry-run, --select, --compact, --agent) all wired correctly across all commands
- Scorecard 88/100 Grade A on first generation with only 2 minor fixes needed
- Doctor command correctly validated auth, API reachability, and credentials against the live API
- Sync successfully populated runs and schedules from the live Trigger.dev API
- All 8 transcendence features (watch, failures, health, costs, stale, bottleneck, timeline, env diff) integrated cleanly with the generated foundation
