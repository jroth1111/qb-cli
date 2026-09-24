package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The statement contract posts type+customerIds+dates to
// /api/neo/v1/company/{realm}/statement/print after resolving the customer
// name via v3 query.
func statementServer(t *testing.T) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
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
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/v3/company/") {
			_, _ = w.Write([]byte(`{"QueryResponse":{"Customer":[{"Id":"1","DisplayName":"CR-Test-Customer-Edited"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestStatementWireShape(t *testing.T) {
	srv := statementServer(t)
	res, err := ReplayStatementCreate(context.Background(),
		"CR-Test-Customer-Edited", "17/08/2026", "18/09/2026", "balance-forward")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Op != "statement create" || res.Entity != "Statement" || res.Item.ID != "1" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/statement/print") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	var doc struct {
		Type        string  `json:"type"`
		CustomerIDs []int64 `json:"customerIds"`
		StartDate   string  `json:"startDate"`
		EndDate     string  `json:"endDate"`
		StmtDate    string  `json:"statementDate"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("body: %v", err)
	}
	if doc.Type != "1" || len(doc.CustomerIDs) != 1 || doc.CustomerIDs[0] != 1 {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.StartDate != "2026-08-17" || doc.EndDate != "2026-09-18" || doc.StmtDate != "2026-09-18" {
		t.Fatalf("dates = %+v", doc)
	}
}

func TestStatementTypeEnum(t *testing.T) {
	srv := statementServer(t)
	for in, want := range map[string]string{
		"balance-forward":       "1",
		"Balance Forward":       "1",
		"open-item":             "2",
		"transaction":           "3",
		"Transaction Statement": "3",
		"customer statement":    "2",
		"1":                     "1",
	} {
		if _, err := ReplayStatementCreate(context.Background(),
			"CR-Test-Customer-Edited", "17/08/2026", "18/09/2026", in); err != nil {
			t.Fatalf("type %q: %v", in, err)
		}
		_, _, _, body := srv.snap()
		var doc struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(body, &doc)
		if doc.Type != want {
			t.Fatalf("type %q → %s, want %s", in, doc.Type, want)
		}
	}
	if _, err := ReplayStatementCreate(context.Background(),
		"CR-Test-Customer-Edited", "17/08/2026", "18/09/2026", "bogus"); err == nil ||
		!strings.Contains(err.Error(), "--type") {
		t.Fatalf("err = %v", err)
	}
}

func TestStatementOpenItemWindow(t *testing.T) {
	srv := statementServer(t)
	if _, err := ReplayStatementCreate(context.Background(),
		"CR-Test-Customer-Edited", "17/08/2026", "18/09/2026", "open-item"); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, _, body := srv.snap()
	var doc struct {
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
	}
	_ = json.Unmarshal(body, &doc)
	// Open Item statements the trailing 365 days ending on the statement date.
	if doc.StartDate != "2025-09-18" || doc.EndDate != "2026-09-18" {
		t.Fatalf("open-item window = %s→%s, want 2025-09-18→2026-09-18", doc.StartDate, doc.EndDate)
	}
}

func TestStatementRequiresPositiveReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/v3/company/") {
			_, _ = w.Write([]byte(`{"QueryResponse":{"Customer":[{"Id":"1","DisplayName":"CR-Test-Customer-Edited"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayStatementCreate(context.Background(), "CR-Test-Customer-Edited", "17/08/2026", "18/09/2026", "transaction"); err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestStatementValidation(t *testing.T) {
	statementServer(t)
	if _, err := ReplayStatementCreate(context.Background(), "", "17/08/2026", "18/09/2026", "transaction"); err == nil ||
		!strings.Contains(err.Error(), "--customer") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayStatementCreate(context.Background(), "CR-Test-Customer-Edited", "not-a-date", "18/09/2026", "transaction"); err == nil ||
		!strings.Contains(err.Error(), "--start-date") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayStatementCreate(context.Background(), "Ghost", "17/08/2026", "18/09/2026", "transaction"); err == nil ||
		!strings.Contains(err.Error(), "customer") {
		t.Fatalf("err = %v", err)
	}
}
