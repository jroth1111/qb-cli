package client

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// DATA_EXPORT_GET — neo POST /qbo7/neo/v1/company/{realm}/reports/exportReport
// on c7.qbo.intuit.com, captured from /app/exportdata (qbo-export-data-ui
// widget fires one POST per selected report token). Live-proven on TC2:
// PANDL → 200, 4265-byte xlsx with real Profit and Loss rows; CUST_CONTACT →
// 3981 bytes. Fresh trace ids ride freshTraceHeaders like txnsrendering.

func TestDataExportWritesXLSX(t *testing.T) {
	saveUsableURIHost(t)
	xlsx := append([]byte("PK\x03\x04"), make([]byte, 2048)...)
	srv := newDomServer(t, http.StatusOK, string(xlsx))
	interceptHTTP(t, srv.URL)
	out := filepath.Join(t.TempDir(), "pandl.xlsx")
	res, err := ReplayDataExport(context.Background(), "PANDL", out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Token != "PANDL" || res.Bytes != len(xlsx) {
		t.Fatalf("got %+v", res)
	}
	got, err := os.ReadFile(out)
	if err != nil || len(got) != len(xlsx) {
		t.Fatalf("file = %d bytes err=%v", len(got), err)
	}
	calls, meth, path, body := srv.snap()
	if calls != 1 || meth != http.MethodPost {
		t.Fatalf("call = %s x%d", meth, calls)
	}
	if !strings.Contains(path, "/qbo7/neo/v1/company/") || !strings.HasSuffix(path, "/reports/exportReport") {
		t.Fatalf("path = %s", path)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	attrs, _ := m["reportCustomizationAttributes"].(map[string]any)
	if attrs["token"] != "PANDL" || m["reportFormat"] != "xlsx" {
		t.Fatalf("body = %v", m)
	}
	if cols, _ := attrs["columns"].(string); !strings.Contains(cols, "debit_label") {
		t.Errorf("PANDL columns = %q", cols)
	}
}

func TestDataExportTokenColumns(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, "PK\x03\x04zip")
	interceptHTTP(t, srv.URL)
	out := filepath.Join(t.TempDir(), "gl.xlsx")
	if _, err := ReplayDataExport(context.Background(), "GEN_LEDGER", out, nil); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	attrs, _ := m["reportCustomizationAttributes"].(map[string]any)
	// GEN_LEDGER is the only token carrying a running-balance column.
	if cols, _ := attrs["columns"].(string); !strings.Contains(cols, "balance_label") {
		t.Errorf("GEN_LEDGER columns = %q", cols)
	}
}

func TestDataExportValidation(t *testing.T) {
	saveUsableURIHost(t)
	// Arbitrary tokens are allowed — the service validates them — but an
	// empty token and a missing output path are still caller errors.
	if _, err := ReplayDataExport(context.Background(), "", "/tmp/x.xlsx", nil); err == nil || !strings.Contains(err.Error(), "--token") {
		t.Fatalf("empty token = %v", err)
	}
	if _, err := ReplayDataExport(context.Background(), "PANDL", "", nil); err == nil || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("missing out = %v", err)
	}
	// Validation happens before any dial.
	srv := newDomServer(t, http.StatusOK, "PK")
	interceptHTTP(t, srv.URL)
	if _, err := ReplayDataExport(context.Background(), "nope", "", nil); err == nil {
		t.Fatal("expected error")
	}
	if calls, _, _, _ := srv.snap(); calls != 0 {
		t.Fatalf("validation dialed %d calls", calls)
	}
}

// TestDataExportCustomAttrs asserts the classic reportv2-style customization
// attributes (mem_rpt_id, klass, low_date, ...) merge into the POST body —
// this is the path that exports a saved custom report like the owner
// statements without hardcoding a per-token column map.
func TestDataExportCustomAttrs(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, "PK\x03\x04zip")
	interceptHTTP(t, srv.URL)
	out := filepath.Join(t.TempDir(), "stmt.xlsx")
	res, err := ReplayDataExport(context.Background(), "TX_DET_BY_ACCT", out, map[string]string{
		"mem_rpt_id": "42", "low_date": "01/04/2026", "high_date": "30/04/2026",
		"klass": "3700000000000906240", "cash_basis": "yes", "customized": "yes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Note, "custom token") {
		t.Errorf("unknown-token note = %q", res.Note)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	attrs, _ := m["reportCustomizationAttributes"].(map[string]any)
	if attrs["token"] != "TX_DET_BY_ACCT" || attrs["mem_rpt_id"] != "42" || attrs["klass"] != "3700000000000906240" || attrs["low_date"] != "01/04/2026" {
		t.Fatalf("attrs = %v", attrs)
	}
}

func TestDataExportHTTPError(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusForbidden, `{"error":"NOT_PERMITTED"}`)
	interceptHTTP(t, srv.URL)
	out := filepath.Join(t.TempDir(), "x.xlsx")
	if _, err := ReplayDataExport(context.Background(), "PANDL", out, nil); err == nil {
		t.Fatal("403 should error")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Error("partial file left behind")
	}
}

func TestPlannedDataExportURL(t *testing.T) {
	u := PlannedDataExportURL()
	if !strings.Contains(u, "c7.qbo.intuit.com") || !strings.Contains(u, "exportReport") {
		t.Fatalf("plan = %s", u)
	}
	if strings.Contains(u, "934145") {
		t.Error("plan URL must not embed the live realm")
	}
}
