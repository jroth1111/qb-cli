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
// Live leads-tab capture (2026-09-12, Test Company 2): the Customers &
// leads list reads through the DataAccessLeads query on
// sbseggraphqlorch.api.intuit.com/graphql — dataAccessLeads edges/node,
// totalCount and pageInfo — not the mccrmmessaging /v3/api/leads route.
// The stale REST list route is never used as a fallback. /leads/search
// remains on its captured REST plan and is separately unproved (no live
// search capture exists); no GraphQL search filter is invented.
//
// Response projections accept known envelopes and field aliases, but reject
// HTTP failures, malformed payloads, and missing acknowledgements. HTTP 204
// represents an empty list or a completed delete, not a fabricated record.
package client

import (
	"bytes"
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

// crmLeadsGraphQLURL serves the captured leads-tab DataAccessLeads query
// (POST {"query","variables"}; the captured payload carries no
// operationName field).
const crmLeadsGraphQLURL = "https://sbseggraphqlorch.api.intuit.com/graphql"

const (
	// crmLeadsDefaultPageSize is the captured page size (variables first:20).
	crmLeadsDefaultPageSize = 20
	// crmLeadsMaxPageSize bounds the $first variable (declared PositiveInt!)
	// so pagination input stays deterministic.
	crmLeadsMaxPageSize = 100
)

// crmDataAccessLeadsQuery is the exact document the SPA POSTs for the
// leads tab (capture 2026-09-12), including the pipeline and meta fields.
const crmDataAccessLeadsQuery = `
  query DataAccessLeads(
    $filter: DataAccess_LeadFilter
    $orderBy: [DataAccess_LeadSort!]
    $first: PositiveInt!
    $offset: Int
  ) {
    dataAccessLeads(
      filter: $filter
      orderBy: $orderBy
      first: $first
      offset: $offset
    ) {
      edges {
        node {
          id
          active
          displayName
          organizationName
          emails {
            emailAddress
          }
          emailDirectory {
            primary {
              id
              address
            }
          }
          phoneDirectory {
            primary {
              id
              number
            }
          }
          pipelineId
          pipelineStageId
          pipelineStageReason
          meta {
            createdAt
            updatedAt
          }
        }
      }
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
`

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
// ({status: "CONVERTED", nameId}) observed in module 46136.js. State and
// customer identity are populated only from an explicit acknowledgement.
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
	base := crmCoreBaseURL
	switch op {
	case "crm.leads.list":
		base = crmLeadsGraphQLURL
	case "crm.leads.get", "crm.leads.create", "crm.leads.update", "crm.leads.delete", "crm.leads.count":
		base = "https://mccrmmessaging.api.intuit.com/v1/api"
	}
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: method,
		URL:    base + pathWithQuery,
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

// --- plans -----------------------------------------------------------------

// crmLeadPageBounds clamps page/size to the captured contract: $first is a
// PositiveInt bounded at crmLeadsMaxPageSize and $offset never negative.
func crmLeadPageBounds(page, size int) (clampedPage, first, offset int) {
	if page < 0 {
		page = 0
	}
	if size < 1 {
		size = crmLeadsDefaultPageSize
	}
	if size > crmLeadsMaxPageSize {
		size = crmLeadsMaxPageSize
	}
	return page, size, page * size
}

// PlanLeadList builds the leads-tab DataAccessLeads POST. The captured
// variables filter to active leads not yet linked to a customer
// (active.equals true, customerIdExists false) ordered DISPLAY_NAME_ASC;
// the inactive flag flips the captured active.equals literal — the only
// activity axis the observed contract carries.
func PlanLeadList(page, size int, inactive bool) *RequestPlan {
	_, first, offset := crmLeadPageBounds(page, size)
	variables := map[string]any{
		"filter": map[string]any{
			"or": []any{map[string]any{
				"and": []any{map[string]any{
					"active":           map[string]any{"equals": !inactive},
					"customerIdExists": false,
				}},
			}},
		},
		"orderBy": []any{"DISPLAY_NAME_ASC"},
		"first":   first,
		"offset":  offset,
	}
	// The body is built from static types only; marshal cannot fail.
	body, _ := json.Marshal(map[string]any{
		"query":     crmDataAccessLeadsQuery,
		"variables": variables,
	})
	plan := crmNewPlan(http.MethodPost, "", "crm.leads.list", body)
	plan.Note = "crm-leads-ui DataAccessLeads POST; not sent"
	return plan
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
func crmLeadBody(in CrmLeadInput) map[string]any {
	body := map[string]any{}
	if in.DisplayName != "" {
		body["displayName"] = in.DisplayName
	}
	if in.OrganizationName != "" {
		body["organizationName"] = in.OrganizationName
	}
	if in.Email != "" {
		body["emails"] = []any{map[string]any{"email": in.Email, "variation": map[string]any{"ordinal": "PRIMARY"}}}
	}
	if in.Phone != "" {
		body["phones"] = []any{map[string]any{"originalNumber": in.Phone, "kind": "UNKNOWN", "variation": map[string]any{"ordinal": "PRIMARY"}}}
	}
	if in.Status != "" {
		body["leadStatus"] = in.Status
	}
	if in.Notes != "" {
		body["leadSummary"] = in.Notes
	}
	return body
}

func PlanLeadCreate(in CrmLeadInput) (*RequestPlan, error) {
	body := crmLeadBody(in)
	body["source"] = map[string]any{"integrationPlatform": "CRM", "platformSourceId": "QBO", "sourceType": "MANUAL"}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return crmNewPlan(http.MethodPut, crmLeadsPath, "crm.leads.create", raw), nil
}

func PlanLeadUpdate(id string, in CrmLeadInput) (*RequestPlan, error) {
	id, err := crmLeadID(id)
	if err != nil {
		return nil, err
	}
	body := crmLeadBody(in)
	body["id"] = id
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return crmNewPlan(http.MethodPatch, crmLeadsPath, "crm.leads.update", raw), nil
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
		v.Set("sourceType", st)
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

// executeCRM is the domain-test seam; the transport remains owned by crmExecute.
var executeCRM = crmExecute

// crmResponse validates HTTP and error envelopes before any domain projection.
// Error messages deliberately omit raw server bodies and request credentials.
func crmResponse(ctx context.Context, plan *RequestPlan) ([]byte, int, error) {
	raw, status, err := executeCRM(ctx, plan)
	if err != nil {
		return nil, status, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, status, &ReplayError{Status: status, Message: "CRM request failed"}
	}
	raw = bytes.TrimSpace(raw)
	if status == http.StatusNoContent {
		if len(raw) != 0 {
			return nil, status, fmt.Errorf("%s: unexpected body for HTTP 204", plan.op)
		}
		return nil, status, nil
	}
	if !json.Valid(raw) || bytes.Equal(raw, []byte("null")) {
		return nil, status, fmt.Errorf("%s: missing or malformed CRM response", plan.op)
	}
	if err := crmErrorEnvelope(raw); err != nil {
		return nil, status, fmt.Errorf("%s: %w", plan.op, err)
	}
	return raw, status, nil
}

func crmErrorEnvelope(raw []byte) error {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil // Arrays and scalar count responses are validated by their reader.
	}
	for _, key := range []string{"error", "errors", "fault", "Fault"} {
		var compact bytes.Buffer
		_ = json.Compact(&compact, m[key])
		switch compact.String() {
		case "", "null", "false", `""`, "[]", "{}":
		default:
			return errors.New("CRM returned an error envelope")
		}
	}
	for _, key := range []string{"ok", "success"} {
		if bytes.Equal(bytes.TrimSpace(m[key]), []byte("false")) {
			return errors.New("CRM did not acknowledge success")
		}
	}
	var status int
	if json.Unmarshal(m["status"], &status) == nil && status >= 400 {
		return errors.New("CRM returned a failing application status")
	}
	var state string
	if json.Unmarshal(m["status"], &state) == nil {
		switch strings.ToUpper(strings.TrimSpace(state)) {
		case "ERROR", "FAILED", "FAILURE":
			return errors.New("CRM returned a failing application status")
		}
	}
	return nil
}

// ReplayLeadList runs the leads-tab DataAccessLeads query. Read-only.
// The stale /v3/api/leads list route is not a fallback.
func ReplayLeadList(ctx context.Context, page, size int, inactive bool) (*CrmLeadListResult, error) {
	page, first, _ := crmLeadPageBounds(page, size)
	raw, status, err := crmResponse(ctx, PlanLeadList(page, size, inactive))
	if err != nil {
		return nil, err
	}
	leads, total, err := crmDataAccessLeadPage(raw)
	if err != nil {
		return nil, err
	}
	return &CrmLeadListResult{Status: status, Page: page, Size: first, Total: total, Count: len(leads), Leads: leads}, nil
}

// crmDataAccessLeadPage validates the typed GraphQL envelope
// {data:{dataAccessLeads:{edges,totalCount,pageInfo}}} and projects every
// edge.node. An empty raw body is possible only after crmResponse has
// validated HTTP 204. A missing or mis-typed collection is an error —
// never an implied empty page and never a legacy REST envelope.
func crmDataAccessLeadPage(raw []byte) ([]CrmLead, int, error) {
	leads := []CrmLead{}
	if len(raw) == 0 {
		return leads, 0, nil
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return nil, 0, errors.New("CRM leads response has no data envelope")
	}
	data := bytes.TrimSpace(envelope.Data)
	if len(data) == 0 || data[0] != '{' {
		return nil, 0, errors.New("CRM leads response has no data envelope")
	}
	var coll struct {
		DataAccessLeads json.RawMessage `json:"dataAccessLeads"`
	}
	var collection json.RawMessage
	if json.Unmarshal(data, &coll) == nil {
		collection = bytes.TrimSpace(coll.DataAccessLeads)
	}
	if len(collection) == 0 || bytes.Equal(collection, []byte("null")) || collection[0] != '{' {
		return nil, 0, errors.New("CRM leads response has no dataAccessLeads collection")
	}
	var page struct {
		Edges      json.RawMessage `json:"edges"`
		TotalCount *int            `json:"totalCount"`
	}
	if json.Unmarshal(collection, &page) != nil {
		return nil, 0, errors.New("CRM leads collection has invalid fields")
	}
	edges := bytes.TrimSpace(page.Edges)
	if len(edges) == 0 || edges[0] != '[' {
		return nil, 0, errors.New("CRM leads collection has no edges array")
	}
	var edgeList []struct {
		Node json.RawMessage `json:"node"`
	}
	if json.Unmarshal(edges, &edgeList) != nil {
		return nil, 0, errors.New("CRM leads collection has an invalid edges array")
	}
	total := len(edgeList)
	if page.TotalCount != nil {
		if *page.TotalCount < 0 {
			return nil, 0, errors.New("CRM leads collection has an invalid totalCount")
		}
		total = *page.TotalCount
	}
	for _, edge := range edgeList {
		lead, err := projectDataAccessLead(edge.Node)
		if err != nil {
			return nil, 0, err
		}
		leads = append(leads, lead)
	}
	return leads, total, nil
}

// projectDataAccessLead maps one dataAccessLeads edge.node onto CrmLead.
// Only native node fields populate the projection; a node without either
// id or displayName is not a lead acknowledgement.
func projectDataAccessLead(raw json.RawMessage) (CrmLead, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return CrmLead{}, errors.New("CRM leads edge has no node object")
	}
	var node struct {
		ID               string `json:"id"`
		DisplayName      string `json:"displayName"`
		OrganizationName string `json:"organizationName"`
		Emails           []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"emails"`
		EmailDirectory *struct {
			Primary *struct {
				Address string `json:"address"`
			} `json:"primary"`
		} `json:"emailDirectory"`
		PhoneDirectory *struct {
			Primary *struct {
				Number string `json:"number"`
			} `json:"primary"`
		} `json:"phoneDirectory"`
		PipelineStageID string `json:"pipelineStageId"`
		Meta            *struct {
			CreatedAt string `json:"createdAt"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"meta"`
	}
	if json.Unmarshal(raw, &node) != nil {
		return CrmLead{}, errors.New("CRM leads node has invalid fields")
	}
	lead := CrmLead{
		ID:               node.ID,
		DisplayName:      node.DisplayName,
		OrganizationName: node.OrganizationName,
		Status:           node.PipelineStageID,
	}
	for _, e := range node.Emails {
		if lead.Email == "" {
			lead.Email = e.EmailAddress
		}
	}
	if lead.Email == "" && node.EmailDirectory != nil && node.EmailDirectory.Primary != nil {
		lead.Email = node.EmailDirectory.Primary.Address
	}
	if node.PhoneDirectory != nil && node.PhoneDirectory.Primary != nil {
		lead.Phone = node.PhoneDirectory.Primary.Number
	}
	if node.Meta != nil {
		lead.CreatedAt = node.Meta.CreatedAt
		lead.UpdatedAt = node.Meta.UpdatedAt
	}
	if lead.ID == "" && lead.DisplayName == "" {
		return CrmLead{}, errors.New("CRM leads node has no lead acknowledgement")
	}
	return lead, nil
}

// crmLeadList reads the REST page envelopes still used by the search route.
func crmLeadList(ctx context.Context, plan *RequestPlan, page, size int) (*CrmLeadListResult, error) {
	raw, status, err := crmResponse(ctx, plan)
	if err != nil {
		return nil, err
	}
	rows, total, err := crmRows(raw)
	if err != nil {
		return nil, err
	}
	res := &CrmLeadListResult{Status: status, Page: page, Size: size, Total: total, Leads: []CrmLead{}}
	for _, row := range rows {
		lead, err := projectCrmLead(row)
		if err != nil {
			return nil, err
		}
		res.Leads = append(res.Leads, lead)
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
	return crmLeadRecord(ctx, plan)
}

func crmLeadRecord(ctx context.Context, plan *RequestPlan) (*CrmLead, error) {
	raw, _, err := crmResponse(ctx, plan)
	if err != nil {
		return nil, err
	}
	lead, err := projectCrmLead(raw)
	if err != nil {
		return nil, err
	}
	return &lead, nil
}

// ReplayLeadCreate POSTs /api/core/leads and projects the acknowledged lead.
func ReplayLeadCreate(ctx context.Context, in CrmLeadInput) (*CrmLead, error) {
	plan, err := PlanLeadCreate(in)
	if err != nil {
		return nil, err
	}
	return crmLeadRecord(ctx, plan)
}

// ReplayLeadUpdate PUTs /api/core/leads/{id} and projects the acknowledged lead.
func ReplayLeadUpdate(ctx context.Context, id string, in CrmLeadInput) (*CrmLead, error) {
	plan, err := PlanLeadUpdate(id, in)
	if err != nil {
		return nil, err
	}
	return crmLeadRecord(ctx, plan)
}

// ReplayLeadDelete DELETEs /api/core/leads/{id}. HTTP 204 needs no body.
func ReplayLeadDelete(ctx context.Context, id string) (int, error) {
	plan, err := PlanLeadDelete(id)
	if err != nil {
		return 0, err
	}
	_, status, err := crmResponse(ctx, plan)
	return status, err
}

// ReplayLeadSearch GETs /api/core/leads/search?q=…. Read-only. The REST
// search route is separately unproved — no live search capture exists —
// and no GraphQL search filter is invented for it.
func ReplayLeadSearch(ctx context.Context, query string, page, size int) (*CrmLeadListResult, error) {
	plan, err := PlanLeadSearch(query, page, size)
	if err != nil {
		return nil, err
	}
	return crmLeadList(ctx, plan, page, size)
}

// ReplayLeadCount requires a nonnegative integer count acknowledgement.
func ReplayLeadCount(ctx context.Context, sourceType string) (int, error) {
	raw, _, err := crmResponse(ctx, PlanLeadCount(sourceType))
	if err != nil {
		return 0, err
	}
	return crmCountValue(raw)
}

// ReplayLeadBulk counts only lead rows actually acknowledged by /leads/bulk.
func ReplayLeadBulk(ctx context.Context, sourceType string, leads []CrmLeadInput) (*CrmBulkResult, error) {
	plan, err := PlanLeadBulk(sourceType, leads)
	if err != nil {
		return nil, err
	}
	raw, status, err := crmResponse(ctx, plan)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("CRM bulk response has no acknowledged rows")
	}
	rows, _, err := crmRows(raw)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, err := projectCrmLead(row); err != nil {
			return nil, err
		}
	}
	st := strings.TrimSpace(sourceType)
	if st == "" {
		st = "CSV"
	}
	return &CrmBulkResult{Status: status, SourceType: st, Created: len(rows)}, nil
}

// ReplayLeadConvert never infers conversion from an empty response.
func ReplayLeadConvert(ctx context.Context, id string) (*CrmConvertResult, error) {
	plan, err := PlanLeadConvert(id)
	if err != nil {
		return nil, err
	}
	raw, status, err := crmResponse(ctx, plan)
	if err != nil {
		return nil, err
	}
	if err := crmObject(raw); err != nil {
		return nil, err
	}
	var ack struct {
		Status          string `json:"status"`
		State           string `json:"state"`
		NameID          string `json:"nameId"`
		CustomerID      string `json:"customerId"`
		CustomerIDAlias string `json:"customerID"`
	}
	if json.Unmarshal(raw, &ack) != nil {
		return nil, errors.New("CRM conversion acknowledgement has invalid fields")
	}
	state := firstNonEmpty(ack.Status, ack.State)
	customerID := firstNonEmpty(ack.NameID, ack.CustomerID, ack.CustomerIDAlias)
	if (state == "" && customerID == "") || (state != "" && !strings.EqualFold(state, "CONVERTED")) {
		return nil, errors.New("CRM response did not acknowledge conversion")
	}
	return &CrmConvertResult{Status: status, State: state, CustomerID: customerID}, nil
}

// ReplayAssociationList GETs /api/core/lead-customer-associations. Read-only.
func ReplayAssociationList(ctx context.Context, customerID string) (*CrmAssociationResult, error) {
	raw, status, err := crmResponse(ctx, PlanAssociationList(customerID))
	if err != nil {
		return nil, err
	}
	rows, _, err := crmRows(raw)
	if err != nil {
		return nil, err
	}
	res := &CrmAssociationResult{Status: status, Associations: []CrmAssociation{}}
	for _, row := range rows {
		if err := crmObject(row); err != nil {
			return nil, err
		}
		var a CrmAssociation
		// encoding/json accepts the existing leadID/customerID case aliases.
		if json.Unmarshal(row, &a) != nil || (a.LeadID == "" && a.CustomerID == "") {
			return nil, errors.New("CRM association response has no valid association")
		}
		res.Associations = append(res.Associations, a)
	}
	res.Count = len(res.Associations)
	return res, nil
}

// ReplayOpportunityList GETs /api/core/opportunities. Read-only.
func ReplayOpportunityList(ctx context.Context, page, size int) (*CrmOpportunityListResult, error) {
	raw, status, err := crmResponse(ctx, PlanOpportunityList(page, size))
	if err != nil {
		return nil, err
	}
	rows, total, err := crmRows(raw)
	if err != nil {
		return nil, err
	}
	res := &CrmOpportunityListResult{Status: status, Page: page, Size: size, Total: total, Items: []CrmOpportunity{}}
	for _, row := range rows {
		opportunity, err := projectCrmOpportunity(row)
		if err != nil {
			return nil, err
		}
		res.Items = append(res.Items, opportunity)
	}
	res.Count = len(res.Items)
	return res, nil
}

// ReplayOpportunityCreate projects only the returned opportunity acknowledgement.
func ReplayOpportunityCreate(ctx context.Context, in CrmOpportunityInput) (*CrmOpportunity, error) {
	plan, err := PlanOpportunityCreate(in)
	if err != nil {
		return nil, err
	}
	raw, _, err := crmResponse(ctx, plan)
	if err != nil {
		return nil, err
	}
	opportunity, err := projectCrmOpportunity(raw)
	if err != nil {
		return nil, err
	}
	return &opportunity, nil
}

// crmRows accepts the observed page aliases or a bare array. An empty raw
// body is possible only after crmResponse has validated HTTP 204.
func crmRows(raw []byte) ([]json.RawMessage, int, error) {
	if len(raw) == 0 {
		return nil, 0, nil
	}
	var rows []json.RawMessage
	if len(raw) > 0 && raw[0] == '[' && json.Unmarshal(raw, &rows) == nil {
		return rows, len(rows), nil
	}
	if err := crmObject(raw); err != nil {
		return nil, 0, err
	}
	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(raw, &envelope)
	found := false
	for _, key := range []string{"content", "items", "data"} {
		value, ok := envelope[key]
		if !ok {
			continue
		}
		value = bytes.TrimSpace(value)
		if len(value) == 0 || value[0] != '[' || json.Unmarshal(value, &rows) != nil {
			return nil, 0, errors.New("CRM list response has an invalid row array")
		}
		found = true
		if len(rows) > 0 {
			break
		}
	}
	if !found {
		return nil, 0, errors.New("CRM list response has no recognized row array")
	}
	total := len(rows)
	if v, ok := envelope["totalElements"]; ok {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) || json.Unmarshal(v, &total) != nil || total < 0 {
			return nil, 0, errors.New("CRM list response has an invalid total")
		}
	}
	return rows, total, nil
}

func crmObject(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' || !json.Valid(raw) {
		return errors.New("CRM response has no record acknowledgement")
	}
	return crmErrorEnvelope(raw)
}

func projectCrmLead(raw json.RawMessage) (CrmLead, error) {
	if err := crmObject(raw); err != nil {
		return CrmLead{}, err
	}
	var row struct {
		CrmLead
		NameID      string `json:"nameId"`
		CustomerID  string `json:"customerId"`
		Name        string `json:"name"`
		CompanyName string `json:"companyName"`
		Emails      []struct {
			Email string `json:"email"`
		} `json:"emails"`
		Phones []struct {
			OriginalNumber string `json:"originalNumber"`
		} `json:"phones"`
		LeadStatus string `json:"leadStatus"`
		CreateDate string `json:"createDate"`
		UpdateDate string `json:"updateDate"`
	}
	if json.Unmarshal(raw, &row) != nil {
		return CrmLead{}, errors.New("CRM lead response has invalid fields")
	}
	row.ID = firstNonEmpty(row.ID, row.NameID, row.CustomerID)
	row.DisplayName = firstNonEmpty(row.DisplayName, row.Name, row.OrganizationName, row.CompanyName)
	if row.Email == "" && len(row.Emails) > 0 {
		row.Email = row.Emails[0].Email
	}
	if row.Phone == "" && len(row.Phones) > 0 {
		row.Phone = row.Phones[0].OriginalNumber
	}
	row.Status = firstNonEmpty(row.Status, row.LeadStatus)
	row.CreatedAt = firstNonEmpty(row.CreatedAt, row.CreateDate)
	row.UpdatedAt = firstNonEmpty(row.UpdatedAt, row.UpdateDate)
	if row.ID == "" && row.DisplayName == "" {
		return CrmLead{}, errors.New("CRM response has no lead acknowledgement")
	}
	return row.CrmLead, nil
}

func projectCrmOpportunity(raw json.RawMessage) (CrmOpportunity, error) {
	if err := crmObject(raw); err != nil {
		return CrmOpportunity{}, err
	}
	var row struct {
		CrmOpportunity
		DisplayName string `json:"displayName"`
	}
	if json.Unmarshal(raw, &row) != nil {
		return CrmOpportunity{}, errors.New("CRM opportunity response has invalid fields")
	}
	row.Name = firstNonEmpty(row.DisplayName, row.Name)
	if row.ID == "" && row.Name == "" {
		return CrmOpportunity{}, errors.New("CRM response has no opportunity acknowledgement")
	}
	return row.CrmOpportunity, nil
}

func crmCountValue(raw []byte) (int, error) {
	var count int
	if len(raw) > 0 && raw[0] != '{' && !bytes.Equal(raw, []byte("null")) && json.Unmarshal(raw, &count) == nil && count >= 0 {
		return count, nil
	}
	var envelope struct {
		Count *int `json:"count"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Count != nil && *envelope.Count >= 0 {
		return *envelope.Count, nil
	}
	return 0, errors.New("CRM count response has no valid count")
}
