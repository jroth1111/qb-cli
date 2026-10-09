package auth

import (
	"context"
	"os"
	"strings"
	"time"
)

// RefreshQuiet never opens an interactive login or follows another principal.
// For a captured Ego source, a rejected server probe may reload only the
// source-pinned existing banking tab; the user-control gate remains authoritative.
// Other sources use matching relay cookies or an existing managed profile.
// MFA/sign-in still requires the user.
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
	if err != nil || !SameBrowserSession(expected, current) {
		return ErrSessionChanged
	}
	if current.CapturedAt.After(expected.CapturedAt) {
		return nil
	}
	if current.Source == "managed-profile" {
		if !allowManaged {
			return ErrRemintNeedsLogin
		}
		return remintManagedLocked(ctx)
	}
	if current.EgoSpace != "" || current.Source == "ego-existing" {
		if allowManaged && SupportsNativeTicket(current) {
			if record, err := loadRecovery(); err == nil && record.Enabled && recoveryBound(record, current) {
				return recoverManagedLocked(ctx, current)
			}
		}
		space := current.EgoSpace
		if space == "" {
			space = "qb-gql" // legacy existing-Ego capture
		}
		ectx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cap, err := captureExistingEgo(WithExistingEgo(ectx, space, current.EgoTargetID), BankingCaptureURL, BankingCaptureURL)
		if err != nil {
			return err
		}
		return SaveRenewedCapture(current, cap, "ego-existing")
	}
	if current.Source == "chrome-existing" || current.Source == "relay-existing" || current.Source == "relay-session" {
		endpoint := current.RelayURL
		if endpoint == "" {
			endpoint = resolveRelayURL()
		}
		if override := os.Getenv("QB_RELAY_URL"); override != "" && current.RelayURL != "" && strings.TrimRight(override, "/") != strings.TrimRight(current.RelayURL, "/") {
			return ErrSessionChanged
		}
		return remintFromRelay(ctx, endpoint)
	}
	rctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	relay := resolveRelayURL()
	if current.RelayURL != "" && os.Getenv("QB_RELAY_URL") == "" {
		relay = current.RelayURL
	}
	tabs, terr := listRelayPages(rctx, relay)
	if terr == nil {
		for _, tab := range tabs {
			if !AuthenticatedURL(tab.URL) || current.CDPTargetID != "" && tab.ID != current.CDPTargetID {
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
			e = SaveRenewedCapture(current, &ATSCapture{Headers: headers, Cookies: cookies, Identity: identity}, current.Source)
			if e == nil {
				cancel()
				return nil
			}
		}
	}
	cancel()
	if _, err := os.Stat(ManagedProfileDir()); allowManaged && err == nil && os.Getenv(managedDisableEnv) != "1" {
		return remintManagedLocked(ctx)
	}
	return ErrRemintNeedsLogin
}
