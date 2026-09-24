# QBO Bank Feed Manipulations — Complete Reference

Everything known about manipulating bank-feed transactions, consolidated from
live captures (2026-08-23/24, test company 2), read-only production enumeration
(2026-09-21), deobfuscated SPA source, and the
qb-cli implementation. Answer: **yes, this now exists as a .md — this file — and
the machine-readable counterpart is the Go source in `internal/quickbooks/client/`
plus `ENDPOINT_GUIDE.md` for reads.**

## Endpoints (all under `https://qbo.intuit.com/api/neo/v1/company/{realm}/olb/ng/`)

| Operation | Path | Purpose | CLI |
|---|---|---|---|
| List | `getTransactions` | Paged feed rows by reviewState | `feed txn list/population` |
| Accounts | `getInitialData` | Accounts + pending-only numTxnToReview oracle | `feed account list` |
| Exclude | POST `excludeTransactions` | Remove row(s) from review queue | `feed txn update exclude --ids` |
| Undo exclude | POST `undoTransactions` | Restore excluded rows to pending | `feed txn update undo-excluded --ids` |
| Accept (batch) | POST `batchAcceptTransactions` | Post pending rows into the books | `feed txn update batch-accept --ids` |
| Categorise | POST `batchAcceptTransactions?acceptOnly=true` | Categorise and post the pending row | `feed txn update categorise` |
| Match | POST `acceptTransactions` | Link rows to existing QBO txns | `feed txn update match` |
| Split | POST `batchAcceptTransactions?acceptOnly=true` | Split and post a pending row across ≥2 accounts | `feed txn update split --lines` |

The current bank-transactions UI enumerates through the first-party `olb/ng`
paths above. A production read through OMP Browser Relay verified both
`PENDING` and `ACCEPTED` walks on this route. The earlier recommendation to
use only `/ats/v1/.../banking/getTransactions` is superseded for these reads;
this does not revalidate the historical mutation contracts below.

## Shared request envelope

Every mutation POST carries the same JSON skeleton plus an operation-specific
`transactionDetail`:

```json
{
  "nextTxnInfo": {
    "accountId": "<qboAccountId>",
    "nextTransactionIndex": 0,
    "reviewState": "PENDING",
    "sort": "-txnDate"
  },
  "txnIdList": {
    "externalTxnIds": [],
    "olbTxnIds": ["3", "2"]
  },
  "transactionDetail": { ...operation-specific... }
}
```

Rules learned live:
- IDs are **olbTxnIds** (`3`, `2:ofx` display form is rejected).
- Empty id lists are rejected client-side before network.
- Auth = full six-header set; cookies alone → 401 AuthenticationFailed.

## Per-operation detail

### exclude / undo-excluded
No `transactionDetail`. Pure id-list operation. Verified round-trip on account 44:
exclude olbTxnId `3` → numTxnToReview dropped 3→2; undo restored it to 3 with ids
[3,2,1] intact. Zero residue after undo.

### batch-accept
Same envelope; no transactionDetail required beyond ids. Posts rows into the books
(acceptType ADD). Irreversible via the feed — verify before running.

### categorise
```json
"transactionDetail": {
  "CategoryRef": { "value": "<categoryId>" },
  "NameId": "<payeeId>",          // optional
  ...
}
```
CLI: `feed txn update categorise --account-id X --ids 3 --category 7 [--class C]`.

### match
Adds `matchTxns[]`: each pairs a feed row with an existing QBO record.
CLI: `--txn-type` selects record type (default Bill).

### split
`transactionDetail.lines[]` (≥2) of `{categoryId, amount}`; amounts must sum to
the row total. CLI parses `--lines "7=-6,8=-4"`.

## Completeness oracle

`numTxnToReview` from `getInitialData` counts **PENDING only**. The earlier
exclude/undo proof changed that count with the review queue. It is not an
oracle for `ACCEPTED` or the union of states.

For each state, send `startIndex`, `chunkSize=300`, and
`X-Range: items=<start>-<end>` together. Continue to a terminal page; retain
raw responses and deduplicate by feed ID within that state. The observed
`totalTransactionsCount` grew with the requested offset before settling at
the terminal population, so neither its first-page value nor the UI table
count proves completeness. Independent ascending and descending `txnDate`
walks produced equal ID sets; pending suggestions changed between reads,
while transaction facts remained stable.

**Implementation limitation:** `ReplayFeedComplete` currently obtains
`expectedPending` regardless of the supplied review state and stops when that
count is reached. Do not use it to certify an accepted population. For
example, a pending count equal to one full page could cause an accepted walk
to report complete while later accepted pages exist. The helper needs a
state-specific completion strategy and regression coverage; this reference
correction does not change that implementation.

For a pending-plus-posted comparison, enumerate both states independently,
check cross-state ID overlap, and preserve state on every row. `PENDING`
category/class suggestions are not posted classification. `ACCEPTED` rows
reference their accounting records in `matchedQboTxns`; read those records by
entity and ID to inspect actual line classes. In the observed feed response,
posted class assignments were absent. Use `Accept: application/json` for v3
entity reads, which can otherwise return XML. Keep authentication headers in
memory and out of response artifacts.

Feed amounts use cashflow sign: card charges are negative and credits are
positive. A statement dataset that treats debt increases as positive needs
one explicit sign inversion. Included foreign-fee annotations are not extra
bank-feed postings.

## Date filtering

Earlier ATS probes found date parameters ignored across three disjoint
windows. Date-filter behaviour was not re-tested on the current first-party
route. Enumerate the population before slicing client-side by `olbTxnDate`;
do not treat a requested date window as evidence of returned coverage.

## GraphQL alternatives (session-bound)

Reconciliation read/write family exists on api/v4/graphql
(GetReconciliation*, CancelReconcile, ClrTxnLine with
UpdateIntegration_ReconciliationInput!) and `BankingDisconnectOlbAccounts`.
These require relay-tab execution (browser-context bound); offline replay 401s.

## Verified matching and current UI observations

`feed txn update match` now preserves a private before-state and request/response
receipt under the active QB home, requires per-row domain success, reads the
ACCEPTED feed independently, verifies the requested target IDs, and checks that
register amounts, dates, accounts and attribution are unchanged. An unverified
outcome is an error, not permission to repeat the POST.

Explicit matching uses `advancedMatchDetails?initial=true&id=<feed-display-id>`
with the UI date window (90 days before, 30 days after). The returned
`homeCurrencyMatchingTxns` supplies `txnSyncToken`, `qboTxnSeqId`, `txnTypeId` and
the match amount. Register `editSequence` is not a substitute: the observed
register version and match-specific version differed, and using the register
value produced `QBO-13104` under HTTP 200. Targets missing from the eligible
candidate response are rejected before mutation.

The current UI has two distinct match requests:

- Find other matches: `POST acceptTransactions`, reduced `olbTxns` entries with
  `selectedMatches.matchedTxns`, and explicit null adjustment/new-transaction
  fields. This is the CLI explicit-match operation.
- Suggested quick match: `POST batchAcceptTransactions?acceptOnly=true`, full
  feed row, `acceptType: MATCH`, and `selectedMatches`. This is not a request
  to create a new transfer.

In the observed register UI, status 2 is blank/uncleared and status 1 is C/cleared.
A successful match normally changes 2 to 1. The verifier allows that transition;
all other status changes remain verification failures. It does not infer that
a numeric value means reconciled without UI evidence.

A shared/common class is genuinely blank, not a class named Common. The
expense form clears it through `UpdateTransactions_Transaction_qbo` with
`lines.accountLines[i].class.id: null`. The form also normalized memo spacing
in the observed saves. Compare the entire saved record and restore unintended
normalization using the current version, rather than claiming a class-only
change without checking. These GraphQL observations do not establish null
serialization for every other API family.

Use the observed native banking Accept header. The v3 JSON helper deliberately
forces `Accept: application/json`; it is not the generic banking POST helper.
`searchFilter` is an observed feed search parameter, but a search result is
not itself an enumeration-completeness proof.

## Verified Undo and Exclude

The UI's Posted Undo uses `undoTransactions` with `reviewState: ACCEPTED`.
For an auto-created expense, the response names the deleted accounting record
under `olbtxns.success[].deletedQboTxns`. The CLI now preserves the linked record
before writing, verifies that a reported deletion actually occurred, verifies
retained linked records, and checks the feed row moved back to PENDING.

Exclude uses `excludeTransactions` with `reviewState: PENDING`. The observed
success response was `{}`; success must be established from the row appearing
in EXCLUDED and disappearing from PENDING, with its transaction facts intact.
Undo-excluded similarly verifies the reverse state transition. Missing or
malformed state responses are not proof of absence. Private receipts are stored
under the active QB home's `mutation-evidence` directory without request headers.

### Preserve an expense while removing its feed link

The expense editor's **online banking match → Unmatch** is different from
Posted **Undo**. The observed editor request uses `/api/v4/graphql`, operation
`UpdateTransactions_Transaction_qbo`. Its transaction input retains the accounting
transaction ID and sets the selected `stageEntity` entry to
`state: TO_BE_REVIEWED`. The expense remains while the feed row returns to Pending.
Do not substitute `feed txn update unpost` when the expense must survive.

Use the editor for this operation; there is no dedicated preserving-unmatch CLI
command. Do not replay another record's full form input: the observed request
also carries accounting fields and can rewrite them. In particular, the editor
collapsed internal whitespace in `header.privateMemo`. Verify the entity, amount,
date, lines, account, class, memo and feed state after unmatching and after any
subsequent match. A successful GraphQL response is not sufficient verification.

`expenses expense update edit --id ID --memo TEXT` exposes the existing sparse
expense memo update. It accepts a non-empty memo and preserves existing lines;
omitting the flag preserves the memo. Verify the retained bank-feed match and
accounting fields after an update. Match requests from the current banking UI
can use `batchAcceptTransactions` with `acceptType: MATCH` and an explicit
`selectedMatches` record; verify that no new expense was created.

## Relay capture reliability and cookie scope

The capture loop must continue paused requests while `Page.navigate` is still
pending: the navigation document itself matches the interception pattern.
Waiting for navigation before draining paused requests deadlocks. Interception
is disabled on success and failure so a capture cannot leave the tab stalled.
Cookie capture is scoped to the QBO origin, including the relay refresh path
and the Ego capture request; unrelated browser cookies are not requested or
persisted by these paths. The relay regression test models a document that
cannot finish navigating until its paused request is continued.

For credit-card balances, preserve the source field's sign convention. In the
observed company, v3 `Account.CurrentBalance` was the opposite sign of the
banking surface's `qboBalance` and UI posted balance. Validate deltas within
one convention or convert explicitly; do not equate the raw fields. This does
not change the CLI's existing native-field projections.
