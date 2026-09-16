package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestCrmTransportErrors(t *testing.T) {
	want := errors.New("fake CRM transport failure")
	old := executeCRM
	executeCRM = func(context.Context, *RequestPlan) ([]byte, int, error) { return nil, 0, want }
	t.Cleanup(func() { executeCRM = old })
	if _, err := ReplayLeadList(context.Background(), 0, 25, false); !errors.Is(err, want) {
		t.Fatalf("lead list: got %v, want transport error", err)
	}
	if _, err := ReplayLeadGet(context.Background(), "L1"); !errors.Is(err, want) {
		t.Fatalf("lead get: got %v, want transport error", err)
	}
	if _, err := ReplayLeadCreate(context.Background(), CrmLeadInput{DisplayName: "test"}); !errors.Is(err, want) {
		t.Fatalf("lead create: got %v, want transport error", err)
	}
}

func TestCrmPlansStillValidateIDs(t *testing.T) {
	old := executeCRM
	executeCRM = func(context.Context, *RequestPlan) ([]byte, int, error) {
		t.Fatal("invalid ID reached transport")
		return nil, 0, nil
	}
	t.Cleanup(func() { executeCRM = old })
	if _, err := ReplayLeadGet(context.Background(), ""); !errors.Is(err, ErrEmptyLeadID) {
		t.Fatalf("got %v, want ErrEmptyLeadID", err)
	}
}

// leadListStub answers every CRM call with a fixed status/body.
func leadListStub(t *testing.T, status int, body string) {
	t.Helper()
	old := executeCRM
	executeCRM = func(context.Context, *RequestPlan) ([]byte, int, error) { return []byte(body), status, nil }
	t.Cleanup(func() { executeCRM = old })
}

// leadListCapture records the plan handed to the transport seam.
func leadListCapture(t *testing.T, status int, body string) **RequestPlan {
	t.Helper()
	var got *RequestPlan
	old := executeCRM
	executeCRM = func(_ context.Context, plan *RequestPlan) ([]byte, int, error) {
		got = plan
		return []byte(body), status, nil
	}
	t.Cleanup(func() { executeCRM = old })
	return &got
}

// TestPlanLeadListDataAccessContract pins the captured request shape: one
// POST to sbseggraphqlorch.api.intuit.com/graphql carrying the exact
// DataAccessLeads document and variables — never the stale /v3/api/leads
// GET route.
func TestPlanLeadListDataAccessContract(t *testing.T) {
	plan := PlanLeadList(0, 20, false)
	if plan.Method != http.MethodPost {
		t.Fatalf("method = %s, want POST", plan.Method)
	}
	if plan.URL != crmLeadsGraphQLURL {
		t.Fatalf("URL = %q, want %q", plan.URL, crmLeadsGraphQLURL)
	}
	if strings.Contains(plan.URL, "mccrmmessaging") {
		t.Fatalf("stale REST route in plan URL %q", plan.URL)
	}
	var payload struct {
		Query         string         `json:"query"`
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(plan.Body, &payload); err != nil {
		t.Fatalf("plan body is not JSON: %v", err)
	}
	if payload.Query != crmDataAccessLeadsQuery {
		t.Fatal("query document does not match the captured DataAccessLeads text")
	}
	if !strings.Contains(payload.Query, "dataAccessLeads(") || !strings.Contains(payload.Query, "pipelineStageId") {
		t.Fatal("query document lost captured selection fields")
	}
	if payload.OperationName != "" {
		t.Fatalf("captured payload carried no operationName, got %q", payload.OperationName)
	}
	vars := payload.Variables
	if vars["first"] != float64(20) || vars["offset"] != float64(0) {
		t.Fatalf("first/offset = %v/%v, want 20/0", vars["first"], vars["offset"])
	}
	orderBy, ok := vars["orderBy"].([]any)
	if !ok || len(orderBy) != 1 || orderBy[0] != "DISPLAY_NAME_ASC" {
		t.Fatalf("orderBy = %v, want [DISPLAY_NAME_ASC]", vars["orderBy"])
	}
	if got := leadActiveEquals(vars["filter"]); got == nil || !*got {
		t.Fatalf("filter active.equals = %v, want true (captured leads-tab filter)", got)
	}
	if got := leadCustomerIDExists(vars["filter"]); got == nil || *got {
		t.Fatalf("filter customerIdExists = %v, want false", got)
	}
}

func leadFilterAnd(filter any) map[string]any {
	f, _ := filter.(map[string]any)
	or, _ := f["or"].([]any)
	if len(or) == 0 {
		return nil
	}
	first, _ := or[0].(map[string]any)
	and, _ := first["and"].([]any)
	if len(and) == 0 {
		return nil
	}
	clause, _ := and[0].(map[string]any)
	return clause
}

func leadActiveEquals(filter any) *bool {
	clause := leadFilterAnd(filter)
	active, _ := clause["active"].(map[string]any)
	eq, ok := active["equals"].(bool)
	if !ok {
		return nil
	}
	return &eq
}

func leadCustomerIDExists(filter any) *bool {
	clause := leadFilterAnd(filter)
	exists, ok := clause["customerIdExists"].(bool)
	if !ok {
		return nil
	}
	return &exists
}

// TestPlanLeadListBounds keeps pagination deterministic: first is a
// PositiveInt clamped to [20-default floor, 100] and offset never negative.
func TestPlanLeadListBounds(t *testing.T) {
	for _, tc := range []struct {
		page, size int
		inactive   bool
		first      float64
		offset     float64
		activeEq   bool
	}{
		{0, 20, false, 20, 0, true},
		{2, 25, false, 25, 50, true},
		{0, 0, false, 20, 0, true},
		{0, -5, false, 20, 0, true},
		{1, 500, false, 100, 100, true},
		{-3, 25, false, 25, 0, true},
		{0, 20, true, 20, 0, false},
	} {
		plan := PlanLeadList(tc.page, tc.size, tc.inactive)
		var payload struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(plan.Body, &payload); err != nil {
			t.Fatalf("%+v: body not JSON: %v", tc, err)
		}
		if payload.Variables["first"] != tc.first || payload.Variables["offset"] != tc.offset {
			t.Fatalf("%+v: first/offset = %v/%v", tc, payload.Variables["first"], payload.Variables["offset"])
		}
		if got := leadActiveEquals(payload.Variables["filter"]); got == nil || *got != tc.activeEq {
			t.Fatalf("%+v: active.equals = %v, want %v", tc, got, tc.activeEq)
		}
	}
}

// TestReplayLeadListTypedResponse maps a full DataAccessLeads page onto
// CrmLead: native node fields only, directory fallbacks for email/phone,
// pipelineStageId surfaced as Status, meta timestamps through.
func TestReplayLeadListTypedResponse(t *testing.T) {
	body := `{"data":{"dataAccessLeads":{"edges":[` +
		`{"node":{"id":"L1","active":true,"displayName":"Acme","organizationName":"Acme Corp",` +
		`"emails":[{"emailAddress":"a@acme.invalid"}],"emailDirectory":{"primary":{"id":"E1","address":"primary@acme.invalid"}},` +
		`"phoneDirectory":{"primary":{"id":"P1","number":"555-1"}},"pipelineId":"PL1","pipelineStageId":"STAGE2",` +
		`"pipelineStageReason":null,"meta":{"createdAt":"2026-09-01T00:00:00Z","updatedAt":"2026-09-02T00:00:00Z"}}},` +
		`{"node":{"id":"L2","active":true,"displayName":"DirOnly","organizationName":"","emails":[],` +
		`"emailDirectory":{"primary":{"id":"E2","address":"dir@x.invalid"}},"phoneDirectory":null,` +
		`"pipelineId":null,"pipelineStageId":null,"pipelineStageReason":"cold",` +
		`"meta":{"createdAt":"2026-09-03T00:00:00Z","updatedAt":null}}}` +
		`],"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`
	leadListStub(t, http.StatusOK, body)
	got, err := ReplayLeadList(context.Background(), 0, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []CrmLead{
		{ID: "L1", DisplayName: "Acme", OrganizationName: "Acme Corp", Email: "a@acme.invalid", Phone: "555-1", Status: "STAGE2", CreatedAt: "2026-09-01T00:00:00Z", UpdatedAt: "2026-09-02T00:00:00Z"},
		{ID: "L2", DisplayName: "DirOnly", Email: "dir@x.invalid", CreatedAt: "2026-09-03T00:00:00Z"},
	}
	if !reflect.DeepEqual(got.Leads, want) {
		t.Fatalf("leads = %+v, want %+v", got.Leads, want)
	}
	if got.Status != http.StatusOK || got.Page != 0 || got.Size != 20 || got.Total != 2 || got.Count != 2 {
		t.Fatalf("envelope = %+v", got)
	}
}

// TestReplayLeadListEmpty covers the captured Test Company 2 response
// (edges:[], totalCount:0) and HTTP 204: both are an empty page, never an
// error and never a fabricated row.
func TestReplayLeadListEmpty(t *testing.T) {
	leadListStub(t, http.StatusOK, `{"data":{"dataAccessLeads":{"edges":[],"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`)
	got, err := ReplayLeadList(context.Background(), 0, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Leads == nil || got.Count != 0 || got.Total != 0 {
		t.Fatalf("empty page = %+v", got)
	}
	leadListStub(t, http.StatusNoContent, "")
	got, err = ReplayLeadList(context.Background(), 0, 20, false)
	if err != nil || got == nil || got.Count != 0 {
		t.Fatalf("204 page = %+v %v", got, err)
	}
}

// TestReplayLeadListGraphQLErrors proves a non-empty errors array fails the
// read — including partial data+errors, which is not a trustworthy page.
func TestReplayLeadListGraphQLErrors(t *testing.T) {
	for _, body := range []string{
		`{"errors":[{"message":"denied"}]}`,
		`{"data":{"dataAccessLeads":{"edges":[],"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":null}}},"errors":[{"message":"partial failure"}]}`,
	} {
		leadListStub(t, http.StatusOK, body)
		if _, err := ReplayLeadList(context.Background(), 0, 20, false); err == nil {
			t.Fatalf("accepted GraphQL error body %s", body)
		}
	}
}

// TestReplayLeadListMalformed proves the typed envelope is required: a
// missing/mis-typed dataAccessLeads collection, a missing edges array,
// malformed nodes and invalid totals all error. The stale REST page
// envelope is rejected — the old list route is not a response fallback.
func TestReplayLeadListMalformed(t *testing.T) {
	for i, body := range []string{
		`{"data":null}`,
		`{"data":{}}`,
		`{"data":[]}`,
		`{"data":{"dataAccessLeads":null}}`,
		`{"data":{"dataAccessLeads":[]}}`,
		`{"data":{"dataAccessLeads":{"totalCount":0}}}`,
		`{"data":{"dataAccessLeads":{"edges":{}}}}`,
		`{"data":{"dataAccessLeads":{"edges":"leads"}}}`,
		`{"data":{"dataAccessLeads":{"edges":[{}]}}}`,
		`{"data":{"dataAccessLeads":{"edges":[null]}}}`,
		`{"data":{"dataAccessLeads":{"edges":[{"node":null}]}}}`,
		`{"data":{"dataAccessLeads":{"edges":[{"node":{"displayName":5}}]}}}`,
		`{"data":{"dataAccessLeads":{"edges":[],"totalCount":-1}}}`,
		`{"data":{"dataAccessLeads":{"edges":[],"totalCount":"2"}}}`,
		`[]`,
		`{"content":[{"id":"L1","displayName":"Stale"}],"totalElements":1}`,
	} {
		leadListStub(t, http.StatusOK, body)
		if _, err := ReplayLeadList(context.Background(), 0, 20, false); err == nil {
			t.Fatalf("case %d: accepted malformed leads response %s", i, body)
		}
	}
}

// TestReplayLeadListNoFabricatedID proves a node that carries neither id
// nor displayName fails the whole read rather than surfacing an invented
// or blank lead.
func TestReplayLeadListNoFabricatedID(t *testing.T) {
	body := `{"data":{"dataAccessLeads":{"edges":[` +
		`{"node":{"id":"L1","displayName":"Real"}},` +
		`{"node":{"emails":[{"emailAddress":"anon@x.invalid"}]}}` +
		`],"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`
	leadListStub(t, http.StatusOK, body)
	if _, err := ReplayLeadList(context.Background(), 0, 20, false); err == nil {
		t.Fatal("anonymous node must fail the page, not yield a fabricated lead")
	}
}

// TestReplayLeadListPlanSentUnchanged pins the transport contract: the
// replayed request is the exact DataAccessLeads plan.
func TestReplayLeadListPlanSentUnchanged(t *testing.T) {
	got := leadListCapture(t, http.StatusOK, `{"data":{"dataAccessLeads":{"edges":[],"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`)
	if _, err := ReplayLeadList(context.Background(), 1, 25, true); err != nil {
		t.Fatal(err)
	}
	plan := *got
	if plan == nil {
		t.Fatal("no plan reached transport")
	}
	if plan.Method != http.MethodPost || plan.URL != crmLeadsGraphQLURL {
		t.Fatalf("sent %s %s, want POST %s", plan.Method, plan.URL, crmLeadsGraphQLURL)
	}
	var payload struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(plan.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Variables["first"] != float64(25) || payload.Variables["offset"] != float64(25) {
		t.Fatalf("page 1 size 25 -> first/offset = %v/%v, want 25/25", payload.Variables["first"], payload.Variables["offset"])
	}
	if eq := leadActiveEquals(payload.Variables["filter"]); eq == nil || *eq {
		t.Fatalf("inactive list -> active.equals = %v, want false", eq)
	}
}

// TestCrmRESTRoutesRemainStrict proves the REST repair is scoped to the
// list read: search keeps its captured REST plan/envelope reader (and its
// separately unproved status), record routes still demand an object
// acknowledgement, and a GraphQL envelope is not a valid REST page.
func TestCrmRESTRoutesRemainStrict(t *testing.T) {
	got := leadListCapture(t, http.StatusOK, `{"content":[{"nameId":"L1","displayName":"Lead"}],"totalElements":1}`)
	res, err := ReplayLeadSearch(context.Background(), "acme", 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if plan := *got; plan == nil || plan.Method != http.MethodGet || !strings.Contains(plan.URL, "/leads/search?") || !strings.Contains(plan.URL, "q=acme") {
		t.Fatalf("search plan changed: %+v", plan)
	}
	if res.Count != 1 || res.Leads[0].ID != "L1" {
		t.Fatalf("search leads = %+v", res)
	}
	leadListStub(t, http.StatusOK, `{"data":{"dataAccessLeads":{"edges":[],"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`)
	if _, err := ReplayLeadSearch(context.Background(), "acme", 0, 25); err == nil {
		t.Fatal("REST search accepted a GraphQL envelope")
	}
	leadListStub(t, http.StatusOK, `{"content":[]}`)
	if _, err := ReplayLeadGet(context.Background(), "L1"); err == nil {
		t.Fatal("record route accepted a list envelope")
	}
}
