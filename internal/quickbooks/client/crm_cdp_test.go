// crm_cdp_test.go exercises the CRM relay-tab transport against a fake OMP
// relay: /json/version and /json/list discovery, the multiplexed CDP
// websocket (Target.attachToTarget → Runtime.evaluate), and the in-page
// fetch expression the client generates. The fake parses the exact JS
// expression produced by crmFetchExpression, so the round trips assert
// endpoint, method, headers, and body precisely as they would reach the
// browser inside the qbo.intuit.com tab.
//
// Negative fixtures prove the guards reject wrong implementations:
// no authenticated tab (websocket never dialed), realm pin (zero dials),
// missing credentials (zero dials), and in-page fetch exceptions
// (transport error, never a fabricated response).
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// crmCapturedRequest records what the last evaluated in-page fetch did.
type crmCapturedRequest struct {
	Expression string
	URL        string
	Method     string
	Headers    map[string]string
	Body       string // "" when body interpolated as JS null
}

// crmEvalReply renders the full CDP Runtime.evaluate response PAYLOAD —
// the object under the frame's "result" key. Success carries the
// RemoteObject nested under its own "result" key (spec shape:
// {"result":{"type":..,"value":..}}); failures carry "exceptionDetails".
type crmEvalReply func(expression string) string

// crmStatusBodyResult mimics the in-page script: the promise resolves to a
// JSON.stringify'd {status, body} string, delivered by returnByValue as a
// quoted-string RemoteObject.
func crmStatusBodyResult(status int, bodyText string) string {
	inner := fmt.Sprintf(`{"status":%d,"body":%s}`, status, crmMustJSON(bodyText))
	return fmt.Sprintf(`{"result":{"type":"string","value":%s}}`, crmMustJSON(inner))
}

// crmExceptionResult makes Runtime.evaluate report a thrown in-page error.
const crmExceptionResult = `{"exceptionDetails":{"text":"Uncaught","exception":{"description":"TypeError: Failed to fetch"}}}`

// crmStatusBodyRawResult wraps an already-rendered {status, body} payload
// (used to simulate relay double-encoding) in the CDP-spec RemoteObject.
func crmStatusBodyRawResult(inner string) string {
	return fmt.Sprintf(`{"result":{"type":"string","value":%s}}`, crmMustJSON(inner))
}

// crmObjectFormResult delivers the status/body as a returnByValue object
// (some relays) whose body is itself a quoted JSON string.
func crmObjectFormResult(bodyText string) string {
	return fmt.Sprintf(`{"result":{"type":"object","value":{"status":200,"body":%s}}}`, crmMustJSON(bodyText))
}

// crmStartFakeRelay serves the OMP relay surface: /json/version pointing at
// a multiplexed debugger socket, /json/list with one authenticated qbo
// page plus one service worker, and /cdp answering attach/detach/evaluate.
// reply(nil) falls back to 200 with an empty JSON object body.
func crmStartFakeRelay(t *testing.T, captured *crmCapturedRequest, reply crmEvalReply) *httptest.Server {
	t.Helper()
	var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		ws := "ws" + strings.TrimPrefix(srv.URL, "http") + "/cdp"
		_ = json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": ws})
	})
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		tabs := []map[string]string{
			{"id": "PAGE1", "type": "page", "url": "https://qbo.intuit.com/app/home?realmId=12345"},
			{"id": "WORKER1", "type": "service_worker", "url": "https://qbo.intuit.com/sw.js"},
		}
		_ = json.NewEncoder(w).Encode(tabs)
	})
	mux.HandleFunc("/cdp", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				ID      int64           `json:"id"`
				Method  string          `json:"method"`
				Session string          `json:"sessionId"`
				Params  json.RawMessage `json:"params"`
			}
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Method {
			case "Target.attachToTarget":
				_ = conn.WriteJSON(map[string]any{"id": msg.ID, "result": json.RawMessage(`{"sessionId":"SESS1"}`)})
			case "Target.detachFromTarget":
				_ = conn.WriteJSON(map[string]any{"id": msg.ID, "result": json.RawMessage(`{}`)})
			case "Runtime.evaluate":
				var p struct {
					Expression string `json:"expression"`
				}
				_ = json.Unmarshal(msg.Params, &p)
				crmRecordFetch(p.Expression, captured)
				out := crmStatusBodyResult(http.StatusOK, `{}`)
				if reply != nil {
					out = reply(p.Expression)
				}
				_ = conn.WriteJSON(map[string]any{"id": msg.ID, "result": json.RawMessage(out)})
			default:
				_ = conn.WriteJSON(map[string]any{"id": msg.ID, "error": map[string]any{"message": "unknown method " + msg.Method}})
			}
		}
	})
	return srv
}

// crmRecordFetch extracts url/method/headers/body from the evaluated
// async-fetch expression. The template is fixed (crmFetchExpression), so
// anchored slicing is deterministic here.
func crmRecordFetch(expr string, into *crmCapturedRequest) {
	if into == nil {
		return
	}
	into.Expression = expr
	into.URL = crmUnquote(crmBetween(expr, `await fetch(`, `, {`))
	into.Method = crmUnquote(crmBetween(expr, `method: `, ",\n"))
	into.Headers = map[string]string{}
	for _, pair := range strings.Split(crmBetween(expr, `headers: {`, `}`), ", ") {
		parts := strings.SplitN(strings.TrimSpace(pair), ": ", 2)
		if len(parts) == 2 && parts[0] != "" {
			into.Headers[crmUnquote(parts[0])] = crmUnquote(parts[1])
		}
	}
	into.Body = crmUnquote(crmBetween(expr, "body: ", "\n"))
	if into.Body == "null" {
		into.Body = "" // JS null interpolation means "no body sent"
	}
}

func crmBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return rest
	}
	return rest[:j]
}

// crmUnquote resolves a double-quoted JSON string token; anything else is
// returned verbatim (single-quoted JS literals, bare null).
func crmUnquote(s string) string {
	if len(s) >= 2 && s[0] == '"' {
		var out string
		if json.Unmarshal([]byte(s), &out) == nil {
			return out
		}
	}
	return s
}

func crmMustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// crmSaveSession persists a token set satisfying auth.Load and the
// authenticated-tab classifier.
func crmSaveSession(t *testing.T) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	err := auth.Save(&auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       "12345",
		Cookies: []auth.Cookie{
			{Name: "qbo.csrftoken", Value: "csrf-token-123"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// --- negative fixtures ------------------------------------------------------

// TestCrmExecuteNoTabFailsWithoutDialingCDP proves the no-auth-tab guard:
// with only a service_worker and a sign-in page listed, crmExecute fails
// with ErrCrmNeedsRelayTab and never opens the debugger websocket. It
// rejects an implementation that attaches to non-page targets or answers
// without an authenticated tab.
func TestCrmExecuteNoTabFailsWithoutDialingCDP(t *testing.T) {
	crmSaveSession(t)
	var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	dialed := false
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		ws := "ws" + strings.TrimPrefix(srv.URL, "http") + "/cdp"
		_ = json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": ws})
	})
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"id": "SW1", "type": "service_worker", "url": "https://qbo.intuit.com/sw.js"},
			{"id": "OTHER", "type": "page", "url": "https://accounts.intuit.com/sign-in"},
		})
	})
	mux.HandleFunc("/cdp", func(w http.ResponseWriter, r *http.Request) {
		dialed = true
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	t.Setenv("QB_RELAY_URL", srv.URL)
	raw, status, err := crmExecute(context.Background(), PlanLeadList(0, 20, false))
	if !errors.Is(err, ErrCrmNeedsRelayTab) {
		t.Fatalf("err = %v, want ErrCrmNeedsRelayTab", err)
	}
	if raw != nil || status != 0 {
		t.Fatalf("raw=%q status=%d, want zero-value on failure", raw, status)
	}
	if dialed {
		t.Fatal("websocket dialed despite no authenticated page tab")
	}
}

// TestCrmExecuteNoCompanyGate proves crmExecute proceeds past credential
// validation for any session realm (no company gate): with a relay serving
// tabs it attempts the live interaction instead of refusing.
func TestCrmExecuteNoCompanyGate(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       "99999",
	}); err != nil {
		t.Fatal(err)
	}
	dials := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dials++
	}))
	t.Cleanup(srv.Close)

	t.Setenv("QB_RELAY_URL", srv.URL)
	_, _, err := crmExecute(context.Background(), PlanLeadList(0, 20, false))
	if err == nil {
		t.Fatal("expected relay-interaction error from stub relay, got nil")
	}
	if dials == 0 {
		t.Fatal("relay never dialed: a company gate is still refusing the session")
	}
}

// TestCrmReplayNoCredentialsZeroDial proves the credentials guard precedes
// every relay interaction, including /json/list.
func TestCrmReplayNoCredentialsZeroDial(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	dials := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dials++
	}))
	t.Cleanup(srv.Close)

	t.Setenv("QB_RELAY_URL", srv.URL)
	start := time.Now()
	_, err := ReplayLeadList(context.Background(), 0, 10, false)
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want ErrNoCredentials", err)
	}
	if dials != 0 {
		t.Fatalf("relay dials = %d, want 0 without credentials", dials)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %v, implies network activity", elapsed)
	}
}

// --- positive round trips ---------------------------------------------------

func TestCrmLeadListViaRelayTab(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	body := `{"content":[{"nameId":"L1","displayName":"Acme Corp"},{"nameId":"L2","displayName":"Globex"}],"totalElements":42}`
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		return crmStatusBodyResult(http.StatusOK, body)
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	res, err := ReplayLeadList(context.Background(), 0, 25, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Total != 42 || res.Count != 2 {
		t.Fatalf("envelope = %+v", res)
	}
	if len(res.Leads) != 2 || res.Leads[0].ID != "L1" || res.Leads[0].DisplayName != "Acme Corp" || res.Leads[1].ID != "L2" {
		t.Fatalf("leads = %+v", res.Leads)
	}
	wantURL := crmCoreBaseURL + "/leads?page=0&size=25&sort=updateDate%2Cdesc"
	if captured.URL != wantURL {
		t.Errorf("in-page fetch URL = %s, want %s", captured.URL, wantURL)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", captured.Method)
	}
	if !strings.Contains(captured.Expression, "credentials: 'include'") {
		t.Error("fetch must send credentials:'include' — cookie auth is the point of the relay tab")
	}
	if got := captured.Headers["csrftoken"]; got != "csrf-token-123" {
		t.Errorf("csrftoken header = %q, want saved cookie value", got)
	}
	if got := captured.Headers["intuit-company-id"]; got != "12345" {
		t.Errorf("intuit-company-id = %q, want 12345", got)
	}
	if captured.Body != "" {
		t.Errorf("GET must interpolate null body, got %q", captured.Body)
	}
}

func TestCrmLeadCreateViaRelayTab(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		return crmStatusBodyResult(http.StatusCreated, `{"nameId":"NEW1","displayName":"Initech"}`)
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	res, err := ReplayLeadCreate(context.Background(), CrmLeadInput{DisplayName: "Initech", Email: "b@initech.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "NEW1" || res.DisplayName != "Initech" {
		t.Fatalf("created = %+v", res)
	}
	if captured.Method != http.MethodPost {
		t.Fatalf("method = %s, want POST", captured.Method)
	}
	if captured.Body == "" || !strings.Contains(captured.Body, `"displayName":"Initech"`) {
		t.Fatalf("POST body not forwarded: %q", captured.Body)
	}
	if captured.URL != crmCoreBaseURL+"/leads" {
		t.Errorf("URL = %s", captured.URL)
	}
}

func TestCrmLeadCountBareNumberViaRelayTab(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		return crmStatusBodyResult(http.StatusOK, `7`)
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	n, err := ReplayLeadCount(context.Background(), "CSV")
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("count = %d, want 7", n)
	}
	want := crmCoreBaseURL + "/leads/count?sourceTypes=CSV"
	if captured.URL != want {
		t.Errorf("URL = %s, want %s", captured.URL, want)
	}
}

func TestCrmOpportunityListAndDeleteStatus(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		return crmStatusBodyResult(http.StatusOK, `{"items":[{"id":"O9","displayName":"Upsell"}],"totalElements":1}`)
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := ReplayOpportunityList(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "Upsell" || res.Total != 1 {
		t.Fatalf("opportunities = %+v", res)
	}
	want := crmCoreBaseURL + "/opportunities?page=0&size=50&sort=updateDate%2Cdesc"
	if captured.URL != want {
		t.Errorf("URL = %s, want %s", captured.URL, want)
	}

	status, err := ReplayLeadDelete(ctx, "L1")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("delete status = %d", status)
	}
	if captured.Method != http.MethodDelete {
		t.Fatalf("delete method = %s, want DELETE", captured.Method)
	}
	if captured.Body != "" {
		t.Fatalf("DELETE must interpolate null body, got %q", captured.Body)
	}
}

// TestCrmSurfacesHTTPErrorStatus proves a completed-but-failed exchange is
// surfaced as a status, not converted into a transport error: callers see
// the server's verdict through the tolerant projections.
func TestCrmSurfacesHTTPErrorStatus(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		return crmStatusBodyResult(http.StatusInternalServerError, `{"message":"boom"}`)
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	res, err := ReplayAssociationList(context.Background(), "C1")
	if err != nil {
		t.Fatalf("transport error returned for HTTP-level failure: %v", err)
	}
	if res.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 surfaced", res.Status)
	}
	if res.Count != 0 {
		t.Fatalf("count = %d, want 0 for error body", res.Count)
	}
}

// --- protocol edge cases ----------------------------------------------------

// TestCrmUnwrapsDoubleEncodedBodies covers relays that double-encode: the
// returnByValue payload arrives as a quoted string holding the JSON
// status/body object, whose own body is again a quoted string. Both layers
// must unwrap before projection.
func TestCrmUnwrapsDoubleEncodedBodies(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	envelope := `{"content":[{"nameId":"L3"}],"totalElements":1}`
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		// Relay double-encoding: the whole {status, body} object arrives
		// as a quoted string (stringify'd twice upstream); the body inside
		// is itself a quoted envelope. Both unwraps must happen.
		inner := fmt.Sprintf(`{"status":200,"body":%s}`, crmMustJSON(envelope))
		return crmStatusBodyRawResult(inner)
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	res, err := ReplayLeadSearch(context.Background(), "acme", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Leads) != 1 || res.Leads[0].ID != "L3" {
		t.Fatalf("expected unwrapped page rows, got %+v", res)
	}
}

// TestCrmObjectFormResultUnwrapsQuotedBody covers relays delivering the
// returnByValue payload as an object (not stringify'd string) with a
// quoted body — the second unwrap branch.
func TestCrmObjectFormResultUnwrapsQuotedBody(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	quotedEnvelope := `"` + `{"content":[],"totalElements":0}` + `"`
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		return crmObjectFormResult(quotedEnvelope)
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	res, err := ReplayAssociationList(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Count != 0 {
		t.Fatalf("expected empty association list from object-form result, got %+v", res)
	}
}

// TestCrmInPageFetchThrowIsTransportError proves an in-page exception
// (e.g. CORS rejection) becomes a transport error wrapping
// ErrCrmNeedsRelayTab — never a fabricated empty response.
func TestCrmInPageFetchThrowIsTransportError(t *testing.T) {
	crmSaveSession(t)
	var captured crmCapturedRequest
	srv := crmStartFakeRelay(t, &captured, func(string) string {
		return crmExceptionResult
	})
	t.Setenv("QB_RELAY_URL", srv.URL)

	plan, err := PlanLeadGet("L1")
	if err != nil {
		t.Fatal(err)
	}
	raw, status, err := crmExecute(context.Background(), plan)
	if !errors.Is(err, ErrCrmNeedsRelayTab) {
		t.Fatalf("err = %v, want ErrCrmNeedsRelayTab", err)
	}
	if !strings.Contains(err.Error(), "threw") {
		t.Fatalf("error should mention the in-page throw, got %v", err)
	}
	if raw != nil || status != 0 {
		t.Fatalf("raw=%q status=%d, want zero-value on failure", raw, status)
	}
}

// TestCrmReplayNeedsRelayTabWhenRelayDead replaces the retired
// ErrCrmBlockedExternal behaviour contract: with a session saved but no
// relay listening, replays fail with ErrCrmNeedsRelayTab instead of a
// blanket external-block error.
func TestCrmReplayNeedsRelayTabWhenRelayDead(t *testing.T) {
	crmSaveSession(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // dead endpoint: deterministic connection refused
	t.Setenv("QB_RELAY_URL", srv.URL)

	ctx := context.Background()
	errs := []error{}
	_, e1 := ReplayLeadList(ctx, 0, 10, false)
	_, e2 := ReplayLeadCreate(ctx, CrmLeadInput{DisplayName: "x"})
	_, e3 := ReplayOpportunityList(ctx, 0, 10)
	errs = append(errs, e1, e2, e3)
	for i, err := range errs {
		if !errors.Is(err, ErrCrmNeedsRelayTab) {
			t.Errorf("case %d: got %v, want ErrCrmNeedsRelayTab", i, err)
		}
	}
}

var _ = strconv.Itoa // keep strconv pinned for future protocol fakes
