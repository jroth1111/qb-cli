// crm_leads.go implements the CRM leads/opportunities client against the
// mccrmmessaging.api.intuit.com api/core service used by crm-leads-ui.
//
// Endpoint evidence (capture 2026-08, deobfuscated bundle
// plugin.intuitcdn.net__crm-leads-ui__6136.d99551dd33489840 module 46136.js):
// every call resolves through a path map rooted at /api/core —
// /leads (list/get/create/update), /leads/{id} (get/delete),
// /leads/search, /leads/count, /leads/bulk, /leads/convert-to-customer,
// /lead-customer-associations, and /opportunities (list/create).
//
// Transport evidence (mitm capture 2026-08): the SPA hits
// mccrmmessaging.api.intuit.com/api/core with cookie-only auth, direct
// cross-origin replay of that traffic is rejected with 403, and the older
// qbo.intuit.com/api/core guess simply 404s. Every call therefore runs as
// an in-page fetch inside an authenticated qbo.intuit.com relay tab (see
// crm_cdp.go): credentials:'include' supplies the browser session context
// and the saved CSRF token is added when present. Unlike the ATS banking
// APIs, the URL carries no realm segment; plans embed nothing
// session-specific.
//
// Response shapes are only partially observed (Spring page envelopes with
// content/totalElements, bare-number count bodies, 204 for empty lists),
// so every projection below is deliberately tolerant: unknown fields are
// ignored and several known aliases are tried per field.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// crmCoreBaseURL is the frontend api/core root. Every CRM path is appended
// verbatim.
const crmCoreBaseURL = "https://mccrmmessaging.api.intuit.com/api/core"

const (
	crmLeadsPath         = "/leads"
	crmLeadsSearchPath   = "/leads/search"
	crmLeadsCountPath    = "/leads/count"
	crmLeadsBulkPath     = "/leads/bulk"
	crmLeadsConvertPath  = "/leads/convert-to-customer"
	crmAssociationsPath  = "/lead-customer-associations"
	crmOpportunitiesPath = "/opportunities"
)

// crmDefaultSort is the sort the SPA appends to every list call
// (module 46136.js: sort=updateDate,desc).
const crmDefaultSort = "updateDate,desc"

// crmNote is stamped on every CRM plan; writePlan prints it on --dry-run.
const crmNote = "crm-leads-ui api/core; not sent"

// Empty-input sentinels. Like ErrEmptyExcludeIDs they fire before any
// session load or dial so callers cannot mistake a no-op for success.
var (
	ErrEmptyLeadID        = errors.New("crm lead requires a non-empty id")
	ErrEmptyOpportunityID = errors.New("crm opportunity requires a non-empty id")
	ErrEmptyLeadQuery     = errors.New("crm lead search requires a non-empty query")
)

// CrmLeadInput is the typed create/update body for /api/core/leads. Field
// names follow the lead projection surfaced by the SPA (displayName,
// organizationName); omitempty keeps partial updates honest.
type CrmLeadInput struct {
	DisplayName      string `json:"displayName,omitempty"`
	OrganizationName string `json:"organizationName,omitempty"`
	Email            string `json:"email,omitempty"`
	Phone            string `json:"phone,omitempty"`
	Status           string `json:"status,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

// CrmOpportunityInput is the typed create body for /api/core/opportunities.
type CrmOpportunityInput struct {
	Name       string  `json:"name,omitempty"`
	Stage      string  `json:"stage,omitempty"`
	Amount     float64 `json:"amount,omitempty"`
	CustomerID string  `json:"customerId,omitempty"`
}

// CrmLead is a secret-free projection of one lead row.
type CrmLead struct {
	ID               string `json:"id,omitempty"`
	DisplayName      string `json:"displayName,omitempty"`
	OrganizationName string `json:"organizationName,omitempty"`
	Email            string `json:"email,omitempty"`
	Phone            string `json:"phone,omitempty"`
	Status           string `json:"status,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
}

// CrmLeadListResult is the envelope returned by ReplayLeadList/Search.
type CrmLeadListResult struct {
	Status int       `json:"status"`
	Page   int       `json:"page"`
	Size   int       `json:"size"`
	Total  int       `json:"total"`
	Count  int       `json:"count"`
	Leads  []CrmLead `json:"leads"`
}

// CrmBulkResult summarises a /leads/bulk POST. Created counts the rows the
// response envelope carried; the endpoint's precise acknowledgement shape is
// unobserved, so no stronger claim is made.
type CrmBulkResult struct {
	Status     int    `json:"status"`
	SourceType string `json:"sourceType"`
	Created    int    `json:"created"`
}

// CrmConvertResult mirrors the convert-to-customer acknowledgement
// ({status: "CONVERTED", nameId}) observed in module 46136.js. A 204/empty
// body is treated as CONVERTED, matching the SPA's own interpretation.
type CrmConvertResult struct {
	Status     int    `json:"status"`
	State      string `json:"state,omitempty"`
	CustomerID string `json:"customerId,omitempty"`
}

// CrmAssociation is one lead↔customer link row.
type CrmAssociation struct {
	LeadID     string `json:"leadId,omitempty"`
	CustomerID string `json:"customerId,omitempty"`
}

// CrmAssociationResult is the envelope returned by ReplayAssociationList.
type CrmAssociationResult struct {
	Status       int              `json:"status"`
	Count        int              `json:"count"`
	Associations []CrmAssociation `json:"associations"`
}

// CrmOpportunity is a secret-free projection of one opportunity row.
type CrmOpportunity struct {
	ID         string  `json:"id,omitempty"`
	Name       string  `json:"name,omitempty"`
	Stage      string  `json:"stage,omitempty"`
	Status     string  `json:"status,omitempty"`
	Amount     float64 `json:"amount,omitempty"`
	CustomerID string  `json:"customerId,omitempty"`
}

// CrmOpportunityListResult is the envelope returned by ReplayOpportunityList.
type CrmOpportunityListResult struct {
	Status int              `json:"status"`
	Page   int              `json:"page"`
	Size   int              `json:"size"`
	Total  int              `json:"total"`
	Count  int              `json:"count"`
	Items  []CrmOpportunity `json:"items"`
}

// crmNewPlan stamps a secret-free RequestPlan for one api/core call.
func crmNewPlan(method, pathWithQuery, op string, body []byte) *RequestPlan {
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: method,
		URL:    crmCoreBaseURL + pathWithQuery,
		Body:   json.RawMessage(body),
		Note:   crmNote,
		op:     op,
	}
}

// crmLeadID sanitizes and validates a lead id. sanitizeToken strips path
// characters so an id can never escape the /leads/… prefix.
func crmLeadID(id string) (string, error) {
	id = sanitizeToken(id)
	if id == "" {
		return "", ErrEmptyLeadID
	}
	return id, nil
}

// crmOpportunityID sanitizes and validates an opportunity id.
func crmOpportunityID(id string) (string, error) {
	id = sanitizeToken(id)
	if id == "" {
		return "", ErrEmptyOpportunityID
	}
	return id, nil
}

// --- plans -----------------------------------------------------------------

// PlanLeadList builds the GET /api/core/leads page request.
func PlanLeadList(page, size int, inactive bool) *RequestPlan {
	v := url.Values{}
	v.Set("page", strconv.Itoa(page))
	v.Set("size", strconv.Itoa(size))
	v.Set("sort", crmDefaultSort)
	if inactive {
		v.Set("inactive", "true")
	}
	return crmNewPlan(http.MethodGet, crmLeadsPath+"?"+v.Encode(), "crm.leads.list", nil)
}

// PlanLeadGet builds the GET /api/core/leads/{id} request.
func PlanLeadGet(id string) (*RequestPlan, error) {
	id, err := crmLeadID(id)
	if err != nil {
		return nil, err
	}
	return crmNewPlan(http.MethodGet, crmLeadsPath+"/"+id, "crm.leads.get", nil), nil
}

// PlanLeadCreate builds the POST /api/core/leads request.
func PlanLeadCreate(in CrmLeadInput) (*RequestPlan, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("encoding lead: %w", err)
	}
	return crmNewPlan(http.MethodPost, crmLeadsPath, "crm.leads.create", body), nil
}

// PlanLeadUpdate builds the PUT /api/core/leads/{id} request.
func PlanLeadUpdate(id string, in CrmLeadInput) (*RequestPlan, error) {
	id, err := crmLeadID(id)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("encoding lead: %w", err)
	}
	return crmNewPlan(http.MethodPut, crmLeadsPath+"/"+id, "crm.leads.update", body), nil
}

// PlanLeadDelete builds the DELETE /api/core/leads/{id} request.
func PlanLeadDelete(id string) (*RequestPlan, error) {
	id, err := crmLeadID(id)
	if err != nil {
		return nil, err
	}
	return crmNewPlan(http.MethodDelete, crmLeadsPath+"/"+id, "crm.leads.delete", nil), nil
}

// PlanLeadSearch builds the GET /api/core/leads/search?q=… request. An empty
// query is rejected so callers cannot mistake an unfiltered page for a match.
func PlanLeadSearch(query string, page, size int) (*RequestPlan, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyLeadQuery
	}
	v := url.Values{}
	v.Set("q", query)
	v.Set("page", strconv.Itoa(page))
	v.Set("size", strconv.Itoa(size))
	return crmNewPlan(http.MethodGet, crmLeadsSearchPath+"?"+v.Encode(), "crm.leads.search", nil), nil
}

// PlanLeadCount builds the GET /api/core/leads/count request. sourceType is
// the optional sourceTypes filter (SPA param name: sourceTypes).
func PlanLeadCount(sourceType string) *RequestPlan {
	path := crmLeadsCountPath
	if st := strings.TrimSpace(sourceType); st != "" {
		v := url.Values{}
		v.Set("sourceTypes", st)
		path += "?" + v.Encode()
	}
	return crmNewPlan(http.MethodGet, path, "crm.leads.count", nil)
}

// crmBulkRequest is the POST /api/core/leads/bulk body
// (module 46136.js bulkCreateLeads: {sourceType, leadData}).
type crmBulkRequest struct {
	SourceType string         `json:"sourceType"`
	LeadData   []CrmLeadInput `json:"leadData"`
}

// PlanLeadBulk builds the POST /api/core/leads/bulk request. An empty
// sourceType defaults to CSV, the SPA default.
func PlanLeadBulk(sourceType string, leads []CrmLeadInput) (*RequestPlan, error) {
	st := strings.TrimSpace(sourceType)
	if st == "" {
		st = "CSV"
	}
	body, err := json.Marshal(crmBulkRequest{SourceType: st, LeadData: leads})
	if err != nil {
		return nil, fmt.Errorf("encoding bulk leadData: %w", err)
	}
	return crmNewPlan(http.MethodPost, crmLeadsBulkPath, "crm.leads.bulk", body), nil
}

// PlanLeadConvert builds the POST /api/core/leads/convert-to-customer
// request. The SPA's generic convert path takes no body in the observed
// estimate-accepted flow; the leadId is sent explicitly so the intent is
// auditable in dry-run output ([INFERENCE] on server acceptance).
func PlanLeadConvert(id string) (*RequestPlan, error) {
	id, err := crmLeadID(id)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]string{"leadId": id})
	if err != nil {
		return nil, fmt.Errorf("encoding convert body: %w", err)
	}
	return crmNewPlan(http.MethodPost, crmLeadsConvertPath, "crm.leads.convert", body), nil
}

// PlanAssociationList builds the GET /api/core/lead-customer-associations
// request. A non-empty customerID filters to that customer's association
// (?customerId=…, module 46136.js fetchLeadCustomerAssociation).
func PlanAssociationList(customerID string) *RequestPlan {
	path := crmAssociationsPath
	if cid := strings.TrimSpace(customerID); cid != "" {
		v := url.Values{}
		v.Set("customerId", cid)
		path += "?" + v.Encode()
	}
	return crmNewPlan(http.MethodGet, path, "crm.associations.list", nil)
}

// PlanOpportunityList builds the GET /api/core/opportunities page request.
func PlanOpportunityList(page, size int) *RequestPlan {
	v := url.Values{}
	v.Set("page", strconv.Itoa(page))
	v.Set("size", strconv.Itoa(size))
	v.Set("sort", crmDefaultSort)
	return crmNewPlan(http.MethodGet, crmOpportunitiesPath+"?"+v.Encode(), "crm.opportunities.list", nil)
}

// PlanOpportunityCreate builds the POST /api/core/opportunities request.
func PlanOpportunityCreate(in CrmOpportunityInput) (*RequestPlan, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("encoding opportunity: %w", err)
	}
	return crmNewPlan(http.MethodPost, crmOpportunitiesPath, "crm.opportunities.create", body), nil
}

// --- executors --------------------------------------------------------------

// ErrCrmNeedsRelayTab and the crmExecute transport live in crm_cdp.go:
// every CRM api/core call runs as an in-page fetch inside an
// authenticated qbo.intuit.com relay tab (Runtime.evaluate over CDP),
// because mccrmmessaging.api.intuit.com is cookie-only and rejects
// direct replay.

// --- replays ----------------------------------------------------------------

// ReplayLeadList GETs /api/core/leads. Read-only.
func ReplayLeadList(ctx context.Context, page, size int, inactive bool) (*CrmLeadListResult, error) {
	raw, status, err := crmExecute(ctx, PlanLeadList(page, size, inactive))
	if err != nil {
		return nil, err
	}
	res := &CrmLeadListResult{Status: status, Page: page, Size: size, Leads: []CrmLead{}}
	rows, total := crmRows(raw)
	if total == 0 {
		total = len(rows)
	}
	res.Total = total
	for _, r := range rows {
		if l := projectCrmLead(r); l.ID != "" || l.DisplayName != "" {
			res.Leads = append(res.Leads, l)
		}
	}
	res.Count = len(res.Leads)
	return res, nil
}

// ReplayLeadGet GETs /api/core/leads/{id}. Read-only.
func ReplayLeadGet(ctx context.Context, id string) (*CrmLead, error) {
	plan, err := PlanLeadGet(id)
	if err != nil {
		return nil, err
	}
	raw, _, err := crmExecute(ctx, plan)
	if err != nil {
		return nil, err
	}
	l := projectCrmLead(raw)
	return &l, nil
}

// ReplayLeadCreate POSTs /api/core/leads and projects the created lead.
func ReplayLeadCreate(ctx context.Context, in CrmLeadInput) (*CrmLead, error) {
	plan, err := PlanLeadCreate(in)
	if err != nil {
		return nil, err
	}
	raw, _, err := crmExecute(ctx, plan)
	if err != nil {
		return nil, err
	}
	l := projectCrmLead(raw)
	return &l, nil
}

// ReplayLeadUpdate PUTs /api/core/leads/{id} and projects the updated lead.
func ReplayLeadUpdate(ctx context.Context, id string, in CrmLeadInput) (*CrmLead, error) {
	plan, err := PlanLeadUpdate(id, in)
	if err != nil {
		return nil, err
	}
	raw, _, err := crmExecute(ctx, plan)
	if err != nil {
		return nil, err
	}
	l := projectCrmLead(raw)
	return &l, nil
}

// ReplayLeadDelete DELETEs /api/core/leads/{id} and returns the HTTP status.
func ReplayLeadDelete(ctx context.Context, id string) (int, error) {
	plan, err := PlanLeadDelete(id)
	if err != nil {
		return 0, err
	}
	_, status, err := crmExecute(ctx, plan)
	return status, err
}

// ReplayLeadSearch GETs /api/core/leads/search?q=…. Read-only.
func ReplayLeadSearch(ctx context.Context, query string, page, size int) (*CrmLeadListResult, error) {
	plan, err := PlanLeadSearch(query, page, size)
	if err != nil {
		return nil, err
	}
	raw, status, err := crmExecute(ctx, plan)
	if err != nil {
		return nil, err
	}
	res := &CrmLeadListResult{Status: status, Page: page, Size: size, Leads: []CrmLead{}}
	rows, total := crmRows(raw)
	if total == 0 {
		total = len(rows)
	}
	res.Total = total
	for _, r := range rows {
		if l := projectCrmLead(r); l.ID != "" || l.DisplayName != "" {
			res.Leads = append(res.Leads, l)
		}
	}
	res.Count = len(res.Leads)
	return res, nil
}

// ReplayLeadCount GETs /api/core/leads/count. The body is a bare number in
// the SPA flow (module 46136.js wraps it in Number()); anything unparsable
// counts as zero rather than inventing a figure.
func ReplayLeadCount(ctx context.Context, sourceType string) (int, error) {
	raw, _, err := crmExecute(ctx, PlanLeadCount(sourceType))
	if err != nil {
		return 0, err
	}
	return crmCountValue(raw), nil
}

// ReplayLeadBulk POSTs /api/core/leads/bulk.
func ReplayLeadBulk(ctx context.Context, sourceType string, leads []CrmLeadInput) (*CrmBulkResult, error) {
	plan, err := PlanLeadBulk(sourceType, leads)
	if err != nil {
		return nil, err
	}
	st := strings.TrimSpace(sourceType)
	if st == "" {
		st = "CSV"
	}
	raw, status, err := crmExecute(ctx, plan)
	if err != nil {
		return nil, err
	}
	rows, _ := crmRows(raw)
	return &CrmBulkResult{Status: status, SourceType: st, Created: len(rows)}, nil
}

// ReplayLeadConvert POSTs /api/core/leads/convert-to-customer.
func ReplayLeadConvert(ctx context.Context, id string) (*CrmConvertResult, error) {
	plan, err := PlanLeadConvert(id)
	if err != nil {
		return nil, err
	}
	raw, status, err := crmExecute(ctx, plan)
	if err != nil {
		return nil, err
	}
	res := &CrmConvertResult{Status: status, State: "CONVERTED"}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil && m != nil {
		if s := crmStr(m, "status", "state"); s != "" {
			res.State = s
		}
		res.CustomerID = crmStr(m, "nameId", "customerId", "customerID")
	}
	return res, nil
}

// ReplayAssociationList GETs /api/core/lead-customer-associations. Read-only.
func ReplayAssociationList(ctx context.Context, customerID string) (*CrmAssociationResult, error) {
	raw, status, err := crmExecute(ctx, PlanAssociationList(customerID))
	if err != nil {
		return nil, err
	}
	res := &CrmAssociationResult{Status: status, Associations: []CrmAssociation{}}
	rows, _ := crmRows(raw)
	for _, r := range rows {
		var m map[string]any
		if json.Unmarshal(r, &m) != nil {
			continue
		}
		a := CrmAssociation{
			LeadID:     crmStr(m, "leadId", "leadID"),
			CustomerID: crmStr(m, "customerId", "customerID"),
		}
		if a.LeadID != "" || a.CustomerID != "" {
			res.Associations = append(res.Associations, a)
		}
	}
	res.Count = len(res.Associations)
	return res, nil
}

// ReplayOpportunityList GETs /api/core/opportunities. Read-only.
func ReplayOpportunityList(ctx context.Context, page, size int) (*CrmOpportunityListResult, error) {
	raw, status, err := crmExecute(ctx, PlanOpportunityList(page, size))
	if err != nil {
		return nil, err
	}
	res := &CrmOpportunityListResult{Status: status, Page: page, Size: size, Items: []CrmOpportunity{}}
	rows, total := crmRows(raw)
	if total == 0 {
		total = len(rows)
	}
	res.Total = total
	for _, r := range rows {
		if o := projectCrmOpportunity(r); o.ID != "" || o.Name != "" {
			res.Items = append(res.Items, o)
		}
	}
	res.Count = len(res.Items)
	return res, nil
}

// ReplayOpportunityCreate POSTs /api/core/opportunities.
func ReplayOpportunityCreate(ctx context.Context, in CrmOpportunityInput) (*CrmOpportunity, error) {
	plan, err := PlanOpportunityCreate(in)
	if err != nil {
		return nil, err
	}
	raw, _, err := crmExecute(ctx, plan)
	if err != nil {
		return nil, err
	}
	o := projectCrmOpportunity(raw)
	return &o, nil
}

// --- projection helpers -----------------------------------------------------

// crmEnvelope is the Spring page shape the SPA consumes (content,
// totalElements); items/data cover alternate envelopes defensively.
type crmEnvelope struct {
	Content       []json.RawMessage `json:"content"`
	Items         []json.RawMessage `json:"items"`
	Data          []json.RawMessage `json:"data"`
	TotalElements int               `json:"totalElements"`
}

// crmRows extracts row payloads from a list body: a recognised envelope, or
// a bare JSON array. total prefers the envelope's totalElements.

// --- CRM response projection helpers (pure functions, testable offline) ---

func crmRows(raw []byte) ([]json.RawMessage, int) {
	var envelope struct {
		Content       []json.RawMessage `json:"content"`
		Items         []json.RawMessage `json:"items"`
		Data          []json.RawMessage `json:"data"`
		TotalElements *int              `json:"totalElements"`
	}
	if json.Unmarshal(raw, &envelope) == nil {
		rows := envelope.Content
		if len(rows) == 0 {
			rows = envelope.Items
		}
		if len(rows) == 0 {
			rows = envelope.Data
		}
		total := len(rows)
		if envelope.TotalElements != nil {
			total = *envelope.TotalElements
		}
		return rows, total
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		return arr, len(arr)
	}
	return nil, 0
}

func crmStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func crmNum(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k].(float64); ok {
			return v
		}
	}
	return 0
}

func projectCrmLead(raw json.RawMessage) CrmLead {
	var m map[string]any
	json.Unmarshal(raw, &m)
	name := crmStr(m, "displayName", "name", "organizationName", "companyName")
	id := crmStr(m, "nameId", "customerId", "id")
	return CrmLead{ID: id, DisplayName: name}
}

func projectCrmOpportunity(raw json.RawMessage) CrmOpportunity {
	var m map[string]any
	json.Unmarshal(raw, &m)
	return CrmOpportunity{ID: crmStr(m, "id"), Name: crmStr(m, "displayName")}
}

func crmCountValue(raw []byte) int {
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return int(n)
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		if c, ok := m["count"].(float64); ok {
			return int(c)
		}
	}
	return 0
}
