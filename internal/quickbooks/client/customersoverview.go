package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Captured 2026-08-19 from POST https://qbo.intuit.com/app/customers-overview
// (catalog-capture-rest.json). Wrap ATS proved HTTP 200 invoiceSummary
// totalCount=5 plus getTransactions open estimates + open invoice edges.
// Honest customers overview — not the Customer stand-in.
const customersOverviewGraphQLURL = "https://qbo.intuit.com/api/v4/graphql"
const customersOverviewReferer = "https://qbo.intuit.com/app/customers-overview"

const invoiceSummaryQuery = `query invoiceSummary($offset: Int, $limit: Int, $orderBy: String, $filterBy: String, $with: String, $first: Int, $after: String) {
  company {
    invoiceSummary(offset: $offset, limit: $limit, orderBy: $orderBy, filterBy: $filterBy, with: $with, first: $first, after: $after) {
      totalCount
    }
  }
}`

const getTransactionsQuery = `query getTransactions($offset:Int, $limit:Int,$with:String, $filterBy:String, $orderBy:String){
                company {
                    transactions(offset:$offset, limit:$limit, with:$with, filterBy:$filterBy, orderBy:$orderBy) {
                        edges {
                            node {
                                type
                                id
                                header {
                                    txnStatus
                                    transactionType
                                }
                            }
                        }
                    }
                }
            }`

const getTransactionsEstimateWith = "calculateTotals=false && skipInvoiceTracker=true && skipContactPseudonym=true && categoryId='-1' && showAttachableCount=false && txnStatus='open' && txnTypeFilter='estimate' && sortBy='status'"
const getTransactionsInvoiceWith = "calculateTotals=false && skipInvoiceTracker=true && skipContactPseudonym=true && categoryId='-1' && showAttachableCount=false && txnStatus='open' && txnTypeFilter='invoice' && sortBy='status'"
const invoiceSummaryFilterBy = "txnDate >= '1900-01-01' && txnDate < '9999-12-31'"

func PlannedCustomersOverviewURL() string { return customersOverviewGraphQLURL }

// replayCustomersOverview POSTs captured invoiceSummary + both getTransactions.
// Must not return a Customer list.
func replayCustomersOverview(ctx context.Context, _, _ string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	extra := map[string]string{
		"Referer":      customersOverviewReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}

	sumBody, err := postCustomersOverviewGQL(ctx, ac, extra, map[string]any{
		"query": invoiceSummaryQuery,
		"variables": map[string]any{
			"orderBy":  "status asc",
			"with":     nil,
			"filterBy": invoiceSummaryFilterBy,
			"offset":   0,
			"limit":    100,
			"first":    nil,
			"after":    nil,
		},
	})
	if err != nil {
		return nil, err
	}

	estBody, err := postCustomersOverviewGQL(ctx, ac, extra, map[string]any{
		"query": getTransactionsQuery,
		"variables": map[string]any{
			"with":   getTransactionsEstimateWith,
			"offset": 0,
			"limit":  500,
		},
	})
	if err != nil {
		return nil, err
	}

	invBody, err := postCustomersOverviewGQL(ctx, ac, extra, map[string]any{
		"query": getTransactionsQuery,
		"variables": map[string]any{
			"with":   getTransactionsInvoiceWith,
			"offset": 0,
			"limit":  500,
		},
	})
	if err != nil {
		return nil, err
	}

	res := projectCustomersOverview(sumBody, estBody, invBody, limit)
	res.Status = http.StatusOK
	return res, nil
}

func postCustomersOverviewGQL(ctx context.Context, ac *apiClient, extra map[string]string, payload map[string]any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("customers-overview: %w", err)
	}
	resp, err := ac.postJSONExtra(ctx, customersOverviewGraphQLURL, raw, extra)
	if err != nil {
		return nil, fmt.Errorf("customers-overview: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading customers-overview: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	return body, nil
}

func projectCustomersOverview(sumBody, estBody, invBody []byte, limit int) *QueryResult {
	note := "v4 graphql invoiceSummary+getTransactions"
	total := 0
	var sumWrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Company struct {
				InvoiceSummary struct {
					TotalCount int `json:"totalCount"`
				} `json:"invoiceSummary"`
			} `json:"company"`
		} `json:"data"`
	}
	if json.Unmarshal(sumBody, &sumWrap) == nil {
		total = sumWrap.Data.Company.InvoiceSummary.TotalCount
	}
	ests := projectOverviewTxns(estBody)
	invs := projectOverviewTxns(invBody)
	items := append([]QueryItem{}, ests...)
	items = append(items, invs...)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		items = []QueryItem{}
	}
	note = fmt.Sprintf("%s totalCount=%d estimates=%d invoices=%d", note, total, len(ests), len(invs))
	if len(sumWrap.Errors) > 0 && sumWrap.Errors[0].Message != "" {
		note += "; gql: " + sumWrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "CustomersOverview",
		Counts: map[string]int{
			"items":         len(items),
			"totalCount":    total,
			"openEstimates": len(ests),
			"openInvoices":  len(invs),
		},
		Items: items,
		Note:  note,
	}
}

func projectOverviewTxns(body []byte) []QueryItem {
	var wrap struct {
		Data struct {
			Company struct {
				Transactions struct {
					Edges []struct {
						Node struct {
							ID     string `json:"id"`
							Type   string `json:"type"`
							Header struct {
								TxnStatus       string `json:"txnStatus"`
								TransactionType string `json:"transactionType"`
							} `json:"header"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"transactions"`
			} `json:"company"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &wrap) != nil {
		return []QueryItem{}
	}
	out := make([]QueryItem, 0, len(wrap.Data.Company.Transactions.Edges))
	for _, e := range wrap.Data.Company.Transactions.Edges {
		n := e.Node
		if n.ID == "" && n.Type == "" {
			continue
		}
		typ := n.Type
		if typ == "" {
			typ = n.Header.TransactionType
		}
		out = append(out, QueryItem{
			ID:   n.ID,
			Name: n.Header.TxnStatus,
			Type: typ,
		})
	}
	return out
}
