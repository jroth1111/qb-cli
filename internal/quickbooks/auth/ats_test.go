package auth

import (
	"errors"
	"testing"
)

func TestServiceHostCapturesSeparatelyKeyedNeoRequests(t *testing.T) {
	authz := "Intuit_APIKey intuit_apikey=neo,intuit_apikey_version=1.0"
	tests := []struct {
		url     string
		headers map[string]string
		want    string
	}{
		{"https://qbo.intuit.com/api/neo/v1/company/1/lists/olbrules/getRules", map[string]string{"Authorization": authz, "apikey": "neo-key"}, "qbo.intuit.com"},
		{"https://qbo.intuit.com/api/neo/v1/company/1/olb/ng/getInitialData", map[string]string{"Authorization": authz}, ""},
		{"https://qbo.intuit.com/ats/v1/company/1/banking", map[string]string{"Authorization": authz, "apikey": "neo-key"}, ""},
		{"https://audit.api.intuit.com/v1/query", map[string]string{"Authorization": authz}, "audit.api.intuit.com"},
	}
	for _, tc := range tests {
		if got := ServiceHost(tc.url, tc.headers); got != tc.want {
			t.Errorf("ServiceHost(%q)=%q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestApplyATSCaptureSetsDistinctHeaders(t *testing.T) {
	tok := &TokenSet{
		Cookies: []Cookie{{Name: "qbn.ticket", Value: "t"}},
	}
	err := tok.ApplyATSCapture(&ATSCapture{
		Headers: map[string]string{
			"Authorization":     "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			"authtype":          "browser_auth",
			"apikey":            "k",
			"intuit_appid":      "app",
			"csrftoken":         "short",
			"x-csrf-token":      "long-distinct",
			"Cookie":            "should-not-store",
			"intuit-company-id": "99999",
		},
		Cookies: []Cookie{{Name: "qbo.ticket", Value: "q"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tok.HasATSAuthorization() {
		t.Fatal("expected ATS authorization")
	}
	if tok.TokenType != "browser_auth" || tok.APIKey != "k" || tok.IntuitAppID != "app" {
		t.Fatalf("typed fields: type=%q key=%q app=%q", tok.TokenType, tok.APIKey, tok.IntuitAppID)
	}
	if tok.RealmID != "99999" {
		t.Fatalf("realm %q", tok.RealmID)
	}
	if headerGet(tok.RequestHeaders, "cookie") != "" {
		t.Fatal("Cookie header must not be stored in request_headers")
	}
	if headerGet(tok.RequestHeaders, "x-csrf-token") == headerGet(tok.RequestHeaders, "csrftoken") {
		t.Fatal("x-csrf-token must stay distinct")
	}
	if len(tok.Cookies) != 2 {
		t.Fatalf("cookies merged: %d", len(tok.Cookies))
	}
}

func TestApplyATSCaptureRejectsCookieOnly(t *testing.T) {
	tok := &TokenSet{Cookies: []Cookie{{Name: "qbn.ticket", Value: "t"}}}
	if err := tok.ApplyATSCapture(&ATSCapture{Headers: map[string]string{"Accept": "*/*"}}); !errors.Is(err, ErrNoATSAuthorization) {
		t.Fatalf("got %v, want ErrNoATSAuthorization", err)
	}
	if err := tok.ApplyATSCapture(nil); !errors.Is(err, ErrNoATSAuthorization) {
		t.Fatalf("nil capture: %v", err)
	}
}

func TestHasATSAuthorizationNegative(t *testing.T) {
	if (&TokenSet{Cookies: []Cookie{{Name: "qbn.ticket", Value: "t"}}}).HasATSAuthorization() {
		t.Fatal("ticket cookie is not ATS authorization")
	}
	if (&TokenSet{}).HasATSAuthorization() {
		t.Fatal("empty set")
	}
}
