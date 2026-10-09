package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestTransportErrorsDoNotExposeSignedURLsOrAPIKeys(t *testing.T) {
	for _, secret := range []string{"intuit_apikey=fixture-private-key", "signature=fixture-private-signature"} {
		original := &url.Error{Op: "Get", URL: "https://service.api.intuit.com/download?" + secret, Err: context.DeadlineExceeded}
		redacted := privateTransportError(original)
		if strings.Contains(redacted.Error(), secret) || !errors.Is(redacted, context.DeadlineExceeded) {
			t.Fatal("private URL leaked or deadline identity lost")
		}
		var typed *url.Error
		if !errors.As(redacted, &typed) || typed != original {
			t.Fatal("transport failure cause was replaced")
		}
	}
}

func TestWrappedAuthenticationAndControlStopsAreNotNetworkRetries(t *testing.T) {
	for _, stop := range []error{auth.ErrRemintNeedsLogin, auth.ErrSessionChanged, auth.ErrEgoUserControl, auth.ErrRecoveryAttention, auth.ErrRecoveryKeyUnavailable} {
		request, _ := http.NewRequest("GET", "https://qbo.intuit.com/api/fixture", nil)
		calls := 0
		_, err := readReliably(request, func(*http.Request) (*http.Response, error) {
			calls++
			return nil, privateTransportError(&url.Error{Op: "Get", URL: request.URL.String(), Err: stop})
		})
		if calls != 1 || !errors.Is(err, stop) {
			t.Fatal("a wrapped stop was classified as a network retry", calls, err)
		}
	}
}

func TestReadRetriesReloadCredentialsButNeverSwitchBrowser(t *testing.T) {
	for _, changeSource := range []bool{false, true} {
		t.Run(fmt.Sprintf("source-change-%t", changeSource), func(t *testing.T) {
			saveUsable(t)
			tok, _ := auth.Load()
			request, _ := http.NewRequest("GET", "https://qbo.intuit.com/api/fixture", nil)
			request.Header.Set("Authorization", tok.Authorization)
			auth.BindRequest(request, tok, "")
			calls := 0
			response, err := sessionDo(request, func(attempt *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					fresh, _ := auth.Load()
					fresh.CredentialGeneration++
					fresh.Authorization = "Intuit_APIKey fresh-fixture"
					if changeSource {
						fresh.Source = "another-browser"
					}
					if err := auth.Save(fresh); err != nil {
						t.Fatal(err)
					}
					return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("temporary"))}, nil
				}
				if attempt.Header.Get("Authorization") != "Intuit_APIKey fresh-fixture" {
					t.Fatal("retry used stale credentials")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Request: attempt}, nil
			})
			if changeSource {
				if calls != 1 || !errors.Is(err, auth.ErrSessionChanged) {
					t.Fatal("retry switched browser", calls, err)
				}
			} else if calls != 2 || err != nil || response.StatusCode != 200 {
				t.Fatal("same-session rotation failed", calls, err)
			}
			if response != nil {
				drainAndClose(response)
			}
		})
	}
}

func TestImpersonatedTransportSendsChromeHelloAndVerifiesTLS(t *testing.T) {
	hellos := make(chan []uint16, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("untrusted TLS certificate was accepted")
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		select {
		case hellos <- append([]uint16(nil), hello.CipherSuites...):
		default:
		}
		return nil, nil
	}}
	srv.StartTLS()
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := impersonatedDo(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("self-signed TLS certificate must fail verification")
	}
	select {
	case ciphers := <-hellos:
		for _, c := range ciphers {
			if c&0x0f0f == 0x0a0a && byte(c>>8) == byte(c) {
				return
			}
		}
		t.Fatalf("no Chrome GREASE cipher in ClientHello: %v", ciphers)
	default:
		t.Fatal("no TLS ClientHello observed")
	}
}

func TestForbiddenPOSTIsNotRetriedAsGET(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || string(body) != `{"probe":true}` {
			t.Errorf("request changed: %s %s", r.Method, body)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type=%q, want application/json", got)
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := &apiClient{tok: &auth.TokenSet{}}
	resp, err := c.post(context.Background(), srv.URL, []byte(`{"probe":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	if calls != 1 || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("calls=%d status=%d", calls, resp.StatusCode)
	}
}

func TestImpersonationPreservesAPIHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept overwritten: %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type overwritten: %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("authorization not preserved")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		req, err := http.NewRequest(method, srv.URL, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer synthetic")
		resp, err := impersonatedDo(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = drainAndClose(resp)
	}
}

func TestImpersonationDoesNotRestoreRedirectCredentials(t *testing.T) {
	end := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"Authorization", "Cookie", "Apikey", "Csrftoken", "X-Csrf-Token", "Intuit-Company-Id", "Referer"} {
			if r.Header.Get(name) != "" {
				t.Errorf("%s leaked across redirect", name)
			}
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer end.Close()
	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(end.URL, "127.0.0.1", "localhost", 1), http.StatusFound)
	}))
	defer start.Close()
	req, err := http.NewRequest(http.MethodGet, start.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer synthetic")
	req.Header.Set("Cookie", "secret=synthetic")
	for _, name := range []string{"Apikey", "Csrftoken", "X-Csrf-Token", "Intuit-Company-Id", "Referer"} {
		req.Header.Set(name, "synthetic")
	}
	resp, err := impersonatedDo(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = drainAndClose(resp)
}

func TestImpersonatedPOSTRedirectKeepsBody(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(body) != "fixture" {
			t.Errorf("method/body changed: %s %s", r.Method, body)
		}
		if r.URL.Path == "/first" {
			http.Redirect(w, r, "/second", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/first", strings.NewReader("fixture"))
	resp, err := impersonatedDo(req)
	if resp != nil {
		drainAndClose(resp)
	}
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
