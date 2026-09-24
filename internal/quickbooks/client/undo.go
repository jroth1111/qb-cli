package client

import (
	"context"
	"errors"
)

// ErrEmptyUndoIDs is returned when ReplayUndo is called with no olbTxnIds.
var ErrEmptyUndoIDs = errors.New("undo requires at least one olbTxnId")

type undoRequest struct {
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

// ReplayUndo POSTs /api/neo/v1/company/{realm}/olb/ng/undoTransactions
// (olb-bank-feed.yaml). Moves EXCLUDED rows back to PENDING. nextTxnInfo
// reviewState is EXCLUDED. Empty id list is rejected. Do not call this on
// live rows unless the same turn will restore the prior state.
func ReplayUndo(ctx context.Context, accountID string, olbTxnIDs []string) error {
	plan, err := PlanUndo(accountID, olbTxnIDs)
	if err != nil {
		return err
	}
	_, err = postPlanned(ctx, plan)
	return err
}

// ReplayUnpost POSTs the same undoTransactions endpoint with nextTxnInfo
// reviewState ACCEPTED — the live contract the Posted tab's Undo button
// sends (verified TC2 2026-09-19). Moves ACCEPTED rows back to PENDING and
// deletes the transaction the accept created. Empty id list is rejected.
func ReplayUnpost(ctx context.Context, accountID string, olbTxnIDs []string) error {
	plan, err := PlanUnpost(accountID, olbTxnIDs)
	if err != nil {
		return err
	}
	_, err = postPlanned(ctx, plan)
	return err
}
