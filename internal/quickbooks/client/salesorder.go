package client

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const salesOrderGraphQLURL = "https://commercecontrol.api.intuit.com/graphql"
const salesOrderHost = "commercecontrol.api.intuit.com"

// Only fields and arguments observed on the native sales-order surface are used.
const getSalesOrdersQuery = `query GetSalesOrders($offset: Int! = 0, $limit: Int! = 100, $filter: SalesOrderFilterCriteria, $sort: [SalesOrderSortCriteria!]) {
 result: getSalesOrders(offset: $offset, limit: $limit, filter: $filter, sort: $sort) {
  salesOrders { id issuedAt transactionDate orderNumber customerId customerName total }
  totalCount __typename
 }
}`

const getSalesOrderQuery = `query GetSalesOrderWithTaxGroup($id: ID!) {
 result: getSalesOrder(id: $id) {
  id version orderNumber orderStatus subtotal total totalTax totalDiscount totalShipping
  issuedAt transactionDate dueDate term customerId customerName customerNote internalNote
  freeformBillingAddress freeformShippingAddress printTemplateId currencyIso
  currencyInfo { symbol homeAmount code exchangeRate name currency }
  attachments { documentId attachableId includeOnSend }
  discount { amount percent isDiscountAfterTax }
  customTaxGroupId referenceNumber email phoneNumber shipVia shipDate trackingNumber customerPONumber
  deposit { requestedAmount paidAmount status }
  department { id } customExtensions { customFields { id values } }
  salesOrderLineItems {
   id variantId description position quantity total taxable type unitPrice
   baseQuantity baseRate unitConversionRatio totalDiscount totalTax isDeleted
   parentItemId parentLineItemId unitId class { id } taxGroup { id }
   links { sources { sourceTxn { id type } } targets { targetTxn { id type } } }
   quantityConsumedInPurchaseOrders quantityConsumedInInvoices quantityConsumedInFulfillmentOrder quantityFulfilled
  }
 }
}`
const createSalesOrderQuery = `mutation CreateSalesOrder($order: CreateSalesOrderInput!) {
 createSalesOrder(input: $order) { id version orderNumber salesOrderLineItems { id position parentLineItemId parentItemId } }
}`
const updateSalesOrderQuery = `mutation UpdateSalesOrder($order: UpdateSalesOrderInput!) {
 updateSalesOrder(input: $order) { id version orderNumber salesOrderLineItems { id position parentLineItemId parentItemId } }
}`
const deleteSalesOrderQuery = `mutation DeleteSalesOrder($id: ID!) {
 deleteSalesOrder(id: $id) { deletedSalesOrderId __typename }
}`

func PlannedSalesOrderURL() string { return salesOrderGraphQLURL }

func salesOrderCall(ctx context.Context, ac *apiClient, operation, query string, variables map[string]any, key string) (map[string]any, error) {
	payload, err := json.Marshal(map[string]any{"operationName": operation, "query": query, "variables": variables})
	if err != nil {
		return nil, fmt.Errorf("sales-order %s: %w", operation, err)
	}
	// The inventory UI uses schema version 2 on this same host. Sales-order
	// operations use its default schema; replaying inventory's version header
	// makes getSalesOrders an unknown field.
	resp, err := ac.doURIHost(ctx, http.MethodPost, salesOrderGraphQLURL, salesOrderHost, payload, map[string]string{"X-Supergraph-Version": ""})
	if err != nil {
		return nil, fmt.Errorf("sales-order %s: %w", operation, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("sales-order %s: %w", operation, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	var envelope struct {
		Errors json.RawMessage            `json:"errors"`
		Data   map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("sales-order %s: malformed GraphQL response: %w", operation, err)
	}
	if len(envelope.Errors) > 0 && string(envelope.Errors) != "null" && string(envelope.Errors) != "[]" {
		return nil, fmt.Errorf("sales-order %s: GraphQL errors: %s", operation, envelope.Errors)
	}
	raw := envelope.Data[key]
	var result map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil || result == nil {
		return nil, fmt.Errorf("sales-order %s: missing or malformed %s result", operation, key)
	}
	return result, nil
}

func salesOrderGet(ctx context.Context, ac *apiClient, id string) (map[string]any, error) {
	row, err := salesOrderCall(ctx, ac, "GetSalesOrderWithTaxGroup", getSalesOrderQuery, map[string]any{"id": id}, "result")
	if err != nil {
		return nil, err
	}
	if salesOrderString(row, "id") != id {
		return nil, fmt.Errorf("sales-order get: missing or mismatched id")
	}
	return row, nil
}

func replayGetSalesOrders(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	id, query = strings.TrimSpace(id), strings.ToLower(strings.TrimSpace(query))
	if id != "" && query != "" {
		return nil, fmt.Errorf("sales-order: id and search query cannot be combined")
	}
	res := &QueryResult{Status: http.StatusOK, Entity: "SalesOrder", Counts: map[string]int{}, Items: []QueryItem{}}
	if id != "" {
		row, err := salesOrderGet(ctx, ac, id)
		if err != nil {
			return nil, err
		}
		item, err := salesOrderItem(row)
		if err != nil {
			return nil, err
		}
		res.Items = append(res.Items, item)
		res.Counts["items"], res.Counts["totalCount"] = 1, 1
		res.Note = "commercecontrol GetSalesOrderWithTaxGroup"
		return res, nil
	}
	seen := map[string]bool{}
	total, matched := -1, 0
	for offset := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		size := 100
		if query == "" && limit > 0 && limit-len(res.Items) < size {
			size = limit - len(res.Items)
		}
		page, err := salesOrderCall(ctx, ac, "GetSalesOrders", getSalesOrdersQuery, map[string]any{"offset": offset, "limit": size, "filter": map[string]any{}, "sort": []any{}}, "result")
		if err != nil {
			return nil, err
		}
		n, err := salesOrderNumber(page["totalCount"], "totalCount")
		if err != nil || n != math.Trunc(n) || n > float64(int(^uint(0)>>1)) {
			return nil, fmt.Errorf("sales-order list: missing or invalid totalCount")
		}
		if total >= 0 && total != int(n) {
			return nil, fmt.Errorf("sales-order list: totalCount changed during pagination; retry")
		}
		total = int(n)
		rows, ok := page["salesOrders"].([]any)
		if !ok || len(rows) > size || offset+len(rows) > total || (len(rows) == 0 && offset < total) {
			return nil, fmt.Errorf("sales-order list: missing rows or inconsistent pagination")
		}
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("sales-order list: malformed row")
			}
			item, err := salesOrderItem(row)
			if err != nil {
				return nil, err
			}
			if seen[item.ID] {
				return nil, fmt.Errorf("sales-order list: repeated id during pagination; retry")
			}
			seen[item.ID] = true
			if query == "" || strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.DocNumber), query) || strings.Contains(strings.ToLower(item.ID), query) {
				matched++
				if limit < 1 || len(res.Items) < limit {
					res.Items = append(res.Items, item)
				}
			}
		}
		offset += len(rows)
		if offset >= total || (query == "" && limit > 0 && len(res.Items) >= limit) {
			break
		}
	}
	res.Counts["items"], res.Counts["totalCount"] = len(res.Items), total
	res.Note = fmt.Sprintf("commercecontrol GetSalesOrders totalCount=%d", total)
	if query != "" {
		res.Counts["matched"] = matched
		res.Note += "; searched all pages locally"
	}
	return res, nil
}

func salesOrderString(row map[string]any, key string) string { v, _ := row[key].(string); return v }

func salesOrderNumber(v any, field string) (float64, error) {
	var s string
	switch n := v.(type) {
	case json.Number:
		s = n.String()
	case float64:
		s = strconv.FormatFloat(n, 'g', -1, 64)
	default:
		return 0, fmt.Errorf("sales-order %s must be a JSON number", field)
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 9007199254740991 {
		return 0, fmt.Errorf("sales-order %s must be a finite non-negative safe number", field)
	}
	original, ok := new(big.Rat).SetString(s)
	rounded, roundedOK := new(big.Rat).SetString(strconv.FormatFloat(n, 'g', -1, 64))
	if !ok || !roundedOK || original.Cmp(rounded) != 0 {
		return 0, fmt.Errorf("sales-order %s loses numeric precision", field)
	}
	return n, nil
}

func salesOrderItem(row map[string]any) (QueryItem, error) {
	id := salesOrderString(row, "id")
	if strings.TrimSpace(id) == "" {
		return QueryItem{}, fmt.Errorf("sales-order: row missing id")
	}
	amount, err := salesOrderNumber(row["total"], "total")
	if err != nil {
		return QueryItem{}, err
	}
	for _, k := range []string{"customerName", "orderNumber", "transactionDate", "issuedAt"} {
		if v, exists := row[k]; exists && v != nil {
			if _, ok := v.(string); !ok {
				return QueryItem{}, fmt.Errorf("sales-order: malformed %s", k)
			}
		}
	}
	date := salesOrderString(row, "transactionDate")
	if date == "" {
		date = salesOrderString(row, "issuedAt")
	}
	return QueryItem{ID: id, Name: salesOrderString(row, "customerName"), DocNumber: salesOrderString(row, "orderNumber"), Amount: amount, Date: date, Type: "SalesOrder"}, nil
}

// Aliases are checked for conflicting values, rather than silently selecting one.
var salesOrderFlagFields = map[string]string{
	"customer": "customerId", "customer-id": "customerId",
	"date": "transactionDate", "txn-date": "transactionDate", "order-date": "transactionDate", "sales-order-date": "transactionDate",
	"due-date": "dueDate", "issued-at": "issuedAt", "term": "term", "terms": "term",
	"memo": "customerNote", "message": "customerNote", "customer-note": "customerNote",
	"order-number": "orderNumber", "order-no": "orderNumber", "doc-number": "orderNumber",
	"currency": "currencyIso", "currency-iso": "currencyIso", "print-template-id": "printTemplateId",
	"billing-address": "freeformBillingAddress", "shipping-address": "freeformShippingAddress",
	"line-items": "lineItems", "lines": "lineItems", "id": "id",
	"ignore-duplicate-order-num": "ignoreDuplicateOrderNum",
}

func salesOrderFlags(op, id string, flags map[string]string) (map[string]any, error) {
	out := map[string]any{}
	for flag, value := range flags {
		field, ok := salesOrderFlagFields[flag]
		if !ok {
			return nil, fmt.Errorf("sales-order: unsupported --%s", flag)
		}
		if op == "delete" && field != "id" {
			return nil, fmt.Errorf("sales-order delete: unsupported --%s", flag)
		}
		if field == "id" {
			if strings.TrimSpace(value) != id || op == "create" {
				return nil, fmt.Errorf("sales-order: conflicting --id")
			}
			continue
		}
		var v any = value
		switch field {
		case "transactionDate", "dueDate":
			value = normalizeDate(value)
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return nil, fmt.Errorf("sales-order --%s: invalid date", flag)
			}
			v = value
		case "issuedAt":
			t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("sales-order --issued-at must be RFC3339")
			}
			v = t.UTC().Format(time.RFC3339Nano)
		case "ignoreDuplicateOrderNum":
			if value != "true" && value != "false" {
				return nil, fmt.Errorf("sales-order --%s must be true or false", flag)
			}
			v = value == "true"
		case "currencyIso":
			value = strings.TrimSpace(value)
			if len(value) != 3 || strings.Trim(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
				return nil, fmt.Errorf("sales-order currency must be an uppercase ISO code")
			}
			v = value
		case "customerId", "term", "printTemplateId":
			value = strings.TrimSpace(value)
			if value == "" || sanitizeToken(value) != value {
				return nil, fmt.Errorf("sales-order --%s requires an id", flag)
			}
			v = value
		case "lineItems":
			// Parse before any reads; resolution of item types happens after validation.
			lines, err := salesOrderParseLines(value, op == "create")
			if err != nil {
				return nil, err
			}
			v = lines
		}
		if prior, exists := out[field]; exists {
			a, _ := json.Marshal(prior)
			b, _ := json.Marshal(v)
			if string(a) != string(b) {
				return nil, fmt.Errorf("sales-order: conflicting aliases for %s", field)
			}
		}
		out[field] = v
	}
	if op == "create" {
		for _, required := range []struct{ field, flag string }{{"customerId", "customer"}, {"transactionDate", "date"}, {"currencyIso", "currency"}, {"lineItems", "line-items"}} {
			if _, ok := out[required.field]; !ok {
				return nil, fmt.Errorf("sales-order create requires --%s", required.flag)
			}
		}
	}
	if op == "update" && len(out) == 0 {
		return nil, fmt.Errorf("sales-order update requires at least one changed field")
	}
	return out, nil
}

func salesOrderOnlyKeys(m map[string]any, allowed ...string) error {
	for k := range m {
		if !slices.Contains(allowed, k) {
			return fmt.Errorf("sales-order: unsupported line field %s", k)
		}
	}
	return nil
}

func salesOrderParseLines(raw string, creating bool) ([]map[string]any, error) {
	var lines []map[string]any
	if !json.Valid([]byte(raw)) {
		return nil, ErrCSVLineItems
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&lines); err != nil || len(lines) == 0 {
		return nil, ErrCSVLineItems
	}
	out := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		if err := salesOrderOnlyKeys(line, "Id", "DetailType", "SalesItemLineDetail", "Amount", "Description"); err != nil {
			return nil, err
		}
		if line == nil {
			return nil, fmt.Errorf("sales-order line %d: null line", i+1)
		}
		if (creating || line["DetailType"] != nil) && line["DetailType"] != "SalesItemLineDetail" {
			return nil, fmt.Errorf("sales-order line %d requires SalesItemLineDetail", i+1)
		}
		det, ok := line["SalesItemLineDetail"].(map[string]any)
		if !ok && (creating || line["SalesItemLineDetail"] != nil) {
			return nil, fmt.Errorf("sales-order line %d: invalid detail", i+1)
		}
		if err := salesOrderOnlyKeys(det, "ItemRef", "Qty", "UnitPrice"); err != nil {
			return nil, err
		}
		native := map[string]any{}
		if refValue, exists := det["ItemRef"]; exists || creating {
			ref, ok := refValue.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("sales-order line %d: ItemRef.value required", i+1)
			}
			if err := salesOrderOnlyKeys(ref, "value"); err != nil {
				return nil, err
			}
			item := salesOrderString(ref, "value")
			if item == "" || sanitizeToken(item) != item {
				return nil, fmt.Errorf("sales-order line %d: ItemRef.value requires an id", i+1)
			}
			native["itemId"] = item
		}
		for _, pair := range [][2]string{{"Qty", "quantity"}, {"UnitPrice", "unitPrice"}, {"Amount", "total"}} {
			source := det
			if pair[0] == "Amount" {
				source = line
			}
			value, exists := source[pair[0]]
			if !exists && !creating {
				continue
			}
			n, err := salesOrderNumber(value, pair[0])
			if err != nil {
				return nil, err
			}
			if pair[0] == "Qty" && n == 0 {
				return nil, fmt.Errorf("sales-order Qty must be positive")
			}
			native[pair[1]] = n
		}
		if creating {
			if !salesOrderSameAmount(native["quantity"].(float64)*native["unitPrice"].(float64), native["total"].(float64)) {
				return nil, fmt.Errorf("sales-order line %d: Amount must equal Qty * UnitPrice; tax/rounding adjustments are unsupported", i+1)
			}
		}
		for _, pair := range [][2]string{{"Id", "id"}, {"Description", "description"}} {
			if v, exists := line[pair[0]]; exists {
				s, ok := v.(string)
				if !ok || (pair[0] == "Id" && strings.TrimSpace(s) == "") {
					return nil, fmt.Errorf("sales-order line %d: invalid %s", i+1, pair[0])
				}
				native[pair[1]] = s
			}
		}
		out = append(out, native)
	}
	return out, nil
}

func salesOrderSameAmount(a, b float64) bool {
	return !math.IsNaN(a) && !math.IsInf(a, 0) && math.Abs(a-b) <= math.Max(1, math.Max(math.Abs(a), math.Abs(b)))*1e-14
}

// ReplaySalesOrderMutate uses the native commercecontrol mutations. It never
// writes an invoice or invents an item, account, tax treatment, or line ID.
func ReplaySalesOrderMutate(ctx context.Context, op, id string, flags map[string]string) (*MutateResult, error) {
	op, id = strings.ToLower(strings.TrimSpace(op)), strings.TrimSpace(id)
	if op != "create" && op != "update" && op != "delete" {
		return nil, fmt.Errorf("sales-order: unsupported operation %q", op)
	}
	if op != "create" && id == "" {
		return nil, ErrMissingMutateID
	}
	if op == "create" && id != "" {
		return nil, fmt.Errorf("sales-order create does not accept an id")
	}
	changes, err := salesOrderFlags(op, id, flags)
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if op == "delete" {
		ack, err := salesOrderCall(ctx, ac, "DeleteSalesOrder", deleteSalesOrderQuery, map[string]any{"id": id}, "deleteSalesOrder")
		if err != nil {
			return nil, err
		}
		if salesOrderString(ack, "deletedSalesOrderId") != id {
			return nil, fmt.Errorf("sales-order delete: missing or mismatched deletedSalesOrderId acknowledgement")
		}
		return &MutateResult{Status: http.StatusOK, Op: op, Entity: "SalesOrder", Item: QueryItem{ID: id, Type: "SalesOrder"}, Note: "commercecontrol DeleteSalesOrder"}, nil
	}
	order := map[string]any{}
	var existing []map[string]any
	if op == "update" {
		row, err := salesOrderGet(ctx, ac, id)
		if err != nil {
			return nil, err
		}
		order, existing, err = salesOrderEditable(row)
		if err != nil {
			return nil, err
		}
		order["id"] = id
	} else {
		order["attachments"] = []any{}
		order["totalTax"] = 0
	}
	requested, _ := changes["lineItems"].([]map[string]any)
	delete(changes, "lineItems")
	maps.Copy(order, changes)
	if date, changed := changes["transactionDate"]; changed {
		if _, explicit := changes["issuedAt"]; !explicit {
			order["issuedAt"] = date.(string) + "T00:00:00Z"
		}
	}
	if _, ok := order["ignoreDuplicateOrderNum"]; !ok {
		order["ignoreDuplicateOrderNum"] = op == "update"
	}
	lines, err := salesOrderResolveLines(ctx, ac, requested, existing, op == "create")
	if err != nil {
		return nil, err
	}
	order["lineItems"] = lines
	// Sum decimal totals exactly so two valid money values do not produce an
	// extra binary-floating-point digit in the outgoing order total.
	sum := new(big.Rat)
	for _, line := range lines {
		amount, err := salesOrderNumber(line["total"], "line total")
		if err != nil {
			return nil, err
		}
		decimal, _ := new(big.Rat).SetString(strconv.FormatFloat(amount, 'g', -1, 64))
		sum.Add(sum, decimal)
	}
	total, _ := sum.Float64()
	if _, err := salesOrderNumber(total, "subtotal"); err != nil {
		return nil, err
	}
	order["subtotal"], order["total"] = total, total
	operation, query, key := "CreateSalesOrder", createSalesOrderQuery, "createSalesOrder"
	if op == "update" {
		operation, query, key = "UpdateSalesOrder", updateSalesOrderQuery, "updateSalesOrder"
	}
	ack, err := salesOrderCall(ctx, ac, operation, query, map[string]any{"order": order}, key)
	if err != nil {
		return nil, err
	}
	ackID := salesOrderString(ack, "id")
	if ackID == "" || (id != "" && ackID != id) {
		return nil, fmt.Errorf("sales-order %s: missing or mismatched id acknowledgement", op)
	}
	version, err := salesOrderNumber(ack["version"], "version")
	if err != nil || version < 1 || version != math.Trunc(version) {
		return nil, fmt.Errorf("sales-order %s: missing version acknowledgement", op)
	}
	ackLines, ok := ack["salesOrderLineItems"].([]any)
	if !ok || len(ackLines) != len(lines) {
		return nil, fmt.Errorf("sales-order %s: missing line acknowledgement", op)
	}
	ackIDs := map[string]bool{}
	for i, raw := range ackLines {
		line, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("sales-order %s: malformed line acknowledgement", op)
		}
		lineID := salesOrderString(line, "id")
		pos, err := salesOrderNumber(line["position"], "position")
		if err != nil || pos != float64(i) || lineID == "" || ackIDs[lineID] || (lines[i]["id"] != nil && lines[i]["id"] != lineID) {
			return nil, fmt.Errorf("sales-order %s: missing or mismatched line acknowledgement", op)
		}
		ackIDs[lineID] = true
	}
	doc, ok := ack["orderNumber"].(string)
	if !ok || (order["orderNumber"] != nil && order["orderNumber"] != doc) {
		return nil, fmt.Errorf("sales-order %s: missing or mismatched orderNumber acknowledgement", op)
	}
	return &MutateResult{Status: http.StatusOK, Entity: "SalesOrder", Op: op, Item: QueryItem{ID: ackID, DocNumber: doc, Date: salesOrderString(order, "transactionDate"), Amount: total, Type: "SalesOrder"}, Note: "commercecontrol " + operation}, nil
}

func salesOrderZero(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case bool:
		return !x
	case map[string]any:
		for _, entry := range x {
			if !salesOrderZero(entry) {
				return false
			}
		}
		return true
	case []any:
		return len(x) == 0
	default:
		n, err := salesOrderNumber(v, "existing value")
		return err == nil && n == 0
	}
}

func salesOrderEditable(row map[string]any) (map[string]any, []map[string]any, error) {
	// Fields without an observed input mapping cannot safely be dropped on update.
	for _, key := range []string{"totalTax", "totalDiscount", "totalShipping", "discount", "customTaxGroupId", "referenceNumber", "email", "phoneNumber", "shipVia", "shipDate", "trackingNumber", "customerPONumber", "deposit", "department", "customExtensions", "internalNote"} {
		if !salesOrderZero(row[key]) {
			return nil, nil, fmt.Errorf("sales-order update: existing %s is unsupported; refusing to discard it", key)
		}
	}
	for _, key := range []string{"totalTax", "totalDiscount", "totalShipping"} {
		if _, err := salesOrderNumber(row[key], key); err != nil {
			return nil, nil, err
		}
	}
	order := map[string]any{}
	for _, key := range []string{"customerId", "customerNote", "freeformBillingAddress", "freeformShippingAddress", "issuedAt", "transactionDate", "term", "dueDate", "orderNumber", "currencyIso", "printTemplateId", "currencyInfo", "attachments"} {
		if value, ok := row[key]; ok && value != nil {
			order[key] = value
		}
	}
	for key, value := range order {
		if key != "currencyInfo" && key != "attachments" {
			if _, ok := value.(string); !ok {
				return nil, nil, fmt.Errorf("sales-order update: malformed %s", key)
			}
		}
	}
	for _, key := range []string{"customerId", "transactionDate", "currencyIso"} {
		if salesOrderString(order, key) == "" {
			return nil, nil, fmt.Errorf("sales-order update: fetched order missing %s", key)
		}
	}
	rawLines, ok := row["salesOrderLineItems"].([]any)
	if !ok || len(rawLines) == 0 {
		return nil, nil, fmt.Errorf("sales-order update: missing native lines")
	}
	lines := make([]map[string]any, 0, len(rawLines))
	ids := map[string]bool{}
	sum := 0.0
	for i, raw := range rawLines {
		line, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("sales-order update: malformed native line")
		}
		for _, key := range []string{"taxable", "totalTax", "totalDiscount", "taxGroup", "isDeleted", "parentItemId", "parentLineItemId", "unitId", "class", "links", "quantityConsumedInPurchaseOrders", "quantityConsumedInInvoices", "quantityConsumedInFulfillmentOrder", "quantityFulfilled"} {
			if !salesOrderZero(line[key]) {
				return nil, nil, fmt.Errorf("sales-order update: existing line %s is unsupported", key)
			}
		}
		if taxable, ok := line["taxable"].(bool); !ok || taxable {
			return nil, nil, fmt.Errorf("sales-order update: missing or unsupported taxable flag")
		}
		for _, key := range []string{"totalTax", "totalDiscount"} {
			if _, err := salesOrderNumber(line[key], key); err != nil {
				return nil, nil, err
			}
		}
		id, item, typ := salesOrderString(line, "id"), salesOrderString(line, "variantId"), salesOrderString(line, "type")
		if id == "" || ids[id] || item == "" || typ == "" {
			return nil, nil, fmt.Errorf("sales-order update: missing or duplicate native line identity")
		}
		ids[id] = true
		for _, key := range []string{"quantity", "unitPrice", "total", "position"} {
			if _, err := salesOrderNumber(line[key], key); err != nil {
				return nil, nil, err
			}
		}
		pos, _ := salesOrderNumber(line["position"], "position")
		if pos != float64(i) {
			return nil, nil, fmt.Errorf("sales-order update: unsupported native line positions")
		}
		qty, _ := salesOrderNumber(line["quantity"], "quantity")
		price, _ := salesOrderNumber(line["unitPrice"], "unitPrice")
		amount, _ := salesOrderNumber(line["total"], "total")
		if qty == 0 || !salesOrderSameAmount(qty*price, amount) {
			return nil, nil, fmt.Errorf("sales-order update: unsupported native line arithmetic")
		}
		sum += amount
		native := map[string]any{"id": id, "itemId": item, "type": typ, "taxable": false, "taxGroup": map[string]any{}}
		for _, key := range []string{"description", "position", "quantity", "total", "unitPrice", "baseQuantity", "baseRate", "unitConversionRatio"} {
			if v, ok := line[key]; ok && v != nil {
				native[key] = v
			}
		}
		lines = append(lines, native)
	}
	for _, key := range []string{"subtotal", "total"} {
		amount, err := salesOrderNumber(row[key], key)
		if err != nil || !salesOrderSameAmount(amount, sum) {
			return nil, nil, fmt.Errorf("sales-order update: totals do not match native lines")
		}
	}
	return order, lines, nil
}

func salesOrderResolveLines(ctx context.Context, ac *apiClient, requested, existing []map[string]any, creating bool) ([]map[string]any, error) {
	if requested == nil {
		return existing, nil
	}
	if !creating && len(requested) != len(existing) {
		return nil, fmt.Errorf("sales-order update: adding/removing lines requires a verified native contract")
	}
	byID := map[string]map[string]any{}
	for _, line := range existing {
		byID[salesOrderString(line, "id")] = line
	}
	used := map[string]bool{}
	types := map[string]string{}
	result := make([]map[string]any, 0, len(requested))
	for i, input := range requested {
		native := map[string]any{}
		oldID := salesOrderString(input, "id")
		if creating && oldID != "" {
			return nil, fmt.Errorf("sales-order create: native line Id must be omitted")
		}
		if !creating {
			var old map[string]any
			if oldID != "" {
				old = byID[oldID]
			} else {
				if len(existing) > 1 {
					return nil, fmt.Errorf("sales-order update: explicit line Id required for multiple lines")
				}
				old = existing[i]
			}
			if old == nil || used[salesOrderString(old, "id")] {
				return nil, fmt.Errorf("sales-order update: unknown or ambiguous line Id")
			}
			used[salesOrderString(old, "id")] = true
			if input["itemId"] != nil && old["itemId"] != input["itemId"] {
				return nil, fmt.Errorf("sales-order update: replacing a referenced item requires a verified native contract")
			}
			maps.Copy(native, old)
		} else {
			native["id"], native["taxable"], native["taxGroup"] = nil, false, map[string]any{}
			item := salesOrderString(input, "itemId")
			typ := types[item]
			if typ == "" {
				obj, err := fetchV3(ctx, ac, "item", item)
				if err != nil {
					return nil, fmt.Errorf("sales-order resolve item: %w", err)
				}
				if salesOrderString(obj, "Id") != item {
					return nil, fmt.Errorf("sales-order resolve item: mismatched Item id")
				}
				// SERVICE is the only item-type mapping established by the captured write.
				if salesOrderString(obj, "Type") != "Service" {
					return nil, fmt.Errorf("sales-order: Item %s type %q has no verified native mapping", item, salesOrderString(obj, "Type"))
				}
				typ = "SERVICE"
				types[item] = typ
			}
			native["type"] = typ
		}
		maps.Copy(native, input)
		qty, err := salesOrderNumber(native["quantity"], "quantity")
		if err != nil {
			return nil, err
		}
		price, err := salesOrderNumber(native["unitPrice"], "unitPrice")
		if err != nil {
			return nil, err
		}
		if !creating && input["total"] == nil && (input["quantity"] != nil || input["unitPrice"] != nil) {
			q, _ := new(big.Rat).SetString(strconv.FormatFloat(qty, 'g', -1, 64))
			rate, _ := new(big.Rat).SetString(strconv.FormatFloat(price, 'g', -1, 64))
			amount, _ := new(big.Rat).Mul(q, rate).Float64()
			native["total"] = amount
		}
		amount, err := salesOrderNumber(native["total"], "total")
		if err != nil {
			return nil, err
		}
		if qty == 0 || !salesOrderSameAmount(qty*price, amount) {
			return nil, fmt.Errorf("sales-order: Amount must equal Qty * UnitPrice; tax/rounding adjustments are unsupported")
		}
		native["position"] = i
		result = append(result, native)
	}
	return result, nil
}
