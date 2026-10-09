package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// ProbeSession performs one live getInitialData request using the saved
// credentials and reports whether the session is accepted. Status codes:
// 200 → healthy; 401/403 → stale/insufficient credentials; anything else
// is reported verbatim for diagnosis.
func ProbeSession(ctx context.Context) (ok bool, httpStatus int, detail string) {
	ac, err := newAPIClient()
	if err != nil {
		return false, 0, "no usable credentials: " + err.Error()
	}
	// A diagnostic observes the saved session, without renewing it or opening
	// another browser when the server reports expired credentials. Mirrors
	// ReplayAccounts: the legacy ATS path can reject a session the neo feed
	// endpoint still accepts, so the probe tries both before declaring stale.
	ac.skipRemint = true
	ok, status, detail := probeGetInitialData(ctx, ac, ac.baseURL()+"/getInitialData")
	if ok || status == 0 {
		return ok, status, detail
	}
	ok, status, detail = probeGetInitialData(ctx, ac, ac.neoFeedURL()+"/getInitialData")
	if ok {
		return true, status, detail + " (neo fallback)"
	}
	return false, status, detail
}

func probeGetInitialData(ctx context.Context, ac *apiClient, url string) (bool, int, string) {
	ok, code, detail, _ := probeInitialData(ctx, ac, url)
	return ok, code, detail
}

func probeInitialData(ctx context.Context, ac *apiClient, url string) (bool, int, string, error) {
	// ac.get rides the impersonated transport — the neo endpoint rejects the
	// plain stdlib client (500/403) even on a healthy session.
	resp, err := ac.get(ctx, url, "")
	if err != nil {
		return false, 0, "transport error: " + err.Error(), err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		body, err := readBody(resp)
		var initial initialData
		if err != nil || json.Unmarshal(body, &initial) != nil || initial.Accounts == nil {
			return false, 200, "unexpected banking response; session not verified", nil
		}
		return true, 200, "session accepted", nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, resp.StatusCode, "credentials rejected; run qb auth remint", nil
	default:
		return false, resp.StatusCode, "unexpected status from getInitialData", nil
	}
}
