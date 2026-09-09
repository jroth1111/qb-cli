package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Captured 2026-08-19 from POST https://qbo.intuit.com/app/salesorders
// (PAGE1240587601). Wrap ATS/cookie already proved HTTP 200 empty
// (totalCount=0) via leftover_ats_probe_test.go. Honest AU sales-order
// list — not the Invoice 115/87 stand-in.
const salesOrderGraphQLURL = "https://commercecontrol.api.intuit.com/graphql"

const getSalesOrdersQuery = `query GetSalesOrders($offset: Int! = 0, $limit: Int! = 100, $filter: SalesOrderFilterCriteria, $sort: [SalesOrderSortCriteria!]) {
  result: getSalesOrders(
    offset: $offset
    limit: $limit
    filter: $filter
    sort: $sort
  ) {
    totalCount
    __typename
  }
}`

func PlannedSalesOrderURL() string { return salesOrderGraphQLURL }

// replayGetSalesOrders POSTs the captured GetSalesOrders GraphQL.
// id/query are accepted for cobra flag parity but not forwarded — the
// captured body has no by-id or filter variables we can honestly send.
func replayGetSalesOrders(ctx context.Context, _, _ string, limit int) (*QueryResult, error) {
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
		"operationName": "GetSalesOrders",
		"variables": map[string]any{
			"offset": 0,
			"limit":  limit,
		},
		"query": getSalesOrdersQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("sales-order GetSalesOrders: %w", err)
	}
	extra := map[string]string{
		"Referer":      "https://qbo.intuit.com/app/salesorders",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, salesOrderGraphQLURL, payload, extra)
	if err != nil {
		return nil, fmt.Errorf("sales-order GetSalesOrders: %w", err)
	}
	defer drainAndClose(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading sales-order GetSalesOrders: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	res := projectGetSalesOrders(body)
	res.Status = resp.StatusCode
	return res, nil
}

func projectGetSalesOrders(body []byte) *QueryResult {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Result struct {
				TotalCount int    `json:"totalCount"`
				Typename   string `json:"__typename"`
			} `json:"result"`
		} `json:"data"`
	}
	note := "commercecontrol GetSalesOrders"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{
			Entity: "SalesOrder",
			Counts: map[string]int{"items": 0},
			Items:  []QueryItem{},
			Note:   note + " (unparsed)",
		}
	}
	total := wrap.Data.Result.TotalCount
	note = fmt.Sprintf("commercecontrol GetSalesOrders totalCount=%d", total)
	if wrap.Data.Result.Typename != "" {
		note += " " + wrap.Data.Result.Typename
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "SalesOrder",
		Counts: map[string]int{"items": 0, "totalCount": total},
		Items:  []QueryItem{},
		Note:   note,
	}
}
