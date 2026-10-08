package client

import (
	"context"
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"net/http"
)

var refreshQuiet = auth.RefreshQuiet
var extendNativeTicket = auth.ExtendNativeTicket

func maintainNativeTicket(ctx context.Context, tok *auth.TokenSet, allowManaged bool) error {
	if !allowManaged || !auth.SupportsNativeTicket(tok) {
		return nil
	}
	return extendNativeTicket(ctx, tok, false)
}

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

// MaintainSession uses banking GETs plus the native sessionextender read when
// due. Credential recovery is attempted only after
// both channels explicitly reject authentication, not on network/contract
// errors or a permission denial. Financial writes are never heartbeats; the
// native authorization POST is the site's sessionextender permission read.
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
		if err = maintainNativeTicket(ctx, ac.tok, allowManaged); err != nil {
			return false, status, nativeMaintenanceFailure(err)
		}
		return true, status, "live"
	}
	if !rejected {
		return false, status, "temporarily_unavailable"
	}
	if err = refreshQuiet(ctx, ac.tok, allowManaged); err != nil {
		if errors.Is(err, auth.ErrSessionChanged) {
			return false, 0, "session_changed"
		}
		if errors.Is(err, auth.ErrEgoUserControl) {
			return false, status, "user_control"
		}
		if errors.Is(err, auth.ErrRecoveryAttention) || errors.Is(err, auth.ErrRecoveryKeyUnavailable) {
			return false, status, "needs_attention"
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
		if err = maintainNativeTicket(ctx, fresh.tok, allowManaged); err != nil {
			return false, status, nativeMaintenanceFailure(err)
		}
		return true, status, "live"
	}
	if rejected {
		return false, status, "needs_login"
	}
	return false, status, "temporarily_unavailable"
}

func nativeMaintenanceFailure(err error) string {
	switch {
	case errors.Is(err, auth.ErrSessionChanged):
		return "session_changed"
	case errors.Is(err, auth.ErrEgoUserControl):
		return "user_control"
	case errors.Is(err, auth.ErrRemintNeedsLogin):
		return "needs_login"
	case errors.Is(err, auth.ErrRecoveryAttention):
		return "needs_attention"
	default:
		return "native_extension_pending"
	}
}
