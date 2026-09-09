# Printing Press Retro: Linear

## Session Stats
- API: Linear (GraphQL)
- Spec source: Official GraphQL SDL from github.com/linear/linear (43,505 lines)
- Scorecard: 90/100 (Grade A)
- Verify pass rate: 97%
- Fix loops: 1 shipcheck loop, 1 live-test fix loop
- Manual code edits: 6 (GraphQL client, sync command, 6 transcendence commands, types dedup, usageErr, FTS5 fix)
- Features built from scratch: 8 (GraphQL client layer, sync, today, stale, bottleneck, similar, workload, velocity)
- Codex delegations: 1 success (store enhancement), 0 failures

## Findings

### 1. GraphQL types.go emits duplicate struct fields (Bug)
- **What happened:** The GraphQL SDL parser produces types where pagination arguments (after, before, first, last) appear as fields on entity types. The types.go template emits all fields without deduplication, causing `go vet` to fail with "After redeclared."
- **Root cause:** `internal/graphql/parser.go` `buildTypeDef()` collects all fields from the parsed GraphQL type including inherited/mixed-in pagination fields. `internal/generator/templates/types.go.tmpl` iterates `$typeDef.Fields` without checking for duplicate field names.
- **Cross-API check:** Affects every GraphQL API. GitHub's schema (30k+ lines) and Shopify's schema have the same Connection pattern with pagination args.
- **Frequency:** Every GraphQL API
- **Fallback if machine doesn't fix it:** Claude has to run a Python dedup script post-generation. Catches it when `go vet` fails, but the fix is fragile (regex-based text manipulation).
- **Worth a machine fix?** Yes. Every GraphQL generation will hit this.
- **Inherent or fixable:** Fixable. Either deduplicate in the parser (`buildTypeDef`) or in the template.
- **Durable fix:** In `buildTypeDef()`, track seen field names and skip duplicates:
  ```go
  seen := map[string]bool{}
  for _, field := range typ.Fields {
      if seen[field.Name] { continue }
      seen[field.Name] = true
      fields = append(fields, ...)
  }
  ```
- **Test:** Parse Linear's schema.graphql -> verify no Go struct has duplicate fields. Parse a minimal SDL with pagination args -> verify dedup.
- **Evidence:** `go vet` failed on generated types.go with "After redeclared" at line 95.

### 2. usageErr conditionally emitted but unconditionally referenced (Bug)
- **What happened:** Generated promoted commands call `usageErr()` but the function is only emitted in helpers.go when `.HasMultiPositional` is true. For GraphQL APIs where promoted commands all have positional ID args, the function is missing.
- **Root cause:** `internal/generator/templates/helpers.go.tmpl` line 97-98 wraps `usageErr` in `{{if .HasMultiPositional}}`. `internal/generator/templates/command_promoted.go.tmpl` calls `usageErr` unconditionally.
- **Cross-API check:** Affects any API where promoted commands have positional args but `.HasMultiPositional` is false. This includes most GraphQL APIs and REST APIs with simple ID-based gets.
- **Frequency:** Most APIs (any with promoted commands that have positional params)
- **Fallback if machine doesn't fix it:** Claude adds `usageErr` manually. The CLI won't compile without it, so it's always caught, but it's wasted effort every time.
- **Worth a machine fix?** Yes. Simple template fix.
- **Inherent or fixable:** Fixable. Remove the conditional or always emit `usageErr`.
- **Durable fix:** Remove the `{{if .HasMultiPositional}}` guard from `usageErr` in helpers.go.tmpl. The function is tiny and harmless even if unused (the dead-code checker won't flag it because promoted commands use it).
- **Test:** Generate any API with promoted commands -> verify `go build` succeeds without manual patching.
- **Evidence:** Build failed with "undefined: usageErr" across all promoted_*.go files.

### 3. GraphQL sync command must be hand-written (Missing scaffolding)
- **What happened:** The generator produces a sync.go template for REST APIs but the generated sync doesn't work for GraphQL. The sync command sends GET requests to `/graphql` instead of POST requests with GraphQL queries. The entire sync command (internal/cli/sync.go) and GraphQL client layer (internal/client/graphql.go, queries.go) had to be written from scratch.
- **Root cause:** The generator's sync.go.tmpl and client.go.tmpl are REST-oriented. GraphQL APIs need: (1) a POST-based query executor, (2) query strings with variables, (3) Connection-pattern pagination handling. None of this exists in the templates.
- **Cross-API check:** Affects every GraphQL API. GitHub, Shopify, and Linear all use the same query/mutation/connection pattern.
- **Frequency:** GraphQL subclass (growing: many modern APIs are GraphQL-first)
- **Fallback if machine doesn't fix it:** Claude writes ~200 lines of GraphQL client code and ~150 lines of sync code from scratch. The quality is good but it's the most labor-intensive part of the generation. Takes ~30% of Phase 3 time.
- **Worth a machine fix?** Yes. GraphQL is a major API category.
- **Inherent or fixable:** Fixable. The GraphQL parser already converts SDL to an APISpec with `method: GET, path: /graphql`. Instead, the generator should detect GraphQL specs and emit GraphQL-specific templates.
- **Durable fix:**
  1. Add `IsGraphQL bool` to the generator's template context (derived from `spec.BaseURL` containing `/graphql` or spec source being `.graphql`)
  2. Create `graphql_client.go.tmpl` that emits a `Query()`, `Mutate()`, and `PaginatedQuery()` method on the Client
  3. Create `graphql_sync.go.tmpl` that generates sync functions using `PaginatedQuery` for each resource's list endpoint
  4. When `IsGraphQL`, use these templates instead of the REST variants
  - **Condition:** Spec source is GraphQL SDL or spec base URL ends in `/graphql`
  - **Guard:** REST APIs continue using existing templates unchanged
  - **Frequency estimate:** ~15-20% of APIs the printing press targets (GitHub, Linear, Shopify, Contentful, Hasura, etc.)
- **Test:** Generate from Linear schema.graphql -> sync command works without manual editing. Generate from Stripe OpenAPI -> sync still uses REST client (negative test).
- **Evidence:** Entire GraphQL client layer (graphql.go, queries.go) and sync.go written from scratch in Phase 3.

### 4. Promoted commands from GraphQL lack examples (Template gap)
- **What happened:** All 40+ promoted commands generated from the GraphQL schema had no `Example:` field in their cobra command definition. Dogfood reported 2/10 example coverage.
- **Root cause:** The `command_promoted.go.tmpl` generates examples using the endpoint's operationId or path, but for GraphQL all paths are `/graphql` with no meaningful path segments to derive examples from.
- **Cross-API check:** GraphQL-specific. REST APIs with meaningful paths produce better examples.
- **Frequency:** GraphQL subclass
- **Fallback if machine doesn't fix it:** Polish worker adds examples post-generation. Reliable but adds 2-3 minutes per run.
- **Worth a machine fix?** Yes, but lower priority than the sync/client issue.
- **Inherent or fixable:** Fixable. For GraphQL promoted commands, derive examples from the entity name and positional args:
  ```
  Example: "  linear-pp-cli issues <issue-id>\n  linear-pp-cli issues <issue-id> --json"
  ```
- **Durable fix:** In `command_promoted.go.tmpl`, when the entity has a positional ID param, generate a default example pattern: `<cli-name> <resource> <example-id> [--json] [--select field1,field2]`.
- **Test:** Generate from GraphQL schema -> promoted commands have non-empty Example fields.
- **Evidence:** Dogfood reported 2/10 example coverage; polish worker had to add examples to 7 promoted commands.

### 5. FTS5 content-linked triggers fail with modernc.org/sqlite (Bug)
- **What happened:** The store created by Codex used `DELETE FROM issues_fts WHERE id = ?` to manage FTS5 entries. This syntax doesn't work with modernc.org/sqlite for FTS5 virtual tables. Had to switch to content-linked FTS5 with triggers.
- **Root cause:** The store.go template (and Codex's enhancement) used a standard SQL DELETE on the FTS5 table, but FTS5 tables in modernc.org/sqlite require either the special `INSERT INTO fts(fts, ...) VALUES('delete', ...)` syntax or content-linked triggers.
- **Cross-API check:** Affects every API that uses FTS5 in the store (which is every printed CLI).
- **Frequency:** Every API
- **Fallback if machine doesn't fix it:** Claude discovers this during live testing and rewrites the FTS management code. The error is clear but the fix is non-trivial (content-linked triggers with proper column references).
- **Worth a machine fix?** Yes. Critical path - FTS5 is core to the CLI's value proposition.
- **Inherent or fixable:** Fixable. The store.go template should use content-linked FTS5 from the start.
- **Durable fix:** Update `store.go.tmpl` to:
  1. Create FTS5 tables with `content='<table>', content_rowid='rowid'`
  2. Create AFTER INSERT/UPDATE/DELETE triggers that maintain the FTS index
  3. Remove manual FTS INSERT/DELETE from Upsert methods
  4. Only create FTS triggers for tables that have searchable text columns
- **Test:** Generate any CLI -> sync data -> `SearchIssues()` returns results. No "no such column" errors in FTS operations.
- **Evidence:** Sync produced hundreds of "warning: issue FTS cleanup failed: SQL logic error: no such column: id" messages until FTS was rewritten with triggers.

### 6. GraphQL query complexity not validated before sync (Recurring friction)
- **What happened:** The initial TeamsQuery fetched nested members, states, labels, and cycles. Linear's API rejected it with "Query too complex" (complexity limit exceeded). Had to simplify the query by removing nested fields.
- **Root cause:** No query complexity budgeting. The GraphQL query constants were hand-written without knowing the API's complexity limits.
- **Cross-API check:** Every GraphQL API has complexity limits. GitHub's is 5,000 points, Linear's appears to be around 1,000.
- **Frequency:** Every GraphQL API
- **Fallback if machine doesn't fix it:** Claude discovers during sync testing and manually simplifies queries. Usually caught on first sync attempt, but requires understanding each API's complexity model.
- **Worth a machine fix?** Worth documenting but hard to automate. Complexity limits vary wildly between APIs.
- **Inherent or fixable:** Partially inherent. Could mitigate by generating conservative queries (flat fields only, no nested connections) as defaults, with opt-in deeper fetches.
- **Durable fix:** When generating GraphQL sync queries, default to flat field selection (no nested connections beyond 1 level). Add a `--depth` flag to sync that controls nesting level. Skill instruction: "For GraphQL APIs, start with shallow queries and deepen only if the API allows it."
- **Test:** Generate from Linear schema -> default sync queries succeed without complexity errors.
- **Evidence:** TeamsQuery returned HTTP 400 "Query too complex" on first sync attempt.

### 7. GraphQL field names don't match API schema (Assumption mismatch)
- **What happened:** The CyclesQuery referenced `completedScopeCount` which doesn't exist on Linear's Cycle type. The field was `completedScopeHistory` or similar. Had to discover the correct field name during live testing.
- **Root cause:** The GraphQL query constants were hand-written based on research assumptions, not derived from the actual schema. The generator's parsed spec had the correct field names but the sync queries were written separately.
- **Cross-API check:** Happens whenever sync queries are hand-written rather than derived from the parsed schema.
- **Frequency:** GraphQL subclass (when queries are manually authored)
- **Fallback if machine doesn't fix it:** Claude discovers during sync and fixes. Always caught on first API call, but wastes a retry cycle.
- **Worth a machine fix?** Yes, as part of the GraphQL template system (finding #3). If queries are generated from the parsed schema, field names will always be correct.
- **Inherent or fixable:** Fixable. Part of the GraphQL sync template solution - generate queries from the parsed schema rather than hand-writing them.
- **Durable fix:** Same as finding #3 - the sync template should derive field selections from the parsed schema's type definitions. The parser already has all field names.
- **Test:** Generate from any GraphQL schema -> all sync queries reference valid field names. Run sync -> no "Cannot query field" errors.
- **Evidence:** CyclesQuery returned "Cannot query field 'completedScopeCount' on type 'Cycle'" during live sync.

## Prioritized Improvements

### Do Now
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 1 | Deduplicate struct fields in types.go for GraphQL | graphql/parser.go | Every GraphQL API | Always caught (go vet) but manual fix | small | None needed |
| 2 | Always emit usageErr in helpers.go | generator/templates/helpers.go.tmpl | Most APIs | Always caught (build fails) but manual fix | small | None needed |
| 5 | Fix FTS5 to use content-linked triggers | generator/templates/store.go.tmpl | Every API | Caught during live test, non-trivial fix | medium | None needed |

### Do Next (needs design/planning)
| # | Fix | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---|-----|-----------|-----------|---------------------|------------|--------|
| 3 | GraphQL-specific sync/client templates | generator/templates/ | GraphQL subclass (~15-20%) | Claude writes ~350 lines from scratch | large | Only activate for GraphQL specs |
| 4 | Generate examples for GraphQL promoted commands | generator/templates/command_promoted.go.tmpl | GraphQL subclass | Polish worker fixes, reliable | small | GraphQL path detection |
| 6 | Conservative default query depth for GraphQL sync | generator/templates/ | Every GraphQL API | Claude simplifies manually, reliable | small | GraphQL only |

### Skip
| # | Fix | Why unlikely to recur |
|---|-----|----------------------|
| 7 | Field name validation in sync queries | Subsumed by finding #3 - generated queries from schema will have correct field names automatically |

## Work Units

### WU-1: GraphQL type deduplication and usageErr fix (findings #1, #2)
- **Goal:** Fix two compilation failures that affect every GraphQL generation
- **Target files:**
  - `internal/graphql/parser.go` (buildTypeDef - add field deduplication)
  - `internal/generator/templates/helpers.go.tmpl` (remove conditional around usageErr)
- **Acceptance criteria:**
  - Parse Linear's 43k-line schema.graphql -> no duplicate fields in any generated struct
  - Generate from any spec with promoted commands -> `go build` succeeds without manual usageErr addition
  - Generate from a REST API spec -> no regression (usageErr present and unused is harmless)
- **Scope boundary:** Does not include the broader GraphQL template system (WU-3)
- **Complexity:** small (2 files, straightforward fixes)

### WU-2: FTS5 content-linked triggers in store template (finding #5)
- **Goal:** Generated store uses content-linked FTS5 tables with triggers instead of manual FTS management
- **Target files:**
  - `internal/generator/templates/store.go.tmpl` (FTS table creation and trigger generation)
  - `internal/generator/templates/sync.go.tmpl` (remove manual FTS INSERT/DELETE if present)
- **Acceptance criteria:**
  - Generate any CLI -> sync data -> SearchIssues returns results without FTS errors
  - Generate any CLI -> no "no such column" warnings during sync
  - FTS triggers only created for tables with searchable text columns (not all tables)
- **Scope boundary:** Does not redesign the store schema - just fixes FTS management
- **Complexity:** medium (1-2 template files, need to identify which tables get FTS)

### WU-3: GraphQL generation pipeline (findings #3, #4, #6, #7)
- **Goal:** The generator emits working GraphQL client, sync, and promoted command examples for GraphQL APIs
- **Target files:**
  - `internal/generator/generator.go` (detect GraphQL and set IsGraphQL flag)
  - `internal/generator/templates/` (new: graphql_client.go.tmpl, graphql_sync.go.tmpl)
  - `internal/generator/templates/command_promoted.go.tmpl` (example generation for GraphQL)
  - `internal/graphql/parser.go` (expose field selections for query generation)
- **Acceptance criteria:**
  - Generate from Linear schema.graphql -> working sync without manual code
  - Generate from Linear schema.graphql -> promoted commands have examples
  - Generate from Linear schema.graphql -> sync queries don't exceed complexity limits
  - Generate from Stripe OpenAPI -> no regression, REST templates still used (negative test)
- **Scope boundary:** Does not include GraphQL mutation support in commands (CREATE/UPDATE still need manual wiring). Focused on read path: sync, list, get.
- **Dependencies:** WU-1 must be complete first (dedup fix needed for types.go)
- **Complexity:** large (new template files, parser enhancements, generator logic changes)

## Anti-patterns

- **Hand-writing GraphQL queries separately from the parsed schema.** The parser has all field names and types, but queries were written as string constants referencing field names from memory/research. This caused field name mismatches (completedScopeCount). Queries should be derived from the parsed schema.
- **Creating FTS5 tables for every entity regardless of whether it has text fields.** Codex created FTS5 tables with triggers for teams, cycles, and workflow_states - none of which have meaningful searchable text. FTS should only be created for entities with string fields like title, description, content.

## What the Machine Got Right

- **GraphQL SDL parser.** The parser correctly detected Linear's 98+ entity types, identified Connection patterns for pagination, classified mutations by action type (create/update/delete), and derived the correct API name and auth defaults. This is solid work.
- **knownGraphQLDefaults.** Having Linear-specific defaults (base URL, auth header, env var name) baked into the parser eliminated auth configuration friction entirely.
- **Promoted command generation.** The 40+ promoted commands from GraphQL types were structurally correct - correct Use/Short descriptions, positional ID args, output formatting. The only gap was examples.
- **Store template.** The generic resource store with JSON data columns plus entity-specific typed columns is a good pattern. It survived Codex enhancement and live testing without fundamental issues.
- **Quality gates.** The 7-gate validation caught both compile errors (types dedup, usageErr) immediately. Without gates, these would have surfaced much later.
- **Scorecard dimensions.** 90/100 on first generation is strong. Output Modes, Auth, Agent Native, and Local Cache all scored 10/10 without manual work.
