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

// REPORTS.CUSTOM_CREATE / REPORTS.MANAGEMENT_* — folio CRUD captured from
// /app/customreports and /app/managementreports. Live-proven on TC2
// 2026-09-19: POST /v1/folio→200 (CRB_GROUP and FOLIO), GET /v1/folio/{id}→200,
// PUT full folio→200 rename, DELETE→200 active:false soft-delete.

// folioServer answers /v1/folio and /v1/folio/{id}.
type folioServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    int
	lastMeth string
	lastPath string
	lastBody []byte
}

func newFolioServer(t *testing.T) *folioServer {
	t.Helper()
	s := &folioServer{}
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
		s.lastBody = b
		s.mu.Unlock()
		switch {
		case r.URL.Path == "/v1/folio" && r.Method == http.MethodPost:
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			m["id"] = "sbg:f9"
			out, _ := json.Marshal(m)
			_, _ = w.Write(out)
		case strings.HasPrefix(r.URL.Path, "/v1/folio/") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"reportKindEnum":"FOLIO","id":"sbg:f1","name":"Old","active":true,"reportingNamespace":"qbo","folioDataRequest":{"templateName":"Old","templateType":"USER","pages":[],"editSequence":0}}`))
		case strings.HasPrefix(r.URL.Path, "/v1/folio/") && r.Method == http.MethodPut:
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			out, _ := json.Marshal(m)
			_, _ = w.Write(out)
		case strings.HasPrefix(r.URL.Path, "/v1/folio/") && r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"reportKindEnum":"FOLIO","id":"sbg:f1","name":"Old (deleted - 1)","active":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *folioServer) snap() (calls int, meth, path string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.lastMeth, s.lastPath, s.lastBody
}

func TestFolioCustomCreatePostsCRBGroup(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "My Group", "PANDL", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Item.ID != "sbg:f9" || res.Entity != "Folio" {
		t.Fatalf("res = %+v", res)
	}
	_, meth, path, body := srv.snap()
	if meth != http.MethodPost || path != "/v1/folio" {
		t.Fatalf("%s %s", meth, path)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["reportKindEnum"] != "CRB_GROUP" || m["name"] != "My Group" || m["active"] != true {
		t.Fatalf("body = %v", m)
	}
	req, _ := m["folioDataRequest"].(map[string]any)
	pages, _ := req["pages"].([]any)
	page, _ := pages[0].(map[string]any)
	if len(pages) != 1 || page["type"] != "URI_REPORT" || page["reportToken"] != "PANDL" || page["customReport"] != false {
		t.Fatalf("pages = %v", pages)
	}
}

func TestFolioManagementCreatePostsCoverAndReport(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayFolioMutate(context.Background(), "management", "create", "", "Mgmt", "PANDL", "", "thisyear", "A Person"); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["reportKindEnum"] != "FOLIO" {
		t.Fatalf("kind = %v", m["reportKindEnum"])
	}
	req, _ := m["folioDataRequest"].(map[string]any)
	pages, _ := req["pages"].([]any)
	if len(pages) != 2 {
		t.Fatalf("pages = %v", pages)
	}
	cover, _ := pages[0].(map[string]any)
	report, _ := pages[1].(map[string]any)
	if cover["type"] != "COVER_PAGE" || cover["preparedBy"] != "A Person" || cover["subTitle"] != "{CompanyName}" {
		t.Fatalf("cover = %v", cover)
	}
	if report["type"] != "URI_REPORT" || report["reportToken"] != "PANDL" || report["reportDateMacro"] != "thisyear" {
		t.Fatalf("report = %v", report)
	}
	df, _ := req["dynamicFields"].(map[string]any)
	if df["{ReportEndDate}"] == "" || req["dateMacro"] != "thisyear" {
		t.Fatalf("req = %v", req)
	}
}

func TestFolioSbgTokenMarksCustomReport(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "G", "sbg:440e822b-x", "", "", ""); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	req, _ := m["folioDataRequest"].(map[string]any)
	page, _ := req["pages"].([]any)[0].(map[string]any)
	if page["customReport"] != true || page["reportToken"] != "sbg:440e822b-x" {
		t.Fatalf("page = %v", page)
	}
}

func TestFolioUpdateGetsThenPutsFullFolio(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayFolioMutate(context.Background(), "management", "update", "sbg:f1", "Renamed", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.Name != "Renamed" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, path, body := srv.snap()
	if calls != 2 || meth != http.MethodPut || path != "/v1/folio/sbg:f1" {
		t.Fatalf("calls=%d %s %s", calls, meth, path)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["id"] != "sbg:f1" || m["name"] != "Renamed" || m["active"] != true {
		t.Fatalf("body = %v", m)
	}
	req, _ := m["folioDataRequest"].(map[string]any)
	if req["templateName"] != "Renamed" {
		t.Fatalf("templateName not synced: %v", req)
	}
}

func TestFolioValidationBeforeHTTP(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "", "", "", "", ""); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, err := ReplayFolioMutate(context.Background(), "management", "update", "", "X", "", "", "", ""); err == nil {
		t.Fatal("empty id should fail")
	}
	if _, err := ReplayFolioMutate(context.Background(), "management", "update", "sbg:f1", "", "", "", "", ""); err == nil {
		t.Fatal("empty update name should fail")
	}
	if _, err := ReplayFolioMutate(context.Background(), "bogus", "create", "", "X", "", "", "", ""); err == nil {
		t.Fatal("unknown kind should fail")
	}
	if calls, _, _, _ := srv.snap(); calls != 0 {
		t.Fatalf("validation made %d HTTP calls", calls)
	}
}

func TestFolioServiceErrorPropagates(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"folio name required"}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	_, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "X", "", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "folio name required") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlannedFolioMutateURL(t *testing.T) {
	if PlannedFolioMutateURL() != "https://universalreportinsights.api.intuit.com/v1/folio" {
		t.Fatalf("url = %q", PlannedFolioMutateURL())
	}
}
