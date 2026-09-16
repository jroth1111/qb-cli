package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const businessFeedURL = "https://business-feed.api.intuit.com/v1/business-feed/items"
const businessFeedHost = "business-feed.api.intuit.com"

func PlannedBusinessFeedURL() string { return businessFeedURL }

func replayBusinessFeedItems(ctx context.Context, _, _ string, _ int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"context": map[string]any{
			"locale":        "en-au",
			"platform":      "web",
			"appVersion":    "1.595.0",
			"widgetVersion": "1.0.0",
			"experience":    "default",
			"location":      "business_feed_page",
		},
		"filter": map[string]any{
			"assetStatus": "active",
		},
		"userAttributes": map[string]any{
			"accountantFirmId": "",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("business-feed items: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, businessFeedURL, businessFeedHost, payload)
	if err != nil {
		return nil, fmt.Errorf("business-feed items: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading business-feed items: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectBusinessFeedItems(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectBusinessFeedItems(body []byte) *QueryResult {
	var wrap struct {
		Items      []json.RawMessage `json:"items"`
		TotalCount int               `json:"totalCount"`
	}
	note := "business-feed POST /v1/business-feed/items"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "BusinessFeed", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	n := len(wrap.Items)
	n = max(n, wrap.TotalCount)
	// Preserve only the secret-free scalar fields each raw item actually
	// carries — arbitrary raw JSON never reaches the envelope.
	items := make([]QueryItem, 0, len(wrap.Items))
	for _, raw := range wrap.Items {
		var o map[string]json.RawMessage
		if err := json.Unmarshal(raw, &o); err != nil || o == nil {
			continue
		}
		items = append(items, QueryItem{
			ID:        firstJSONString(o, "id", "Id", "itemId", "assetId"),
			Name:      firstJSONString(o, "name", "Name", "title", "Title", "displayName", "DisplayName"),
			Date:      firstJSONString(o, "date", "Date", "txnDate", "TxnDate", "createdDate", "updatedDate"),
			DocNumber: firstJSONString(o, "docNumber", "DocNumber"),
			Amount:    jsonFloat(o, "amount", "Amount", "totalAmount"),
			Type:      firstJSONString(o, "type", "Type", "itemType", "assetType"),
		})
	}
	return &QueryResult{
		Entity: "BusinessFeed",
		Counts: map[string]int{"items": len(items), "totalCount": n},
		Items:  items,
		Note:   fmt.Sprintf("%s n=%d", note, n),
	}
}
