package cli

import (
	"context"
	"encoding/json"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"testing"
	"time"
)

func keeperTestSession(t *testing.T) *auth.TokenSet {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	tok := &auth.TokenSet{RealmID: "1", Email: "person@example.test", Authorization: "Intuit_APIKey intuit_apikey=test"}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestKeeperTransientFailureUsesConfiguredBackoffAndCancels(t *testing.T) {
	tok := keeperTestSession(t)
	if err := writeKeeperFile("keepalive-policy.json", keeperPolicy{true, tok.SessionID, time.Minute, false}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := maintainSession
	maintainSession = func(context.Context, *auth.TokenSet, bool) (bool, int, string) {
		cancel()
		return false, 0, "temporarily_unavailable"
	}
	defer func() { maintainSession = old }()
	_ = runKeeper(ctx, tok.SessionID)
	var s keeperStatus
	if readKeeperFile("keepalive-status.json", &s) != nil {
		t.Fatal("missing keeper status")
	}
	delay := s.NextCheck.Sub(s.LastCheck)
	if delay < 119*time.Second || delay > 121*time.Second {
		t.Fatalf("first 1m failure backoff=%v", delay)
	}
}

func TestStaleKeeperStatusDoesNotClaimADeadProcessIsRunning(t *testing.T) {
	tok := keeperTestSession(t)
	if err := writeKeeperFile("keepalive-policy.json", keeperPolicy{true, tok.SessionID, 5 * time.Minute, false}); err != nil {
		t.Fatal(err)
	}
	if err := writeKeeperFile("keepalive-status.json", keeperStatus{SessionID: tok.SessionID, State: "live", NextCheck: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	out, err := runCoverageRoot(t, "auth", "keepalive", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if json.Unmarshal([]byte(out), &state) != nil || state["running"] != false {
		t.Fatal("dead keeper reported running")
	}
}

func TestKeeperStartBindsLoginAndCanStop(t *testing.T) {
	tok := keeperTestSession(t)
	old := launchKeeper
	calls := 0
	launchKeeper = func(home, id string) error {
		calls++
		if home != auth.HomeDir() || id != tok.SessionID {
			t.Error("wrong worker profile")
		}
		return nil
	}
	defer func() { launchKeeper = old }()
	if action, err := startKeeper(5*time.Minute, false); err != nil || action != "start_requested" {
		t.Fatalf("%s %v", action, err)
	}
	if calls != 1 || !keeperStillWanted(tok.SessionID) {
		t.Fatal("keeper did not register")
	}
	if err := stopKeeper(); err != nil {
		t.Fatal(err)
	}
	if keeperStillWanted(tok.SessionID) {
		t.Fatal("stop ignored")
	}
	if _, err := auth.Load(); err != nil {
		t.Fatal("stop logged out")
	}
}

func TestKeeperHarnessAndIntervals(t *testing.T) {
	keeperTestSession(t)
	for _, d := range []time.Duration{0, time.Second, time.Hour} {
		if _, err := startKeeper(d, false); err == nil {
			t.Fatal("unsafe interval accepted")
		}
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	if _, err := startKeeper(5*time.Minute, false); err == nil {
		t.Fatal("harness spawned keeper")
	}
}

func TestKeeperDoesNotFollowNewLoginAndStopsForReauthentication(t *testing.T) {
	tok := keeperTestSession(t)
	if err := writeKeeperFile("keepalive-policy.json", keeperPolicy{true, tok.SessionID, 5 * time.Minute, false}); err != nil {
		t.Fatal(err)
	}
	old := maintainSession
	calls := 0
	maintainSession = func(context.Context, *auth.TokenSet, bool) (bool, int, string) {
		calls++
		return false, 401, "needs_login"
	}
	defer func() { maintainSession = old }()
	if err := runKeeper(context.Background(), tok.SessionID); err != nil {
		t.Fatal(err)
	}
	var s keeperStatus
	if readKeeperFile("keepalive-status.json", &s) != nil || s.State != "needs_login" || calls != 1 {
		t.Fatal("reauthentication stop was not retained")
	}
	if err := auth.Save(&auth.TokenSet{RealmID: "2", Authorization: tok.Authorization}); err != nil {
		t.Fatal(err)
	}
	if keeperStillWanted(tok.SessionID) {
		t.Fatal("keeper followed a new login")
	}
	if err := runKeeper(context.Background(), tok.SessionID); err == nil {
		t.Fatal("stale worker started")
	}
}

func TestKeeperStopsWhenUserTakesEgoControl(t *testing.T) {
	tok := keeperTestSession(t)
	if err := writeKeeperFile("keepalive-policy.json", keeperPolicy{true, tok.SessionID, 5 * time.Minute, false}); err != nil {
		t.Fatal(err)
	}
	old := maintainSession
	maintainSession = func(context.Context, *auth.TokenSet, bool) (bool, int, string) { return false, 401, "user_control" }
	t.Cleanup(func() { maintainSession = old })
	if err := runKeeper(context.Background(), tok.SessionID); err != nil {
		t.Fatal(err)
	}
	var s keeperStatus
	if readKeeperFile("keepalive-status.json", &s) != nil || s.State != "user_control" || !s.NextCheck.IsZero() {
		t.Fatalf("keeper failed to stop for user control: %+v", s)
	}
}

func TestLoginKeepaliveDefaultsAndOptout(t *testing.T) {
	flags := &rootFlags{}
	cmd := buildLoginCmd("login", flags)
	enabled, _ := cmd.Flags().GetBool("keep-alive")
	if !enabled {
		t.Fatal("login does not default to maintenance")
	}
	if err := cmd.Flags().Set("keep-alive", "false"); err != nil {
		t.Fatal(err)
	}
	if autoStartKeeper(cmd, flags) != "disabled" {
		t.Fatal("login optout ignored")
	}
}
