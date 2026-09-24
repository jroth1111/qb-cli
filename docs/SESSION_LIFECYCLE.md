# Keeping a QBO session usable

`qb login` and `qb auth remint` now start one background session keeper per
`--home`/`QB_HOME` profile by default. Login reports `keep_alive` as
`start_requested`, `already_running`, `disabled` or `unavailable`; a start
request is not proof that QBO has accepted the first probe.

```sh
qb login
qb auth keepalive status --json

# Existing captured session; no new interactive login:
qb auth keepalive start --interval 5m

# Stop maintenance without deleting credentials:
qb auth keepalive stop

# Disable automatic startup for this login:
qb login --keep-alive=false
# Or set QB_NO_KEEPALIVE=1 for login processes.

# Foreground maintenance until interrupted:
qb auth keepalive
```

`qb auth logout` deletes local credentials and stops maintenance. It does not
sign the user out of every browser. Stop/logout/profile replacement prevents
further probes; a currently in-flight read can finish within its bounded
timeout. Idle control checks run at most 30 seconds apart. A new login has a
new generation, so old keepers and responses cannot restore an old session.

## What is automatic

- Native API requests reload current credentials and persist QBO's `Set-Cookie`
  rotation, including expiry/deletion, domain/path scope and matching CSRF
  changes. The separate captured `x-csrf-token` is not overwritten merely
  because it differs from the cookie token.
- Credential updates are atomic and serialized across processes. Late responses
  cannot overwrite a new login or a cookie already rotated by another request.
- The keeper uses validated banking **GETs**, normally every five minutes.
  Temporary outages, contract errors and permission denials back off rather
  than being reported as a healthy connection. Backoff is capped at 30 minutes.
- After definite authentication rejection, quiet recovery can copy cookies from
  an already-authenticated matching relay tab without navigating it, or use an
  **existing** managed profile headlessly. It never fills login forms or handles
  MFA. Use `--managed-refresh=false` on `keepalive start`, or `QB_NO_MANAGED=1`,
  to disable the managed path. `login --no-open` starts a relay-only keeper.
- Full managed renewal now preserves captured per-service headers and the audit
  key. Renewal must prove the original company and principal; a missing email
  can use matching captured user IDs, not a guess.
- Login retains its owned Ego capture space when maintenance is enabled. An
  operator-designated shared Ego space is never closed by capture. User-control
  errors are surfaced; qb-cli does not automatically claim the space back.

Safe native GETs and explicitly read-only GraphQL queries can recover once from
a 401. Native writes are not automatically repeated after renewal, and GraphQL
mutations are never repeated by the renewal wrapper. Inspect the target before
retrying an uncertain write. A local cookie-persistence failure does not change
the outcome of a request that QBO may already have applied.

## Status and limits

`qb auth keepalive status --json` reports the local worker lease, last completed
check, next check, HTTP status and last result. `live` means QBO accepted the
last validated probe, not a promise about the next request. `needs_login`
means explicit reauthentication is required; `temporarily_unavailable` includes
an inconclusive recovery attempt. A dead worker's old status file alone is not
reported as a running process. Files stay in the private credential profile;
no cookie or authorization value is printed.

This is **best-effort API session maintenance**, not unlimited authentication.
It cannot maintain connectivity while the computer is asleep/offline or override
Intuit's expiry, revocation, MFA or company policy. A read-only heartbeat does
not prove that the browser UI's inactivity timer has reset.

Intuit documents a default one-hour browser inactivity timeout, configurable by
the Primary Admin up to three hours for all company users. Its UI inactivity
measurement is based on interaction with transaction forms. qb-cli does not
change that company-wide security setting or synthesize user input.
See [Intuit's timeout documentation](https://quickbooks.intuit.com/learn-support/en-au/help-article/data-security/change-timeout-duration-quickbooks-online/L1osjv4ex_AU_en_AU).

No keeper or browser-refresh subprocess starts under
`PRINTING_PRESS_VERIFY=1` or `PRINTING_PRESS_DOGFOOD=1`. Unit tests use isolated
profiles and mock transports; a successful test run is not evidence of hours of
live session uptime.
