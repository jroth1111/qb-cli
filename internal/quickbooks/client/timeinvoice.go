package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ReplayTimeInvoice creates invoices for unbilled (Billable) TimeActivity
// rows — the CLI equivalent of the invoice form's "Suggested transactions:
// Billable time → Add" flow. With --customer it bills one customer; without
// it the sweep creates one invoice per customer that has unbilled time.
//
// Contract proven live on TC2 2026-09-18: the invoice form's save rides
// createTransactions_Transaction / UpdateTransactions_Transaction_qbo on
// qbo.intuit.com/api/v4/graphql where each billed item line carries
//
//	links.sources[] {id: <v4TxnId>, sourceTxn{id: <v4TxnId>, type:"ACTIVITY_TIME"}}
//
// The v4 txn id is NOT the v3 TimeActivity id — the v3 entity exposes it as
// TimeChargeId (activity 1073741831 → charge 162). Consuming a source through
// links is the only path that flips the activity to HasBeenBilled server-side:
// a v3 sparse BillableStatus write returns 200 but is silently ignored, and a
// v3 LinkedTxn persists the reference without consuming the charge (both
// verified live).
//
// --auto is not implemented: "automatically invoice unbilled activity" is a
// company-automation setting whose UI route 404s in this build and whose
// settings endpoint was never captured. Passing it fails loudly.
func ReplayTimeInvoice(ctx context.Context, customer, auto string) (*MutateResult, error) {
	if auto = strings.TrimSpace(auto); auto != "" && auto != "false" && auto != "0" {
		return nil, fmt.Errorf("--auto (automatic unbilled-time invoicing) is a company-automation setting not available on this build; omit it or pass --auto false")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	customer = strings.TrimSpace(customer)
	stmt := "select * from TimeActivity where BillableStatus = 'Billable'"
	if customer != "" {
		cid, err := resolveNameID(ctx, "Customer", customer)
		if err != nil {
			return nil, fmt.Errorf("time-invoice customer: %w", err)
		}
		stmt = fmt.Sprintf("select * from TimeActivity where CustomerRef = '%s' and BillableStatus = 'Billable'", cid)
	}
	rows, err := queryV3Rows(ctx, ac, "TimeActivity", stmt)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if customer != "" {
			return nil, fmt.Errorf("no unbilled (Billable) time activity for customer %q", customer)
		}
		return nil, fmt.Errorf("no unbilled (Billable) time activity on file")
	}
	// v3 BillableStatus lags the v4 consumption write — an activity already
	// linked to an invoice can keep reporting Billable for a while (observed
	// live on TC2: activity 1073741831 invoiced via INV-9006 still read
	// Billable). The charge node's links.targets is authoritative: skip
	// charges already consumed or the save mutation rejects the whole
	// invoice ("already been invoiced").
	fresh := rows[:0]
	for _, row := range rows {
		chargeID := fmt.Sprint(row["TimeChargeId"])
		if chargeID == "" || chargeID == "<nil>" {
			return nil, fmt.Errorf("time activity %s has no TimeChargeId — cannot link it to an invoice", row["Id"])
		}
		consumed, err := chargeConsumed(ctx, ac, chargeID)
		if err != nil {
			return nil, err
		}
		if !consumed {
			fresh = append(fresh, row)
		}
	}
	rows = fresh
	if len(rows) == 0 {
		return nil, fmt.Errorf("unbilled time found but every charge is already linked to an invoice")
	}
	// Group activities by customer so the sweep produces one invoice each.
	groups := map[string][]map[string]any{}
	order := []string{}
	for _, row := range rows {
		ref, _ := row["CustomerRef"].(map[string]any)
		cid := fmt.Sprint(ref["value"])
		if cid == "" || cid == "<nil>" {
			continue // activities without a customer cannot be invoiced
		}
		if _, ok := groups[cid]; !ok {
			order = append(order, cid)
		}
		groups[cid] = append(groups[cid], row)
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("unbilled time found but none carries a CustomerRef")
	}
	var (
		firstID  string
		created  []string
		billed   int
		lastName string
		total    float64
	)
	for _, cid := range order {
		invID, custName, amt, err := invoiceUnbilledGroup(ctx, ac, cid, groups[cid])
		if err != nil {
			return nil, err
		}
		if firstID == "" {
			firstID = invID
		}
		lastName = custName
		total += amt
		created = append(created, invID)
		billed += len(groups[cid])
	}
	note := fmt.Sprintf("%d time activit%s billed across %d invoice(s)", billed, plural(billed), len(created))
	if len(created) > 1 {
		note += ": " + strings.Join(created, ",")
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "create",
		Entity: "Invoice",
		Item:   QueryItem{ID: firstID, Name: lastName, Amount: total},
		Note:   note,
	}, nil
}

// invoiceUnbilledGroup creates one SALE_INVOICE via
// UpdateTransactions_Transaction_qbo — the invoice form's own save mutation —
// where each billed line consumes its activity's v4 charge through
// links.sources{id,sourceLine,sourceTxn} plus a transaction-level
// links.sources entry carrying the full sourceTxn stub. Contract captured
// live on TC2 2026-09-19 (invoice INV-9006). Returns the invoice txn id,
// customer display name and invoiced total.
func invoiceUnbilledGroup(ctx context.Context, ac *apiClient, cid string, acts []map[string]any) (string, string, float64, error) {
	itemLines := make([]any, 0, len(acts))
	srcLines := make([]any, 0, len(acts))
	total := 0.0
	today := time.Now().Format("2006-01-02")
	custName, _ := actCustomerName(acts)
	for i, act := range acts {
		chargeID, _ := act["TimeChargeId"].(string)
		if chargeID == "" {
			chargeID = fmt.Sprint(act["TimeChargeId"])
		}
		if chargeID == "" || chargeID == "<nil>" {
			return "", "", 0, fmt.Errorf("time activity %s has no TimeChargeId — cannot link it to an invoice", act["Id"])
		}
		qty, rate, desc, itemID := timeActivityLine(act)
		amt := qty * rate
		total += amt
		amtS := fmt.Sprintf("%.2f", amt)
		svcDate := strFieldOr(act, "TxnDate", today)
		line := map[string]any{
			"description": desc,
			"projectId":   "",
			"amount":      amtS,
			"id":          fmt.Sprintf("ITEM_client_%d", i),
			"qboAppData":  map[string]any{},
			"traits": map[string]any{
				"subTotal": map[string]any{},
				"item": map[string]any{
					"quantity":            trimNum(qty),
					"rateType":            "HOURLY_RATE",
					"rate":                trimNum(rate),
					"unitConversionRatio": 1,
					"serviceDate":         svcDate,
					"item":                map[string]any{"id": itemID},
					"bundleDetails":       map[string]any{},
				},
				"deferredRecognition": map[string]any{},
				"purchaseOrder":       map[string]any{},
				"progressivelyBilled": map[string]any{},
				"billable":            map[string]any{},
			},
			"links": map[string]any{
				"sources": []map[string]any{{
					"id":         chargeID,
					"sourceLine": map[string]any{"sequence": "1", "amount": amtS, "totalAmountConsumed": "0.00"},
					"sourceTxn":  map[string]any{"id": chargeID, "type": "ACTIVITY_TIME"},
				}},
				"targets": []any{},
			},
			"contact": map[string]any{"id": cid},
		}
		// The activity's NameOf party rides the line as vendor/employee —
		// captured vendor:{id:3} for a vendor-owned time charge.
		switch strings.ToUpper(strFieldOr(act, "NameOf", "")) {
		case "VENDOR":
			if ref, ok := act["VendorRef"].(map[string]any); ok {
				line["vendor"] = map[string]any{"id": fmt.Sprint(ref["value"])}
			}
		case "EMPLOYEE":
			if ref, ok := act["EmployeeRef"].(map[string]any); ok {
				line["employee"] = map[string]any{"id": fmt.Sprint(ref["value"])}
			}
		}
		itemLines = append(itemLines, line)
		srcLines = append(srcLines, map[string]any{
			"id": chargeID,
			"sourceTxn": map[string]any{
				"id":          chargeID,
				"type":        "ACTIVITY_TIME",
				"itemLines":   []map[string]any{{"account": map[string]any{}}},
				"header":      map[string]any{"amount": amtS, "txnDate": svcDate},
				"attachments": []any{},
			},
		})
	}
	txn := map[string]any{
		"id":            txnDraftID(ac),
		"entityVersion": "0",
		"stageEntity":   nil,
		"meta":          map[string]any{},
		"qboAppData":    map[string]any{"txnTypeId": "4", "eligibleToReclaimTaxWithFRS": false, "purchSaleLocationEnabled": false},
		"header": map[string]any{
			"projectId": "",
			"contact": map[string]any{
				"displayName": custName,
				"externalIds": []map[string]any{{"namespaceId": "intuit.qbo.name.id", "localId": cid}},
				"id":          cid,
				"qboAppData":  map[string]any{"projectRef": nil},
			},
			"convenienceFee":  map[string]any{"ach": map[string]any{}},
			"transactionType": "SALE_INVOICE",
			"txnDate":         today,
			"currencyInfo": map[string]string{
				"symbol": "A$", "homeAmount": fmt.Sprintf("%.2f", total), "code": "AUD",
				"exchangeRate": "1.00", "name": "Australian Dollar", "currency": "AUD",
			},
			"regionExtensions": map[string]any{"defaultTxnSubType": map[string]any{}},
			"voided":           false,
			"account":          map[string]any{},
			"tags":             []any{},
		},
		"type":         "SALE_INVOICE",
		"lines":        map[string]any{"itemLines": itemLines, "accountLines": []any{}},
		"attachments":  []any{},
		"customFields": []any{},
		"links":        map[string]any{"sources": srcLines, "targets": []any{}},
		"traits": map[string]any{
			"costEstimate":    map[string]any{},
			"esignature":      map[string]any{},
			"delivery":        map[string]any{"printStatus": "NOT_SET", "type": "DONT"},
			"shipping":        map[string]any{"shipAddress": map[string]any{"freeFormAddressLine": ""}, "shipFromAddress": map[string]any{}},
			"discount":        map[string]any{},
			"style":           map[string]any{"stylePreferences": map[string]string{"id": invoicesStyleID}},
			"prePayment":      map[string]any{},
			"balance":         map[string]any{"balance": "0"},
			"invoiceDeposit":  nil,
			"payment":         map[string]any{"paymentDetailsMessage": nil, "paymentMethod": map[string]any{"id": nil}, "processedPayment": map[string]any{}, "qboAppData": map[string]any{}, "creditCardInfo": map[string]any{}, "checkInfo": map[string]any{}},
			"expirationTrait": map[string]any{},
			"transfer":        map[string]any{"fromAccount": map[string]any{}, "toAccount": map[string]any{}},
			"acceptStatus":    map[string]any{"acceptStatus": "PENDING"},
			"tax":             map[string]any{},
			"bankDeposit":     map[string]any{"cashBackAccount": map[string]any{}},
			"journal":         map[string]any{},
		},
	}
	payload, err := json.Marshal(map[string]any{
		"query": `mutation UpdateTransactions_Transaction_qbo($input: UpdateTransactions_TransactionInput!) {
			updateTransactions_Transaction(input: $input) {
				clientMutationId
				transactionsTransaction { id entityVersion }
			}
		}`,
		"variables": map[string]any{"input": map[string]any{
			"clientMutationId":        "0",
			"transactionsTransaction": txn,
		}},
	})
	if err != nil {
		return "", "", 0, err
	}
	extra := map[string]string{
		"Referer":      "https://qbo.intuit.com/app/invoice",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, invoicesImportGraphQLURL, payload, extra)
	if err != nil {
		return "", "", 0, fmt.Errorf("time-invoice create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", "", 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
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
		return "", "", 0, fmt.Errorf("parsing time-invoice response: %w", err)
	}
	if len(out.Errors) > 0 {
		return "", "", 0, fmt.Errorf("time-invoice create: %s", out.Errors[0].Message)
	}
	invID := out.Data.Mut.Txn.ID
	if invID == "" {
		return "", "", 0, fmt.Errorf("time-invoice create: no node id in response: %.300s", raw)
	}
	// The mutation returns the Relay node id; the trailing segment is the
	// transaction id shown to the user.
	if i := strings.LastIndex(invID, ":"); i >= 0 {
		invID = invID[i+1:]
	}
	return invID, custName, total, nil
}

// txnDraftID renders the UI's draft-transaction sentinel: the transaction
// node namespace plus ":-1". The namespace is the realm-embedded constant the
// invoice form itself sends (v4TxNSTestCompany2).
func txnDraftID(ac *apiClient) string {
	if ac.realm == v4TxNSRealm {
		return v4TxNSTestCompany2 + ":-1"
	}
	return ":-1"
}

// txnNodeID renders the Relay node id for a Transactions_Transaction. The
// namespace decodes to "v4.1:<realm>:80271edd8a" — the constant the invoice
// nodes carry (see chequeprint.go).
func txnNodeID(ac *apiClient, txnID string) string {
	if ac.realm == v4TxNSRealm {
		return v4TxNSTestCompany2 + ":" + txnID
	}
	return base64.RawURLEncoding.EncodeToString([]byte("v4.1:"+ac.realm+":80271edd8a")) + ":" + txnID
}

// chargeConsumed reads the charge node's links.targets — the authoritative
// consumption marker. links:null means the charge is still uninvoiced; a
// target edge means some transaction already consumed it (verified live:
// charge 162 targeted by invoice 170 / INV-9006).
func chargeConsumed(ctx context.Context, ac *apiClient, chargeID string) (bool, error) {
	payload, err := json.Marshal(map[string]any{
		"query":     `query c($id: ID!) { node(id: $id) { ... on Transactions_Transaction { id type links { targets { edges { node { id targetTxn { id type } } } } } } } }`,
		"variables": map[string]any{"id": txnNodeID(ac, chargeID)},
	})
	if err != nil {
		return false, err
	}
	resp, err := ac.postJSONExtra(ctx, invoicesImportGraphQLURL, payload, map[string]string{
		"Referer":      "https://qbo.intuit.com/app/invoice",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return false, fmt.Errorf("checking charge %s: %w", chargeID, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out struct {
		Data struct {
			Node struct {
				Type  string `json:"type"`
				Links *struct {
					Targets struct {
						Edges []struct {
							Node struct {
								TargetTxn struct {
									Type string `json:"type"`
								} `json:"targetTxn"`
							} `json:"node"`
						} `json:"edges"`
					} `json:"targets"`
				} `json:"links"`
			} `json:"node"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, fmt.Errorf("parsing charge %s: %w", chargeID, err)
	}
	if len(out.Errors) > 0 {
		return false, fmt.Errorf("checking charge %s: %s", chargeID, out.Errors[0].Message)
	}
	if out.Data.Node.Type == "" {
		return false, fmt.Errorf("charge %s: no ACTIVITY_TIME transaction on file", chargeID)
	}
	if out.Data.Node.Type != "ACTIVITY_TIME" {
		return false, fmt.Errorf("charge %s resolves to %s, not ACTIVITY_TIME", chargeID, out.Data.Node.Type)
	}
	return out.Data.Node.Links != nil && len(out.Data.Node.Links.Targets.Edges) > 0, nil
}

func actCustomerName(acts []map[string]any) (string, bool) {
	for _, act := range acts {
		if ref, ok := act["CustomerRef"].(map[string]any); ok {
			if n, ok := ref["name"].(string); ok && n != "" {
				return n, true
			}
		}
	}
	return "", false
}

// timeActivityLine extracts qty (hours + minutes/60), hourly rate,
// description and item id from a raw TimeActivity row.
func timeActivityLine(act map[string]any) (qty, rate float64, desc, itemID string) {
	qty = numField(act, "Hours") + numField(act, "Minutes")/60
	rate = numField(act, "HourlyRate")
	desc, _ = act["Description"].(string)
	if ref, ok := act["ItemRef"].(map[string]any); ok {
		itemID = fmt.Sprint(ref["value"])
	}
	return qty, rate, desc, itemID
}

func numField(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

func strFieldOr(m map[string]any, key, fallback string) string {
	if s, ok := m[key].(string); ok && s != "" {
		return s
	}
	return fallback
}

// trimNum renders a quantity/rate the way the invoice form sends it —
// plain decimal, no trailing zeros.
func trimNum(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// queryV3Rows runs a raw v3 select and returns the entity collection as
// decoded objects — unlike ReplayQuery, fields are not projected away.
func queryV3Rows(ctx context.Context, ac *apiClient, entity, stmt string) ([]map[string]any, error) {
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/query?minorversion=73&query=%s", ac.realm, url.QueryEscape(stmt))
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("v3 query %s: %w", entity, err)
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
		return nil, fmt.Errorf("v3 query %s: missing QueryResponse", entity)
	}
	var rows []map[string]any
	if raw := wrap.QueryResponse[entity]; raw != nil {
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, fmt.Errorf("v3 query %s: %w", entity, err)
		}
	}
	return rows, nil
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
