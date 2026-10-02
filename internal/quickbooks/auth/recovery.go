package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

var recoveryCapture = captureManagedRecovery

type recoveryGrantKey struct{}

func verifyRecoveryIdentity(expected *TokenSet, cap *ATSCapture) error {
	if expected == nil || expected.RealmID == "" || expected.Email == "" || cap == nil || !cap.HasKey() || cap.Identity.Realm != expected.RealmID || cap.Identity.Email == "" || !strings.EqualFold(cap.Identity.Email, expected.Email) {
		return ErrSessionChanged
	}
	if realm := headerGet(cap.Headers, "intuit-company-id"); realm != "" && realm != expected.RealmID {
		return ErrSessionChanged
	}
	if user := headerGet(expected.RequestHeaders, "intuit-user-id"); user != "" && headerGet(cap.Headers, "intuit-user-id") != user {
		return ErrSessionChanged
	}
	return nil
}

// recoverManagedLocked is called while the renewal lease is held. Only a
// confirmed sign-in wall after warm capture permits resolving login secrets.
func recoverManagedLocked(ctx context.Context, expected *TokenSet) error {
	if IsHarness() || expected.Source != "managed-profile" {
		return ErrRecoveryDisabled
	}
	r, err := loadRecovery()
	if err != nil {
		return err
	}
	if !r.Enabled {
		return ErrRecoveryDisabled
	}
	if !recoveryBound(r, expected) {
		return ErrSessionChanged
	}
	ctx = withManagedHeaded(ctx, r.Headed)
	cap, err := recoveryCapture(ctx, expected, nil)
	if err == nil {
		return SaveRenewedCapture(expected, cap, "managed-profile")
	}
	if !errors.Is(err, ErrRemintNeedsLogin) {
		return err
	}
	if r.State == recoveryNeedsAttention || r.State == recoveryIdentityMismatch {
		return ErrRecoveryAttention
	}
	lock, err := credentialLock(HomeDir())
	if err != nil {
		return err
	}
	current, loadErr := Load()
	fresh, recordErr := loadRecovery()
	if loadErr != nil || recordErr != nil || !SameSession(expected, current) || !fresh.Enabled || fresh.ID != r.ID || !recoveryBound(fresh, current) {
		lock()
		return ErrSessionChanged
	}
	if time.Since(fresh.LastAttempt) < recoveryCooldown {
		lock()
		return ErrRecoveryCooldown
	}
	fresh.LastAttempt = time.Now().UTC()
	fresh.State = recoveryRecovering
	err = writeRecovery(fresh)
	lock()
	if err != nil {
		return err
	}
	secrets, err := fresh.decrypt(ctx)
	if err != nil {
		recoveryOutcome(fresh, recoveryKeyUnavailable)
		return err
	}
	cap, err = recoveryCapture(context.WithValue(ctx, recoveryGrantKey{}, fresh.ID), expected, secrets)
	secrets.Username, secrets.Password, secrets.TOTP = "", "", ""
	if err != nil {
		recoveryOutcome(fresh, recoveryNeedsAttention)
		return err
	}
	if err = verifyRecoveryIdentity(expected, cap); err != nil {
		recoveryOutcome(fresh, recoveryIdentityMismatch)
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = saveRecoveredCapture(expected, fresh, cap); err != nil {
		return err
	}
	recoveryOutcome(fresh, recoveryRecovered)
	return nil
}

func recoveryOutcome(r *recoveryRecord, state recoveryState) {
	unlock, err := credentialLock(HomeDir())
	if err != nil {
		return
	}
	defer unlock()
	current, err := loadRecovery()
	if err == nil && current.ID == r.ID && current.Enabled && current.LastAttempt.Equal(r.LastAttempt) {
		current.State = state
		_ = writeRecovery(current)
	}
}

func saveRecoveredCapture(expected *TokenSet, record *recoveryRecord, cap *ATSCapture) error {
	unlock, err := credentialLock(HomeDir())
	if err != nil {
		return err
	}
	defer unlock()
	current, err := Load()
	r, rerr := loadRecovery()
	if err != nil || rerr != nil || !SameSession(expected, current) || !r.Enabled || r.ID != record.ID || !recoveryBound(r, current) {
		return ErrSessionChanged
	}
	if current.CredentialGeneration != expected.CredentialGeneration {
		return ErrSessionChanged
	}
	// Full login starts a new browser credential generation. Old cookie/audit/
	// service credentials must not survive or merge from late HTTP responses.
	next := &TokenSet{Version: CurrentVersion, SessionID: current.SessionID, CredentialGeneration: current.CredentialGeneration + 1, RealmID: current.RealmID, Email: current.Email, CompanyName: current.CompanyName, Source: "managed-profile", CapturedAt: time.Now().UTC(), LoginURL: DefaultLoginURL, FinalURL: BankingCaptureURL}
	if err = next.ApplyATSCapture(cap); err != nil {
		return err
	}
	next.ClassifyCookies()
	r.LastEvidence = cap.Evidence
	if err = writeRecovery(r); err != nil {
		return err
	}
	return saveAt(HomeDir(), next)
}
