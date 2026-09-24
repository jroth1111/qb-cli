package auth

import (
	"context"
	"os"
	"strings"
	"time"
)

// RefreshQuiet never navigates an existing user tab or opens an interactive
// login. First read cookies from a matching live tab, then try an already-
// existing managed profile headlessly. MFA/sign-in still requires the user.
func RefreshQuiet(ctx context.Context, expected *TokenSet, allowManaged bool) error {
	if IsHarness() {
		return ErrRemintNeedsLogin
	}
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return err
	}
	defer unlock()
	current, err := Load()
	if err != nil || !SameSession(expected, current) {
		return ErrSessionChanged
	}
	if current.CapturedAt.After(expected.CapturedAt) {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	relay := resolveRelayURL()
	if current.RelayURL != "" && os.Getenv("QB_RELAY_URL") == "" {
		relay = current.RelayURL
	}
	tabs, terr := listRelayPages(rctx, relay)
	if terr == nil {
		for _, tab := range tabs {
			if !AuthenticatedURL(tab.URL) {
				continue
			}
			identity, e := FetchTabIdentity(rctx, relay, tab.ID)
			if e != nil || identity.Realm != current.RealmID || current.Email != "" && !strings.EqualFold(identity.Email, current.Email) {
				continue
			}
			cookies, e := FetchTabCookies(rctx, relay, tab.ID)
			if e != nil || len(cookies) == 0 {
				continue
			}
			headers := current.RequestHeaders
			if !isIntuitAPIKey(headerGet(headers, "authorization")) {
				headers = map[string]string{"Authorization": current.Authorization, "intuit-company-id": current.RealmID}
			}
			e = SaveRenewedCapture(current, &ATSCapture{Headers: headers, Cookies: cookies, Identity: identity}, "relay-cookie-refresh")
			if e == nil {
				cancel()
				return nil
			}
		}
	}
	cancel()
	if _, err := os.Stat(ManagedProfileDir()); allowManaged && err == nil && os.Getenv(managedDisableEnv) != "1" {
		return RemintManaged(ctx)
	}
	return ErrRemintNeedsLogin
}
