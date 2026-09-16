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
	// another browser when the server reports expired credentials.
	resp, err := ac.doStdlibJSON(ctx, http.MethodGet, ac.baseURL()+"/getInitialData", nil, "")
	if err != nil {
		return false, 0, "transport error: " + err.Error()
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	switch resp.StatusCode {
	case http.StatusOK:
		body, err := readBody(resp)
		var initial initialData
		if err != nil || json.Unmarshal(body, &initial) != nil || initial.Accounts == nil {
			return false, 200, "unexpected banking response; session not verified"
		}
		return true, 200, "session accepted"
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, resp.StatusCode, "credentials rejected; run qb auth remint"
	default:
		return false, resp.StatusCode, "unexpected status from getInitialData"
	}
}
