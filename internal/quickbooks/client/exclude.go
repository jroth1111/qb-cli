package client

import (
	"context"
	"errors"
	"strings"
)

// ErrEmptyExcludeIDs is returned when ReplayExclude is called with no
// olbTxnIds. The OpenAPI excludeTransactions body requires olbTxnIds.
// An empty list is a no-op on the server; we reject it so callers cannot
// think they excluded something.
var ErrEmptyExcludeIDs = errors.New("exclude requires at least one olbTxnId")

type excludeRequest struct {
	NextTxnInfo struct {
		AccountID            string `json:"accountId"`
		NextTransactionIndex int    `json:"nextTransactionIndex"`
		ReviewState          string `json:"reviewState"`
		Sort                 string `json:"sort"`
	} `json:"nextTxnInfo"`
	TxnIDList struct {
		ExternalTxnIDs []string `json:"externalTxnIds"`
		OlbTxnIDs      []string `json:"olbTxnIds"`
		TxnIDPairs     []any    `json:"txnIdPairs"`
	} `json:"txnIdList"`
}

// ReplayExclude POSTs /api/neo/v1/company/{realm}/olb/ng/excludeTransactions
// as specified in olb-bank-feed.yaml. Empirically: empty olbTxnIds returned
// 200 and did not change PENDING/EXCLUDED populations. This function still
// refuses an empty id list. Callers must only pass olbTxnIds (not :ofx
// display ids) and must undo any live exclude in the same session.
func ReplayExclude(ctx context.Context, accountID string, olbTxnIDs []string) error {
	plan, err := PlanExclude(accountID, olbTxnIDs)
	if err != nil {
		return err
	}
	_, err = postPlanned(ctx, plan)
	return err
}

func normalizeExcludeIDs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, id := range in {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}
