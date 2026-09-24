package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQuietRefreshRenewsOnlyPinnedEgoSource(t *testing.T) {
	setQBHome(t)
	tok := &TokenSet{Authorization: "Intuit_APIKey old", RealmID: "1", Email: "same@example.test", Source: "ego-existing", EgoSpace: "qb-gql", EgoTargetID: "bank-target"}
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	old := captureExistingEgo
	t.Cleanup(func() { captureExistingEgo = old })
	calls := 0
	captureExistingEgo = func(ctx context.Context, login, banking string) (*ATSCapture, error) {
		calls++
		opt, ok := ctx.Value(existingEgoKey{}).(existingEgoOptions)
		if !ok || opt.space != "qb-gql" || opt.target != "bank-target" || login != BankingCaptureURL || banking != BankingCaptureURL {
			t.Fatal("renewal changed browser source")
		}
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
			t.Fatal("unbounded Ego renewal")
		}
		return &ATSCapture{Headers: map[string]string{"Authorization": "Intuit_APIKey new", "intuit-company-id": "1"}, Identity: Identity{Realm: "1", Email: "same@example.test"}}, nil
	}
	if err := RefreshQuiet(context.Background(), tok, false); err != nil {
		t.Fatal(err)
	}
	fresh, err := Load()
	if err != nil || calls != 1 || fresh.Authorization != "Intuit_APIKey new" || fresh.SessionID != tok.SessionID || fresh.EgoSpace != "qb-gql" {
		t.Fatalf("renewal failed or changed identity: calls=%d err=%v", calls, err)
	}
}

func TestQuietRefreshStopsForUserControl(t *testing.T) {
	setQBHome(t)
	tok := &TokenSet{Authorization: "Intuit_APIKey old", RealmID: "1", Email: "same@example.test", Source: "ego-existing", EgoSpace: "qb-gql", EgoTargetID: "bank-target"}
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	old := captureExistingEgo
	t.Cleanup(func() { captureExistingEgo = old })
	captureExistingEgo = func(context.Context, string, string) (*ATSCapture, error) { return nil, ErrEgoUserControl }
	if err := RefreshQuiet(context.Background(), tok, false); !errors.Is(err, ErrEgoUserControl) {
		t.Fatalf("user-control stop lost: %v", err)
	}
	fresh, _ := Load()
	if fresh.Authorization != tok.Authorization {
		t.Fatal("failed capture changed credentials")
	}
}
