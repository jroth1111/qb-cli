package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Captured 2026-08-19 on /app/sales (open-39 follow-up).
// POST qbo.intuit.com/api/v4/graphql query transactions → 200 list items=24.
// Honest sales-hub list — not Invoice stand-in, not allSales aggregates.
const salesHubGraphQLURL = "https://qbo.intuit.com/api/v4/graphql"
const salesHubReferer = "https://qbo.intuit.com/app/sales"

const salesHubTransactionsQuery = `query transactions($offset:Int, $limit:Int, $with:String, $filterBy:String, $orderBy:String){
  company {
    transactions(offset:$offset, limit:$limit, with:$with, filterBy:$filterBy, orderBy:$orderBy) {
      edges {
        node {
          id
          type
          header {
            amount
            transactionType
            txnDate
            referenceNumber
            txnStatus
          }
        }
      }
    }
  }
}`

const salesHubWith = "calculateTotals=false && skipInvoiceTracker=true && skipContactPseudonym=true && categoryId='-1' && showAttachableCount=true && status='all' && txnTypeFilter='incometxns'"
const salesHubFilterBy = "txnDate >= '1900-01-01' && txnDate < '9999-12-31'"

func PlannedSalesHubURL() string { return salesHubGraphQLURL }

func replaySalesHub(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 99999 // unbounded ask; service applies its own ceiling
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	extra := map[string]string{
		"Referer":      salesHubReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	payload := map[string]any{
		"query": salesHubTransactionsQuery,
		"variables": map[string]any{
			"offset":   0,
			"limit":    limit,
			"orderBy":  "txnDate desc",
			"with":     salesHubWith,
			"filterBy": salesHubFilterBy,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sales-hub transactions: %w", err)
	}
	resp, err := ac.postJSONExtra(ctx, salesHubGraphQLURL, raw, extra)
	if err != nil {
		return nil, fmt.Errorf("sales-hub transactions: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading sales-hub transactions: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	res := projectSalesHub(body, query, limit)
	res.Status = resp.StatusCode
	return res, nil
}

func projectSalesHub(body []byte, query string, limit int) *QueryResult {
	note := "v4 graphql transactions (sales hub)"
	var root struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return &QueryResult{Entity: "SalesHub", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	var edges []json.RawMessage
	var top map[string]json.RawMessage
	if json.Unmarshal(root.Data, &top) == nil {
		if c, ok := top["company"]; ok {
			var company map[string]json.RawMessage
			if json.Unmarshal(c, &company) == nil {
				if tx, ok := company["transactions"]; ok {
					var txo map[string]json.RawMessage
					if json.Unmarshal(tx, &txo) == nil {
						_ = json.Unmarshal(txo["edges"], &edges)
					}
				}
			}
		}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(edges))
	for _, e := range edges {
		var wrapE struct {
			Node struct {
				ID     string `json:"id"`
				Type   string `json:"type"`
				Header struct {
					TransactionType string          `json:"transactionType"`
					TxnDate         string          `json:"txnDate"`
					ReferenceNumber string          `json:"referenceNumber"`
					TxnStatus       string          `json:"txnStatus"`
					Amount          json.RawMessage `json:"amount"`
				} `json:"header"`
			} `json:"node"`
		}
		if json.Unmarshal(e, &wrapE) != nil {
			continue
		}
		n := wrapE.Node
		amt := 0.0
		var f float64
		if json.Unmarshal(n.Header.Amount, &f) == nil {
			amt = f
		} else {
			var obj map[string]any
			if json.Unmarshal(n.Header.Amount, &obj) == nil {
				if v, ok := obj["value"].(float64); ok {
					amt = v
				}
			}
		}
		it := QueryItem{
			ID: n.ID, Name: n.Header.TransactionType, Date: n.Header.TxnDate,
			DocNumber: n.Header.ReferenceNumber, Amount: amt,
			Type: firstNonEmpty(n.Type, n.Header.TransactionType),
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Name+" "+it.DocNumber+" "+it.ID+" "+it.Type), q) {
			continue
		}
		items = append(items, it)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	if len(root.Errors) > 0 && root.Errors[0].Message != "" {
		note += "; gql: " + root.Errors[0].Message
	}
	if len(root.Data) == 0 {
		note += " (empty data)"
	}
	note = fmt.Sprintf("%s edges=%d items=%d", note, len(edges), len(items))
	return &QueryResult{
		Entity: "SalesHub",
		Counts: map[string]int{"items": len(items), "totalCount": len(edges)},
		Items:  items,
		Note:   note,
	}
}
