package client

import (
	"context"
	"errors"
	"strconv"
)

// ErrEmptyTransferIDs is returned when a transfer accept is called with no
// olbTxnIds; ErrEmptyTransferAccount when the destination account is empty.
var (
	ErrEmptyTransferIDs     = errors.New("transfer requires at least one olbTxnId")
	ErrEmptyTransferAccount = errors.New("transfer requires --to-account-id (destination bank/credit card account)")
)

// transferRowForPost applies the observed edits the SPA applies to a live
// feed row when the user picks Transaction type = Transfer and posts:
// acceptType TRANSFER, transfer:true, addAsQboTxn.txnTypeId "26" (transfer),
// txnDate from the feed row, and a single detail whose categoryId is the
// destination bank/credit-card account (verified live TC2 2026-09-19:
// source account 44, destination 45, row olbTxnId 9). Unknown row fields
// are preserved.
func transferRowForPost(row map[string]any, toAccountID string) {
	row["acceptType"] = "TRANSFER"
	row["transfer"] = true
	if n, ok := mapNum(row, "openBalance"); ok {
		row["openBalance"] = strconv.FormatFloat(n, 'f', -1, 64)
	}
	setIfAbsent(row, "attachmentIds", []any{})
	setIfAbsent(row, "docs", []any{})
	setIfAbsent(row, "recommendationId", "")
	add := addAsQboTxnMap(row)
	add["txnTypeId"] = "26"
	if d := mapStr(row, "olbTxnDate"); d != "" {
		add["txnDate"] = d
	}
	add["details"] = []any{map[string]any{
		"categoryId":      toAccountID,
		"billable":        false,
		"taxApplicableOn": "SALES",
	}}
	setIfAbsent(add, "attachments", []any{})
	row["trackingInfo"] = map[string]any{
		"initialCategoryId":      toAccountID,
		"weChangedCatForUser":    false,
		"userChangedCat":         true,
		"userOverrodeChangedCat": false,
		"userChangedName":        false,
		"isCatChanged":           toAccountID != mapStr(row, "originalCategoryId"),
	}
}

// ReplayTransfer POSTs
// /api/neo/v1/company/{realm}/olb/ng/batchAcceptTransactions?acceptOnly=true
// with each pending feed row shaped as a transfer to toAccountID — the same
// shape the SPA's Transaction type=Transfer + Post sends. Callers must only
// pass olbTxnIds (or the :ofx display id for the same row); --to-account-id
// is the destination bank/credit-card account id.
func ReplayTransfer(ctx context.Context, accountID string, olbTxnIDs []string, toAccountID string) (int, error) {
	plan, err := PlanTransfer(accountID, olbTxnIDs, toAccountID)
	if err != nil {
		return 0, err
	}
	return replayAccept(ctx, accountID, olbTxnIDs, plan, func(row map[string]any) error {
		transferRowForPost(row, toAccountID)
		return nil
	})
}
