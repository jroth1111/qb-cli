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
)

// The receipt-upload contract posts multipart/form-data to
// financialdocument.platform.intuit.com/v2/documents?autoClassification=true
// with a `document` JSON blob (metadata) + `image` file field.
func receiptUploadServer(t *testing.T) *domServer {
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
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"doc-1","status":"ACCEPTED"}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func writeReceiptFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReceiptUploadWireShape(t *testing.T) {
	srv := receiptUploadServer(t)
	p := writeReceiptFile(t, "receipt.pdf", []byte("%PDF-1.4 fake"))
	res, err := ReplayReceiptUpload(context.Background(), p)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if res.Op != "receipt import" || res.Item.Name != "receipt.pdf" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost {
		t.Fatalf("method = %s", meth)
	}
	if !strings.Contains(pathq, "/v2/documents") || !strings.Contains(pathq, "autoClassification=true") {
		t.Fatalf("path = %s", pathq)
	}
	s := string(body)
	if !strings.Contains(s, `name="document"`) || !strings.Contains(s, `name="image"`) {
		t.Fatalf("missing multipart fields: %.200s", s)
	}
	if !strings.Contains(s, `"documentType":"receipt"`) || !strings.Contains(s, `"name":"subsource","value":"FILE_UPLOAD"`) {
		t.Fatalf("document meta wrong: %.600s", s)
	}
	if !strings.Contains(s, `"name":"receipt.pdf"`) || !strings.Contains(s, `"is7216":false`) {
		t.Fatalf("document meta missing name/is7216: %.600s", s)
	}
	if !strings.Contains(s, `filename="receipt.pdf"`) || !strings.Contains(s, "application/pdf") {
		t.Fatalf("image part wrong: %.400s", s)
	}
	if !strings.Contains(s, "WebKitFormBoundary") {
		t.Fatalf("boundary shape wrong: %.80s", s)
	}
}

func TestReceiptUploadBatch(t *testing.T) {
	srv := receiptUploadServer(t)
	a := writeReceiptFile(t, "a.pdf", []byte("x"))
	b := writeReceiptFile(t, "b.png", []byte("y"))
	res, err := ReplayReceiptUpload(context.Background(), a+","+b)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if res.Item.Name != "2 files" {
		t.Fatalf("result = %+v", res)
	}
	if calls := srv.calls; calls != 2 {
		t.Fatalf("calls = %d, want 2 (one POST per file)", calls)
	}
}

func TestReceiptUploadValidation(t *testing.T) {
	receiptUploadServer(t)
	if _, err := ReplayReceiptUpload(context.Background(), ""); err == nil {
		t.Fatal("expected error for missing --files")
	}
	if _, err := ReplayReceiptUpload(context.Background(), "/nonexistent.pdf"); err == nil {
		t.Fatal("expected error for missing file")
	}
	empty := writeReceiptFile(t, "empty.pdf", nil)
	if _, err := ReplayReceiptUpload(context.Background(), empty); err == nil ||
		!strings.Contains(err.Error(), "is empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestReceiptUploadRequiresAcceptedDocumentReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	p := writeReceiptFile(t, "no-receipt.pdf", []byte("x"))
	if _, err := ReplayReceiptUpload(context.Background(), p); err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestReceiptUploadPartialBatchNamesCreatedDocumentBeforeRetry(t *testing.T) {
	saveUsableURIHost(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if calls == 1 {
			_, _ = w.Write([]byte(`{"id":"doc-first","status":"ACCEPTED"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	a := writeReceiptFile(t, "first.pdf", []byte("x"))
	b := writeReceiptFile(t, "second.pdf", []byte("y"))
	if _, err := ReplayReceiptUpload(context.Background(), a+","+b); err == nil || !strings.Contains(err.Error(), "doc-first") || !strings.Contains(err.Error(), "1 receipt") || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}
