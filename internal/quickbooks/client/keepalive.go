package client

import (
	"context"
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"net/http"
)

var refreshQuiet = auth.RefreshQuiet
var extendNativeTicket = auth.ExtendNativeTicket
var maintenanceBankingProbe = probeInitialData

func maintainNativeTicket(ctx context.Context, tok *auth.TokenSet, allowManaged bool) error {
	if !allowManaged || !auth.SupportsNativeTicket(tok) {
		return nil
	}
	return extendNativeTicket(ctx, tok, false)
}

func maintenanceProbe(ctx context.Context, ac *apiClient) (bool, int, bool, error) {
	rejected := true
	status := 0
	for _, endpoint := range []string{ac.neoFeedURL() + "/getInitialData", ac.baseURL() + "/getInitialData"} {
		ok, code, _, err := maintenanceBankingProbe(ctx, ac, endpoint)
		status = code
		if ok {
			return true, code, false, nil
		}
		if errors.Is(err, auth.ErrRemintNeedsLogin) {
			observeMaintenance(ctx, MaintenanceBrowserSignedOut, code)
			return false, code, true, nil
		}
		if code == 0 {
			return false, code, false, err
		}
		if code != http.StatusUnauthorized {
			rejected = false
		}
	}
	return false, status, rejected, nil
}

// MaintainSession uses banking GETs plus the native sessionextender read when
// due. Recovery requires explicit banking authentication rejection or a
// confirmed sign-in wall in the owned native browser, never a network/contract
// error or permission denial. Financial writes are never heartbeats; the
// native authorization POST is the site's sessionextender permission read.
func MaintainSession(ctx context.Context, expected *auth.TokenSet, allowManaged bool) (bool, int, string) {
	ac, err := newAPIClient()
	if err != nil {
		return false, 0, "credentials_unavailable"
	}
	if !auth.SameBrowserSession(expected, ac.tok) {
		return false, 0, "session_changed"
	}
	ac.skipRemint = true
	ok, status, rejected, probeErr := maintenanceProbe(ctx, ac)
	if probeErr != nil {
		return false, status, bankingMaintenanceFailure(probeErr)
	}
	if ok {
		observeMaintenance(ctx, MaintenanceBankingLive, status)
		if err = maintainNativeTicket(ctx, ac.tok, allowManaged); err == nil {
			if allowManaged && auth.SupportsNativeTicket(ac.tok) {
				observeMaintenance(ctx, MaintenanceNativeVerified, status)
			}
			return true, status, "live"
		}
		// Browser SSO can expire while the exported API credentials remain
		// usable. The same source-pinned recovery must heal that sign-in wall.
		if !errors.Is(err, auth.ErrRemintNeedsLogin) {
			return false, status, nativeMaintenanceFailure(err)
		}
		observeMaintenance(ctx, MaintenanceBrowserSignedOut, status)
	}
	if !ok && !rejected {
		return false, status, "temporarily_unavailable"
	}
	observeMaintenance(ctx, MaintenanceRecovering, status)
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
	if err != nil || !auth.SameBrowserSession(expected, fresh.tok) {
		return false, 0, "session_changed"
	}
	fresh.skipRemint = true
	ok, status, rejected, probeErr = maintenanceProbe(ctx, fresh)
	if probeErr != nil {
		return false, status, bankingMaintenanceFailure(probeErr)
	}
	if ok {
		observeMaintenance(ctx, MaintenanceBankingReverified, status)
		if err = maintainNativeTicket(ctx, fresh.tok, allowManaged); err != nil {
			return false, status, nativeMaintenanceFailure(err)
		}
		if allowManaged && auth.SupportsNativeTicket(fresh.tok) {
			observeMaintenance(ctx, MaintenanceNativeVerified, status)
		}
		return true, status, "live"
	}
	if rejected {
		return false, status, "needs_login"
	}
	return false, status, "temporarily_unavailable"
}

func bankingMaintenanceFailure(err error) string {
	if errors.Is(err, auth.ErrSessionChanged) || errors.Is(err, auth.ErrEgoUserControl) || errors.Is(err, auth.ErrRecoveryAttention) || errors.Is(err, auth.ErrRecoveryKeyUnavailable) {
		return nativeMaintenanceFailure(err)
	}
	return "temporarily_unavailable"
}

func nativeMaintenanceFailure(err error) string {
	switch {
	case errors.Is(err, auth.ErrSessionChanged):
		return "session_changed"
	case errors.Is(err, auth.ErrEgoUserControl):
		return "user_control"
	case errors.Is(err, auth.ErrRemintNeedsLogin):
		return "needs_login"
	case errors.Is(err, auth.ErrRecoveryAttention), errors.Is(err, auth.ErrRecoveryKeyUnavailable):
		return "needs_attention"
	default:
		return "native_extension_pending"
	}
}
