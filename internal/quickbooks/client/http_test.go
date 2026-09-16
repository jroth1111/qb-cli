package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestApplyHeadersUsesAllCapturedTokens(t *testing.T) {
	c := &apiClient{
		tok: &auth.TokenSet{
			RequestHeaders: map[string]string{
				"Authorization":               "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
				"authtype":                    "browser_auth",
				"apikey":                      "apikey-value",
				"csrftoken":                   "csrf-short",
				"x-csrf-token":                "xcsrf-long-different",
				"User-Agent":                  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/151.0.0.0",
				"Accept":                      "*/*",
				"Accept-Encoding":             "gzip, deflate, br, zstd",
				"Referer":                     "https://qbo.intuit.com/app/banking?locale=en-gb",
				"intuit_tid":                  "tid-uuid",
				"access-control-allow-origin": "*",
				"sec-ch-ua-platform":          `"macOS"`,
				"x-range":                     "items=0-299",
			},
		},
		cookies: "qbn.ticket=t; AMCV=adobe",
	}
	req, err := http.NewRequest(http.MethodGet, "https://qbo.intuit.com/ats/v1/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.applyHeaders(req, "items=0-19")

	if got, want := req.Header.Get("x-csrf-token"), "xcsrf-long-different"; got != want {
		t.Fatalf("x-csrf-token = %q, want distinct captured value", got)
	}
	if req.Header.Get("x-csrf-token") == req.Header.Get("csrftoken") {
		t.Fatal("x-csrf-token must stay distinct from csrftoken")
	}
	if got := req.Header.Get("Accept"); got != "*/*" {
		t.Fatalf("Accept = %q, want */*", got)
	}
	if got := req.Header.Get("User-Agent"); got == "" {
		t.Fatal("User-Agent missing")
	}
	if got := req.Header.Get("intuit_tid"); got != "tid-uuid" {
		t.Fatalf("intuit_tid = %q", got)
	}
	if got := req.Header.Get("Cookie"); got != "qbn.ticket=t; AMCV=adobe" {
		t.Fatalf("Cookie jar not applied: %q", got)
	}
	if got := req.Header.Get("x-range"); got != "items=0-19" {
		t.Fatalf("x-range overlay = %q, want items=0-19", got)
	}
	if got := req.Header.Get("Accept-Encoding"); got != "" {
		t.Fatalf("Accept-Encoding must not be replayed, got %q", got)
	}

}

func TestApplyHeadersFallbackDuplicatesCSRF(t *testing.T) {
	c := &apiClient{
		tok: &auth.TokenSet{
			Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			TokenType:     "browser_auth",
		},
		csrf:    "same-csrf",
		cookies: "qbo.csrftoken=same-csrf",
	}
	req, err := http.NewRequest(http.MethodGet, "https://qbo.intuit.com/ats/v1/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.applyHeaders(req, "")
	if req.Header.Get("csrftoken") != "same-csrf" || req.Header.Get("x-csrf-token") != "same-csrf" {
		t.Fatalf("fallback should copy csrf to both headers")
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Fatalf("fallback Accept = %q", req.Header.Get("Accept"))
	}
}

func TestPostJSONDropsCapturedPaginationRange(t *testing.T) {
	var gotRange, gotContentType string
	var gotTrace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("X-Range")
		gotContentType = r.Header.Get("Content-Type")
		gotTrace = r.Header.Get("X-B3-TraceId")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	original := impersonatedTransport
	impersonatedTransport = http.DefaultTransport
	t.Cleanup(func() { impersonatedTransport = original })
	c := &apiClient{tok: &auth.TokenSet{RequestHeaders: map[string]string{
		"Authorization": "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		"x-range":       "items=0-49",
		"x-b3-sampled":  "1",
		"x-b3-spanid":   "stale-span",
		"x-b3-traceid":  "stale-trace",
	}}}
	resp, err := c.postJSON(context.Background(), srv.URL, []byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = drainAndClose(resp)
	if gotRange != "" {
		t.Fatalf("mutation replay sent captured pagination range %q", gotRange)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type=%q, want application/json", gotContentType)
	}
	if gotTrace != "" {
		t.Fatalf("mutation replay sent stale B3 trace header %q", gotTrace)
	}
}

func TestGetRemintsOnceOn401(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	usable := &auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       "1",
	}
	if err := auth.Save(usable); err != nil {
		t.Fatal(err)
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"expired"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": []any{}})
	}))
	t.Cleanup(srv.Close)

	calls := 0
	orig := remint
	remint = func(context.Context) error {
		calls++
		return nil
	}
	t.Cleanup(func() { remint = orig })

	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.get(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if hits != 2 || calls != 1 {
		t.Fatalf("hits=%d remint=%d", hits, calls)
	}
}

func TestGetRemintFailureSurfaces(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0", RealmID: "1"}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	orig := remint
	remint = func(context.Context) error { return errors.New("ego cold") }
	t.Cleanup(func() { remint = orig })
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.get(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatal("expected remint failure")
	}
}

func TestNewAPIClientRejectsCookieOnly(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	// Bypass Save (which now rejects cookies-only) and write a stale file.
	raw := []byte(`{"version":1,"cookies":[{"name":"qbn.ticket","value":"t"}]}`)
	if err := os.MkdirAll(auth.HomeDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auth.CredPath(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := newAPIClient()
	if err == nil {
		t.Fatal("cookie-only session must fail before network")
	}
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}
