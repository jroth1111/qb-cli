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
