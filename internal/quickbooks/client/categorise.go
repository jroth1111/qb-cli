package client

import (
	"context"
	"errors"
)

// Sentinel errors mirror ErrEmptyExcludeIDs/ErrEmptyUndoIDs so callers get
// the same fail-fast contract before any network activity.
var (
	ErrEmptyCategoriseIDs  = errors.New("categorise requires at least one olbTxnId")
	ErrEmptyCategory       = errors.New("categorise requires a --category account id")
	ErrEmptyMatchIDs       = errors.New("match requires at least one olbTxnId")
	ErrEmptyMatchTxns      = errors.New("match requires at least one match transaction id")
	ErrEmptySplitIDs       = errors.New("split requires at least one olbTxnId")
	ErrEmptySplitLines     = errors.New("split requires at least two category lines")
	ErrEmptyBatchAcceptIDs = errors.New("batch accept requires at least one olbTxnId")
)

// RefValue is the QBO reference shape ({value}) used by categoryRef/classRef.
type RefValue struct {
	Value string `json:"value"`
}

// SplitLine is one categorised line of a split row.
type SplitLine struct {
	CategoryRef RefValue `json:"categoryRef"`
	Amount      float64  `json:"amount"`
}

// TransactionDetail is the transactionDetail payload shared by categorise
// and split. For categorise, CategoryRef is set and Lines is empty; for
// split, Lines is set (≥2) and CategoryRef is nil.
type TransactionDetail struct {
	EntityType  string      `json:"entityType,omitempty"`
	CategoryRef *RefValue   `json:"categoryRef,omitempty"`
	ClassRef    *RefValue   `json:"classRef,omitempty"`
	Lines       []SplitLine `json:"lines,omitempty"`
}

// MatchTxn pairs an existing QuickBooks record id with its type for matching.
type MatchTxn struct {
	TxnID   string `json:"txnId"`
	TxnType string `json:"txnType"`
}

// categoriseRequest is the body for POST categoriseTransactions.
type categoriseRequest struct {
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
	TransactionDetail TransactionDetail `json:"transactionDetail"`
}

// ReplayCategorise POSTs /api/neo/v1/company/{realm}/olb/ng/categoriseTransactions.
// It categorises pending feed rows into a QBO account (category) and posts them.
// Empty olbTxnIds or an empty category are rejected before any network call.
// Callers must only pass olbTxnIds (not :ofx display ids).
func ReplayCategorise(ctx context.Context, accountID string, olbTxnIDs []string, detail TransactionDetail) (int, error) {
	plan, err := PlanCategorise(accountID, olbTxnIDs, detail)
	if err != nil {
		return 0, err
	}
	return postPlanned(ctx, plan)
}
