package client

import "context"

// VerifyResult reports whether the locally fetched pending population
// matches the server-authoritative count for an account.
type VerifyResult struct {
	AccountID string         `json:"accountId"`
	Expected  int            `json:"expected"`
	Fetched   int            `json:"fetched"`
	Complete  bool           `json:"complete"`
	States    map[string]int `json:"states"`
}

// VerifyFeed walks the PENDING population and reports completeness without
// erroring on a mismatch — verification reports, it does not fail.
func VerifyFeed(ctx context.Context, accountID string) (*VerifyResult, error) {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	expected := ac.expectedPending(ctx, accountID)

	const pageSize = 300
	seen := make(map[string]struct{})
	for page := 0; page < 40; page++ {
		p, err := fetchFeedPage(ctx, ac, accountID, "PENDING", page*pageSize, pageSize)
		if err != nil {
			return nil, err
		}
		for _, t := range p.Items {
			seen[t.ID] = struct{}{}
		}
		if len(p.Items) < pageSize {
			break
		}
	}

	// expected==0 means the server reports an empty population — that is
	// a verified-complete state, not an unknown one.
	complete := (expected == 0 && len(seen) == 0) || (expected > 0 && len(seen) == expected)
	return &VerifyResult{
		AccountID: accountID,
		Expected:  expected,
		Fetched:   len(seen),
		Complete:  complete,
		States:    map[string]int{"PENDING": len(seen)},
	}, nil
}

// PlannedVerifyURL is the captured getTransactions GET used by verify.
func PlannedVerifyURL(accountID string) string {
	return PlannedFeedURL(accountID, "PENDING")
}
