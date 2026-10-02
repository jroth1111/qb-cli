package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// managedDisableEnv is the kill-switch for headless managed refresh
// (CI, or operators who never want the CLI launching a browser).
const managedDisableEnv = "QB_NO_MANAGED"

// managedRemintCeiling bounds a headless refresh: warm profile loads,
// banking fires ATS, done. A cold profile sits at the sign-in wall until
// the auth wait gives up — never long enough to matter interactively.
const managedRemintCeiling = 100 * time.Second

// RemintManaged refreshes the stored session headlessly from the persistent
// profile. No window appears and no clicks are needed when the profile
// session is warm; a cold profile fails fast instead of waiting for a user
// who isn't there. Merge rules mirror the relay refresh: a freshly
// intercepted ATS key wins, otherwise fresh cookies join the stored key.
func RemintManaged(ctx context.Context) error {
	if IsHarness() {
		return ErrRemintNeedsLogin
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, managedRemintCeiling)
		defer cancel()
	}
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return err
	}
	defer unlock()
	return remintManagedLocked(ctx)
}

func remintManagedLocked(ctx context.Context) error {
	if IsHarness() {
		return ErrRemintNeedsLogin
	}
	if os.Getenv(managedDisableEnv) == "1" {
		return fmt.Errorf("managed refresh disabled via %s", managedDisableEnv)
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
	if r, rerr := loadRecovery(); rerr == nil && r.Enabled {
		return recoverManagedLocked(ctx, expected)
	} else if rerr != nil && !errors.Is(rerr, ErrRecoveryDisabled) {
		return rerr
	}
	cap, err := captureManagedHeadless(ctx, BankingCaptureURL)
	if err != nil {
		return err
	}
	return SaveRenewedCapture(expected, cap, "managed-profile")
}

// HasKey reports whether the capture holds a live ATS Authorization.
func (c *ATSCapture) HasKey() bool {
	return c != nil && isIntuitAPIKey(headerGet(c.Headers, "authorization"))
}
