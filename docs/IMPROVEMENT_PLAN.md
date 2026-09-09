# qb-cli Improvement Plan — JS/Capture Evidence vs CLI Surface Audit

Date: 2026-08-23. Inputs: 1,439 captured bundles (60MB), 20,484 deobfuscated modules,
5,000+ mitmproxy flows, live probes on test company 2 (realm 12345),
CLI action catalog (340 actions: 117 read / 98 wired / 123 blocked / 2 excluded).

## Audit findings

### A. What the JS proves that the CLI lacks
1. **GraphQL layer absent from the CLI.** The capture/deobfuscation corpus contains:
   - 286 GraphQL operations (293 full documents in /tmp/qbo-cap/graphql/)
   - 84 mutations with typed inputs (the write surface: Accounting_CreateAccountV2,
     Banking_DisconnectOlbAccounts, UpdateIntegration_ReconciliationInput reconcile
     family, Commerce_* inventory ops, BatchUpdateProducts…)
   - 26 cursor-paginated queries with REAL server-side date filters
     (Commerce_BillFilter.transactionDate{createdOnOrAfter/Before} etc.) —
     including GetBills, GetPayouts, GetSchedules, Items, TaskManagementTasks.
   - Endpoints: qbo.intuit.com/api/v4/graphql (main), commercecontrol.api.intuit.com,
     warehouse-management-svc.api.intuit.com, smallbusiness.api.intuit.com/graphql
     (169 successful POSTs observed; auth = browser cookies + csrftoken header).
   - Constraint proven today: cross-host replay of smallbusiness GraphQL with saved
     ATS headers returns 401 — these calls are bound to the browser session context.
     Any `qb gql` implementation must route through a live relay tab (same ladder as
     `auth remint`), not an offline HTTP client.
2. **94 prod services never surfaced** (accountreconcile, billpaysvc, budgeting,
     coa-core, custom-objects-svc, company-management-svc…). Full list:
     /tmp/qbo-cap/service_diff.json. Several map directly onto existing "blocked"
     CLI stubs.
3. **943 feature flags + evaluation protocol**
     (POST ixp-ff.api.intuit.com/api/v6/flags/boolean/{name}/env/{env}/users/{userId};
     response body IS the boolean). Only 7 evaluated in our session — flag-gated
     commands could explain some blocked surfaces.
4. **REST verb+path pairs** recovered from bundles beyond the current catalog
     (/v4/search, /v2/documents, /v2/print, /v1/audit/logs/batch, /v2/consent…).

### B. Structural issues in the CLI itself
1. **Verb chaos:** 54 distinct verbs across 340 actions. read(80)/create(76)/edit(51)/
   delete(36)/search(29) dominate; 25 verbs appear once each (lodge, bounce, revalue,
   recategorise, reprint…). No grammar discipline.
2. **Inconsistent entity nesting:** feed uses `feed txn population`, accounting splits
   `audit-log` vs `integration-txn`, expenses has both `bill-list` AND `bill-form`
   as entities instead of `bill list|form`.
3. **Mode opacity:** 123 blocked actions sit in the same namespace as wired ones;
   users cannot see what works without running `actions`.
4. **population command naming drift:** `feed txn population` now performs the
   completeness-checked walk but its help text historically said "limit 300";
   verify/population-all exist but aren't reflected in the root help text.
5. **No GraphQL gateway abstraction:** client package is ATS-REST only.

## Improvement plan (breaking changes allowed; no back-compat)

### Phase 1 — Command grammar overhaul (breaking)
Unify every domain on `qb <domain> <entity> <verb>` with a CLOSED verb set:
list | get | search | create | update | delete | run | verify | export | import.
Map legacy one-off verbs: lodge→run, bounce→update, revalue→update, recategorise→update,
reprint→export, review→verify. Kill entity-as-verb-shape (`bill-list` → `bill list`,
`invoice-form` → `invoice form`). Regenerate params_*.go and the actions catalog.

### Phase 2 — Mode transparency
- Every leaf command gets hidden metadata (mode, spec source) surfaced via
  `qb actions --mode blocked` filtering and in --help output ("[not implemented]").
- `qb doctor` extended: report count of blocked-vs-wired per domain so users know
  coverage before scripting.

### Phase 3 — GraphQL gateway (new capability)
New package internal/quickbooks/gql:
- Transport: relay-tab execution (reuse auth.RemintATS ladder pattern — requests
  MUST originate from the authenticated browser context; proven 401 otherwise).
- Embedded catalog of the 286 operations + 84 mutation input types generated into Go
  (from /tmp/qbo-cap/graphql/ and gql_pass3.json).
- Commands: `qb gql query <OpName> [--vars k=v]`, `qb gql mutate <OpName> [--vars]`,
  `qb gql ops [filter]`. Cursor pagination helper for the 26 date-filterable queries:
  `qb gql walk <OpName> --from --to --page-size`.
- Start with verified-working ops: GetBills/spend_lists_get_billpay_payments_ets
  (smallbusiness host), reconciliation read family (banking-reconcile-ui).

### Phase 4 — Wire blocked stubs with known endpoints
Highest-value mappings from service_diff.json + bundle evidence:
- `accounting reconcile create` → Integration_Reconciliation mutations (12 fragments
  already extracted)
- feed disconnect → BankingDisconnectOlbAccounts(Banking_DisconnectOlbInput!)
- advanced tasks create/edit/delete → TaskManagementTasks query + workflow mutations
- expenses bill-list date windows → GetBills transactionDate filter
Each wiring follows the existing capture-first discipline: probe against test
company 2, record request/response shapes, then implement.

### Phase 5 — Docs/help regeneration
Root help lists domains with wired counts. README gains a "coverage matrix" table
(domain × verb × mode). CHANGELOG entry for the breaking grammar change with a
migration table old→new.

## Execution order
Phases 1+2 can run in parallel (grammar vs metadata). Phase 3 depends on nothing
but is largest; start its catalog-generation early. Phase 4 requires phase 3's
gateway for GraphQL-backed wirings. Phase 5 last.
