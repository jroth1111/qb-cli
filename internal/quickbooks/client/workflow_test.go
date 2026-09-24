package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ADVANCED.WORKFLOWS_CREATE/EDIT — appconnect
// /api/v1/subscriptions/{sub}/workflows/ CRUD, reverse-engineered from the
// workflow-plugin bundle's Apollo @rest directives and live-proven on TC2
// 2026-09-19 (create→201, PUT rename→200, ?action=activate→real IAC-002803
// incomplete-rule validation, DELETE→204). The subscription id resolves live
// per company; nothing is hardcoded.

// wfServer answers the subscription lookup and the workflows call off one
// httptest.Server, discriminating on the path suffix.
type wfServer struct {
	*httptest.Server
	mu        sync.Mutex
	calls     int
	lastMeth  string
	lastPath  string
	lastQuery string
	lastBody  []byte

	subsBody string
	wfStatus int
	wfBody   string
}

func newWFServer(t *testing.T) *wfServer {
	t.Helper()
	s := &wfServer{
		subsBody: `[{"id":"sub-9","intuitAppId":"AP15231","companyId":"r","state":"activated","name":"Workflow AppConnect"}]`,
		wfStatus: http.StatusCreated,
		wfBody:   `{"id":"wf-7","name":"zz","status":{"status":"inactive"}}`,
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.calls++
		s.lastMeth = r.Method
		s.lastPath = r.URL.Path
		s.lastQuery = r.URL.RawQuery
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/subscriptions") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(s.subsBody))
			return
		}
		w.WriteHeader(s.wfStatus)
		_, _ = w.Write([]byte(s.wfBody))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *wfServer) snap() (calls int, meth, path, query string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.lastMeth, s.lastPath, s.lastQuery, s.lastBody
}

func TestWorkflowCreatePostsNameAndTrack(t *testing.T) {
	saveUsableURIHost(t)
	srv := newWFServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayWorkflowMutate(context.Background(), "create", "", "zz-rule", "desc here", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusCreated || res.Item.ID != "wf-7" || res.Entity != "Workflow" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, path, _, body := srv.snap()
	if calls != 2 || meth != http.MethodPost {
		t.Fatalf("calls=%d meth=%s", calls, meth)
	}
	if !strings.HasSuffix(path, "/api/v1/subscriptions/sub-9/workflows/") {
		t.Fatalf("path = %s", path)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	if m["name"] != "zz-rule" || m["description"] != "desc here" || m["track"] != false {
		t.Fatalf("body = %v", m)
	}
}

func TestWorkflowUpdatePutsPatch(t *testing.T) {
	saveUsableURIHost(t)
	srv := newWFServer(t)
	srv.wfStatus = http.StatusOK
	srv.wfBody = `{"id":"wf-7","name":"renamed","status":"active"}`
	interceptHTTP(t, srv.URL)
	res, err := ReplayWorkflowMutate(context.Background(), "update", "wf-7", "renamed", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Item.ID != "wf-7" || res.Item.Type != "Workflow (active)" {
		t.Fatalf("res = %+v", res)
	}
	_, meth, path, _, body := srv.snap()
	if meth != http.MethodPut || !strings.HasSuffix(path, "/workflows/wf-7") {
		t.Fatalf("%s %s", meth, path)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["name"] != "renamed" {
		t.Fatalf("patch = %v", m)
	}
	if _, hasDesc := m["description"]; hasDesc {
		t.Fatalf("empty --description must not be sent: %v", m)
	}
}

func TestWorkflowUpdateActionQuery(t *testing.T) {
	saveUsableURIHost(t)
	srv := newWFServer(t)
	srv.wfStatus = http.StatusOK
	interceptHTTP(t, srv.URL)
	if _, err := ReplayWorkflowMutate(context.Background(), "update", "wf-7", "", "", "Deactivate"); err != nil {
		t.Fatal(err)
	}
	_, meth, path, query, body := srv.snap()
	if meth != http.MethodPost || !strings.HasSuffix(path, "/workflows/wf-7") {
		t.Fatalf("%s %s", meth, path)
	}
	if query != "action=deactivate" {
		t.Fatalf("query = %s", query)
	}
	if len(body) != 0 {
		t.Fatalf("action POST must be bodyless, got %s", body)
	}
}

func TestWorkflowValidation(t *testing.T) {
	saveUsableURIHost(t)
	srv := newWFServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayWorkflowMutate(context.Background(), "create", "", "  ", "", ""); err == nil {
		t.Fatal("blank --name must fail")
	}
	if _, err := ReplayWorkflowMutate(context.Background(), "update", " ", "x", "", ""); err == nil {
		t.Fatal("blank --id must fail")
	}
	if _, err := ReplayWorkflowMutate(context.Background(), "update", "wf-7", "", "", "pause"); err == nil {
		t.Fatal("bad --action must fail")
	}
	if _, err := ReplayWorkflowMutate(context.Background(), "update", "wf-7", "", "", ""); err == nil {
		t.Fatal("empty update must fail")
	}
	if _, err := ReplayWorkflowMutate(context.Background(), "nuke", "", "", "", ""); err == nil {
		t.Fatal("unknown op must fail")
	}
	if calls, _, _, _, _ := srv.snap(); calls != 0 {
		t.Fatalf("validation must precede HTTP, calls=%d", calls)
	}
}

func TestWorkflowHTTPErrorSurfaces(t *testing.T) {
	saveUsableURIHost(t)
	srv := newWFServer(t)
	srv.wfStatus = http.StatusBadRequest
	srv.wfBody = `{"errors":[{"message":"Workflow is incomplete","code":"IAC-002803","type":"INVALID_RESOURCE"}]}`
	interceptHTTP(t, srv.URL)
	_, err := ReplayWorkflowMutate(context.Background(), "update", "wf-7", "", "", "activate")
	if err == nil {
		t.Fatal("400 must error")
	}
	re, ok := err.(*ReplayError)
	if !ok || re.Status != http.StatusBadRequest || !strings.Contains(re.Message, "incomplete") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkflowNoActivatedSubscription(t *testing.T) {
	saveUsableURIHost(t)
	srv := newWFServer(t)
	srv.subsBody = `[{"id":"sub-1","intuitAppId":"AP15231","state":"suspended"}]`
	interceptHTTP(t, srv.URL)
	_, err := ReplayWorkflowMutate(context.Background(), "create", "", "zz", "", "")
	if err == nil || !strings.Contains(err.Error(), "no activated") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlannedWorkflowURLs(t *testing.T) {
	if !strings.Contains(PlannedWorkflowSubsURL(), "intuitAppId=AP15231") {
		t.Fatalf("subs URL = %s", PlannedWorkflowSubsURL())
	}
	if !strings.HasSuffix(PlannedWorkflowsURL(), "/{subscription}/workflows/") {
		t.Fatalf("workflows URL = %s", PlannedWorkflowsURL())
	}
}
