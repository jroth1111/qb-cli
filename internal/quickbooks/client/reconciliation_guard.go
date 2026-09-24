package client

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
)

// ErrReconciliationUnsafe is a pre-submission refusal, never permission to
// unreconcile a transaction or retry the write through a different transport.
var ErrReconciliationUnsafe = errors.New("reconciliation safety not established; no mutation submitted")

// A CLEARED feed link must agree with the independently read register's
// UI-observed state 1 (cleared). State 2 (blank) is not reconciled, but it
// contradicts that feed link and must not authorize deletion. Unknown,
// absent, reconciled, and contradictory states fail closed. A date inside a
// statement period is never used as a proxy for reconciliation membership.
func requireClearedRegister(row map[string]any) error {
	if row != nil {
		switch jsonNumberString(row["clearState"]) {
		case "1":
			return nil
		}
	}
	return ErrReconciliationUnsafe
}

func preflightUnpostReconciliation(ctx context.Context, ac *apiClient, account string, feed []map[string]any) error {
	register, err := reconciliationRegisterRows(ctx, ac, account)
	if err != nil {
		return fmt.Errorf("%w: register read: %v", ErrReconciliationUnsafe, err)
	}
	for _, row := range feed {
		links, ok := row["matchedQboTxns"].([]any)
		if !ok || len(links) == 0 {
			return fmt.Errorf("%w: feed %s has no proven accounting links", ErrReconciliationUnsafe, mapStr(row, "id"))
		}
		for _, value := range links {
			link, _ := value.(map[string]any)
			id := jsonNumberString(link["qboTxnId"])
			// Require agreement with the separately read register. Unknown feed
			// enums must not be silently treated as unreconciled.
			if id == "" || mapStr(link, "clearState") != "CLEARED" {
				return fmt.Errorf("%w: linked transaction %s is not explicitly CLEARED", ErrReconciliationUnsafe, id)
			}
			found := false
			for _, entry := range register {
				if jsonNumberString(entry["txnId"]) != id {
					continue
				}
				found = true
				if jsonNumberString(entry["lineAccountId"]) != account || requireClearedRegister(entry) != nil {
					return fmt.Errorf("%w: linked transaction %s register state", ErrReconciliationUnsafe, id)
				}
			}
			if !found {
				return fmt.Errorf("%w: linked transaction %s absent from register read", ErrReconciliationUnsafe, id)
			}
		}
	}
	return nil
}

// Keep every register line, including multiple lines for the same transaction.
// A short page is not absence proof; only an empty page completes this census.
func reconciliationRegisterRows(ctx context.Context, ac *apiClient, account string) ([]map[string]any, error) {
	if !mutationEntityID.MatchString(account) {
		return nil, fmt.Errorf("invalid register account id")
	}
	var rows []map[string]any
	seen := map[[32]byte]bool{}
	for page := range 100 {
		resp, err := ac.get(ctx, fmt.Sprintf(registerBaseURL+"?accountId=%s", ac.realm, account), fmt.Sprintf("items=%d-%d", page*300, page*300+299))
		if err != nil {
			return nil, err
		}
		raw, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("register HTTP %d", resp.StatusCode)
		}
		items, err := registerRowItems(raw)
		if err != nil {
			return nil, err
		}
		// null is not an independently observed empty array.
		if items == nil {
			return nil, fmt.Errorf("register returned null")
		}
		if len(items) == 0 {
			return rows, nil
		}
		hash := sha256.Sum256(raw)
		if seen[hash] {
			return nil, fmt.Errorf("register pagination repeated")
		}
		seen[hash] = true
		rows = append(rows, items...)
	}
	return nil, fmt.Errorf("register pagination ceiling reached")
}

// Preserve bank-side identity and reconciliation state, independently of V3,
// which does not expose the register clearing state. Category/Class/payee are
// intentionally not compared here: the V3 verifier checks their intended edits.
func purchaseRegisterSnapshot(ctx context.Context, ac *apiClient, account, id string) (map[string]map[string]any, error) {
	rows, err := reconciliationRegisterRows(ctx, ac, account)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, row := range rows {
		if jsonNumberString(row["txnId"]) != id {
			continue
		}
		sequence := jsonNumberString(row["sequence"])
		if sequence == "" || out[sequence] != nil || jsonNumberString(row["lineAccountId"]) != account || jsonNumberString(row["clearState"]) == "" || row["amount"] == nil || row["date"] == nil {
			return nil, fmt.Errorf("ambiguous or incomplete purchase register identity/state")
		}
		entry := map[string]any{}
		for _, key := range []string{"txnId", "sequence", "lineAccountId", "date", "amount", "payment", "deposit", "currencyType", "clearState"} {
			entry[key] = row[key]
		}
		out[sequence] = entry
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("purchase absent from funding register")
	}
	return out, nil
}
