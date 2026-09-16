package client

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

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
