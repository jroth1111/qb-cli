# QBO Bank Feed Manipulations — Complete Reference

Everything known about manipulating bank-feed transactions, consolidated from
live captures (2026-08-23/24, test company 2), deobfuscated SPA source, and the
qb-cli implementation. Answer: **yes, this now exists as a .md — this file — and
the machine-readable counterpart is the Go source in `internal/quickbooks/client/`
plus `ENDPOINT_GUIDE.md` for reads.**

## Endpoints (all under `https://qbo.intuit.com/api/neo/v1/company/{realm}/olb/ng/`)

| Operation | Path | Purpose | CLI |
|---|---|---|---|
| List | `getTransactions` | Paged feed rows by reviewState | `feed txn list/population` |
| Accounts | `getInitialData` (ats/v1) | Accounts + numTxnToReview oracle | `feed account list` |
| Exclude | POST `excludeTransactions` | Remove row(s) from review queue | `feed txn update exclude --ids` |
| Undo exclude | POST `undoTransactions` | Restore excluded rows to pending | `feed txn update undo-excluded --ids` |
| Accept (batch) | POST `batchAcceptTransactions` | Post pending rows into the books | `feed txn update batch-accept --ids` |
| Categorise | POST `categoriseTransactions` | Assign one category + payee | `feed txn update categorise` |
| Match | POST `matchTransactions` | Link rows to existing QBO txns | `feed txn update match` |
| Split | POST `splitTransactions` | Split one row across ≥2 accounts | `feed txn update split --lines` |

Reads also exist on `/api/neo/v1/company/{realm}/olb/ng/getTransactions` (the
reconcile-flow client) but it hardcodes `fromDate=01/01/2000` MM/DD/YYYY and is
not the enumeration path — use `/ats/v1/.../banking/getTransactions`.

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

`numTxnToReview` from getInitialData is server-authoritative. Live proof:
exclude dropped it 3→2 in lock-step with the walk; undo restored 3/3 complete.

## Date filtering

ATS getTransactions ignores all date parameters server-side (falsified across
three disjoint windows). The olb/ng reconcile client hardcodes
`fromDate=01/01/2000` MM/DD/YYYY. Slice client-side by `olbTxnDate`.

## GraphQL alternatives (session-bound)

Reconciliation read/write family exists on api/v4/graphql
(GetReconciliation*, CancelReconcile, ClrTxnLine with
UpdateIntegration_ReconciliationInput!) and `BankingDisconnectOlbAccounts`.
These require relay-tab execution (browser-context bound); offline replay 401s.
