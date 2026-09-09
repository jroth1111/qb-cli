package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const itemReceiptGraphQLURL = "https://warehouse-management-svc.api.intuit.com/graphql"
const itemReceiptHost = "warehouse-management-svc.api.intuit.com"

const getItemReceiptsQuery = `query GetItemReceipts($first: PositiveInt, $after: String, $before: String, $last: PositiveInt, $filter: WarehouseManagement_ItemReceiptFilter, $orderBy: [WarehouseManagement_ItemReceiptOrderBy!]) {
  warehouseManagementItemReceipts(first: $first, after: $after, before: $before, last: $last, filter: $filter, orderBy: $orderBy) {
    ... on WarehouseManagement_ItemReceiptConnection {
      edges { cursor node { id __typename } __typename }
      nodes { id txnDate referenceNo status memo vendor { id name __typename } __typename }
      pageInfo { hasPreviousPage hasNextPage startCursor endCursor __typename }
      totalCount
      __typename
    }
    ... on WarehouseManagement_ItemReceiptError { message __typename }
    __typename
  }
}`

func PlannedItemReceiptURL() string { return itemReceiptGraphQLURL }

func replayGetItemReceipts(ctx context.Context, _, _ string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "GetItemReceipts",
		"variables": map[string]any{
			"filter":  map[string]any{},
			"orderBy": []string{"TXN_DATE_DESC", "ID_DESC"},
			"first":   limit,
			"after":   "",
		},
		"query": getItemReceiptsQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("item-receipt GetItemReceipts: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, itemReceiptGraphQLURL, itemReceiptHost, payload)
	if err != nil {
		return nil, fmt.Errorf("item-receipt GetItemReceipts: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading item-receipt GetItemReceipts: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectGetItemReceipts(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectGetItemReceipts(body []byte) *QueryResult {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Result struct {
				TotalCount int    `json:"totalCount"`
				Typename   string `json:"__typename"`
			} `json:"warehouseManagementItemReceipts"`
		} `json:"data"`
	}
	note := "warehouse-management-svc GetItemReceipts"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "ItemReceipt", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	total := wrap.Data.Result.TotalCount
	note = fmt.Sprintf("warehouse-management-svc GetItemReceipts totalCount=%d", total)
	if wrap.Data.Result.Typename != "" {
		note += " " + wrap.Data.Result.Typename
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "ItemReceipt",
		Counts: map[string]int{"items": 0, "totalCount": total},
		Items:  []QueryItem{},
		Note:   note,
	}
}
