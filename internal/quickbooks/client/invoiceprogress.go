package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// v4LinkNSTestCompany2 is the v4 Relay namespace for transaction LINK nodes on
// Test Company 2 — decodes to "v4.1:9341457769756854:060824e6cb". Captured
// from the progress-invoice save mutation (links.sources ids).
const v4LinkNSTestCompany2 = "djQuMTo5MzQxNDU3NzY5NzU2ODU0OjA2MDgyNGU2Y2I"

// v4ItemNSTestCompany2 is the v4 Relay namespace for Item nodes on Test
// Company 2 — decodes to "v4.1:9341457769756854:112de74699". Captured from
// the progress-invoice itemLines item.id.
const v4ItemNSTestCompany2 = "djQuMTo5MzQxNDU3NzY5NzU2ODU0OjExMmRlNzQ2OTk"

// v4LineNSTestCompany2 is the v4 Relay namespace for transaction LINE nodes —
// decodes to "v4.1:9341457769756854:360f609f55". Estimate/invoice line ids.
const v4LineNSTestCompany2 = "djQuMTo5MzQxNDU3NzY5NzU2ODU0OjM2MGY2MDlmNTU"

func v4NS(ac *apiClient, shard, tc2Const string) string {
	if ac.realm == v4TxNSRealm {
		return tc2Const
	}
	return base64.RawURLEncoding.EncodeToString([]byte("v4.1:" + ac.realm + ":" + shard))
}

func linkNodeID(ac *apiClient, key string) string {
	return v4NS(ac, "060824e6cb", v4LinkNSTestCompany2) + ":" + key
}
func itemNodeID(ac *apiClient, itemID string) string {
	return v4NS(ac, "112de74699", v4ItemNSTestCompany2) + ":" + itemID
}
func lineNodeID(ac *apiClient, key string) string {
	return v4NS(ac, "360f609f55", v4LineNSTestCompany2) + ":" + key
}

// ReplayInvoiceProgress creates a progress (partial) invoice from an estimate
// through the v4 transaction-form mutation — UpdateTransactions_Transaction_qbo
// with type SALE_INVOICE, itemLines carrying qboAppData.detailType
// ESTIMATE_INVOICE_LINK, traits.item.progressTracking PERCENT, and
// links.sources at both line and transaction level consuming the estimate.
// Contract captured on TC2 (estimate 178 → invoice 179 at 50%): the
// transaction-level sourceTxn embeds the full estimate object with
// consumableBalance reduced by the invoiced amount; line-level sourceLine
// reports totalAmountConsumed cumulative.
//
// Requires company setting "Progress Invoicing" on (verified enabled on TC2).
func ReplayInvoiceProgress(ctx context.Context, estimateRef, percentS, idAlias string) (*MutateResult, error) {
	estimateRef = strings.TrimSpace(estimateRef)
	if estimateRef == "" {
		estimateRef = strings.TrimSpace(idAlias)
	}
	if estimateRef == "" {
		return nil, fmt.Errorf("invoice progress requires --estimate (id or doc number)")
	}
	pct := 100.0
	if s := strings.TrimSpace(percentS); s != "" {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil || v <= 0 || v > 100 {
			return nil, fmt.Errorf("--percent must be a number in (0,100], got %q", percentS)
		}
		pct = v
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	estID, err := resolveEstimateID(ctx, ac, estimateRef)
	if err != nil {
		return nil, err
	}
	node, err := fetchTxnNode(ctx, ac, estID)
	if err != nil {
		return nil, err
	}
	if str(node, "type") != "SALE_ESTIMATE" {
		return nil, fmt.Errorf("estimate %s resolves to %s, not SALE_ESTIMATE", estID, str(node, "type"))
	}
	if b, _ := strconv.ParseBool(str(headerOf(node), "voided")); b {
		return nil, fmt.Errorf("estimate %s is voided", estID)
	}
	balance := traitsOf(node, "balance")
	consumable := parseAmt(str(balance, "consumableBalance"))
	if consumable <= 0.004 {
		return nil, fmt.Errorf("estimate %s is fully invoiced (consumable balance %.2f)", estID, consumable)
	}
	srcLines := itemLinesOf(node)
	if len(srcLines) == 0 {
		return nil, fmt.Errorf("estimate %s has no item lines to bill", estID)
	}
	contact := contactOf(headerOf(node))
	if contact == nil {
		for _, l := range srcLines {
			if c := contactOf(l); c != nil {
				contact = c
				break
			}
		}
	}
	if contact == nil {
		return nil, fmt.Errorf("estimate %s: no customer contact on node", estID)
	}

	itemLines := make([]any, 0, len(srcLines))
	embedLines := make([]any, 0, len(srcLines))
	total := 0.0
	for i, l := range srcLines {
		seq := str(l, "sequence")
		if seq == "" {
			seq = strconv.Itoa(i + 1)
		}
		full := parseAmt(str(l, "amount"))
		prev := parseAmt(str(traitsOf(l, "purchaseOrder"), "amountReceived"))
		billed := full * pct / 100
		if pct >= 100 {
			billed = full - prev // total mode bills the line's remainder
		}
		billed = round2(billed)
		if billed < 0 {
			billed = 0
		}
		if billed > full-prev+0.005 {
			return nil, fmt.Errorf("estimate %s line %s: %.2f exceeds remaining billable %.2f", estID, seq, billed, full-prev)
		}
		consumed := round2(prev + billed)
		total += billed
		desc := str(l, "description")
		item := traitsOf(l, "item")
		itemRef := mapOf(item, "item")
		lineID := str(l, "id")
		if lineID == "" {
			lineID = lineNodeID(ac, estID+"-"+seq)
		}
		lc := contactOf(l)
		lineContact := map[string]any{"profiles": map[string]any{"vendor": nil}}
		if lc != nil {
			lineContact["id"] = lc["id"]
			lineContact["externalIds"] = lc["externalIds"]
		}
		itemLines = append(itemLines, map[string]any{
			"description": desc,
			"projectId":   "",
			"amount":      fmt.Sprintf("%.2f", billed),
			"id":          fmt.Sprintf("ITEM_client_%d", i),
			"sequence":    seq,
			"qboAppData":  map[string]any{"detailType": "ESTIMATE_INVOICE_LINK"},
			"traits": map[string]any{
				"journal":  map[string]any{"journalCode": nil, "cleared": nil, "postingType": nil},
				"discount": map[string]any{"amount": nil, "proratedAmount": nil, "discount": nil},
				"deposit":  nil,
				"tax":      map[string]any{"totalTaxableAmount": nil, "taxable": false, "totalTaxOverrideDeltaAmount": nil, "taxDetails": nil, "taxGroup": nil, "totalTaxAmount": nil, "taxInclusiveAmount": nil},
				"subTotal": map[string]any{},
				"item": map[string]any{
					"rateType":         orStr(str(item, "rateType"), "PER_QTY_RATE"),
					"progressTracking": "PERCENT",
					"item": map[string]any{
						"id":       itemNodeID(ac, relayTail(str(itemRef, "id"))),
						"itemType": orStr(str(itemRef, "itemType"), "SERVICE"),
						"name":     str(itemRef, "name"),
						"fullName": str(itemRef, "fullName"),
						"traits":   map[string]any{"sale": map[string]any{}},
					},
					"bundleDetails": map[string]any{},
				},
				"deferredRecognition": map[string]any{"isDeferred": false},
				"purchaseOrder":       map[string]any{},
				"progressivelyBilled": map[string]any{"quantityBilled": nil, "taxInclusiveAmountBilled": nil},
				"billable":            map[string]any{},
			},
			"links": map[string]any{
				"sources": []map[string]any{{
					"sourceLine": map[string]any{
						"sequence":            seq,
						"amount":              fmt.Sprintf("%.2f", full),
						"traits":              map[string]any{"item": map[string]any{}, "tax": map[string]any{}},
						"totalAmountConsumed": fmt.Sprintf("%.2f", consumed),
					},
					"id":        linkNodeID(ac, estID+"-"+seq+"--1"),
					"sourceTxn": map[string]any{"id": txnNodeID(ac, estID), "type": "SALE_ESTIMATE"},
				}},
				"targets": []any{},
			},
			"contact": lineContact,
		})
		// Embedded estimate line mirrors the captured post-mutation shape:
		// consumption fields advance by the billed amount.
		embedLines = append(embedLines, map[string]any{
			"sequence":    seq,
			"amount":      fmt.Sprintf("%.2f", full),
			"description": desc,
			"id":          lineID,
			"contact":     lc,
			"department":  nil,
			"class":       nil,
			"links":       nil,
			"traits": map[string]any{
				"costEstimate":        map[string]any{"costRate": nil, "excludeFromCustomerEstimate": nil, "costAmount": nil, "costEstimateMarkup": nil},
				"item":                map[string]any{"item": itemRef, "quantity": nil, "serviceDate": nil, "rate": nil, "bundleDetails": nil},
				"journal":             map[string]any{"journalCode": nil, "cleared": nil, "postingType": nil},
				"progressivelyBilled": map[string]any{"quantityBilled": nil, "closed": false},
				"markup":              nil,
				"purchaseOrder":       map[string]any{"closed": false, "quantityReceived": nil, "amountReceived": fmt.Sprintf("%.2f", consumed)},
				"discount":            map[string]any{"amount": nil, "proratedAmount": nil, "discount": nil},
				"deposit":             nil,
				"tax":                 map[string]any{"totalTaxableAmount": nil, "taxable": nil, "totalTaxOverrideDeltaAmount": nil, "taxDetails": nil, "taxGroup": nil, "totalTaxAmount": nil, "taxInclusiveAmount": nil},
				"subTotal":            nil,
				"billable":            traitsOf(l, "billable"),
			},
		})
	}
	if total <= 0.004 {
		return nil, fmt.Errorf("estimate %s: %.2f%% of remaining produces a $0 invoice", estID, pct)
	}
	if total > consumable+0.005 {
		return nil, fmt.Errorf("estimate %s: invoice total %.2f exceeds consumable balance %.2f", estID, total, consumable)
	}
	today := nowSydneyDate()
	amtS := fmt.Sprintf("%.2f", total)
	hdr := headerOf(node)
	cur := mapOf(hdr, "currencyInfo")
	if cur == nil {
		cur = map[string]any{"symbol": "A$", "homeAmount": nil, "code": "AUD", "exchangeRate": "1.00", "name": "Australian Dollar"}
	}
	remaining := round2(consumable - total)
	srcTxn := map[string]any{
		"links": map[string]any{"sources": []any{}},
		"traits": map[string]any{
			"balance":      map[string]any{"consumableBalance": fmt.Sprintf("%.2f", remaining), "balance": "0", "dueDate": nil},
			"prePayment":   nil,
			"costEstimate": map[string]any{},
			"tax":          map[string]any{"taxExemptionOverridden": false, "taxReclaimable": false, "taxType": "NOT_APPLICABLE"},
		},
		"attachments": []any{},
		"header": map[string]any{
			"currencyInfo": cur,
			"voided":       false,
			"account":      nil,
			"amount":       str(hdr, "amount"),
			"txnDate":      str(hdr, "txnDate"),
		},
		"id":    txnNodeID(ac, estID),
		"type":  "SALE_ESTIMATE",
		"lines": map[string]any{"accountLines": []any{}, "itemLines": embedLines},
	}
	draftID := txnDraftID(ac)
	txn := map[string]any{
		"id":            draftID,
		"entityVersion": "0",
		"stageEntity":   nil,
		"meta":          map[string]any{"created": today + "T00:00:00.000+10:00", "updated": today + "T00:00:00.000+10:00"},
		"qboAppData": map[string]any{
			"txnTypeId": "4", "purchSaleLocationEnabled": false,
			"hasCustomPurchaseRefNum": false, "eligibleToReclaimTaxWithFRS": false,
		},
		"header": map[string]any{
			"preAccounting":   false,
			"is_ict_txn":      false,
			"userNamedFields": []any{},
			"projectId":       "",
			"contact": map[string]any{
				"subsidiaryCompanyId": nil,
				"profiles":            map[string]any{"customer": map[string]any{"customerType": nil}},
				"displayName":         str(contact, "displayName"),
				"externalIds":         contact["externalIds"],
				"id":                  contact["id"],
				"qboAppData":          map[string]any{},
			},
			"convenienceFee":  map[string]any{"type": nil, "feeRate": nil, "enabled": nil, "ach": map[string]any{}},
			"transactionType": "SALE_INVOICE",
			"txnDate":         today,
			"amount":          amtS,
			"currencyInfo": map[string]string{
				"symbol": orStr(str(cur, "symbol"), "A$"), "homeAmount": "0.00",
				"code": orStr(str(cur, "code"), "AUD"), "exchangeRate": orStr(str(cur, "exchangeRate"), "1.00"),
				"name": orStr(str(cur, "name"), "Australian Dollar"), "currency": orStr(str(cur, "code"), "AUD"),
			},
			"regionExtensions": map[string]any{"txnSubTypes": nil, "offsetting": nil, "txnTaxFormNum": nil, "txnTaxFormType": nil, "govtRegistrationStatus": nil, "defaultTxnSubType": map[string]any{}},
			"voided":           false,
			"account":          map[string]any{},
			"tags":             []any{},
		},
		"type":         "SALE_INVOICE",
		"lines":        map[string]any{"itemLines": itemLines, "accountLines": []any{}},
		"attachments":  []any{},
		"customFields": []any{},
		"links": map[string]any{
			"sources": []map[string]any{{
				"targetTxn": map[string]any{"id": draftID, "type": "SALE_INVOICE"},
				"id":        linkNodeID(ac, estID+"--1"),
				"sourceTxn": srcTxn,
			}},
			"targets": []any{},
		},
		"traits": map[string]any{
			"costEstimate": map[string]any{},
			"esignature":   map[string]any{},
			"delivery": map[string]any{
				"correspondenceAddress": map[string]any{},
				"emailDeliveryInfo":     map[string]any{},
			},
			"shipping":   map[string]any{"shipAddress": map[string]any{}, "shipFromAddress": map[string]any{}},
			"discount":   map[string]any{},
			"prePayment": map[string]any{},
			"balance": map[string]any{
				"onlinePaymentInfo": map[string]any{"enableCCPayment": false, "enableBankPayment": false, "enableFinancing": false},
				"balance":           "0",
				"dueDate":           today,
			},
			"invoiceDeposit":  nil,
			"payment":         map[string]any{"paymentDetailsMessage": nil, "paymentMethod": map[string]any{}, "paymentType": nil, "processedPayment": map[string]any{}, "creditCardInfo": map[string]any{}, "checkInfo": map[string]any{}, "qboAppData": map[string]any{}},
			"expirationTrait": map[string]any{},
			"transfer":        map[string]any{"fromAccount": map[string]any{}, "toAccount": map[string]any{}},
			"acceptStatus":    map[string]any{},
			"tax":             map[string]any{},
			"bankDeposit":     map[string]any{"cashBackAccount": map[string]any{}},
			"journal":         map[string]any{},
			"redactionTrait":  map[string]any{"lineRedactionSummary": map[string]any{}},
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
		"Referer":      "https://qbo.intuit.com/app/invoice",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("invoice progress: %w", err)
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
		return nil, fmt.Errorf("parsing invoice-progress response: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("invoice progress: %s", out.Errors[0].Message)
	}
	txnID := out.Data.Mut.Txn.ID
	if txnID == "" {
		return nil, fmt.Errorf("invoice progress: no node id in response: %.300s", raw)
	}
	if i := strings.LastIndex(txnID, ":"); i >= 0 {
		txnID = txnID[i+1:]
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "progress",
		Entity: "Invoice",
		Item:   QueryItem{Type: "Invoice", ID: txnID, Amount: total},
		Note:   fmt.Sprintf("%.0f%% of estimate %s (remaining %.2f)", pct, estID, remaining),
	}, nil
}

// resolveEstimateID accepts a numeric id or a DocNumber (EST-131 / 131 style
// matches on digits too) and returns the v3 Estimate id.
func resolveEstimateID(ctx context.Context, ac *apiClient, ref string) (string, error) {
	if _, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return ref, nil
	}
	doc := strings.TrimPrefix(strings.ToUpper(ref), "EST-")
	rows, err := queryV3Rows(ctx, ac, "Estimate", fmt.Sprintf("select * from Estimate where DocNumber = '%s'", doc))
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no estimate matching %q", ref)
	}
	return fmt.Sprint(rows[0]["Id"]), nil
}

// fetchTxnNode reads a transaction's v4 node with the fields the
// sourceTxn embed + link bookkeeping need: header amount/date/currency,
// consumable balance, and per-line sequence/amount/description/item/contact/
// consumption state. Shared by progress-invoice (estimate) and form-delete
// (any transaction — type/existence preflight).
func fetchTxnNode(ctx context.Context, ac *apiClient, txnID string) (map[string]any, error) {
	payload, err := json.Marshal(map[string]any{
		"query": `query c($id: ID!) { node(id: $id) { ... on Transactions_Transaction {
  id type
  header { amount txnDate voided currencyInfo { symbol homeAmount code exchangeRate name } contact { id displayName externalIds { namespaceId localId } } }
  traits { balance { consumableBalance balance dueDate } }
  lines { itemLines { edges { node { id sequence amount description
    contact { id externalIds { namespaceId localId } }
    traits { item { rateType item { id name fullName itemType sku } } purchaseOrder { closed quantityReceived amountReceived } billable { billableTo { id externalIds { namespaceId localId } } billable salesAmt } }
  } } } }
  links { targets { edges { node { id targetTxn { id type } } } } }
} } }`,
		"variables": map[string]any{"id": txnNodeID(ac, txnID)},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, invoicesImportGraphQLURL, payload, map[string]string{
		"Referer":      "https://qbo.intuit.com/app/invoice",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("reading transaction %s: %w", txnID, err)
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
			Node map[string]any `json:"node"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parsing transaction %s node: %w", txnID, err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("reading transaction %s: %s", txnID, out.Errors[0].Message)
	}
	if out.Data.Node == nil {
		return nil, fmt.Errorf("transaction %s: no v4 transaction node", txnID)
	}
	return out.Data.Node, nil
}

// --- map helpers (map[string]any pass-through, no shared-name collisions) ---

func mapOf(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func parseAmt(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func headerOf(node map[string]any) map[string]any { return mapOf(node, "header") }

func traitsOf(m map[string]any, key string) map[string]any {
	return mapOf(mapOf(m, "traits"), key)
}

func itemLinesOf(node map[string]any) []map[string]any {
	conn := mapOf(mapOf(node, "lines"), "itemLines")
	if conn == nil {
		return nil
	}
	edges, _ := conn["edges"].([]any)
	out := make([]map[string]any, 0, len(edges))
	for _, e := range edges {
		if em, ok := e.(map[string]any); ok {
			if n, ok := em["node"].(map[string]any); ok {
				out = append(out, n)
			}
		}
	}
	return out
}

func contactOf(m map[string]any) map[string]any {
	c := mapOf(m, "contact")
	if c == nil || c["id"] == nil {
		return nil
	}
	return c
}

// relayTail returns the trailing segment of a Relay node id ("ns:key" → "key").
func relayTail(id string) string {
	if i := strings.LastIndex(id, ":"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func nowSydneyDate() string {
	// Invoice form stamps dates in Australia/Sydney (the captured meta used
	// +10:00).
	loc, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		loc = time.FixedZone("AEST", 10*60*60)
	}
	return time.Now().In(loc).Format("2006-01-02")
}
