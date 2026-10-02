package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNativeTicketDueUsesRuntimeTimingAndBackoff(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		state *NativeTicketStatus
		want  bool
	}{
		{nil, true},
		{&NativeTicketStatus{NextDue: now.Add(26 * time.Minute)}, false},
		{&NativeTicketStatus{NextDue: now.Add(5 * time.Minute)}, false},
		{&NativeTicketStatus{NextDue: now.Add(time.Minute)}, true},
		{&NativeTicketStatus{State: "extension_failed", LastAttempt: now.Add(-time.Minute)}, false},
		{&NativeTicketStatus{State: "extension_failed", LastAttempt: now.Add(-6 * time.Minute)}, true},
	} {
		if got := nativeTicketDue(tc.state, now); got != tc.want {
			t.Fatalf("due=%v want=%v", got, tc.want)
		}
	}
}

func TestNativeTicketExtensionIsBoundedAndPreservesLogin(t *testing.T) {
	tok, _ := recoveryFixture(t)
	old := nativeTicketCapture
	t.Cleanup(func() { nativeTicketCapture = old })
	calls := 0
	nativeTicketCapture = func(_ context.Context, expected *TokenSet) (*nativeTicketResult, error) {
		calls++
		return &nativeTicketResult{Extended: true, NextExpiry: time.Now().Add(26 * time.Minute).UnixMilli(), Capture: fixtureRecoveryCapture(expected)}, nil
	}
	if err := ExtendNativeTicket(context.Background(), tok, false); err != nil {
		t.Fatal(err)
	}
	after, _ := Load()
	if !SameSession(tok, after) || tok.CredentialGeneration != after.CredentialGeneration || after.NativeTicket.State != "extended" {
		t.Fatal("native extension changed login or lacked proof")
	}
	if err := ExtendNativeTicket(context.Background(), after, false); err != nil || calls != 1 {
		t.Fatal("not-due extension launched another browser")
	}
	if err := ExtendNativeTicket(context.Background(), after, true); err != nil || calls != 2 {
		t.Fatal("explicit forced extension did not run once")
	}
}

func TestNativeTicketFailuresNeverResolveCredentials(t *testing.T) {
	tok, _ := recoveryFixture(t)
	old := nativeTicketCapture
	t.Cleanup(func() { nativeTicketCapture = old })
	nativeTicketCapture = func(context.Context, *TokenSet) (*nativeTicketResult, error) { return nil, context.DeadlineExceeded }
	if err := ExtendNativeTicket(context.Background(), tok, false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	after, _ := Load()
	if !SameSession(tok, after) || after.Authorization != tok.Authorization || after.NativeTicket.State != "extension_failed" {
		t.Fatal("failed extension overwrote credentials")
	}
	r, _ := loadRecovery()
	if !r.LastAttempt.IsZero() {
		t.Fatal("native maintenance initiated a credential attempt")
	}
	nativeTicketCapture = func(context.Context, *TokenSet) (*nativeTicketResult, error) {
		t.Fatal("backoff invoked browser")
		return nil, nil
	}
	if err := ExtendNativeTicket(context.Background(), after, false); err != nil {
		t.Fatal(err)
	}
}

func TestNativeTicketRefusesReplacedSessionsAndHarness(t *testing.T) {
	tok, _ := recoveryFixture(t)
	old := nativeTicketCapture
	t.Cleanup(func() { nativeTicketCapture = old })
	nativeTicketCapture = func(context.Context, *TokenSet) (*nativeTicketResult, error) {
		_ = Save(&TokenSet{RealmID: "other", Source: "managed-profile", Authorization: "Intuit_APIKey fixture", Email: "other@example.invalid"})
		return &nativeTicketResult{Extended: true, NextExpiry: time.Now().Add(time.Hour).UnixMilli(), Capture: fixtureRecoveryCapture(tok)}, nil
	}
	if err := ExtendNativeTicket(context.Background(), tok, true); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("in-flight native extension resurrected old login")
	}
	after, _ := Load()
	if after.NativeTicket != nil || after.RealmID != "other" {
		t.Fatal("extension modified replacement login")
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	if err := ExtendNativeTicket(context.Background(), after, true); !errors.Is(err, ErrRecoveryDisabled) {
		t.Fatal("harness permitted native network work")
	}
}
