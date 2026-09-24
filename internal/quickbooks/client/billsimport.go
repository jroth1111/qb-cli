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
	"time"
)

// errImportNameNoMatch distinguishes a confirmed empty v3 lookup from a
// transport, authentication, decoding, or ambiguous-match failure. Only the
// former permits an import wizard to create a new contact.
var errImportNameNoMatch = errors.New("import name has no confirmed match")

// BILLS_IMPORT rides two captured contracts — both from the
// /app/dataimport/bills wizard on TC2 2026-09-18 (upload → map → review →
// Start import):
//
//  1. Missing supplier, when "Add new suppliers" is on:
//     POST https://qbo.intuit.com/api/neo/v1/company/<realm>/lists/name/save
//     {"fullName":{"displayName":"QBCliBillSupp2"},"currency":"AUD","nameTypeId":1}
//
//  2. Each bill — a v4 GraphQL mutation (NOT the neo importdata endpoint the
//     entity wizards use):
//     POST https://qbo.intuit.com/api/v4/graphql
//     mutation Company_qbo { createTransactions_Transaction(input:…) }
//     variables.createTransactionInput.transactionsTransaction {
//     header{txnDate,referenceNumber,currencyInfo{…},contact{id},transactionType:"PURCHASE_BILL"},
//     traits{balance{dueDate},delivery{emailDeliveryInfo{to}}},
//     lines{accountLines:[{account{id},traits{},amount}]},
//     type:"PURCHASE_BILL", qboAppData:{} }
//
// GraphQL contact/account ids share the v3 id space (vendor QBCliBillSupp2 =
// contact 27 = v3 27; "Office expenses" = account 17 = v3 17), so names are
// resolved through ReplayQuery like the reclassify lane.
const (
	billsImportGraphQLURL = "https://qbo.intuit.com/api/v4/graphql"
	billsImportReferer    = "https://qbo.intuit.com/app/dataimport/bills"
	billsNameSaveHost     = "qbo.intuit.com"
)

const billsImportMutation = `mutation Company_qbo($createTransactionInput: CreateTransactions_TransactionInput!){
            createTransactions_Transaction(input: $createTransactionInput) {
                clientMutationId
                transactionsTransactionEdge {
                    node {
                        id
                    }
                }
            }
        }`

// billsCSVFields maps required/optional CSV headers (alias → column key).
var billsCSVFields = map[string]string{
	"supplier": "supplier", "vendor": "supplier", "vendor name": "supplier", "supplier name": "supplier",
	"bill no": "refNo", "ref no.": "refNo", "ref no": "refNo", "reference": "refNo", "doc number": "refNo",
	"bill date": "date", "date": "date", "txn date": "date",
	"due date": "dueDate", "due": "dueDate",
	"terms":   "terms",
	"account": "account", "expense account": "account", "category": "account",
	"line description": "description", "description": "description", "desc": "description",
	"line amount": "amount", "amount": "amount",
	"currency": "currency",
	"memo":     "memo",
}

var billsRequiredCols = []string{"supplier", "refNo", "date", "dueDate", "account", "amount"}

// billImportRow is one CSV row after column mapping.
type billImportRow map[string]string

// PlannedBillsImportURL reports the mutation URL for dry-run plans.
func PlannedBillsImportURL() string { return billsImportGraphQLURL }

// ReplayBillsImport imports bills from a CSV through the dataimport wizard's
// captured contract. Missing suppliers are created via lists/name/save — the
// same path the wizard's "Add new suppliers" toggle takes.
func ReplayBillsImport(ctx context.Context, path string) (*MutateResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("bills import requires --file <csv>")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("bills import: %w", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("bills import: reading CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("bills import: %s needs a header row plus at least one data row", path)
	}
	header := rows[0]
	colKey := make([]string, len(header))
	for ci, h := range header {
		colKey[ci] = billsCSVFields[strings.ToLower(strings.TrimSpace(h))]
	}
	have := map[string]bool{}
	for _, k := range colKey {
		if k != "" {
			have[k] = true
		}
	}
	var missing []string
	for _, req := range billsRequiredCols {
		if !have[req] {
			missing = append(missing, req)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("bills import: CSV missing required columns %s (mandatory wizard fields: Bill No, Supplier, Bill Date, Due Date, Account, Line Amount)", strings.Join(missing, ", "))
	}

	bills := make([]billImportRow, 0, len(rows)-1)
	for _, rec := range rows[1:] {
		r := billImportRow{}
		for ci, k := range colKey {
			if k != "" && ci < len(rec) {
				r[k] = strings.TrimSpace(rec[ci])
			}
		}
		bills = append(bills, r)
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if ac.realm == "" {
		return nil, fmt.Errorf("bills import: no realm id in credentials")
	}

	var created []string
	var newVendors []string
	for i, b := range bills {
		id, err := importBill(ctx, ac, b, &newVendors)
		if err != nil {
			return nil, fmt.Errorf("bills import row %d (%s): %w", i+1, b["refNo"], err)
		}
		created = append(created, id)
	}
	note := "createTransactions_Transaction PURCHASE_BILL"
	if len(newVendors) > 0 {
		note += "; suppliers created via lists/name/save: " + strings.Join(newVendors, ",")
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "bills import",
		Entity: "Bill",
		Item:   QueryItem{Type: "Bill", ID: strings.Join(created, ","), Name: strconv.Itoa(len(created)) + " bills"},
		Note:   note,
	}, nil
}

// importBill creates one bill: resolve supplier (create if missing), resolve
// account, then post the captured mutation.
func importBill(ctx context.Context, ac *apiClient, b billImportRow, newVendors *[]string) (string, error) {
	vendorID, err := resolveNameID(ctx, "Vendor", b["supplier"])
	if err != nil {
		if !errors.Is(err, errImportNameNoMatch) {
			return "", fmt.Errorf("supplier lookup: %w", err)
		}
		vendorID, err = createImportVendor(ctx, ac, b["supplier"])
		if err != nil {
			return "", err
		}
		*newVendors = append(*newVendors, b["supplier"])
	}
	acctID, err := reclassifyAccountID(ctx, b["account"])
	if err != nil {
		return "", fmt.Errorf("account: %w", err)
	}
	txnDate, err := billDateISO(b["date"])
	if err != nil {
		return "", fmt.Errorf("bill date %q: %w", b["date"], err)
	}
	dueDate, err := billDateISO(b["dueDate"])
	if err != nil {
		return "", fmt.Errorf("due date %q: %w", b["dueDate"], err)
	}
	curr := b["currency"]
	if curr == "" {
		curr = "AUD"
	}
	if !strings.EqualFold(curr, "AUD") {
		return "", fmt.Errorf("currency %s: only home-currency (AUD) rows are wired — the wizard fetches a live exchangeRate for foreign currencies, an uncaptured contract", curr)
	}
	txn := map[string]any{
		"header": map[string]any{
			"txnDate":         txnDate,
			"referenceNumber": b["refNo"],
			"currencyInfo": map[string]string{
				"code": "AUD", "name": "Australian Dollar",
				"symbol": "A$", "currency": "AUD", "exchangeRate": "1",
			},
			"contact":         map[string]string{"id": vendorID},
			"transactionType": "PURCHASE_BILL",
		},
		"traits": map[string]any{
			"balance":  map[string]string{"dueDate": dueDate},
			"delivery": map[string]any{"emailDeliveryInfo": map[string]string{"to": ""}},
		},
		"lines": map[string]any{
			"accountLines": []any{map[string]any{
				"account": map[string]string{"id": acctID},
				"traits":  map[string]any{},
				"amount":  b["amount"],
			}},
		},
		"type":       "PURCHASE_BILL",
		"qboAppData": map[string]any{},
	}
	if b["memo"] != "" {
		txn["header"].(map[string]any)["privateNote"] = b["memo"]
	}
	if b["description"] != "" {
		txn["lines"].(map[string]any)["accountLines"].([]any)[0].(map[string]any)["description"] = b["description"]
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
		"Referer":      billsImportReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, billsImportGraphQLURL, payload, extra)
	if err != nil {
		return "", fmt.Errorf("create bill: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", fmt.Errorf("reading bill create: %w", err)
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
		return "", fmt.Errorf("parsing bill create response: %w", err)
	}
	if len(out.Errors) > 0 {
		return "", fmt.Errorf("create bill: %s", out.Errors[0].Message)
	}
	if out.Data.Mut.Edge.Node.ID == "" {
		return "", fmt.Errorf("create bill: no node id in response: %.300s", raw)
	}
	return out.Data.Mut.Edge.Node.ID, nil
}

// createImportVendor creates a supplier through the wizard's
// lists/name/save contract (nameTypeId 1 = supplier on this surface).
func createImportVendor(ctx context.Context, ac *apiClient, displayName string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"fullName":   map[string]string{"displayName": displayName},
		"currency":   "AUD",
		"nameTypeId": 1,
	})
	if err != nil {
		return "", err
	}
	u := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/lists/name/save"
	overrides := map[string]string{
		"Referer":      billsImportReferer,
		"Content-Type": "application/json",
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, u, billsNameSaveHost, body, overrides)
	if err != nil {
		return "", fmt.Errorf("create supplier: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", fmt.Errorf("reading supplier create: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	// Response shape: {"name":{"id":"27",...}} or the id nested deeper —
	// extract the first numeric "id" value.
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("create supplier: unreadable response: %.300s", raw)
	}
	if id := findNumericID(doc); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("create supplier: no id in response: %.300s", raw)
}

// findNumericID walks a decoded JSON doc for the first numeric "id" field.
func findNumericID(v any) string {
	switch t := v.(type) {
	case map[string]any:
		if id, ok := t["id"]; ok {
			switch n := id.(type) {
			case string:
				if _, err := strconv.ParseInt(n, 10, 64); err == nil {
					return n
				}
			case float64:
				return strconv.FormatInt(int64(n), 10)
			}
		}
		for _, val := range t {
			if id := findNumericID(val); id != "" {
				return id
			}
		}
	case []any:
		for _, val := range t {
			if id := findNumericID(val); id != "" {
				return id
			}
		}
	}
	return ""
}

// resolveNameID resolves a vendor/customer display name to its v3 id.
func resolveNameID(ctx context.Context, entity, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty %s name", entity)
	}
	if _, err := strconv.ParseInt(name, 10, 64); err == nil {
		return name, nil
	}
	res, err := ReplayQuery(ctx, entity, "", name, 10)
	if err != nil {
		return "", err
	}
	var matches []QueryItem
	for _, it := range res.Items {
		if strings.EqualFold(it.Name, name) {
			matches = append(matches, it)
		}
	}
	switch len(matches) {
	case 0:
		if len(res.Items) >= 10 {
			return "", fmt.Errorf("%s lookup for %q reached result limit before confirming no match", entity, name)
		}
		return "", fmt.Errorf("%w: no %s named %q", errImportNameNoMatch, entity, name)
	case 1:
		if strings.TrimSpace(matches[0].ID) == "" {
			return "", fmt.Errorf("%s lookup for %q returned an exact match without an id", entity, name)
		}
		return matches[0].ID, nil
	default:
		return "", fmt.Errorf("ambiguous %s lookup for %q (%d exact matches)", entity, name, len(matches))
	}
}

// billDateISO normalises a CSV date to ISO. Accepts D/M/YYYY (the wizard's
// selected format), YYYY-MM-DD, YYYY/M/D, and D-M-YYYY.
func billDateISO(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty date")
	}
	for _, layout := range []string{"2/1/2006", "02/01/2006", "2006-01-02", "2006/01/02", "02-01-2006", "2-1-2006", "1/2/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("unrecognised date format (wizard accepts D/M/YYYY, M/D/YYYY, YYYY/M/D, D-M-YYYY, M-D-YYYY, YYYY-M-D)")
}
