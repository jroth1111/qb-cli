package auth

import (
	"context"
	"errors"
	"testing"
)

func TestRelayRenewalSelectsBoundIdentityAndRejectsAmbiguity(t *testing.T) {
	old := renewalRelayIdentity
	defer func() { renewalRelayIdentity = old }()
	expected := &TokenSet{RealmID: "1", Email: "fixture@example.invalid"}
	identities := map[string]Identity{"wrong": {Realm: "2", Email: expected.Email}, "match": {Realm: "1", Email: expected.Email}, "duplicate": {Realm: "1", Email: expected.Email}}
	renewalRelayIdentity = func(_ context.Context, _ string, target string) (Identity, error) { return identities[target], nil }
	tabs := []relayPage{{ID: "wrong", URL: BankingCaptureURL}, {ID: "match", URL: BankingCaptureURL}}
	selected, err := selectRenewalRelayTab(context.Background(), "http://127.0.0.1:9222", tabs, expected)
	if err != nil || selected.ID != "match" {
		t.Fatal("first authenticated tab was treated as identity proof", err)
	}
	tabs = append(tabs, relayPage{ID: "duplicate", URL: BankingCaptureURL})
	if _, err = selectRenewalRelayTab(context.Background(), "endpoint", tabs, expected); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("ambiguous tabs accepted")
	}
	expected.CDPTargetID = "match"
	selected, err = selectRenewalRelayTab(context.Background(), "endpoint", tabs, expected)
	if err != nil || selected.ID != "match" {
		t.Fatal("exact target pin not respected")
	}
	expected.CDPTargetID = "missing"
	if _, err = selectRenewalRelayTab(context.Background(), "endpoint", tabs, expected); !errors.Is(err, ErrRemintNeedsLogin) {
		t.Fatal("missing pinned tab fell back")
	}
}
