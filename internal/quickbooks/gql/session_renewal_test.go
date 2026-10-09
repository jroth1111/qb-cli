package gql

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"testing"
)

func TestOnlyReadQueriesRetryAfterAuthenticationRenewal(t *testing.T) {
	for _, kind := range []string{"query", "mutation", ""} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			if err := auth.Save(&auth.TokenSet{RealmID: "1", Authorization: "Intuit_APIKey intuit_apikey=test"}); err != nil {
				t.Fatal(err)
			}
			previous, renew := executeEgoFn, remintGQL
			defer func() { executeEgoFn, remintGQL = previous, renew }()
			calls, refreshes := 0, 0
			executeEgoFn = func(context.Context, Request) (*Response, error) {
				calls++
				if calls == 1 {
					return &Response{Status: 401}, nil
				}
				return &Response{Status: 200, Body: json.RawMessage(`{"data":{}}`)}, nil
			}
			remintGQL = func(context.Context) error { refreshes++; return nil }
			_, err := Execute(context.Background(), Request{Op: &Op{Name: "Test", Kind: kind, Document: "query Test { fixture }"}})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "query" {
				if calls != 2 || refreshes != 1 {
					t.Fatal("read did not recover once")
				}
			} else if calls != 1 || refreshes != 0 {
				t.Fatal("non-read operation retried")
			}
		})
	}
}

func TestQueryRenewalRefusesNewLogin(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{RealmID: "1", Authorization: "Intuit_APIKey intuit_apikey=test"}); err != nil {
		t.Fatal(err)
	}
	previous, renew := executeEgoFn, remintGQL
	defer func() { executeEgoFn, remintGQL = previous, renew }()
	calls := 0
	executeEgoFn = func(context.Context, Request) (*Response, error) { calls++; return &Response{Status: 401}, nil }
	remintGQL = func(context.Context) error {
		return auth.Save(&auth.TokenSet{RealmID: "2", Authorization: "Intuit_APIKey intuit_apikey=other"})
	}
	if _, err := Execute(context.Background(), Request{Op: &Op{Name: "Test", Kind: "query", Document: "query Test { fixture }"}}); !errors.Is(err, auth.ErrSessionChanged) {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 {
		t.Fatal("retried into a new login")
	}
}
