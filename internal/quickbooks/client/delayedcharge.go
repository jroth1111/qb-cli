package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// DELAYED_CHARGE_CREATE / DELAYED_CREDIT_CREATE — v3 has no DelayedCharge/
// DelayedCredit entity on this AU build ("invalid context declaration",
// verified live), but both non-posting forms exist and save through the same
// UpdateTransactions_Transaction_qbo mutation as every other form. Contracts
// captured on TC2 2026-09-19:
//
//	POST https://qbo.intuit.com/api/v4/graphql
//	  charge:  Referer /app/nonpostingcharge — type ACTIVITY_CHARGE,
//	           header.transactionType ACTIVITY_CHARGE, txnTypeId "25"
//	  credit:  Referer /app/nonpostingcredit — type ACTIVITY_CREDIT,
//	           header.transactionType ACTIVITY_CREDIT, txnTypeId "13"
//
// Draft id is the transaction namespace + ":-1"; header.contact carries the
// customer (bare v3 id + externalIds.localId); itemLines ride "ITEM_client_N"
// ids with traits.item{quantity, rateType PER_QTY_RATE, rate, item{id}}.
// traits.acceptStatus.acceptStatus is "PENDING" in both captures.
func ReplayDelayedCharge(ctx context.Context, kind, customer, date, lineItems string) (*MutateResult, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	var txnType, txnTypeID, referer, entity, label string
	switch kind {
	case "charge":
		txnType, txnTypeID, entity, label = "ACTIVITY_CHARGE", "25", "DelayedCharge", "delayed charge"
		referer = "https://qbo.intuit.com/app/nonpostingcharge"
	case "credit":
		txnType, txnTypeID, entity, label = "ACTIVITY_CREDIT", "13", "DelayedCredit", "delayed credit"
		referer = "https://qbo.intuit.com/app/nonpostingcredit"
	default:
		return nil, fmt.Errorf("delayed kind must be charge or credit, got %q", kind)
	}
	customer = strings.TrimSpace(customer)
	if customer == "" {
		return nil, fmt.Errorf("%s create requires --customer", label)
	}
	iso, err := billDateISO(strings.TrimSpace(date))
	if err != nil {
		return nil, fmt.Errorf("--%s-date: %w", kind, err)
	}
	lines, err := parseDelayedLines(lineItems)
	if err != nil {
		return nil, fmt.Errorf("%s create: %w", label, err)
	}
	custID, err := resolveNameID(ctx, "Customer", customer)
	if err != nil {
		return nil, fmt.Errorf("customer: %w", err)
	}
	custName, err := entityDisplayName(ctx, "Customer", custID)
	if err != nil {
		return nil, fmt.Errorf("customer %s: %w", custID, err)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	itemLines := make([]any, 0, len(lines))
	total := 0.0
	for i, l := range lines {
		itemID, err := resolveNameID(ctx, "Item", l.Item)
		if err != nil {
			return nil, fmt.Errorf("line %d item: %w", i+1, err)
		}
		itemType, itemName := delayedItemInfo(ctx, ac, itemID)
		amt := l.Amount
		if amt == 0 {
			amt = l.Qty * l.Rate
		}
		rate := l.Rate
		if rate == 0 && l.Qty != 0 {
			rate = amt / l.Qty
		}
		total += amt
		itemLines = append(itemLines, map[string]any{
			"id":          fmt.Sprintf("ITEM_client_%d", i),
			"description": l.Description,
			"projectId":   "",
			"amount":      fmt.Sprintf("%.2f", amt),
			"class":       map[string]any{},
			"qboAppData":  map[string]any{},
			"traits": map[string]any{
				"subTotal": map[string]any{},
				"item": map[string]any{
					"quantity": strconv.FormatFloat(l.Qty, 'f', -1, 64),
					"rateType": "PER_QTY_RATE",
					"rate":     strconv.FormatFloat(rate, 'f', -1, 64),
					"item": map[string]any{
						"id":       itemID,
						"itemType": itemType,
						"name":     itemName,
						"fullName": itemName,
					},
					"bundleDetails": map[string]any{},
				},
				"deferredRecognition": map[string]any{},
				"purchaseOrder":       map[string]any{},
				"progressivelyBilled": map[string]any{},
				"billable":            map[string]any{"billable": false},
			},
		})
	}
	amtS := fmt.Sprintf("%.2f", total)
	txn := map[string]any{
		"id":            txnDraftID(ac),
		"entityVersion": "0",
		"stageEntity":   nil,
		"meta":          map[string]any{},
		"qboAppData": map[string]any{
			"txnTypeId":                   txnTypeID,
			"hasShipping":                 false,
			"eligibleToReclaimTaxWithFRS": false,
			"purchSaleLocationEnabled":    false,
		},
		"header": map[string]any{
			"projectId": "",
			"contact": map[string]any{
				"displayName": custName,
				"externalIds": []map[string]any{{"namespaceId": "intuit.qbo.name.id", "localId": custID}},
				"id":          custID,
				"qboAppData":  map[string]any{},
			},
			"convenienceFee":  map[string]any{"ach": map[string]any{}},
			"transactionType": txnType,
			"txnDate":         iso,
			"amount":          amtS,
			"currencyInfo": map[string]string{
				"symbol": "A$", "homeAmount": amtS, "code": "AUD",
				"exchangeRate": "1.00", "name": "Australian Dollar", "currency": "AUD",
			},
			"regionExtensions": map[string]any{"defaultTxnSubType": map[string]any{}},
			"voided":           false,
			"account":          map[string]any{},
			"tags":             []any{},
		},
		"type":         txnType,
		"attachments":  []any{},
		"customFields": []any{},
		"lines":        map[string]any{"itemLines": itemLines, "accountLines": []any{}},
		"traits": map[string]any{
			"costEstimate":           map[string]any{},
			"esignature":             map[string]any{},
			"delivery":               map[string]any{"printStatus": "NOT_SET", "correspondenceAddress": map[string]any{}, "emailDeliveryInfo": map[string]any{}},
			"shipping":               map[string]any{"shipAddress": map[string]any{"freeFormAddressLine": ""}, "shipFromAddress": map[string]any{}},
			"discount":               map[string]any{},
			"prePayment":             map[string]any{},
			"txnWithholdingTaxTrait": map[string]any{},
			"balance": map[string]any{
				"onlinePaymentInfo": map[string]any{"enableCCPayment": false, "enableBankPayment": false, "enableSavedPaymentMethod": false, "enableFinancing": false},
				"balance":           "0",
				"dueDate":           iso,
			},
			"invoiceDeposit": nil,
			"payment": map[string]any{
				"paymentDetailsMessage": nil, "paymentMethod": map[string]any{"id": nil},
				"processedPayment": map[string]any{}, "qboAppData": map[string]any{},
				"creditCardInfo": map[string]any{}, "checkInfo": map[string]any{},
			},
			"expirationTrait": map[string]any{},
			"transfer":        map[string]any{"fromAccount": map[string]any{}, "toAccount": map[string]any{}},
			"acceptStatus":    map[string]any{"acceptStatus": "PENDING"},
			"tax":             map[string]any{},
			"bankDeposit":     map[string]any{"cashBackAccount": map[string]any{}},
			"journal":         map[string]any{},
		},
	}
	payload, err := json.Marshal(map[string]any{
		"query": `mutation UpdateTransactions_Transaction_qbo($input_0: UpdateTransactions_TransactionInput!) {
  updateTransactions_Transaction(input: $input_0) {
    clientMutationId
    transactionsTransaction { id entityVersion }
  }
}`,
		"variables": map[string]any{"input_0": map[string]any{
			"clientMutationId":        "0",
			"transactionsTransaction": txn,
		}},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, invoicesImportGraphQLURL, payload, map[string]string{
		"Referer":      referer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("%s create: %w", label, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out struct {
		Data struct {
			Mut struct {
				Txn struct {
					ID string `json:"id"`
				} `json:"transactionsTransaction"`
			} `json:"updateTransactions_Transaction"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parsing %s response: %w", label, err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("%s create: %s", label, out.Errors[0].Message)
	}
	txnID := out.Data.Mut.Txn.ID
	if txnID == "" {
		return nil, fmt.Errorf("%s create: no node id in response: %.300s", label, raw)
	}
	if i := strings.LastIndex(txnID, ":"); i >= 0 {
		txnID = txnID[i+1:]
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "create",
		Entity: entity,
		Item:   QueryItem{Type: entity, ID: txnID, Name: custName, Amount: total},
		Note:   fmt.Sprintf("%d item line(s)", len(lines)),
	}, nil
}

// delayedItemInfo fetches an Item's v4 itemType and display name via v3.
// Defaults mirror the common SERVICE item when the row is sparse.
func delayedItemInfo(ctx context.Context, ac *apiClient, itemID string) (itemType, name string) {
	rows, err := queryV3Rows(ctx, ac, "Item", fmt.Sprintf("select * from Item where Id='%s'", itemID))
	if err != nil || len(rows) == 0 {
		return "SERVICE", ""
	}
	row := rows[0]
	itemType, _ = row["Type"].(string)
	if itemType == "" {
		itemType = "SERVICE"
	}
	itemType = strings.ToUpper(itemType)
	if n, ok := row["FullyQualifiedName"].(string); ok && n != "" {
		name = n
	} else if n, ok := row["Name"].(string); ok {
		name = n
	}
	return itemType, name
}

type delayedLine struct {
	Item        string
	Qty         float64
	Rate        float64
	Amount      float64
	Description string
}

// parseDelayedLines decodes --line-items JSON: [{item, qty?, rate?, amount?,
// description?}]. amount defaults to qty*rate; qty defaults to 1.
func parseDelayedLines(raw string) ([]delayedLine, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("requires --line-items (JSON array of {item, qty?, rate?, amount?})")
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(raw), &rows); err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("--line-items must decode to a non-empty JSON array")
	}
	out := make([]delayedLine, 0, len(rows))
	for i, row := range rows {
		l := delayedLine{
			Item:        kvStr(row, "item", "product", "service", "item-id"),
			Description: kvStr(row, "description", "desc", "memo"),
			Qty:         kvFloat(row, 1, "qty", "quantity"),
			Rate:        kvFloat(row, 0, "rate", "unit-price", "price"),
			Amount:      kvFloat(row, 0, "amount", "total"),
		}
		if l.Item == "" {
			return nil, fmt.Errorf("line %d: missing item (delayed lines carry products/services, not accounts)", i+1)
		}
		if l.Amount == 0 && l.Rate == 0 {
			return nil, fmt.Errorf("line %d: set rate or amount", i+1)
		}
		out = append(out, l)
	}
	return out, nil
}

// kvFloat reads a numeric row value (number or numeric string), def when absent.
func kvFloat(row map[string]any, def float64, keys ...string) float64 {
	for _, k := range keys {
		v, ok := row[k]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return n
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f
			}
		case json.Number:
			if f, err := n.Float64(); err == nil {
				return f
			}
		}
	}
	return def
}
