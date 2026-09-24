package client

import (
	"encoding/json"
	"fmt"
)

// A successful HTTP response can still contain per-row validation failures.
// Require receipts for exactly the submitted feed rows before reporting success.
func validateAcceptResponse(raw, request []byte) error {
	var req struct {
		Next struct {
			Account string `json:"accountId"`
		} `json:"nextTxnInfo"`
		List struct {
			Rows []map[string]any `json:"olbTxns"`
		} `json:"txnList"`
	}
	if err := json.Unmarshal(request, &req); err != nil || len(req.List.Rows) == 0 {
		return fmt.Errorf("accept response cannot be checked against request")
	}
	want := map[string]bool{}
	for _, row := range req.List.Rows {
		id := mapStr(row, "olbTxnId")
		if id == "" {
			return fmt.Errorf("accept request lacks downloaded transaction id")
		}
		if want[id] {
			return fmt.Errorf("accept request repeats transaction %s", id)
		}
		want[id] = true
	}
	var response struct {
		Errors   json.RawMessage `json:"errorDetails"`
		Accepted []struct {
			ID      string `json:"olbTxnId"`
			Account struct {
				ID string `json:"accountId"`
			} `json:"qboAccount"`
			Added []struct {
				ID flexibleString `json:"qboTxnId"`
			} `json:"addedQboTxns"`
			Matched []struct {
				ID flexibleString `json:"qboTxnId"`
			} `json:"matchedQboTxns"`
		} `json:"acceptedTxns"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return fmt.Errorf("invalid accept response: %w", err)
	}
	if len(response.Errors) > 0 && string(response.Errors) != "null" && string(response.Errors) != "{}" {
		return fmt.Errorf("accept returned row errors; inspect state before retrying")
	}
	if len(response.Accepted) != len(want) {
		return fmt.Errorf("accept receipt count differs; inspect state before retrying")
	}
	for _, row := range response.Accepted {
		if !want[row.ID] || row.Account.ID != req.Next.Account {
			return fmt.Errorf("accept receipt has an unexpected transaction or account")
		}
		found := false
		for _, ref := range row.Added {
			found = found || ref.ID.String() != ""
		}
		for _, ref := range row.Matched {
			found = found || ref.ID.String() != ""
		}
		if !found {
			return fmt.Errorf("accept receipt has no accounting record for %s", row.ID)
		}
		delete(want, row.ID)
	}
	return nil
}
