package client

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ITEMS_IMPORT rides the batch-transactions bulk-action GraphQL service —
// captured live on TC2 2026-09-18 from /app/importdata/Items
// (Import from file → Upload → Review columns → grid → Save):
//
//	POST https://qbbulkaction.api.intuit.com/v4/graphql
//	mutation Qbshared_Bulk_ProductBulkAction_bulkActionOnProductList
//	variables.input_0.qbsharedBulkProductBulkActionProductListInput:
//	  {mode:"ASYNC", name:"Products-<yyyyMMddHHmmss>", action:"CREATE",
//	   products:[{clientMutationId:"<i>", product:{…}}]}
//
// mode ASYNC means the mutation only queues the bulk action; the UI then
// polls company.transactionBulkActionList on the same endpoint until the
// row reports terminal status. Account/category names resolve through the
// v3 query service like the bills/reclassify lanes.
const bulkActionHost = "qbbulkaction.api.intuit.com"
const bulkActionURL = "https://qbbulkaction.api.intuit.com/v4/graphql"

const bulkProductCreateDoc = `mutation Qbshared_Bulk_ProductBulkAction_bulkActionOnProductList($input_0: Qbshared_Bulk_ProductBulkAction_bulkActionOnProductListInput!){
      Qbshared_Bulk_ProductBulkAction_bulkActionOnProductList(input:$input_0){
          clientMutationId,
          qbsharedBulkProductBulkActionProductListInput{
              id,
              status,
              successEntities {
      clientMutationId
  },
  status,
  failedEntities {
      clientMutationId
      errors {
          message,
          code
      }
  },
  inProgressEntities
          }
      }
  }`

const bulkActionListDoc = `query {
    company {
        transactionBulkActionList {
            edges {
                node {
                    id
                    name
                    entityVersion
                    totalRequests
                    totalSuccessful
                    totalUnsuccessful
                    createdDate
                    status
                }
            }
        }
    }
}`

// itemsImportColumns maps CSV headers (case-insensitive) to product fields.
var itemsImportColumns = map[string]string{
	"product/service name": "name", "name": "name", "product name": "name", "service name": "name", "item name": "name",
	"category":  "category",
	"item type": "type", "type": "type",
	"sku":               "sku",
	"sales description": "salesDescription", "description": "salesDescription",
	"sales price/rate": "price", "sales price / rate": "price", "sales price": "price", "price": "price", "rate": "price", "sales rate": "price",
	"income account":       "incomeAccount",
	"purchase description": "purchaseDescription",
	"purchase cost":        "cost", "cost": "cost",
	"expense account": "expenseAccount",
}

// PlannedItemsImportURL reports the mutation URL for dry-run plans.
func PlannedItemsImportURL() string { return bulkActionURL }

// ReplayItemsImport creates products/services from a CSV through the
// captured bulk-action contract. Service and non-inventory rows only —
// inventory rows need the quantity/asset-account flow the wizard drives
// differently and were not proven.
func ReplayItemsImport(ctx context.Context, path string) (*MutateResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("items import requires --file <csv>")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("items import: %w", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("items import: reading CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("items import: %s needs a header row plus at least one data row", path)
	}
	header := rows[0]
	data := rows[1:]

	colField := make([]string, len(header))
	for ci, h := range header {
		colField[ci] = itemsImportColumns[strings.ToLower(strings.TrimSpace(h))]
	}
	mapped := map[string]bool{}
	for _, k := range colField {
		if k != "" {
			mapped[k] = true
		}
	}
	if !mapped["name"] {
		return nil, fmt.Errorf("items import: a Product/Service name column is required")
	}
	if !mapped["type"] {
		return nil, fmt.Errorf("items import: an Item type column is required")
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if ac.realm == "" {
		return nil, fmt.Errorf("items import: no realm id in credentials")
	}

	products := make([]any, 0, len(data))
	for ri, rec := range data {
		get := func(field string) string {
			for ci, k := range colField {
				if k == field && ci < len(rec) {
					return strings.TrimSpace(rec[ci])
				}
			}
			return ""
		}
		name := get("name")
		if name == "" {
			return nil, fmt.Errorf("items import: row %d has no product/service name", ri+2)
		}
		typ, err := normalizeItemType(get("type"))
		if err != nil {
			return nil, fmt.Errorf("items import: row %d: %w", ri+2, err)
		}
		ref := map[string]any{}
		if v := get("incomeAccount"); v != "" {
			id, err := resolveNameID(ctx, "Account", v)
			if err != nil {
				return nil, fmt.Errorf("items import: row %d income account %q: %w", ri+2, v, err)
			}
			ref["salesAccountId"] = id
		}
		if v := get("expenseAccount"); v != "" {
			id, err := resolveNameID(ctx, "Account", v)
			if err != nil {
				return nil, fmt.Errorf("items import: row %d expense account %q: %w", ri+2, v, err)
			}
			ref["purchaseAccountId"] = id
		}
		if v := get("salesDescription"); v != "" {
			ref["salesDescription"] = v
		}
		if v := get("purchaseDescription"); v != "" {
			ref["purchaseDescription"] = v
		}
		ref["salesTaxCodeId"] = ""
		ref["purchaseTaxCodeId"] = ""

		price := parseAmount(get("price"))
		cost := parseAmount(get("cost"))
		product := map[string]any{
			"clientMutationId":    "0",
			"name":                name,
			"trackStock":          false,
			"type":                typ,
			"sellable":            "true",
			"taxable":             nil,
			"accountingReference": ref,
			"source":              "PURCHASED",
			"defaultVariantPrice": price,
			"defaultVariantCost":  cost,
			"variants": []any{map[string]any{
				"variabilityValues":       []any{},
				"purchaseRateIncludesTax": false,
				"salesRateIncludesTax":    false,
				"sku":                     get("sku"),
				"price":                   price,
				"cost":                    cost,
				"sellable":                "true",
				"costGroupId":             nil,
				"trackStock":              false,
			}},
			"variabilityDimensions": []any{},
		}
		if v := get("category"); v != "" {
			id, err := resolveNameID(ctx, "Item", v)
			if err != nil {
				return nil, fmt.Errorf("items import: row %d category %q: %w", ri+2, v, err)
			}
			product["categoryId"] = id
		}
		products = append(products, map[string]any{
			"clientMutationId": strconv.Itoa(ri),
			"product":          product,
		})
	}
	if len(products) == 0 {
		return nil, fmt.Errorf("items import: no data rows")
	}

	bulkName := "Products-" + time.Now().Format("20060102150405")
	payload, err := json.Marshal(map[string]any{
		"query": bulkProductCreateDoc,
		"variables": map[string]any{
			"input_0": map[string]any{
				"clientMutationId": "1",
				"qbsharedBulkProductBulkActionProductListInput": map[string]any{
					"mode":     "ASYNC",
					"name":     bulkName,
					"action":   "CREATE",
					"products": products,
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	overrides := map[string]string{
		"Referer":           "https://qbo.intuit.com/app/importdata/Items",
		"intuit-plugin-id":  "batch-transactions-ui",
		"intuit-company-id": ac.realm,
		"content-type":      "application/json",
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, bulkActionURL, bulkActionHost, payload, overrides)
	if err != nil {
		return nil, fmt.Errorf("items import: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("reading items import: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			M struct {
				Result struct {
					ID             string `json:"id"`
					Status         string `json:"status"`
					FailedEntities []struct {
						Errors []struct {
							Message string `json:"message"`
							Code    string `json:"code"`
						} `json:"errors"`
					} `json:"failedEntities"`
					InProgressEntities []any `json:"inProgressEntities"`
				} `json:"qbsharedBulkProductBulkActionProductListInput"`
			} `json:"Qbshared_Bulk_ProductBulkAction_bulkActionOnProductList"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("items import outcome unknown: unreadable bulk-action receipt; inspect before retrying: %w", err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("items import: %s", wrap.Errors[0].Message)
	}
	res0 := wrap.Data.M.Result
	if msgs := bulkFailedMessages(res0.FailedEntities); msgs != "" {
		return nil, fmt.Errorf("items import rejected: %s", msgs)
	}

	// ASYNC mode: the mutation queued the bulk action — poll the list until
	// the row reports terminal status so failures surface honestly.
	if res0.ID == "" {
		return nil, fmt.Errorf("items import outcome unknown: bulk action id missing; inspect before retrying")
	}
	node, err := waitBulkAction(ctx, ac, res0.ID, overrides, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("items import outcome unknown for bulk action %q; inspect before retrying: %w", res0.ID, err)
	}
	if node == nil {
		return nil, fmt.Errorf("items import outcome unknown for bulk action %q; inspect before retrying", res0.ID)
	}
	summary := fmt.Sprintf("%s status=%s ok=%d failed=%d", bulkName, node.Status, node.TotalSuccessful, node.TotalUnsuccessful)
	if !strings.EqualFold(node.Status, "COMPLETED") {
		return nil, fmt.Errorf("items import outcome unknown for bulk action %q; inspect before retrying: expected completed status, got %q", res0.ID, node.Status)
	}
	if node.TotalUnsuccessful > 0 || node.TotalSuccessful != len(products) {
		return nil, fmt.Errorf("items import partially completed for bulk action %q: %d/%d rows reported created, %d failed; inspect before retrying", res0.ID, node.TotalSuccessful, len(products), node.TotalUnsuccessful)
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "items import",
		Entity: "Item",
		Item:   QueryItem{Type: "Item", ID: res0.ID, Name: strconv.Itoa(len(products)) + " rows"},
		Note:   "qbbulkaction bulkActionOnProductList ASYNC — " + summary,
	}, nil
}

type bulkActionNode struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Status            string `json:"status"`
	TotalRequests     int    `json:"totalRequests"`
	TotalSuccessful   int    `json:"totalSuccessful"`
	TotalUnsuccessful int    `json:"totalUnsuccessful"`
}

// waitBulkAction polls transactionBulkActionList until the bulk action
// leaves in-progress state or the deadline passes.
func waitBulkAction(ctx context.Context, ac *apiClient, id string, overrides map[string]string, budget time.Duration) (*bulkActionNode, error) {
	payload, err := json.Marshal(map[string]any{"query": bulkActionListDoc, "variables": map[string]any{}})
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(budget)
	for {
		resp, err := ac.doURIHost(ctx, http.MethodPost, bulkActionURL, bulkActionHost, payload, overrides)
		if err != nil {
			return nil, err
		}
		raw, rerr := readBody(resp)
		_ = drainAndClose(resp)
		if rerr != nil {
			return nil, rerr
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
		}
		var wrap struct {
			Data struct {
				Company struct {
					List struct {
						Edges []struct {
							Node bulkActionNode `json:"node"`
						} `json:"edges"`
					} `json:"transactionBulkActionList"`
				} `json:"company"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return nil, fmt.Errorf("parsing bulk action status: %w", err)
		}
		if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
			return nil, fmt.Errorf("bulk action status: %s", wrap.Errors[0].Message)
		}
		for _, e := range wrap.Data.Company.List.Edges {
			if e.Node.ID == id {
				if !strings.EqualFold(e.Node.Status, "IN_PROGRESS") && !strings.EqualFold(e.Node.Status, "QUEUED") && e.Node.Status != "" {
					return &e.Node, nil
				}
				if e.Node.TotalSuccessful+e.Node.TotalUnsuccessful >= e.Node.TotalRequests && e.Node.TotalRequests > 0 {
					return &e.Node, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("bulk action %q did not reach a terminal status before timeout", id)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func bulkFailedMessages(failed []struct {
	Errors []struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"errors"`
}) string {
	var msgs []string
	for _, f := range failed {
		for _, e := range f.Errors {
			if e.Message != "" {
				msgs = append(msgs, e.Message)
			}
		}
	}
	return strings.Join(msgs, "; ")
}

// normalizeItemType maps wizard type labels to canonical enum values.
// Inventory rows are rejected — their quantity/asset-account shape was not
// captured.
func normalizeItemType(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "service", "services", "service item":
		return "SERVICE", nil
	case "non-inventory", "noninventory", "non inventory", "non_inventory":
		return "NON_INVENTORY", nil
	case "inventory":
		return "", fmt.Errorf("inventory items need quantity/asset columns — not supported by this import (service/non-inventory only)")
	default:
		return "", fmt.Errorf("unknown item type %q (want Service or Non-inventory)", v)
	}
}
