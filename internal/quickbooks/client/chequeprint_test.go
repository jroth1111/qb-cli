package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// saveUsableTC2 writes a usable TokenSet with the Test Company 2 realm so the
// captured v4 namespace constant applies to plain ids.
func saveUsableTC2(t *testing.T) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       v4TxNSRealm,
	}); err != nil {
		t.Fatal(err)
	}
	orig := uriHostTransport
	uriHostTransport = func(req *http.Request) (*http.Response, error) {
		return http.DefaultTransport.RoundTrip(req)
	}
	t.Cleanup(func() { uriHostTransport = orig })
}

// chequePrintServer answers the v4 transaction .pdf GET.
func chequePrintServer(t *testing.T, body []byte, status int) *domServer {
	t.Helper()
	saveUsableTC2(t)
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
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestChequeExportWireShape(t *testing.T) {
	pdf := []byte("%PDF-1.4 fake cheque body")
	srv := chequePrintServer(t, pdf, http.StatusOK)
	res, err := ReplayChequeExport(context.Background(), "159", "")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if res.Op != "cheque export" || res.Item.ID != "159" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, _ := srv.snap()
	if meth != http.MethodGet {
		t.Fatalf("method = %s", meth)
	}
	if !strings.Contains(pathq, "/api/v4/companies/") || !strings.Contains(pathq, ".pdf") {
		t.Fatalf("path = %s", pathq)
	}
	// plain id must be namespaced with the captured v4 transaction prefix
	if !strings.Contains(pathq, v4TxNSTestCompany2+":159.pdf") {
		t.Fatalf("missing namespace in %s", pathq)
	}
	if !strings.Contains(pathq, "intuit_apikey=") || !strings.Contains(pathq, "intuit-company-id=") {
		t.Fatalf("missing query params in %s", pathq)
	}
}

func TestChequeExportNamespacedID(t *testing.T) {
	srv := chequePrintServer(t, []byte("%PDF-1.4 x"), http.StatusOK)
	if _, err := ReplayChequeExport(context.Background(), "djQuCustom:42", ""); err != nil {
		t.Fatalf("export: %v", err)
	}
	_, _, pathq, _ := srv.snap()
	if !strings.Contains(pathq, "djQuCustom:42.pdf") {
		t.Fatalf("namespaced id not used verbatim: %s", pathq)
	}
}

func TestChequeExportWritesFile(t *testing.T) {
	pdf := []byte("%PDF-1.4 fake")
	chequePrintServer(t, pdf, http.StatusOK)
	out := filepath.Join(t.TempDir(), "cheque.pdf")
	res, err := ReplayChequeExport(context.Background(), "159", out)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != string(pdf) {
		t.Fatalf("written file = %v %q", err, got)
	}
	if !strings.Contains(res.Note, out) {
		t.Fatalf("note = %s", res.Note)
	}
}

func TestChequeExportValidation(t *testing.T) {
	chequePrintServer(t, []byte("%PDF-1.4 x"), http.StatusOK)
	if _, err := ReplayChequeExport(context.Background(), "", ""); err == nil ||
		!strings.Contains(err.Error(), "--id") {
		t.Fatalf("err = %v", err)
	}
}

func TestChequeExportRejectsNonPDF(t *testing.T) {
	chequePrintServer(t, []byte(`{"fault":"nope"}`), http.StatusOK)
	if _, err := ReplayChequeExport(context.Background(), "159", ""); err == nil ||
		!strings.Contains(err.Error(), "expected a PDF") {
		t.Fatalf("err = %v", err)
	}
}
