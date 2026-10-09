package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestBrowserDestinationsRejectCredentialExfiltrationAndWrongRealm(t *testing.T) {
	tok := &TokenSet{RealmID: "1", Email: "fixture@example.invalid"}
	for _, endpoint := range []string{"https://evil.invalid/graphql", "https://accounts.intuit.com/api", "https://qbo.intuit.com.evil.invalid/", "http://qbo.intuit.com/api", "https://user:private@qbo.intuit.com/api", "https://qbo.intuit.com:444/api", "https://qbo.intuit.com/api#fragment", "https://qbo.intuit.com/api/v3/company/2/query"} {
		if ValidateBrowserDestination(endpoint, tok, nil) == nil {
			t.Fatal("unsafe browser destination accepted", endpoint)
		}
	}
	for _, endpoint := range []string{"https://qbo.intuit.com/api/v4/graphql", "https://fitransactions.api.intuit.com/graphql"} {
		if err := ValidateBrowserDestination(endpoint, tok, map[string]string{"Intuit-Company-ID": "1"}); err != nil {
			t.Fatal(err)
		}
		if !errors.Is(ValidateBrowserDestination(endpoint, tok, map[string]string{"Intuit-Realm-ID": "2"}), ErrSessionChanged) {
			t.Fatal("wrong header realm accepted")
		}
	}
}

func TestBrowserSessionCannotSwitchBackendWithinLogicalLogin(t *testing.T) {
	before := &TokenSet{SessionID: "same", RealmID: "1", Email: "fixture@example.invalid", Source: "ego-existing", EgoSpace: "4", EgoTargetID: "p1"}
	for _, field := range []string{"source", "space", "target", "endpoint", "cdp"} {
		next := *before
		switch field {
		case "source":
			next.Source = "managed-profile"
		case "space":
			next.EgoSpace = "5"
		case "target":
			next.EgoTargetID = "p2"
		case "endpoint":
			next.RelayURL = "http://127.0.0.1:9222"
		case "cdp":
			next.CDPTargetID = "different"
		}
		if !SameSession(before, &next) || SameBrowserSession(before, &next) {
			t.Fatal("backend changes were not distinguished from rotation", field)
		}
	}
	rotated := *before
	rotated.CredentialGeneration++
	if !SameBrowserSession(before, &rotated) {
		t.Fatal("credential rotation rejected")
	}
}

func TestBoundHTTPRequestsCannotSendCapturedCredentialsOffSite(t *testing.T) {
	tok, _ := recoveryFixture(t)
	for _, destination := range []string{"https://evil.invalid/", "http://qbo.intuit.com/", "https://user:private@qbo.intuit.com/", "https://qbo.intuit.com:444/"} {
		request, _ := http.NewRequestWithContext(context.Background(), "GET", destination, nil)
		request.Header.Set("Authorization", tok.Authorization)
		BindRequest(request, tok, "")
		if _, err := PrepareSessionRequest(request); err == nil {
			t.Fatal("captured auth escaped its destination boundary", destination)
		}
	}
}

func TestBoundRedirectsRejectWritesAndRecoverOnlyKnownSignIn(t *testing.T) {
	tok, _ := recoveryFixture(t)
	for _, tc := range []struct {
		method, destination string
		want                error
	}{
		{"GET", "https://accounts.intuit.com/app/sign-in", ErrRemintNeedsLogin},
		{"GET", "https://qbo.intuit.com/v3/company/wrong/query", ErrSessionChanged},
		{"POST", "https://qbo.intuit.com/api/forwarded", ErrBrowserRequestUncertain},
	} {
		original, _ := http.NewRequest(tc.method, "https://qbo.intuit.com/api/fixture", nil)
		BindRequest(original, tok, "")
		redirected, _ := http.NewRequestWithContext(original.Context(), "GET", tc.destination, nil)
		if err := ValidateSessionRedirect(redirected, []*http.Request{original}); !errors.Is(err, tc.want) {
			t.Fatal("incorrect bound redirect classification", tc.method, err)
		}
	}
	original, _ := http.NewRequest("GET", "https://qbo.intuit.com/api/fixture", nil)
	BindRequest(original, tok, "")
	foreign, _ := http.NewRequestWithContext(original.Context(), "GET", "https://evil.invalid/", nil)
	if err := ValidateSessionRedirect(foreign, []*http.Request{original}); err == nil {
		t.Fatal("cross-site redirect escaped the captured session")
	}
}
