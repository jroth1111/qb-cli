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
	// ErrMissingAcceptCategory reports a row that the live service silently
	// ignores when an ADD has no addAsQboTxn.details[].categoryId.
	ErrMissingAcceptCategory = errors.New("batch accept requires a category on the live feed row")

	// ErrUnsupportedClass reports a --class value. The captured
	// batchAcceptTransactions detail carries categoryId only; the class
	// field name is not in the observed contract, so we refuse rather
	// than guess a wire key.
	ErrUnsupportedClass = errors.New("categorise --class is not in the captured feed contract; omit --class")

	// ErrFeedRowsNotFound reports a requested olbTxnId that is absent from
	// the account's pending feed, so no mutation is sent for it.
	ErrFeedRowsNotFound = errors.New("feed row not found in pending transactions")

	// ErrMatchTargetNotFound reports a --match-id absent from the feed
	// account's register, so its contract fields cannot be resolved.
	ErrMatchTargetNotFound = errors.New("match target not found in account register")

	// ErrMatchFieldMissing reports a register or feed row lacking a field
	// the acceptTransactions contract requires (we never invent values).
	ErrMatchFieldMissing = errors.New("match contract field missing from live read")
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

// TransactionDetail is the categorise input: CategoryRef carries the account
// id folded into addAsQboTxn.details[0].categoryId on the live
// batchAcceptTransactions contract. ClassRef is rejected (not in the
// captured contract); EntityType and Lines are vestigial from the retired
// categoriseTransactions request and are no longer sent.
type TransactionDetail struct {
	EntityType  string      `json:"entityType,omitempty"`
	CategoryRef *RefValue   `json:"categoryRef,omitempty"`
	ClassRef    *RefValue   `json:"classRef,omitempty"`
	Lines       []SplitLine `json:"lines,omitempty"`
}

// MatchTxn identifies one existing QuickBooks record to match a feed row
// against. TxnID is the record's qbo txn id; the register read supplies the
// remaining contract fields (txnTypeId, sequence, editSequence, amount).
// TxnType is advisory only — the register's recorded type wins.
type MatchTxn struct {
	TxnID   string `json:"txnId"`
	TxnType string `json:"txnType"`
}

// ReplayCategorise POSTs
// /api/neo/v1/company/{realm}/olb/ng/batchAcceptTransactions?acceptOnly=true.
// It folds the chosen category into each row's addAsQboTxn.details[0]
// .categoryId and posts it — the same call the SPA's Post button sends.
// Empty olbTxnIds, an empty category, or a --class value are rejected before
// any network call. Callers must only pass olbTxnIds (or the :ofx display
// id for the same row).
func ReplayCategorise(ctx context.Context, accountID string, olbTxnIDs []string, detail TransactionDetail) (int, error) {
	plan, err := PlanCategorise(accountID, olbTxnIDs, detail)
	if err != nil {
		return 0, err
	}
	cat := ""
	if detail.CategoryRef != nil {
		cat = detail.CategoryRef.Value
	}
	return replayAccept(ctx, accountID, olbTxnIDs, plan, func(row map[string]any) error {
		acceptRowForPost(row, cat, true)
		add := addAsQboTxnMap(row)
		details := addDetailsList(add)
		if len(details) == 0 {
			details = []map[string]any{{"billable": false}}
			add["details"] = details
		}
		details[0]["categoryId"] = cat
		setIfAbsent(details[0], "billable", false)
		setIfAbsent(details[0], "taxApplicableOn", "SALES")
		add["txnDate"] = mapStr(row, "olbTxnDate")
		return nil
	})
}
