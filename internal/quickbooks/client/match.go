package client

import "context"

// matchRequest is the body for POST matchTransactions.
type matchRequest struct {
	NextTxnInfo struct {
		AccountID            string `json:"accountId"`
		NextTransactionIndex int    `json:"nextTransactionIndex"`
		ReviewState          string `json:"reviewState"`
		Sort                 string `json:"sort"`
	} `json:"nextTxnInfo"`
	TxnIDList struct {
		ExternalTxnIDs []string `json:"externalTxnIds"`
		OlbTxnIDs      []string `json:"olbTxnIds"`
	} `json:"txnIdList"`
	MatchTxns []MatchTxn `json:"matchTxns"`
}

// ReplayMatch POSTs /api/neo/v1/company/{realm}/olb/ng/matchTransactions.
// It matches pending feed rows to existing QuickBooks records. Empty
// olbTxnIds or an empty matchTxns list are rejected before any network call.
// Callers must only pass olbTxnIds (not :ofx display ids).
func ReplayMatch(ctx context.Context, accountID string, olbTxnIDs []string, matchTxns []MatchTxn) (int, error) {
	plan, err := PlanMatch(accountID, olbTxnIDs, matchTxns)
	if err != nil {
		return 0, err
	}
	return postPlanned(ctx, plan)
}
