package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrLookupDrift identifies an otherwise well-formed feed walk whose counts or
// IDs changed between sequential review-state pages. One fresh retry may
// recover a transient move; a second drift remains fail-closed.
var ErrLookupDrift = errors.New("feed changed during lookup")

// LookupCoverage separates search coverage from the output limit. A successful
// walk is still a sequential observation, not an atomic database snapshot.
type LookupCoverage struct {
	Complete     bool           `json:"complete"`
	TotalMatches int            `json:"totalMatches"`
	Truncated    bool           `json:"truncated"`
	RowsByState  map[string]int `json:"rowsByState"`
	PagesByState map[string]int `json:"pagesByState"`
}

func replayLookupComplete(ctx context.Context, accountID, query string, limit int) (*PendingResult, error) {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	coverage := &LookupCoverage{RowsByState: map[string]int{}, PagesByState: map[string]int{}}
	matches := make([]Transaction, 0)
	seen := make(map[string]struct{})
	const pageSize = 300
	for _, state := range []string{"PENDING", "ACCEPTED", "EXCLUDED"} {
		coverage.RowsByState[state] = 0
		terminal := false
		for page := 0; ; page++ {
			batch, err := fetchFeedPage(ctx, ac, accountID, state, page*pageSize, pageSize)
			if err != nil {
				return nil, err
			}
			coverage.PagesByState[state]++
			if batch.Items == nil {
				return nil, fmt.Errorf("%w: missing items array for %s", ErrIncomplete, state)
			}
			if len(batch.Items) == 0 {
				if batch.TotalTransactionsCount == nil || *batch.TotalTransactionsCount != coverage.RowsByState[state] {
					return nil, fmt.Errorf("%w: %w: terminal total does not verify %d collected %s rows", ErrIncomplete, ErrLookupDrift, coverage.RowsByState[state], state)
				}
				terminal = true
				break
			}
			for _, raw := range batch.Items {
				if raw.ID == "" || rawID(raw.QBOAccountID) != accountID {
					return nil, fmt.Errorf("%w: invalid identity or account in %s", ErrIncomplete, state)
				}
				if _, exists := seen[raw.ID]; exists {
					return nil, fmt.Errorf("%w: %w: repeated feed ID %s across lookup pages or states", ErrIncomplete, ErrLookupDrift, raw.ID)
				}
				seen[raw.ID] = struct{}{}
				coverage.RowsByState[state]++
				txn := projectTxn(raw)
				txn.ReviewState = state
				if raw.Description != txn.Description {
					txn.DisplayDescription = raw.Description
				}
				if txnMatches(txn, query) {
					coverage.TotalMatches++
					if limit < 1 || len(matches) < limit {
						matches = append(matches, txn)
					}
				}
			}
		}
		if !terminal {
			return nil, fmt.Errorf("%w: %s lookup reached page ceiling", ErrIncomplete, state)
		}
	}
	coverage.Complete = true
	coverage.Truncated = len(matches) < coverage.TotalMatches
	return &PendingResult{
		Status: http.StatusOK, Counts: map[string]int{"transactions": len(matches)},
		Transactions: matches, AccountID: accountID, Lookup: coverage,
	}, nil
}
