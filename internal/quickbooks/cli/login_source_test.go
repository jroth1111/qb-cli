package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestCaptureEndpointValidation(t *testing.T) {
	for _, good := range []string{"http://127.0.0.1:9222", "http://[::1]:9222", "https://relay.example"} {
		if err := validateCaptureEndpoint(good); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{"", "http://remote.example", "https://user:pass@relay.example", "ws://localhost:9222", "https://relay.example?token=secret"} {
		if validateCaptureEndpoint(bad) == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestExplicitChromeRejectsIdentityDrift(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(whoamiTabs)) }))
	defer srv.Close()
	stubFetchTabIdentity(t, auth.Identity{Realm: "123", Email: "one@example.com"}, nil)
	old := captureATS
	t.Cleanup(func() { captureATS = old })
	captureATS = func(context.Context, string, string, string) (*auth.ATSCapture, error) {
		return &auth.ATSCapture{Identity: auth.Identity{Realm: "123", Email: "other@example.com"}}, nil
	}
	cmd := buildLoginCmd("login", &rootFlags{})
	cmd.SetArgs([]string{"--source", "chrome", "--cdp-url", srv.URL, "--keep-alive=false"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "principal changed") {
		t.Fatalf("got %v", err)
	}
	if _, err := auth.Load(); err == nil {
		t.Fatal("saved mismatched capture")
	}
}

func TestExplicitChromeRejectsAmbiguousTabs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"one","url":"https://qbo.intuit.com/app/banking"},{"id":"two","url":"https://qbo.intuit.com/app/banking"}]`))
	}))
	defer srv.Close()
	cmd := buildLoginCmd("login", &rootFlags{})
	cmd.SetArgs([]string{"--source", "chrome", "--cdp-url", srv.URL})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "found 2") {
		t.Fatalf("got %v", err)
	}
}
