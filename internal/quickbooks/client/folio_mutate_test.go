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
			_, _ = w.Write([]byte(`{"reportKindEnum":"FOLIO","id":"sbg:f1","name":"Old","active":true,"reportingNamespace":"qbo","folioDataRequest":{"templateName":"Old","templateType":"USER","dateMacro":"custom","startDate":"2026-05-01","endDate":"2026-05-31","pages":[{"pageType":"page","type":"COVER_PAGE","title":"Old"},{"pageType":"report","type":"URI_REPORT","title":"Detail","reportToken":"42","reportDateMacro":"custom","startDate":"2026-05-01","endDate":"2026-05-31","customReport":true},{"pageType":"report","type":"URI_REPORT","title":"Owner Distributions","reportToken":"70","reportDateMacro":"all","customReport":true}],"editSequence":0}}`))
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
	res, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "My Group", []string{"PANDL"}, "", "", "", "", "")
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
	if _, err := ReplayFolioMutate(context.Background(), "management", "create", "", "Mgmt", []string{"PANDL"}, "", "thisyear", "", "", "A Person"); err != nil {
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
	if _, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "G", []string{"sbg:440e822b-x"}, "", "", "", "", ""); err != nil {
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

// Repeated --report produces one report page per token in flag order — the
// live multi-property statement shape (per-unit details + combined summary
// + owner distributions) as a single create.
func TestFolioManagementCreateMultiReportPages(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	reports := []string{"42", "41", "59", "70"}
	if _, err := ReplayFolioMutate(context.Background(), "management", "create", "", "Melo and Gingelly Statement", reports, "", "", "2026-05-01", "2026-05-31", "P"); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	req, _ := m["folioDataRequest"].(map[string]any)
	pages := req["pages"].([]any)
	if len(pages) != 5 {
		t.Fatalf("pages = %d, want cover + 4 report pages", len(pages))
	}
	for i, tok := range reports {
		page, _ := pages[i+1].(map[string]any)
		if page["reportToken"] != tok || page["customReport"] != true || page["reportDateMacro"] != "custom" {
			t.Fatalf("page %d = %v", i+1, page)
		}
	}
}

// A TOKEN:macro suffix overrides that page's period scope: "70:all" keeps
// an owner-distributions page all-time (the live folio shape) while sibling
// pages pin the statement window.
func TestFolioCreatePageMacroOverride(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	reports := []string{"42", "70:all"}
	if _, err := ReplayFolioMutate(context.Background(), "management", "create", "", "S", reports, "", "", "2026-06-01", "2026-06-30", "P"); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	req, _ := m["folioDataRequest"].(map[string]any)
	pages := req["pages"].([]any)
	if len(pages) != 3 {
		t.Fatalf("pages = %d, want cover + 2 report pages", len(pages))
	}
	detail, _ := pages[1].(map[string]any)
	if detail["reportToken"] != "42" || detail["reportDateMacro"] != "custom" || detail["startDate"] != "2026-06-01" || detail["endDate"] != "2026-06-30" {
		t.Fatalf("detail page = %v", detail)
	}
	dist, _ := pages[2].(map[string]any)
	if dist["reportToken"] != "70" || dist["reportDateMacro"] != "all" {
		t.Fatalf("distributions page = %v", dist)
	}
	if _, ok := dist["startDate"]; ok {
		t.Fatalf("all-macro page must not pin dates: %v", dist)
	}
	if dist["customReport"] != true {
		t.Fatalf("distributions page customReport = %v", dist["customReport"])
	}
	if dist["title"] != "70" {
		t.Fatalf("title should be the stripped token, got %v", dist["title"])
	}
}

// sbg: tokens contain a colon but must not split on a macro-looking
// boundary — the UUID tail is never a macro name.
func TestFolioSbgTokenDoesNotSplitMacro(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	reports := []string{"sbg:3ba75975-cb3c-494b-b458-be738a884e1c"}
	if _, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "S", reports, "", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	req, _ := m["folioDataRequest"].(map[string]any)
	page, _ := req["pages"].([]any)[0].(map[string]any)
	if page["reportToken"] != "sbg:3ba75975-cb3c-494b-b458-be738a884e1c" || page["customReport"] != true {
		t.Fatalf("page = %v", page)
	}
}

// An unknown suffix stays part of the token — the server, not the parser,
// rejects bad tokens.
func TestFolioUnknownSuffixStaysInToken(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	reports := []string{"foo:bar"}
	if _, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "S", reports, "", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	req, _ := m["folioDataRequest"].(map[string]any)
	page, _ := req["pages"].([]any)[0].(map[string]any)
	if page["reportToken"] != "foo:bar" {
		t.Fatalf("token = %v", page["reportToken"])
	}
}

// A relative macro other than "all" also carries no pinned dates.
func TestFolioRelativeMacroPageNoDates(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	reports := []string{"42:lastmonth"}
	if _, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "S", reports, "", "", "2026-06-01", "2026-06-30", ""); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	req, _ := m["folioDataRequest"].(map[string]any)
	page, _ := req["pages"].([]any)[0].(map[string]any)
	if page["reportToken"] != "42" || page["reportDateMacro"] != "lastmonth" {
		t.Fatalf("page = %v", page)
	}
	if _, ok := page["startDate"]; ok {
		t.Fatalf("relative-macro page must not pin dates: %v", page)
	}
}

// Numeric reportTokens are classic mem_rpt_ids — live management folios
// carry customReport:true on those pages (tokens 41/42/59/70/162/558).
func TestFolioNumericTokenMarksCustomReport(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayFolioMutate(context.Background(), "management", "create", "", "S", []string{"42"}, "", "", "2026-05-01", "2026-05-31", "P"); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	req, _ := m["folioDataRequest"].(map[string]any)
	if req["dateMacro"] != "custom" || req["startDate"] != "2026-05-01" || req["endDate"] != "2026-05-31" {
		t.Fatalf("folio period = %v", req)
	}
	pages := req["pages"].([]any)
	report, _ := pages[1].(map[string]any)
	if report["customReport"] != true || report["reportToken"] != "42" ||
		report["reportDateMacro"] != "custom" || report["startDate"] != "2026-05-01" || report["endDate"] != "2026-05-31" {
		t.Fatalf("report page = %v", report)
	}
	if df := req["dynamicFields"].(map[string]any); df["{ReportEndDate}"] != "31 May 2026" {
		t.Fatalf("dynamicFields = %v", df)
	}
}

// management update --start-date/--end-date repins the statement period:
// the folio window and every period-bound report page take the new range,
// while reportDateMacro:"all" pages (owner distributions) stay untouched.
func TestFolioManagementUpdatePinsPeriod(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayFolioMutate(context.Background(), "management", "update", "sbg:f1", "", nil, "", "", "2026-06-01", "2026-06-30", ""); err != nil {
		t.Fatal(err)
	}
	_, meth, _, body := srv.snap()
	if meth != http.MethodPut {
		t.Fatalf("method = %s", meth)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["name"] != "Old" {
		t.Fatalf("name changed without --name: %v", m["name"])
	}
	req, _ := m["folioDataRequest"].(map[string]any)
	if req["dateMacro"] != "custom" || req["startDate"] != "2026-06-01" || req["endDate"] != "2026-06-30" {
		t.Fatalf("folio period = %v", req)
	}
	pages := req["pages"].([]any)
	detail, _ := pages[1].(map[string]any)
	if detail["startDate"] != "2026-06-01" || detail["endDate"] != "2026-06-30" || detail["reportDateMacro"] != "custom" {
		t.Fatalf("detail page = %v", detail)
	}
	dist, _ := pages[2].(map[string]any)
	if dist["reportDateMacro"] != "all" || dist["startDate"] != nil || dist["endDate"] != nil {
		t.Fatalf("distributions page was repinned: %v", dist)
	}
}

func TestPinFolioPeriodPinsEveryReportAndCoverButPreservesAllDates(t *testing.T) {
	var req map[string]any
	if err := json.Unmarshal([]byte(`{"dynamicFields":{"{CompanyName}":"Company","{ReportEndDate}":"31 May 2026"},"pages":[{"type":"COVER_PAGE","reportPeriod":"For the period ended {ReportEndDate}"},{"type":"CUSTOM_PAGE","title":"Notes"},{"type":"REPORT","reportToken":"42"},{"type":"URI_REPORT","reportToken":"41","reportDateMacro":"lastmonth"},{"type":"REPORT","reportToken":"59","reportDateMacro":"custom","startDate":"2026-05-01","endDate":"2026-05-31"},{"type":"REPORT","reportToken":"70","reportDateMacro":"ALL"}]}`), &req); err != nil {
		t.Fatal(err)
	}
	pinFolioPeriod(req, "2026-06-01", "2026-06-30")
	fields := req["dynamicFields"].(map[string]any)
	if fields["{ReportEndDate}"] != "30 June 2026" || fields["{CompanyName}"] != "Company" {
		t.Fatalf("cover fields = %v", fields)
	}
	pages := req["pages"].([]any)
	for _, i := range []int{2, 3, 4} {
		p := pages[i].(map[string]any)
		if p["reportDateMacro"] != "custom" || p["startDate"] != "2026-06-01" || p["endDate"] != "2026-06-30" {
			t.Fatalf("segment %d not pinned: %v", i, p)
		}
	}
	for _, i := range []int{0, 1, 5} {
		p := pages[i].(map[string]any)
		if p["startDate"] != nil || p["endDate"] != nil {
			t.Fatalf("non-period segment %d changed: %v", i, p)
		}
	}
	if pages[5].(map[string]any)["reportDateMacro"] != "ALL" {
		t.Fatal("all-date ledger macro changed")
	}
}

func TestFolioUpdateGetsThenPutsFullFolio(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFolioServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayFolioMutate(context.Background(), "management", "update", "sbg:f1", "Renamed", nil, "", "", "", "", "")
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
	if _, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "", nil, "", "", "", "", ""); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, err := ReplayFolioMutate(context.Background(), "management", "update", "", "X", nil, "", "", "", "", ""); err == nil {
		t.Fatal("empty id should fail")
	}
	if _, err := ReplayFolioMutate(context.Background(), "management", "update", "sbg:f1", "", nil, "", "", "", "", ""); err == nil {
		t.Fatal("empty update name should fail")
	}
	if _, err := ReplayFolioMutate(context.Background(), "bogus", "create", "", "X", nil, "", "", "", "", ""); err == nil {
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
	_, err := ReplayFolioMutate(context.Background(), "custom", "create", "", "X", nil, "", "", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "folio name required") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlannedFolioMutateURL(t *testing.T) {
	if PlannedFolioMutateURL() != "https://universalreportinsights.api.intuit.com/v1/folio" {
		t.Fatalf("url = %q", PlannedFolioMutateURL())
	}
}
