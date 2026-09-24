package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Captured *.api.intuit.com read surfaces, live-proven on Test Company 2
// (2026-09-17): identity Users, settingsfacade qbAppFoundationQbSettings,
// customextensions CustomFieldsQueryCES, cashflowplanningsvc
// GetCashflowSummary, experimentassignment /api/v6/assignments, appflow
// /v1/integrations, paymentaccount-sbg /v2/accounts/{realm}.

const identityUsersDoc = `query Users($input: Identity_UserFilterInput!, $pagination: PaginationInput!) {
        identityUsers(input: $input, pagination: $pagination) {
            edges {
                node {
                    profile {
                        id
                        accountId
                        personInfo {
                            contactInfo { emails { email } }
                            name { familyName givenName }
                        }
                    }
                }
            }
        }
    }`

const identityAccountDoc = `query ($input: AccountInput!) {
  account (input: $input) {
    accountProfile {
      profileType
      personInfo {
        name { familyName givenName }
        contactInfo { phoneNumbers { kind originalNumber } emails { email } }
      }
    }
  }
  identityDigitalIdentity { username tenantId }
}`

const settingsFacadeDoc = `query accountingCoreCompanySettingsQuery {
  qbAppFoundationQbSettings {
    finance {
      accounting {
        accountingCore {
          accountingCoreSettings {
            closeBookPasswordEnabled
            closeBookDateEnabled
            closeBookDate
            accountNumberVisible
          }
        }
      }
    }
  }
}`

const customExtDoc = `query CustomFieldsQueryCES {
  customFieldDefinitions(limit: 1000) {
    edges {
      node {
        id
        name
        deleted
        schema { type title format }
        associatedEntityTypes { type entityConditions { subtype } }
      }
    }
  }
}`

const cashflowSummaryDoc = `query GetCashflowSummary($input: CashflowSummaryInput!) {
  getCashflowSummary(input: $input) {
    summaryStartDate
    summaryEndDate
    summarySpan
    source
    summaryData { startDate endDate moneyIn moneyOut endingBalance summaryType }
    lastModifiedInUTC
  }
}`

// apixGraphQL POSTs a captured GraphQL document to an api.intuit.com host and
// projects the response through edgePath (object payloads project as one row).
func apixGraphQL(ctx context.Context, ac *apiClient, entity, url, host string, doc string, vars map[string]any, edgePath []string, note string, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 50
	}
	payload, err := json.Marshal(map[string]any{"query": doc, "variables": vars})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, url, host, payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", note, err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", note, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return projectEdgeNodes(entity, note, raw, edgePath, query, limit, resp.StatusCode), nil
}

// ReplayIdentityUsers lists company users via identity.api.intuit.com
// identityUsers — proven live on TC2 (returns Hang Bopp).
func ReplayIdentityUsers(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	vars := map[string]any{
		"input":      map[string]any{"accountFilter": map[string]any{"accountId": ac.realm}},
		"pagination": map[string]any{"first": 50},
	}
	return apixGraphQL(ctx, ac, "User", "https://identity.api.intuit.com/v2/graphql",
		"identity.api.intuit.com", identityUsersDoc, vars,
		[]string{"data", "identityUsers", "edges"}, "identity.api.intuit.com identityUsers", query, limit)
}

// ReplayIdentityAccount reads the session identity profile via
// identity.api.intuit.com account/identityDigitalIdentity.
func ReplayIdentityAccount(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	// AccountInput.id is the realm (organization account) — proven live; the
	// auth id validates shape but 404s with "Account does not exist".
	vars := map[string]any{"input": map[string]any{"id": ac.realm}}
	return apixGraphQL(ctx, ac, "IdentityAccount", "https://identity.api.intuit.com/v2/graphql",
		"identity.api.intuit.com", identityAccountDoc, vars,
		[]string{"data", "account"}, "identity.api.intuit.com account", query, limit)
}

// ReplaySettingsFacade reads accounting-core company settings via
// settingsfacade.api.intuit.com — proven live on TC2.
func ReplaySettingsFacade(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return apixGraphQL(ctx, ac, "SettingsFacade", "https://settingsfacade.api.intuit.com/graphql",
		"settingsfacade.api.intuit.com", settingsFacadeDoc, map[string]any{},
		[]string{"data", "qbAppFoundationQbSettings"}, "settingsfacade qbAppFoundationQbSettings", query, limit)
}

// ReplayCustomExtensions lists per-entity custom extension recommendations via
// customextensions.api.intuit.com CustomFieldsQueryCES.
func ReplayCustomExtensions(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return apixGraphQL(ctx, ac, "CustomExtension", "https://customextensions.api.intuit.com/graphql",
		"customextensions.api.intuit.com", customExtDoc, map[string]any{},
		[]string{"data", "customFieldDefinitions", "edges"}, "customextensions CustomFieldsQueryCES", query, limit)
}

// ReplayCashflowSummary reads the cash-flow forecast summary via
// cashflowplanningsvc.api.intuit.com GetCashflowSummary — proven live on TC2
// (real summaryData rows).
func ReplayCashflowSummary(ctx context.Context, query string, limit int) (*QueryResult, error) {
	vars := map[string]any{"input": map[string]any{
		"endDate":         "2027-06-30",
		"startDate":       "2025-09-01",
		"currentDate":     "2026-09-17",
		"summarySpan":     "DAILY",
		"source":          "BANK",
		"forecastEnabled": true,
		"refreshCache":    false,
	}}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return apixGraphQL(ctx, ac, "CashflowSummary", "https://cashflowplanningsvc.api.intuit.com/graphql",
		"cashflowplanningsvc.api.intuit.com", cashflowSummaryDoc, vars,
		[]string{"data", "getCashflowSummary", "summaryData"}, "cashflowplanningsvc GetCashflowSummary", query, limit)
}

// experimentAssignmentDoc is the captured POST body for
// experimentassignment.api.intuit.com /api/v6/assignments (REST, not GraphQL).
var experimentAssignmentDoc = map[string]any{
	"entityId": map[string]any{
		"AUTH_ID":             "9411837596151089",
		"REALM_OR_COMPANY_ID": "9341457769756854",
		"ns":                  "QBO",
	},
	"paramsWithFilter": map[string]any{
		"assignmentFilter": map[string]any{
			"applications": []string{"QBO"},
			"countries":    []string{"AU"},
		},
		"assignmentParams": map[string]any{
			"contextMap": map[string]any{
				"intuit-webapp-id": "qbo-web-app-experience",
			},
			"assignmentOptions": map[string]any{"allowRemoteAssgmnts": true},
		},
	},
}

// ReplayExperimentAssignments returns A/B experiment bucket assignments via
// experimentassignment.api.intuit.com — proven live on TC2 (real assignment rows).
func ReplayExperimentAssignments(ctx context.Context, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 50
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	doc := experimentAssignmentDoc
	doc["entityId"].(map[string]any)["REALM_OR_COMPANY_ID"] = ac.realm
	if uid := headerGetFold(ac.tok.RequestHeaders, "intuit-user-id"); uid != "" {
		doc["entityId"].(map[string]any)["AUTH_ID"] = uid
	}
	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost,
		"https://experimentassignment.api.intuit.com/api/v6/assignments/businessUnits/SBSEG/users",
		"experimentassignment.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("experimentassignment assignments: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("experimentassignment assignments: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("experimentassignment assignments: %w", err)
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(arr))
	for _, a := range arr {
		rawb, _ := json.Marshal(a)
		if q != "" && !strings.Contains(strings.ToLower(string(rawb)), q) {
			continue
		}
		id := fmt.Sprintf("%v", a["experimentId"])
		name := fmt.Sprintf("%v", a["experimentKey"])
		items = append(items, QueryItem{ID: id, Name: name, Type: fmt.Sprintf("%v", a["treatmentKey"])})
		if len(items) >= limit {
			break
		}
	}
	return &QueryResult{Entity: "ExperimentAssignment", Status: resp.StatusCode,
		Counts: map[string]int{"items": len(items), "totalCount": len(arr)}, Items: items,
		Note: "experimentassignment /api/v6/assignments"}, nil
}

// ReplayAppflowIntegrations GETs appflow /v1/integrations — proven live on TC2
// (200, empty data list).
func ReplayAppflowIntegrations(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet,
		"https://appflow.api.intuit.com/v1/integrations",
		"appflow.api.intuit.com", nil)
	if err != nil {
		return nil, fmt.Errorf("appflow integrations: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("appflow integrations: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("appflow integrations: %w", err)
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(doc.Data))
	for _, a := range doc.Data {
		rawb, _ := json.Marshal(a)
		if q != "" && !strings.Contains(strings.ToLower(string(rawb)), q) {
			continue
		}
		items = append(items, QueryItem{ID: fmt.Sprintf("%v", a["id"]), Name: fmt.Sprintf("%v", a["name"])})
		if len(items) >= limit {
			break
		}
	}
	return &QueryResult{Entity: "AppflowIntegration", Status: resp.StatusCode,
		Counts: map[string]int{"items": len(items), "totalCount": len(doc.Data)}, Items: items,
		Note: "appflow /v1/integrations"}, nil
}

// ReplayPaymentAccount GETs paymentaccount-sbg /v2/accounts/{realm} — on TC2
// (no payments onboarding) the service answers typed 404 ACCOUNT_0002, which is
// the honest "no payment account" result for this company.
func ReplayPaymentAccount(ctx context.Context) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet,
		"https://paymentaccount-sbg.api.intuit.com/v2/accounts/"+ac.realm,
		"paymentaccount-sbg.api.intuit.com", nil)
	if err != nil {
		return nil, fmt.Errorf("paymentaccount accounts: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("paymentaccount accounts: %w", err)
	}
	res := &QueryResult{Entity: "PaymentAccount", Status: resp.StatusCode,
		Counts: map[string]int{"items": 0}, Items: []QueryItem{},
		Note: "paymentaccount-sbg /v2/accounts"}
	if resp.StatusCode == http.StatusNotFound {
		res.Note += " (typed 404: no payment account onboarded)"
		return res, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err == nil {
		res.Items = []QueryItem{{ID: ac.realm, Name: "payment-account"}}
		res.Counts["items"] = 1
		_ = doc
	}
	return res, nil
}

// apixGet issues a captured GET against an api.intuit.com host and returns the
// decoded body (any JSON shape) plus status.
func apixGet(ctx context.Context, ac *apiClient, entity, url, host, note string) (any, int, error) {
	resp, err := ac.doURIHost(ctx, http.MethodGet, url, host, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", note, err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", note, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		// Non-JSON payloads (b2bbillpay/bill-pay-onboarding answer XML) still
		// project honestly — one row carrying the raw body snippet.
		return map[string]any{"rawBody": strings.TrimSpace(string(raw))}, resp.StatusCode, nil
	}
	return doc, resp.StatusCode, nil
}

// apixGetItems projects a decoded GET body onto QueryItems: arrays map to rows;
// objects map to a single row. Substring-filters on the raw node blob.
func apixGetItems(entity, note string, doc any, status int, query string, limit int) *QueryResult {
	if limit < 1 {
		limit = 50
	}
	var rows []any
	switch v := doc.(type) {
	case []any:
		rows = v
	case map[string]any:
		for _, key := range []string{"items", "data", "dashboards", "entities", "results"} {
			if arr, ok := v[key].([]any); ok {
				rows = arr
				break
			}
		}
		if rows == nil {
			rows = []any{v}
		}
	default:
		rows = []any{}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(rows))
	for _, r := range rows {
		raw, _ := json.Marshal(r)
		if q != "" && !strings.Contains(strings.ToLower(string(raw)), q) {
			continue
		}
		it := projectJSONItems(entity, []json.RawMessage{raw}, itemKeys{
			id:   []string{"id", "Id", "ID", "globalId", "entityId"},
			name: []string{"name", "Name", "label", "title", "displayName", "legacyName"},
		})
		if len(it) == 0 {
			it = []QueryItem{{Type: entity}}
		}
		items = append(items, it[0])
		if len(items) >= limit {
			break
		}
	}
	return &QueryResult{Entity: entity, Status: status,
		Counts: map[string]int{"items": len(items), "totalCount": len(rows)},
		Items:  items, Note: note}
}

// ReplaySalesOverview posts the captured allSales aggregate query to
// universalreportsgraphql — the sales-overview cards (unbilled, estimates,
// open/overdue invoices, last-30-days paid). Live on TC2.
func ReplaySalesOverview(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	from := time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	to := time.Now().Format("2006-01-02")
	vars := map[string]any{
		"unbilledFilter":    map[string]any{"fromDate": from, "toDate": to},
		"estimatesFilter":   map[string]any{"date": map[string]any{"fromDate": from, "toDate": to}},
		"invoiceFilter":     map[string]any{"date": map[string]any{"fromDate": from, "toDate": to}, "dueStatus": "OVERDUE"},
		"openInvoiceFilter": map[string]any{"date": map[string]any{"fromDate": from, "toDate": to}},
	}
	payload, err := json.Marshal(map[string]any{"query": allSalesDoc, "variables": vars})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost,
		"https://universalreportsgraphql.api.intuit.com/graphql?intuit-company-id="+ac.realm,
		"universalreportsgraphql.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("allSales: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("allSales: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("allSales: %w", err)
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(doc.Data))
	for k, v := range doc.Data {
		rawb, _ := json.Marshal(v)
		if q != "" && !strings.Contains(strings.ToLower(k+string(rawb)), q) {
			continue
		}
		items = append(items, QueryItem{ID: k, Name: k, Type: "SalesOverview"})
		if len(items) >= limit {
			break
		}
	}
	return &QueryResult{Entity: "SalesOverview", Status: resp.StatusCode,
		Counts: map[string]int{"items": len(items)}, Items: items,
		Note: "universalreportsgraphql allSales aggregates"}, nil
}

const allSalesDoc = `query allSales(
  $unbilledFilter: DateFilter!,
  $estimatesFilter: MetricFilter!,
  $invoiceFilter: InvoicesFilter!,
  $openInvoiceFilter: InvoicesFilter!
) {
  unbilled(filter: $unbilledFilter) {
      aggregate { total(field: AMOUNT) { value } count(field: ID) { value } }
  }
  estimates(filter: $estimatesFilter) {
      aggregate { total(field: AMOUNT) { value } count(field: ID) { value } }
  },
  overdueInvoice: invoices(filter: $invoiceFilter) {
      aggregate { total(field: AMOUNT) { value }, count(field: AMOUNT) { value } },
      dueStatus
  },
  openInvoices: invoices(filter: $openInvoiceFilter) {
      aggregate { total(field: AMOUNT) { value }, count(field: AMOUNT) { value } }
  },
  last30daysPaid { total { value } count { value } }
}`

// ReplayExpensesOverview GETs dashboardframework /v1/dashboards?name=
// EXPENSES_DASHBOARD — the spend-overview dashboard definition.
func ReplayExpensesOverview(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	doc, status, err := apixGet(ctx, ac, "ExpensesOverview",
		"https://dashboardframework.api.intuit.com/v1/dashboards?name=EXPENSES_DASHBOARD",
		"dashboardframework.api.intuit.com", "dashboardframework EXPENSES_DASHBOARD")
	if err != nil {
		return nil, err
	}
	return apixGetItems("ExpensesOverview", "dashboardframework EXPENSES_DASHBOARD", doc, status, query, limit), nil
}

// ReplaySalesTxnRisk GETs salestxnrisksvc /v2/risk/eligibility — whether the
// sales surface is risk-eligible for this company.
func ReplaySalesTxnRisk(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	doc, status, err := apixGet(ctx, ac, "SalesTxnRisk",
		"https://salestxnrisksvc.api.intuit.com/v2/risk/eligibility?intuit-company-id="+ac.realm+"&actionName=app-sales",
		"salestxnrisksvc.api.intuit.com", "salestxnrisksvc risk/eligibility")
	if err != nil {
		return nil, err
	}
	return apixGetItems("SalesTxnRisk", "salestxnrisksvc risk/eligibility", doc, status, query, limit), nil
}

// ReplayBillPayOnboarding GETs bill-pay-onboarding-svc
// /v1/bill-payment-eligibility — bill-pay enrolment state.
func ReplayBillPayOnboarding(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	doc, status, err := apixGet(ctx, ac, "BillPayOnboarding",
		"https://bill-pay-onboarding-svc.api.intuit.com/v1/bill-payment-eligibility",
		"bill-pay-onboarding-svc.api.intuit.com", "bill-pay-onboarding-svc bill-payment-eligibility")
	if err != nil {
		return nil, err
	}
	return apixGetItems("BillPayOnboarding", "bill-pay-onboarding-svc bill-payment-eligibility", doc, status, query, limit), nil
}

// ReplayStageTransactions POSTs the captured StageEntity query to
// stagetransactions /stage/entities — receipts/bills staged for review.
func ReplayStageTransactions(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	to := time.Now().Format("2006-01-02")
	from := time.Now().AddDate(0, 0, -46).Format("2006-01-02")
	body := []any{map[string]any{
		"$type": "/Query",
		"preparedQuery": map[string]any{
			"type": "/integration/StageEntity",
			"where": map[string]any{
				"$type": "/query/CompoundExpression", "op": "&&",
				"args": []any{
					map[string]any{"$type": "/query/FilterExpression", "args": []any{"RECEIPT_UPLOAD", "B2B_NETWORK", "WORKFORCE_EXP_MGMT"}, "op": "in", "property": "transactionTrait.transactionSource"},
					map[string]any{"$type": "/query/FilterExpression", "op": "in", "property": "transaction_type", "args": []any{"PURCHASE_BILL"}},
					map[string]any{"$type": "/query/FilterExpression", "op": "==", "property": "state", "args": []any{"TO_BE_REVIEWED"}},
					map[string]any{"$type": "/query/FilterExpression", "op": "in", "property": "transactionTrait.receiptProcessingState", "args": []any{"PROCESSED", "MISSING_DATA", "ERROR"}},
					map[string]any{"$type": "/query/FilterExpression", "op": "between", "property": "transactionTrait.transaction.header.txnDate", "args": []any{from, to}},
				},
			},
			"limit": 3000, "offset": 0,
		},
	}}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost,
		"https://stagetransactions.api.intuit.com/stage/entities",
		"stagetransactions.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("stagetransactions entities: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("stagetransactions entities: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("stagetransactions entities: %w", err)
	}
	return apixGetItems("StageTransaction", "stagetransactions /stage/entities", doc, resp.StatusCode, query, limit), nil
}

// ReplayB2BBillpay GETs b2bbillpay paymentApproval pendingInstructions +
// indicators — B2B bill-pay payment status.
func ReplayB2BBillpay(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	pend, ps, err := apixGet(ctx, ac, "B2BBillpay",
		"https://b2bbillpay.api.intuit.com/v1/paymentApproval/pendingInstructions",
		"b2bbillpay.api.intuit.com", "b2bbillpay pendingInstructions")
	if err != nil {
		return nil, err
	}
	ind, _, ierr := apixGet(ctx, ac, "B2BBillpay",
		"https://b2bbillpay.api.intuit.com/v1/paymentApproval/indicators",
		"b2bbillpay.api.intuit.com", "b2bbillpay indicators")
	res := apixGetItems("B2BBillpay", "b2bbillpay paymentApproval", pend, ps, query, limit)
	if ierr == nil {
		if m, ok := ind.(map[string]any); ok {
			raw, _ := json.Marshal(m)
			res.Items = append(res.Items, QueryItem{ID: "indicators", Name: "indicators", Type: "B2BBillpay"})
			res.Counts["items"]++
			_ = raw
		}
	}
	return res, nil
}

// ReplayB2BNetwork GETs b2bnetworkservice /v2/directory/attributes — supplier
// network directory attributes.
func ReplayB2BNetwork(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	doc, status, err := apixGet(ctx, ac, "B2BNetwork",
		"https://b2bnetworkservice.api.intuit.com/v2/directory/attributes",
		"b2bnetworkservice.api.intuit.com", "b2bnetworkservice directory/attributes")
	if err != nil {
		return nil, err
	}
	return apixGetItems("B2BNetwork", "b2bnetworkservice directory/attributes", doc, status, query, limit), nil
}

// ReplayTaxConfigGroups posts IndirectTaxTaxGropups_taxconfig to
// taxconfig.api.intuit.com — the production tax-group catalogue the ledger UI
// reads (previously only the non-prod -test mirror was documented).
func ReplayTaxConfigGroups(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return apixGraphQL(ctx, ac, "TaxConfigGroup", "https://taxconfig.api.intuit.com/graphql",
		"taxconfig.api.intuit.com", taxConfigGroupsDoc, map[string]any{"first": 9999},
		[]string{"data", "indirectTaxTaxGroups", "edges"}, "taxconfig IndirectTaxTaxGropups", query, limit)
}

const taxConfigGroupsDoc = `query IndirectTaxTaxGropups_taxconfig($first: PositiveInt) {
  indirectTaxTaxGroups(first: $first) {
    edges {
      node {
        globalId
        legacyName
        legacyHidden
        activeStatus
        taxRates {
          edges {
            node {
              globalId
              applicableRates {
                applicableOn
                effectiveRates { ratePercentage __typename }
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}`

const budgetsDoc = `query fetchAllBudgets($first: Int, $after: String, $last: Int, $before: String, $sortBy: BusinessPlanning_SortByColumn, $sortDirection: BusinessPlanning_SortDirection, $budgetFilters: BusinessPlanning_BudgetFilters) {
  businessPlanningBudgets(first: $first, after: $after, last: $last, before: $before, sortBy: $sortBy, sortDirection: $sortDirection, budgetFilters: $budgetFilters) {
    pageInfo {
      hasPreviousPage
      hasNextPage
      startCursor
      endCursor
      __typename
    }
    edges {
      cursor
      node {
        budgetId
        budgetName
        budgetType
        startDate
        endDate
        linkedEntityId
        intervalType
        syncToken
        secondaryListType
        dimensionDefId
        budgetMetaData {
          createdBy
          createdAt
          lastUpdatedBy
          updatedAt
          __typename
        }
        viewSettings {
          archived
          __typename
        }
        approvalDetails {
          approvalStatus
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}

`

// ReplayBudgetPlanning runs the captured fetchAllBudgets query against
// budgeting.api.intuit.com — the BusinessPlanning budget list (distinct from
// the v3 budget entity read). Live on TC2.
func ReplayBudgetPlanning(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 10
	}
	vars := map[string]any{
		"last": nil, "first": limit, "after": nil, "before": nil,
		"sortBy": "LAST_MODIFIED", "sortDirection": "DESC",
		"budgetFilters": map[string]any{"archived": false, "budgetType": []string{"PROFIT_AND_LOSS", "BALANCE_SHEET"}},
	}
	return apixGraphQL(ctx, ac, "BudgetPlanning", "https://budgeting.api.intuit.com/graphql",
		"budgeting.api.intuit.com", budgetsDoc, vars,
		[]string{"data", "businessPlanningBudgets", "edges"}, "budgeting fetchAllBudgets", query, limit)
}

// SALES_SETTINGS_GET — "sales salesettings get". Captured on TC2 2026-09-19:
// Account and settings > Sales issues readSettings against
// salesettings.api.intuit.com/v4/graphql under the qbo-settings-ui plugin.
// On this AU company the salesSettings resolver answers null — the service
// has no row — which the CLI reports honestly (same shape as
// ReplayPaymentAccount's typed 404). x-csrf-token is session-bound and must
// ride fresh from the live cookie jar, not the captured header snapshot.
const salesSettingsDoc = `query readSettings {
    company {
        id
        salesSettings {
            entityVersion
            showFullyConsumedLines
        }
    }
}`

// ReplaySalesSettings replays the captured readSettings document.
func ReplaySalesSettings(ctx context.Context, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{"query": salesSettingsDoc, "variables": map[string]any{}})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost,
		"https://salesettings.api.intuit.com/v4/graphql?intuit-company-id="+ac.realm,
		"salesettings.api.intuit.com", payload,
		map[string]string{"x-csrf-token": ac.csrf})
	if err != nil {
		return nil, fmt.Errorf("salesettings readSettings: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("salesettings readSettings: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data struct {
			Company struct {
				ID            string          `json:"id"`
				SalesSettings json.RawMessage `json:"salesSettings"`
			} `json:"company"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("salesettings readSettings: malformed response %.200s", raw)
	}
	if len(doc.Errors) > 0 && doc.Errors[0].Message != "" {
		return nil, fmt.Errorf("salesettings readSettings: %s", doc.Errors[0].Message)
	}
	res := &QueryResult{Entity: "SalesSettings", Status: resp.StatusCode,
		Counts: map[string]int{"items": 0}, Items: []QueryItem{},
		Note: "salesettings readSettings"}
	if len(doc.Data.Company.SalesSettings) == 0 || string(doc.Data.Company.SalesSettings) == "null" {
		res.Note += " (null: no sales-settings row provisioned for this company)"
		return res, nil
	}
	var ss map[string]any
	if err := json.Unmarshal(doc.Data.Company.SalesSettings, &ss); err == nil {
		it := QueryItem{ID: doc.Data.Company.ID, Type: "SalesSettings"}
		if v, ok := ss["entityVersion"]; ok {
			it.Name = "entityVersion=" + fmt.Sprintf("%v", v)
		}
		res.Items = []QueryItem{it}
		res.Counts["items"] = 1
		res.Prefs = ss
	}
	return res, nil
}
