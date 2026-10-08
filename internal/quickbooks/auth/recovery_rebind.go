package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"strings"
	"time"
)

// Rebinding is explicit and restricted to the same home, company and principal.
// Independent warm-source proof precedes any key or secret resolution.
func RebindLoginRecovery(ctx context.Context) error {
	if IsHarness() {
		return ErrRecoveryDisabled
	}
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return err
	}
	defer unlock()
	r, err := loadRecovery()
	if err != nil {
		return err
	}
	tok, err := Load()
	if err != nil {
		return err
	}
	if !r.Enabled || !SupportsNativeTicket(tok) || r.Realm != tok.RealmID || !strings.EqualFold(r.Email, tok.Email) {
		return ErrSessionChanged
	}
	cap, err := recoveryCapture(ctx, tok, nil)
	if err != nil {
		return err
	}
	if err = verifyRecoveryIdentity(tok, cap); err != nil {
		return err
	}
	if cap.Evidence == nil || !cap.Evidence.APIProof || !cap.Evidence.IdentityVerified {
		return ErrSessionIdentityUnverified
	}
	if err = SaveRenewedCapture(tok, cap, tok.Source); err != nil {
		return err
	}
	secrets, err := r.decrypt(ctx)
	if err != nil {
		return err
	}
	defer func() { secrets.Username, secrets.Password, secrets.TOTP = "", "", "" }()
	if !strings.EqualFold(secrets.Username, tok.Email) {
		return ErrSessionChanged
	}
	key, err := r.key(ctx, nil)
	if err != nil {
		return err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return ErrRecoveryKeyUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	lock, err := credentialLock(HomeDir())
	if err != nil {
		return err
	}
	defer lock()
	current, err := Load()
	record, rerr := loadRecovery()
	if err != nil || rerr != nil || !SameSession(tok, current) || tok.CredentialGeneration != current.CredentialGeneration || current.Source != tok.Source || current.EgoSpace != tok.EgoSpace || current.EgoTargetID != tok.EgoTargetID || record.ID != r.ID || !record.Enabled {
		return ErrSessionChanged
	}
	plain, err := json.Marshal(secrets)
	if err != nil {
		return err
	}
	defer clear(plain)
	secrets.Username, secrets.Password, secrets.TOTP = "", "", ""
	record.SessionID = current.SessionID
	record.Nonce = make([]byte, aead.NonceSize())
	if _, err = rand.Read(record.Nonce); err != nil {
		return err
	}
	record.Ciphertext = aead.Seal(nil, record.Nonce, plain, record.aad())
	record.State = recoveryEnrolled
	record.LastAttempt = time.Time{}
	record.LastEvidence = cap.Evidence
	return writeRecovery(record)
}
