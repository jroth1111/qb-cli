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
- **Keep the same session first.** The read-only keeper probes every five
  minutes and persists server-issued cookie rotation. A definite 401 allows
  one source-pinned warm recapture; only reads may be replayed. Managed
  sessions never switch to an unrelated relay/Ego profile. `QB_NO_MANAGED=1`
  disables the managed-browser rung. `status --live` proves liveness via the v3 channel
  (ATS headers — never the homepage shell, which redirects even for
  healthy jars). `auth whoami` reports the live-tab company first.
- Credentials: `$QB_HOME/credentials.json`, 0600, atomic write.
- **Autonomous re-login is opt-in.** `auth enroll --launch --enable-recovery`
  binds terminal or explicit `--credentials-stdin` username/password/TOTP input to a verified managed
  session. Secrets are encrypted; keys use macOS Keychain, Linux Secret
  Service, cross-platform GPG, or an explicitly provisioned external private
  key file. `--bootstrap` supports an initial credential login in a fresh home.
  Only a confirmed sign-in wall after warm recapture permits using
  these credentials. Recovery never requests approval or replays writes;
  unsupported challenges/locked keys stop safely. See [AUTH-RECOVERY.md](AUTH-RECOVERY.md).

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
- Management reports (statement folios): `qb reports management list` /
  `qb reports management get --id sbg:…` read universalreportinsights
  `/v1/folio` (`reportKindEnum=FOLIO`). `get` projects `folioDataRequest`
  into the ordered page table — REPORT pages carry `reportToken` (the
  classic `mem_rpt_id`), a per-page `reportDateMacro`/`startDate`/`endDate`
  that overrides the saved report's own period at render, and `fullName`
  resolved from the MEMORIZED or builder registry. Stored page dates are
  the last-saved configuration, not evidence of the last-generated period
  (renders can override dates without persisting).
- Management period pinning updates the folio window, every `REPORT` and
  `URI_REPORT` segment (including segments with no existing macro), and
  the cover's `{ReportEndDate}` dynamic field. Explicit `all` segments
  retain their running-ledger scope. Native preview was observed using
  `POST universalreportsfolios.api.intuit.com/v1/managementreports/{templateID}/instances`;
  this is a different service from `/v1/folio` CRUD. The CLI period-helper
  fix does not enable unverified folio mutations or provide a PDF renderer.
- `qb reports memorized run --id <N>` executes the saved report through
  `createReports_Report` on v4/entities — the same call the classic grid
  makes — and returns the section/transaction/total row tree. Bare `--id`
  runs the saved period (and echoes the resolved option set under
  `header.resolvedOptions`); `--date-range`/`--basis` replay those saved
  filters (token, account whitelist, class list) through the equivalent
  v3 report (`TX_DET_BY_ACCT` → ProfitAndLossDetail, `PANDL` →
  ProfitAndLoss, …) for the requested window.
- `qb reports management audit --id sbg:… --from YYYY-MM --to YYYY-MM`
  atomically audits one folio across a statement window: every REPORT page
  resolves its saved filters via `resolvedOptions`, classifies by scope
  (`reportDateMacro=all` → alltime ledger page; >1 class → combined;
  1 class → unit; 0 → company/unscoped), then replays each dated page per
  month through the equivalent v3 report. Per period it checks
  Σ unit nets == combined net, class-set coverage (unit ∪ == combined),
  and account-whitelist drift both directions. Alltime pages run once per
  class with `date_macro=All` and are sliced per month by row date; the
  cumulative position per unit is Σ period nets − in-window distributions.
  Limits: requests are sequential (one snapshot, not a transaction); saved
  report defs have no history so this proves what the folio computes
  *today* for each period — the delivered PDF remains the only proof of
  what was actually sent.
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
- `accounting journal create` uses the independently verified batch adapter:
  every created ID is read back and compared against its intended date, amounts,
  accounts, class and descriptions. Partial results return nonzero with recovery
  IDs; do not replay a write on an unverified outcome. Items support an optional
  `description` for both accounting lines, separate from the private `memo`.
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
