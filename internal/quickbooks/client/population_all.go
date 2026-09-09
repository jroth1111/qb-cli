package client

import (
	"context"
)

// AccountPopulation is one account's completeness manifest entry.
type AccountPopulation struct {
	AccountID    string        `json:"accountId"`
	Expected     int           `json:"expected"`
	Fetched      int           `json:"fetched"`
	Complete     bool          `json:"complete"`
	Transactions []Transaction `json:"transactions"`
}

// PopulationAllResult aggregates the per-account manifests.
type PopulationAllResult struct {
	Status      int                 `json:"status"`
	Accounts    []AccountPopulation `json:"accounts"`
	AllComplete bool                `json:"allComplete"`
}

// ReplayPopulationAll walks the pending population for every connected
// account. Zero-count accounts are skipped without a walk (recorded as
// complete/empty). A walk failure or mismatch marks that account
// incomplete but never aborts the remaining accounts.
func ReplayPopulationAll(ctx context.Context, maxPagesPerAccount int) (*PopulationAllResult, error) {
	if maxPagesPerAccount < 1 {
		maxPagesPerAccount = 40
	}
	accts, err := ReplayAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := &PopulationAllResult{Status: 200, AllComplete: true}
	for _, a := range accts.Accounts {
		entry := AccountPopulation{AccountID: a.ID}
		ac, err := newAPIClient()
		if err != nil {
			return nil, err
		}
		expected := ac.expectedPending(ctx, a.ID)
		entry.Expected = expected

		if expected == 0 {
			entry.Complete = true
			out.Accounts = append(out.Accounts, entry)
			continue
		}

		const pageSize = 300
		seen := make(map[string]struct{})
		txs := make([]Transaction, 0)
		for page := 0; page < maxPagesPerAccount; page++ {
			start := page * pageSize
			p, err := fetchFeedPage(ctx, ac, a.ID, "PENDING", start, pageSize)
			if err != nil {
				break
			}
			for _, t := range p.Items {
				if _, dup := seen[t.ID]; !dup {
					seen[t.ID] = struct{}{}
					txs = append(txs, projectTxn(t))
				}
			}
			if len(p.Items) < pageSize {
				break
			}
		}
		entry.Fetched = len(seen)
		entry.Transactions = txs
		entry.Complete = expected > 0 && entry.Fetched == expected
		if !entry.Complete {
			out.AllComplete = false
		}
		out.Accounts = append(out.Accounts, entry)
	}
	return out, nil
}
