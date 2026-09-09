package client

import (
	"context"
	"encoding/json"
	"fmt"
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
	Prefs  any            `json:"prefs,omitempty"`
	Note   string         `json:"note,omitempty"`
}

// ReportResult is a secret-free v3 report projection.
type ReportResult struct {
	Status  int            `json:"status"`
	Report  string         `json:"report"`
	Counts  map[string]int `json:"counts"`
	Header  map[string]any `json:"header,omitempty"`
	Columns []string       `json:"columns,omitempty"`
	Note    string         `json:"note,omitempty"`
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
	if limit > 100 {
		limit = 100
	}
	if entity == "SalesOrder" {
		return replayGetSalesOrders(ctx, id, query, limit)
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
	if entity == "FormStyle" {
		return replayFormStyles(ctx, id, query, limit)
	}
	if entity == "ManagementFolio" {
		return replayManagementFolio(ctx, id, query, limit)
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
	u := strings.ReplaceAll(v3QueryTmpl, realmToken, ac.realm) + url.QueryEscape(stmt)
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("v3 query %s: %w", entity, err)
	}
	defer drainAndClose(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading v3 query %s: %w", entity, err)
	}
	if resp.StatusCode == http.StatusBadRequest && query != "" && id == "" {
		drainAndClose(resp)
		stmt = buildQuery(entity, "", "", limit, act)
		u = strings.ReplaceAll(v3QueryTmpl, realmToken, ac.realm) + url.QueryEscape(stmt)
		resp, err = ac.getJSON(ctx, u, "")
		if err != nil {
			return nil, fmt.Errorf("v3 query %s: %w", entity, err)
		}
		defer drainAndClose(resp)
		body, err = readBody(resp)
		if err != nil {
			return nil, fmt.Errorf("reading v3 query %s: %w", entity, err)
		}
	} else if resp.StatusCode == http.StatusBadRequest {
		return &QueryResult{
			Status: resp.StatusCode,
			Entity: entity,
			Counts: map[string]int{"items": 0},
			Items:  []QueryItem{},
			Note:   "entity not available in this company",
		}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	items := projectQuery(entity, body)
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

// ReplayReport GETs /api/v3/company/{realm}/reports/{name}. Read-only.
func ReplayReport(ctx context.Context, name, start, end string) (*ReportResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("report requires a name")
	}
	if !reportNameOK(name) {
		return nil, fmt.Errorf("invalid report name")
	}
	if name == "TAXABLE_PAYMENTS" {
		return replayTPARReport(ctx, start, end)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := strings.ReplaceAll(v3ReportTmpl, realmToken, ac.realm) + url.PathEscape(name) + "?minorversion=73"
	if start != "" {
		u += "&start_date=" + url.QueryEscape(start)
	}
	if end != "" {
		u += "&end_date=" + url.QueryEscape(end)
	}
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("v3 report %s: %w", name, err)
	}
	defer drainAndClose(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading v3 report %s: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	return projectReport(name, body), nil
}

func PlannedQueryURL(entity string) string {
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
	if entity == "FormStyle" {
		return PlannedFormStyleURL()
	}
	if entity == "ManagementFolio" {
		return PlannedManagementFolioURL()
	}
	return v3QueryTmpl + url.QueryEscape("select * from "+entity+" maxresults 20")
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
	var b strings.Builder
	fmt.Fprintf(&b, "select * from %s", entity)
	id = sanitizeToken(id)
	q := sanitizeToken(query)
	act := ""
	if len(active) > 0 {
		act = sanitizeActive(active[0])
	}
	var wheres []string
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
	out := make([]QueryItem, 0, len(objs))
	for _, o := range objs {
		it := QueryItem{Type: entity}
		it.ID = jsonString(o, "Id")
		it.Name = firstJSONString(o, "DisplayName", "Name", "FullyQualifiedName", "CompanyName", "SourceCurrencyCode")
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

func projectReport(name string, body []byte) *ReportResult {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return &ReportResult{Status: http.StatusOK, Report: name, Counts: map[string]int{"bytes": len(body)}}
	}
	header := map[string]any{}
	if h, ok := raw["Header"]; ok {
		var hm map[string]any
		if json.Unmarshal(h, &hm) == nil {
			for _, k := range []string{"ReportName", "StartPeriod", "EndPeriod", "Currency", "Time"} {
				if v, ok := hm[k]; ok {
					header[k] = v
				}
			}
		}
	}
	cols := []string{}
	if c, ok := raw["Columns"]; ok {
		var wrap struct {
			Column []struct {
				ColTitle string `json:"ColTitle"`
			} `json:"Column"`
		}
		if json.Unmarshal(c, &wrap) == nil {
			for _, col := range wrap.Column {
				if col.ColTitle != "" {
					cols = append(cols, col.ColTitle)
				}
			}
		}
	}
	return &ReportResult{
		Status:  http.StatusOK,
		Report:  name,
		Counts:  map[string]int{"columns": len(cols), "bytes": len(body)},
		Header:  header,
		Columns: cols,
	}
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
