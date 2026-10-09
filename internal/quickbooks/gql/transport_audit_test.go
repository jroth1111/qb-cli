package gql

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestAuthRenewalNeverReplaysMislabelledMutationOrEmptyDocument(t *testing.T) {
	for _, document := range []string{"", "mutation DoIt { change }", "subscription Watch { changed }", "query Read { x } mutation Write { change }"} {
		t.Run(document, func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			if err := auth.Save(&auth.TokenSet{RealmID: "1", Authorization: "Intuit_APIKey fixture"}); err != nil {
				t.Fatal(err)
			}
			oldExec, oldRemint := executeEgoFn, remintGQL
			defer func() { executeEgoFn, remintGQL = oldExec, oldRemint }()
			calls, renewals := 0, 0
			executeEgoFn = func(context.Context, Request) (*Response, error) { calls++; return &Response{Status: 401}, nil }
			remintGQL = func(context.Context) error { renewals++; return nil }
			_, _ = Execute(context.Background(), Request{Op: &Op{Kind: "query", Document: document}})
			if calls != 1 || renewals != 0 {
				t.Fatal("unsafe GraphQL document was replayed", calls, renewals)
			}
		})
	}
}

func TestEgoFailureCodesKeepAuthenticationAndControlDistinct(t *testing.T) {
	for code, want := range map[string]error{"need-login": auth.ErrRemintNeedsLogin, "user-control": auth.ErrEgoUserControl, "dialog-open": auth.ErrRecoveryAttention, "pinned-page-unavailable": auth.ErrSessionChanged, "identity-mismatch": auth.ErrSessionChanged} {
		if !errors.Is(egoExecutionFailure(code), want) {
			t.Fatal("wrong failure classification", code)
		}
	}
	private := "PRIVATE_PROVIDER_DIAGNOSTIC"
	if strings.Contains(egoExecutionFailure(private).Error(), private) {
		t.Fatal("provider output escaped")
	}
}

func TestBrowserHEADReauthUsesFreshHeadersWithoutChangingSource(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	tok := &auth.TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey old"}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
	oldFetch, oldRemint := fetchManaged, remintGQL
	defer func() { fetchManaged, remintGQL = oldFetch, oldRemint }()
	calls, renewals := 0, 0
	fetchManaged = func(_ context.Context, _ *auth.TokenSet, _ string, method string, headers map[string]string, _ []byte) (*auth.ManagedResponse, error) {
		calls++
		if method != "HEAD" {
			t.Fatal("HEAD changed method")
		}
		if calls == 1 {
			return nil, auth.ErrRemintNeedsLogin
		}
		if headers["Authorization"] != "Intuit_APIKey new" {
			t.Fatal("stale auth replayed")
		}
		return &auth.ManagedResponse{Status: 200}, nil
	}
	remintGQL = func(context.Context) error {
		renewals++
		fresh, _ := auth.Load()
		fresh.Authorization = "Intuit_APIKey new"
		return auth.Save(fresh)
	}
	response, err := FetchEgo(context.Background(), "https://qbo.intuit.com/api/v4/fixture", "HEAD", map[string]string{"Authorization": "Intuit_APIKey old"}, nil)
	if err != nil || response.Status != 200 || calls != 2 || renewals != 1 {
		t.Fatal("HEAD recovery failed", err, calls, renewals)
	}
}

func TestQueryRenewalCannotSwitchTransportWithinSameLogicalSession(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	tok := &auth.TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey old"}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
	oldExec, oldRemint := executeEgoFn, remintGQL
	defer func() { executeEgoFn, remintGQL = oldExec, oldRemint }()
	calls := 0
	executeEgoFn = func(context.Context, Request) (*Response, error) {
		calls++
		return &Response{Status: 401, Body: json.RawMessage(`{}`)}, nil
	}
	remintGQL = func(context.Context) error {
		fresh, _ := auth.Load()
		fresh.Source = "ego-existing"
		fresh.EgoSpace = "2"
		fresh.EgoTargetID = "p1"
		return auth.Save(fresh)
	}
	if _, err := Execute(context.Background(), Request{Op: &Op{Kind: "query", Document: "query Read { x }"}}); !errors.Is(err, auth.ErrSessionChanged) {
		t.Fatal("backend switched", err)
	}
	if calls != 1 {
		t.Fatal("read replayed through another transport")
	}
}

func TestBrowserRESTDownloadsAreNotAssumedToBeJSON(t *testing.T) {
	for _, accept := range []string{"", "application/octet-stream", "application/json"} {
		t.Run(accept, func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			tok := &auth.TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey fixture"}
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			oldFetch := fetchManaged
			defer func() { fetchManaged = oldFetch }()
			calls := 0
			fetchManaged = func(context.Context, *auth.TokenSet, string, string, map[string]string, []byte) (*auth.ManagedResponse, error) {
				calls++
				return &auth.ManagedResponse{Status: 200, Body: string([]byte{0, 0xff, 0xfe, 0x80})}, nil
			}
			response, err := FetchEgo(context.Background(), "https://qbo.intuit.com/api/v4/fixture", "GET", map[string]string{"Accept": accept}, nil)
			if accept == "application/json" {
				if err == nil || calls != 3 {
					t.Fatal("incomplete expected JSON accepted", err, calls)
				}
			} else if err != nil || calls != 1 || string(response.Body) != string([]byte{0, 0xff, 0xfe, 0x80}) {
				t.Fatal("binary download was changed or retried", err, calls)
			}
		})
	}
}
