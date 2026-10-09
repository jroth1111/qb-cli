package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

const keepaliveInterval = 5 * time.Minute

type keeperPolicy struct {
	Enabled      bool          `json:"enabled"`
	SessionID    string        `json:"session_id"`
	Interval     time.Duration `json:"interval_ns"`
	AllowManaged bool          `json:"allow_managed"`
}
type keeperStatus struct {
	SessionID   string                    `json:"session_id"`
	State       string                    `json:"state"`
	PID         int                       `json:"pid"`
	Realm       string                    `json:"realm"`
	LastCheck   time.Time                 `json:"last_check"`
	NextCheck   time.Time                 `json:"next_check"`
	HTTPStatus  int                       `json:"http_status,omitempty"`
	Maintenance []client.MaintenanceEvent `json:"maintenance,omitempty"`
}

var maintainSession = client.MaintainSession
var launchKeeper = launchKeeperProcess

func nativeKeeperDelay(interval time.Duration, tok *auth.TokenSet, now time.Time) time.Duration {
	if !auth.SupportsNativeTicket(tok) || tok.NativeTicket == nil || tok.NativeTicket.State != auth.NativeTicketExtended {
		return interval
	}
	until := tok.NativeTicket.NextDue.Add(-time.Minute).Sub(now)
	if until > 0 {
		return min(interval, until)
	}
	return interval
}

func readKeeperFile(name string, out any) error {
	raw, err := os.ReadFile(filepath.Join(auth.HomeDir(), name))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
func writeKeeperFile(name string, value any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	unlock, err := auth.LockProfile(ctx, auth.HomeDir(), "credentials")
	if err != nil {
		return err
	}
	defer unlock()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(auth.HomeDir(), ".keepalive-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(auth.HomeDir(), name))
}
func keeperHarness() bool {
	return os.Getenv("PRINTING_PRESS_VERIFY") == "1" || os.Getenv("PRINTING_PRESS_DOGFOOD") == "1"
}

func startKeeper(interval time.Duration, allowManaged bool) (string, error) {
	if keeperHarness() {
		return "", fmt.Errorf("keep-alive processes are refused under verification harnesses")
	}
	if interval < time.Minute || interval > 15*time.Minute {
		return "", fmt.Errorf("keep-alive interval must be between 1m and 15m")
	}
	tok, err := auth.EnsureSession()
	if err != nil {
		return "", err
	}
	if !tok.HasUsableCredential() || tok.RealmID == "" {
		return "", fmt.Errorf("keep-alive requires a usable company-bound login")
	}
	var old keeperStatus
	_ = readKeeperFile("keepalive-status.json", &old)
	policy := keeperPolicy{true, tok.SessionID, interval, allowManaged}
	if err := writeKeeperFile("keepalive-policy.json", policy); err != nil {
		return "", err
	}
	// A recent matching status plus an OS-held lease proves another keeper owns
	// this profile. A dead process releases the lease even if its status survives.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	unlock, lockErr := auth.LockProfile(ctx, auth.HomeDir(), "keepalive")
	if errors.Is(lockErr, context.DeadlineExceeded) && old.SessionID == tok.SessionID && old.State != "stopped" && time.Now().Before(old.NextCheck.Add(time.Minute)) {
		return "already_running", nil
	}
	if lockErr == nil {
		unlock()
	}
	if lockErr != nil && !errors.Is(lockErr, context.DeadlineExceeded) {
		return "", lockErr
	}
	if err := launchKeeper(auth.HomeDir(), tok.SessionID); err != nil {
		return "", err
	}
	return "start_requested", nil
}

func launchKeeperProcess(home, sessionID string) error {
	var err error
	home, err = filepath.Abs(home)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Tests exercise launch through a seam, never recursively spawn test binaries.
	if strings.HasSuffix(exe, ".test") || strings.HasSuffix(exe, ".test.exe") {
		return fmt.Errorf("cannot launch a keeper from a test binary")
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = devnull.Close() }()
	cmd := exec.Command(exe, "auth", "keepalive", "--session-id", sessionID, "--home", home)
	cmd.Dir = home
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	detachKeeper(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func stopKeeper() error {
	var p keeperPolicy
	_ = readKeeperFile("keepalive-policy.json", &p)
	p.Enabled = false
	return writeKeeperFile("keepalive-policy.json", p)
}

func keeperStillWanted(id string) bool {
	var p keeperPolicy
	if readKeeperFile("keepalive-policy.json", &p) != nil || !p.Enabled || p.SessionID != id {
		return false
	}
	tok, err := auth.Load()
	return err == nil && tok.SessionID == id
}

func keeperLeaseHeld() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	unlock, err := auth.LockProfile(ctx, auth.HomeDir(), "keepalive")
	if err == nil {
		unlock()
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// A quiet keeper owns one profile lease. Only GET probes run; transient outages
// back off up to 30 minutes. Control-file checks (no network) make logout/stop
// responsive while preserving that network backoff.
func runKeeper(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if keeperHarness() {
		return fmt.Errorf("keep-alive is refused under verification harnesses")
	}
	expected, err := auth.Load()
	if err != nil {
		return err
	}
	if !expected.HasUsableCredential() || expected.RealmID == "" {
		return fmt.Errorf("keep-alive requires a usable company-bound login")
	}
	if id == "" {
		expected, err = auth.EnsureSession()
		if err != nil {
			return err
		}
		id = expected.SessionID
		if err = writeKeeperFile("keepalive-policy.json", keeperPolicy{true, id, keepaliveInterval, true}); err != nil {
			return err
		}
	}
	if expected.SessionID != id || !keeperStillWanted(id) {
		return auth.ErrSessionChanged
	}
	lctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	unlock, err := auth.LockProfile(lctx, auth.HomeDir(), "keepalive")
	if err != nil {
		return err
	}
	defer unlock()
	status := keeperStatus{SessionID: id, State: "starting", PID: os.Getpid(), Realm: expected.RealmID}
	defer func() {
		switch status.State {
		case "needs_login", "needs_attention", "user_control", "session_changed", "credentials_unavailable":
		default:
			status.State = "stopped"
		}
		_ = writeKeeperFile("keepalive-status.json", status)
	}()
	delay := time.Duration(0)
	for keeperStillWanted(id) {
		var policy keeperPolicy
		if err = readKeeperFile("keepalive-policy.json", &policy); err != nil {
			return err
		}
		if policy.Interval < time.Minute || policy.Interval > 15*time.Minute {
			return fmt.Errorf("invalid stored keep-alive interval")
		}
		pctx, pcancel := context.WithTimeout(ctx, 2*time.Minute)
		status.Maintenance = nil
		pctx = client.WithMaintenanceObserver(pctx, func(event client.MaintenanceEvent) {
			if len(status.Maintenance) < 8 {
				status.Maintenance = append(status.Maintenance, event)
			}
			_ = writeKeeperFile("keepalive-status.json", status)
		})
		ok, httpStatus, state := maintainSession(pctx, expected, policy.AllowManaged)
		pcancel()
		status.LastCheck = time.Now().UTC()
		status.HTTPStatus = httpStatus
		status.State = state
		if state == "needs_login" || state == "needs_attention" || state == "user_control" || state == "session_changed" || state == "credentials_unavailable" {
			status.NextCheck = time.Time{}
			return nil
		}
		if ok {
			delay = policy.Interval
			if policy.AllowManaged {
				if tok, err := auth.Load(); err == nil && tok.SessionID == id {
					delay = nativeKeeperDelay(delay, tok, time.Now())
				}
			}
		} else if state == "native_extension_pending" {
			delay = policy.Interval
		} else {
			delay = min(max(delay, policy.Interval)*2, 30*time.Minute)
		}
		nextCheck := time.Now().Add(delay)
		status.NextCheck = nextCheck.UTC()
		if err = writeKeeperFile("keepalive-status.json", status); err != nil {
			return err
		}
		for time.Now().Before(nextCheck) {
			timer := time.NewTimer(min(time.Until(nextCheck), 30*time.Second))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			if !keeperStillWanted(id) {
				return nil
			}
		}
	}
	return nil
}

func newAuthKeepaliveCmd(flags *rootFlags) *cobra.Command {
	var id string
	cmd := &cobra.Command{Use: "keepalive", Short: "Maintain the current QBO session with read-only probes (foreground)", RunE: func(cmd *cobra.Command, args []string) error {
		if flags.dryRun {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"dry_run": true, "dial": false})
		}
		err := runKeeper(cmd.Context(), id)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}}
	cmd.Flags().StringVar(&id, "session-id", "", "internal pinned login generation")
	_ = cmd.Flags().MarkHidden("session-id")
	var interval time.Duration
	var managed bool
	start := &cobra.Command{Use: "start", Short: "Start one background keeper for the current login", RunE: func(cmd *cobra.Command, args []string) error {
		if flags.dryRun {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"dry_run": true, "dial": false})
		}
		action, err := startKeeper(interval, managed)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"action": action, "interval": interval.String()})
	}}
	start.Flags().DurationVar(&interval, "interval", keepaliveInterval, "read-only probe interval (1m–15m; default 5m)")
	start.Flags().BoolVar(&managed, "managed-refresh", true, "allow source-pinned warm refresh and enrolled autonomous login (never an interactive sign-in)")
	stop := &cobra.Command{Use: "stop", Short: "Stop background session maintenance (does not log out)", RunE: func(cmd *cobra.Command, args []string) error {
		if flags.dryRun {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"dry_run": true, "dial": false})
		}
		if err := stopKeeper(); err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"action": "stop_requested"})
	}}
	status := &cobra.Command{Use: "status", Short: "Read local keeper status without contacting QBO", RunE: func(cmd *cobra.Command, args []string) error {
		var p keeperPolicy
		var s keeperStatus
		_ = readKeeperFile("keepalive-policy.json", &p)
		_ = readKeeperFile("keepalive-status.json", &s)
		running := p.Enabled && p.SessionID == s.SessionID && keeperStillWanted(p.SessionID) && s.State != "stopped" && s.State != "needs_login" && s.State != "user_control" && time.Now().Before(s.NextCheck.Add(time.Minute)) && keeperLeaseHeld()
		out := map[string]any{"enabled": p.Enabled, "running": running, "state": s.State, "realm": s.Realm, "pid": s.PID, "last_check": s.LastCheck, "next_check": s.NextCheck, "http_status": s.HTTPStatus}
		if len(s.Maintenance) > 0 {
			out["maintenance"] = s.Maintenance
		}
		if tok, err := auth.Load(); err == nil && tok.SessionID == s.SessionID && tok.NativeTicket != nil {
			out["native_ticket"] = tok.NativeTicket
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
	}}
	cmd.AddCommand(start, stop, status)
	return cmd
}

// Login opts into maintenance by default; pure probes/status never start it.
func autoStartKeeper(cmd *cobra.Command, flags *rootFlags) string {
	if flags.dryRun || keeperHarness() || os.Getenv("QB_NO_KEEPALIVE") == "1" {
		return "disabled"
	}
	option := cmd.Flags().Lookup("keep-alive")
	if option == nil {
		return "disabled"
	}
	enabled, _ := cmd.Flags().GetBool("keep-alive")
	if !enabled {
		return "disabled"
	}
	exe, _ := os.Executable()
	if strings.HasSuffix(exe, ".test") || strings.HasSuffix(exe, ".test.exe") {
		return "disabled"
	}
	noOpen, _ := cmd.Flags().GetBool("no-open")
	action, err := startKeeper(keepaliveInterval, !noOpen)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "qb: session saved; keep-alive not started:", err)
		return "unavailable"
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "qb: session maintenance", action, "(qb auth keepalive status / stop)")
	return action
}
