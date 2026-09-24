# Today's transcript-backed primitive parity

Scope: QBO-related actions in Codex and Devin records dated 24 September 2026
in Australia/Melbourne. This is code/contract parity, not certification of the
current ledger or a replay of historical writes. Private raw locators and
coverage inventories are in `.sol/evidence/today-parity-20260924/`.

| Observed operation | Native surface / contract |
|---|---|
| Adopt current browser authentication | `login --source ego|relay|chrome`; fresh headers plus cookies and company/principal checks |
| Repair header-only credential aliases | `auth.Load` restores missing ATS typed fields in memory, case-insensitively; no external credential rewrite needed |
| Browser GraphQL requests | `gql exec`; `QB_EGO_SPACE_ID` takes precedence over `QB_EGO_SPACE`, then `qb-gql`; existing unlabelled tabs adopted by target ID |
| Posted register category/Class audit | `accounting register get --account-id ID --offset START --limit N --json`; preserves account/class names, identifiers, memo and row sequence |
| Pending row categorise / match / unpost | `feed txn update categorise`, `match`, `unpost`; use exact feed IDs, inspect receipts and readback before retry |
| Posted Purchase recategorise | `expenses expense update recategorise`; selected line and expected-version/category/Class guards |
| Typed entity reads and direct-v3 creation fallback | Existing v3 entity read/create and journal primitives; full mixed-entity/attachment queries use `advanced batch run` Query items; creating a record does not match its pending bank-feed row |
| Managed Purchase memo metadata | `expenses expense update edit --memo ...`; preserves omitted managed sections, rejects malformed sections and accidental legacy annotation loss |
| Typed metadata notes and attachment reads | Existing transaction attach-note and company attachable download primitives |
| Multi-target metadata rollout | `advanced batch run` verifies bId coverage and per-item faults, then caller performs target-specific readback |

Feed commands reject positional arguments: `--ids 1 2 3` is an error, not a
one-ID operation. Use `--ids 1,2,3` or repeated `--ids`. Today's apparent
51-to-one unpost limit was proved to be argument loss, not a QBO batch limit.

Register `account` / `account_id` represent captured `accountName` / `accountId`.
`line_account_id` is separately captured `lineAccountId` (the bank-side account
in today's sample); it must not be substituted for the booked category. Class
comes from `klass` / `klassId`. Missing fields remain absent, not inferred. A
bounded register list is not a complete census and makes no split-line claim.
Use `(id, sequence)` as the register row identity when paging; a transaction can
have multiple rows. Stop on an empty page and reject repeated/changing row sets
in a multi-page census rather than silently counting duplicates.

For full-fidelity readback, the native batch endpoint accepts read-only Query
items as well as mutations. A file for `advanced batch run --file reads.json`
may contain `[{"bId":"read-1","Query":"SELECT * FROM Attachable WHERE Id = '123' MAXRESULTS 1"}]`.
This returns the actual per-item QueryResponse, including Note/AttachableRef;
The CLI rejects missing/duplicate/wrong bIds, missing payloads and per-item
faults with a nonzero exit while retaining partial results on stdout. Callers
must still verify requested entity identities and actual postconditions. No
external cookie-bearing curl wrapper is needed. Use `STARTPOSITION` and
`MAXRESULTS` in queries requiring explicit pages.

Managed sections use `[[QBO-METADATA:key]]` and `[[/QBO-METADATA:key]]`.
Replacing a same-key section is explicit; other existing sections survive a
memo replacement. Duplicate, nested, unmatched and mismatched closing markers
fail before a request. Legacy bracketed annotations require retention or an
explicit managed replacement. The generic v3 batch endpoint accepts raw bodies;
it does not automatically apply the single-Purchase memo merge to those bodies.

Operational limits retained from today's transcripts:

- The interrupted 51-row unpost/repost workflow needs target-specific GET-only
  reconciliation before any resume. A primitive's existence does not establish
  the final state of a detached script.
- The direct-v3 fallback created records while feed rows remained pending.
  Verify the exact record/feed pairing before Match; do not create or Add again.
- The metadata rollout's scoped receipts are not proof of a complete global
  census. Pagination overlap/repetition invalidated that broader claim.
- UI navigation, external invoice research and accounting judgments are not
  replaced by an API primitive; the CLI executes explicit, evidenced decisions.
