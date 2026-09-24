package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// COMPANY.SEARCH — "company search run". Captured on TC2 2026-09-19: the
// nav searchbox (Search → type → Enter) lands on /app/fullSearch/transactions
// and fans out four queries to ceressos.api.intuit.com/graphql under the
// base-list-ui plugin. The transaction pane is GetTransactionGlobalSearch
// Entities; jumpTos/helpArticles/alsoSearch are navigation+help surfaces and
// stay out of scope for the CLI row projection.
//
//	POST https://ceressos.api.intuit.com/graphql
//	{query: GetTransactionGlobalSearchEntities, variables: {search,
//	 transactionLimit, transactionOffset, transactionSortBy, with* flags,
//	 useNER, allowMultipleContacts}}
//	→ data.transactions.{data[],totalCount}
//
// The host's own header set (base-list-ui Intuit_APIKey, intuit-plugin-id,
// intuit_locale) is required — captured into URIHostHeaders from the same
// session; without it the gateway cannot route the document.
const globalSearchDoc = `query GetTransactionGlobalSearchEntities(
	$search: String!

  $transactionOffset: Int = 0
  $transactionLimit: Int = 5
  $transactionFilters: [TransactionOrConditionInput!]
  $transactionSortBy: [TransactionSortInput!]

  $withReferenceNumber: Boolean = false
  $withContact: Boolean = false
  $withDate: Boolean = false
  $withDueDate: Boolean = false
  $withBalance: Boolean = false
  $withMemo: Boolean = false
  $withLastModifiedDate: Boolean = false

  $allowMultipleContacts: Boolean = false
  $useNER: Boolean = false
) {

  transactions(
    page: { offset: $transactionOffset, limit: $transactionLimit }, search: $search, sortBy:$transactionSortBy, transactionFilters: $transactionFilters, useNER: $useNER, allowMultipleContacts: $allowMultipleContacts
  ) {
    data {
      ...transactionAttributes
      __typename
    }
    totalCount
    __typename
  }
}

fragment transactionAttributes on Transaction {
  type
  id
  referenceNumber @include(if: $withReferenceNumber)
  contact @include(if: $withContact) {
    id
    fullName
    type
    __typename
  }
  currency {
    currencyType
  }
  date @include(if: $withDate)
  amount
  transactionStatus
  dueDate @include(if: $withDueDate)
  balance @include(if: $withBalance)
  memo @include(if: $withMemo)
  lastModified @include(if: $withLastModifiedDate)
  meta @include(if: $withLastModifiedDate) {
    modified
  }
  __typename
}`

// ReplayGlobalSearch runs the global transaction search captured from the
// nav searchbox — the fullSearch transactions pane verbatim (same sort,
// same with* fields, NER on). --limit maps to transactionLimit.
func ReplayGlobalSearch(ctx context.Context, query string, limit int) (*QueryResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("--query is required")
	}
	if limit < 1 {
		limit = 20
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	vars := map[string]any{
		"search":                q,
		"transactionOffset":     0,
		"transactionLimit":      limit,
		"transactionSortBy":     []map[string]any{{"field": "DATE", "order": "DESC"}, {"field": "SCORE", "order": "DESC"}, {"field": "CREATED", "order": "DESC"}},
		"withDate":              true,
		"withReferenceNumber":   true,
		"withContact":           true,
		"withDueDate":           true,
		"withBalance":           true,
		"withLastModifiedDate":  true,
		"withMemo":              true,
		"useNER":                true,
		"allowMultipleContacts": false,
	}
	payload, err := json.Marshal(map[string]any{"query": globalSearchDoc, "variables": vars})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost,
		"https://ceressos.api.intuit.com/graphql",
		"ceressos.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("ceressos global search: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("ceressos global search: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data struct {
			Transactions struct {
				Data       []map[string]any `json:"data"`
				TotalCount int              `json:"totalCount"`
			} `json:"transactions"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("ceressos global search: malformed response %.200s", raw)
	}
	if len(doc.Errors) > 0 && doc.Errors[0].Message != "" {
		return nil, fmt.Errorf("ceressos global search: %s", doc.Errors[0].Message)
	}
	items := make([]QueryItem, 0, len(doc.Data.Transactions.Data))
	for _, t := range doc.Data.Transactions.Data {
		it := QueryItem{
			ID:        fmt.Sprintf("%v", t["id"]),
			Type:      fmt.Sprintf("%v", t["type"]),
			Amount:    floatVal(t["amount"]),
			Date:      fmt.Sprintf("%v", t["date"]),
			DocNumber: fmt.Sprintf("%v", t["referenceNumber"]),
		}
		if c, ok := t["contact"].(map[string]any); ok {
			it.Name = fmt.Sprintf("%v", c["fullName"])
		}
		if it.Name == "" {
			it.Name = it.DocNumber
		}
		if it.Date == "<nil>" {
			it.Date = ""
		}
		if it.DocNumber == "<nil>" {
			it.DocNumber = ""
		}
		items = append(items, it)
	}
	return &QueryResult{
		Entity: "Transaction",
		Status: resp.StatusCode,
		Counts: map[string]int{"items": len(items), "totalCount": doc.Data.Transactions.TotalCount},
		Items:  items,
		Note:   fmt.Sprintf("ceressos GetTransactionGlobalSearchEntities %q", q),
	}, nil
}

// floatVal coerces a decoded JSON number (float64) or numeric string.
func floatVal(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}
