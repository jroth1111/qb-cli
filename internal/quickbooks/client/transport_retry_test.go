package client

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/enetx/surf"
)

func TestImpersonatedPOSTDoesNotReplayAfterServerProcessesBody(t *testing.T) {
	for _, payload := range []string{`{"mutation":"fixture"}`, ""} {
		t.Run(fmt.Sprintf("body-bytes-%d", len(payload)), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != payload {
					t.Errorf("body=%s err=%v", body, err)
				}
				calls.Add(1)
				if r.ProtoMajor == 2 {
					panic(http.ErrAbortHandler)
				}
				_, _ = w.Write([]byte(`{"id":"duplicate"}`))
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()
			// This loopback fault fixture uses surf's test TLS defaults; production
			// certificate verification is covered by TestImpersonatedTransportSendsChromeHelloAndVerifiesTLS.
			sc, err := surf.NewClient().Builder().Impersonate().Chrome().Timeout(3 * time.Second).Build().Result()
			if err != nil {
				t.Fatal(err)
			}
			client := sc.Std()
			client.Transport = requestHeaderTransport{base: client.Transport}
			client.Timeout = 3 * time.Second
			req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			if calls.Load() != 1 || err == nil {
				t.Fatalf("server processed %d requests; err=%v; mutation must not replay after stream failure", calls.Load(), err)
			}
		})
	}
}

func TestImpersonatedPOSTStillWorksOnHTTP1OnlyTLS(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if string(body) != "fixture" {
			t.Errorf("body=%s", body)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	sc, err := surf.NewClient().Builder().Impersonate().Chrome().Timeout(3 * time.Second).Build().Result()
	if err != nil {
		t.Fatal(err)
	}
	client := sc.Std()
	client.Transport = requestHeaderTransport{base: client.Transport}
	defer client.CloseIdleConnections()
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("fixture"))
	resp, err := client.Do(req)
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if err != nil || calls.Load() != 1 {
		t.Fatalf("requests=%d err=%v", calls.Load(), err)
	}
}
