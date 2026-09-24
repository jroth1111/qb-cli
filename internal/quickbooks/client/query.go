package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const v3QueryTmpl = "https://qbo.intuit.com/api/v3/company/{realm}/query?minorversion=73&query="
const v3ReportTmpl = "https://qbo.intuit.com/api/v3/company/{realm}/reports/"

// QueryItem is a secret-free projection of one v3 entity.
type QueryItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name,omitempty"`
	Date      string  `json:"date,omitempty"`
	DocNumber string  `json:"docNumber,omitempty"`
	Amount    float64 `json:"amount,omitempty"`
	Active    *bool   `json:"active,omitempty"`
	Type      string  `json:"type,omitempty"`
	Account   string  `json:"account,omitempty"`
	AccountID string  `json:"accountId,omitempty"`
	Item      string  `json:"item,omitempty"`
	ItemID    string  `json:"itemId,omitempty"`
	Qty       float64 `json:"qty,omitempty"`
}

// QueryResult is the JSON envelope returned by ReplayQuery.
type QueryResult struct {
	Status int            `json:"status"`
	Entity string         `json:"entity"`
	Counts map[string]int `json:"counts"`
	Items  []QueryItem    `json:"items"`
	Detail map[string]any `json:"detail,omitempty"`
	Prefs  any            `json:"prefs,omitempty"`
	Note   string         `json:"note,omitempty"`
}

// ReportResult is a secret-free v3 report projection.
type ReportResult struct {
	Status  int             `json:"status"`
	Report  string          `json:"report"`
	Counts  map[string]int  `json:"counts"`
	Header  map[string]any  `json:"header,omitempty"`
	Columns []string        `json:"columns,omitempty"`
	Rows    json.RawMessage `json:"rows,omitempty"`
	Note    string          `json:"note,omitempty"`
}

// ErrUnsupportedQueryContext reports a v3 entity the query service rejects
// as a context declaration. Live probing returned 4001 for DelayedCharge.
var ErrUnsupportedQueryContext = errors.New("unsupported v3 query context")

// v3QueryContextsUnsupported names entities whose query contract is not
// captured. DelayedCharge remains separate from entity-path mutations; only
// the v3 query context is unsupported.
var v3QueryContextsUnsupported = map[string]bool{
	"DelayedCharge": true,
}

// V3Queryable reports whether entity is a valid v3 query context.
func V3Queryable(entity string) bool {
	return !v3QueryContextsUnsupported[strings.TrimSpace(entity)]
}

// ReplayQuery GETs /api/v3/company/{realm}/query. Read-only.
// id, if set, becomes WHERE Id = '...'. query is a sanitized LIKE on
// DisplayName, Name, or DocNumber. Unknown/empty QueryResponse → empty items.
func ReplayQuery(ctx context.Context, entity, id, query string, limit int, active ...string) (*QueryResult, error) {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return nil, fmt.Errorf("query requires an entity")
	}
	if limit < 1 {
		limit = 20
	}
	if entity == "SalesOrder" {
		return replayGetSalesOrders(ctx, id, query, limit)
	}
	if limit > 100 {
		limit = 100
	}
	if entity == "TPAR" {
		return replayTPARInstances(ctx, id, query, limit)
	}
	if entity == "ItemReceipt" {
		return replayGetItemReceipts(ctx, id, query, limit)
	}
	if entity == "Contract" {
		return replayModifiedEnvelopes(ctx, id, query, limit)
	}
	if entity == "CustomField" {
		return replayCustomFieldsQueryCES(ctx, id, query, limit)
	}
	if entity == "PrepaidSchedule" {
		return replayGetSchedules(ctx, id, query, limit)
	}
	if entity == "Workflow" {
		return replayWASAllRules(ctx, id, query, limit)
	}
	if entity == "BusinessFeed" {
		return replayBusinessFeedItems(ctx, id, query, limit)
	}
	if entity == "Proposal" {
		return replayProposals(ctx, id, query, limit)
	}
	if entity == "Review" {
		return replayFeedbackEngagement(ctx, id, query, limit)
	}
	if entity == "IdentityUser" {
		return ReplayIdentityUsers(ctx, query, limit)
	}
	if entity == "IdentityAccount" {
		return ReplayIdentityAccount(ctx, query, limit)
	}
	if entity == "SettingsFacade" {
		return ReplaySettingsFacade(ctx, query, limit)
	}
	if entity == "CustomExtension" {
		return ReplayCustomExtensions(ctx, query, limit)
	}
	if entity == "ExperimentAssignment" {
		return ReplayExperimentAssignments(ctx, query, limit)
	}
	if entity == "PaymentAccount" {
		return ReplayPaymentAccount(ctx)
	}
	if entity == "Folio" {
		return replayFolio(ctx, id, query, limit)
	}
	if entity == "RevenueRecognition" {
		return replayAccountingDeferredTransactionLineDetails(ctx, id, query, limit)
	}
	if entity == "ListsPrefs" {
		return replayListsPrefs(ctx, id, query, limit)
	}
	if entity == "CustomersOverview" {
		return replayCustomersOverview(ctx, id, query, limit)
	}
	if entity == "InventoryOverview" {
		return replayGetAllListViewEntities(ctx, id, query, limit)
	}
	if entity == "InventoryOverviewSearch" {
		return replaySearchAllListViewEntities(ctx, id, query, limit)
	}
	if entity == "invoiceSummary" {
		return replayCustomersOverview(ctx, id, query, limit)
	}
	if entity == "GetAllListViewEntities" {
		return replayGetAllListViewEntities(ctx, id, query, limit)
	}
	if entity == "SearchAllListViewEntities" {
		return replaySearchAllListViewEntities(ctx, id, query, limit)
	}
	if entity == "SalesHub" {
		return replaySalesHub(ctx, id, query, limit)
	}
	if entity == "Role" {
		return replayAccountRoles(ctx, id, query, limit)
	}
	if entity == "MarketingOffer" {
		return replayMarketingOffers(ctx, id, query, limit)
	}
	if entity == "FormStyle" {
		return replayFormStyles(ctx, id, query, limit)
	}
	if entity == "Tag" {
		return replayTags(ctx, id, query, limit)
	}
	if entity == "ManagementFolio" {
		return replayManagementFolio(ctx, id, query, limit)
	}
	if entity == "IDX" {
		return replayIDX(ctx, id, query, limit)
	}
	if entity == "IMSPref" {
		return replayIMSPref(ctx, id, query, limit)
	}
	if entity == "DimensionDefinition" {
		return replayDimensionDefinitions(ctx, id, query, limit)
	}
	if entity == "Task" {
		return replayTasks(ctx, id, query, limit)
	}
	if entity == "TaxReturn" {
		return ReplayTaxReturns(ctx, entity, "", query, limit)
	}
	if entity == "BusinessForecast" {
		return ReplayBusinessForecasts(ctx, id, query, limit)
	}
	if entity == "PerformanceMetric" {
		return ReplayPerformanceMetrics(ctx, query, limit)
	}
	if entity == "TaxJurisdiction" {
		return replayTaxJurisdictions(ctx, id, query, limit)
	}
	if entity == "TaxLiability" {
		return ReplayTaxLiability(ctx, "", "", query, limit)
	}
	if entity == "InventoryLocation" {
		return replayWarehouseLocations(ctx, id, query, limit)
	}
	if entity == "SalesOverview" {
		return ReplaySalesOverview(ctx, query, limit)
	}
	if entity == "ExpensesOverview" {
		return ReplayExpensesOverview(ctx, query, limit)
	}
	if entity == "SalesTxnRisk" {
		return ReplaySalesTxnRisk(ctx, query, limit)
	}
	if entity == "BillPayOnboarding" {
		return ReplayBillPayOnboarding(ctx, query, limit)
	}
	if entity == "StageTransaction" {
		return ReplayStageTransactions(ctx, query, limit)
	}
	if entity == "B2BBillpay" {
		return ReplayB2BBillpay(ctx, query, limit)
	}
	if entity == "B2BNetwork" {
		return ReplayB2BNetwork(ctx, query, limit)
	}
	if entity == "TaxConfigGroup" {
		return ReplayTaxConfigGroups(ctx, query, limit)
	}
	if entity == "Reconciliation" {
		return ReplayReconciliation(ctx, id, query, limit)
	}
	if entity == "QBOnlineGraphql" {
		return ReplayQBOnlineGraphql(ctx, id, query, limit)
	}
	if entity == "BudgetPlanning" {
		return ReplayBudgetPlanning(ctx, query, limit)
	}
	if entity == "FixedAsset" {
		return ReplayFinanceAssets(ctx, id, query, limit)
	}
	act := ""
	if len(active) > 0 {
		act = active[0]
	}
	stmt := buildQuery(entity, id, query, limit, act)
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if entity == "Purchase" && id != "" {
		obj, err := fetchV3(ctx, ac, "purchase", id)
		if err != nil {
			return nil, err
		}
		if fmt.Sprint(obj["Id"]) != id {
			return nil, fmt.Errorf("purchase readback did not match the requested id")
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		items := projectQueryObjects(entity, []map[string]json.RawMessage{fields})
		return &QueryResult{Status: 200, Entity: entity, Counts: map[string]int{"items": 1}, Items: items, Detail: obj}, nil
	}
	if entity == "Department" && id != "" {
		// Department listing works, but the live v3 query service rejects
		// filtering it by Id. Use the resource's actual GET endpoint.
		obj, err := fetchV3(ctx, ac, "department", id)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
		items := projectQueryObjects(entity, []map[string]json.RawMessage{fields})
		if filter := sanitizeActive(act); filter != "" && filter != "all" && len(items) == 1 && items[0].Active != nil && *items[0].Active != (filter == "true") {
			items = []QueryItem{}
		}
		return &QueryResult{Status: 200, Entity: entity, Counts: map[string]int{"items": len(items)}, Items: items}, nil
	}
	u := strings.ReplaceAll(v3QueryTmpl, realmToken, ac.realm) + url.QueryEscape(stmt)
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("v3 query %s: %w", entity, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading v3 query %s: %w", entity, err)
	}
	if resp.StatusCode == http.StatusBadRequest && query != "" && id == "" {
		_ = drainAndClose(resp)
		stmt = buildQuery(entity, "", "", limit, act)
		u = strings.ReplaceAll(v3QueryTmpl, realmToken, ac.realm) + url.QueryEscape(stmt)
		resp, err = ac.getJSON(ctx, u, "")
		if err != nil {
			return nil, fmt.Errorf("v3 query %s: %w", entity, err)
		}
		defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
		body, err = readBody(resp)
		if err != nil {
			return nil, fmt.Errorf("reading v3 query %s: %w", entity, err)
		}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	responseEntity := queryResponseEntity(entity)
	if err := validateQueryEnvelope(responseEntity, body); err != nil {
		return nil, err
	}
	items := projectQuery(responseEntity, body)
	// The live v3 query service accepts the Estimate DocNumber predicate but
	// returns an empty 200 even when the matching estimate exists. A bounded
	// unfiltered retry preserves the read/search contract, then the existing
	// client-side filter applies the requested text. Keep this fallback scoped
	// to Estimate so ordinary queries do not gain an extra network round trip.
	if entity == "Estimate" && query != "" && id == "" && len(items) == 0 {
		stmt = buildQuery(entity, "", "", limit, act)
		u = strings.ReplaceAll(v3QueryTmpl, realmToken, ac.realm) + url.QueryEscape(stmt)
		fallbackResp, ferr := ac.getJSON(ctx, u, "")
		if ferr != nil {
			return nil, fmt.Errorf("v3 query %s fallback: %w", entity, ferr)
		}
		defer func(r *http.Response) { _ = drainAndClose(r) }(fallbackResp)
		fallbackBody, ferr := readBody(fallbackResp)
		if ferr != nil {
			return nil, fmt.Errorf("reading v3 query %s fallback: %w", entity, ferr)
		}
		if fallbackResp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: fallbackResp.StatusCode, Message: errorMessage(fallbackBody)}
		}
		if ferr := validateQueryEnvelope(responseEntity, fallbackBody); ferr != nil {
			return nil, ferr
		}
		items = projectQuery(responseEntity, fallbackBody)
	}
	if query != "" && id == "" {
		items = filterItems(items, query)
	}

	if len(items) > limit {
		items = items[:limit]
	}
	return &QueryResult{
		Status: resp.StatusCode,
		Entity: entity,
		Counts: map[string]int{"items": len(items)},
		Items:  items,
	}, nil
}

// queryResponseEntity maps a CLI query entity onto the QueryResponse
// collection key the API actually returns. Cheque reads come back as
// Purchase rows; AU/regional schemas return CreditCardPaymentTxn for the
// CreditCardPayment query.
func queryResponseEntity(entity string) string {
	switch entity {
	case "Cheque":
		return "Purchase"
	case "CreditCardPayment":
		return "CreditCardPaymentTxn"
	}
	return entity
}

// validateQueryEnvelope distinguishes a legitimate empty query from an error,
// XML response, login page, or malformed entity collection.
func validateQueryEnvelope(entity string, body []byte) error {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil {
		return fmt.Errorf("v3 query %s: expected a JSON QueryResponse", entity)
	}
	for key, value := range envelope {
		if (strings.EqualFold(key, "fault") || strings.EqualFold(key, "error") || strings.EqualFold(key, "errors")) && string(value) != "null" {
			return fmt.Errorf("v3 query %s: API returned an error envelope", entity)
		}
	}
	var response map[string]json.RawMessage
	if json.Unmarshal(envelope["QueryResponse"], &response) != nil || response == nil {
		return fmt.Errorf("v3 query %s: missing or invalid QueryResponse", entity)
	}
	if raw, ok := response[entity]; ok {
		var rows []map[string]json.RawMessage
		if json.Unmarshal(raw, &rows) == nil && rows != nil {
			return nil
		}
		var row map[string]json.RawMessage
		if json.Unmarshal(raw, &row) != nil || len(row) == 0 {
			return fmt.Errorf("v3 query %s: invalid entity collection", entity)
		}
	}
	return nil
}

// ReportOptions carries optional v3 report query parameters. SummarizeColumnBy
// groups period columns; it is distinct from the API's detail-field "columns".
// Parameter names: Intuit's ReportService / CoreConstants SDK documentation.
type ReportOptions struct {
	AccountingMethod  string
	SummarizeColumnBy string
}

// ReportDateParams follows the live balance-report snapshot contract.
func ReportDateParams(name, start, end string) url.Values {
	q := url.Values{}
	switch name {
	case "VendorBalance", "VendorBalanceDetail", "CustomerBalanceDetail", "AgedPayables", "AgedPayableDetail", "AgedReceivables", "AgedReceivableDetail":
		if end == "" {
			end = start
		}
		if end != "" {
			q.Set("report_date", end)
		}
	default:
		if start != "" {
			q.Set("start_date", start)
		}
		if end != "" {
			q.Set("end_date", end)
		}
	}
	return q
}

// QueryParams validates and encodes options for both requests and dry-run plans.
func (o ReportOptions) QueryParams(name string) (url.Values, error) {
	q := url.Values{}
	if name == "TAXABLE_PAYMENTS" && (o.AccountingMethod != "" || o.SummarizeColumnBy != "") {
		return nil, fmt.Errorf("TAXABLE_PAYMENTS does not support --accounting-method or --columns; omit these options")
	}
	switch strings.ToLower(strings.TrimSpace(o.AccountingMethod)) {
	case "":
	case "cash":
		q.Set("accounting_method", "Cash")
	case "accrual":
		q.Set("accounting_method", "Accrual")
	default:
		return nil, fmt.Errorf("--accounting-method must be Cash or Accrual")
	}
	columns := strings.TrimSpace(o.SummarizeColumnBy)
	// Preserve the CLI's period wording while sending the SDK enum values.
	switch strings.ToLower(columns) {
	case "daily":
		columns = "Days"
	case "weekly":
		columns = "Week"
	case "monthly":
		columns = "Month"
	case "quarterly":
		columns = "Quarter"
	case "yearly":
		columns = "Year"
	}
	if columns != "" {
		// Intuit SummarizeColumnsByEnum defines these canonical values.
		for _, value := range []string{"Total", "Year", "Quarter", "FiscalYear", "FiscalQuarter", "Month", "Week", "Days", "Customers", "Vendors", "Employees", "Departments", "Classes", "ProductsAndServices"} {
			if strings.EqualFold(columns, value) {
				q.Set("summarize_column_by", value)
				return q, nil
			}
		}
		return nil, fmt.Errorf("--columns requires a summarize_column_by value (for example Total, Month, Quarter or Year); comparison and detail-field lists are not supported")
	}
	return q, nil
}

// ReplayReport GETs /api/v3/company/{realm}/reports/{name}. Read-only.
// At most one options value is accepted; the four-argument call is unchanged.
func ReplayReport(ctx context.Context, name, start, end string, options ...ReportOptions) (*ReportResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("report requires a name")
	}
	if !reportNameOK(name) {
		return nil, fmt.Errorf("invalid report name")
	}
	if len(options) > 1 {
		return nil, fmt.Errorf("report accepts at most one ReportOptions value")
	}
	var opts ReportOptions
	if len(options) == 1 {
		opts = options[0]
	}
	params, err := opts.QueryParams(name)
	if err != nil {
		return nil, err
	}
	if name == "TAXABLE_PAYMENTS" {
		return replayTPARReport(ctx, start, end)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := strings.ReplaceAll(v3ReportTmpl, realmToken, ac.realm) + url.PathEscape(name) + "?minorversion=73"
	maps.Copy(params, ReportDateParams(name, start, end))
	if len(params) > 0 {
		u += "&" + params.Encode()
	}
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("v3 report %s: %w", name, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading v3 report %s: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	return projectReport(name, body)
}

func PlannedQueryURL(entity string, ids ...string) string {
	if entity == "Purchase" && len(ids) > 0 && strings.TrimSpace(ids[0]) != "" {
		return "https://qbo.intuit.com/api/v3/company/{realm}/purchase/" + url.PathEscape(sanitizeToken(ids[0])) + "?minorversion=73"
	}
	if entity == "Department" && len(ids) > 0 && strings.TrimSpace(ids[0]) != "" {
		return "https://qbo.intuit.com/api/v3/company/{realm}/department/" + url.PathEscape(sanitizeToken(ids[0])) + "?minorversion=73"
	}
	if entity == "SalesOrder" {
		return PlannedSalesOrderURL()
	}
	if entity == "TPAR" {
		return PlannedTPARURL()
	}
	if entity == "ItemReceipt" {
		return PlannedItemReceiptURL()
	}
	if entity == "Contract" {
		return PlannedContractURL()
	}
	if entity == "CustomField" {
		return PlannedCustomFieldURL()
	}
	if entity == "PrepaidSchedule" {
		return PlannedPrepaidURL()
	}
	if entity == "Workflow" {
		return PlannedWorkflowURL()
	}
	if entity == "BusinessFeed" {
		return PlannedBusinessFeedURL()
	}
	if entity == "Proposal" {
		return PlannedProposalURL()
	}
	if entity == "Review" {
		return PlannedReviewURL()
	}
	if entity == "Folio" {
		return PlannedFolioURL()
	}
	if entity == "RevenueRecognition" {
		return PlannedRevenueRecognitionURL()
	}
	if entity == "ListsPrefs" {
		return PlannedListsPrefsURL()
	}
	if entity == "CustomersOverview" {
		return PlannedCustomersOverviewURL()
	}
	if entity == "InventoryOverview" || entity == "InventoryOverviewSearch" {
		return PlannedInventoryOverviewURL()
	}
	if entity == "invoiceSummary" {
		return PlannedCustomersOverviewURL()
	}
	if entity == "GetAllListViewEntities" || entity == "SearchAllListViewEntities" {
		return PlannedInventoryOverviewURL()
	}
	if entity == "SalesHub" {
		return PlannedSalesHubURL()
	}
	if entity == "Role" {
		return PlannedRoleURL()
	}
	if entity == "MarketingOffer" {
		placement := ""
		if len(ids) > 0 {
			placement = ids[0]
		}
		return PlannedMarketingURL(placement)
	}
	if entity == "FormStyle" {
		return PlannedFormStyleURL()
	}
	if entity == "Tag" {
		return PlannedTagURL()
	}
	if entity == "ManagementFolio" {
		return PlannedManagementFolioURL()
	}
	if entity == "TaxReturn" {
		return "https://qbo.intuit.com/api/v4/graphql (node__indirect_tax_ui_qbo taxReturns)"
	}
	if entity == "BusinessForecast" {
		return "https://planningforecasting.api.intuit.com/graphql (getAllBusinessForecasts)"
	}
	if entity == "PerformanceMetric" {
		return "https://universalreportinsights.api.intuit.com/v1/metrics/<Metric>"
	}
	if entity == "SalesOverview" {
		return "https://universalreportsgraphql.api.intuit.com/graphql (allSales)"
	}
	if entity == "ExpensesOverview" {
		return "https://dashboardframework.api.intuit.com/v1/dashboards?name=EXPENSES_DASHBOARD"
	}
	if entity == "SalesTxnRisk" {
		return "https://salestxnrisksvc.api.intuit.com/v2/risk/eligibility"
	}
	if entity == "BillPayOnboarding" {
		return "https://bill-pay-onboarding-svc.api.intuit.com/v1/bill-payment-eligibility"
	}
	if entity == "StageTransaction" {
		return "https://stagetransactions.api.intuit.com/stage/entities"
	}
	if entity == "B2BBillpay" {
		return "https://b2bbillpay.api.intuit.com/v1/paymentApproval/pendingInstructions"
	}
	if entity == "B2BNetwork" {
		return "https://b2bnetworkservice.api.intuit.com/v2/directory/attributes"
	}
	if entity == "TaxConfigGroup" {
		return "https://taxconfig.api.intuit.com/graphql (IndirectTaxTaxGropups_taxconfig)"
	}
	if entity == "Reconciliation" {
		return "https://qbo.intuit.com/api/v4/graphql (Integration_Reconciliation node)"
	}
	if entity == "QBOnlineGraphql" {
		return "https://qbonline-aws.api.intuit.com/v4/graphql (company probe or --op)"
	}
	if entity == "BudgetPlanning" {
		return "https://budgeting.api.intuit.com/graphql (fetchAllBudgets)"
	}
	if entity == "FixedAsset" {
		return "https://assetservice.api.intuit.com/v1/graphql (financeAssets)"
	}
	return v3QueryTmpl + url.QueryEscape(buildQuery(entity, "", "", 20))
}

func PlannedReportURL(name string) string {
	if name == "" {
		name = "ProfitAndLoss"
	}
	if name == "TAXABLE_PAYMENTS" {
		return PlannedTPARURL()
	}
	return v3ReportTmpl + name + "?minorversion=73"
}

func buildQuery(entity, id, query string, limit int, active ...string) string {
	cheque := entity == "Cheque"
	if cheque {
		entity = "Purchase"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "select * from %s", entity)
	id = sanitizeToken(id)
	q := sanitizeToken(query)
	act := ""
	if len(active) > 0 {
		act = sanitizeActive(active[0])
	}
	var wheres []string
	if cheque {
		wheres = append(wheres, "PaymentType = 'Check'")
	}
	switch {
	case id != "":
		wheres = append(wheres, fmt.Sprintf("Id = '%s'", id))
	case q != "":
		field := "DisplayName"
		switch entity {
		case "Invoice", "Estimate", "SalesReceipt", "CreditMemo", "Payment",
			"RefundReceipt", "Bill", "Purchase", "VendorCredit", "JournalEntry",
			"Transfer", "Deposit", "PurchaseOrder", "ItemReceipt", "InventoryAdjustment":
			field = "DocNumber"
		case "Account", "Class", "Department", "Item", "TaxCode", "CustomerType":
			field = "Name"
		case "ExchangeRate":
			field = "SourceCurrencyCode"
		case "Employee":
			field = "DisplayName"
		case "Attachable":
			field = "FileName"
		}
		wheres = append(wheres, fmt.Sprintf("%s like '%%%s%%'", field, q))
	}
	if act == "all" {
		wheres = append(wheres, "Active IN (true, false)")
	} else if act != "" {
		wheres = append(wheres, "Active = "+act)
	} else if id != "" && entity == "Employee" {
		// QBO defaults Active=true even on Id lookups. Include inactive so
		// payroll employee read --id finds terminated/deleted rows.
		wheres = append(wheres, "Active IN (true, false)")
	}
	if len(wheres) > 0 {
		fmt.Fprintf(&b, " where %s", strings.Join(wheres, " AND "))
	}
	fmt.Fprintf(&b, " maxresults %d", limit)
	return b.String()
}

// sanitizeActive allows true/false or all (include inactive). Unknown values
// are dropped so we never inject an Active clause the caller did not request.
func sanitizeActive(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return "true"
	case "false", "0", "no":
		return "false"
	case "all", "any", "both":
		return "all"
	default:
		return ""
	}
}

func sanitizeToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func reportNameOK(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func projectQuery(entity string, body []byte) []QueryItem {
	var wrap struct {
		QueryResponse map[string]json.RawMessage `json:"QueryResponse"`
	}
	if json.Unmarshal(body, &wrap) != nil || wrap.QueryResponse == nil {
		return []QueryItem{}
	}
	raw, ok := wrap.QueryResponse[entity]
	if !ok {
		return []QueryItem{}
	}
	var objs []map[string]json.RawMessage
	if json.Unmarshal(raw, &objs) != nil {
		var one map[string]json.RawMessage
		if json.Unmarshal(raw, &one) != nil {
			return []QueryItem{}
		}
		objs = []map[string]json.RawMessage{one}
	}
	return projectQueryObjects(entity, objs)
}

func projectQueryObjects(entity string, objs []map[string]json.RawMessage) []QueryItem {
	out := make([]QueryItem, 0, len(objs))
	for _, o := range objs {
		it := QueryItem{Type: entity}
		it.ID = jsonString(o, "Id")
		it.Name = firstJSONString(o, "DisplayName", "Name", "FileName", "FullyQualifiedName", "CompanyName", "SourceCurrencyCode")
		it.Date = firstJSONString(o, "TxnDate", "MetaData")
		if strings.HasPrefix(it.Date, "{") {
			it.Date = ""
		}
		it.DocNumber = jsonString(o, "DocNumber")
		it.Amount = jsonFloat(o, "TotalAmt", "Balance", "Amount")
		if v, ok := o["Active"]; ok {
			var a bool
			if json.Unmarshal(v, &a) == nil {
				it.Active = &a
			}
		}
		if t := jsonString(o, "AccountType"); t != "" {
			it.Type = t
		} else if t := jsonString(o, "Type"); t != "" {
			it.Type = t
		} else if t := jsonString(o, "PaymentType"); t != "" {
			it.Type = t
		}
		if aid, aname := jsonRef(o, "AdjustAccountRef", "AccountRef"); aid != "" || aname != "" {
			it.AccountID = aid
			it.Account = aname
		}
		if iid, iname, qty, ok := firstAdjLine(o); ok {
			it.ItemID = iid
			it.Item = iname
			it.Qty = qty
		}
		if it.ID == "" {
			unwrapRecurring(&it, o)
		}
		out = append(out, it)
	}
	return out
}

func unwrapRecurring(it *QueryItem, o map[string]json.RawMessage) {
	for _, nest := range []string{"Invoice", "Bill", "Purchase", "Estimate", "JournalEntry", "SalesReceipt", "CreditMemo", "VendorCredit", "RefundReceipt"} {
		raw, ok := o[nest]
		if !ok {
			continue
		}
		var inner map[string]json.RawMessage
		if json.Unmarshal(raw, &inner) != nil {
			continue
		}
		it.ID = jsonString(inner, "Id")
		if it.Name == "" {
			it.Name = firstJSONString(inner, "DisplayName", "Name")
		}
		if it.Date == "" {
			it.Date = jsonString(inner, "TxnDate")
		}
		if it.DocNumber == "" {
			it.DocNumber = jsonString(inner, "DocNumber")
		}
		if it.Amount == 0 {
			it.Amount = jsonFloat(inner, "TotalAmt", "Balance", "Amount")
		}
		if rec, ok := inner["RecurringInfo"]; ok {
			var info map[string]json.RawMessage
			if json.Unmarshal(rec, &info) == nil {
				if n := jsonString(info, "Name"); n != "" {
					it.Name = n
				}
			}
		}
		return
	}
}

// itemKeys selects which raw JSON object keys feed each secret-free
// QueryItem field in the non-v3 read projectors. Each list is tried in
// order and the first scalar hit wins. refs reads {"value"|"id","name"}
// reference objects into AccountID/Account; deleted holds boolean keys
// that invert into Active (deleted:true → Active:false). Keys not named
// here never reach the envelope — no nested blobs, headers, or tokens.
type itemKeys struct {
	id, name, date, docNumber, amount, typeOf []string
	refs, deleted                             []string
}

// gqlEdgeNodes unwraps GraphQL [{"node":{...}}] edge elements to their
// node objects. Edges without an object node are dropped.
func gqlEdgeNodes(edges []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(edges))
	for _, e := range edges {
		var w struct {
			Node json.RawMessage `json:"node"`
		}
		if json.Unmarshal(e, &w) != nil || len(w.Node) == 0 {
			continue
		}
		out = append(out, w.Node)
	}
	return out
}

// projectJSONItems maps raw JSON objects to secret-free QueryItems under
// keys, one item per valid object so counts.items reflects the rows the
// API actually returned. Non-object elements are skipped.
func projectJSONItems(entity string, raws []json.RawMessage, keys itemKeys) []QueryItem {
	items := make([]QueryItem, 0, len(raws))
	for _, raw := range raws {
		var o map[string]json.RawMessage
		if err := json.Unmarshal(raw, &o); err != nil || o == nil {
			continue
		}
		it := QueryItem{Type: entity}
		it.ID = firstJSONString(o, keys.id...)
		it.Name = firstJSONString(o, keys.name...)
		it.Date = firstJSONString(o, keys.date...)
		if strings.HasPrefix(it.Date, "{") {
			it.Date = ""
		}
		it.DocNumber = firstJSONString(o, keys.docNumber...)
		it.Amount = jsonFloat(o, keys.amount...)
		if t := firstJSONString(o, keys.typeOf...); t != "" {
			it.Type = t
		}
		for _, k := range keys.refs {
			v, ok := o[k]
			if !ok {
				continue
			}
			var ref map[string]json.RawMessage
			if json.Unmarshal(v, &ref) != nil || ref == nil {
				continue
			}
			aid, aname := firstJSONString(ref, "value", "id"), jsonString(ref, "name")
			if aid != "" || aname != "" {
				it.AccountID, it.Account = aid, aname
				break
			}
		}
		for _, k := range keys.deleted {
			v, ok := o[k]
			if !ok {
				continue
			}
			var b bool
			if json.Unmarshal(v, &b) == nil {
				a := !b
				it.Active = &a
				break
			}
		}
		items = append(items, it)
	}
	return items
}

func filterItems(items []QueryItem, q string) []QueryItem {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return items
	}
	out := items[:0]
	for _, it := range items {
		blob := strings.ToLower(it.ID + " " + it.Name + " " + it.DocNumber + " " + it.Date)
		if strings.Contains(blob, q) {
			out = append(out, it)
		}
	}
	return out
}

func projectReport(name string, body []byte) (*ReportResult, error) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil || raw == nil {
		return nil, fmt.Errorf("v3 report %s: expected a JSON report object", name)
	}
	for key, value := range raw {
		if (strings.EqualFold(key, "fault") || strings.EqualFold(key, "error") || strings.EqualFold(key, "errors")) && strings.TrimSpace(string(value)) != "null" {
			// Do not echo arbitrary response text (it may contain session data).
			return nil, fmt.Errorf("v3 report %s: API returned an error envelope (HTTP 200)", name)
		}
	}
	header := map[string]any{}
	var hm map[string]any
	if json.Unmarshal(raw["Header"], &hm) != nil || hm == nil {
		return nil, fmt.Errorf("v3 report %s: missing or invalid Header", name)
	}
	if reportName, ok := hm["ReportName"].(string); !ok || strings.TrimSpace(reportName) == "" {
		return nil, fmt.Errorf("v3 report %s: missing or invalid Header.ReportName", name)
	}
	for _, k := range []string{"ReportName", "ReportBasis", "StartPeriod", "EndPeriod", "Currency", "Time"} {
		if v, ok := hm[k]; ok {
			if _, ok := v.(string); !ok {
				return nil, fmt.Errorf("v3 report %s: invalid Header.%s", name, k)
			}
			header[k] = v
		}
	}
	cols := []string{}
	var wrap struct {
		Column []*struct {
			ColTitle string `json:"ColTitle"`
		} `json:"Column"`
	}
	if json.Unmarshal(raw["Columns"], &wrap) != nil || wrap.Column == nil {
		return nil, fmt.Errorf("v3 report %s: missing or invalid Columns.Column", name)
	}
	for _, col := range wrap.Column {
		if col == nil {
			return nil, fmt.Errorf("v3 report %s: invalid column object", name)
		}
		if col.ColTitle != "" {
			cols = append(cols, col.ColTitle)
		}
	}
	return &ReportResult{
		Status:  http.StatusOK,
		Report:  name,
		Counts:  map[string]int{"columns": len(cols), "bytes": len(body)},
		Header:  header,
		Columns: cols,
		Rows:    raw["Rows"],
	}, nil
}

// firstAdjLine copies ItemRef / QtyDiff from the first ItemAdjustmentLineDetail
// on Line. Missing Line or detail is skipped; nothing is invented. Leftover
// InventoryAdjustment 159 has no Amount (not top-level, not on Line).
func firstAdjLine(o map[string]json.RawMessage) (id, name string, qty float64, ok bool) {
	raw, exists := o["Line"]
	if !exists {
		return "", "", 0, false
	}
	var lines []map[string]json.RawMessage
	if json.Unmarshal(raw, &lines) != nil || len(lines) == 0 {
		return "", "", 0, false
	}
	for _, line := range lines {
		detRaw, has := line["ItemAdjustmentLineDetail"]
		if !has {
			continue
		}
		var det map[string]json.RawMessage
		if json.Unmarshal(detRaw, &det) != nil {
			continue
		}
		id, name = jsonRef(det, "ItemRef")
		_, hasQty := det["QtyDiff"]
		if hasQty {
			qty = jsonFloat(det, "QtyDiff")
		}
		if id != "" || name != "" || hasQty {
			return id, name, qty, true
		}
	}
	return "", "", 0, false
}

func jsonString(o map[string]json.RawMessage, key string) string {
	v, ok := o[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(v, &n) == nil {
		return n.String()
	}
	return ""
}

func firstJSONString(o map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if s := jsonString(o, k); s != "" {
			return s
		}
	}
	return ""
}

// jsonRef reads a v3 *Ref object ({"value":"id","name":"..."}). First key that
// has a value or name wins. Missing keys are skipped; nothing is invented.
func jsonRef(o map[string]json.RawMessage, keys ...string) (id, name string) {
	for _, k := range keys {
		v, ok := o[k]
		if !ok {
			continue
		}
		var ref map[string]json.RawMessage
		if json.Unmarshal(v, &ref) != nil {
			continue
		}
		id = jsonString(ref, "value")
		name = jsonString(ref, "name")
		if id != "" || name != "" {
			return id, name
		}
	}
	return "", ""
}

func jsonFloat(o map[string]json.RawMessage, keys ...string) float64 {
	for _, k := range keys {
		v, ok := o[k]
		if !ok {
			continue
		}
		var f float64
		if json.Unmarshal(v, &f) == nil {
			return f
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			if n, err := strconv.ParseFloat(s, 64); err == nil {
				return n
			}
		}
	}
	return 0
}
