# qb — QuickBooks Online CLI

[![CI](https://github.com/mvanhorn/cli-printing-press/actions/workflows/lint.yml/badge.svg)](https://github.com/mvanhorn/cli-printing-press/actions/workflows/lint.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/mvanhorn/cli-printing-press)](go.mod)
[![License](https://img.shields.io/github/license/mvanhorn/cli-printing-press)](LICENSE)

`qb` drives the whole of QuickBooks Online (AU) from the terminal: books, banking feeds, sales, expenses, payroll, tax, inventory, reports — 543 catalogued operations across 16 domains, with live reads, live writes, dry-runs, and an authenticated session that heals itself. Built agent-first (`--json` everywhere, `qb actions` as a machine-readable contract), equally usable by humans.

Printed by the [Printing Press](https://printingpress.dev) (`cli-printing-press`, same repo) from the official v3 API, the QBO webapp's own captured GraphQL, and the banking-feed surfaces Intuit never published.

## Quickstart

```bash
go build -o qb ./cmd/qb
./qb login                 # sign in once (managed Chromium window)
./qb auth whoami           # Signed in live as <company> (<email>, realm <id>)
./qb company company-info get
```

`login` tries, in order: an already-open QBO tab (Chrome relay) → a **managed Chromium profile** (`~/.config/qb/profiles/qbo`, reused across logins) → an isolated browser Space. You sign in; `qb` mines the ATS session, cookies, and company identity. Every success leads with the bound company — you always know which books you're touching.

## Auth model

- **Session rules.** No configured company, no allowlist: every command operates on whatever company you signed in as. Verify anytime with `qb auth whoami` (live tab) or `qb auth status --live` (v3 ping: `live` / `STALE` / `unknown`).
- **Auto-renewal is the default.** Any 401 ladders relay → headless profile refresh → replays the failed call. A dead session costs one slow command, then just works. Nothing to configure; `QB_NO_MANAGED=1` opts out of the browser rung.
- Secrets live only in `~/.config/qb/credentials.json` (0600). `--json` output never includes them.

## Coverage (543 primitives)

| Domain | Wired | Read | Command root |
|---|---|---|---|
| Accounting (COA, class, budget, journals, fixed assets, CDC) | 30 | 25 | `qb accounting` |
| Expenses (bills, suppliers, cheques, time activities) | 23 | 16 | `qb expenses` |
| Sales (invoices, estimates, payments, sends, PDFs, sales orders) | 29 | 16 | `qb sales` |
| Company (settings, users, lists, attachables, rates) | 12 | 18 | `qb company` |
| Feed (banking classify/split/exclude/undo) | 7 | 10 | `qb feed` |
| Payroll (pay runs, STP, super) | 5 | 3 | `qb payroll` |
| Tax (GST, BAS, TPAR, agencies, codes) | 2 | 5 | `qb tax` |
| Inventory (items, purchase orders) | 7 | 8 | `qb inventory` |
| Customers | 3 | 11 | `qb customers` |
| Reports + forecasts | 0 | 28 | `qb reports` |
| GraphQL (captured webapp ops) | 8 | 3 | `qb gql` |
| Cost groups, custom objects, accountant | 3 | 1 | `qb costgroups`, `qb customobjects`, `qb accountant` |
| Integrations, advanced (service maps, batch) | 1 | 2 | `qb integrations`, `qb advanced` |

`qb crm` and `qb salestx` are additional command trees outside the actions catalog. `salestx` includes live v3 sales transactions and tax-service operations; its plan IDs are command identifiers, not catalog entries.

Totals: **130 wired, 146 read-only, 265 blocked, 2 excluded of 543 actions.** Run `qb actions` for exact per-row counts — the table above is the map, the catalog is the truth.

Examples:

```bash
qb feed txn list --account-id 44 --limit 5
qb accounting class create --name "Marketing"   # then get / update / delete
qb sales invoice get --id 198
qb company company-info get
qb reports profit-loss get --report ProfitAndLoss --date-range 2026-07-01,2026-09-01
qb actions --mode wired --json | jq '.[].id'
```

## Banking feeds

The differentiator: full replay of the QBO banking UI — pending/excluded/register views, classify, split, exclude, undo, CSV import — with `qb doctor` gating session health first and `--dry-run` planning every mutation before it dials.

```bash
qb doctor
qb feed txn list --account-id 44
qb feed txn import --account-id 44 --file rows.csv --dry-run
```

## Captured GraphQL

`qb gql` runs operations captured from the production webapp (293 in catalog, 11 wired to verbs) inside your authenticated tab:

```bash
qb gql ops Commerce        # browse offline
qb gql mutate receive-inventory --txn-id 198 --txn-type Bill --lines-json '[...]'
qb gql query CommerceInventoryMovement --help
```

## Safety

- `--dry-run` plans any mutation without dialing.
- Writes announce their target; `whoami`/`status` always show the bound company.
- There is no safety allowlist: `qb` writes to the signed-in company. Check `whoami` before bulk jobs.

## vs the official MCP server

Compared tool-for-tool against [`intuit/quickbooks-online-mcp-server`](https://github.com/intuit/quickbooks-online-mcp-server)
(142 tools, read from its `src/tools/*.tool.ts` + handlers, not its docs).
It speaks only the official OAuth v3 API and needs an Intuit developer app
(`QUICKBOOKS_CLIENT_ID/SECRET`, refresh token in env). `qb` signs in as you
and additionally replays the webapp's own surfaces — no developer app.

**Entire surfaces the MCP server has nothing for:** banking feeds
(classify/split/exclude/undo/import, 37 rows), payroll (38), AU tax
(GST/BAS/TPAR reads), captured webapp GraphQL (11 verbs, 8 wired: inventory
movements, product activation, reconcile…), inventory adjustments, cost
groups, custom/forecast/management/performance reports, company
settings/users/roles/tags/currency, audit log, recurring transactions,
projects, mileage/cheques/bank fees, sales orders, customer/supplier
overviews and proposals.

**Also covered:** attachable file upload + CRUD, Department and TimeActivity
CRUD + search, Profit-and-Loss and Balance Sheet reports, Preferences,
vendor/customer balances, customer sales, vendor expenses, Purchase search,
invoice/estimate/credit-memo/receipt PDF download and sends, CDC change
streams, ExchangeRate/CustomerType/CreditCardPayment reads, TaxCode create
via taxservice (with agency + rate creates), webhook HMAC verify, and detail
reports (ProfitAndLossDetail, InventoryValuation, TransactionList,
JournalReport). v3 CRUD on the ~25 core entities (bill, invoice, customer,
vendor, item, account, …) is covered by both.

**Out of scope:** current-user needs OAuth bearer (not the session model);
Desktop IIF import targets QuickBooks Desktop, not Online.


## How it works

```mermaid
flowchart TB
    user([You]) --> cmd[qb command]
    cmd --> sess{credentials.json\nusable?}
    sess -- no --> login[qb login]
    login --> relay{relay tab\nopen?}
    relay -- yes --> intercept[intercept ATS key]
    relay -- no --> managed[managed Chromium\npersistent profile]
    managed --> signin([sign in once])
    signin --> intercept
    intercept --> mine[mine company/email/realm]
    mine --> save[(credentials.json)]
    save --> announce[Logged in as ...]
    sess -- yes --> route{operation type}
    route --> v3[v3 query/mutate\nqbo.intuit.com/api/v3]
    route --> ats[banking feed\nqbo.intuit.com/ats + neo]
    route --> gqlq[GraphQL in-page fetch\nwarehouse / v4 / spendlists]
    v3 & ats & gqlq --> ok{HTTP 200?}
    ok -- yes --> out([result])
    ok -- 401 --> heal[auto-renew:\nrelay -> headless refresh -> replay]
    heal --> out
```

## Develop

```bash
go build ./... && go test ./internal/quickbooks/... -count=1
```

Layout: `cmd/qb` (entrypoint) · `internal/quickbooks/{cli,client,auth,gql,doctor}` (product) · `internal/pressauth` (managed-Chrome auth) · `internal/quickbooks/gql/gen` (catalog generator; `catalog_data.go` is generated — fix the generator, never the file).

## The Press

This repo also holds `cli-printing-press`, the generator that printed `qb` (API studies, sniffing, verification, skills + MCP output). `qb` is its reference print. See [printingpress.dev](https://printingpress.dev).

Printing Press runs generate their working CLI at `~/printing-press/.runstate/<scope>/runs/<run-id>/working/<api>-pp-cli` and promote it to the local library at `~/printing-press/library/<api>`. Archived manuscripts live at `~/printing-press/manuscripts/<api>/<run-id>/`, with `research/`, `proofs/`, `discovery/`, and `pipeline/` evidence. See [Local Artifacts and Public Library](docs/ARTIFACTS.md) for the artifact lifecycle.
