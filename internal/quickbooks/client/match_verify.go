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
	byRow := map[string][]MatchTxn{}
	for _, row := range rows {
		byRow[mapStr(row, "id")] = targets
	}
	return verifyMatchResponseMapped(body, accountID, rows, byRow)
}

func verifyMatchResponseMapped(body []byte, accountID string, rows []map[string]any, targets map[string][]MatchTxn) error {
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
		if len(targets[id]) == 0 || jsonNumberString(account["accountId"]) != accountID || !matchTargetsEqual(found, targets[id]) {
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
	byRow := map[string][]MatchTxn{}
	for _, row := range before {
		byRow[mapStr(row, "id")] = targets
	}
	return verifyMatchReadbackMapped(ctx, ac, accountID, before, registers, byRow)
}

func verifyMatchReadbackMapped(ctx context.Context, ac *apiClient, accountID string, before []map[string]any, registers []map[string]any, targets map[string][]MatchTxn) error {
	// Retry reads only: an eventually consistent read must never repeat the write.
	var last error
	for attempt := range 3 {
		last = verifyMatchReadbackMappedOnce(ctx, ac, accountID, before, registers, targets)
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
	byRow := map[string][]MatchTxn{}
	for _, row := range before {
		byRow[mapStr(row, "id")] = targets
	}
	return verifyMatchReadbackMappedOnce(ctx, ac, accountID, before, registers, byRow)
}

func verifyMatchReadbackMappedOnce(ctx context.Context, ac *apiClient, accountID string, before []map[string]any, registers []map[string]any, targets map[string][]MatchTxn) error {
	wanted := map[string]map[string]any{}
	for _, row := range before {
		wanted[mapStr(row, "id")] = row
	}
	found := map[string]bool{}
	seen := map[string]bool{}
	start := 0
	size := ac.censusSize
	for page := 0; page < 100 && len(found) < len(wanted); page++ {
		url := fmt.Sprintf("%s/getTransactions?sort=-txnDate&reviewState=ACCEPTED&ignoreMatching=false&accountId=%s&startIndex=%d&chunkSize=%d", ac.neoFeedURL(), accountID, start, size)
		resp, err := ac.get(ctx, url, fmt.Sprintf("items=%d-%d", start, start+size-1))
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
			if len(targets[id]) == 0 || jsonNumberString(row["qboAccountId"]) != accountID || !matchTargetsEqual(row, targets[id]) {
				return fmt.Errorf("accepted row %s has different account or target", id)
			}
			for _, key := range []string{"amount", "olbTxnDate", "origDescription"} {
				if !reflect.DeepEqual(old[key], row[key]) {
					return fmt.Errorf("accepted row %s changed %s", id, key)
				}
			}
			if len(targets[id]) == 1 {
				book := findRegisterRow(registers, targets[id][0].TxnID)
				links := sliceObjects(row["matchedQboTxns"])
				if book != nil && jsonNumberString(book["txnTypeId"]) != "" {
					if len(links) != 1 || jsonNumberString(links[0]["txnTypeId"]) != jsonNumberString(book["txnTypeId"]) || jsonNumberString(links[0]["qboTxnSeqId"]) != jsonNumberString(book["sequence"]) || !reflect.DeepEqual(links[0]["amount"], old["amount"]) {
						return fmt.Errorf("accepted link type, sequence or amount differs for %s", id)
					}
				}
			}
			found[id] = true
		}
		if len(env.Items) == 0 {
			break
		}
		if fresh == 0 {
			return fmt.Errorf("accepted pagination repeated before requested rows were found")
		}
		start += len(env.Items)
	}
	if len(found) != len(wanted) {
		return fmt.Errorf("accepted population did not contain every requested row")
	}
	after, err := registerRowsForAccount(ctx, ac, accountID)
	if err != nil {
		return err
	}
	checked := map[string]bool{}
	for _, list := range targets {
		for _, t := range list {
			if checked[t.TxnID] {
				continue
			}
			checked[t.TxnID] = true
			old, err := uniqueMatchRegister(registers, t.TxnID)
			if err != nil {
				return err
			}
			now, err := uniqueMatchRegister(after, t.TxnID)
			if err != nil {
				return err
			}
			if !matchRegisterPreserved(old, now) {
				return fmt.Errorf("register amount, attribution or clearing status changed unexpectedly for %s", t.TxnID)
			}
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
