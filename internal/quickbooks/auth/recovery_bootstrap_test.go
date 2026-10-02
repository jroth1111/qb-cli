package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapVerifiesLoginAndEnrollsWithoutManualSession(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("QB_HOME", home)
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "key")
	if err := CreateRecoveryKeyFile(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	oldBootstrap, oldCapture := bootstrapCapture, recoveryCapture
	defer func() { bootstrapCapture = oldBootstrap; recoveryCapture = oldCapture }()
	cap := fixtureRecoveryCapture(&TokenSet{RealmID: "1", Email: "fixture@example.invalid"})
	cap.Evidence = &LoginEvidence{CredentialPurposes: []string{"username", "password", "totp"}, IdentityVerified: true, APIProof: true}
	bootstrapCapture = func(context.Context, RecoverySecrets) (*ATSCapture, error) { return cap, nil }
	recoveryCapture = func(context.Context, *TokenSet, *RecoverySecrets) (*ATSCapture, error) { return cap, nil }
	secrets := RecoverySecrets{Username: "fixture@example.invalid", Password: "fixture-private"}
	if err := EnrollLoginRecovery(context.Background(), secrets, RecoveryOptions{KeyFile: key, Bootstrap: true, TestProfile: true, Headed: true}); err != nil {
		t.Fatal(err)
	}
	status, err := LoginRecoveryStatus()
	if err != nil || !status.Enabled || !status.Bound || status.LastEvidence == nil || len(status.LastEvidence.CredentialPurposes) != 3 {
		t.Fatal("bootstrap lacked verified evidence")
	}
	r, _ := loadRecovery()
	if !r.TestProfile || !r.Headed {
		t.Fatal("test/headed profile policy not persisted")
	}
	if err := EnrollLoginRecovery(context.Background(), secrets, RecoveryOptions{KeyFile: key, Bootstrap: true}); err == nil {
		t.Fatal("bootstrap replaced an existing login")
	}
}

func TestBootstrapFailureNeverCreatesStoredSession(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	old := bootstrapCapture
	defer func() { bootstrapCapture = old }()
	bootstrapCapture = func(context.Context, RecoverySecrets) (*ATSCapture, error) { return nil, ErrRecoveryAttention }
	if err := EnrollLoginRecovery(context.Background(), RecoverySecrets{Username: "fixture", Password: "fixture"}, RecoveryOptions{Bootstrap: true}); !errors.Is(err, ErrRecoveryAttention) {
		t.Fatal("bootstrap failure not propagated")
	}
	if _, err := Load(); !errors.Is(err, ErrNoCredentials) {
		t.Fatal("failed bootstrap created session")
	}
}
