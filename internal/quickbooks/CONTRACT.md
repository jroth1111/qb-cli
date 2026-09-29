# qb CLI contract (current state)

Binary: `qb`, built from `cmd/qb/main.go` in this repo.
Module: `github.com/mvanhorn/cli-printing-press/v4`.

## Auth model

- **Session rules.** No configured company, no allowlist: every command
  operates on the login session's company. Never add a company gate.
- **Login ladder** (`qb login`, shared by `qb auth remint`):
  1. Open QBO tab on the OMP relay (`$QB_RELAY_URL` or
     `http://127.0.0.1:9224`), else
  2. managed Chromium on the persistent profile
     (`$QB_HOME/profiles/qbo`), else
  3. isolated ego-browser Space.
- You sign in; `qb` mines ATS headers, cookies, and company/email/realm
  from the hydrated tab. Every success leads with the bound company
  (`Logged in as ...`); `--json` output never carries secrets.
- **Auto-renewal is default.** Any 401 ladders relay → headless managed
  refresh → ego, then replays. `QB_NO_MANAGED=1` opts out of the browser
  rung only. `status --live` proves liveness via the v3 channel
  (ATS headers — never the homepage shell, which redirects even for
  healthy jars). `auth whoami` reports the live-tab company first.
- Credentials: `$QB_HOME/credentials.json`, 0600, atomic write.

## CLI conventions

- Shape: `qb <domain> <entity> <verb>`. Agent entry point: `qb actions --json`.
- Row modes: `read` / `wired` honour flags; `blocked` documents flags then
  exits not-wired; `excluded` has no command.
- Dates are `dd/MM/yyyy` (en-AU). Feed mutations take olbTxnIds.
- `--dry-run` plans without dialing. `--home` overrides `QB_HOME`.
- Reports: `qb reports <entity> get` runs the named v3 report
  (`/reports/{Name}?minorversion=73`). `--date-range`/`--accounting-method`/
  `--columns` map to `start_date`/`end_date`/`accounting_method`/
  `summarize_column_by`; every other documented ReportService param is a
  typed flag (`--klass`, `--account`, `--customer`, `--vendor`,
  `--department`, `--transaction-type`, `--group-by`, `--sort-by`, …) or
  repeatable `--param key=value`; unknown keys are rejected, never silently
  dropped.
- Saved custom reports: `qb reports saved list [--query name]` /
  `qb reports saved get --id sbg:…` read the CRB_REPORT definitions on
  universalreportinsights `/v1/reports/qbo` — `get` returns the embedded
  `dataRequest` (filters/groups/projections), which is where a saved
  report's class/account/date filters live. `reports report
  create|update|delete` mutate those definitions.
- Memorized custom reports: `qb reports memorized list [--query name]` /
  `qb reports memorized get --id <composite|mem_rpt_id>` read the
  MEMORIZED registry behind `/app/customreports` via the v4/entities
  `reportDefinitions` graphQuery (first-party qbo.intuit.com POST — not
  the leftover-host lane). The `docNumber` field carries the classic
  reportv2 `mem_rpt_id` (the trailing `:N` of the composite id), so
  `--id 42` resolves "Melo Detailed Statement".
- Classic exports: `qb company data-export get --token <T> --out f.xlsx`
  POSTs the exportReport renderer; `--attr key=value` merges arbitrary
  reportCustomizationAttributes, so a saved report exports the way the
  classic renderer sees it (`--token TX_DET_BY_ACCT --attr mem_rpt_id=N
  --attr low_date=… --attr klass=… --attr cash_basis=yes`). Dates in
  `low_date`/`high_date` are `dd/MM/yyyy`.
- `qb policy` serves embedded company reference data (curated static
  content, `pp:novel-static-reference`): fee rules, paired-journal
  templates, and the chart-of-accounts/classification docs for the
  Mega Style Apartments profile. Read-only, offline, no company gate —
  it documents treatment rules; it never posts or asserts them. The
  chart-of-accounts and class tables were regenerated from a live v3
  query (2026-09-29) after the source capture proved truncated.
- `qb policy check-journal` validates a prepared journal (v3
  JournalEntry or the `--items-json` create shape) against those rules:
  balance, paired accounts, same class both lines, 20.9% fee recompute,
  cleaning carry-forward, recharge margin evidence. Balanced multi-pair
  entries decompose and check each pair against its own template — the
  documented "complete balanced pairs" convention. Exit is nonzero
  unless every journal passes; unresolved evidence is `unverified`,
  never silently pass. `accounting journal get --id` returns the full
  entry in `detail` so it pipes straight into check-journal.
- `qb policy categorise <text>` suggests account/class treatment from
  the embedded signal registry (posted-pattern evidence counts). Weak
  (<98%) signals and known conflicts never auto-suggest; they surface
  discriminators and alternatives. Exit is nonzero unless the verdict
  is `suggested`.
- `qb policy owner-net` computes the owner-statement equation. Evidence
  intake is composable and stays offline: `--journals` derives the
  deduction side from a `journal get --id` stream; `--pnl` derives the
  revenue/charged side from a class-filtered `reports profit-loss get
  --klass <id> --json` payload (income rows: Client Property Revenue,
  Management Income, Cleaning Amenities Income). Explicit flags win;
  agreeing sources confirm, disagreeing sources are reported — never
  silently folded.
- v3 query reads page via `STARTPOSITION` when `--limit` exceeds 100
  (ordered windows of 100); earlier requests were silently clamped at
  one page.

## Tests

Package tests only. Every suite keeps a negative case; new falsifiers for
regressions. Contract tests assert behavior, never source text. Tests must
not dial production: stub relays, dead relay URLs, temp `QB_HOME`.

## Forbidden

- Do not mutate QBO outside explicit lifecycle tests (create→verify→
  delete→verify-absent) on test companies.
- Do not print cookie values, Authorization headers, or token strings
  (tests, logs, stdout, diffs).
- Do not hand-edit generated files (`catalog_data.go`, `*_gen.go`
  while their generator exists — fix the generator instead).
- Do not write secrets to stdout, logs, or tests.
