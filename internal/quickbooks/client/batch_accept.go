package client

import "context"

// batchAcceptRequest is the body for POST batchAcceptTransactions.
type batchAcceptRequest struct {
	NextTxnInfo struct {
		AccountID            string `json:"accountId"`
		NextTransactionIndex int    `json:"nextTransactionIndex"`
		ReviewState          string `json:"reviewState"`
		Sort                 string `json:"sort"`
	} `json:"nextTxnInfo"`
	TxnIDList struct {
		ExternalTxnIDs []string `json:"externalTxnIds,omitempty"`
		OlbTxnIDs      []string `json:"olbTxnIds"`
	} `json:"txnIdList"`
}

// ReplayBatchAccept POSTs /api/neo/v1/company/{realm}/olb/ng/batchAcceptTransactions.
// It accepts (posts) pending feed rows in bulk without per-row review. Empty
// olbTxnIds are rejected before any network call. Callers must only pass
// olbTxnIds (not :ofx display ids).
func ReplayBatchAccept(ctx context.Context, accountID string, olbTxnIDs []string) (int, error) {
	plan, err := PlanBatchAccept(accountID, olbTxnIDs)
	if err != nil {
		return 0, err
	}
	return postPlanned(ctx, plan)
}
