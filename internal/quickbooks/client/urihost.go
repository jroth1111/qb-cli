package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/enetx/surf"
)

// doURIHost sends reminted cookies plus the leftover host's own request
// headers (including that host's Intuit_APIKey). It does not copy ATS
// RequestHeaders — those 403 these URI hosts.
//
// uriHostTransport is a test seam: when set it replaces the surf Chrome
// client so offline tests can route the production URL through the
// DefaultTransport intercept. Production code never sets it.
var uriHostTransport func(req *http.Request) (*http.Response, error)

func (c *apiClient) doURIHost(ctx context.Context, method, rawURL, host string, body []byte) (*http.Response, error) {
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
		// of dialing a doomed request.
		auth := headerGetFold(hdrs, "authorization")
		if !strings.HasPrefix(strings.TrimSpace(auth), "Intuit_APIKey") {
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
	if req.Header.Get("Origin") == "" {
		req.Header.Set("Origin", "https://qbo.intuit.com")
	}
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}
	if uriHostTransport != nil {
		return uriHostTransport(req)
	}
	builder := surf.NewClient().Builder().Impersonate().Chrome().Timeout(httpTimeout)
	sc, err := builder.Build().Result()
	if err != nil {
		return nil, fmt.Errorf("uri-host %s: building surf client: %w", host, err)
	}
	std := sc.Std()
	std.Timeout = httpTimeout
	return std.Do(req)
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
