package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrMissingMutateID is returned when update/delete/void has no id.
var ErrMissingMutateID = errors.New("mutate requires --id")

// ErrCSVLineItems is returned when --line-items carries CSV rows. Only a
// JSON array of v3 line objects is honored; anything else was silently
// dropped (recording a $1 default line), so it is rejected loudly instead.
var ErrCSVLineItems = errors.New("line-items must be a JSON array of v3 line objects (CSV rows are rejected, never coerced)")

// CheckLineItemsJSON rejects non-JSON --line-items/--lines/--items before any
// plan or POST is built, so both dry-run and live paths fail fast on CSV input.
func CheckLineItemsJSON(flags map[string]string) error {
	raw := strings.TrimSpace(firstFlag(flags, "line-items", "lines", "items"))
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var parsed []map[string]any
		if json.Unmarshal([]byte(raw), &parsed) == nil && len(parsed) > 0 {
			return nil
		}
		return fmt.Errorf("%w: empty or invalid JSON array: got %q", ErrCSVLineItems, truncLineItems(raw))
	}
	return fmt.Errorf("%w: got %q", ErrCSVLineItems, truncLineItems(raw))
}

func truncLineItems(raw string) string {
	if len(raw) > 80 {
		return raw[:80] + "…"
	}
	return raw
}

// normalizeAccountType maps UI labels to v3 API enums (the API rejects
// "Cost of Sales" but accepts "Cost of Goods Sold"). Unknown values pass
// through untouched for the server to judge.
func normalizeAccountType(t string) string {
	switch t {
	case "Cost of Sales":
		return "Cost of Goods Sold"
	}
	return t
}

// parseIDList normalizes StringSlice flag values ("[33]", "33,34", "33")
// into clean ids.
func parseIDList(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	var out []string
	for p := range strings.SplitSeq(raw, ",") {
		p = strings.Trim(strings.TrimSpace(p), `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

var v3Path = map[string]string{
	"Account":              "account",
	"Attachable":           "attachable",
	"Bill":                 "bill",
	"BillPayment":          "billpayment",
	"DelayedCharge":        "delayedcharge",
	"DelayedCredit":        "delayedcredit",
	"Budget":               "budget",
	"Class":                "class",
	"CompanyCurrency":      "companycurrency",
	"CompanyInfo":          "companyinfo",
	"CreditCardCredit":     "creditcardcredit",
	"CreditMemo":           "creditmemo",
	"Customer":             "customer",
	"Department":           "department",
	"Deposit":              "deposit",
	"Employee":             "employee",
	"Estimate":             "estimate",
	"Invoice":              "invoice",
	"Item":                 "item",
	"ItemReceipt":          "itemreceipt",
	"InventoryAdjustment":  "inventoryadjustment",
	"JournalEntry":         "journalentry",
	"Payment":              "payment",
	"PaymentMethod":        "paymentmethod",
	"Project":              "project",
	"Preferences":          "preferences",
	"Purchase":             "purchase",
	"PurchaseOrder":        "purchaseorder",
	"RecurringTransaction": "recurringtransaction",
	"RefundReceipt":        "refundreceipt",
	"SalesReceipt":         "salesreceipt",
	"TaxAgency":            "taxagency",
	"TaxCode":              "taxcode",
	"TaxRate":              "taxrate",
	"Term":                 "term",
	"TimeActivity":         "timeactivity",
	"Transfer":             "transfer",
	"Vendor":               "vendor",
	"VendorCredit":         "vendorcredit",
}

// MutateResult is the secret-free envelope for a v3 write.
type MutateResult struct {
	Status   int       `json:"status"`
	Op       string    `json:"op"`
	Entity   string    `json:"entity"`
	Item     QueryItem `json:"item"`
	Note     string    `json:"note,omitempty"`
	Verified bool      `json:"verified"`
	Evidence string    `json:"evidence,omitempty"`
}

// ReplayMutate POSTs a v3 create/update/delete/void against the session company.
func ReplayMutate(ctx context.Context, entity, op, id string, flags map[string]string) (*MutateResult, error) {
	entity = strings.TrimSpace(entity)
	op = strings.ToLower(strings.TrimSpace(op))
	cheque := entity == "Cheque"
	if cheque {
		entity = "Purchase"
		copyFlags := make(map[string]string, len(flags)+1)
		maps.Copy(copyFlags, flags)
		flags = copyFlags
		for _, key := range []string{"payment-type", "mode", "type"} {
			if value := strings.ToLower(strings.TrimSpace(flags[key])); value != "" && value != "check" && value != "cheque" {
				return nil, fmt.Errorf("cheque requires payment type Check, got --%s %s", key, value)
			}
		}
		flags["cheque"] = "true"
	}
	path, ok := v3Path[entity]
	if !ok || entity == "" {
		return nil, fmt.Errorf("no v3 path for entity %q", entity)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if err := CheckLineItemsJSON(flags); err != nil {
		return nil, err
	}
	if op == "send" || entity == "RecurringTransaction" || entity == "Budget" || entity == "SalesOrder" {
		if (op == "send" || op == "writeoff") && strings.TrimSpace(id) == "" && strings.TrimSpace(flags["id"]) == "" {
			return nil, ErrMissingMutateID
		}
		return nil, fmt.Errorf("%w: %s %s requires a compound/service-specific verifier", ErrReadbackUnavailable, entity, op)
	}
	var before map[string]any
	fetchExisting := func(path, id string) (map[string]any, error) {
		existing, err := fetchV3(ctx, ac, path, id)
		if err == nil && jsonNumberString(existing["Id"]) != id {
			return nil, fmt.Errorf("preflight entity identity mismatch")
		}
		if err == nil {
			before = existing
		}
		if err == nil && cheque && existing["PaymentType"] != "Check" {
			return nil, fmt.Errorf("purchase %s is not a cheque; expected PaymentType Check", id)
		}
		return existing, err
	}

	var body map[string]any
	opQ := ""
	var writeoffApply *writeoffLink
	switch op {
	case "create":
		if entity == "Budget" {
			gres, gerr := ReplayBudgetCreate(ctx, flags)
			if gerr == nil {
				return gres, nil
			}
			var re *ReplayError
			if !errors.As(gerr, &re) || re.Status != http.StatusForbidden {
				return nil, gerr
			}
			// budgeting.api GraphQL 403s from this process (captured UI is 200);
			// v3 /budget creates the same entity the UI lists.
		}
		body, err = buildCreateBody(entity, flags)
	case "update", "edit":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		existing, errf := fetchExisting(path, id)
		if errf != nil {
			return nil, errf
		}
		body, err = buildUpdateBody(entity, flags, existing)
		if err == nil && entity == "RecurringTransaction" {
			body = map[string]any{"Invoice": body}
		}
		op = "update"

	case "delete":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		existing, errf := fetchExisting(path, id)
		if errf != nil {
			return nil, errf
		}
		body = map[string]any{"Id": existing["Id"], "SyncToken": existing["SyncToken"]}
		if entity == "RecurringTransaction" {
			body = map[string]any{"Invoice": body}
		}
		opQ = "delete"

	case "void":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		existing, errf := fetchExisting(path, id)
		if errf != nil {
			return nil, errf
		}
		body = map[string]any{"Id": existing["Id"], "SyncToken": existing["SyncToken"]}
		opQ = "void"
	case "deactivate":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		existing, errf := fetchExisting(path, id)
		if errf != nil {
			return nil, errf
		}
		body = map[string]any{"Id": existing["Id"], "SyncToken": existing["SyncToken"], "Active": false}
		for _, k := range []string{"Name", "DisplayName", "GivenName", "FamilyName", "AccountType", "AccountSubType", "Type", "Code", "DueDays", "DiscountPercent", "DiscountDays", "DayOfMonthDue", "DueNextMonthDays", "IncomeAccountRef", "ExpenseAccountRef", "AssetAccountRef"} {
			if v := existing[k]; v != nil {
				body[k] = v
			}
		}

		op = "update"
	case "discount":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		existing, errf := fetchExisting(path, id)
		if errf != nil {
			return nil, errf
		}
		if flags == nil {
			flags = map[string]string{}
		}
		if firstFlag(flags, "discount") == "" {
			if a := firstFlag(flags, "amount"); a != "" {
				flags["discount"] = a
			}
		}
		if firstFlag(flags, "discount-percent") == "" {
			if p := firstFlag(flags, "percent"); p != "" {
				flags["discount-percent"] = p
			}
		}
		delete(flags, "amount")
		body, err = buildUpdateBody(entity, flags, existing)
		op = "update"
	case "writeoff":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		inv, errf := fetchExisting("invoice", id)
		if errf != nil {
			return nil, errf
		}
		if flags == nil {
			flags = map[string]string{}
		}
		if firstFlag(flags, "customer") == "" {
			if cref, ok := inv["CustomerRef"].(map[string]any); ok {
				flags["customer"] = fmt.Sprint(cref["value"])
			}
		}
		if firstFlag(flags, "amount") == "" {
			amt := parseAmount(fmt.Sprint(inv["Balance"]))
			if amt == 0 {
				amt = parseAmount(fmt.Sprint(inv["TotalAmt"]))
			}
			if amt != 0 {
				flags["amount"] = strconv.FormatFloat(amt, 'f', 2, 64)
			}
		}
		if firstFlag(flags, "item") == "" {
			if itemID := firstItemRef(inv); itemID != "" {
				flags["item"] = itemID
			}
		}
		flags["invoice-id"] = id
		body, err = buildCreateBody("CreditMemo", flags)
		path = "creditmemo"
		entity = "CreditMemo"
		op = "create"
		// The credit memo alone leaves the invoice balance open; a write-off
		// also posts a $0 payment that links the invoice to the new credit
		// memo so the credit applies and the invoice settles.
		writeoffApply = &writeoffLink{
			invoiceID:     id,
			customer:      firstFlag(flags, "customer"),
			amount:        parseAmount(flags["amount"]),
			beforeInvoice: inv,
		}
		balance, valid := inv["Balance"].(float64)
		if !valid || writeoffApply.amount <= 0 || writeoffApply.amount > balance {
			return nil, fmt.Errorf("write-off requires an observed open balance covering the requested amount")
		}
	case "send":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/%s/%s/send?minorversion=73", ac.realm, path, id)
		if e := firstFlag(flags, "to", "email", "send-to"); e != "" {
			u += "&sendTo=" + url.QueryEscape(e)
		}
		resp, err := ac.postJSON(ctx, u, []byte("{}"))
		if err != nil {
			return nil, fmt.Errorf("v3 send %s: %w", entity, err)
		}
		defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
		got, err := readBody(resp)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
		}
		item := projectMutateItem(entity, got)
		return &MutateResult{Status: resp.StatusCode, Op: "send", Entity: entity, Item: item}, nil
	case "copy":
		if strings.TrimSpace(id) == "" {
			id = flags["id"]
		}
		if strings.TrimSpace(id) == "" {
			return nil, ErrMissingMutateID
		}
		existing, errf := fetchExisting(path, id)
		if errf != nil {
			return nil, errf
		}
		body = stripForCopy(existing)
		op = "create"
	default:
		return nil, fmt.Errorf("unsupported mutate op %q", op)
	}
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/%s?minorversion=73", ac.realm, path)
	if opQ != "" {
		u += "&operation=" + opQ
	}
	evidence, err := startMutationEvidence("v3-"+op, map[string]any{"entity": entity, "id": id, "before": before}, raw)
	if err != nil {
		return nil, err
	}
	submittingMutation(ctx)
	resp, err := ac.postJSON(ctx, u, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: v3 %s %s: %v (evidence %s)", ErrMutationUnverified, op, entity, err, evidence)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	got, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
	}
	item := projectMutateItem(entity, got)
	if id != "" && op != "create" && item.ID != id {
		return nil, fmt.Errorf("%w: receipt target mismatch (evidence %s)", ErrMutationUnverified, evidence)
	}
	if err := verifyV3Readback(ctx, ac, entity, op, item.ID, before, body, evidence); err != nil {
		return &MutateResult{Status: resp.StatusCode, Op: op, Entity: entity, Item: item, Evidence: evidence}, err
	}
	if writeoffApply != nil {
		if err := applyWriteoffCredit(ctx, ac, writeoffApply, item); err != nil {
			return nil, err
		}
	}
	confirmMutation(ctx)
	return &MutateResult{Status: resp.StatusCode, Op: op, Entity: entity, Item: item, Verified: true, Evidence: evidence}, nil
}

// writeoffLink carries the invoice write-off target through the shared
// mutate POST so the apply-payment can link the created credit memo.
type writeoffLink struct {
	invoiceID     string
	customer      string
	amount        float64
	beforeInvoice map[string]any
}

// applyWriteoffCredit posts the $0 payment that links the write-off credit
// memo to the invoice. Without it the credit memo sits unapplied and the
// invoice balance stays open, so the write-off is incomplete.
func applyWriteoffCredit(ctx context.Context, ac *apiClient, link *writeoffLink, cmItem QueryItem) error {
	if cmItem.ID == "" || link.invoiceID == "" || link.amount <= 0 {
		return fmt.Errorf("write-off apply requires invoice id, credit-memo id and a positive amount")
	}
	body := map[string]any{
		"CustomerRef": map[string]any{"value": link.customer},
		"TotalAmt":    0,
		"Line": []map[string]any{
			{"Amount": link.amount, "LinkedTxn": []map[string]any{{"TxnId": link.invoiceID, "TxnType": "Invoice"}}},
			{"Amount": link.amount, "LinkedTxn": []map[string]any{{"TxnId": cmItem.ID, "TxnType": "CreditMemo"}}},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/payment?minorversion=73", ac.realm)
	resp, err := ac.postJSON(ctx, u, raw)
	if err != nil {
		return fmt.Errorf("v3 write-off apply payment: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	got, err := readBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &ReplayError{Status: resp.StatusCode, Message: "write-off apply payment: " + errorMessage(got)}
	}
	item := projectMutateItem("Payment", got)
	if err := verifyV3Readback(ctx, ac, "Payment", "create", item.ID, nil, body, ""); err != nil {
		return err
	}
	invoice, err := fetchV3(ctx, ac, "invoice", link.invoiceID)
	if err != nil {
		return fmt.Errorf("%w: write-off invoice readback: %v", ErrMutationUnverified, err)
	}
	want := normalizedJSON(link.beforeInvoice).(map[string]any)
	want["Balance"] = want["Balance"].(float64) - link.amount
	delete(want, "LinkedTxn") // applying the payment intentionally appends this linkage
	if err := expectedFields(want, invoice, "Invoice"); err != nil {
		return fmt.Errorf("%w: write-off invoice: %v", ErrMutationUnverified, err)
	}
	credit, err := fetchV3(ctx, ac, "creditmemo", cmItem.ID)
	if err != nil || jsonNumberString(credit["Id"]) != cmItem.ID || credit["Balance"] != float64(0) {
		return fmt.Errorf("%w: credit memo consumption not independently proved", ErrMutationUnverified)
	}
	return nil
}

func PlannedMutateURL(entity, op string) string {
	if entity == "Cheque" {
		entity = "Purchase"
	}
	path := v3Path[entity]
	if path == "" {
		path = strings.ToLower(entity)
	}
	u := "https://qbo.intuit.com/api/v3/company/{realm}/" + path + "?minorversion=73"
	if op == "send" {
		u = "https://qbo.intuit.com/api/v3/company/{realm}/" + path + "/{id}/send?minorversion=73"
	}
	if op == "writeoff" {
		u = "https://qbo.intuit.com/api/v3/company/{realm}/creditmemo?minorversion=73"
	}
	if op == "delete" || op == "void" {
		u += "&operation=" + op
	}
	return u
}

func fetchV3(ctx context.Context, ac *apiClient, path, id string) (map[string]any, error) {
	id = sanitizeToken(id)
	if id == "" {
		return nil, ErrMissingMutateID
	}
	if path == "recurringtransaction" {
		if obj, err := fetchRecurringByID(ctx, ac, id); err == nil {
			return obj, nil
		}
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/%s/%s?minorversion=73", ac.realm, path, id)
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	for k, v := range wrap {
		if !strings.EqualFold(k, path) {
			continue
		}
		var obj map[string]any
		if json.Unmarshal(v, &obj) == nil && obj["Id"] != nil {
			return obj, nil
		}
	}
	return nil, fmt.Errorf("v3 get %s/%s: no entity object", path, id)
}

func fetchRecurringByID(ctx context.Context, ac *apiClient, id string) (map[string]any, error) {
	qs := "select * from RecurringTransaction maxresults 100"
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/query?query=%s&minorversion=73", ac.realm, url.QueryEscape(qs))

	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		QueryResponse map[string]json.RawMessage `json:"QueryResponse"`
	}
	if json.Unmarshal(raw, &wrap) != nil || wrap.QueryResponse == nil {
		return nil, fmt.Errorf("v3 get recurringtransaction/%s: no entity object", id)
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(wrap.QueryResponse["RecurringTransaction"], &rows) != nil {
		return nil, fmt.Errorf("v3 get recurringtransaction/%s: no entity object", id)
	}
	for _, row := range rows {
		for _, nest := range []string{"Invoice", "Bill", "Purchase", "Estimate", "JournalEntry", "SalesReceipt"} {
			raw, ok := row[nest]
			if !ok {
				continue
			}
			var obj map[string]any
			if json.Unmarshal(raw, &obj) != nil {
				continue
			}
			if fmt.Sprint(obj["Id"]) == id {
				return obj, nil
			}
		}
	}
	return nil, fmt.Errorf("v3 get recurringtransaction/%s: no entity object", id)
}

func buildCreateBody(entity string, flags map[string]string) (map[string]any, error) {
	out := map[string]any{}
	name := firstFlag(flags, "name", "title")
	date := transactionDateFlag(flags)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else {
		date = normalizeDate(date)
	}
	amt, amtSet, amtErr := numericFlag(flags, "amount")
	cust := firstFlag(flags, "customer")
	vend := firstFlag(flags, "supplier")
	item := firstFlag(flags, "item")
	acct := firstFlag(flags, "from-account", "account")
	toAcct := firstFlag(flags, "to-account")
	cashAccount := firstFlag(flags, "deposit-to", "refund-from", "account")

	switch entity {
	case "Customer", "Vendor", "Employee":
		if name == "" {
			return nil, fmt.Errorf("%s create requires --name", entity)
		}
		out["DisplayName"] = name
		if entity == "Vendor" {
			if tpar := firstFlag(flags, "tpar"); tpar != "" {
				out["Vendor1099"] = tpar == "true" || tpar == "1"
			}
		}
		if entity == "Employee" {
			given := firstFlag(flags, "given-name")
			if given == "" {
				given = name
			}
			out["GivenName"] = given
			family := firstFlag(flags, "family-name")
			if family == "" {
				family = "CLI"
			}
			out["FamilyName"] = family
		}
		if e := firstFlag(flags, "email"); e != "" {
			out["PrimaryEmailAddr"] = map[string]any{"Address": e}
		}
		if entity == "Customer" {
			line1 := firstFlag(flags, "address", "line1")
			if line1 == "" {
				line1 = "1 Test Street"
			}
			city := firstFlag(flags, "city")
			if city == "" {
				city = "Sydney"
			}
			out["BillAddr"] = map[string]any{
				"Line1":                  line1,
				"City":                   city,
				"CountrySubDivisionCode": firstFlag(flags, "state"),
				"PostalCode":             firstFlag(flags, "postcode", "postal"),
				"Country":                "AU",
			}
			if out["BillAddr"].(map[string]any)["CountrySubDivisionCode"] == "" {
				out["BillAddr"].(map[string]any)["CountrySubDivisionCode"] = "NSW"
			}
			if out["BillAddr"].(map[string]any)["PostalCode"] == "" {
				out["BillAddr"].(map[string]any)["PostalCode"] = "2000"
			}
		}
	case "Budget":
		if name == "" {
			name = "QB-CLI-TEST-BUDGET"
		}
		out["Name"] = name
		start := firstFlag(flags, "start-date")
		if start == "" {
			start = date
		}
		if len(start) == 4 {
			start = start + "-07-01"
		}
		out["StartDate"] = normalizeDate(start)
		end := firstFlag(flags, "end-date")
		if end == "" {
			// AU FY: 1 Jul -> 30 Jun next year.
			if t, err := time.Parse("2006-01-02", normalizeDate(start)); err == nil {
				end = t.AddDate(1, 0, -1).Format("2006-01-02")
			}
		}
		if end != "" {
			out["EndDate"] = normalizeDate(end)
		}
		typ := firstFlag(flags, "type")
		if strings.Contains(strings.ToLower(typ), "balance") {
			out["BudgetType"] = "BalanceSheet"
		} else {
			out["BudgetType"] = "ProfitAndLoss"
		}
		entry := firstFlag(flags, "entry-type", "interval")
		switch strings.ToLower(entry) {
		case "month", "monthly":
			out["BudgetEntryType"] = "Monthly"
		case "quarter", "quarterly":
			out["BudgetEntryType"] = "Quarterly"
		default:
			out["BudgetEntryType"] = "Yearly"
		}
		acctID := firstFlag(flags, "account")
		if acctID == "" {
			acctID = "7"
		}
		if amtErr != nil {
			return nil, amtErr
		}
		if !amtSet {
			amt = 1
		}
		out["BudgetDetail"] = []map[string]any{{
			"BudgetDate": normalizeDate(start),
			"AccountRef": map[string]any{"value": acctID},
			"Amount":     amt,
		}}

	case "RecurringTransaction":
		if name == "" {
			name = "QB-CLI-TEST-REC"
		}
		if cust == "" {
			return nil, fmt.Errorf("recurring create requires --customer")
		}
		if amtErr != nil {
			return nil, amtErr
		}
		if !amtSet {
			amt = 1
		}
		out["Invoice"] = map[string]any{
			"CustomerRef": map[string]any{"value": cust},
			"Line":        salesLines(flags, amt, amtSet, firstFlag(flags, "item")),
			"RecurringInfo": map[string]any{
				"Name":      name,
				"RecurType": "Unscheduled",
				"Active":    true,
			},
		}

	case "Project":
		if name == "" {
			return nil, fmt.Errorf("project create requires --name")
		}
		if cust == "" {
			return nil, fmt.Errorf("project create requires --customer")
		}
		out["Name"] = name
		out["CustomerRef"] = map[string]any{"value": cust}
	case "CreditCardCredit":
		ccAcct := firstFlag(flags, "credit-card-account", "cc-account")
		if ccAcct == "" {
			return nil, fmt.Errorf("credit-card-credit create requires --credit-card-account")
		}
		out["AccountRef"] = map[string]any{"value": ccAcct}
		out["TxnDate"] = date
		if vend != "" {
			out["EntityRef"] = map[string]any{"value": vend}
		}
		if amtErr != nil {
			return nil, amtErr
		}
		out["Line"] = expenseLines(flags, amt, amtSet, firstFlag(flags, "category-account", "account"))
	case "Class", "Department":
		if name == "" {
			return nil, fmt.Errorf("%s create requires --name", entity)
		}
		out["Name"] = name
	case "TaxAgency":
		if name == "" {
			return nil, fmt.Errorf("%s create requires --name", entity)
		}
		out["DisplayName"] = name
	case "TaxRate":
		if name == "" {
			return nil, fmt.Errorf("%s create requires --name", entity)
		}
		out["Name"] = name
		if rv := firstFlag(flags, "rate"); rv != "" {
			out["RateValue"] = rv
		}
		if ag := firstFlag(flags, "agency", "agency-id"); ag != "" {
			out["AgencyRef"] = map[string]any{"value": ag}
		}
	case "Term":
		if name == "" {
			return nil, fmt.Errorf("%s create requires --name", entity)
		}
		out["Name"] = name
		if typ := firstFlag(flags, "type"); typ != "" {
			out["Type"] = typ
		}
		if days, set, derr := numericFlag(flags, "due-days", "due_days", "duedays"); derr != nil {
			return nil, derr
		} else if set {
			out["DueDays"] = int(days)
		}
	case "PaymentMethod":
		if name == "" {
			return nil, fmt.Errorf("%s create requires --name", entity)
		}
		out["Name"] = name

	case "Account":
		if name == "" {
			return nil, fmt.Errorf("account create requires --name")
		}
		out["Name"] = name
		out["AccountType"] = normalizeAccountType(firstFlag(flags, "type"))
		if out["AccountType"] == "" {
			out["AccountType"] = "Expense"
		}
		if sub := firstFlag(flags, "subtype", "sub-type"); sub != "" {
			out["AccountSubType"] = sub
		}
	case "Item":
		if name == "" {
			return nil, fmt.Errorf("item create requires --name")
		}
		out["Name"] = name
		typ := firstFlag(flags, "type")
		if typ == "" {
			typ = "Service"
		}
		out["Type"] = typ
		inc := firstFlag(flags, "income-account")
		if inc == "" {
			inc = "1"
		}
		out["IncomeAccountRef"] = map[string]any{"value": inc}
		if strings.EqualFold(typ, "Inventory") {
			exp := firstFlag(flags, "expense-account", "cogs-account")
			if exp == "" {
				exp = "40"
			}
			asset := firstFlag(flags, "asset-account")
			if asset == "" {
				asset = "42"
			}
			out["ExpenseAccountRef"] = map[string]any{"value": exp}
			out["AssetAccountRef"] = map[string]any{"value": asset}
			out["TrackQtyOnHand"] = true
			qty, qtySet, qerr := numericFlag(flags, "qty", "quantity", "new-qty")
			if qerr != nil {
				return nil, qerr
			}
			if !qtySet {
				qty = 1
			}
			out["QtyOnHand"] = qty
			out["InvStartDate"] = date
		}
	case "Invoice", "Estimate", "CreditMemo", "SalesReceipt", "RefundReceipt", "DelayedCharge", "DelayedCredit":
		if cust == "" {
			return nil, fmt.Errorf("%s create requires --customer", entity)
		}
		out["CustomerRef"] = map[string]any{"value": cust}
		out["TxnDate"] = date
		if amtErr != nil {
			return nil, amtErr
		}
		out["Line"] = salesLines(flags, amt, amtSet, item)
		if e := firstFlag(flags, "email", "to"); e != "" {
			out["BillEmail"] = map[string]any{"Address": e}
		}
		if line1 := firstFlag(flags, "address", "line1"); line1 != "" || entity == "Invoice" {
			if line1 == "" {
				line1 = "1 Test Street"
			}
			city := firstFlag(flags, "city")
			if city == "" {
				city = "Sydney"
			}
			state := firstFlag(flags, "state")
			if state == "" {
				state = "NSW"
			}
			post := firstFlag(flags, "postcode", "postal")
			if post == "" {
				post = "2000"
			}
			out["BillAddr"] = map[string]any{
				"Line1": line1, "City": city, "CountrySubDivisionCode": state,
				"PostalCode": post, "Country": "AU",
			}
			if entity == "Invoice" {
				out["ShipAddr"] = out["BillAddr"]
				out["DeliveryInfo"] = map[string]any{"DeliveryType": "Email"}
			}
		}
		if entity == "SalesReceipt" || entity == "RefundReceipt" {
			if cashAccount == "" {
				cashAccount = firstFlag(flags, "refund-from", "deposit-to")
			}
			if cashAccount != "" {
				out["DepositToAccountRef"] = map[string]any{"value": cashAccount}
			}
		}
		if entity == "CreditMemo" {
			if inv := firstFlag(flags, "invoice-id", "id"); inv != "" {
				if lines, ok := out["Line"].([]map[string]any); ok && len(lines) > 0 {
					lines[0]["LinkedTxn"] = []map[string]any{{"TxnId": inv, "TxnType": "Invoice"}}
					out["Line"] = lines
				}
			}
		}
		if entity == "Invoice" {
			if ta := firstFlag(flags, "time-activity", "time-activity-id"); ta != "" {
				if lines, ok := out["Line"].([]map[string]any); ok && len(lines) > 0 {
					lines[0]["LinkedTxn"] = []map[string]any{{"TxnId": ta, "TxnType": "TimeActivity"}}
					out["Line"] = lines
				}
			}
		}
	case "Payment":
		if cust == "" {
			return nil, fmt.Errorf("payment create requires --customer")
		}
		out["CustomerRef"] = map[string]any{"value": cust}
		out["TxnDate"] = date
		if amtErr != nil {
			return nil, amtErr
		}
		if !amtSet {
			amt = 0.01
		}
		out["TotalAmt"] = amt
		if cashAccount != "" {
			out["DepositToAccountRef"] = map[string]any{"value": cashAccount}
		}
		if inv := firstFlag(flags, "invoice-id"); inv != "" {
			out["Line"] = []map[string]any{{
				"Amount":    amt,
				"LinkedTxn": []map[string]any{{"TxnId": inv, "TxnType": "Invoice"}},
			}}
		}
	case "Bill", "VendorCredit", "PurchaseOrder", "ItemReceipt":
		if vend == "" {
			return nil, fmt.Errorf("%s create requires --supplier", entity)
		}
		out["VendorRef"] = map[string]any{"value": vend}
		out["TxnDate"] = date
		if amtErr != nil {
			return nil, amtErr
		}
		out["Line"] = expenseLines(flags, amt, amtSet, firstFlag(flags, "category-account", "account"))
	case "BillPayment":
		if vend == "" {
			return nil, fmt.Errorf("bill-payment create requires --supplier")
		}
		billID := firstFlag(flags, "bill-id", "bill-ids", "id")
		if billID == "" {
			return nil, fmt.Errorf("bill-payment create requires --bill-id")
		}
		if ids := parseIDList(billID); len(ids) > 1 {
			return nil, fmt.Errorf("bill-payment create pays one bill per call (got %d ids); multi-bill allocation is not implemented", len(ids))
		} else if len(ids) == 1 {
			billID = ids[0]
		}
		payAcct := firstFlag(flags, "payment-account", "bank-account", "account")
		if payAcct == "" {
			return nil, fmt.Errorf("bill-payment create requires --account")
		}
		out["VendorRef"] = map[string]any{"value": vend}
		out["PayType"] = "Check"
		mode := strings.ToLower(firstFlag(flags, "pay-type", "payment-type", "type"))
		if strings.Contains(mode, "credit") {
			out["PayType"] = "CreditCard"
		}
		out["CheckPayment"] = map[string]any{"BankAccountRef": map[string]any{"value": payAcct}}
		if out["PayType"] == "CreditCard" {
			out["CreditCardPayment"] = map[string]any{"CCAccountRef": map[string]any{"value": payAcct}}
			delete(out, "CheckPayment")
		}
		out["TxnDate"] = date
		if amtErr != nil {
			return nil, amtErr
		}
		if !amtSet {
			amt = 0.01
		}
		out["TotalAmt"] = amt
		out["Line"] = []map[string]any{{
			"Amount":    amt,
			"LinkedTxn": []map[string]any{{"TxnId": billID, "TxnType": "Bill"}},
		}}
	case "Purchase":
		// --account, --payment-account and --bank-account are aliases for
		// the cash side; previously only --account fed payAcct, so the
		// catalog-required payment flags led nowhere and minimal calls
		// failed with "requires --account".
		payAcct := firstFlag(flags, "payment-account", "bank-account", "account")
		if payAcct == "" {
			payAcct = cashAccount // --account and its aliases are the unified cash side
		}
		// --asset-account names a balance-sheet line account (prepaid
		// vouchers etc.) and takes precedence over the expense default;
		// without it a gift-asset purchase could silently post to
		// expense account 7.
		lineAcct := firstFlag(flags, "asset-account", "category-account")
		if lineAcct == "" && firstFlag(flags, "payment-account", "bank-account") != "" {
			// With an explicit cash side, --account names the expense/category account.
			lineAcct = firstFlag(flags, "account")
		}
		if payAcct == "" {
			return nil, fmt.Errorf("purchase create requires --account")
		}
		if lineAcct == "" {
			lineAcct = "7"
		}
		if lineAcct == payAcct {
			return nil, fmt.Errorf("purchase create requires a category --category-account distinct from the payment account")
		}
		out["PaymentType"] = "Cash"
		mode := strings.ToLower(firstFlag(flags, "mode", "type", "payment-type"))
		switch {
		case strings.Contains(mode, "check") || strings.Contains(mode, "cheque") || flags["cheque"] != "":
			out["PaymentType"] = "Check"
		case strings.Contains(mode, "credit"):
			out["PaymentType"] = "CreditCard"
		}
		out["AccountRef"] = map[string]any{"value": payAcct}
		out["TxnDate"] = date
		if vend != "" {
			out["EntityRef"] = map[string]any{"value": vend}
		}
		if amtErr != nil {
			return nil, amtErr
		}
		out["Line"] = expenseLines(flags, amt, amtSet, lineAcct)

	case "JournalEntry":
		out["TxnDate"] = date
		if raw := firstFlag(flags, "lines", "line-items"); raw != "" && strings.HasPrefix(strings.TrimSpace(raw), "[") {
			var parsed []map[string]any
			if json.Unmarshal([]byte(raw), &parsed) == nil && len(parsed) > 0 {
				out["Line"] = parsed
				break
			}
		}
		if amtErr != nil {
			return nil, amtErr
		}
		from := firstFlag(flags, "from-account")
		to := firstFlag(flags, "to-account")
		if from == "" || to == "" || amt == 0 {
			return nil, fmt.Errorf("journal create requires --from-account --to-account --amount")
		}
		out["Line"] = []map[string]any{
			{"Amount": amt, "DetailType": "JournalEntryLineDetail", "JournalEntryLineDetail": map[string]any{
				"PostingType": "Debit", "AccountRef": map[string]any{"value": from},
			}},
			{"Amount": amt, "DetailType": "JournalEntryLineDetail", "JournalEntryLineDetail": map[string]any{
				"PostingType": "Credit", "AccountRef": map[string]any{"value": to},
			}},
		}
	case "Transfer":
		if acct == "" || toAcct == "" {
			return nil, fmt.Errorf("transfer create requires --from-account --to-account")
		}
		if amtErr != nil {
			return nil, amtErr
		}
		out["FromAccountRef"] = map[string]any{"value": acct}
		out["ToAccountRef"] = map[string]any{"value": toAcct}
		out["Amount"] = amt
		out["TxnDate"] = date
	case "Deposit":
		dep := cashAccount
		if dep == "" {
			return nil, fmt.Errorf("deposit create requires --account")
		}
		out["DepositToAccountRef"] = map[string]any{"value": dep}
		out["TxnDate"] = date
		if raw := firstFlag(flags, "line-items", "lines"); raw != "" && strings.HasPrefix(strings.TrimSpace(raw), "[") {
			var parsed []map[string]any
			if json.Unmarshal([]byte(raw), &parsed) == nil && len(parsed) > 0 {
				out["Line"] = parsed
				break
			}
		}
		if amtErr != nil {
			return nil, amtErr
		}
		src := firstFlag(flags, "category-account", "from-account")
		if src == "" {
			src = firstFlag(flags, "to-account")
		}
		if src == "" {
			src = "4"
		}
		out["Line"] = []map[string]any{{
			"Amount":     amt,
			"DetailType": "DepositLineDetail",
			"DepositLineDetail": map[string]any{
				"AccountRef": map[string]any{"value": src},
			},
		}}
	case "TimeActivity":
		out["TxnDate"] = date
		if err := applyTimeActivityFields(out, flags, true); err != nil {
			return nil, err
		}
	case "InventoryAdjustment":
		itemID := firstFlag(flags, "item")
		adj := firstFlag(flags, "account", "adjust-account")
		if itemID == "" || adj == "" {
			return nil, fmt.Errorf("inventory adjust create requires --item and --account")
		}
		qty, qtySet, qerr := numericFlag(flags, "qty", "quantity", "new-qty")
		if qerr != nil {
			return nil, qerr
		}
		if !qtySet {
			if amtErr != nil {
				return nil, amtErr
			}
			qty = amt
			if !amtSet {
				qty = 1
			}
		}
		out["TxnDate"] = date
		doc := firstFlag(flags, "doc-number", "number")
		if doc == "" {
			doc = "QB-ADJ-" + time.Now().Format("150405")
		}
		out["DocNumber"] = doc
		out["AdjustAccountRef"] = map[string]any{"value": adj}
		out["Line"] = []map[string]any{{
			"DetailType": "ItemAdjustmentLineDetail",
			"ItemAdjustmentLineDetail": map[string]any{
				"ItemRef": map[string]any{"value": itemID},
				"QtyDiff": qty,
			},
		}}
	case "CompanyCurrency":
		code := firstFlag(flags, "code", "name")
		if code == "" {
			return nil, fmt.Errorf("currency create requires --code")
		}
		out["Code"] = strings.ToUpper(code)
	case "TaxCode":
		if name == "" {
			name = firstFlag(flags, "period")
		}
		if name == "" {
			return nil, fmt.Errorf("tax code create requires --name")
		}
		out["Name"] = name
	default:
		if name != "" {
			out["Name"] = name
		}
		if date != "" {
			out["TxnDate"] = date
		}
	}
	if due := firstFlag(flags, "due-date"); due != "" && (entity == "Invoice" || entity == "Bill") {
		out["DueDate"] = normalizeDate(due)
	}
	if memo := firstFlag(flags, "memo", "notes"); memo != "" {
		out["PrivateNote"] = memo
	}
	return out, nil
}

func buildUpdateBody(entity string, flags map[string]string, existing map[string]any) (map[string]any, error) {
	out := map[string]any{
		"Id":        existing["Id"],
		"SyncToken": existing["SyncToken"],
		"sparse":    true,
	}
	a, amountSet, amountErr := numericFlag(flags, "amount")
	if amountErr != nil {
		return nil, amountErr
	}
	if entity == "Transfer" {
		copyExisting(out, existing, "FromAccountRef", "ToAccountRef", "Amount")
	}
	if entity == "Deposit" {
		copyExisting(out, existing, "DepositToAccountRef", "Line")
	}
	if entity == "JournalEntry" {
		copyExisting(out, existing, "Line", "TxnDate", "CurrencyRef")
		if raw := firstFlag(flags, "amount"); raw != "" {
			amount, err := strconv.ParseFloat(raw, 64)
			if err != nil || amount == 0 {
				return nil, fmt.Errorf("journal amount must be a non-zero number")
			}
			lines, ok := existing["Line"].([]any)
			if !ok || len(lines) != 2 {
				return nil, fmt.Errorf("journal amount updates require a balanced two-line entry")
			}
			updated := make([]any, 0, 2)
			kinds := map[string]bool{}
			for _, value := range lines {
				line, ok := value.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid journal line")
				}
				detail, ok := line["JournalEntryLineDetail"].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid journal detail")
				}
				kinds[fmt.Sprint(detail["PostingType"])] = true
				copy := maps.Clone(line)
				copy["Amount"] = amount
				updated = append(updated, copy)
			}
			if !kinds["Debit"] || !kinds["Credit"] {
				return nil, fmt.Errorf("journal must have one debit and one credit")
			}
			out["Line"] = updated
		}
	}
	if entity == "Item" {
		copyExisting(out, existing, "Type", "IncomeAccountRef", "ExpenseAccountRef")
	}
	if entity == "Bill" || entity == "VendorCredit" || entity == "PurchaseOrder" || entity == "ItemReceipt" {
		copyExisting(out, existing, "VendorRef", "Line", "CurrencyRef")
		if lines := lineItemsFlag(flags); lines != nil {
			out["Line"] = lines
		}
	}
	if entity == "CreditCardCredit" {
		copyExisting(out, existing, "AccountRef", "Line", "TxnDate", "EntityRef", "CurrencyRef")
		if lines := lineItemsFlag(flags); lines != nil {
			out["Line"] = lines
		}
	}
	if entity == "Project" {
		copyExisting(out, existing, "CustomerRef", "Status", "Description")
		if cust := firstFlag(flags, "customer"); cust != "" {
			out["CustomerRef"] = map[string]any{"value": cust}
		}
	}
	if entity == "BillPayment" {
		copyExisting(out, existing, "VendorRef", "APAccountRef", "PayType", "Line", "TotalAmt", "CurrencyRef")
	}
	if entity == "Payment" {
		copyExisting(out, existing, "CustomerRef", "DepositToAccountRef", "Line", "TotalAmt", "TxnDate", "PaymentRefNum")
		if firstFlag(flags, "amount") != "" {
			if lines, ok := existing["Line"].([]any); ok && len(lines) > 1 {
				return nil, fmt.Errorf("payment amount updates require a single allocation; multi-invoice reallocation is not supported")
			}
		}
	}
	if entity == "Purchase" {
		copyExisting(out, existing, "AccountRef", "PaymentType", "EntityRef", "Line", "CurrencyRef")
		if err := CheckLineItemsJSON(flags); err != nil {
			return nil, err
		}
		if lines := lineItemsFlag(flags); lines != nil {
			out["Line"] = lines
		}
	}
	if entity == "TimeActivity" {
		copyExisting(out, existing, "NameOf", "EmployeeRef", "VendorRef", "CustomerRef", "ItemRef", "Hours", "Minutes", "HourlyRate", "TxnDate", "Description", "BillableStatus")
		if err := applyTimeActivityFields(out, flags, false); err != nil {
			return nil, err
		}
	}
	if entity == "Class" {
		copyExisting(out, existing, "Active", "SubClass", "ParentRef")
	}
	if entity == "Budget" {
		// Server rejects nameless budget updates (6000); carry it over.
		copyExisting(out, existing, "Name")
	}
	if entity == "Invoice" || entity == "Estimate" || entity == "CreditMemo" {
		copyExisting(out, existing, "CustomerRef", "Line", "TxnDate", "BillEmail", "BillAddr")
	}
	if entity == "Preferences" {
		copyExisting(out, existing, "SalesFormsPrefs", "AccountingInfoPrefs", "VendorAndPurchasesPrefs", "TimeTrackingPrefs", "TaxPrefs", "CurrencyPrefs", "ProductAndServicesPrefs", "ReportPrefs", "OtherPrefs")
	}
	if entity == "SalesReceipt" || entity == "RefundReceipt" {
		// Without copied lines the server recomputes the total from the
		// old lines and TotalAmt-only updates silently do nothing.
		copyExisting(out, existing, "Line", "CustomerRef", "TxnDate", "DepositToAccountRef")
	}
	if entity == "Term" {
		// Server revalidates the whole term on update; Name-only sparse
		// bodies 400 demanding day-of-month (TC2 2026-09-08). Carry the
		// schedule fields so renames keep DueDays/Type intact.
		copyExisting(out, existing, "Type", "DueDays", "DiscountPercent", "DiscountDays", "DayOfMonthDue", "DueNextMonthDays")
	}

	if name := firstFlag(flags, "name", "title"); name != "" {
		switch entity {
		case "Customer", "Vendor", "Employee":
			out["DisplayName"] = name
		case "CompanyInfo":
			out["CompanyName"] = name
		case "Attachable":
			out["FileName"] = name
		case "RecurringTransaction":
			info := map[string]any{"Name": name, "RecurType": "Unscheduled"}
			if existing != nil {
				if prev, ok := existing["RecurringInfo"].(map[string]any); ok {
					if rt := fmt.Sprint(prev["RecurType"]); rt != "" && rt != "<nil>" {
						info["RecurType"] = rt
					}
				}
			}
			out["RecurringInfo"] = info
		case "Transfer":
		default:
			out["Name"] = name
		}
	}
	if entity == "Vendor" {
		if tpar := firstFlag(flags, "tpar"); tpar != "" {
			out["Vendor1099"] = tpar == "true" || tpar == "1"
		}
	}
	if due := firstFlag(flags, "due-date"); due != "" && (entity == "Invoice" || entity == "Bill") {
		out["DueDate"] = normalizeDate(due)
	}
	if memo := firstFlag(flags, "memo", "notes"); memo != "" {
		if entity == "Purchase" {
			previous, _ := existing["PrivateNote"].(string)
			merged, err := preserveManagedMemo(previous, memo)
			if err != nil {
				return nil, err
			}
			memo = merged
		}
		out["PrivateNote"] = memo
	}
	if d := transactionDateFlag(flags); d != "" {
		out["TxnDate"] = normalizeDate(d)
	}
	if amountSet {
		if entity == "Transfer" {
			out["Amount"] = a
		} else {
			out["TotalAmt"] = a
			applyLineAmount(out, a)
		}
		if entity == "RecurringTransaction" {
			// fetchRecurringByID returns the nested template itself, so
			// its lines live on existing (not existing["Invoice"]). Copy
			// them or amount updates silently do nothing.
			copyExisting(out, existing, "Line", "CustomerRef", "TxnDate")
			applyLineAmount(out, a)
		}
	}
	if disc := parseAmount(firstFlag(flags, "discount")); disc != 0 && entity == "Invoice" {
		lines, _ := out["Line"].([]map[string]any)
		if lines == nil {
			if raw, ok := existing["Line"].([]any); ok {
				for _, r := range raw {
					if m, ok := r.(map[string]any); ok {
						lines = append(lines, m)
					}
				}
			}
		}
		lines = append(lines, map[string]any{
			"Amount":     disc,
			"DetailType": "DiscountLineDetail",
			"DiscountLineDetail": map[string]any{
				"PercentBased": false,
			},
		})
		out["Line"] = lines
		out["sparse"] = false
	} else if pct := parseAmount(firstFlag(flags, "discount-percent", "percent")); pct != 0 && entity == "Invoice" {
		lines, _ := out["Line"].([]map[string]any)
		if lines == nil {
			if raw, ok := existing["Line"].([]any); ok {
				for _, r := range raw {
					if m, ok := r.(map[string]any); ok {
						lines = append(lines, m)
					}
				}
			}
		}
		amt := parseAmount(fmt.Sprint(existing["TotalAmt"]))
		lines = append(lines, map[string]any{
			"Amount":     amt * pct / 100,
			"DetailType": "DiscountLineDetail",
			"DiscountLineDetail": map[string]any{
				"PercentBased":    true,
				"DiscountPercent": pct,
			},
		})
		out["Line"] = lines
		out["sparse"] = false
	}
	if e := firstFlag(flags, "email"); e != "" {
		switch entity {
		case "Invoice", "Estimate", "CreditMemo", "SalesReceipt", "RefundReceipt", "DelayedCharge", "DelayedCredit":
			out["BillEmail"] = map[string]any{"Address": e}
		default:
			out["PrimaryEmailAddr"] = map[string]any{"Address": e}
		}
	}
	switch strings.ToLower(firstFlag(flags, "active")) {
	case "true":
		out["Active"] = true
	case "false":
		out["Active"] = false
	}
	return out, nil
}

// applyTimeActivityFields keeps native Hours and Minutes separate. Empty flag
// values mean omitted (collectFlags also includes unset CLI flags); explicit
// zero must overwrite existing values rather than selecting a default.
func applyTimeActivityFields(out map[string]any, flags map[string]string, create bool) error {
	if create {
		out["Hours"], out["Minutes"], out["HourlyRate"] = 0, 0, 0.0
		if firstFlag(flags, "hours", "minutes") == "" {
			// Preserve the existing one-hour default only for omitted duration.
			out["Hours"] = 1
		}
	}
	for _, field := range []struct{ flag, key string }{{"hours", "Hours"}, {"minutes", "Minutes"}} {
		if raw := firstFlag(flags, field.flag); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return fmt.Errorf("time activity --%s must be a non-negative whole number", field.flag)
			}
			if field.flag == "minutes" && value > 59 {
				return fmt.Errorf("time activity --minutes must be between 0 and 59; use --hours for whole hours")
			}
			out[field.key] = value
		}
	}
	if raw := firstFlag(flags, "rate"); raw != "" {
		rate, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			return fmt.Errorf("time activity --rate must be a finite non-negative number")
		}
		out["HourlyRate"] = rate
	}

	emp, vendor := firstFlag(flags, "employee"), firstFlag(flags, "vendor", "supplier")
	nameOf := firstFlag(flags, "name-of")
	if nameOf == "" {
		switch {
		case emp != "":
			nameOf = "Employee"
		case vendor != "":
			nameOf = "Vendor"
		case create:
			return fmt.Errorf("time activity create requires --employee or --vendor")
		}
	}
	if nameOf != "" {
		switch {
		case strings.EqualFold(nameOf, "Employee"):
			if emp != "" {
				out["EmployeeRef"] = map[string]any{"value": emp}
			}
			if out["EmployeeRef"] == nil {
				return fmt.Errorf("time activity NameOf Employee requires --employee")
			}
			out["NameOf"] = "Employee"
			delete(out, "VendorRef")
		case strings.EqualFold(nameOf, "Vendor"):
			if vendor != "" {
				out["VendorRef"] = map[string]any{"value": vendor}
			}
			if out["VendorRef"] == nil {
				return fmt.Errorf("time activity NameOf Vendor requires --vendor")
			}
			out["NameOf"] = "Vendor"
			delete(out, "EmployeeRef")
		default:
			return fmt.Errorf("time activity --name-of must be Employee or Vendor")
		}
	}
	if customer := firstFlag(flags, "customer", "customer-id"); customer != "" {
		out["CustomerRef"] = map[string]any{"value": customer}
	}
	if item := firstFlag(flags, "item", "item-id"); item != "" {
		out["ItemRef"] = map[string]any{"value": item}
	}
	if description := firstFlag(flags, "description"); description != "" {
		out["Description"] = description
	}
	if billable := firstFlag(flags, "billable"); billable != "" {
		switch strings.ToLower(billable) {
		case "true", "1", "yes", "billable":
			out["BillableStatus"] = "Billable"
		case "false", "0", "no", "notbillable", "not-billable":
			out["BillableStatus"] = "NotBillable"
		default:
			out["BillableStatus"] = billable
		}
	}
	return nil
}

// stripForCopy removes server-managed fields from a fetched entity so the
// remaining object can be POSTed as a new create. Id, SyncToken, MetaData,
// DocNumber, and any LinkedTxn references are stripped; everything else
// (CustomerRef, Line, TxnDate, etc.) is preserved for the clone.
func stripForCopy(existing map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range existing {
		switch k {
		case "Id", "SyncToken", "MetaData", "DocNumber", "CustomField":
			continue
		}
		out[k] = v
	}
	// Strip LinkedTxn from each line so the clone is not linked to the
	// original transaction.
	if lines, ok := out["Line"].([]map[string]any); ok {
		for i := range lines {
			delete(lines[i], "LinkedTxn")
		}
		out["Line"] = lines
	}
	return out
}

func salesLines(flags map[string]string, amt float64, amtSet bool, item string) []map[string]any {
	if raw := firstFlag(flags, "line-items", "lines"); raw != "" && strings.HasPrefix(strings.TrimSpace(raw), "[") {
		var parsed []map[string]any
		if json.Unmarshal([]byte(raw), &parsed) == nil && len(parsed) > 0 {
			return parsed
		}
	}
	if !amtSet {
		amt = 1
	}
	if item == "" {
		item = "1"
	}
	lines := []map[string]any{{
		"Amount":     amt,
		"DetailType": "SalesItemLineDetail",
		"SalesItemLineDetail": map[string]any{
			"ItemRef": map[string]any{"value": item},
		},
	}}
	if disc := parseAmount(firstFlag(flags, "discount")); disc != 0 {
		lines = append(lines, map[string]any{
			"Amount":     disc,
			"DetailType": "DiscountLineDetail",
			"DiscountLineDetail": map[string]any{
				"PercentBased": false,
			},
		})
	} else if pct := parseAmount(firstFlag(flags, "discount-percent", "percent")); pct != 0 {
		lines = append(lines, map[string]any{
			"Amount":     amt * pct / 100,
			"DetailType": "DiscountLineDetail",
			"DiscountLineDetail": map[string]any{
				"PercentBased":    true,
				"DiscountPercent": pct,
			},
		})
	}
	return lines
}

// lineItemsFlag parses the JSON line array shared by --line-items, --lines,
// and the purchase-order --items alias (the generated catalog name). Returns
// nil when absent or unparsable so callers can apply their default path.
func lineItemsFlag(flags map[string]string) []map[string]any {
	raw := strings.TrimSpace(firstFlag(flags, "line-items", "lines", "items"))
	if raw == "" || !strings.HasPrefix(raw, "[") {
		return nil
	}
	var parsed []map[string]any
	if json.Unmarshal([]byte(raw), &parsed) != nil || len(parsed) == 0 {
		return nil
	}
	return parsed
}

func expenseLines(flags map[string]string, amt float64, amtSet bool, acct string) []map[string]any {
	if lines := lineItemsFlag(flags); lines != nil {
		return lines
	}
	if !amtSet {
		amt = 1
	}
	if acct == "" {
		acct = "7"
	}
	return []map[string]any{{
		"Amount":     amt,
		"DetailType": "AccountBasedExpenseLineDetail",
		"AccountBasedExpenseLineDetail": map[string]any{
			"AccountRef": map[string]any{"value": acct},
		},
	}}
}

func projectMutateItem(entity string, body []byte) QueryItem {
	var wrap map[string]json.RawMessage
	if json.Unmarshal(body, &wrap) != nil {
		return QueryItem{Type: entity}
	}
	raw, ok := wrap[entity]
	if !ok && entity == "RecurringTransaction" {
		raw, ok = wrap["Invoice"]
	}
	if !ok {
		return QueryItem{Type: entity}
	}
	items := projectQuery(entity, []byte(`{"QueryResponse":{"`+entity+`":[`+string(raw)+`]}}`))
	if entity == "RecurringTransaction" && len(items) == 0 {
		items = projectQuery(entity, []byte(`{"QueryResponse":{"RecurringTransaction":[{"Invoice":`+string(raw)+`}]}}`))
	}
	if len(items) == 1 {
		return items[0]
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		return QueryItem{ID: jsonString(obj, "Id"), Type: entity}
	}
	return QueryItem{Type: entity}
}

func copyExisting(out, existing map[string]any, keys ...string) {
	for _, k := range keys {
		if v := existing[k]; v != nil {
			if _, ok := out[k]; !ok {
				out[k] = v
			}
		}
	}
}

func applyLineAmount(out map[string]any, amt float64) {
	raw, ok := out["Line"]
	if !ok {
		return
	}
	switch lines := raw.(type) {
	case []map[string]any:
		if len(lines) > 0 {
			lines[0]["Amount"] = amt
			out["Line"] = lines
		}
	case []any:
		if len(lines) == 0 {
			return
		}
		if m, ok := lines[0].(map[string]any); ok {
			m["Amount"] = amt
			lines[0] = m
			out["Line"] = lines
		}
	}
}

func firstItemRef(obj map[string]any) string {
	raw, ok := obj["Line"]
	if !ok {
		return ""
	}
	var lines []any
	switch v := raw.(type) {
	case []any:
		lines = v
	case []map[string]any:
		for _, m := range v {
			lines = append(lines, m)
		}
	}
	for _, r := range lines {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		for _, detKey := range []string{"SalesItemLineDetail", "ItemBasedExpenseLineDetail"} {
			det, ok := m[detKey].(map[string]any)
			if !ok {
				continue
			}
			if iref, ok := det["ItemRef"].(map[string]any); ok {
				if id := fmt.Sprint(iref["value"]); id != "" && id != "<nil>" {
					return id
				}
			}
		}
	}
	return ""
}

// transactionDateFlag accepts the domain date names published in qb actions.
func transactionDateFlag(flags map[string]string) string {
	return firstFlag(flags, "date", "txn-date", "invoice-date", "bill-date", "purchase-date", "quote-date", "payment-date", "refund-date", "charge-date", "credit-date")
}

func firstFlag(flags map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(flags[k]); v != "" {
			return v
		}
	}
	return ""
}

func parseAmount(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// numericFlag reads an explicitly provided numeric flag, distinguishing an
// omitted flag (set=false) from an explicit finite zero (set=true, value=0).
// Invalid or non-finite input is rejected so it can never silently coerce
// to a builder default before POST; callers only consult the error on code
// paths that actually consume the flag.
func numericFlag(flags map[string]string, keys ...string) (value float64, set bool, err error) {
	for _, k := range keys {
		if v := strings.TrimSpace(flags[k]); v != "" {
			f, perr := strconv.ParseFloat(v, 64)
			if perr != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return 0, true, fmt.Errorf("--%s must be a finite number, got %q", k, v)
			}
			return f, true, nil
		}
	}
	return 0, false, nil
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 10 && s[2] == '/' {
		// dd/MM/yyyy -> yyyy-MM-dd
		return s[6:10] + "-" + s[3:5] + "-" + s[0:2]
	}
	return s
}

// voidableEntities maps CLI entity names to v3 entities that accept
// ?operation=void. Empirically on Test Company 2 (2026-09-17): invoice void
// succeeds; purchase and estimate both return "Operation void is not
// supported" (estimates are non-posting; purchase void is not offered on
// this realm's v3). Purchase-order is non-posting too — excluded.
var voidableEntities = map[string]string{
	"invoice":          "Invoice",
	"payment":          "Payment",
	"salesreceipt":     "SalesReceipt",
	"sales-receipt":    "SalesReceipt",
	"bill":             "Bill",
	"creditmemo":       "CreditMemo",
	"credit-memo":      "CreditMemo",
	"creditcardcredit": "CreditCardCredit",
	"vendorcredit":     "VendorCredit",
	"vendor-credit":    "VendorCredit",
	"deposit":          "Deposit",
	"transfer":         "Transfer",
	"journalentry":     "JournalEntry",
	"journal":          "JournalEntry",
	"refundreceipt":    "RefundReceipt",
	"refund-receipt":   "RefundReceipt",
}

// VoidableEntity resolves a --entity flag value into a v3 entity name that
// supports ?operation=void, or an error listing what is missing.
func VoidableEntity(name string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "", fmt.Errorf("void requires --entity (invoice, payment, salesreceipt, bill, ...)")
	}
	if ent, ok := voidableEntities[key]; ok {
		return ent, nil
	}
	return "", fmt.Errorf("entity %q cannot be voided via v3; voidable: invoice, payment, salesreceipt, bill, creditmemo, vendorcredit, creditcardcredit, deposit, transfer, journalentry, refundreceipt", name)
}
