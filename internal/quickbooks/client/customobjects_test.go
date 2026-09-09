package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- test seam ---------------------------------------------------------------
//
// Both GraphQL entry points build production URLs and POST through
// postJSONExtra, whose http.Client has a nil Transport that falls back to
// http.DefaultTransport. Swapping http.DefaultTransport (interceptHTTP, same
// seam as client_complete_test.go) routes the production-built URLs to a
// local httptest server. No production file is touched; the swap is restored
// via t.Cleanup and nothing in this file calls t.Parallel.

// swapCostGroupSurfStdlib routes cost-group-svc calls through the stdlib
// client so interceptHTTP applies, mirroring saveUsableAudit for budgeting.
// Production keeps surf impersonation; the swap restores via t.Cleanup.
func swapCostGroupSurfStdlib(t *testing.T) {
	t.Helper()
	orig := postCostGroupGraphQLFn
	postCostGroupGraphQLFn = func(ctx context.Context, ac *apiClient, plan *RequestPlan, extra map[string]string) ([]byte, int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, plan.URL, bytes.NewReader([]byte(plan.Body)))
		if err != nil {
			return nil, 0, err
		}
		ac.applyHeaders(req, "")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		for k, v := range extra {
			if v != "" {
				req.Header.Set(k, v)
			}
		}
		resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer drainAndClose(resp)
		raw, err := readBody(resp)
		if err != nil {
			return nil, 0, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
		}
		return raw, resp.StatusCode, nil
	}
	t.Cleanup(func() { postCostGroupGraphQLFn = orig })
}

// cgServer records every request (path, body, Referer) and replies with a
// fixed status/body. One handler serves both hosts because the transport
// rewrite sends them to the same test server; path assertions distinguish
// them where it matters.
type cgServer struct {
	t          *testing.T
	status     int
	respBody   string
	lastPath   string
	lastMethod string
	lastBody   []byte
	lastRef    string
}

func newCGServer(t *testing.T, status int, respBody string) *cgServer {
	return &cgServer{t: t, status: status, respBody: respBody}
}

func (s *cgServer) start(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		s.lastPath = r.URL.Path
		s.lastMethod = r.Method
		s.lastBody = b
		s.lastRef = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.respBody))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (s *cgServer) calls() (path, method string, body []byte, referer string) {
	return s.lastPath, s.lastMethod, s.lastBody, s.lastRef
}

// listShape is the minimal GetCostGroupWithFilter envelope projectCostGroups
// needs; one edge is an error-union node to prove those are skipped.
const cgListShape = `{"data":{"projectManagementGetCostGroupWithFilter":{"edges":[
  {"node":{"__typename":"ProjectManagement_CostGroupError"},"cursor":"c0"},
  {"node":{"__typename":"ProjectManagement_CostGroupPayload","costGroup":{"id":"301","name":"Freight","description":"inbound","type":"ITEM","deleted":false,"version":3}},"cursor":"c1"},
  {"node":{"__typename":"ProjectManagement_CostGroupPayload","costGroup":{"id":"302","name":"Packaging","deleted":true}},"cursor":"c2"}
],"pageInfo":{"hasNextPage":false,"hasPreviousPage":false}}}}`

// --- happy path: list ---------------------------------------------------------

func TestReplayCostGroupListProjectsGroups(t *testing.T) {
	saveUsable(t)
	swapCostGroupSurfStdlib(t)
	srv := newCGServer(t, http.StatusOK, cgListShape)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayCostGroupList(context.Background(), 50, "active", "")
	if err != nil {
		t.Fatalf("ReplayCostGroupList: %v", err)
	}
	if res.Status != http.StatusOK {
		t.Fatalf("status = %d", res.Status)
	}
	if res.Count != 2 || len(res.Groups) != 2 {
		t.Fatalf("count=%d groups=%+v, want 2 (error node skipped)", res.Count, res.Groups)
	}
	g := res.Groups[0]
	if g.ID != "301" || g.Name != "Freight" || g.Type != "ITEM" || g.Version != 3 || g.Deleted {
		t.Fatalf("group[0] = %+v", g)
	}
	if !res.Groups[1].Deleted || res.Groups[1].ID != "302" {
		t.Fatalf("group[1] = %+v", res.Groups[1])
	}
	_, method, body, referer := srv.calls()
	if method != http.MethodPost {
		t.Fatalf("method = %s, want POST", method)
	}
	if !strings.Contains(referer, "/app/cost-groups") {
		t.Fatalf("referer = %q, want cost-groups SPA page", referer)
	}
	var req struct {
		OperationName string `json:"operationName"`
		Variables     struct {
			First   int            `json:"first"`
			Filter  map[string]any `json:"filter"`
			OrderBy []string       `json:"orderBy"`
		} `json:"variables"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, body)
	}
	if req.OperationName != "GetCostGroupsWithFilter" {
		t.Fatalf("operationName = %q", req.OperationName)
	}
	if req.Query == "" || !strings.Contains(req.Query, "projectManagementGetCostGroupWithFilter") {
		t.Fatalf("query missing captured operation: %q", req.Query)
	}
	if req.Variables.First != 50 {
		t.Fatalf("first = %d, want 50", req.Variables.First)
	}
	fl, isBool := req.Variables.Filter["deleted"].(bool)
	if !isBool || fl != false {
		t.Fatalf("filter.deleted = %v, want false for active status", req.Variables.Filter["deleted"])
	}
	if len(req.Variables.OrderBy) != 1 || req.Variables.OrderBy[0] != "NAME_ASC" {
		t.Fatalf("orderBy = %v, want [NAME_ASC]", req.Variables.OrderBy)
	}
}

// TestCostGroupFilterTriState pins the captured deleted tri-state mapping:
// all -> nil (no constraint), active -> false, inactive/deleted -> true, and
// unknown values fall back to active. Rejects implementations that send the
// Go zero value (false) for ALL — that would silently hide deleted rows.
func TestCostGroupFilterTriState(t *testing.T) {
	cases := []struct {
		status string
		want   any // nil means JSON null / absent constraint
		isNil  bool
		isTrue bool
	}{
		{status: "all", isNil: true},
		{status: "ALL", isNil: true},
		{status: " active ", isTrue: false},
		{status: "", isTrue: false},
		{status: "inactive", isTrue: true},
		{status: "deleted", isTrue: true},
		{status: "nonsense", isTrue: false}, // falls back to active
	}
	for _, tc := range cases {
		f := costGroupFilter(tc.status, "")
		got := f["deleted"]
		if tc.isNil {
			if got != nil {
				t.Fatalf("status %q: filter = %v, want deleted:null", tc.status, f)
			}
			continue
		}
		b, isBool := got.(bool)
		if !isBool || b != tc.isTrue {
			t.Fatalf("status %q: deleted = %v, want %v", tc.status, got, tc.isTrue)
		}
	}
	// The schema demands NonNull type: empty defaults to ITEM, explicit passes through.
	if f := costGroupFilter("active", ""); f["type"] != "ITEM" {
		t.Fatalf("default type = %v, want ITEM", f["type"])
	}
	if f := costGroupFilter("active", "SERVICE"); f["type"] != "SERVICE" {
		t.Fatalf("explicit type = %v, want SERVICE", f["type"])
	}
}

// TestPlanCustomObjectListBodyShape asserts the appFoundations query POST
// carries the captured operation name and query text.
func TestPlanCustomObjectListBodyShape(t *testing.T) {
	p := PlanCustomObjectList()
	if p.Method != http.MethodPost || p.Dial || !p.DryRun {
		t.Fatalf("plan envelope = %+v", p)
	}
	if p.URL != PlannedCustomObjectsURL() {
		t.Fatalf("url = %q", p.URL)
	}
	var req struct {
		OperationName string `json:"operationName"`
		Query         string `json:"query"`
	}
	if err := json.Unmarshal(p.Body, &req); err != nil {
		t.Fatalf("plan body not JSON: %v (%s)", err, p.Body)
	}
	if req.OperationName != "GetCustomObjectDefinitions" ||
		!strings.Contains(req.Query, "appFoundationsCustomObjectDefinitions") {
		t.Fatalf("plan = %s", p.Body)
	}
}

func TestReplayCustomObjectListProjectsDefinitions(t *testing.T) {
	saveUsable(t)
	srv := newCGServer(t, http.StatusOK, `{"data":{"appFoundationsCustomObjectDefinitions":{"edges":[
      {"node":{"id":"co1","label":"Contracts","active":true,"appliesTo":[{"entity":"Customer","scope":"HEADER","attribute":"","active":true}]}},
      {"node":{"id":"","label":"dropped"}},
      {"node":{"id":"co2","label":"Vessels","active":false}}
    ]}}}`)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayCustomObjectList(context.Background())
	if err != nil {
		t.Fatalf("ReplayCustomObjectList: %v", err)
	}
	if res.Count != 2 || len(res.Definitions) != 2 {
		t.Fatalf("result = %+v, want 2 definitions", res)
	}
	if res.Definitions[0].ID != "co1" || res.Definitions[0].Label != "Contracts" || !res.Definitions[0].Active {
		t.Fatalf("def[0] = %+v", res.Definitions[0])
	}
	if len(res.Definitions[0].AppliesTo) != 1 || res.Definitions[0].AppliesTo[0].Entity != "Customer" {
		t.Fatalf("appliesTo = %+v", res.Definitions[0].AppliesTo)
	}
	if _, _, _, referer := srv.calls(); !strings.Contains(referer, "/app/customobjects") {
		t.Fatalf("referer = %q, want customobjects SPA page", referer)
	}
}

func TestReplayCustomObjectGetFiltersByID(t *testing.T) {
	saveUsable(t)
	srv := newCGServer(t, http.StatusOK, `{"data":{"appFoundationsCustomObjectDefinitions":{"edges":[
      {"node":{"id":"co1","label":"Contracts","active":true}},
      {"node":{"id":"co2","label":"Vessels","active":false}}
    ]}}}`)
	interceptHTTP(t, srv.start(t))

	def, err := ReplayCustomObjectGet(context.Background(), " co2 ")
	if err != nil {
		t.Fatalf("ReplayCustomObjectGet: %v", err)
	}
	if def.ID != "co2" || def.Label != "Vessels" {
		t.Fatalf("def = %+v", def)
	}
}

func TestReplayCustomObjectGetNotFoundIsHonest(t *testing.T) {
	saveUsable(t)
	srv := newCGServer(t, http.StatusOK, `{"data":{"appFoundationsCustomObjectDefinitions":{"edges":[]}}}`)
	interceptHTTP(t, srv.start(t))

	_, err := ReplayCustomObjectGet(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not-found error", err)
	}
}

// --- happy path: create/update/delete round-trip shapes -----------------------

func TestReplayCostGroupCreatePostsCapturedInput(t *testing.T) {
	saveUsable(t)
	swapCostGroupSurfStdlib(t)
	srv := newCGServer(t, http.StatusOK,
		`{"data":{"projectManagementCreateCostGroup":{"costGroup":{"id":"777","name":"Freight","description":"","type":"ITEM","deleted":false,"version":0}}}}`)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayCostGroupCreate(context.Background(), " Freight ", "d", "")
	if err != nil {
		t.Fatalf("ReplayCostGroupCreate: %v", err)
	}
	if res.Status != http.StatusOK || res.Op != "create" || res.Entity != "CostGroup" || res.Item.ID != "777" || res.Item.Name != "Freight" {
		t.Fatalf("envelope = %+v", res)
	}
	_, _, body, _ := srv.calls()
	var req struct {
		OperationName string `json:"operationName"`
		Variables     struct {
			Input struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Type        string `json:"type"`
			} `json:"input"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, body)
	}
	if req.OperationName != "CreateCostGroup" {
		t.Fatalf("operationName = %q", req.OperationName)
	}
	if req.Variables.Input.Name != "Freight" {
		t.Fatalf("name = %q, want trimmed Freight", req.Variables.Input.Name)
	}
	if req.Variables.Input.Type != "ITEM" {
		t.Fatalf("type = %q, want ITEM default", req.Variables.Input.Type)
	}
}

func TestReplayCostGroupUpdateSendsVersion(t *testing.T) {
	saveUsable(t)
	swapCostGroupSurfStdlib(t)
	srv := newCGServer(t, http.StatusOK,
		`{"data":{"projectManagementUpdateCostGroup":{"costGroup":{"id":"777","name":"Freight II","version":4}}}}`)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayCostGroupUpdate(context.Background(), "777", "Freight II", "new desc", 3)
	if err != nil {
		t.Fatalf("ReplayCostGroupUpdate: %v", err)
	}
	if res.Op != "update" || res.Item.ID != "777" || res.Item.Name != "Freight II" {
		t.Fatalf("envelope = %+v", res)
	}
	_, _, body, _ := srv.calls()
	var req struct {
		Variables struct {
			Input struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			} `json:"input"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if req.Variables.Input.ID != "777" || req.Variables.Input.Version != 3 {
		t.Fatalf("input = %+v, want id 777 version 3", req.Variables.Input)
	}
}

func TestReplayCostGroupDeleteAndActivateToggle(t *testing.T) {
	for _, tc := range []struct {
		activate bool
		wantDel  bool
		wantOp   string
	}{
		{activate: false, wantDel: true, wantOp: "delete"},
		{activate: true, wantDel: false, wantOp: "activate"},
	} {
		t.Run(map[bool]string{false: "delete", true: "activate"}[tc.activate], func(t *testing.T) {
			saveUsable(t)
			swapCostGroupSurfStdlib(t)
			srv := newCGServer(t, http.StatusOK,
				`{"data":{"projectManagementToggleCostGroupDeletion":{"costGroup":{"id":"777","deleted":true,"version":5}}}}`)
			interceptHTTP(t, srv.start(t))

			res, err := ReplayCostGroupDelete(context.Background(), "777", 4, tc.activate)
			if err != nil {
				t.Fatalf("ReplayCostGroupDelete: %v", err)
			}
			if res.Op != tc.wantOp {
				t.Fatalf("op = %q, want %q", res.Op, tc.wantOp)
			}
			_, _, body, _ := srv.calls()
			var req struct {
				Variables struct {
					Input struct {
						ID      string `json:"id"`
						Version int    `json:"version"`
						Deleted bool   `json:"deleted"`
					} `json:"input"`
				} `json:"variables"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("body not JSON: %v", err)
			}
			in := req.Variables.Input
			if in.ID != "777" || in.Version != 4 || in.Deleted != tc.wantDel {
				t.Fatalf("toggle input = %+v, want id 777 v4 deleted=%v", in, tc.wantDel)
			}
		})
	}
}

// --- error surfaces ------------------------------------------------------------

func TestReplayCostGroupSurfacesGraphQLErrors(t *testing.T) {
	saveUsable(t)
	swapCostGroupSurfStdlib(t)
	srv := newCGServer(t, http.StatusOK,
		`{"data":{"projectManagementCreateCostGroup":{"code":"NAME_TAKEN","message":"duplicate name"}}}`)
	interceptHTTP(t, srv.start(t))

	_, err := ReplayCostGroupCreate(context.Background(), "Freight", "", "")
	if err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Fatalf("err = %v, want CostGroupError message", err)
	}
}

func TestReplayCostGroupSurfacesHTTPErrors(t *testing.T) {
	saveUsable(t)
	swapCostGroupSurfStdlib(t)
	srv := newCGServer(t, http.StatusForbidden, `{"message":"forbidden"}`)
	interceptHTTP(t, srv.start(t))

	_, err := ReplayCostGroupList(context.Background(), 25, "all", "")
	re, ok := err.(*ReplayError)
	if !ok || re.Status != http.StatusForbidden {
		t.Fatalf("err = %v, want *ReplayError 403", err)
	}
}

func TestReplayCostGroupsTopLevelErrorsSurface(t *testing.T) {
	saveUsable(t)
	swapCostGroupSurfStdlib(t)
	srv := newCGServer(t, http.StatusOK, `{"errors":[{"message":"bad token"}]}`)
	interceptHTTP(t, srv.start(t))

	if _, err := ReplayCostGroupList(context.Background(), 25, "", ""); err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("list err = %v", err)
	}
	if _, err := ReplayCostGroupDelete(context.Background(), "x", 0, true); err == nil || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("delete err = %v", err)
	}
}

// --- validation negatives (no server attached: no network possible) ------------

func TestCostGroupPlannersRejectMissingInput(t *testing.T) {
	cases := []struct {
		name    string
		plan    func() (*RequestPlan, error)
		wantErr error
	}{
		{"create empty name", func() (*RequestPlan, error) { return PlanCostGroupCreate("  ", "d", "") }, ErrEmptyCostGroupName},
		{"update empty id", func() (*RequestPlan, error) { return PlanCostGroupUpdate("", "n", "", 0) }, ErrEmptyCostGroupID},
		{"update empty name", func() (*RequestPlan, error) { return PlanCostGroupUpdate("1", "", "", 0) }, ErrEmptyCostGroupName},
		{"delete empty id", func() (*RequestPlan, error) { return PlanCostGroupDelete("  ", 0, false) }, ErrEmptyCostGroupID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.plan()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if p != nil {
				t.Fatalf("plan = %+v, want nil on validation failure", p)
			}
		})
	}
	if _, err := PlanCustomObjectGet(""); !errors.Is(err, ErrEmptyCustomObjectID) {
		t.Fatalf("PlanCustomObjectGet(\"\") err = %v, want ErrEmptyCustomObjectID", err)
	}
}

// TestClampFirstRejectsBadLimits pins page-size clamping: <1 defaults to 25,
// >100 caps at 100 (the SPA's own constant), in-between passes through.
func TestClampFirstRejectsBadLimits(t *testing.T) {
	for _, tc := range []struct{ in, want int }{{0, 25}, {-5, 25}, {1, 1}, {100, 100}, {101, 100}, {5000, 100}} {
		if got := clampFirst(tc.in); got != tc.want {
			t.Fatalf("clampFirst(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// --- no-credentials fast-fail ---------------------------------------------------

func TestCostGroupAndCustomObjectFailFastOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayCostGroupList(context.Background(), 25, "active", "")
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("list: got %v, want ErrNoCredentials", err)
	}

	start = time.Now()
	_, err = ReplayCostGroupCreate(context.Background(), "Freight", "", "")
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("create: got %v, want ErrNoCredentials", err)
	}

	start = time.Now()
	_, err = ReplayCustomObjectList(context.Background())
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("custom objects list: got %v, want ErrNoCredentials", err)
	}
}

// --- planned URLs never embed secrets or realms ---------------------------------

func TestPlannedGraphQLURLsAreSecretFree(t *testing.T) {
	if u := PlannedCostGroupsURL(); u != "https://cost-group-svc.api.intuit.com/graphql" {
		t.Fatalf("cost groups url = %q", u)
	}
	if u := PlannedCustomObjectsURL(); !strings.HasPrefix(u, "https://qbo.intuit.com/") {
		t.Fatalf("custom objects url = %q", u)
	}
	for _, p := range []*RequestPlan{
		PlanCostGroupList(10, "all", ""),
		mustPlan(t, func() (*RequestPlan, error) { return PlanCostGroupCreate("n", "d", "") }),
		mustPlan(t, func() (*RequestPlan, error) { return PlanCostGroupUpdate("1", "n", "", 0) }),
		mustPlan(t, func() (*RequestPlan, error) { return PlanCostGroupDelete("1", 0, true) }),
		PlanCustomObjectList(),
	} {
		if p.URL != PlannedCostGroupsURL() && p.URL != PlannedCustomObjectsURL() {
			t.Fatalf("%s url = %q, want service constant", p.op, p.URL)
		}
	}
}

func mustPlan(t *testing.T, f func() (*RequestPlan, error)) *RequestPlan {
	t.Helper()
	p, err := f()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
