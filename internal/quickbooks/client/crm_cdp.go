// crm_cdp.go routes every CRM api/core call through the OMP relay's CDP
// debugger socket, mirroring gql/cdp_fetch.go. The CRM host
// mccrmmessaging.api.intuit.com authenticates with session cookies alone —
// direct cross-origin replay 403s — so each request runs as an in-page
// fetch inside an authenticated qbo.intuit.com tab via Runtime.evaluate,
// where credentials:'include' attaches the browser's QBO session context.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// ErrCrmNeedsRelayTab reports that a CRM call cannot run: the OMP relay is
// unreachable, no authenticated qbo.intuit.com tab exists, or the in-page
// fetch was blocked. Run `qb auth login` first.
var ErrCrmNeedsRelayTab = errors.New("crm api/core: no authenticated qbo.intuit.com relay tab; run `qb auth login`")

// crmEvalTimeout bounds dial + attach + evaluate for one CRM fetch.
const crmEvalTimeout = 30 * time.Second

// crmCdpID is the process-wide CDP request id source for CRM calls.
var crmCdpID atomic.Int64

type crmCDPRequest struct {
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	Session string          `json:"sessionId,omitempty"`
}

// crmExecute runs one planned CRM api/core call inside the authenticated
// relay tab and returns the response body and HTTP status. Guards run in
// order so each failure is attributable before any dial: missing
// credentials, realm pin, relay reachability, authenticated tab. Transport
// failures (no relay, no tab, blocked fetch, evaluate error) return an
// error; a completed HTTP exchange returns its status even for 4xx/5xx —
// the CRM projections surface the status rather than second-guessing the
// server, matching the tolerant-envelope design of this file.
func crmExecute(ctx context.Context, plan *RequestPlan) ([]byte, int, error) {
	tok, err := auth.Load()
	if err != nil {
		if isNoCredentials(err) {
			return nil, 0, ErrNoCredentials
		}
		return nil, 0, fmt.Errorf("crm: loading credentials: %w", err)
	}
	_ = tok // credentials validated; the session realm is the company.
	relayURL := crmRelayURL()
	tabs, err := crmListRelayPages(ctx, relayURL)
	if err != nil {
		return nil, 0, fmt.Errorf("%w (%s unreachable: %v)", ErrCrmNeedsRelayTab, relayURL, err)
	}
	tab := crmFindAuthTab(tabs)
	if tab == nil {
		return nil, 0, ErrCrmNeedsRelayTab
	}

	status, raw, err := crmEvalFetch(ctx, relayURL, tab.ID, plan.URL, plan.Method,
		[]byte(plan.Body), crmAuthHeaders(tok))
	if err != nil {
		return nil, 0, err
	}
	httpStatus := int(status)
	// status==0 means the in-page fetch threw — almost always a CORS
	// rejection for a cross-origin host from the qbo.intuit.com tab. The
	// SPA fetches mccrmmessaging successfully from that origin, so this
	// indicates the wrong tab or a broken relay, never a server answer.
	if httpStatus == 0 {
		return nil, 0, fmt.Errorf("%w: in-page fetch to %s was blocked (cross-origin/CORS)",
			ErrCrmNeedsRelayTab, plan.URL)
	}
	return raw, httpStatus, nil
}

// crmAuthHeaders returns the saved-session headers the SPA attaches to
// api/core calls. Cookies travel automatically via credentials:'include'
// (that is the actual auth for this cookie-only host); the company scope
// and CSRF token round the request out. Absent values are omitted.
func crmAuthHeaders(tok *auth.TokenSet) map[string]string {
	h := map[string]string{}
	if tok.RealmID != "" {
		h["intuit-company-id"] = tok.RealmID
	}
	for _, c := range tok.Cookies {
		if c.Name == "qbo.csrftoken" && c.Value != "" {
			h["csrftoken"] = c.Value
			break
		}
	}
	return h
}

// crmEvalFetch dials the relay debugger socket, attaches to the page
// target and evaluates an async fetch of endpoint with credentials
// included. The fetch runs in the SPA's origin, so session cookies and
// auth context apply — offline header replay 403s, this does not. An
// empty body renders as JS null (a body on GET/HEAD throws in-page).
// It returns the HTTP status and raw response body.
func crmEvalFetch(ctx context.Context, relayURL, targetID, endpoint, method string, body []byte, extraHeaders map[string]string) (float64, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, crmEvalTimeout)
	defer cancel()

	wsURL, err := crmRelayDebuggerURL(ctx, relayURL)
	if err != nil {
		return 0, nil, fmt.Errorf("%w (relay debugger endpoint: %v)", ErrCrmNeedsRelayTab, err)
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("%w (relay dial: %v)", ErrCrmNeedsRelayTab, err)
	}
	defer conn.Close()

	sid, err := crmAttachTarget(ctx, conn, targetID)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		dctx, dcancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer dcancel()
		crmCallCDP(dctx, conn, "", "Target.detachFromTarget", map[string]any{"sessionId": sid})
	}()

	headers := "{'Content-Type': 'application/json'"
	for k, v := range extraHeaders {
		headers += ", " + crmJSString(k) + ": " + crmJSString(v)
	}
	headers += "}"
	bodyExpr := "null"
	if len(body) > 0 {
		bodyExpr = crmJSString(string(body))
	}
	expr := fmt.Sprintf(crmFetchExpression, crmJSString(endpoint), crmJSString(method), headers, bodyExpr)
	raw, err := crmCallCDP(ctx, conn, sid, "Runtime.evaluate", map[string]any{
		"expression":    expr,
		"awaitPromise":  true,
		"returnByValue": true,
	})
	if err != nil {
		return 0, nil, fmt.Errorf("crm: Runtime.evaluate failed: %w", err)
	}
	var result struct {
		Result struct {
			Type        string          `json:"type"`
			Value       json.RawMessage `json:"value"`
			Description string          `json:"description"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, nil, fmt.Errorf("crm: decode evaluate result: %w", err)
	}
	if result.ExceptionDetails != nil {
		desc := result.ExceptionDetails.Exception.Description
		if desc == "" {
			desc = result.ExceptionDetails.Text
		}
		return 0, nil, fmt.Errorf("%w: in-page fetch threw: %s", ErrCrmNeedsRelayTab, desc)
	}
	if result.Result.Type == "string" {
		// returnByValue of a JSON.stringify'd string: value is a quoted
		// string holding {"status":..,"body":".."}.
		var s string
		if json.Unmarshal(result.Result.Value, &s) == nil && strings.HasPrefix(s, "{") {
			var parsed struct {
				Status     int             `json:"status"`
				StatusText string          `json:"statusText"`
				Body       json.RawMessage `json:"body"`
			}
			if json.Unmarshal([]byte(s), &parsed) == nil {
				// The inner body may itself be a quoted string holding
				// the envelope (double-encoded by the relay). Unwrap so
				// callers always receive the JSON payload itself.
				body := crmUnwrapQuotedObject(parsed.Body)
				return float64(parsed.Status), body, nil
			}
		}
	}
	if result.Result.Type != "object" || len(result.Result.Value) == 0 {
		desc := result.Result.Description
		if desc == "" {
			desc = "fetch did not return a status/body object"
		}
		return 0, nil, fmt.Errorf("crm: %s", desc)
	}
	var val struct {
		Status     float64         `json:"status"`
		StatusText string          `json:"statusText"`
		Body       json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(result.Result.Value, &val); err != nil {
		return 0, nil, fmt.Errorf("crm: decode status/body object: %w", err)
	}
	rawBody := crmUnwrapQuotedObject(val.Body)

	return val.Status, rawBody, nil
}

// crmUnwrapQuotedObject undoes relay double-encoding. Some relays deliver
// the response body as a quoted JSON string (occasionally nested more than
// one layer deep) instead of the value itself. Each iteration strips one
// quoting layer; the loop terminates because every unquote strictly
// shortens the payload.
func crmUnwrapQuotedObject(raw json.RawMessage) json.RawMessage {
	for len(raw) > 1 && raw[0] == '"' {
		var inner string
		if json.Unmarshal(raw, &inner) != nil {
			break
		}
		raw = json.RawMessage(inner)
	}
	return raw
}

// crmAttachTarget attaches to a relay page target and returns its session id.
func crmAttachTarget(ctx context.Context, conn *websocket.Conn, targetID string) (string, error) {
	raw, err := crmCallCDP(ctx, conn, "", "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	})
	var att struct {
		SessionID string `json:"sessionId"`
	}
	if err != nil {
		return "", fmt.Errorf("crm: Target.attachToTarget: %w", err)
	}
	if json.Unmarshal(raw, &att) != nil || att.SessionID == "" {
		return "", errors.New("crm: attach returned no sessionId")
	}
	return att.SessionID, nil
}

// crmCallCDP sends one CDP command on the shared socket and waits for its
// matching response, draining interleaved events.
func crmCallCDP(ctx context.Context, conn *websocket.Conn, session, method string, params map[string]any) (json.RawMessage, error) {
	pbytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	id := crmCdpID.Add(1)
	req := crmCDPRequest{ID: id, Method: method, Params: pbytes, Session: session}
	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("crm: CDP write %s: %w", method, err)
	}
	type frame struct {
		ID     int64           `json:"id"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
		Method  string          `json:"method,omitempty"`
		Params  json.RawMessage `json:"params,omitempty"`
		Session string          `json:"sessionId,omitempty"`
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var f frame
		if err := conn.ReadJSON(&f); err != nil {
			return nil, fmt.Errorf("crm: CDP read %s: %w", method, err)
		}
		if f.ID != id {
			continue // event or response to another request
		}
		if f.Error != nil {
			return nil, fmt.Errorf("%s", f.Error.Message)
		}
		return f.Result, nil
	}
}

// crmFetchExpression is the in-page async fetch template. It is evaluated
// with awaitPromise so the promise resolves before Runtime.evaluate
// replies. credentials:'include' sends the QBO session cookies bound to
// qbo.intuit.com; an empty body interpolates as JS null.
const crmFetchExpression = `(async () => {
  try {
    const resp = await fetch(%s, {
      method: %s,
      credentials: 'include',
      headers: %s,
      body: %s
    });
    const text = await resp.text();
    return JSON.stringify({status: resp.status, statusText: resp.statusText, body: text});
  } catch (e) {
    return JSON.stringify({status: 0, statusText: String(e), body: ''});
  }
})()`

// crmJSString renders s as a JavaScript double-quoted string literal via
// JSON encoding, which is valid JS for our controlled inputs.
func crmJSString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// crmRelayURL resolves the CDP relay endpoint: $QB_RELAY_URL or the
// default loopback port used by the OMP relay.
func crmRelayURL() string {
	if v := strings.TrimRight(os.Getenv("QB_RELAY_URL"), "/"); v != "" {
		return v
	}
	return auth.DefaultRelayURL
}

// crmRelayDebuggerURL resolves the multiplexed debugger WebSocket from the
// relay's /json/version, mirroring the gql gateway and auth helpers.
func crmRelayDebuggerURL(ctx context.Context, relayURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayURL+"/json/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("/json/version returned %s", resp.Status)
	}
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	if v.WebSocketDebuggerURL == "" {
		return "", errors.New("relay reported no webSocketDebuggerUrl")
	}
	return v.WebSocketDebuggerURL, nil
}

type crmRelayPage struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

// crmListRelayPages fetches /json/list from the relay. Duplicated here
// rather than imported because auth keeps these helpers unexported, the
// same trade-off the gql gateway accepted.
func crmListRelayPages(ctx context.Context, relayURL string) ([]crmRelayPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayURL+"/json/list", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relay /json/list returned %s", resp.Status)
	}
	var tabs []crmRelayPage
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, err
	}
	return tabs, nil
}

// crmFindAuthTab picks the first page target on an authenticated QBO app
// URL, mirroring auth's classification.
func crmFindAuthTab(tabs []crmRelayPage) *crmRelayPage {
	for i := range tabs {
		t := &tabs[i]
		if t.Type != "" && t.Type != "page" {
			continue
		}
		if auth.AuthenticatedURL(t.URL) {
			return t
		}
	}
	return nil
}
