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
	if err := ExtendNativeTicket(context.Background(), after, false); !errors.Is(err, ErrNativeTicketPending) {
		t.Fatal(err)
	}
}

func TestNativeTicketRefusesChangedBrowserWithinSameLogin(t *testing.T) {
	for _, field := range []string{"source", "space", "target"} {
		t.Run(field, func(t *testing.T) {
			expected, _ := recoveryFixture(t)
			expected.Source, expected.EgoSpace, expected.EgoTargetID = "ego-existing", "52", "p1"
			if err := Save(expected); err != nil {
				t.Fatal(err)
			}
			expected, _ = Load()
			changed := *expected
			switch field {
			case "source":
				changed.Source = "managed-profile"
			case "space":
				changed.EgoSpace = "53"
			case "target":
				changed.EgoTargetID = "p2"
			}
			if err := Save(&changed); err != nil {
				t.Fatal(err)
			}
			if !SameSession(expected, &changed) {
				t.Fatal("fixture must preserve the login identity")
			}
			old := nativeTicketCapture
			t.Cleanup(func() { nativeTicketCapture = old })
			nativeTicketCapture = func(context.Context, *TokenSet) (*nativeTicketResult, error) {
				t.Fatal("different browser was contacted")
				return nil, nil
			}
			if err := ExtendNativeTicket(context.Background(), expected, true); !errors.Is(err, ErrSessionChanged) {
				t.Fatalf("browser switch accepted: %v", err)
			}
		})
	}
}

func TestPinnedEgoNativeTicketPreservesSourceAndNeverReadsVault(t *testing.T) {
	tok, _ := recoveryFixture(t)
	tok.Source, tok.EgoSpace, tok.EgoTargetID = "ego-existing", "52", "p1"
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	tok, _ = Load()
	old := nativeTicketCapture
	t.Cleanup(func() { nativeTicketCapture = old })
	nativeTicketCapture = func(_ context.Context, expected *TokenSet) (*nativeTicketResult, error) {
		if expected.Source != "ego-existing" || expected.EgoSpace != "52" || expected.EgoTargetID != "p1" {
			t.Fatal("Ego source not preserved")
		}
		return &nativeTicketResult{Extended: true, NextExpiry: time.Now().Add(26 * time.Minute).UnixMilli(), Capture: fixtureRecoveryCapture(expected)}, nil
	}
	if err := ExtendNativeTicket(context.Background(), tok, true); err != nil {
		t.Fatal(err)
	}
	after, _ := Load()
	if after.Source != tok.Source || after.EgoSpace != tok.EgoSpace || after.EgoTargetID != tok.EgoTargetID || !SameSession(tok, after) || after.NativeTicket.State != NativeTicketExtended {
		t.Fatal("native extension replaced the pinned source")
	}
	r, _ := loadRecovery()
	if !r.LastAttempt.IsZero() {
		t.Fatal("Ego native extension read recovery credentials")
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
