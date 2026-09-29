package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const itemReceiptGraphQLURL = "https://warehouse-management-svc.api.intuit.com/graphql"
const itemReceiptHost = "warehouse-management-svc.api.intuit.com"

// Single-receipt read; captured from /app/itemreceipt?txnId=<id> load on
// Test Company 2 (2026-09-16). The response carries every field needed to
// rebuild a WarehouseManagement_UpdateItemReceiptInput.
const singleItemReceiptQuery = `query WarehouseManagementItemReceipt($id: ID!) {
  warehouseManagementItemReceipt(id: $id) {
    id
    txnDate
    referenceNo
    status
    totalAmount
    memo
    version
    vendor { id name __typename }
    currency { code currencyCode exchangeRate __typename }
    lines {
      id
      item { id __typename }
      status
      lineOrder
      expectedQuantity
      receivedQuantity
      rejectedQuantity
      description
      cost
      amount
      sourceLinks { id transactionId transactionLineId transactionType quantityConsumed __typename }
      targetLinks { id transactionId transactionLineId transactionType quantityConsumed __typename }
      customExtensions { dimensions { definition { id } value __typename } __typename }
      __typename
    }
    customExtensions { customFields { definition { id } value __typename } __typename }
    __typename
  }
}`

// Captured verbatim from the /app/itemreceipt save on Test Company 2
// (2026-09-16); payload trimmed to the identity fields the CLI prints.
const createItemReceiptMutation = `mutation CreateItemReceipt($input: WarehouseManagement_CreateItemReceiptInput!) {
  warehouseManagementCreateItemReceipt(input: $input) {
    ... on WarehouseManagement_CreateItemReceiptPayload {
      itemReceipt { id txnDate referenceNo status totalAmount version vendor { id name __typename } __typename }
      __typename
    }
    ... on WarehouseManagement_ItemReceiptError { message __typename }
    __typename
  }
}`

// Update mirrors create plus id+version; proven live on Test Company 2
// 2026-09-16 (receipt 1823000002 totalAmount 30 -> 40, version 0 -> 1).
const updateItemReceiptMutation = `mutation UpdateItemReceipt($input: WarehouseManagement_UpdateItemReceiptInput!) {
  warehouseManagementUpdateItemReceipt(input: $input) {
    ... on WarehouseManagement_UpdateItemReceiptPayload {
      itemReceipt { id txnDate referenceNo status totalAmount version vendor { id name __typename } __typename }
      __typename
    }
    ... on WarehouseManagement_ItemReceiptError { message __typename }
    __typename
  }
}`

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

func replayGetItemReceipts(ctx context.Context, id, _ string, limit int) (*QueryResult, error) {
	if strings.TrimSpace(id) != "" {
		return replayGetItemReceipt(ctx, id)
	}
	if limit < 1 {
		limit = 99999 // unbounded ask; service applies its own ceiling
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
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
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
				Nodes      []json.RawMessage `json:"nodes"`
				Edges      []json.RawMessage `json:"edges"`
				TotalCount int               `json:"totalCount"`
				Typename   string            `json:"__typename"`
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
	// The connection's known nodes array carries the full receipt fields;
	// edge nodes (id only) are the fallback when nodes is absent.
	raws := wrap.Data.Result.Nodes
	if len(raws) == 0 {
		raws = gqlEdgeNodes(wrap.Data.Result.Edges)
	}
	items := projectJSONItems("ItemReceipt", raws, itemReceiptItemKeys)
	return &QueryResult{
		Entity: "ItemReceipt",
		Counts: map[string]int{"items": len(items), "totalCount": total},
		Items:  items,
		Note:   note,
	}
}

// replayGetItemReceipt fetches one receipt by warehouse id.
func replayGetItemReceipt(ctx context.Context, id string) (*QueryResult, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": "WarehouseManagementItemReceipt",
		"variables":     map[string]any{"id": strings.TrimSpace(id)},
		"query":         singleItemReceiptQuery,
	})
	if err != nil {
		return nil, err
	}
	raw, note, err := postWarehouseOp(ctx, payload, "WarehouseManagementItemReceipt")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data struct {
			Result json.RawMessage `json:"warehouseManagementItemReceipt"`
		} `json:"data"`
	}
	res := &QueryResult{Entity: "ItemReceipt", Status: http.StatusOK, Note: note}
	if err := json.Unmarshal(raw, &wrap); err != nil || len(wrap.Data.Result) == 0 || string(wrap.Data.Result) == "null" {
		res.Counts = map[string]int{"items": 0}
		res.Items = []QueryItem{}
		return res, nil
	}
	items := projectJSONItems("ItemReceipt", []json.RawMessage{wrap.Data.Result}, itemReceiptItemKeys)
	res.Counts = map[string]int{"items": len(items)}
	res.Items = items
	return res, nil
}

// postWarehouseOp sends one GraphQL op body to the warehouse host and
// surfaces a WarehouseManagement_ItemReceiptError union as an error.
func postWarehouseOp(ctx context.Context, payload []byte, op string) ([]byte, string, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, "", err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, itemReceiptGraphQLURL, itemReceiptHost, payload)
	if err != nil {
		return nil, "", fmt.Errorf("warehouse %s: %w", op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, "", fmt.Errorf("reading warehouse %s: %w", op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, "", fmt.Errorf("warehouse %s: %s", op, wrap.Errors[0].Message)
	}
	return raw, "warehouse-management-svc " + op, nil
}

// itemReceiptLine builds one WarehouseManagement receipt line for create.
func itemReceiptLine(itemID string, qty, rate float64) map[string]any {
	return map[string]any{
		"itemId":           itemID,
		"status":           "RECEIVED",
		"expectedQuantity": 0,
		"receivedQuantity": qty,
		"rejectedQuantity": 0,
		"cost":             rate,
		"lineOrder":        1,
		"amount":           qty * rate,
		"description":      "",
		"sourceLinks":      []any{},
		"targetLinks":      []any{},
		"customExtensions": map[string]any{},
	}
}

// ReplayItemReceiptCreate posts CreateItemReceipt for a single-line
// standalone receipt: vendor + item + received qty + unit rate.
func ReplayItemReceiptCreate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	vendorID := strings.TrimSpace(flags["vendor-id"])
	itemID := strings.TrimSpace(flags["item-id"])
	if vendorID == "" || itemID == "" {
		return nil, fmt.Errorf("item-receipt create requires --vendor-id and --item-id")
	}
	qty, qset, err := numericFlag(flags, "qty")
	if err != nil {
		return nil, err
	}
	rate, rset, err := numericFlag(flags, "rate")
	if err != nil {
		return nil, err
	}
	if !qset || !rset {
		return nil, fmt.Errorf("item-receipt create requires --qty and --rate")
	}
	txnDate := normalizeDate(flags["date"])
	if txnDate == "" {
		txnDate = time.Now().Format("2006-01-02")
	}
	input := map[string]any{
		"txnDate":            txnDate + "T00:00:00.000Z",
		"referenceNo":        strings.TrimSpace(flags["ref-no"]),
		"status":             "RECEIVED",
		"memo":               flags["memo"],
		"attachments":        []any{},
		"lines":              []any{itemReceiptLine(itemID, qty, rate)},
		"vendorId":           vendorID,
		"totalAmount":        qty * rate,
		"closedBookPassword": nil,
		"customExtensions":   map[string]any{"customFields": []any{}},
	}
	if cur := strings.TrimSpace(flags["currency"]); cur != "" {
		cur = strings.ToUpper(cur)
		input["currency"] = map[string]any{"code": cur, "currencyCode": cur, "exchangeRate": "1.00"}
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "CreateItemReceipt",
		"variables":     map[string]any{"input": input},
		"query":         createItemReceiptMutation,
	})
	if err != nil {
		return nil, err
	}
	return itemReceiptMutate(ctx, payload, "CreateItemReceipt", "create")
}

// ReplayItemReceiptUpdate fetches the current receipt, applies flag
// overrides (memo, ref-no, date, first-line qty/rate), and posts
// UpdateItemReceipt with the bumped version the server requires.
func ReplayItemReceiptUpdate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	id := strings.TrimSpace(flags["id"])
	if id == "" {
		return nil, fmt.Errorf("item-receipt update requires --id")
	}
	cur, err := fetchItemReceiptInput(ctx, id)
	if err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(flags["memo"]); v != "" {
		cur["memo"] = v
	}
	if v := strings.TrimSpace(flags["ref-no"]); v != "" {
		cur["referenceNo"] = v
	}
	if v := normalizeDate(flags["date"]); v != "" {
		cur["txnDate"] = v + "T00:00:00.000Z"
	}
	qty, qset, err := numericFlag(flags, "qty")
	if err != nil {
		return nil, err
	}
	rate, rset, err := numericFlag(flags, "rate")
	if err != nil {
		return nil, err
	}
	if qset || rset {
		lines, _ := cur["lines"].([]map[string]any)
		if len(lines) == 0 {
			return nil, fmt.Errorf("item-receipt %s has no lines to edit", id)
		}
		line := lines[0]
		q, _ := line["receivedQuantity"].(float64)
		c, _ := line["cost"].(float64)
		if qset {
			q = qty
		}
		if rset {
			c = rate
		}
		line["receivedQuantity"] = q
		line["cost"] = c
		line["amount"] = q * c
		cur["totalAmount"] = q * c
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "UpdateItemReceipt",
		"variables":     map[string]any{"input": cur},
		"query":         updateItemReceiptMutation,
	})
	if err != nil {
		return nil, err
	}
	return itemReceiptMutate(ctx, payload, "UpdateItemReceipt", "update")
}

// fetchItemReceiptInput reads a receipt and reshapes it into a
// WarehouseManagement_UpdateItemReceiptInput: id+version added, line
// items flattened to itemId, link nodes stripped of server-only fields.
func fetchItemReceiptInput(ctx context.Context, id string) (map[string]any, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": "WarehouseManagementItemReceipt",
		"variables":     map[string]any{"id": id},
		"query":         singleItemReceiptQuery,
	})
	if err != nil {
		return nil, err
	}
	raw, _, err := postWarehouseOp(ctx, payload, "WarehouseManagementItemReceipt")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data struct {
			Result map[string]any `json:"warehouseManagementItemReceipt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("parsing item-receipt %s: %w", id, err)
	}
	r := wrap.Data.Result
	if len(r) == 0 {
		return nil, fmt.Errorf("item-receipt %s not found", id)
	}
	input := map[string]any{
		"id":                 r["id"],
		"version":            r["version"],
		"txnDate":            r["txnDate"],
		"referenceNo":        r["referenceNo"],
		"status":             r["status"],
		"memo":               r["memo"],
		"totalAmount":        r["totalAmount"],
		"closedBookPassword": nil,
		"attachments":        []any{},
		"customExtensions":   map[string]any{"customFields": []any{}},
	}
	if v, ok := r["vendor"].(map[string]any); ok {
		input["vendorId"] = v["id"]
	}
	if c, ok := r["currency"].(map[string]any); ok && c != nil {
		input["currency"] = c
	}
	rawLines, _ := r["lines"].([]any)
	lines := make([]map[string]any, 0, len(rawLines))
	for _, rl := range rawLines {
		lm, _ := rl.(map[string]any)
		line := map[string]any{
			"id":               lm["id"],
			"status":           lm["status"],
			"lineOrder":        lm["lineOrder"],
			"expectedQuantity": lm["expectedQuantity"],
			"receivedQuantity": lm["receivedQuantity"],
			"rejectedQuantity": lm["rejectedQuantity"],
			"description":      lm["description"],
			"cost":             lm["cost"],
			"amount":           lm["amount"],
			"customExtensions": lm["customExtensions"],
			"sourceLinks":      itemReceiptLinks(lm["sourceLinks"]),
			"targetLinks":      itemReceiptLinks(lm["targetLinks"]),
		}
		if it, ok := lm["item"].(map[string]any); ok {
			line["itemId"] = it["id"]
		}
		lines = append(lines, line)
	}
	input["lines"] = lines
	return input, nil
}

// itemReceiptLinks strips server-only link fields (id, __typename) so the
// array can round-trip into an update input.
func itemReceiptLinks(v any) []any {
	arr, _ := v.([]any)
	out := make([]any, 0, len(arr))
	for _, e := range arr {
		m, _ := e.(map[string]any)
		out = append(out, map[string]any{
			"transactionId":     m["transactionId"],
			"transactionLineId": m["transactionLineId"],
			"transactionType":   m["transactionType"],
			"quantityConsumed":  m["quantityConsumed"],
		})
	}
	return out
}

// itemReceiptMutate posts a create/update mutation and projects the
// returned itemReceipt into a MutateResult.
func itemReceiptMutate(ctx context.Context, payload []byte, op, verb string) (*MutateResult, error) {
	raw, note, err := postWarehouseOp(ctx, payload, op)
	if err != nil {
		return nil, err
	}
	field := "warehouseManagement" + op
	var wrap struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("parsing item-receipt %s: %w", op, err)
	}
	var payloadWrap struct {
		Receipt  json.RawMessage `json:"itemReceipt"`
		Message  string          `json:"message"`
		Typename string          `json:"__typename"`
	}
	if err := json.Unmarshal(wrap.Data[field], &payloadWrap); err != nil {
		return nil, fmt.Errorf("parsing item-receipt %s payload: %w", op, err)
	}
	if strings.HasSuffix(payloadWrap.Typename, "ItemReceiptError") && payloadWrap.Message != "" {
		return nil, fmt.Errorf("warehouse %s: %s", op, payloadWrap.Message)
	}
	items := projectJSONItems("ItemReceipt", []json.RawMessage{payloadWrap.Receipt}, itemReceiptItemKeys)
	res := &MutateResult{Status: http.StatusOK, Op: verb, Entity: "ItemReceipt", Note: note}
	if len(items) > 0 {
		res.Item = items[0]
	}
	return res, nil
}

// itemReceiptItemKeys maps warehouseManagementItemReceipts nodes onto
// QueryItem; the vendor {id,name} reference fills AccountID/Account.
var itemReceiptItemKeys = itemKeys{
	id:        []string{"id", "Id"},
	name:      []string{"memo", "name", "Name"},
	date:      []string{"txnDate", "TxnDate", "date"},
	docNumber: []string{"referenceNo", "docNumber", "DocNumber"},
	typeOf:    []string{"status", "Status"},
	refs:      []string{"vendor"},
}
