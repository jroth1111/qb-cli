package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CaptureATSFromRelay attaches to the OMP relay CDP multiplex socket,
// enables Fetch on the given page target, navigates (or reloads) the
// banking SPA, and returns the first first-party request whose Authorization
// starts with Intuit_APIKey. Header values are secret; callers must not
// log the capture.
func CaptureATSFromRelay(ctx context.Context, relayURL, targetID, navigateURL string) (*ATSCapture, error) {
	if navigateURL == "" {
		navigateURL = BankingCaptureURL
	}
	return captureFromRelayHarvest(ctx, relayURL, targetID,
		[]string{"*://qbo.intuit.com/*", "*://*.api.intuit.com/*"},
		navigateURL, isATSCredentialRequest, isAPIKeyHeaderRequest)
}

// CaptureAuditFromRelay navigates the Audit Log UI and returns the
// Intuit_APIKey Authorization header the page sends to audit.api.intuit.com.
// It is best-effort: callers (remint) treat an error as non-fatal. The
// returned value is secret; callers must not log it verbatim.
func CaptureAuditFromRelay(ctx context.Context, relayURL, targetID, navigateURL string) (string, error) {
	if navigateURL == "" {
		navigateURL = AuditCaptureURL
	}
	cap, err := captureFromRelay(ctx, relayURL, targetID, "*://audit.api.intuit.com/*", navigateURL, func(reqURL string, headers map[string]string) bool {
		return strings.Contains(reqURL, "audit.api.intuit.com") && isIntuitAPIKey(headerGet(headers, "authorization"))
	})
	if err != nil {
		return "", err
	}
	return headerGet(cap.Headers, "authorization"), nil
}

// captureFromRelay is the shared CDP capture path: attach to a relay page
// target, enable Fetch for fetchPattern, navigate to navigateURL, and return
// the first paused request satisfying match. Cookies are best-effort.
func captureFromRelay(ctx context.Context, relayURL, targetID, fetchPattern, navigateURL string, match func(reqURL string, headers map[string]string) bool) (*ATSCapture, error) {
	return captureFromRelayDual(ctx, relayURL, targetID, fetchPattern, navigateURL, match, nil)
}

// captureFromRelayDual is captureFromRelay plus an optional secondary
// request predicate: when secondary is non-nil, paused requests satisfying
// it are harvested alongside the primary so split-credential fields
// (apikey/authtype/intuit_appid) fill in the same pass.
func captureFromRelayDual(ctx context.Context, relayURL, targetID, fetchPattern, navigateURL string, match, secondary func(reqURL string, headers map[string]string) bool) (*ATSCapture, error) {
	return captureFromRelayHarvest(ctx, relayURL, targetID, []string{fetchPattern}, navigateURL, match, secondary)
}

// captureFromRelayHarvest additionally harvests per-service-host request
// headers (*.api.intuit.com carrying an Intuit_APIKey) into
// ATSCapture.HostHeaders, refreshing TokenSet.URIHostHeaders each pass.
func captureFromRelayHarvest(ctx context.Context, relayURL, targetID string, fetchPatterns []string, navigateURL string, match, secondary func(reqURL string, headers map[string]string) bool) (*ATSCapture, error) {
	if targetID == "" {
		return nil, fmt.Errorf("missing relay tab id")
	}
	if navigateURL == "" {
		return nil, fmt.Errorf("missing navigate url")
	}
	wsURL, err := relayDebuggerURL(ctx, relayURL)
	if err != nil {
		return nil, fmt.Errorf("relay debugger url: %w", err)
	}
	conn, err := dialCDP(ctx, wsURL)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	raw, err := conn.call(ctx, "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	}, "")
	if err != nil {
		return nil, fmt.Errorf("Target.attachToTarget: %w", err)
	}
	var att struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &att); err != nil || att.SessionID == "" {
		return nil, fmt.Errorf("attach returned no sessionId")
	}
	sid := att.SessionID
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.call(dctx, "Target.detachFromTarget", map[string]any{"sessionId": sid}, "")
	}()
	// Cookie dump is best-effort; Fetch intercept is required.
	_, _ = conn.call(ctx, "Network.enable", map[string]any{}, sid)
	patterns := make([]map[string]string, len(fetchPatterns))
	for i, p := range fetchPatterns {
		patterns[i] = map[string]string{"urlPattern": p, "requestStage": "Request"}
	}
	if _, err := conn.call(ctx, "Fetch.enable", map[string]any{"patterns": patterns}, sid); err != nil {
		return nil, fmt.Errorf("fetch.enable: %w", err)
	}
	if _, err := conn.call(ctx, "Page.navigate", map[string]any{"url": navigateURL}, sid); err != nil {
		return nil, fmt.Errorf("page.navigate: %w", err)
	}
	headers, secondaryHeaders, hostHeaders, err := waitPausedMatch(ctx, conn, sid, match, secondary)
	if err != nil {
		return nil, err
	}
	cap := &ATSCapture{Headers: headers, SecondaryHeaders: secondaryHeaders, HostHeaders: hostHeaders}
	if cookies, cerr := getAllCookies(ctx, conn, sid); cerr == nil {
		cap.Cookies = cookies
	}
	// Identity mining: the tab just served ATS traffic, so the SPA is
	// hydrated and the bootstrap carries company/email/realm. Best-effort:
	// a relay that refuses Runtime keeps headers+cookies regardless.
	if raw, rerr := conn.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    IdentityJS,
		"returnByValue": true,
	}, sid); rerr == nil {
		cap.Identity = parseEvalIdentity(raw)
	}
	return cap, nil
}

// waitPausedMatch blocks until a paused Fetch request satisfies match, then
// returns its header map. When secondary is non-nil it keeps draining for a
// short grace window to also harvest the first secondary match, and every
// service-host request (*.api.intuit.com with its own Intuit_APIKey) is
// recorded into hostHeaders. Every paused request is continued so the tab
// never hangs on an intercept it does not care about.
func waitPausedMatch(ctx context.Context, conn *cdpConn, sid string, match, secondary func(reqURL string, headers map[string]string) bool) (map[string]string, map[string]string, map[string]map[string]string, error) {
	const secondaryGrace = 8 * time.Second
	var primary, extra map[string]string
	hostHeaders := map[string]map[string]string{}
	var graceDeadline <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			if primary != nil {
				// Deadline hit inside the post-primary grace window: the
				// credential is captured; the secondary fill is best-effort.
				return primary, extra, hostHeaders, nil
			}
			return nil, nil, nil, fmt.Errorf("%w: %v", ErrNoATSAuthorization, ctx.Err())
		case <-graceDeadline:
			return primary, extra, hostHeaders, nil
		case ev, ok := <-conn.events:
			if !ok {
				if primary != nil {
					return primary, extra, hostHeaders, nil
				}
				return nil, nil, nil, fmt.Errorf("%w: cdp closed", ErrNoATSAuthorization)
			}
			if ev.Method != "Fetch.requestPaused" {
				continue
			}
			var p fetchPaused
			if json.Unmarshal(ev.Params, &p) != nil {
				continue
			}
			// Continue every paused request so the tab does not hang,
			// even if this one is not the auth we want.
			go func(id string) {
				cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, _ = conn.call(cctx, "Fetch.continueRequest", map[string]any{"requestId": id}, sid)
			}(p.RequestID)
			headers := stringifyHeaders(p.Request.Headers)
			if host := ServiceHost(p.Request.URL, headers); host != "" {
				hostHeaders[host] = headers
			}
			if extra == nil && secondary != nil && secondary(p.Request.URL, headers) {
				extra = headers
				if primary != nil {
					return primary, extra, hostHeaders, nil
				}
			}
			if primary == nil && match != nil && match(p.Request.URL, headers) {
				primary = headers
				if secondary == nil || extra != nil {
					return primary, extra, hostHeaders, nil
				}
				graceDeadline = time.After(secondaryGrace)
			}
		}
	}
}

type fetchPaused struct {
	RequestID string `json:"requestId"`
	Request   struct {
		URL     string         `json:"url"`
		Headers map[string]any `json:"headers"`
	} `json:"request"`
}

func stringifyHeaders(in map[string]any) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		switch t := v.(type) {
		case string:
			if t != "" {
				out[k] = t
			}
		case []any:
			if len(t) > 0 {
				if s, ok := t[0].(string); ok && s != "" {
					out[k] = s
				}
			}
		}
	}
	return out
}

func getAllCookies(ctx context.Context, conn *cdpConn, sid string) ([]Cookie, error) {
	raw, err := conn.call(ctx, "Network.getAllCookies", map[string]any{}, sid)
	if err != nil {
		// chrome.debugger sometimes only exposes Network.getCookies.
		raw, err = conn.call(ctx, "Network.getCookies", map[string]any{}, sid)
		if err != nil {
			return nil, err
		}
	}
	var res struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	out := make([]Cookie, 0, len(res.Cookies))
	for _, c := range res.Cookies {
		if c.Name == "" || !intuitCookieHost(c.Domain) {
			continue
		}
		out = append(out, Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  unixExpiry(c.Expires),
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
		})
	}
	return out, nil
}

type cdpCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
}

func unixExpiry(sec float64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(sec), 0).UTC()
}

func intuitCookieHost(domain string) bool {
	d := strings.ToLower(strings.TrimPrefix(domain, "."))
	return strings.Contains(d, "intuit.com") || strings.Contains(d, "quickbooks")
}
