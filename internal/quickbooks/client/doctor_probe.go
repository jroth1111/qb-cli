package client

import (
	"context"
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
	resp, err := ac.get(ctx, ac.baseURL()+"/getInitialData", "")
	if err != nil {
		return false, 0, "transport error: " + err.Error()
	}
	defer drainAndClose(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		return true, 200, "session accepted"
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, resp.StatusCode, "credentials rejected; run qb auth remint"
	default:
		return false, resp.StatusCode, "unexpected status from getInitialData"
	}
}
