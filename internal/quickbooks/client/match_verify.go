package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

// ErrMatchVerification means a match may have been applied but has not been
// proved by readback. Callers must inspect current state before retrying a POST.
var ErrMatchVerification = errors.New("match outcome unverified; inspect current state before retrying")

func verifyMatchResponse(body []byte, accountID string, rows []map[string]any, targets []MatchTxn) error {
	var result struct {
		Success []map[string]any `json:"success"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.Success) != len(rows) {
		return fmt.Errorf("%w: response lacks the requested success entries", ErrMatchVerification)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		id := mapStr(row, "id")
		var found map[string]any
		for _, entry := range result.Success {
			if mapStr(entry, "id") == id {
				found = entry
				break
			}
		}
		if found == nil || seen[id] {
			return fmt.Errorf("%w: missing or duplicate feed result %s", ErrMatchVerification, id)
		}
		seen[id] = true
		account, _ := found["qboAccount"].(map[string]any)
		if jsonNumberString(account["accountId"]) != accountID || !matchTargetsEqual(found, targets) {
			return fmt.Errorf("%w: result account or targets differ for %s", ErrMatchVerification, id)
		}
	}
	return nil
}

func matchTargetsEqual(row map[string]any, targets []MatchTxn) bool {
	entries, ok := row["matchedQboTxns"].([]any)
	if !ok || len(entries) != len(targets) {
		return false
	}
	want := map[string]bool{}
	for _, t := range targets {
		if want[t.TxnID] {
			return false
		}
		want[t.TxnID] = true
	}
	for _, v := range entries {
		entry, ok := v.(map[string]any)
		if !ok {
			return false
		}
		id := jsonNumberString(entry["qboTxnId"])
		if !want[id] {
			return false
		}
		delete(want, id)
	}
	return len(want) == 0
}

// Only the UI-observed blank (2) -> cleared (1) transition is allowed to
// differ. Other states, including any reconciled state, must be preserved.
func matchRegisterPreserved(before, after map[string]any) bool {
	if after == nil {
		return false
	}
	for _, k := range []string{"txnId", "sequence", "txnTypeId", "date", "amount", "payment", "deposit", "lineAccountId", "accountId", "currencyType", "memo", "klassId", "tax"} {
		if !reflect.DeepEqual(before[k], after[k]) {
			return false
		}
	}
	a, b := jsonNumberString(before["clearState"]), jsonNumberString(after["clearState"])
	return a == b || a == "2" && b == "1"
}

func verifyMatchReadback(ctx context.Context, ac *apiClient, accountID string, before []map[string]any, registers []map[string]any, targets []MatchTxn) error {
	// Retry reads only: an eventually consistent read must never repeat the write.
	var last error
	for attempt := range 3 {
		last = verifyMatchReadbackOnce(ctx, ac, accountID, before, registers, targets)
		if last == nil {
			confirmMutation(ctx)
			return nil
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("%w: %v", ErrMatchVerification, ctx.Err())
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	return fmt.Errorf("%w: %v", ErrMatchVerification, last)
}

func verifyMatchReadbackOnce(ctx context.Context, ac *apiClient, accountID string, before []map[string]any, registers []map[string]any, targets []MatchTxn) error {
	wanted := map[string]map[string]any{}
	for _, row := range before {
		wanted[mapStr(row, "id")] = row
	}
	found := map[string]bool{}
	seen := map[string]bool{}
	for page := 0; page < 100 && len(found) < len(wanted); page++ {
		start := page * feedMutationPageSize
		url := fmt.Sprintf("%s/getTransactions?sort=-txnDate&reviewState=ACCEPTED&ignoreMatching=false&accountId=%s&startIndex=%d&chunkSize=%d", ac.neoFeedURL(), accountID, start, feedMutationPageSize)
		resp, err := ac.get(ctx, url, fmt.Sprintf("items=%d-%d", start, start+feedMutationPageSize-1))
		if err != nil {
			return err
		}
		body, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("accepted read returned %d", resp.StatusCode)
		}
		var env struct {
			Items []map[string]any `json:"items"`
		}
		if err = json.Unmarshal(body, &env); err != nil {
			return err
		}
		fresh := 0
		for _, row := range env.Items {
			id := mapStr(row, "id")
			if !seen[id] {
				seen[id] = true
				fresh++
			}
			old, ok := wanted[id]
			if !ok {
				continue
			}
			if jsonNumberString(row["qboAccountId"]) != accountID || !matchTargetsEqual(row, targets) {
				return fmt.Errorf("accepted row %s has different account or target", id)
			}
			for _, key := range []string{"amount", "olbTxnDate", "origDescription"} {
				if !reflect.DeepEqual(old[key], row[key]) {
					return fmt.Errorf("accepted row %s changed %s", id, key)
				}
			}
			found[id] = true
		}
		if len(env.Items) < feedMutationPageSize || fresh == 0 {
			break
		}
	}
	if len(found) != len(wanted) {
		return fmt.Errorf("accepted population did not contain every requested row")
	}
	after, err := registerRowsForAccount(ctx, ac, accountID)
	if err != nil {
		return err
	}
	for _, t := range targets {
		old := findRegisterRow(registers, t.TxnID)
		now := findRegisterRow(after, t.TxnID)
		if !matchRegisterPreserved(old, now) {
			return fmt.Errorf("register amount, attribution or clearing status changed unexpectedly for %s", t.TxnID)
		}
	}
	ids := make([]string, 0, len(before))
	for _, row := range before {
		ids = append(ids, mapStr(row, "olbTxnId"))
	}
	pending, err := stateRows(ctx, ac, accountID, "PENDING", ids, false)
	if err != nil || len(pending) != 0 {
		return fmt.Errorf("matched rows remain pending or absence could not be proved")
	}
	return nil
}
