package client

import (
	"bytes"
	"context"
	"fmt"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"net/http"
	"net/url"
	"strings"
)

// doURIHost sends reminted cookies plus the leftover host's own request
// headers (including that host's Intuit_APIKey). It does not copy ATS
// RequestHeaders — those 403 these URI hosts.
//
// uriHostTransport is a test seam: when set it replaces the surf Chrome
// client so offline tests can route the production URL through the
// DefaultTransport intercept. Production code never sets it.
var uriHostTransport func(req *http.Request) (*http.Response, error)

func (c *apiClient) doURIHost(ctx context.Context, method, rawURL, host string, body []byte, overrides ...map[string]string) (*http.Response, error) {
	resp, err := c.doURIHostOnce(ctx, method, rawURL, host, body, overrides...)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || c.skipRemint || method != http.MethodGet {
		return resp, err
	}
	_ = drainAndClose(resp)
	rctx, cancel := context.WithTimeout(ctx, remintTimeout)
	defer cancel()
	if err = remint(rctx); err != nil {
		return nil, err
	}
	next, err := c.reloadedSession()
	if err != nil {
		return nil, err
	}
	next.skipRemint = true
	return next.doURIHostOnce(ctx, method, rawURL, host, body, overrides...)
}

func (c *apiClient) doURIHostOnce(ctx context.Context, method, rawURL, host string, body []byte, overrides ...map[string]string) (*http.Response, error) {
	if err := validateIntuitDestination(rawURL); err != nil {
		return nil, err
	}
	if c.tok == nil {
		return nil, fmt.Errorf("uri-host %s: missing credentials", host)
	}
	hdrs := c.tok.URIHostHeaders[host]
	if uriHostTransport != nil {
		// Test seam active: the intercepted transport never inspects these
		// headers, so synthesize a minimal Intuit_APIKey map instead.
		hdrs = map[string]string{
			"Authorization": "Intuit_APIKey intuit_apikey=uri-host-test,intuit_apikey_version=1.0",
		}
	} else {
		// Production: use captured per-host headers when present. Without
		// them the call cannot authenticate, so fail with guidance instead
		// of dialing a doomed request. Hosts like personalization.api key
		// their Intuit_APIKey in the URL query (the browser sends no
		// Authorization header there), so a captured apikey URL param also
		// satisfies the guard.
		auth := headerGetFold(hdrs, "authorization")
		u, _ := url.Parse(rawURL)
		if !strings.HasPrefix(strings.TrimSpace(auth), "Intuit_APIKey") && u.Query().Get("intuit_apikey") == "" {
			return nil, fmt.Errorf("uri-host %s: missing leftover-host Intuit_APIKey; recapture from the live Test Company tab", host)
		}
	}
	var rdr *bytes.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	var req *http.Request
	var err error
	if rdr != nil {
		req, err = http.NewRequestWithContext(ctx, method, rawURL, rdr)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, rawURL, nil)
	}
	if err != nil {
		return nil, err
	}
	skip := map[string]struct{}{
		"cookie": {}, "content-length": {}, "host": {},
		"connection": {}, "accept-encoding": {},
	}
	for k, v := range hdrs {
		if v == "" {
			continue
		}
		if _, ok := skip[strings.ToLower(k)]; ok {
			continue
		}
		req.Header.Set(k, v)
	}
	// A host may serve several schemas. Apply operation-specific routing to
	// this request only; an empty value removes a captured header.
	for _, headers := range overrides {
		for key, value := range headers {
			if value == "" {
				req.Header.Del(key)
			} else {
				req.Header.Set(key, value)
			}
		}
	}
	if strings.HasPrefix(req.URL.Path, "/api/neo/") {
		req.Header.Set("Accept", "*/*")
	}
	if req.Header.Get("Origin") == "" {
		req.Header.Set("Origin", "https://qbo.intuit.com")
	}
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}
	auth.BindRequest(req, c.tok, host)
	if uriHostTransport != nil {
		return sessionDo(req, uriHostTransport)
	}
	return impersonatedDo(req)
}

// Credentials captured for Intuit must never be attached to arbitrary probe
// URLs. The service set uses HTTPS Intuit subdomains, including first-party
// gateway calls that intentionally reuse qbo.intuit.com headers.
func validateIntuitDestination(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") ||
		(!strings.EqualFold(u.Hostname(), "intuit.com") && !strings.HasSuffix(strings.ToLower(u.Hostname()), ".intuit.com")) {
		return fmt.Errorf("refusing credentials outside an HTTPS Intuit destination")
	}
	return nil
}

func headerGetFold(h map[string]string, name string) string {
	want := strings.ToLower(name)
	for k, v := range h {
		if strings.ToLower(k) == want {
			return v
		}
	}
	return ""
}
