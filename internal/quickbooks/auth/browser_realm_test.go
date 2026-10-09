package auth

import (
	"errors"
	"net/http"
	"testing"
)

func TestBrowserDestinationPinsCompanyOnEveryAPIHost(t *testing.T) {
	tok := &TokenSet{RealmID: "12", Email: "owner@example.invalid"}
	for _, host := range []string{"qbo.intuit.com", "quickbooks.api.intuit.com"} {
		for _, prefix := range []string{"/api/v3/company/", "/api/neo/v1/company/", "/ats/v1/company/", "/v3/company/"} {
			t.Run(host+prefix, func(t *testing.T) {
				if err := ValidateBrowserDestination("https://"+host+prefix+"12/query", tok, nil); err != nil {
					t.Fatal(err)
				}
				for _, realm := range []string{"13", "%31%33", "", ".."} {
					if err := ValidateBrowserDestination("https://"+host+prefix+realm+"/query", tok, nil); !errors.Is(err, ErrSessionChanged) {
						t.Fatalf("different company accepted: %q, err=%v", realm, err)
					}
				}
			})
		}
	}
}

func TestDirectHTTPRequestsPinURLCompanyBeforeDispatch(t *testing.T) {
	tok, _ := recoveryFixture(t)
	for _, host := range []string{"qbo.intuit.com", "quickbooks.api.intuit.com"} {
		request, _ := http.NewRequest("GET", "https://"+host+"/v3/company/another-company/query", nil)
		BindRequest(request, tok, "")
		if _, err := PrepareSessionRequest(request); !errors.Is(err, ErrSessionChanged) {
			t.Fatal("cross-company HTTP request accepted", err)
		}
	}
}
