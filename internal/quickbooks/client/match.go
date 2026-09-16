package client

import (
	"context"
)

// ReplayMatch POSTs /api/neo/v1/company/{realm}/olb/ng/acceptTransactions.
// It matches pending feed rows to existing QuickBooks records: each feed row
// collapses to the observed entry shape (acceptType MATCH, id, olbTxnDate,
// qboAccountId) and each --match-id is resolved against the feed account's
// register so txnTypeId, qboTxnSeqId, txnSyncToken and paymentAmount all
// come from a live read. Empty olbTxnIds or an empty match list are
// rejected before any network call; a match id absent from the register
// fails with ErrMatchTargetNotFound before the POST is sent.
func ReplayMatch(ctx context.Context, accountID string, olbTxnIDs []string, matchTxns []MatchTxn) (int, error) {
	plan, err := PlanMatch(accountID, olbTxnIDs, matchTxns)
	if err != nil {
		return 0, err
	}
	acct := accountID
	if acct == "" {
		acct = DefaultAccountID
	}
	ac, err := newAPIClient()
	if err != nil {
		return 0, err
	}
	rows, err := feedRowsForIDs(ctx, ac, acct, normalizeExcludeIDs(olbTxnIDs))
	if err != nil {
		return 0, err
	}
	regRows, err := registerRowsForAccount(ctx, ac, acct)
	if err != nil {
		return 0, err
	}
	body, err := buildMatchBody(acct, rows, regRows, cleanMatchTxns(matchTxns))
	if err != nil {
		return 0, err
	}
	return postResolved(ctx, ac, plan, body)
}
