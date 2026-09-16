package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Captured 2026-08-19 from POST https://qbo.intuit.com/app/inventory/overview
// (catalog-capture-rest.json). Wrap ATS proved GetAllListViewEntities 200 data
// (10 PRODUCT entities) and SearchAllListViewEntities 200 empty (totalCount=0).
// Honest inventory overview — not the Item stand-in. Do not wire GetIMSPref
// (wrap 400).
const inventoryOverviewGraphQLURL = "https://commercecontrol.api.intuit.com/graphql"
const inventoryOverviewReferer = "https://qbo.intuit.com/app/inventory/overview"

const getAllListViewEntitiesQuery = `query GetAllListViewEntities($filter: ProductAndServiceListEntityFilterCriteria = {entityType: {value: [PRODUCT], comparator: IN}}, $sort: [ProductAndServicesListEntitySortCriteria!] = [], $offset: Int = 0, $limit: Int = 10) {
  result: getAllListViewEntities(filter: $filter, sort: $sort, offset: $offset, limit: $limit) {
    entities {
      ...ZeroStateListEntity
      __typename
    }
    __typename
  }
}

fragment ZeroStateListEntity on ProductAndServicesListEntityWithVariants {
  productsAndServicesListEntity {
    id
    default
    __typename
  }
  __typename
}`

const searchAllListViewEntitiesQuery = `query SearchAllListViewEntities($filter: ProductAndServiceListEntityFilterCriteria, $keyword: String, $limit: Int) {
  searchAllListViewEntities(filter: $filter, keyword: $keyword, limit: $limit) {
    entities {
      entityFullyQualifiedName
      entityName
      flat
      qtyOnHand
      id
      __typename
    }
    totalCount
    __typename
  }
}`

func PlannedInventoryOverviewURL() string { return inventoryOverviewGraphQLURL }

func replayGetAllListViewEntities(ctx context.Context, _, _ string, limit int) (*QueryResult, error) {
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
		"operationName": "GetAllListViewEntities",
		"variables": map[string]any{
			"filter": map[string]any{
				"entityType": map[string]any{
					"value":      []string{"PRODUCT"},
					"comparator": "IN",
				},
			},
			"sort":   []any{},
			"offset": 0,
			"limit":  limit,
		},
		"query": getAllListViewEntitiesQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("inventory-overview GetAllListViewEntities: %w", err)
	}
	extra := map[string]string{
		"Referer":      inventoryOverviewReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, inventoryOverviewGraphQLURL, payload, extra)
	if err != nil {
		return nil, fmt.Errorf("inventory-overview GetAllListViewEntities: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading inventory-overview GetAllListViewEntities: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	res := projectGetAllListViewEntities(body)
	res.Status = resp.StatusCode
	return res, nil
}

func replaySearchAllListViewEntities(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	keyword := strings.TrimSpace(query)
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	// Captured SearchAllListViewEntities query. keyword from cobra --query
	// (empty if absent). Do not hard-require LOW_ON_STOCK as the only search.
	payload, err := json.Marshal(map[string]any{
		"operationName": "SearchAllListViewEntities",
		"variables": map[string]any{
			"filter": map[string]any{
				"status": map[string]any{
					"value":      []string{"ACTIVE"},
					"comparator": "IN",
				},
			},
			"limit":   limit,
			"keyword": keyword,
		},
		"query": searchAllListViewEntitiesQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("inventory-overview SearchAllListViewEntities: %w", err)
	}
	extra := map[string]string{
		"Referer":      inventoryOverviewReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, inventoryOverviewGraphQLURL, payload, extra)
	if err != nil {
		return nil, fmt.Errorf("inventory-overview SearchAllListViewEntities: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading inventory-overview SearchAllListViewEntities: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	res := projectSearchAllListViewEntities(body)
	res.Status = resp.StatusCode
	return res, nil
}

func projectGetAllListViewEntities(body []byte) *QueryResult {
	note := "commercecontrol GetAllListViewEntities"
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Result struct {
				Typename string `json:"__typename"`
				Entities []struct {
					Typename string `json:"__typename"`
					Entity   struct {
						ID      string `json:"id"`
						Default bool   `json:"default"`
					} `json:"productsAndServicesListEntity"`
				} `json:"entities"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{
			Entity: "InventoryOverview",
			Counts: map[string]int{"items": 0},
			Items:  []QueryItem{},
			Note:   note + " (unparsed)",
		}
	}
	items := make([]QueryItem, 0, len(wrap.Data.Result.Entities))
	for _, e := range wrap.Data.Result.Entities {
		id := e.Entity.ID
		if id == "" {
			continue
		}
		it := QueryItem{ID: id, Type: "PRODUCT"}
		if e.Entity.Default {
			it.Name = "default"
		}
		items = append(items, it)
	}
	note = fmt.Sprintf("%s entities=%d", note, len(wrap.Data.Result.Entities))
	if wrap.Data.Result.Typename != "" {
		note += " " + wrap.Data.Result.Typename
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "InventoryOverview",
		Counts: map[string]int{"items": len(items), "entities": len(wrap.Data.Result.Entities)},
		Items:  items,
		Note:   note,
	}
}

func projectSearchAllListViewEntities(body []byte) *QueryResult {
	note := "commercecontrol SearchAllListViewEntities"
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Search struct {
				Typename   string `json:"__typename"`
				TotalCount int    `json:"totalCount"`
				Entities   []struct {
					ID                       string  `json:"id"`
					EntityName               string  `json:"entityName"`
					EntityFullyQualifiedName string  `json:"entityFullyQualifiedName"`
					QtyOnHand                float64 `json:"qtyOnHand"`
					Typename                 string  `json:"__typename"`
				} `json:"entities"`
			} `json:"searchAllListViewEntities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{
			Entity: "InventoryOverviewSearch",
			Counts: map[string]int{"items": 0},
			Items:  []QueryItem{},
			Note:   note + " (unparsed)",
		}
	}
	items := make([]QueryItem, 0, len(wrap.Data.Search.Entities))
	for _, e := range wrap.Data.Search.Entities {
		if e.ID == "" && e.EntityName == "" {
			continue
		}
		name := e.EntityName
		if name == "" {
			name = e.EntityFullyQualifiedName
		}
		items = append(items, QueryItem{
			ID:   e.ID,
			Name: name,
			Qty:  e.QtyOnHand,
			Type: "PRODUCT",
		})
	}
	total := wrap.Data.Search.TotalCount
	note = fmt.Sprintf("%s totalCount=%d entities=%d", note, total, len(wrap.Data.Search.Entities))
	if wrap.Data.Search.Typename != "" {
		note += " " + wrap.Data.Search.Typename
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "InventoryOverviewSearch",
		Counts: map[string]int{"items": len(items), "totalCount": total, "entities": len(wrap.Data.Search.Entities)},
		Items:  items,
		Note:   note,
	}
}
