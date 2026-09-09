package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPingV3Live(t *testing.T) {
	t.Parallel()
	var gotAuth, gotCookie, gotQuery bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization") == "Intuit_APIKey k"
		gotCookie = strings.Contains(r.Header.Get("Cookie"), "a=1")
		gotQuery = strings.Contains(r.URL.RawQuery, "CompanyInfo")
		_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
	}))
	defer srv.Close()
	tok := &TokenSet{
		RealmID: "123",
		Cookies: []Cookie{{Name: "a", Value: "1"}},
	}
	if err := pingV3URL(context.Background(), tok, "Intuit_APIKey k", srv.URL); err != nil {
		t.Fatalf("live ping: %v", err)
	}
	if !gotAuth || !gotCookie || !gotQuery {
		t.Errorf("request shape wrong: auth=%v cookie=%v query=%v", gotAuth, gotCookie, gotQuery)
	}
}

func TestPingV3Guards(t *testing.T) {
	t.Parallel()
	if err := PingV3(context.Background(), nil); err == nil {
		t.Error("nil token set must fail")
	}
	if err := PingV3(context.Background(), &TokenSet{}); err == nil {
		t.Error("missing realm must fail fast")
	}
	if err := PingV3(context.Background(), &TokenSet{RealmID: "123"}); err == nil {
		t.Error("missing authorization must fail fast")
	}
}

func TestPingV3Stale(t *testing.T) {
	t.Parallel()
	// 401 from the channel means proof-of-death, typed for branch logic.
	// 500 is transport trouble, not proof the session died.
	for code, wantStale := range map[int]bool{401: true, 403: true, 500: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		tok := &TokenSet{RealmID: "123"}
		err := pingV3URL(context.Background(), tok, "Intuit_APIKey k", srv.URL)
		srv.Close()
		if wantStale && !errors.Is(err, ErrSessionStale) {
			t.Errorf("%d must wrap ErrSessionStale, got %v", code, err)
		}
		if !wantStale && errors.Is(err, ErrSessionStale) {
			t.Errorf("%d must not read stale, got %v", code, err)
		}
	}
}

func TestApplyIdentityNeverOverwrites(t *testing.T) {
	t.Parallel()
	tok := &TokenSet{CompanyName: "Old Co", RealmID: "123"}
	tok.ApplyIdentity(Identity{Company: "New Co", Email: "n@x.io", Realm: "999"})
	if tok.CompanyName != "Old Co" {
		t.Errorf("company overwritten: %q", tok.CompanyName)
	}
	if tok.Email != "n@x.io" {
		t.Errorf("email not filled: %q", tok.Email)
	}
	if tok.RealmID != "123" {
		t.Errorf("realm overwritten: %q", tok.RealmID)
	}
}

func TestIdentityJSHasProvenShapes(t *testing.T) {
	t.Parallel()
	// Guards the shared mining expression against drift: the ego script and
	// relay evaluation use IdentityJS verbatim, so its patterns must keep
	// matching the verified bootstrap shapes.
	for _, want := range []string{`companyName`, `email`, `realmId`} {
		if !strings.Contains(IdentityJS, want) {
			t.Errorf("IdentityJS lost %q pattern", want)
		}
	}
}
