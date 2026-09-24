# Mutation success means independent readback

The public `qb` command tree gates every catalogued mutation and legacy alias.
A successful write response is only a receipt. A live mutation returns success
only when a reviewed adapter independently reads the resulting system state and
confirms the requested changes and relevant preserved fields.

`qb actions --json` exposes `verification`:

- `independent_readback`: a reviewed adapter is available. Runtime readback can
  still fail or be inconclusive, in which case the command exits nonzero.
- `blocked_missing_readback`: no reviewed adapter. Live submission is blocked
  **before the handler runs**; `--dry-run` remains available.

Unknown future operational commands default to the same pre-submission block.
Legacy read aliases are explicitly classified; POST is not assumed to mean a
write. Raw `gql mutate` and mutation operations through `gql exec` are blocked
without an operation-specific verifier. `--yes` does not waive verification.
Authentication/local maintenance commands are outside the accounting-mutation
gate; they do not mutate ledger records.

## Implemented independent checks

| Adapter | Evidence required before success |
|---|---|
| Shared V3 create/update/deactivate/copy | Fresh entity GET, matching resource/ID, submitted field values, line identities/counts, preserved unmodified fields |
| V3 delete | Valid resource-scoped query proving absence; HTML/404, null or another resource is not proof |
| V3 void | Independent void marker or changed version plus zero total/nonzero-line removal; relevant unaffected fields preserved |
| Purchase recategorise | Fresh Purchase GET with requested category/Class/payee and preserved funding account, other lines and amounts |
| V3 batch | All-item preflight, unique bIds, correlated receipts, per-mutation entity readback; partial results retained on failure |
| Invoice write-off | Credit memo GET, apply-payment GET with both links, invoice balance/invariants, consumed credit memo balance |
| Feed ADD/categorise/split | Accepted-state read, distinct linked accounting IDs, funding account/direction, category/Class/individual split amounts/memo, pending-state absence |
| Feed exclude/undo/unpost | Destination presence, source absence, feed invariants, linked record preservation/deletion |
| Match | Independent accepted feed/link and register readback |
| Bank rules | Before/after snapshots, target rule fields or deletion, unrelated rule preservation |
| Attachment note/link | Fresh Attachable GET proves Note and EntityRef and preserves other metadata |
| File upload | Independent download bytes equal submitted file, with matching name |

Comparisons ignore transport fields, versions/timestamps and explicitly
server-derived fields where appropriate. They do not accept a returned ID as
proof that a requested category or amount changed. Missing data or unsupported
shapes fail closed rather than infer success.

Some formerly wired operations are now blocked, including service mutations
whose independently observed state is not yet mapped, arbitrary GraphQL writes,
feed transfers without a two-sided verifier, and email delivery claims. Their
request builders and dry-run plans remain available. This is an intentional
safety restriction, not a claim that all services now have readback adapters.

## Failure and recovery

- `not_submitted`: the handler was blocked before it ran; no write was submitted.
- `unverified`: a write was attempted but completion is not proved. Partial
  application is possible. JSON includes `verified:false`,
  `readback_required:true` and available receipt information.
  Once a handler has run, an error conservatively uses this outcome even if
  the failure may have occurred during preflight; missing instrumentation must
  never be presented as proof of no write.
- Confirmed success includes `verified:true`, issued by the adapter's in-process
  proof—not copied from a server's response property.

Never blindly retry an unverified write. Perform GET-only recovery against the
exact entity/feed IDs and evidence directory. V3 mutation and batch evidence is
private under the profile's mutation-evidence directory. A timeout, a readback
error or an unsuccessful batch item does not prove that nothing changed.
Feed posting also preserves its request and response privately. Feed reads
expose `accountingLinks` with the linked entity type/ID and sequence, so recovery
can inspect the actual accounting record rather than guess from dates/amounts.

The gate applies to the public CLI. Internal low-level service functions without
adapters remain available for implementation/testing but are not independently
verified public write capabilities. Offline mocks test success, unchanged state,
wrong identities, unrelated field changes, partial batches and failed reads;
they are not a claim of live certification for every QBO service.
