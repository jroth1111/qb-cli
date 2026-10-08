package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"
)

// RemintTimeout is the ceiling for automatic 401 remint when the caller
// supplies no deadline. It fits the whole ladder (relay + headless managed
// refresh); only a 401 pays it, and success heals silently.
const RemintTimeout = 12 * time.Second

func IsHarness() bool {
	return os.Getenv("PRINTING_PRESS_VERIFY") == "1" || os.Getenv("PRINTING_PRESS_DOGFOOD") == "1"
}

// AuditCaptureTimeout bounds the best-effort audit-ui key capture that runs
// after a successful ATS banking capture. It is shorter than RemintTimeout so
// the audit pass can never block a `qb feed` remint past its own deadline.
const AuditCaptureTimeout = 8 * time.Second

// ErrRemintNeedsLogin means no authenticated QBO tab was available for a
// fast remint. The user must run `qb auth remint` or `qb login`.
var ErrRemintNeedsLogin = errors.New("no authenticated QBO tab; run `qb auth remint` or `qb login`")

// TryRelayRemint recaptures ATS headers from an already-authenticated
// OMP-relay tab. It does not open ego.
func TryRelayRemint(ctx context.Context) error {
	return remintFromRelay(ctx, resolveRelayURL())
}

// RemintATS recaptures ATS Intuit_APIKey headers. Automatic on every 401 —
// no flag needed (QB_NO_MANAGED=1 opts out of the managed rung only).
//
// Ladder:
//  1. OMP relay, if an authenticated qbo.intuit.com/app/* tab exists.
//  2. Headless managed-profile refresh (silent when warm).
//  3. ego Space, only when the context deadline is longer than
//     RemintTimeout (explicit remint / login).
func RemintATS(ctx context.Context) error {
	if IsHarness() {
		return fmt.Errorf("%w: browser renewal disabled under verification harness", ErrRemintNeedsLogin)
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, managedRemintCeiling)
		defer cancel()
	}
	expected, err := Load()
	if err != nil {
		return err
	}
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return err
	}
	defer unlock()
	current, err := Load()
	if err != nil || !SameSession(expected, current) {
		return ErrSessionChanged
	}
	if current.CapturedAt.After(expected.CapturedAt) {
		return nil
	}
	// Managed sessions never switch to an unrelated browser during recovery.
	if current.Source == "managed-profile" {
		return remintManagedLocked(ctx)
	}
	// A captured Ego session must renew from that same existing source. Trying
	// an unrelated managed profile first exhausts the deadline and never reaches
	// the authenticated browser. Never open login or claim a user-controlled tab.
	if current.EgoSpace != "" || current.Source == "ego-existing" {
		if record, err := loadRecovery(); err == nil && record.Enabled && recoveryBound(record, current) {
			return recoverManagedLocked(ctx, current)
		}
		ectx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		space := current.EgoSpace
		if space == "" {
			space = "qb-gql"
		} // legacy existing-Ego captures
		cap, err := captureExistingEgo(WithExistingEgo(ectx, space, current.EgoTargetID), BankingCaptureURL, BankingCaptureURL)
		if err != nil {
			return fmt.Errorf("existing Ego session renewal failed (no browser fallback): %w", err)
		}
		return SaveRenewedCapture(current, cap, "ego-existing")
	}

	relayErr := remintFromRelay(ctx, resolveRelayURL())
	if relayErr == nil {
		return nil
	}

	// Headless managed refresh: silent when the profile session is warm,
	// fast failure when it needs a human. Runs before ego because ego
	// needs an interactive Space the remint deadline cannot accommodate.
	if allowManagedRemint(ctx) {
		if err := remintManagedLocked(ctx); err == nil {
			return nil
		}
	}

	if allowEgoRemint(ctx) {
		if err := remintFromEgo(ctx); err == nil {
			return nil
		} else {
			return fmt.Errorf("remint: relay: %v; ego: %w", relayErr, err)
		}
	}
	if errors.Is(relayErr, ErrRemintNeedsLogin) {
		return relayErr
	}
	return fmt.Errorf("remint: %w", relayErr)
}

var captureExistingEgo = CaptureATSFromEgo

// allowManagedRemint gates the headless refresh: enough deadline left to
// matter (a warm refresh takes ~30s) and not explicitly disabled.
func allowManagedRemint(ctx context.Context) bool {
	if os.Getenv(managedDisableEnv) == "1" {
		return false
	}
	dl, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(dl) > 60*time.Second
}

func allowEgoRemint(ctx context.Context) bool {
	dl, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(dl) > RemintTimeout+2*time.Second
}

func remintFromRelay(ctx context.Context, relayURL string) error {
	expected, err := Load()
	if err != nil {
		return err
	}
	tabs, err := listRelayPages(ctx, relayURL)
	if err != nil {
		return fmt.Errorf("relay list: %w", err)
	}
	tab := findRelayAuthTab(tabs)
	if tab == nil {
		return ErrRemintNeedsLogin
	}
	cap, err := CaptureATSFromRelay(ctx, relayURL, tab.ID, BankingCaptureURL)
	if err != nil {
		return fmt.Errorf("relay capture: %w", err)
	}
	// Best-effort audit-ui key capture. The Audit Log UI sends a shorter
	// Intuit_APIKey to audit.api.intuit.com; capture it in the same remint
	// pass so `accounting audit-log read` works without a separate login.
	// Failure must not fail remint — the banking ATS key is the hard
	// requirement — and an empty audit key preserves any existing one
	// via ApplyATSCapture's merge rule.
	if cap.AuditAuthorization == "" {
		cap.AuditAuthorization = captureAuditBestEffort(relayURL, tab.ID)
	}
	return SaveRenewedCapture(expected, cap, "relay-session")
}

// captureAuditBestEffort navigates the audit log UI and returns its
// Intuit_APIKey, or "" with a non-secret warning when the audit capture
// fails. It never returns an error: callers treat audit as optional.
func captureAuditBestEffort(relayURL, tabID string) string {
	actx, cancel := context.WithTimeout(context.Background(), AuditCaptureTimeout)
	defer cancel()
	auditKey, aerr := CaptureAuditFromRelay(actx, relayURL, tabID, AuditCaptureURL)
	if aerr != nil || auditKey == "" {
		log.Printf("remint: audit-ui key capture skipped (%v); banking ATS key still saved, existing audit key preserved", aerr)
		return ""
	}
	log.Printf("remint: captured audit-ui key %s", redactAuthKey(auditKey))
	return auditKey
}

func remintFromEgo(ctx context.Context) error {
	expected, err := Load()
	if err != nil {
		return err
	}
	cap, err := CaptureATSFromEgo(ctx, DefaultLoginURL, BankingCaptureURL)
	if err != nil {
		return err
	}
	return SaveRenewedCapture(expected, cap, "ego-space")
}
