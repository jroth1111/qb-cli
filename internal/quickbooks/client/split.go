package client

import "context"

// splitRequest is the body for POST splitTransactions. It is the same shape
// as categoriseRequest but transactionDetail carries a Lines array (≥2)
// instead of a single CategoryRef.
type splitRequest struct {
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
	TransactionDetail struct {
		EntityType string      `json:"entityType,omitempty"`
		Lines      []SplitLine `json:"lines"`
	} `json:"transactionDetail"`
}

// ReplaySplit POSTs /api/neo/v1/company/{realm}/olb/ng/splitTransactions.
// It splits pending feed rows across multiple QBO accounts (lines). Empty
// olbTxnIds or fewer than two lines are rejected before any network call.
// Callers must only pass olbTxnIds (not :ofx display ids).
func ReplaySplit(ctx context.Context, accountID string, olbTxnIDs []string, entityType string, lines []SplitLine) (int, error) {
	plan, err := PlanSplit(accountID, olbTxnIDs, entityType, lines)
	if err != nil {
		return 0, err
	}
	return postPlanned(ctx, plan)
}
