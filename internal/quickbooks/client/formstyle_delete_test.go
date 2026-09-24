package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Form-style delete rides txnsrendering GET + DELETE on /v2/customizations/{id}
// with ?templateEditSequence={seq}&forceDelete=true. The service rejects
// replayed tracing ids (jar-captured intuit_tid/x-b3 are consumed nonces →
// generic TRS_V2_INTERNAL_DB_ERROR), so the delete mints fresh b3/tid headers.

type fsDeleteProbe struct {
	*domServer
	lastQuery   string
	lastHeaders http.Header
}

func formStyleDeleteServer(t *testing.T, status int, body string) *fsDeleteProbe {
	t.Helper()
	saveUsableURIHost(t)
	s := &fsDeleteProbe{domServer: &domServer{t: t}}
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
		if r.Method == http.MethodDelete {
			s.lastQuery = r.URL.RawQuery
			s.lastHeaders = r.Header.Clone()
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.URL.Path, "/v2/customizations/") {
			_, _ = w.Write([]byte(`{"error":"unmatched"}`))
			return
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(formStyleDoc))
		case http.MethodDelete:
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		default:
			_, _ = w.Write([]byte(`{"error":"method"}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestFormStyleDeleteSendsSeqAndForceDelete(t *testing.T) {
	srv := formStyleDeleteServer(t, http.StatusOK, `{"status":"Deleted Successfully","id":"1125188"}`)
	res, err := ReplayFormStyleDelete(context.Background(), "1125188")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Op != "form-style delete" || res.Item.ID != "1125188" || res.Item.Name != "Standard" {
		t.Fatalf("result = %+v", res)
	}
	calls, meth, path, _ := srv.snap()
	if calls != 2 || meth != http.MethodDelete {
		t.Fatalf("calls=%d meth=%s", calls, meth)
	}
	if !strings.Contains(path, "/v2/customizations/1125188") {
		t.Fatalf("path = %s", path)
	}
	if !strings.Contains(srv.lastQuery, "templateEditSequence=7") {
		t.Fatalf("query missing doc seq (GET answered 7): %s", srv.lastQuery)
	}
	if !strings.Contains(srv.lastQuery, "forceDelete=true") {
		t.Fatalf("query missing forceDelete: %s", srv.lastQuery)
	}
}

func TestFormStyleDeleteMintsFreshTraceIDs(t *testing.T) {
	srv := formStyleDeleteServer(t, http.StatusOK, `{"status":"Deleted Successfully","id":"1125188"}`)
	if _, err := ReplayFormStyleDelete(context.Background(), "1125188"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	tid := srv.lastHeaders.Get("intuit_tid")
	trace := srv.lastHeaders.Get("x-b3-traceid")
	span := srv.lastHeaders.Get("x-b3-spanid")
	if tid == "" || trace == "" || span == "" {
		t.Fatalf("missing trace headers: tid=%q trace=%q span=%q", tid, trace, span)
	}
	if trace != strings.ReplaceAll(tid, "-", "") {
		t.Fatalf("traceid should be tid without dashes: tid=%q trace=%q", tid, trace)
	}
	if len(span) != 16 {
		t.Fatalf("spanid = %q, want 16 hex chars", span)
	}
	if tid == "0c86e185-723e-43b0-9ba1-faf5fcb668f0" {
		t.Fatal("replayed jar-captured intuit_tid")
	}
}

func TestFormStyleDeleteEmpty204IsUnverified(t *testing.T) {
	formStyleDeleteServer(t, http.StatusNoContent, ``)
	res, err := ReplayFormStyleDelete(context.Background(), "1125188")
	if err == nil || res != nil {
		t.Fatalf("empty 204 claimed a verified deletion: res=%+v err=%v", res, err)
	}
}

func TestFormStyleDeleteValidation(t *testing.T) {
	if _, err := ReplayFormStyleDelete(context.Background(), "  "); err == nil ||
		!strings.Contains(err.Error(), "--template-id") {
		t.Fatalf("err = %v", err)
	}
}

func TestFormStyleDeleteGetError(t *testing.T) {
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		s.lastMeth = r.Method
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	if _, err := ReplayFormStyleDelete(context.Background(), "999"); err == nil ||
		!strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v", err)
	}
	calls, meth, _, _ := s.snap()
	if calls != 1 || meth != http.MethodGet {
		t.Fatalf("delete must not fire when the read fails: calls=%d meth=%s", calls, meth)
	}
}

func TestFormStyleDeleteHTTPError(t *testing.T) {
	formStyleDeleteServer(t, http.StatusInternalServerError,
		`{"error":"Failed to save customization template, unexpected failure"}`)
	if _, err := ReplayFormStyleDelete(context.Background(), "1125188"); err == nil ||
		!strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v", err)
	}
}
