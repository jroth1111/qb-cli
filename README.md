# qb — QuickBooks Online, beyond the public API

[![CI](https://github.com/jroth1111/qb-cli/actions/workflows/lint.yml/badge.svg)](https://github.com/jroth1111/qb-cli/actions/workflows/lint.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/jroth1111/qb-cli)](go.mod)
[![License](https://img.shields.io/github/license/jroth1111/qb-cli)](LICENSE)

**Operate QuickBooks Online from a terminal or an AI agent—not just its OAuth CRUD endpoints.**

`qb` combines accounting API operations, native banking feeds, webapp GraphQL, detailed reporting, historical audit evidence, and verified mutation workflows. It uses your signed-in company, maintains that session headlessly, and can recover with encrypted login credentials when explicitly enrolled.

- **561 catalogued operations across 16 domains**, with explicit capability and verification status.
- **197 read-only catalog entries** and **107 wired operations with independent-readback adapters**.
- **293 captured GraphQL operations:** 207 queries and 86 mutations, with paginated query walkers and operation-specific write gates.
- Additional command trees for **CRM, document services, sales transactions, and accounting policy**.
- **Native session extension, five-minute health probes, cookie rotation, and optional autonomous password/TOTP recovery.**
- **JSON-first, scriptable, company-bound execution.** Dry-run plans, bounded retries, private evidence, and nonzero exits for unverified outcomes.

Catalog entries, GraphQL operations, and CLI commands are different inventories—not additive measures of live coverage. A discovered endpoint or request builder is not a promise that a live write is enabled.

## Quickstart

Requirements: Go **1.26.9 or newer**, QuickBooks Online access, and Chrome/Chromium for the managed-session path. Browser-session workflows depend on the permissions, subscription and regional features available to the signed-in user. The banking and tax surfaces are AU-oriented.

For an existing, captured Ego session, `QB_HTTP_TRANSPORT=ego qb ...` sends first-party, company-scoped REST requests through that exact browser space and page. This is an explicit alternative to direct HTTP, not a fallback or retry after a failed write. The page's company and principal are checked before and after each request; user control stops execution. Uncertain writes still require read-only recovery, and posting retains independent feed/ledger verification. Other hosts and unpinned sessions fail closed.

Browser-backed reads save complete response bytes privately before parsing. Native response parsing supports pages larger than 1 MiB and reports an explicit error above its 32 MiB safety budget instead of silently truncating them. GET/HEAD requests and known GraphQL queries retry transient transport failures and HTTP 408/429/502/503/504 with a bounded, cancellation-aware budget. Incomplete JSON pages are rejected before pagination can accumulate their rows; financial writes and unknown operations are never transport-retried. Match safety censuses advance by the number of rows actually returned and require an empty terminal page to prove absence.

`QB_CENSUS_PAGE_SIZE` tunes match/register safety-census request capacity (1–999; default 300). It does not change the evidence requirements or treat short pages as exhaustion. Larger pages reduce browser round trips; any server-imposed cap is handled using observed row counts.

```bash
git clone https://github.com/jroth1111/qb-cli.git
cd qb-cli
go build -o ./qb ./cmd/qb

./qb login --source managed
./qb auth status --json
./qb auth keepalive status --json
./qb company company-info get --json
./qb actions --json
```

Use `./qb` from the checkout, or install the built binary on your `PATH` as `qb` for the examples below. No Intuit developer app, OAuth client secret, or manually provisioned refresh token is required for this browser-session workflow.

An explicit `--source managed` owns a persistent Chromium profile. Other login sources include an existing relay tab, an existing Ego session, and Chrome/CDP; explicit sources never silently fall back to another browser. Automatic login can choose an available source, but subsequent recovery stays pinned to the captured session source and principal.

## Why choose this over the QuickBooks MCP server?

For bookkeeping that extends into the actual QuickBooks webapp—bank-feed review, historical investigation, complex reports and evidence-backed changes—`qb` has a materially broader operational surface than [Intuit's open-source QuickBooks MCP server](https://github.com/intuit/quickbooks-online-mcp-server).

| Requirement | `qb` | Intuit's open-source MCP server |
|---|---|---|
| Standard accounting entities | Queries and reviewed mutation workflows for accounts, customers, vendors, transactions, items and more | OAuth-based entity CRUD/search tools |
| Banking workflow | Pending, accepted and excluded feed views; full-state lookup; match links; categorisation, splits, rules and reviewed state transitions | No banking-feed tool family in its registered surface |
| Webapp-only services | Captured GraphQL and native service access beyond v3 | Public accounting API integration |
| Reporting depth | Summary/detail reports, saved and memorized definitions, management folios, constituent audits, class/account filters and attribution analysis | Standard financial report tools |
| Historical investigation | Audit-event paging, transaction revision snapshots, event/entity ID separation and feed-to-ledger links | No equivalent audit-history tool family in its registry |
| Proof of a write | Public writes require a reviewed adapter and independent state readback; missing verification blocks submission | The journal-create handler returns the create API result |
| Planning | `--dry-run` emits a plan without submitting the mutation | Structured MCP tools with input schemas |
| Authentication | Your browser session; native ticket extension; optional encrypted password/TOTP recovery | Developer-app OAuth setup and automatic access/refresh-token renewal |
| Automation interface | Standalone CLI, JSON, exit codes, pipes, scripts, scheduled jobs and terminal-capable agents | MCP stdio tools for compatible clients |

The comparison is about these implementations, not a limitation of the MCP protocol. Both cover ordinary accounting records and core reports. The differentiator is **native workflow access plus operational proof**, not an inflated tool count.

The MCP server's [tool registration](https://github.com/intuit/quickbooks-online-mcp-server/blob/main/src/index.ts), [OAuth client](https://github.com/intuit/quickbooks-online-mcp-server/blob/main/src/clients/quickbooks-client.ts), and [journal-create handler](https://github.com/intuit/quickbooks-online-mcp-server/blob/main/src/handlers/create-quickbooks-journal-entry.handler.ts) define the comparison above.

**Choose the MCP server when** you want a conventional OAuth integration, MCP-native tool discovery, and public-API operations without running a browser after setup. Its OAuth renewal is automatic too. **Choose `qb` when** you need the banking/webapp surfaces, shell automation, granular evidence and strict write verification. Browser-backed breadth comes with greater sensitivity to QuickBooks webapp changes.

## Capabilities

### Books, transactions and financial records

- Chart of accounts, classes, departments/locations, registers and accounting transaction queries.
- Journals, deposits, transfers, bills, bill payments, purchases, expenses, credits, payments, invoices, estimates, sales receipts and purchase orders.
- Customers, suppliers, employees, items, time activities and related reference lists.
- Recurring-transaction lookups, budgets, fixed assets, projects, mileage, deferred/prepaid/revenue-recognition surfaces and CDC change data, subject to each command's catalog mode.
- Reviewed journal batch creation: positive finite amounts, canonical account references, valid dates, separate line descriptions/private notes, receipt correlation and independent readback for every intended entry.
- Journal updates preserve caller-supplied line edits and relevant unmodified fields rather than treating a returned ID as proof.

Use the catalog to determine which reads, builders and verified writes are available for a particular resource. These categories do not imply every resource supports every CRUD verb.

### Company, documents and supporting services

- Company information, preferences, settings, supported user/role reads and reference lists such as terms, payment methods, currencies, exchange rates, customer types, tax agencies and tax rates.
- Attachable metadata and typed entity links, note attachments, file uploads and independent download-byte verification for reviewed upload paths.
- Transaction PDF/download and document-rendering surfaces, email-service inspection and explicitly gated send/reminder builders.
- CDC feeds, webhook signature verification, service discovery and verified batch execution.
- Employee and time-activity operations, alongside regional tax/BAS/IAS/TPAR reads. Payroll finalisation and tax submission are separate capabilities, not implied by these reads.

Catalog mode, readback support and company entitlements remain the authority for each individual operation.

### Native banking feeds

- Connected accounts, transaction populations, pending/accepted/excluded views and register evidence.
- Search across all review states and pages; output limits cap returned matches, not the search window.
- Original and display descriptions, native transaction IDs, linked accounting entity types/IDs, match signals and split/link evidence.
- Reviewed categorisation, split, match, exclude, undo, unpost, bank-rule and attachment workflows where a verifier is available.
- CSV import planning, account checks and session diagnostics.

Feed IDs, accounting entity IDs and audit-event IDs are separate namespaces. Posting/matching uses native banking IDs, not display identifiers. Account IDs are company-scoped: select the intended account explicitly instead of relying on defaults.

For distinct one-to-one links, `qb feed txn update match --account-id <account-id> --mapping-file mappings.json` accepts up to 25 mappings in one submission. The JSON array uses string `olb_txn_id` and `match_id` fields; optional `expected_amount`, `expected_date` (`YYYY-MM-DD`, checked against both feed and ledger), `expected_category_id` and `expected_class_id` (`none` for blank) make stale plans fail before submission. Duplicate feed IDs, reused ledger targets, existing accepted-feed allocations, ambiguous ledger lines and failed eligibility/version checks are refused. `--dry-run` validates and prints the plan without network access. Shared before/after censuses still verify every individual link, original feed fields, ledger attribution and permitted clearing-state transition.

Mapped matches retain private before-state, request, response and verification evidence. If a response is lost or verification fails, do not repeat the write: `qb feed txn update match --verify-evidence <evidence-directory>` performs read-only recovery against the captured company. The mapping-file, evidence-recovery and ordinary `--ids`/`--match-id` modes are mutually exclusive.

See [feed lookup completeness](docs/FEED_LOOKUP.md) and [mutation verification](docs/MUTATION_READBACK.md).

### Reports and investigation

- Profit and loss, balance sheet, cash flow, trial balance, general ledger and journal reports.
- Receivables/payables aging, customer/vendor balances, customer income/sales, item/class/department sales and expense reports.
- Transaction lists, split/detail variants, inventory valuation, tax summaries and account-list reports.
- Class/account/date filters and cash/accrual selection where supported.
- Saved custom-report registries, memorized report definitions and execution of saved reports.
- Management-report folio reads and audits across statement months; comparison of constituent detail, combined totals, distribution sections and filter coverage.
- Management-period request pinning across report segments and cover fields, while preserving explicitly all-date ledger sections; live folio writes remain subject to their verification gates.
- Attribution analysis for policy-specific revenue treatment, plus custom, forecast and performance-report surfaces where exposed.
- Historical audit paging with date bounds, server totals, audit-event IDs, affected entity IDs and optional transaction snapshots.

An exhaustive result requires valid pagination and identity evidence. A row count, matching total or HTTP 200 alone is not completeness proof. Unknown response shapes, cursor repetition and unsupported pagination fail explicitly.

See [audit history](docs/AUDIT_LOG.md).

### GraphQL and additional services

`qb gql ops` browses the captured-operation inventory offline. `query` executes with the appropriate authenticated browser backend; `walk` traverses supported pagination and merges records. Convenience walkers cover bills, items/variants and tasks. Raw mutations require operation-specific verification; a captured document is not permission to execute it live.

Additional trees cover:

| Tree | Surface |
|---|---|
| `crm` | Leads, opportunities and lead/customer associations |
| `documents` | Document listing, rendering/PDF, stored document objects and email-service inspection; delivery/mutation claims remain gated |
| `salestx` | Sales-transaction aliases, PDF/send builders and indirect-tax service operations |
| `policy` | Offline reference documents, rules, journal templates, account/Class lookups, categorisation suggestions, configured fee calculations, owner-net calculations and journal-policy validation |
| `advanced` | Verified batch execution, task/workflow reads, custom-role reads and additional gated builders |
| `costgroups`, `integrations`, `customobjects`, `accountant` | Cost-group reads/builders, integration/service discovery and explicitly catalogued unsupported surfaces |

Company-specific policy is guidance, not authorization to post. Calculators and attribution rules are profile-specific, not universal accounting advice. Keep private statements, personal information, credentials and local policy overrides out of public source control.

## Exact catalog coverage

`qb actions --json` is the machine-readable contract: IDs, modes, commands, usage, parameters, globals, notes and mutation verification status.

| Domain | Read | Wired builders | Blocked | Excluded |
|---|---:|---:|---:|---:|
| Accounting | 30 | 37 | 8 | 0 |
| Expenses | 22 | 37 | 16 | 0 |
| Sales | 21 | 38 | 8 | 0 |
| Company | 28 | 24 | 31 | 1 |
| Banking feeds | 11 | 14 | 12 | 0 |
| Customers | 11 | 9 | 13 | 0 |
| Inventory | 9 | 10 | 5 | 0 |
| Payroll | 3 | 5 | 30 | 0 |
| Tax | 10 | 2 | 16 | 0 |
| Reports | 43 | 12 | 0 | 0 |
| GraphQL wrappers | 3 | 8 | 0 | 0 |
| Advanced | 3 | 8 | 10 | 1 |
| Cost groups | 1 | 3 | 0 | 0 |
| Integrations | 2 | 0 | 2 | 0 |
| Custom objects | 0 | 0 | 3 | 0 |
| Accountant | 0 | 0 | 1 | 0 |
| **Total** | **197** | **207** | **155** | **2** |

Of the **207 wired entries**, **107** report `verification: independent_readback`; **100** report `blocked_missing_readback` and cannot submit live writes. `wired` identifies an implemented request path, **not** an unconditional write capability. The additional trees and full GraphQL inventory are not fully represented by these catalog totals.

```bash
qb actions --mode read --json
qb actions --mode wired --json | jq '.[] | {id, command, verification}'
qb actions --json | jq '.[] | select(.verification == "independent_readback")'
qb gql ops --json
qb help
qb reports --help
```

Every command exposes its own `--help`; use it for exact supported parameters and restrictions.

## Sessions that maintain themselves

**Preserve the existing session → extend its native ticket → recover from the same source → use enrolled credentials only when signed out.**

1. Login starts a profile-bound background keeper by default. Banking health probes run every five minutes and retain server-issued cookie rotation.
2. For managed profiles and numerically pinned, agent-owned Ego tabs, due maintenance invokes QuickBooks' own TicketManager/authorization SDK. It observes the native session-extension success callback, verifies the same principal/company, captures fresh banking authorization and performs an independent CompanyInfo read. A failed extension is not reported as healthy maintenance; user-controlled or unavailable tabs are not reclaimed or replaced.
3. The schedule comes from the plugin's runtime extension time—not a fixed assumed ticket lifetime. The keeper wakes one minute early if the normal health interval would overshoot it.
4. Authentication rejection first permits source-pinned warm recovery, including restoration of the same session's usable exported cookies. Healthy sessions do not decrypt login credentials.
5. Only a confirmed sign-in wall and an enabled enrollment permit headless password/TOTP login. The logical session and identity stay bound; a new credential generation fences stale headers and late responses.

Transient network errors, rate limits and permission denials do not trigger password submission. Failed or unsupported verification challenges stop recovery rather than repeatedly retrying credentials. Logout, a new explicit login, disable and forget invalidate the relevant recovery state.

```bash
qb auth status --live --json
qb auth keepalive status --json
qb auth ticket status --json
qb auth recovery status --json
qb auth keepalive stop
qb auth keepalive start --interval 5m --managed-refresh
```

### Optional autonomous login recovery

After a verified managed login or capture of an existing, numerically pinned Ego space/tab:

```bash
qb auth enroll --launch --enable-recovery
qb auth recovery status --json
```

The prompts hide username, password and an existing authenticator BASE32 seed or `otpauth://totp/` URI. Automation can provide a single JSON object through `--credentials-stdin`; `--bootstrap` supports first login in a fresh profile, including headless setup. Never put secret values in command arguments, environment variables, shell history or committed files.

Credentials remain in qb-cli's own encrypted vault. Recovery needs neither Wright nor `op`. Enrollment starts session maintenance by default (`--keep-alive=false` disables startup). First-time setup selects the sole available company automatically; multiple companies require a choice, using a macOS dialog or terminal selection. Machine mode returns the choices for its host to display, then accepts `--company "Exact company name"`. Recovery always retains the selected company and account.

If an existing encrypted enrollment becomes unbound after an explicit login, `qb auth recovery rebind --launch` verifies the same company and principal before re-encrypting the binding for the current session. It never submits credentials during that verification or silently enables a disabled enrollment.

| Key backend | Use |
|---|---|
| macOS Keychain | Already-accessible key; no runtime approval/unlock prompt |
| Linux Secret Service | An available, unlocked service on the process's session D-Bus |
| GPG | Cross-platform; a service-accessible private key or unlocked agent, with runtime pinentry disabled |
| External private key file | Unattended macOS/Linux services, including environments without a desktop keyring |

Passwords/TOTP seeds are encrypted in `login-recovery.json`; principal/company/session binding metadata is not secret-encrypted. Browser/session tokens live in the private profile and mode-0600 `credentials.json`; they are not all encrypted by the recovery vault. No plaintext password fallback is provided. A same-user compromise can access a usable key and credentials; storing a password and TOTP seed gives that machine full login capability.

CAPTCHA, device/push approval, mandatory passkeys and unsupported MFA remain provider-controlled. Optional passkey prompts are cancelled through their AbortSignal only when the offered password method is selected. Account security settings are not changed.

Disable with `qb auth recovery disable`; remove enrollment with `qb auth recovery forget`. `login --keep-alive=false` or `QB_NO_KEEPALIVE=1` disables automatic keeper startup; `QB_NO_MANAGED=1` disables managed-browser maintenance/recovery. A process must still be running to maintain a session—configure service startup separately if required after reboot.

See [authentication and key provisioning](internal/quickbooks/AUTH-RECOVERY.md) for enrollment, Linux/systemd guidance and controlled recovery verification. Use `qb auth ticket extend --launch --force` for an explicit native-extension test; without `--launch` it only prints instructions.

## Financial writes require proof

**A successful API response is a receipt, not proof that the requested accounting change happened.**

- Public mutations without reviewed readback adapters are blocked before submission, even with `--yes`.
- `--dry-run` plans the request without executing the mutation.
- Reviewed writes compare independent resulting state with the intended resource, identity, fields and preserved invariants.
- Feed changes verify source/destination review states and accounting links. Batch operations correlate every receipt and verify every intended item.
- Confirmed outcomes report `verified:true`. Unverified/partial outcomes exit nonzero and preserve available receipt IDs and private evidence.
- Safe reads may recover and retry once after definite authentication rejection. Financial writes and unknown operations are never automatically replayed.
- After an uncertain write, recover with reads against the exact entity/feed IDs. Do not resubmit merely because a timeout or failed readback occurred.

The signed-in company is the target; there is no global company allowlist. Verify the company, select account/Class IDs explicitly, inspect the plan and confirm that the catalog exposes a verifier before submitting.

See [independent-readback contracts](docs/MUTATION_READBACK.md).

## Everyday workflows

Replace symbolic IDs below with identifiers from the intended company. Examples containing `--dry-run` do not submit financial changes.

```bash
# Discover accounts and search the complete banking review-state population.
qb feed account list --json
qb feed txn get --account-id BANK_ACCOUNT_ID --query "example reference" --limit 0 --json

# Summary/detail reporting and saved definitions.
qb reports profit-loss get --report ProfitAndLoss --date-range 2026-01-01,2026-01-31 --json
qb reports profit-loss-detail get --klass CLASS_ID --accounting-method Cash --json
qb reports saved list --json
qb reports memorized --help
qb reports management audit --id FOLIO_ID --from 2026-01 --to 2026-03 --json

# Historical audit evidence, including revision snapshots.
qb accounting audit-log get --from 2026-01-01T00:00:00Z --to 2026-01-31T23:59:59.999Z --offset 0 --limit 50 --include-snapshots --json

# Browser-backed GraphQL discovery and pagination.
qb gql ops Inventory --json
qb gql walk items --json
qb gql walk bills --help
qb gql walk tasks --help

# Inspect private/local accounting policy without posting.
qb policy docs --json
qb policy check-journal --help

# Preview a journal batch; descriptions and private notes are separate.
qb accounting journal create --items-json '[{"date":"2026-01-31","amount":100,"from-account":"SOURCE_ACCOUNT_ID","to-account":"DESTINATION_ACCOUNT_ID","class":"CLASS_ID","description":"Example balancing entry","memo":"Example supporting reference"}]' --dry-run --json
```

## Boundaries and trade-offs

- **QuickBooks Online, not Desktop.** Desktop IIF workflows and OAuth-only current-user endpoints are outside this session model.
- **AU-oriented webapp coverage.** Entitlements, locale, permissions and regional service availability can limit an operation.
- **Not every payroll/tax workflow is executable.** Employee/time-activity adapters and tax/BAS/IAS/TPAR reads do not imply live pay-run finalisation, STP submission, BAS lodgement or tax-code mutation coverage.
- **Not every service builder is a live capability.** Arbitrary GraphQL mutations, unverified management-folio writes, feed transfers without two-sided proof and unsupported delivery claims remain gated.
- **Receipts and attachments are different resources.** OCR receipt read/search/delete aliases are blocked where they would address ordinary attachables instead. Consult the catalog for receipt upload/conversion and attachment capabilities.
- **Native surfaces can change.** Browser-backed APIs are broader than v3 but are not Intuit's stable, documented public integration contract. Unsupported shapes fail explicitly.
- **No blanket live-certification claim.** Tests and capability registrations are not evidence that every endpoint has been independently exercised in every company.
- **Privacy is local-first.** Credentials, captures, private statements and private directories are excluded from public source control. Do not use real customer identities or private financial records in published examples.

## Develop and extend

```bash
go build -o ./qb ./cmd/qb
go test ./...
go fmt ./...
golangci-lint run ./...
bash scripts/golden.sh verify
```

The module requires Go 1.26.9; set `GOTOOLCHAIN=go1.26.9` when validating against that toolchain. Opt-in isolated Chrome regressions use `QB_BROWSER_TEST=1`. Linux cross-builds check compilation, not a live Linux/Intuit session.

| Location | Responsibility |
|---|---|
| `cmd/qb` | CLI entry point |
| `internal/quickbooks/cli` | Commands, catalog, parameters and public safety gates |
| `internal/quickbooks/client` | Accounting/native service requests and mutation readback |
| `internal/quickbooks/auth` | Profiles, sessions, native ticket maintenance, vaults, TOTP and recovery |
| `internal/quickbooks/gql` | Captured operations, browser execution and pagination |
| `internal/quickbooks/gql/gen` | GraphQL catalog generation |
| `internal/quickbooks/doctor` | Session/channel diagnostics |

The repository also provides **`cli-printing-press`**, a separate developer tool for API discovery, spec-driven Go CLI generation, MCP generation/bundling, runtime verification, workflow tests, capability audits and local library management. It is not the `qb` runtime or a claim that `qb` itself exposes a standalone MCP server. Discover its complete surface with `./cli-printing-press --help`; see [pipeline](docs/PIPELINE.md), [artifact layout](docs/ARTIFACTS.md) and [development conventions](AGENTS.md).

That developer tool uses `~/printing-press/.runstate/<scope>/runs/<run-id>/working/<api>-pp-cli` for working prints, `~/printing-press/library/<api>` for accepted local CLIs, and `~/printing-press/manuscripts/<api>/<run-id>/` for retained `research/`, `proofs/`, `discovery/`, and `pipeline/` evidence. These are Printing Press artifacts, not financial backups or `qb` mutation evidence.
