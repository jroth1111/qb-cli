package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RECEIPT_EXPENSE_CREATE rides the receipts UI's "Create expense" contract,
// captured on TC2 2026-09-18 (receipts → For review → Create expense):
//
//	POST https://stagetransactions.api.intuit.com/stage/entities
//	[{"$type":"/integration/StageEntity",
//	  "id":"<receipt stage-entity id>",
//	  "state":"ACCEPTED_ADDED",
//	  "transactionTrait":{"transactionSource":"RECEIPT_UPLOAD","fiDescription":"",
//	    "transaction":{"$type":"/transactions/Transaction",
//	      "meta":{"createdByApp":{"name":"Receipt Upload"}},"id":"-1",
//	      "header":{"txnDate,"referenceNumber","amount","account":{"id"},
//	                "contact":{"id"}},
//	      "traits":{},
//	      "lines":{"accountLines":[{"amount","account":{"id"}}]},
//	      "type":"PURCHASE"}}}]
//
// The write marks the staged receipt ACCEPTED_ADDED; the platform then
// materialises a v3 Purchase (proved: Purchase 148, $42.50 on TC2). The same
// host serves the receipts list query used to resolve --receipt-id.
const receiptExpenseStageURL = "https://stagetransactions.api.intuit.com/stage/entities"
const receiptExpenseStageHost = "stagetransactions.api.intuit.com"

// stageEntity is the minimal projection of a staged receipt.
type stageEntity struct {
	ID       string
	TxnDate  string
	Amount   string
	RefNo    string
	Source   string
	RawState string
}

// listStagedReceipts runs the receipts UI's own For-review query — identical
// filters to the captured list call (no transaction_type/date restriction).
func listStagedReceipts(ctx context.Context, ac *apiClient) ([]stageEntity, error) {
	body := []any{map[string]any{
		"$type": "/Query",
		"preparedQuery": map[string]any{
			"type": "/integration/StageEntity",
			"where": map[string]any{
				"$type": "/query/CompoundExpression", "op": "&&",
				"args": []any{
					map[string]any{"$type": "/query/FilterExpression", "args": []any{"RECEIPT_UPLOAD", "WORKFORCE_EXP_MGMT"}, "op": "in", "property": "transactionTrait.transactionSource"},
					map[string]any{"$type": "/query/FilterExpression", "op": "==", "property": "state", "args": []any{"TO_BE_REVIEWED"}},
					map[string]any{"$type": "/query/FilterExpression", "op": "in", "property": "transactionTrait.receiptProcessingState", "args": []any{"PROCESSED", "MISSING_DATA", "ERROR"}},
				},
			},
			"order": map[string]any{"$type": "/query/SortExpression", "op": "DESC", "property": "transactionTrait.transaction.header.txnDate"},
			"limit": 300, "offset": 0,
		},
	}}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, receiptExpenseStageURL, receiptExpenseStageHost, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("staged receipts: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("staged receipts: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return parseStageEntities(raw), nil
}

// parseStageEntities walks the /Result wrapper for StageEntity rows.
func parseStageEntities(raw []byte) []stageEntity {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	var out []stageEntity
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if t["$type"] == "/integration/StageEntity" {
				e := stageEntity{ID: strField(t, "id"), RawState: strField(t, "state")}
				if tt, ok := t["transactionTrait"].(map[string]any); ok {
					e.Source = strField(tt, "transactionSource")
					if txn, ok := tt["transaction"].(map[string]any); ok {
						if h, ok := txn["header"].(map[string]any); ok {
							e.TxnDate = strField(h, "txnDate")
							e.Amount = strField(h, "amount")
							e.RefNo = strField(h, "referenceNumber")
						}
					}
				}
				out = append(out, e)
				return
			}
			for _, v2 := range t {
				walk(v2)
			}
		case []any:
			for _, v2 := range t {
				walk(v2)
			}
		}
	}
	walk(doc)
	return out
}

func strField(m map[string]any, k string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

// PlannedReceiptExpenseURL reports the endpoint for dry-run plans.
func PlannedReceiptExpenseURL() string { return receiptExpenseStageURL }

// ReplayReceiptExpenseCreate converts a staged receipt to a PURCHASE expense
// via the captured ACCEPTED_ADDED writeInSync contract.
//
//	receiptID: stage-entity id or unique suffix fragment (list via
//	           `expenses stagetxn get --json`); resolves against live state.
//	payee:     supplier/customer/employee display name or v3 id.
//	paymentAccount: bank/credit-card account name or id (e.g. "QBVerify Credit Card").
//	lineItems: "category:amount[,category:amount…]" — category is an expense
//	           account name/id; header amount is the sum of line amounts.
func ReplayReceiptExpenseCreate(ctx context.Context, receiptID, payee, paymentAccount, lineItems string) (*MutateResult, error) {
	receiptID = strings.TrimSpace(receiptID)
	if receiptID == "" {
		return nil, fmt.Errorf("receipt-expense create requires --receipt-id")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	staged, err := listStagedReceipts(ctx, ac)
	if err != nil {
		return nil, err
	}
	var target *stageEntity
	for i := range staged {
		e := staged[i]
		if e.ID == receiptID || strings.HasSuffix(e.ID, receiptID) {
			if target != nil {
				return nil, fmt.Errorf("--receipt-id %q matches multiple staged receipts; pass a longer id", receiptID)
			}
			cp := e
			target = &cp
		}
	}
	if target == nil {
		return nil, fmt.Errorf("no staged receipt matching --receipt-id %q (see `expenses stagetxn get`)", receiptID)
	}

	// Resolve payee → Vendor, Customer, Employee in order.
	var contactID string
	for _, ent := range []string{"Vendor", "Customer", "Employee"} {
		if id, err := resolveNameID(ctx, ent, payee); err == nil {
			contactID = id
			break
		}
	}
	if contactID == "" {
		return nil, fmt.Errorf("payee %q: no vendor/customer/employee with that name", payee)
	}
	payAcctID, err := reclassifyAccountID(ctx, paymentAccount)
	if err != nil {
		return nil, fmt.Errorf("payment-account: %w", err)
	}
	type line struct{ acct, amt string }
	var lines []line
	var total float64
	for part := range strings.SplitSeq(lineItems, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cat, amt, ok := strings.Cut(part, ":")
		if !ok {
			cat, amt = part, ""
		}
		acctID, err := reclassifyAccountID(ctx, strings.TrimSpace(cat))
		if err != nil {
			return nil, fmt.Errorf("line-items %q: %w", part, err)
		}
		amt = strings.TrimSpace(amt)
		if amt == "" {
			return nil, fmt.Errorf("line-items %q needs an amount (category:amount)", part)
		}
		f, err := strconv.ParseFloat(amt, 64)
		if err != nil {
			return nil, fmt.Errorf("line-items %q: bad amount %q", part, amt)
		}
		total += f
		lines = append(lines, line{acct: acctID, amt: amt})
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("receipt-expense create requires --line-items 'category:amount[,…]'")
	}
	txnDate := target.TxnDate
	if txnDate == "" {
		txnDate = time.Now().Format("2006-01-02")
	}
	acctLines := make([]any, 0, len(lines))
	for _, l := range lines {
		acctLines = append(acctLines, map[string]any{
			"amount":  l.amt,
			"account": map[string]any{"id": l.acct},
		})
	}
	var contact any = map[string]any{"id": nil}
	if contactID != "" {
		contact = map[string]any{"id": contactID}
	}
	entity := map[string]any{
		"$type": "/integration/StageEntity",
		"id":    target.ID,
		"state": "ACCEPTED_ADDED",
		"transactionTrait": map[string]any{
			"transactionSource": "RECEIPT_UPLOAD",
			"fiDescription":     "",
			"transaction": map[string]any{
				"$type": "/transactions/Transaction",
				"meta":  map[string]any{"createdByApp": map[string]string{"name": "Receipt Upload"}},
				"id":    "-1",
				"header": map[string]any{
					"txnDate":         txnDate,
					"referenceNumber": target.RefNo,
					"amount":          strconv.FormatFloat(total, 'f', 2, 64),
					"account":         map[string]any{"id": payAcctID},
					"contact":         contact,
				},
				"traits": map[string]any{},
				"lines":  map[string]any{"accountLines": acctLines},
				"type":   "PURCHASE",
			},
		},
	}
	payload, err := json.Marshal([]any{entity})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, receiptExpenseStageURL, receiptExpenseStageHost, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("receipt-expense create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("receipt-expense create: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var accepted bool
	for _, e := range parseStageEntities(raw) {
		if e.ID == target.ID && e.RawState == "ACCEPTED_ADDED" {
			accepted = true
			break
		}
	}
	if !accepted {
		return nil, fmt.Errorf("receipt-expense create outcome unknown; inspect before retrying: expected %s receipt state ACCEPTED_ADDED", target.ID)
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "receipt-expense create",
		Entity: "Purchase",
		Item:   QueryItem{Type: "Purchase", Name: fmt.Sprintf("receipt %s → expense", shortID(target.ID))},
		Note:   "stagetransactions ACCEPTED_ADDED writeInSync; purchase materialises async on v3",
	}, nil
}

func shortID(id string) string {
	if i := strings.LastIndex(id, ":"); i >= 0 {
		return id[i+1:]
	}
	if len(id) > 12 {
		return id[len(id)-12:]
	}
	return id
}
