package client

import (
	"context"
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"net/http"
)

var refreshQuiet = auth.RefreshQuiet

func maintenanceProbe(ctx context.Context, ac *apiClient) (bool, int, bool) {
	rejected := true
	status := 0
	for _, endpoint := range []string{ac.neoFeedURL() + "/getInitialData", ac.baseURL() + "/getInitialData"} {
		ok, code, _ := probeGetInitialData(ctx, ac, endpoint)
		status = code
		if ok {
			return true, code, false
		}
		if code == 0 {
			return false, code, false
		}
		if code != http.StatusUnauthorized {
			rejected = false
		}
	}
	return false, status, rejected
}

// MaintainSession performs only banking GETs. Renewal is attempted only after
// both channels explicitly reject authentication, not on network/contract
// errors or a permission denial. A write is never used as a heartbeat.
func MaintainSession(ctx context.Context, expected *auth.TokenSet, allowManaged bool) (bool, int, string) {
	ac, err := newAPIClient()
	if err != nil {
		return false, 0, "credentials_unavailable"
	}
	if !auth.SameSession(expected, ac.tok) {
		return false, 0, "session_changed"
	}
	ac.skipRemint = true
	ok, status, rejected := maintenanceProbe(ctx, ac)
	if ok {
		return true, status, "live"
	}
	if !rejected {
		return false, status, "temporarily_unavailable"
	}
	if err = refreshQuiet(ctx, ac.tok, allowManaged); err != nil {
		if errors.Is(err, auth.ErrSessionChanged) {
			return false, 0, "session_changed"
		}
		if errors.Is(err, auth.ErrRemintNeedsLogin) || errors.Is(err, auth.ErrNoATSAuthorization) || errors.Is(err, auth.ErrSessionIdentityUnverified) {
			return false, status, "needs_login"
		}
		return false, status, "temporarily_unavailable"
	}
	fresh, err := newAPIClient()
	if err != nil || !auth.SameSession(expected, fresh.tok) {
		return false, 0, "session_changed"
	}
	fresh.skipRemint = true
	ok, status, rejected = maintenanceProbe(ctx, fresh)
	if ok {
		return true, status, "live"
	}
	if rejected {
		return false, status, "needs_login"
	}
	return false, status, "temporarily_unavailable"
}
