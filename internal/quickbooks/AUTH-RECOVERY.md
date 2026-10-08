# Session maintenance and autonomous login recovery

Keep the same QBO session alive first. The existing keeper sends read-only
banking GETs at five-minute intervals and persists server-issued cookie rotation.
Healthy sessions do not resolve passwords, TOTP seeds or vault keys. Network
errors, rate limits and permission denials do not initiate re-login.

Managed sessions and numerically pinned, agent-owned Ego tabs also use the site's native TicketManager/authorization SDK.
When due, it makes the real sessionextender permission read (a POST authorization
batch, not a financial write). A native PERMIT success callback, unchanged
principal/company, fresh banking authorization and an independent CompanyInfo
GET are required before saving renewed state. The keeper uses the plugin's
runtime next-extension time and wakes one minute early if the ordinary banking
interval would overshoot it; it does not hard-code a ticket lifetime or send an
extension request every minute. Failed extensions remain visibly unverified and never resolve
login credentials while the banking probe is healthy.

Inspect timing with `qb auth ticket status --json` or the `native_ticket` field
in `qb auth keepalive status --json`. `qb auth ticket extend` prints instructions
by default; `--launch` permits a due headless extension, and `--force` explicitly
tests one even when it is not due. Verification/dogfood harnesses refuse native
browser maintenance. New login/logout or credential recovery resets the ticket
schedule with the stored login, without reusing another generation's timing.

After a definite authentication rejection, renewal recaptures fresh native
headers from the same browser profile. If that profile shows a sign-in wall,
renewal first tries the same session's unexpired exported cookies (which the
keeper may have rotated while Chromium was closed); it never restores over a
live browser identity. Native headers and independent API proof are still required.
Only when that owned browser is confirmed
signed out does an enabled enrollment submit username/password and an authenticator
code. Routine recovery requires no user approval. CAPTCHA, SMS/email codes, push
approval, mandatory passkeys, unexpected pages and rejected credentials fail closed; they
are not bypassed. A failed login latches `needs_attention` instead of repeatedly
submitting the password. Re-enroll after fixing a rejected credential/challenge.

## One-time enrollment

Use hidden terminal prompts, or explicit stdin input for automation:

```sh
qb login --source managed
qb auth enroll --launch --enable-recovery
qb auth recovery status --json
qb auth keepalive status --json
```

The enrollment prompts hide the username, password and existing authenticator
BASE32 seed (or `otpauth://totp/` URI). A six-digit code is not a seed. Enrollment
does not enable/change MFA or account security settings. `--credentials-stdin`
accepts a JSON object with `username`, `password` and optional `totp` from an agent,
pipe or other automation. It works with `--json --no-input`; these options do not
require terminal input or an additional confirmation prompt. Keep secret values
out of command arguments, shell history and environment variables. The enrollment
command is hidden from MCP because its stdin stream is not an MCP argument, and
refuses verification/dogfood harnesses. Without both
opt-in flags it prints instructions and performs no enrollment or browser launch.

For first-login automation, add `--bootstrap` in a fresh QB_HOME:

```sh
qb --home /absolute/private/qb-profile auth enroll \
  --credentials-stdin --bootstrap --launch --enable-recovery \
  --key-file /absolute/private/key --json --no-input
```

Supply the JSON through the command's stdin stream. `--bootstrap` submits the
credentials to Intuit, obtains fresh native authorization, verifies hydrated
identity and a real CompanyInfo response, then binds encrypted recovery to that
observed identity. It never replaces an existing saved login. `--headed` selects
a visible Chromium window for enrollment/recovery; otherwise the flow is headless.
When Intuit automatically opens an optional passkey chooser, password recovery
waits for that request and cancels it with its AbortSignal before selecting the
offered password method. No passkey is synthesized and no MFA requirement is
removed. A passkey-only flow still stops for attention. The same cancellation
works without a desktop or native-dialog interaction.
Secret-free diagnostics report the step, control descriptors and credential
purposes used, never input values, authorization headers or URL query strings.

Enrollment independently verifies hydrated browser identity and an actual
CompanyInfo GET before storing secrets. This proves the current browser session,
not the correctness of credentials that have not yet been used for a fresh login.

The encrypted `login-recovery.json` is profile/account/company/login bound.
A new explicit login or logout invalidates that binding; enroll again for the
new session, or use `qb auth recovery rebind --launch` to re-encrypt the existing
enrollment after independent warm verification of the same home, company and
principal. Rebinding never submits credentials or silently enables a disabled
enrollment. This is not a global company allowlist: the initial explicit login
still selects the company. Automatic recovery cannot switch it.

Existing Ego enrollment requires a saved numeric space ID and exact tab target.
The tab must remain agent-owned; recovery never claims user-controlled/inactive
spaces, opens a replacement tab, or falls back to another browser. Healthy warm
capture does not decrypt the vault. Confirmed sign-in permits only the enrolled
username/password and offered authenticator flow, with values sent in a private
process stdin pipe—not arguments, environment variables or plaintext files.
Managed bootstrap remains the first-login path; local-cookie-loss verification
is restricted to its isolated test profile, never a user's daily Ego profile.

## Key storage on macOS and Linux

| Backend | Availability | Autonomous behavior |
| --- | --- | --- |
| Default native | macOS Keychain; Linux Secret Service | Reads only already-accessible secrets; no runtime unlock/approval prompt |
| `--gpg-recipient <full-fingerprint>` | GnuPG on either platform | Requires an already-unlocked agent or service-accessible private key; locked keys fail closed |
| `--key-file /absolute/private/key` | Either platform, including headless Linux | Direct machine access, including after restart; no keyring/agent prompt |

Linux native enrollment uses `secret-tool` (libsecret) and requires a running
Secret Service on the service user's session D-Bus. Runtime access uses
SearchItems/OpenSession/GetSecrets directly and never calls Unlock or Prompt.
An SSH/cron/system service often lacks that desktop session; do not assume it
inherits the user's unlocked keyring. In that case choose GPG or a private key
file explicitly. There is no automatic plaintext fallback.

GPG uses a full trusted encryption-key fingerprint, not an ambiguous name or
email. Encryption/decryption use stdin/stdout pipes; no plaintext temporary
files, clipboard or git synchronization. Recovery disables pinentry and remote
key retrieval. An agent's cache timeout/reboot can remove access, so GPG alone
does not guarantee unattended recovery across restarts. Unlock/provision keys
outside `qb`; `qb` does not weaken GPG protection or save its passphrase.

For fully autonomous service access, create a private directory outside QB_HOME
and provision a 32-byte key without overwriting an existing file:

```sh
qb auth recovery key-create --path /absolute/private/key --write
qb auth enroll --launch --enable-recovery --key-file /absolute/private/key
```

`key-create` requires an existing private directory. The key file must be a
regular non-symlink file readable by the process, with no group/world permissions,
owned by the service user or root. The CLI never deletes externally provisioned
key files. Protect it separately from QB_HOME; disk encryption and a dedicated
unprivileged service user are recommended. A same-user compromise can read the
key and encrypted credentials: encryption does not prevent that. Storing both
password and TOTP seed gives the machine full login capability.

For systemd, prefer `LoadCredentialEncrypted=` and its protected runtime
credential file. Set `QB_RECOVERY_KEY_FILE` to the delivered key-file path if it
differs from the path used at enrollment. This variable contains a path, not a
secret. systemd credential files may be mode 0400; they are accepted. See
[systemd credentials](https://systemd.io/CREDENTIALS/). Use the same key bytes
at enrollment/runtime, keep a stable QB_HOME, and run Chromium as an unprivileged
user. Chromium must be installed; `qb` does not disable its sandbox or install
software. `--bootstrap --credentials-stdin` supports first login without a display.
MFA/device challenges may still prevent unattended login.

## Recovery and revocation guarantees

- HTTP GETs, classified GraphQL queries and browser GETs may retry once.
  Financial writes and unknown operations are never automatically replayed.
- Managed GraphQL/browser REST execution uses the same managed profile, not
  an unrelated Ego space. Existing Ego sessions retain their own backend.
- A renewal lease serializes keepalive/request recovery; a browser lease
  prevents concurrent Chromium launches on the same persistent profile.
- Full login verifies native authorization, principal/realm and independent
  API access before atomic credential replacement. It retains the logical
  login ID but advances the credential generation, rejecting late old-session
  cookie writes and discarding stale service/audit headers.
- Logout, a newer explicit login, disable or forget prevents in-flight recovery
  from replacing/resurrecting state. Failed captures leave stored credentials alone.
- Credential attempts are bounded and failures are reported without provider
  stderr, credential values, page text or input values.

```sh
qb auth recovery disable
qb auth recovery forget
qb auth logout
```

Disable keeps encrypted secrets but prohibits re-login. Forget also removes the
encrypted record and its owned native-keyring item; it does not delete a GPG key
or an external key file. Logout removes session state and stops its keeper.

## Verification boundary

Unit/state-machine, isolated GPG and transport tests establish the mechanics,
not a live Intuit login. Final live acceptance requires a dedicated enrolled
profile, controlled local auth loss, observed credential/TOTP login, native
capture, same-identity proof and independent HTTP/browser readback with zero
financial writes. Simulated local loss is not proof of natural server expiry.

Design references: [gopass GPG backend](https://github.com/gopasspw/gopass/tree/master/internal/backend/crypto/gpg/cli)
for storage/crypto/agent separation, and [KeePassXC CLI](https://github.com/keepassxreboot/keepassxc/blob/develop/src/cli/Show.cpp)
for resolving fields/TOTP on demand. These are design references, not copied
implementation code or runtime password-manager dependencies.
