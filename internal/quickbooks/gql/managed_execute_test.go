package gql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestManagedGraphQLUsesSessionProfileAndRealRequest(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	tok := &auth.TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey fixture"}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
	old := fetchManaged
	defer func() { fetchManaged = old }()
	calls := 0
	fetchManaged = func(_ context.Context, expected *auth.TokenSet, endpoint, method string, headers map[string]string, payload []byte) (*auth.ManagedResponse, error) {
		calls++
		if !auth.SameSession(tok, expected) || endpoint != "https://fitransactions.api.intuit.com/graphql" || method != "POST" || headers["accept"] != "application/json" {
			t.Fatal("wrong session backend or request headers")
		}
		var body map[string]any
		if json.Unmarshal(payload, &body) != nil || body["operationName"] != "FixtureQuery" || body["query"] != "query FixtureQuery { fixture }" {
			t.Fatal("real GraphQL request was not preserved")
		}
		return &auth.ManagedResponse{Status: 200, Body: `{"data":{"fixture":true}}`}, nil
	}
	response, err := Execute(context.Background(), Request{Op: &Op{Name: "FixtureQuery", Kind: "query", Document: "query FixtureQuery { fixture }", Endpoint: "https://fitransactions.api.intuit.com/graphql"}})
	if err != nil || calls != 1 || response.Status != 200 {
		t.Fatal("managed GraphQL was not executed")
	}
}

func TestBrowserDispatchCannotFallBackAfterLogoutOrNewLogin(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			tok := &auth.TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey fixture"}
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), executionSessionKey{}, tok)
			if replace {
				if err := auth.Save(&auth.TokenSet{Source: "managed-profile", RealmID: "2", Authorization: "Intuit_APIKey other"}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := auth.Delete(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := executeSession(ctx, Request{Op: &Op{Kind: "query"}}); err == nil {
				t.Fatal("dispatch accepted a replaced/deleted session")
			}
			if _, err := FetchEgo(ctx, "https://qbo.intuit.com/api/v4/fixture", "GET", nil, nil); err == nil {
				t.Fatal("browser REST dispatch fell back to another source")
			}
		})
	}
}

func TestManagedBrowserGETRotatesCredentialsButNeverRetriesWrites(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			tok := &auth.TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey old"}
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			oldFetch, oldRenew := fetchManaged, remintGQL
			defer func() { fetchManaged = oldFetch; remintGQL = oldRenew }()
			calls, renewals := 0, 0
			fetchManaged = func(_ context.Context, _ *auth.TokenSet, _ string, _ string, headers map[string]string, _ []byte) (*auth.ManagedResponse, error) {
				calls++
				if calls == 1 {
					return &auth.ManagedResponse{Status: 401, Body: `{}`}, nil
				}
				if headers["Authorization"] != "Intuit_APIKey new" || headers["X-Operation"] != "keep" {
					t.Fatal("browser GET kept stale credentials or dropped operation headers")
				}
				return &auth.ManagedResponse{Status: 200, Body: `{}`}, nil
			}
			remintGQL = func(context.Context) error {
				renewals++
				fresh, _ := auth.Load()
				fresh.Authorization = "Intuit_APIKey new"
				return auth.Save(fresh)
			}
			response, err := FetchEgo(context.Background(), "https://qbo.intuit.com/api/v4/fixture", method, map[string]string{"Authorization": "Intuit_APIKey old", "X-Operation": "keep"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if method == http.MethodGet {
				if calls != 2 || renewals != 1 || response.Status != 200 {
					t.Fatal("GET recovery failed")
				}
			} else if calls != 1 || renewals != 0 || response.Status != 401 {
				t.Fatal("browser write was replayed")
			}
		})
	}
}

func TestManagedSignedOutReadRecoversOnceButMutationNeverRetries(t *testing.T) {
	for _, kind := range []string{"query", "mutation", ""} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			if err := auth.Save(&auth.TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey fixture"}); err != nil {
				t.Fatal(err)
			}
			oldFetch, oldRemint := fetchManaged, remintGQL
			defer func() { fetchManaged = oldFetch; remintGQL = oldRemint }()
			calls, renewals := 0, 0
			fetchManaged = func(context.Context, *auth.TokenSet, string, string, map[string]string, []byte) (*auth.ManagedResponse, error) {
				calls++
				if calls == 1 {
					return nil, auth.ErrRemintNeedsLogin
				}
				return &auth.ManagedResponse{Status: 200, Body: `{"data":{}}`}, nil
			}
			remintGQL = func(context.Context) error { renewals++; return nil }
			_, err := Execute(context.Background(), Request{Op: &Op{Name: "Fixture", Kind: kind, Document: "fixture"}})
			if kind == "query" {
				if err != nil || calls != 2 || renewals != 1 {
					t.Fatal("read did not recover once")
				}
			} else if !errors.Is(err, auth.ErrRemintNeedsLogin) || calls != 1 || renewals != 0 {
				t.Fatal("mutation/unknown operation recovered or replayed")
			}
		})
	}
}
