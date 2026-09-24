package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Fixed assets ride assetservice.api.intuit.com/v1/graphql — live-proven on
// Test Company 2 (2026-09-17): financeAssets returns a typed
// Finance_AssetConnection (zero assets on the company). Static APIKey auth is
// stored under uri_host_headers["assetservice.api.intuit.com"].

const assetServiceHost = "assetservice.api.intuit.com"
const assetServiceGraphQLURL = "https://assetservice.api.intuit.com/v1/graphql"

const financeAssetsDoc = `query financeAssets($first: Int, $last: Int, $after: String, $before: String, $financeAssetFilter: Finance_AssetFilter) {
  financeAssets(first: $first, last: $last, after: $after, before: $before, financeAssetFilter: $financeAssetFilter) {
    edges { node {
      createdAt assetId name description assetNumber associatedIds purchaseDate disposalDate status lifecycleStatus
      purchaseValue { currency value }
      netBookValue { currency value }
      salvageValue { currency value }
      depreciationInfos { method lifeYear depreciationStartDate depreciationRate accumulatedDepreciation { currency value } }
      __typename
    } __typename }
    pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
    __typename
  }
}`

// ReplayFinanceAssets queries financeAssets on the asset service. --id filters
// on assetId; --query substring-filters the projected node JSON.
func ReplayFinanceAssets(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "financeAssets",
		"variables": map[string]any{
			"first":              limit,
			"offset":             0,
			"limit":              limit,
			"financeAssetFilter": map[string]any{"associatedIDFilter": []string{}},
		},
		"query": financeAssetsDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, assetServiceGraphQLURL, assetServiceHost, payload)
	if err != nil {
		return nil, fmt.Errorf("assetservice financeAssets: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("assetservice financeAssets: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectEdgeNodes("FixedAsset", "assetservice.api.intuit.com financeAssets", raw,
		[]string{"data", "financeAssets", "edges"}, query, limit, resp.StatusCode)
	if id = strings.TrimSpace(id); id != "" {
		var keep []QueryItem
		for _, it := range res.Items {
			if it.ID == id || strings.HasSuffix(it.ID, ":"+id) {
				keep = append(keep, it)
			}
		}
		res.Items = keep
		res.Counts["items"] = len(keep)
	}
	return res, nil
}
