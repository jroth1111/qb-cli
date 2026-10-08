# Bank-rule edit safety

Editing a rule preserves omitted criteria. Operator-only edits use the existing
unique selector value; repeated selectors fail closed instead of choosing one.
Exclusions are appended without deleting existing selectors.

Use `feed rule update --id ID --preview true ...` for a live, read-only hydrated
before/proposed-body comparison. This is distinct from `--dry-run`, which remains
offline. Preview never calls save and does not verify QuickBooks' native matching
population. Do not treat it as an auto-post acceptance gate by itself.

Full replacement requires `--replace-conditions true`. Potentially broader
auto-post criteria require `--allow-broadened-auto-post true`; the conservative
guard accepts preservation or additional ALL constraints without that override.
The acknowledgement is not a matching-population proof.

Before any recovery write, obtain the exact record identity, current sequence,
financial fields, bank link and clearing state. Compare per-record audit history
against the proposed rollback point. Stop on intervening legitimate edits rather
than restoring a stale population snapshot. Never use editing controls during a
read-only UI check: Unmatch and similar controls may commit immediately.

These changes do not restore transactions, deploy a binary, or certify books.
