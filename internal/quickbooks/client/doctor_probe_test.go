package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeObservesStatusWithoutRenewing(t *testing.T) {
	for _, status := range []int{200, 401, 403, 500} {
		saveUsable(t)
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"accounts":[]}`)
		}))
		interceptHTTP(t, srv.URL)
		original := remint
		remint = func(context.Context) error { t.Fatal("diagnostic must not renew credentials"); return nil }
		ok, got, _ := ProbeSession(context.Background())
		remint = original
		srv.Close()
		// A healthy first probe returns early; a failed one retries against
		// the neo fallback endpoint, so non-200 statuses make two calls.
		wantCalls := 2
		if status == 200 {
			wantCalls = 1
		}
		if got != status || ok != (status == 200) || calls != wantCalls {
			t.Fatalf("probe = %v/%d, calls=%d; expected status %d, calls %d", ok, got, calls, status, wantCalls)
		}
	}
}

func TestProbeRejectsLoginPage(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<html>login</html>") }))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	if ok, _, _ := ProbeSession(context.Background()); ok {
		t.Fatal("HTTP200 login page was treated as an authenticated session")
	}
}
