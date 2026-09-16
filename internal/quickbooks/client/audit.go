package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AuditEvent is the secret-free projection of an audit-log row.
type AuditEvent struct {
	ID        string `json:"id,omitempty"`
	Date      string `json:"date,omitempty"`
	User      string `json:"user,omitempty"`
	EventType string `json:"eventType,omitempty"`
	Name      string `json:"name,omitempty"`
}

// AuditResult is the JSON envelope returned by ReplayAuditLog.
type AuditResult struct {
	Status int            `json:"status"`
	Counts map[string]int `json:"counts"`
	Events []AuditEvent   `json:"events"`
}

type rawAuditEvent struct {
	ID          string `json:"id"`
	EventId     string `json:"eventId"`
	Date        string `json:"date"`
	EventDate   string `json:"eventDate"`
	Timestamp   string `json:"timestamp"`
	CreatedDate string `json:"createdDate"`
	User        string `json:"user"`
	UserName    string `json:"userName"`
	UserEmail   string `json:"userEmail"`
	EventType   string `json:"eventType"`
	Type        string `json:"type"`
	Action      string `json:"action"`
	Name        string `json:"name"`
	Description string `json:"description"`
	EntityName  string `json:"entityName"`
	Who         struct {
		UserID  string `json:"userId"`
		AuthID  string `json:"authId"`
		Profile string `json:"profileId"`
	} `json:"who"`
	What struct {
		QBOAuditID              string         `json:"qboAuditId"`
		TaskID                  string         `json:"taskId"`
		EventType               string         `json:"eventType"`
		GroupEventType          string         `json:"groupEventType"`
		DefaultEventDescription string         `json:"defaultEventDescription"`
		EntityType              string         `json:"entityType"`
		Entity                  map[string]any `json:"entity"`
	} `json:"what"`

	When struct {
		CreatedDate string `json:"createdDate"`
		UpdatedDate string `json:"updatedDate"`
	} `json:"when"`
}

const auditLogsURL = "https://audit.api.intuit.com/v1/audit/logs"

// auditEntities is the entity filter the Audit Log UI POSTs
// (recaptured 2026-08-18 from /app/auditlog, Test Company, audit-ui 1.292.2).
var auditEntities = []string{
	"audit_info",
	"com.intuit.finance.financialplanning.businessbudgetmanagement.BusinessBudget",
	"com.intuit.finance.financialplanning.businessforecastmanagement.BusinessForecast",
	"com.intuit.smallbusiness.qbo.finance.accounting.bookkeepingtransactions.InterCompanyJournalEntry",
	"com.intuit.foundation.extensibility.customextensionsmanagement.CustomDimensionDefinition",
	"com.intuit.foundation.extensibility.customextensionsmanagement.CustomDimensionValue",
	"com.intuit.foundation.extensibility.customextensionsmanagement.customextensionsmanagement.CustomObjectDefinitionV2",
	"com.intuit.foundation.extensibility.customextensionsmanagement.customextensionsmanagement.CustomObjectValueV2",
	"curation.commerce.salesordermanagement.entities.SalesOrder",
	"com.intuit.finance.accounting.financialtransactions.FinancialConnection",
	"com.intuit.finance.accounting.financialtransactions.FinancialTransaction",
	"com.intuit.finance.accounting.financialtransactions.FinancialConnectionAccount",
	"com.intuit.foundation.extensibility.customextensionsmanagement.CustomFieldDefinition",
	"com.intuit.work.projectmanagement.ProjectV2",
	"datamap.work.workermanagement.employeemgmt.entities.Employee",
	"com.intuit.smallbusiness.qbo.finance.accounting.bookkeepingtransactions.ManualEliminationJournal",
	"com.intuit.finance.accounting.financialtransactions.FinancialCategory",
	"com.intuit.work.timetracking.timeentrymanagement.quantumleap.TimeEntryEntity",
	"com.intuit.commerce.pricerules.PriceRule",
	"com.intuit.foundation.businesstransaction.commonlistentitesanddimensions.Term",
	"com.intuit.commerce.warehousemanagement.inboundreceivingandstowing.itemreceipt",
	"com.intuit.commerce.productinformationmanagement.product",
	"com.intuit.commerce.productinformationmanagement.variant",
	"com.intuit.commerce.productinformationmanagement.category",
	"com.intuit.commerce.productinformationmanagement.catalogproduct",
	"com.intuit.commerce.productinformationmanagement.catalogvariant",
	"com.intuit.commerce.productinformationmanagement.combo",
	"com.intuit.commerce.inventorymanagement.InventoryAdjustmentV2",
	"com.intuit.commerce.inventorymanagement.InventoryStartV2",
	"com.intuit.commerce.inventorymanagement.InventoryAdjustment",
	"com.intuit.commerce.inventorymanagement.InventoryStart",
	"com.intuit.smallbusiness.qbo.payroll.aiagent.AuditEvent",
	"com.intuit.smallbusiness.qbo.finance.accounting.bookkeepingtransactions.Allocation",
	"com.intuit.foundation.businesstransaction.commonlistentitesanddimensions.CompanyCurrency",
	"com.intuit.foundation.businesstransaction.commonlistentitesanddimensions.CompanyExchangeRate",
	"com.intuit.work.timetracking.timeoffapproval.timeoffrequest",
	"datamap.aifabric.librosvc.accountingagent.entities.AccountingAgentAuditLog",
	"com.intuit.commerce.manufacturingassembly.WorkOrder",
	"com.intuit.crmandmarketing.customerlifecyclemanagement.contactmanagementservice.CustomerType",
	"com.intuit.commerce.vendormanagement.Vendor",
	"com.intuit.crmandmarketing.customerlifecyclemanagement.contactmanagementservice.Customer",
	"com.intuit.incometax.taxpreparationandplanning.taxstrategiesandplanning.IncomeAndExpenses",
	"com.intuit.work.workermanagement.applicanttracking.job",
	"com.intuit.work.workermanagement.applicanttracking.candidate",
	"com.intuit.ies.multientity.enterprisehierarchy.ConsolidationRules",
	"com.intuit.ies.multientity.enterprisehierarchy.EnterpriseHierarchy",
	"com.intuit.smallbusiness.qbo.finance.accounting.bookkeepingtransactions.EliminationSettings",
	"com.intuit.businessfinance.accounting.ledgeraccountmanagement.InterCompanyAccountsMapping",
	"com.intuit.foundation.businesstransaction.commonlistentitesanddimensions.Department",
	"com.intuit.foundation.businesstransaction.commonlistentitesanddimensions.PaymentMethod",
	"com.intuit.foundation.audit.Skill",
	"com.intuit.ies.multientity.multicurrency.CVMultiCurrency",
	"com.intuit.ies.multientity.multicurrency.CVExchangeRate",
	"com.intuit.foundation.workflow.agentstudio.AgentDefinition",
	"com.intuit.foundation.workflow.agentstudio.AgentTemplate",
	"com.intuit.foundation.workflow.agentstudio.AgentExecution",
	"com.intuit.foundation.workflow.agentstudio.AgentStepExecution",
	"com.intuit.commerce.inventorymanagement.InventoryCostAllocation",
	"com.intuit.foundation.attachment.AttachmentMetadata",
	"com.intuit.commerce.productinformationmanagement.ItemV2",
}

// auditThisMonthWindow is the Audit Log UI "This Month" range in the
// Test Company timezone (Australia/Sydney). Captured 2026-08-18:
// fromDate=2026-07-31T14:00:00.000Z toDate=2026-08-31T13:59:59.000Z.
func auditThisMonthWindow() (from, to string) {
	loc, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		loc = time.FixedZone("AEST", 10*60*60)
	}
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0).Add(-time.Second)
	return start.UTC().Format("2006-01-02T15:04:05.000Z"), end.UTC().Format("2006-01-02T15:04:05.000Z")
}

// AuditFilter selects rows after the captured /v1/audit/logs POST.
type AuditFilter struct {
	Limit     int
	EventType string
	User      string
	Query     string
}

// ReplayAuditLog POSTs /v1/audit/logs as captured from /app/auditlog.
// Unknown shapes return status + byte count — never the raw body.
func ReplayAuditLog(ctx context.Context, limit int) (*AuditResult, error) {
	return ReplayAuditLogFiltered(ctx, AuditFilter{Limit: limit})
}

func ReplayAuditLogFiltered(ctx context.Context, f AuditFilter) (*AuditResult, error) {
	limit := f.Limit
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	page := limit
	if f.EventType != "" || f.User != "" || f.Query != "" {
		page = 50
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	fromDate, toDate := auditThisMonthWindow()
	eventTypes := []any{}
	if f.EventType != "" {
		eventTypes = []any{f.EventType}
	}
	body, err := json.Marshal(map[string]any{
		"realmId":                  ac.realm,
		"offset":                   0,
		"pageSize":                 page,
		"sortOrder":                "desc",
		"entities":                 auditEntities,
		"userIds":                  []any{},
		"entityId":                 "",
		"listTypeIds":              []any{},
		"actionTypeIds":            []any{},
		"eventTypes":               eventTypes,
		"appAssetId":               "",
		"onlyCompleteDocuments":    false,
		"exceptionToCloseBookDate": false,
		"txTypeIds":                []any{},
		"accessType":               "external",
		"realmIds":                 []any{},
		"aiAgentIds":               []any{},
		"userAgentOrLogic":         true,
		"excludeTransactionLines":  false,
		"fromDate":                 fromDate,
		"toDate":                   toDate,
	})

	if err != nil {
		return nil, err
	}
	extra := map[string]string{
		"intuit-plugin-id":  "audit-ui",
		"intuit-company-id": ac.realm,
		"Referer":           "https://qbo.intuit.com/app/auditlog",
		"Content-Type":      "application/json;charset=UTF-8",
	}
	if ac.tok != nil && ac.tok.AuditAuthorization != "" {
		extra["Authorization"] = ac.tok.AuditAuthorization
	}
	resp, err := ac.postJSONExtra(ctx, auditLogsURL, body, extra)

	if err != nil {
		return nil, fmt.Errorf("audit logs: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading audit logs: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	evs, ok := extractAuditEvents(raw)
	if !ok {
		return &AuditResult{
			Status: resp.StatusCode,
			Counts: map[string]int{"bytes": len(raw)},
			Events: []AuditEvent{},
		}, nil
	}
	out := projectAudit(evs)
	out.Events = filterAuditEvents(out.Events, f)
	if len(out.Events) > limit {
		out.Events = out.Events[:limit]
	}
	out.Counts = map[string]int{"events": len(out.Events)}
	return out, nil

}

func extractAuditEvents(body []byte) ([]rawAuditEvent, bool) {
	var arr []rawAuditEvent
	if json.Unmarshal(body, &arr) == nil && arr != nil {
		return arr, true
	}
	var wrap struct {
		Events []rawAuditEvent `json:"events"`
		Items  []rawAuditEvent `json:"items"`
		Logs   []rawAuditEvent `json:"logs"`
		Data   []rawAuditEvent `json:"data"`
	}
	if json.Unmarshal(body, &wrap) != nil {
		return nil, false
	}
	switch {
	case wrap.Events != nil:
		return wrap.Events, true
	case wrap.Items != nil:
		return wrap.Items, true
	case wrap.Logs != nil:
		return wrap.Logs, true
	case wrap.Data != nil:
		return wrap.Data, true
	default:
		return nil, false
	}
}

func projectAudit(in []rawAuditEvent) *AuditResult {
	out := make([]AuditEvent, 0, len(in))
	for _, e := range in {
		date := firstNonEmpty(e.When.CreatedDate, e.When.UpdatedDate, e.Date, e.EventDate, e.Timestamp, e.CreatedDate)
		user := firstNonEmpty(e.Who.UserID, e.Who.AuthID, e.Who.Profile, e.User, e.UserName, e.UserEmail)
		et := firstNonEmpty(e.What.EventType, e.What.GroupEventType, e.EventType, e.Type, e.Action)
		name := auditDisplayName(e)
		id := firstNonEmpty(e.What.QBOAuditID, e.What.TaskID, e.ID, e.EventId)
		out = append(out, AuditEvent{ID: id, Date: date, User: user, EventType: et, Name: name})
	}

	return &AuditResult{
		Status: http.StatusOK,
		Counts: map[string]int{"events": len(out)},
		Events: out,
	}
}

func auditDisplayName(e rawAuditEvent) string {
	tx := anyString(e.What.Entity["TX_NAME"])
	doc := anyString(e.What.Entity["DOC_NUM"])
	switch {
	case tx != "" && doc != "":
		return tx + " #" + doc
	case tx != "":
		return tx
	case doc != "":
		return "#" + doc
	}
	desc := e.What.DefaultEventDescription
	if desc != "" && desc != "audit_info" && desc != e.What.EntityType {
		return desc
	}
	return firstNonEmpty(e.What.EntityType, desc, e.Name, e.Description, e.EntityName)
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case int, int64:
		return fmt.Sprintf("%d", t)
	default:
		return ""
	}
}
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func filterAuditEvents(in []AuditEvent, f AuditFilter) []AuditEvent {
	if f.EventType == "" && f.User == "" && f.Query == "" {
		return in
	}
	et := strings.ToLower(f.EventType)
	user := strings.ToLower(f.User)
	q := strings.ToLower(f.Query)
	out := make([]AuditEvent, 0, len(in))
	for _, e := range in {
		if et != "" && !strings.Contains(strings.ToLower(e.EventType), et) && !strings.Contains(strings.ToLower(e.Name), et) {
			continue
		}
		if user != "" && !strings.Contains(strings.ToLower(e.User), user) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.EventType+" "+e.Name+" "+e.User+" "+e.ID), q) {
			continue
		}
		out = append(out, e)
	}
	return out
}
