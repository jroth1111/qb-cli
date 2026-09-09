// http.go provides the in-process HTTP transport for QBO frontend banking
// API replays. It replaces the former replay.py (curl_cffi) helper.
//
// Transport ladder (evidence 2026-08-17, /tmp/qb-transport-probe):
//   - stdlib net/http succeeds (200) on both getInitialData and
//     getTransactions with the full captured header set, so it is the
//     default.
//   - On a 403 only, a single retry is issued through a surf
//     Impersonate().Chrome() client (no ForceHTTP3; HTTP/3 measured at
//     ~11s and rejected). surf is pinned at v1.0.199 in go.mod.
//
// No secrets are ever printed or logged: headers carry Authorization,
// apikey, csrf, and cookie values, and this file never writes any of them
// to an output stream.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/enetx/surf"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// bankingBaseURL is the ATS banking API root scoped to a realm.
const bankingBaseURL = "https://qbo.intuit.com/ats/v1/company/%s/banking"

// bankingReferer is the SPA page the frontend APIs expect as Referer.
const bankingReferer = "https://qbo.intuit.com/app/banking"

// csrfCookieName is the cookie that carries the QBO CSRF token.
const csrfCookieName = "qbo.csrftoken"

// httpTimeout is the per-request ceiling for both ladder rungs.
const httpTimeout = 30 * time.Second

// apiClient holds the loaded credentials and provides the two API entry
// points. It is constructed once per replay and reused across ladder rungs.
type apiClient struct {
	tok        *auth.TokenSet
	realm      string
	cookies    string // pre-built Cookie header value (secret)
	csrf       string // qbo.csrftoken value (secret)
	skipRemint bool   // set after one 401 remint so we never loop
}

// remint recaptures ATS credentials after a 401. Tests replace it.
var remint = auth.RemintATS

// remintTimeout is the auto-remint ceiling inside get(). It covers the
// relay attempt plus a headless managed-profile refresh (~60-90s warm);
// only a 401 pays it, and success heals silently.
const remintTimeout = 100 * time.Second

func newAPIClient() (*apiClient, error) {
	tok, err := auth.Load()
	if err != nil {
		if isNoCredentials(err) {
			return nil, ErrNoCredentials
		}
		return nil, fmt.Errorf("loading credentials: %w", err)
	}
	// No company gate: the login session's realm is the company. Every
	// command operates on whatever company the user signed in as.
	if !tok.HasUsableCredential() {
		return nil, fmt.Errorf("%w: saved session has no ATS Intuit_APIKey", ErrNoCredentials)
	}
	c := &apiClient{
		tok:   tok,
		realm: tok.RealmID,
	}
	c.cookies = cookieHeader(tok.Cookies)
	c.csrf = csrfToken(tok.Cookies)
	return c, nil
}

// baseURL returns the realm-scoped banking API root.
func (c *apiClient) baseURL() string {
	return fmt.Sprintf(bankingBaseURL, c.realm)
}

// applyHeaders sets the full captured header set on req. The headers carry
// secrets; callers must never log req or its Header map.
//
// When TokenSet.RequestHeaders is present (mitm import), every captured
// ATS header is sent as stored — including distinct csrftoken vs
// x-csrf-token, User-Agent, Accept, Referer, intuit_tid, sec-ch-ua*.
// Cookie is always taken from the full persisted jar. x-range from the
// caller overlays the captured pagination header.
func (c *apiClient) applyHeaders(req *http.Request, xRange string) {
	if len(c.tok.RequestHeaders) > 0 {
		for k, v := range c.tok.RequestHeaders {
			if v == "" || skipReplayHeader(k) {
				continue
			}
			if strings.EqualFold(k, "x-range") && xRange != "" {
				continue
			}
			req.Header.Set(k, v)
		}
		if c.cookies != "" {
			req.Header.Set("Cookie", c.cookies)
		}
		if xRange != "" {
			req.Header.Set("x-range", xRange)
		}
		return
	}
	authz := strings.TrimSpace(c.tok.Authorization)
	if authz == "" {
		authz = strings.TrimSpace(c.tok.AccessToken)
	}
	if strings.HasPrefix(authz, "Intuit_APIKey") || strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		req.Header.Set("Authorization", authz)
	} else if authz != "" {
		tt := c.tok.TokenType
		if tt == "" {
			tt = "Intuit_APIKey"
		}
		req.Header.Set("Authorization", tt+" "+authz)
	}
	if c.tok.TokenType != "" {
		req.Header.Set("authtype", c.tok.TokenType)
	} else if authz != "" {
		req.Header.Set("authtype", "Intuit_APIKey")
	}
	if c.tok.APIKey != "" {
		req.Header.Set("apikey", c.tok.APIKey)
	}
	if c.csrf != "" {
		req.Header.Set("csrftoken", c.csrf)
		req.Header.Set("x-csrf-token", c.csrf)
	}
	if c.realm != "" {
		req.Header.Set("intuit-company-id", c.realm)
	}
	if c.tok.IntuitAppID != "" {
		req.Header.Set("intuit_appid", c.tok.IntuitAppID)
	}
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}
	req.Header.Set("Referer", bankingReferer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("content-type", "application/json")
	if xRange != "" {
		req.Header.Set("x-range", xRange)
	}
}

// skipReplayHeader drops hop-by-hop / transport headers that must not be
// copied from a captured Chrome request. Accept-Encoding in particular
// disables net/http's automatic decompression.
func skipReplayHeader(name string) bool {
	switch strings.ToLower(name) {
	case "cookie", "accept-encoding", "content-length", "host",
		"connection", "transfer-encoding", "keep-alive", "upgrade":
		return true
	default:
		return false
	}
}

// get performs a GET against url with the captured headers, applying the
// transport ladder: stdlib first, and on a 403 a single retry through a
// surf Chrome-impersonating client. A non-403 error or status is returned
// immediately (no further rungs). The response body is the caller's to
// close.
func (c *apiClient) get(ctx context.Context, url, xRange string) (*http.Response, error) {
	resp, err := c.doStdlib(ctx, http.MethodGet, url, nil, xRange)

	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && !c.skipRemint {
		_ = drainAndClose(resp)
		rctx, cancel := context.WithTimeout(ctx, remintTimeout)
		rerr := remint(rctx)
		cancel()
		if rerr != nil {
			return nil, fmt.Errorf("ATS 401: %w", rerr)
		}
		next, nerr := newAPIClient()

		if nerr != nil {
			return nil, fmt.Errorf("ATS 401 remint reload: %w", nerr)
		}
		next.skipRemint = true
		return next.get(ctx, url, xRange)
	}
	if resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}
	_ = drainAndClose(resp)
	return c.doSurf(ctx, url, xRange)
}

// getJSON is get with Accept: application/json overlaid after captured headers.
func (c *apiClient) getJSON(ctx context.Context, url, xRange string) (*http.Response, error) {
	resp, err := c.doStdlibJSON(ctx, http.MethodGet, url, nil, xRange)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && !c.skipRemint {
		_ = drainAndClose(resp)
		rctx, cancel := context.WithTimeout(ctx, remintTimeout)
		rerr := remint(rctx)
		cancel()
		if rerr != nil {
			return nil, fmt.Errorf("ATS 401: %w", rerr)
		}
		next, nerr := newAPIClient()
		if nerr != nil {
			return nil, fmt.Errorf("ATS 401 remint reload: %w", nerr)
		}
		next.skipRemint = true
		return next.getJSON(ctx, url, xRange)
	}
	if resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}
	_ = drainAndClose(resp)
	return c.doSurf(ctx, url, xRange)
}

func (c *apiClient) postJSON(ctx context.Context, rawURL string, body []byte) (*http.Response, error) {
	return c.postJSONExtra(ctx, rawURL, body, nil)
}

func (c *apiClient) postJSONExtra(ctx context.Context, rawURL string, body []byte, extra map[string]string) (*http.Response, error) {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, rdr)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.applyHeaders(req, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extra {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	cli := &http.Client{Timeout: httpTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && !c.skipRemint {
		_ = drainAndClose(resp)
		rctx, cancel := context.WithTimeout(ctx, remintTimeout)
		rerr := remint(rctx)
		cancel()
		if rerr != nil {
			return nil, fmt.Errorf("ATS 401: %w", rerr)
		}
		next, nerr := newAPIClient()
		if nerr != nil {
			return nil, fmt.Errorf("ATS 401 remint reload: %w", nerr)
		}
		next.skipRemint = true
		return next.postJSONExtra(ctx, rawURL, body, extra)
	}
	return resp, nil
}

func (c *apiClient) doStdlibJSON(ctx context.Context, method, rawURL string, body []byte, xRange string) (*http.Response, error) {

	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.applyHeaders(req, xRange)
	req.Header.Set("Accept", "application/json")
	cli := &http.Client{Timeout: httpTimeout}
	return cli.Do(req)
}

func (c *apiClient) doStdlib(ctx context.Context, method, url string, body []byte, xRange string) (*http.Response, error) {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.applyHeaders(req, xRange)
	cli := &http.Client{Timeout: httpTimeout}
	return cli.Do(req)
}

func (c *apiClient) post(ctx context.Context, url string, body []byte) (*http.Response, error) {
	resp, err := c.doStdlib(ctx, http.MethodPost, url, body, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && !c.skipRemint {
		_ = drainAndClose(resp)
		rctx, cancel := context.WithTimeout(ctx, remintTimeout)
		rerr := remint(rctx)
		cancel()
		if rerr != nil {
			return nil, fmt.Errorf("ATS 401: %w", rerr)
		}
		next, nerr := newAPIClient()
		if nerr != nil {
			return nil, fmt.Errorf("ATS 401 remint reload: %w", nerr)
		}
		next.skipRemint = true
		return next.post(ctx, url, body)
	}
	if resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}
	_ = drainAndClose(resp)
	return c.doSurf(ctx, url, "")
}

// doSurf issues the request via a surf Impersonate().Chrome() client with
// a 30s timeout and no ForceHTTP3.
func (c *apiClient) doSurf(ctx context.Context, url, xRange string) (*http.Response, error) {
	builder := surf.NewClient().
		Builder().
		Impersonate().
		Chrome().
		Timeout(httpTimeout)
	sc, err := builder.Build().Result()
	if err != nil {
		return nil, fmt.Errorf("building surf client: %w", err)
	}
	std := sc.Std()
	std.Timeout = httpTimeout

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building surf request: %w", err)
	}
	c.applyHeaders(req, xRange)
	return std.Do(req)
}

// qboQueryCookieCap is the qbo.intuit.com v3 /query WAF limit.
// A remint jar that keeps every VS* ticket exceeds this and the
// query returns 400 "maximum allowed number of cookies [200]".
// ReplayQuery then lies with "entity not available".
const qboQueryCookieCap = 200

func isRemintTicketCookie(name string) bool {
	return strings.HasPrefix(name, "VS")
}

// cookieHeader builds a "name=value; name=value" Cookie header from the
// captured jar. Values are taken verbatim. The result is secret.
// VS* remint tickets are dropped first so the header stays at or under
// qboQueryCookieCap. The on-disk jar is not rewritten.
func cookieHeader(cookies []auth.Cookie) string {
	keep := make([]auth.Cookie, 0, len(cookies))
	for _, c := range cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		if isRemintTicketCookie(c.Name) {
			continue
		}
		keep = append(keep, c)
	}
	if len(keep) > qboQueryCookieCap {
		keep = keep[:qboQueryCookieCap]
	}
	parts := make([]string, 0, len(keep))
	for _, c := range keep {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// csrfToken pulls the CSRF token from the qbo.csrftoken cookie. The result
// is secret.
func csrfToken(cookies []auth.Cookie) string {
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			return c.Value
		}
	}
	return ""
}

// drainAndClose reads remaining bytes from resp.Body and closes it so the
// connection can be reused. Errors are ignored; the body is being discarded.
func drainAndClose(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	defer resp.Body.Close()
	// Read a bounded amount; we only need to drain for pool reuse.
	const drainLimit = 1 << 16
	_, _ = readBounded(resp.Body, drainLimit)
	return nil
}

// readBounded reads up to n bytes from r and discards them.
func readBounded(r interface{ Read([]byte) (int, error) }, n int) (int, error) {
	buf := make([]byte, 4096)
	var total int
	for total < n {
		max := n - total
		if max > len(buf) {
			max = len(buf)
		}
		m, err := r.Read(buf[:max])
		total += m
		if err != nil {
			return total, err
		}
		if m == 0 {
			return total, nil
		}
	}
	return total, nil
}

// isNoCredentials reports whether err is (or wraps) auth.ErrNoCredentials.
func isNoCredentials(err error) bool {
	return err != nil && (err == auth.ErrNoCredentials ||
		strings.Contains(err.Error(), auth.ErrNoCredentials.Error()))
}
