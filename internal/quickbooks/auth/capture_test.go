package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCaptureATSFromRelayFakeCDP(t *testing.T) {
	var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		ws := "ws" + strings.TrimPrefix(srv.URL, "http") + "/cdp"
		_ = json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": ws})
	})
	mux.HandleFunc("/cdp", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		sid := "SESS1"
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg cdpMsg
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Method {
			case "Target.attachToTarget":
				_ = conn.WriteJSON(cdpMsg{ID: msg.ID, Result: json.RawMessage(`{"sessionId":"` + sid + `"}`)})
			case "Network.enable", "Fetch.enable", "Fetch.continueRequest", "Target.detachFromTarget":
				_ = conn.WriteJSON(cdpMsg{ID: msg.ID, Result: json.RawMessage(`{}`)})
			case "Page.navigate":
				_ = conn.WriteJSON(cdpMsg{ID: msg.ID, Result: json.RawMessage(`{"frameId":"f"}`)})
				paused, _ := json.Marshal(map[string]any{
					"requestId": "R1",
					"request": map[string]any{
						"url": "https://qbo.intuit.com/ats/v1/company/99999/banking/getInitialData",
						"headers": map[string]any{
							"Authorization":     "Intuit_APIKey intuit_apikey=secret,intuit_apikey_version=1.0",
							"authtype":          "browser_auth",
							"csrftoken":         "csrf-short",
							"x-csrf-token":      "xcsrf-long",
							"intuit-company-id": "99999",
						},
					},
				})
				_ = conn.WriteJSON(cdpMsg{Method: "Fetch.requestPaused", Params: paused, SessionID: sid})
				apiPaused, _ := json.Marshal(map[string]any{
					"requestId": "R2",
					"request": map[string]any{
						"url": "https://qbo.intuit.com/api/v4/graphql",
						"headers": map[string]any{
							"Authorization": "Intuit_APIKey intuit_apikey=secondary,intuit_apikey_version=1.0",
							"apikey":        "k",
							"intuit_appid":  "app",
						},
					},
				})
				_ = conn.WriteJSON(cdpMsg{Method: "Fetch.requestPaused", Params: apiPaused, SessionID: sid})
			case "Network.getAllCookies", "Network.getCookies":
				_ = conn.WriteJSON(cdpMsg{ID: msg.ID, Result: json.RawMessage(`{"cookies":[{"name":"qbo.ticket","value":"v","domain":".qbo.intuit.com","path":"/","secure":true,"httpOnly":true}]}`)})
			default:
				_ = conn.WriteJSON(cdpMsg{ID: msg.ID, Error: &cdpErr{Message: "unknown " + msg.Method}})
			}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cap, err := CaptureATSFromRelay(ctx, srv.URL, "PAGE1", BankingCaptureURL)
	if err != nil {
		t.Fatal(err)
	}
	if !isIntuitAPIKey(headerGet(cap.Headers, "authorization")) {
		t.Fatal("missing Intuit_APIKey")
	}
	if headerGet(cap.Headers, "x-csrf-token") == headerGet(cap.Headers, "csrftoken") {
		t.Fatal("csrf headers not distinct")
	}
	if len(cap.Cookies) != 1 || cap.Cookies[0].Name != "qbo.ticket" {
		t.Fatalf("cookies %+v", cap.Cookies)
	}
	if headerGet(cap.SecondaryHeaders, "apikey") != "k" || headerGet(cap.SecondaryHeaders, "intuit_appid") != "app" {
		t.Fatal("secondary apikey request not harvested")
	}
	// Never leak the secret into the test name/log via Fatalf of the header map.
	if strings.Contains(cap.Headers["Authorization"], "Intuit_APIKey") == false {
		t.Fatal("authorization prefix missing")
	}
}

func TestCDPConnSerializesConcurrentWrites(t *testing.T) {
	var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

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
			var msg cdpMsg
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			_ = conn.WriteJSON(cdpMsg{ID: msg.ID, Result: json.RawMessage(`{}`)})
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/cdp"
	conn, err := dialCDP(ctx, wsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.close()

	const n = 32
	errCh := make(chan error, n)
	for range n {
		go func() {
			_, e := conn.call(ctx, "Fetch.continueRequest", map[string]any{"requestId": "R"}, "")
			errCh <- e
		}()
	}
	for range n {
		if e := <-errCh; e != nil {
			t.Fatalf("concurrent CDP write: %v", e)
		}
	}
}

func TestCaptureATSFromRelayMissingTab(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := CaptureATSFromRelay(ctx, "http://127.0.0.1:9", "", BankingCaptureURL)
	if err == nil {
		t.Fatal("expected error for empty target")
	}
}
