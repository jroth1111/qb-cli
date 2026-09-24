package client

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// advancedMatchDetails is the UI's source of match eligibility and version.
// Register editSequence is not the transaction's match SyncToken.
func buildLiveMatchBody(ctx context.Context, ac *apiClient, accountID string, rows []map[string]any, targets []MatchTxn) ([]byte, error) {
	entries := make([]any, 0, len(rows))
	for _, row := range rows {
		dateText := mapStr(row, "olbTxnDate")
		date, err := time.Parse(time.RFC3339, dateText)
		if err != nil {
			return nil, fmt.Errorf("invalid feed date for matching")
		}
		query := url.Values{"initial": {"true"}, "id": {mapStr(row, "id")}, "lowDate": {date.AddDate(0, 0, -90).Format(time.DateOnly)}, "highDate": {date.AddDate(0, 0, 30).Format(time.DateOnly)}}
		resp, err := ac.get(ctx, ac.neoFeedURL()+"/advancedMatchDetails?"+query.Encode(), "")
		if err != nil {
			return nil, err
		}
		body, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
		}
		var details struct {
			Candidates []map[string]any `json:"homeCurrencyMatchingTxns"`
			Row        map[string]any   `json:"olbTxn"`
		}
		if err = json.Unmarshal(body, &details); err != nil {
			return nil, err
		}
		if mapStr(details.Row, "id") != mapStr(row, "id") || jsonNumberString(details.Row["qboAccountId"]) != accountID {
			return nil, fmt.Errorf("match-details feed/account mismatch")
		}
		oldAmount, oldOK := mapNum(row, "amount")
		liveAmount, liveOK := mapNum(details.Row, "amount")
		if !oldOK || !liveOK || oldAmount != liveAmount || mapStr(details.Row, "olbTxnDate") != dateText {
			return nil, fmt.Errorf("feed changed during match lookup")
		}
		selected := make([]any, 0, len(targets))
		for _, target := range targets {
			var candidate map[string]any
			for _, c := range details.Candidates {
				if jsonNumberString(c["qboTxnId"]) == target.TxnID {
					if candidate != nil {
						return nil, fmt.Errorf("ambiguous match candidate %s", target.TxnID)
					}
					candidate = c
				}
			}
			if candidate == nil {
				return nil, fmt.Errorf("%w: %s is not an eligible home-currency candidate", ErrMatchTargetNotFound, target.TxnID)
			}
			entry := map[string]any{"qboTxnId": target.TxnID}
			for _, field := range []string{"txnTypeId", "qboTxnSeqId", "txnSyncToken"} {
				value := jsonNumberString(candidate[field])
				if value == "" {
					return nil, fmt.Errorf("%w: %s in advancedMatchDetails", ErrMatchFieldMissing, field)
				}
				entry[field] = value
			}
			amount, ok := mapNum(candidate, "amount")
			if !ok {
				return nil, fmt.Errorf("%w: amount in advancedMatchDetails", ErrMatchFieldMissing)
			}
			entry["paymentAmount"] = strconv.FormatFloat(math.Abs(amount), 'f', 2, 64)
			selected = append(selected, entry)
		}
		entries = append(entries, map[string]any{"acceptType": "MATCH", "id": row["id"], "olbTxnDate": row["olbTxnDate"], "qboAccountId": accountID, "selectedMatches": map[string]any{"matchedTxns": selected, "addAdjQboTxn": nil, "addAsQboTxn": nil}})
	}
	return json.Marshal(map[string]any{"olbTxns": entries})
}
