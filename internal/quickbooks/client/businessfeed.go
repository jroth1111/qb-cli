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
	defer drainAndClose(resp)
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
		Items []json.RawMessage `json:"items"`
	}
	note := "business-feed POST /v1/business-feed/items"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "BusinessFeed", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	n := len(wrap.Items)
	return &QueryResult{
		Entity: "BusinessFeed",
		Counts: map[string]int{"items": 0, "totalCount": n},
		Items:  []QueryItem{},
		Note:   fmt.Sprintf("%s n=%d", note, n),
	}
}
