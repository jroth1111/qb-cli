package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// FetchTabIdentity evaluates IdentityJS in the live relay tab targetID and
// returns the signed-in company/email/realm. It attaches, evaluates, and
// detaches; Fetch interception is untouched so an in-flight capture keeps
// working. Best-effort at the call site: a tab that refuses Runtime yields
// an error, never partial identity.
func FetchTabIdentity(ctx context.Context, relayURL, targetID string) (Identity, error) {
	if targetID == "" {
		return Identity{}, fmt.Errorf("missing relay tab id")
	}
	wsURL, err := relayDebuggerURL(ctx, relayURL)
	if err != nil {
		return Identity{}, fmt.Errorf("relay debugger url: %w", err)
	}
	conn, err := dialCDP(ctx, wsURL)
	if err != nil {
		return Identity{}, fmt.Errorf("cdp dial: %w", err)
	}
	defer conn.close()
	raw, err := conn.call(ctx, "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	}, "")
	if err != nil {
		return Identity{}, fmt.Errorf("Target.attachToTarget: %w", err)
	}
	var att struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &att); err != nil || att.SessionID == "" {
		return Identity{}, fmt.Errorf("attach returned no sessionId")
	}
	sid := att.SessionID
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.call(dctx, "Target.detachFromTarget", map[string]any{"sessionId": sid}, "")
	}()
	evalRaw, err := conn.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    IdentityJS,
		"returnByValue": true,
	}, sid)
	if err != nil {
		return Identity{}, fmt.Errorf("runtime.evaluate: %w", err)
	}
	id := parseEvalIdentity(evalRaw)
	if id == (Identity{}) {
		return Identity{}, fmt.Errorf("tab returned no identity (not hydrated or not signed in)")
	}
	return id, nil
}

// parseEvalIdentity decodes a Runtime.evaluate response holding the
// IdentityJS object. Standard CDP nests it at result.result.value; some
// relays flatten to result.value — accept either, so mining never fails on
// envelope shape.
func parseEvalIdentity(raw json.RawMessage) Identity {
	var nested struct {
		Result struct {
			Result struct {
				Value Identity `json:"value"`
			} `json:"result"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &nested); err == nil {
		if nested.Result.Result.Value != (Identity{}) {
			return nested.Result.Result.Value
		}
	}
	var flat struct {
		Result struct {
			Value Identity `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &flat); err == nil {
		return flat.Result.Value
	}
	return Identity{}
}

// FetchTabCookies reads the live tab's cookie jar over relay CDP. This is
// the freshest jar available — the Chrome on-disk store lags it. Only
// Intuit/QuickBooks cookies are returned. Fails when the relay refuses
// Network domain access.
func FetchTabCookies(ctx context.Context, relayURL, targetID string) ([]Cookie, error) {
	if targetID == "" {
		return nil, fmt.Errorf("missing relay tab id")
	}
	wsURL, err := relayDebuggerURL(ctx, relayURL)
	if err != nil {
		return nil, fmt.Errorf("relay debugger url: %w", err)
	}
	conn, err := dialCDP(ctx, wsURL)
	if err != nil {
		return nil, fmt.Errorf("cdp dial: %w", err)
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
	return getQBOCookies(ctx, conn, sid)
}

// ErrSessionStale reports a proof-of-death liveness check: the server
// refused the saved session (401/403). Distinct from transport failures,
// which are unknown, not stale.
var ErrSessionStale = fmt.Errorf("saved session is stale")

// PingV3 proves the session against the v3 query channel the CLI actually
// uses (qbo.intuit.com/api/v3, ATS headers — not the homepage shell, which
// redirects to sign-in even for API-healthy jars). A minimal read-only
// CompanyInfo query: 200 = live; 401/403 = stale.
func PingV3(ctx context.Context, t *TokenSet) error {
	if t == nil || t.RealmID == "" {
		return fmt.Errorf("no realm to ping")
	}
	authz := t.Authorization
	if authz == "" {
		authz = headerGet(t.RequestHeaders, "authorization")
	}
	if authz == "" {
		return fmt.Errorf("no authorization to ping with")
	}
	return pingV3URL(ctx, t, authz, "https://qbo.intuit.com/api/v3/company/"+t.RealmID)
}

// pingV3URL is PingV3 with injectable auth and base URL so tests prove the
// request shape and status mapping without dialing production.
func pingV3URL(ctx context.Context, t *TokenSet, authz, base string) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	url := base + "/query?minorversion=73&query=" + "select%20%2A%20from%20CompanyInfo%20maxresults%201"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building v3 ping: %w", err)
	}
	req.Header.Set("Cookie", sessionCookieHeader(t.Cookies))
	req.Header.Set("Authorization", authz)
	if ua := headerGet(t.RequestHeaders, "user-agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("Accept", "application/json")
	BindRequest(req, t, "")
	req, err = PrepareSessionRequest(req)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("v3 ping: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_ = ObserveSessionResponse(req, resp)
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w (v3 ping %d)", ErrSessionStale, resp.StatusCode)
	default:
		return fmt.Errorf("v3 ping: unexpected status %d", resp.StatusCode)
	}
}
