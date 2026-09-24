package client

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// INVOICE_LIST_CREATE rides the /app/dataimport/invoices wizard's captured
// contract on TC2 2026-09-18 (upload → map → review → Start import):
//
//  1. Missing customer, when "Add new contacts" is on:
//     POST https://qbo.intuit.com/api/neo/v1/company/<realm>/lists/name/save
//     {"fullName":{"displayName":"QBCliInvCust"},"currency":"AUD","nameTypeId":0}
//
//  2. Each invoice — the same v4 createTransactions_Transaction mutation the
//     bills wizard uses:
//     POST https://qbo.intuit.com/api/v4/graphql
//     variables.createTransactionInput.transactionsTransaction {
//     header{txnDate,referenceNumber,currencyInfo{…},contact{id},transactionType:"SALE_INVOICE"},
//     traits{balance{dueDate},tax{taxType:"EXCLUSIVE",taxDetails:[]},
//     delivery{emailDeliveryInfo{to},contactMessage},
//     style{stylePreferences{id:"1000000002"}}},
//     lines{itemLines:[{traits:{item:{item:{id}}},amount}]},
//     type:"SALE_INVOICE", qboAppData:{} }
//
// Proven live: invoice 146 (INV-9002, customer 37) created on TC2. The wizard
// sends item.id:null when the item name doesn't resolve — resolveNameID
// failures therefore fall through to null rather than erroring, matching the
// captured request byte-for-byte.
const (
	invoicesImportGraphQLURL = "https://qbo.intuit.com/api/v4/graphql"
	invoicesImportReferer    = "https://qbo.intuit.com/app/dataimport/invoices"
	invoicesStyleID          = "1000000002"
)

var invoiceCSVFields = map[string]string{
	"invoice no": "refNo", "invoice number": "refNo", "ref no.": "refNo", "ref no": "refNo",
	"customer": "customer", "customer name": "customer",
	"invoice date": "date", "date": "date", "txn date": "date",
	"due date": "dueDate", "due": "dueDate",
	"terms":        "terms",
	"service date": "serviceDate",
	"item":         "item", "item (product/service)": "item", "product/service": "item", "product": "item",
	"item description": "description", "description": "description", "desc": "description",
	"item quantity": "qty", "qty": "qty", "quantity": "qty",
	"item rate": "rate", "rate": "rate",
	"item amount": "amount", "amount": "amount", "line amount": "amount",
	"item tax amount": "taxAmount",
	"item tax code":   "taxCode", "tax code": "taxCode",
	"currency": "currency",
	"memo":     "memo",
}

var invoiceRequiredCols = []string{"refNo", "customer", "date", "dueDate", "amount"}

type invoiceImportRow map[string]string

// PlannedInvoicesImportURL reports the mutation URL for dry-run plans.
func PlannedInvoicesImportURL() string { return invoicesImportGraphQLURL }

// ReplayInvoicesImport imports invoices from a CSV through the dataimport
// wizard's captured contract. Missing customers are created via
// lists/name/save (nameTypeId 0) — the wizard's "Add new contacts" path.
func ReplayInvoicesImport(ctx context.Context, path string) (*MutateResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("invoice import requires --file <csv>")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("invoice import: %w", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invoice import: reading CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("invoice import: %s needs a header row plus at least one data row", path)
	}
	header := rows[0]
	colKey := make([]string, len(header))
	for ci, h := range header {
		colKey[ci] = invoiceCSVFields[strings.ToLower(strings.TrimSpace(h))]
	}
	have := map[string]bool{}
	for _, k := range colKey {
		if k != "" {
			have[k] = true
		}
	}
	var missing []string
	for _, req := range invoiceRequiredCols {
		if !have[req] {
			missing = append(missing, req)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("invoice import: CSV missing required columns %s (mandatory wizard fields: Invoice No, Customer, Invoice Date, Due Date, Item Amount)", strings.Join(missing, ", "))
	}
	if have["taxCode"] {
		return nil, fmt.Errorf("invoice import: a Tax code column needs a tax agency on the company — none of TC2's custom codes carries one (the wizard throws 'Tax agency'); export without the tax column")
	}

	invoices := make([]invoiceImportRow, 0, len(rows)-1)
	for _, rec := range rows[1:] {
		r := invoiceImportRow{}
		for ci, k := range colKey {
			if k != "" && ci < len(rec) {
				r[k] = strings.TrimSpace(rec[ci])
			}
		}
		invoices = append(invoices, r)
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if ac.realm == "" {
		return nil, fmt.Errorf("invoice import: no realm id in credentials")
	}

	var created []string
	var newCustomers []string
	for i, inv := range invoices {
		id, err := importInvoice(ctx, ac, inv, &newCustomers)
		if err != nil {
			return nil, fmt.Errorf("invoice import row %d (%s): %w", i+1, inv["refNo"], err)
		}
		created = append(created, id)
	}
	note := "createTransactions_Transaction SALE_INVOICE"
	if len(newCustomers) > 0 {
		note += "; customers created via lists/name/save: " + strings.Join(newCustomers, ",")
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "invoice import",
		Entity: "Invoice",
		Item:   QueryItem{Type: "Invoice", ID: strings.Join(created, ","), Name: strconv.Itoa(len(created)) + " invoices"},
		Note:   note,
	}, nil
}

// importInvoice creates one invoice: resolve customer (create if missing),
// resolve item (null when unknown — the wizard's behaviour), post mutation.
func importInvoice(ctx context.Context, ac *apiClient, inv invoiceImportRow, newCustomers *[]string) (string, error) {
	custID, err := resolveNameID(ctx, "Customer", inv["customer"])
	if err != nil {
		if !errors.Is(err, errImportNameNoMatch) {
			return "", fmt.Errorf("customer lookup: %w", err)
		}
		custID, err = createImportCustomer(ctx, ac, inv["customer"])
		if err != nil {
			return "", err
		}
		*newCustomers = append(*newCustomers, inv["customer"])
	}
	var itemID any
	if inv["item"] != "" {
		if id, err := resolveNameID(ctx, "Item", inv["item"]); err == nil {
			itemID = id
		}
	}
	txnDate, err := billDateISO(inv["date"])
	if err != nil {
		return "", fmt.Errorf("invoice date %q: %w", inv["date"], err)
	}
	dueDate, err := billDateISO(inv["dueDate"])
	if err != nil {
		return "", fmt.Errorf("due date %q: %w", inv["dueDate"], err)
	}
	curr := inv["currency"]
	if curr == "" {
		curr = "AUD"
	}
	if !strings.EqualFold(curr, "AUD") {
		return "", fmt.Errorf("currency %s: only home-currency (AUD) rows are wired — the wizard fetches a live exchangeRate for foreign currencies, an uncaptured contract", curr)
	}
	line := map[string]any{
		"traits": map[string]any{"item": map[string]any{"item": map[string]any{"id": itemID}}},
		"amount": inv["amount"],
	}
	if inv["description"] != "" {
		line["description"] = inv["description"]
	}
	txn := map[string]any{
		"header": map[string]any{
			"txnDate":         txnDate,
			"referenceNumber": inv["refNo"],
			"currencyInfo": map[string]string{
				"code": "AUD", "name": "Australian Dollar",
				"symbol": "A$", "currency": "AUD", "exchangeRate": "1",
			},
			"contact":         map[string]string{"id": custID},
			"transactionType": "SALE_INVOICE",
		},
		"traits": map[string]any{
			"balance": map[string]string{"dueDate": dueDate},
			"tax":     map[string]any{"taxType": "EXCLUSIVE", "taxDetails": []any{}},
			"delivery": map[string]any{
				"emailDeliveryInfo": map[string]string{"to": ""},
				"contactMessage":    "",
			},
			"style": map[string]any{"stylePreferences": map[string]string{"id": invoicesStyleID}},
		},
		"lines":      map[string]any{"itemLines": []any{line}},
		"type":       "SALE_INVOICE",
		"qboAppData": map[string]any{},
	}
	if inv["memo"] != "" {
		txn["header"].(map[string]any)["privateNote"] = inv["memo"]
	}
	payload, err := json.Marshal(map[string]any{
		"query": billsImportMutation,
		"variables": map[string]any{
			"createTransactionInput": map[string]any{
				"clientMutationId":        "0",
				"transactionsTransaction": txn,
			},
		},
	})
	if err != nil {
		return "", err
	}
	extra := map[string]string{
		"Referer":      invoicesImportReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, invoicesImportGraphQLURL, payload, extra)
	if err != nil {
		return "", fmt.Errorf("create invoice: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", fmt.Errorf("reading invoice create: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out struct {
		Data struct {
			Mut struct {
				Edge struct {
					Node struct {
						ID string `json:"id"`
					} `json:"node"`
				} `json:"transactionsTransactionEdge"`
			} `json:"createTransactions_Transaction"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("parsing invoice create response: %w", err)
	}
	if len(out.Errors) > 0 {
		return "", fmt.Errorf("create invoice: %s", out.Errors[0].Message)
	}
	if out.Data.Mut.Edge.Node.ID == "" {
		return "", fmt.Errorf("create invoice: no node id in response: %.300s", raw)
	}
	return out.Data.Mut.Edge.Node.ID, nil
}

// createImportCustomer creates a customer through the wizard's
// lists/name/save contract (nameTypeId 0 = customer on this surface).
func createImportCustomer(ctx context.Context, ac *apiClient, displayName string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"fullName":   map[string]string{"displayName": displayName},
		"currency":   "AUD",
		"nameTypeId": 0,
	})
	if err != nil {
		return "", err
	}
	u := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/lists/name/save"
	overrides := map[string]string{
		"Referer":      invoicesImportReferer,
		"Content-Type": "application/json",
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, u, billsNameSaveHost, body, overrides)
	if err != nil {
		return "", fmt.Errorf("create customer: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", fmt.Errorf("reading customer create: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("create customer: unreadable response: %.300s", raw)
	}
	if id := findNumericID(doc); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("create customer: no id in response: %.300s", raw)
}
