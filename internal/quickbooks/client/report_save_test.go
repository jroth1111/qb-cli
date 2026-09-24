package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// REPORT_CREATE/EDIT/DELETE — saved custom reports (CRB_REPORT) on
// universalreportinsights /v1/reports/qbo, captured from the builder's
// "Save as new report" dialog and live-proven on TC2 2026-09-19 (POST →
// sbg:1dd30edb, PUT rename, DELETE → active:false "(deleted - …)", share
// PUT …/share?type=private). Delete is a soft flag like folio delete.

// savedReportServer answers the universalreportinsights saved-report routes,
// discriminating on method and the /share suffix.
type savedReportServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    int
	lastMeth string
	lastPath string
	lastBody []byte
	shareHit bool

	reportBody string
	status     int
}

func newSavedReportServer(t *testing.T) *savedReportServer {
	t.Helper()
	s := &savedReportServer{
		reportBody: `{"report":{"reportKindEnum":"CRB_REPORT","id":"sbg:e4d4b28d-9aca-4688-963b-7f6ce327a79c","name":"QBVerify upd","active":true,"dataRequest":{"filters":[{"on":{"datasetId":"TRANSACTION_DETAILS_V1","name":"DETAIL_TRANSACTION_DATE"},"operator":"DATE_MACRO","isPrimary":true,"value":{"macroName":"THIS_FISCAL_YEAR_TO_DATE"}}],"groups":[],"projections":[{"id":"p1","of":{"operation":null,"on":[{"name":"DETAIL_ACCOUNT_ID_REF","datasetId":"TRANSACTION_DETAILS_V1","type":"REF"}]},"as":"ACCOUNT","type":"RAW"}]}}}`,
		status:     http.StatusOK,
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
		s.lastPath = r.URL.Path + "?" + r.URL.RawQuery
		s.lastBody = b
		if strings.HasSuffix(r.URL.Path, "/share") {
			s.shareHit = true
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if s.status != http.StatusOK {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(`{"errorCode":500006,"errorMessage":"Required attribute not set"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(s.reportBody))
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"id":"sbg:e4d4b28d-9aca-4688-963b-7f6ce327a79c","active":false}`))
		default:
			_, _ = w.Write([]byte(`{"id":"sbg:e4d4b28d-9aca-4688-963b-7f6ce327a79c"}`))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *savedReportServer) snap() (calls int, meth, path string, body []byte, share bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.lastMeth, s.lastPath, s.lastBody, s.shareHit
}

func TestSavedReportCreatePostsNameAndSpec(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSavedReportServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySavedReportMutate(context.Background(), "create", "", "My Report", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Op != "create" || res.Item.ID != "sbg:e4d4b28d-9aca-4688-963b-7f6ce327a79c" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, path, body, _ := srv.snap()
	if calls != 1 || meth != http.MethodPost {
		t.Fatalf("request = %s (calls=%d)", meth, calls)
	}
	if !strings.Contains(path, "/v1/reports/qbo/") || !strings.Contains(path, "modelDimensionsAsLinkedRef=true") {
		t.Fatalf("path = %s", path)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	if m["name"] != "My Report" {
		t.Fatalf("name = %v", m["name"])
	}
	dr, _ := m["dataRequest"].(map[string]any)
	if dr == nil || dr["projections"] == nil || dr["filters"] == nil {
		t.Fatalf("dataRequest = %v", dr)
	}
}

func TestSavedReportCreateRequiresName(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSavedReportServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplaySavedReportMutate(context.Background(), "create", "", "  ", "", ""); err == nil {
		t.Fatal("expected missing-name error")
	}
	if calls, _, _, _, _ := srv.snap(); calls != 0 {
		t.Fatalf("server should not be called, calls=%d", calls)
	}
}

func TestSavedReportCreateCloneReusesDataRequest(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSavedReportServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySavedReportMutate(context.Background(), "create", "", "Cloned", "sbg:src", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "create" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, _, body, _ := srv.snap()
	if calls != 2 {
		t.Fatalf("expected GET source + POST, calls=%d", calls)
	}
	if meth != http.MethodPost {
		t.Fatalf("last method = %s", meth)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	dr, _ := m["dataRequest"].(map[string]any)
	projs, _ := dr["projections"].([]any)
	if len(projs) != 1 {
		t.Fatalf("cloned dataRequest projections = %v", dr)
	}
}

func TestSavedReportCreateSharePutsScope(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSavedReportServer(t)
	interceptHTTP(t, srv.URL)
	_, err := ReplaySavedReportMutate(context.Background(), "create", "", "Shared", "", "private")
	if err != nil {
		t.Fatal(err)
	}
	calls, meth, path, _, share := srv.snap()
	if calls != 2 || !share {
		t.Fatalf("expected POST + share PUT, calls=%d share=%v", calls, share)
	}
	if meth != http.MethodPut || !strings.Contains(path, "/share?type=private") {
		t.Fatalf("share request = %s %s", meth, path)
	}
}

func TestSavedReportUpdateGetsThenPuts(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSavedReportServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySavedReportMutate(context.Background(), "update", "sbg:e4d4b28d-9aca-4688-963b-7f6ce327a79c", "Renamed", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "update" || res.Item.Name != "Renamed" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, path, body, _ := srv.snap()
	if calls != 2 || meth != http.MethodPut {
		t.Fatalf("expected GET + PUT, calls=%d meth=%s", calls, meth)
	}
	if !strings.Contains(path, "sbg%3Ae4d4b28d-9aca-4688-963b-7f6ce327a79c") && !strings.Contains(path, "sbg:e4d4b28d-9aca-4688-963b-7f6ce327a79c") {
		t.Fatalf("PUT path = %s", path)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	if m["name"] != "Renamed" || m["dataRequest"] == nil {
		t.Fatalf("PUT body = %v", m)
	}
}

func TestSavedReportDeleteSoft(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSavedReportServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySavedReportMutate(context.Background(), "delete", "sbg:e4d4b28d-9aca-4688-963b-7f6ce327a79c", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "delete" || res.Item.Type != "SavedReport (deleted)" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, path, _, _ := srv.snap()
	if calls != 1 || meth != http.MethodDelete {
		t.Fatalf("request = %s %s (calls=%d)", meth, path, calls)
	}
}

func TestSavedReportErrorPropagatesStatus(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSavedReportServer(t)
	srv.status = http.StatusBadRequest
	interceptHTTP(t, srv.URL)
	_, err := ReplaySavedReportMutate(context.Background(), "create", "", "X", "", "")
	var re *ReplayError
	if err == nil || !errors.As(err, &re) || re.Status != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
}
