package client

import (
	"context"
	"math"
	"strconv"
)

// splitDetailList projects the split lines onto the observed
// addAsQboTxn.details entries: each line carries its categoryId, the amount
// as a string, an empty description and the taxApplicableOn marker.
func splitDetailList(lines []SplitLine) []any {
	details := make([]any, 0, len(lines))
	for _, l := range lines {
		details = append(details, map[string]any{
			"categoryId":      l.CategoryRef.Value,
			"billable":        false,
			"amount":          strconv.FormatFloat(math.Abs(l.Amount), 'f', -1, 64),
			"description":     "",
			"taxApplicableOn": "SALES",
		})
	}
	return details
}

// ReplaySplit POSTs
// /api/neo/v1/company/{realm}/olb/ng/batchAcceptTransactions?acceptOnly=true.
// It replaces each row's addAsQboTxn.details with the split lines and posts
// it — the same call the SPA's split-and-post flow sends. Empty olbTxnIds or
// fewer than two lines are rejected before any network call. Callers must
// only pass olbTxnIds (or the :ofx display id for the same row). entityType
// is vestigial: the feed row carries the created txn type.
func ReplaySplit(ctx context.Context, accountID string, olbTxnIDs []string, entityType string, lines []SplitLine) (int, error) {
	plan, err := PlanSplit(accountID, olbTxnIDs, entityType, lines)
	if err != nil {
		return 0, err
	}
	return replayAccept(ctx, accountID, olbTxnIDs, plan, func(row map[string]any) error {
		effective := ""
		if len(lines) > 0 {
			effective = lines[0].CategoryRef.Value
		}
		acceptRowForPost(row, effective, false)
		add := addAsQboTxnMap(row)
		add["details"] = splitDetailList(lines)
		add["txnMemo"] = mapStr(row, "description")
		return nil
	})
}
