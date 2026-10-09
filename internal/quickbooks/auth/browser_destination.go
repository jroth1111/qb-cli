package auth

import (
	"errors"
	"net/url"
	"strings"
)

// Browser credentials must not accompany an arbitrary endpoint or a different
// company, even when the destination shares Intuit's parent domain.
func ValidateBrowserDestination(endpoint string, tok *TokenSet, headers map[string]string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || (u.Hostname() != "qbo.intuit.com" && !strings.HasSuffix(u.Hostname(), ".api.intuit.com")) {
		return errors.New("browser fetch requires an HTTPS QBO API endpoint")
	}
	if tok == nil || tok.RealmID == "" || tok.Email == "" {
		return ErrSessionIdentityUnverified
	}
	for key, value := range headers {
		if (strings.EqualFold(key, "intuit-company-id") || strings.EqualFold(key, "intuit-realm-id")) && value != "" && value != tok.RealmID {
			return ErrSessionChanged
		}
	}
	return validateCompanyPath(u, tok.RealmID)
}

func validateCompanyPath(u *url.URL, expectedRealm string) error {
	for _, prefix := range []string{"/api/v3/company/", "/api/neo/v1/company/", "/ats/v1/company/", "/v3/company/"} {
		if rest, ok := strings.CutPrefix(u.Path, prefix); ok {
			realm, _, _ := strings.Cut(rest, "/")
			if realm != expectedRealm {
				return ErrSessionChanged
			}
		}
	}
	return nil
}
