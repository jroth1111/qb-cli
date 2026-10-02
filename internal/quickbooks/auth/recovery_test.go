package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func recoveryFixture(t *testing.T) (*TokenSet, RecoverySecrets) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("QB_HOME", home)
	t.Setenv("QB_NO_MANAGED", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	tok := &TokenSet{RealmID: "1", Email: "user@example.invalid", Source: "managed-profile", Authorization: "Intuit_APIKey fixture-old", CapturedAt: time.Now().Add(-time.Hour), AuditAuthorization: "Intuit_APIKey fixture-audit", URIHostHeaders: map[string]map[string]string{"old.api.intuit.com": {"Authorization": "Intuit_APIKey fixture-host"}}, Cookies: []Cookie{{Name: "old-only", Value: "fixture", Domain: "qbo.intuit.com", Path: "/"}}}
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	previous := recoveryCapture
	recoveryCapture = func(context.Context, *TokenSet, *RecoverySecrets) (*ATSCapture, error) {
		return fixtureRecoveryCapture(tok), nil
	}
	t.Cleanup(func() { recoveryCapture = previous })
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(dir, "key")
	if err = CreateRecoveryKeyFile(context.Background(), key); err != nil {
		info, _ := os.Stat(dir)
		t.Fatalf("key-create: %v (mode %v, owner type %T, accepted %v)", err, info.Mode(), info.Sys(), recoveryFileOwned(info))
	}
	secrets := RecoverySecrets{Username: "fixture-user", Password: "fixture-password-not-for-logs", TOTP: "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"}
	if err = EnrollLoginRecovery(context.Background(), secrets, RecoveryOptions{KeyFile: key}); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	return tok, secrets
}

func TestConcurrentRecoveryOnlySubmitsCredentialsOnce(t *testing.T) {
	_, _ = recoveryFixture(t)
	var full atomic.Int32
	var signedIn atomic.Bool
	recoveryCapture = func(_ context.Context, expected *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
		if secrets == nil && !signedIn.Load() {
			return nil, ErrRemintNeedsLogin
		}
		if secrets != nil {
			full.Add(1)
			signedIn.Store(true)
		}
		return fixtureRecoveryCapture(expected), nil
	}
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for range 4 {
		wg.Go(func() { failures <- RemintATS(context.Background()) })
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if full.Load() != 1 {
		t.Fatal("concurrent requests submitted login credentials more than once")
	}
}

func TestRecoveryRuntimeKeyPathCanUseSystemdDeliveredFile(t *testing.T) {
	_, _ = recoveryFixture(t)
	r, _ := loadRecovery()
	key, err := os.ReadFile(r.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	delivered := filepath.Join(dir, "delivered-key")
	if err = os.WriteFile(delivered, key, 0400); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QB_RECOVERY_KEY_FILE", delivered)
	if _, err = r.decrypt(context.Background()); err != nil {
		t.Fatal("systemd-style read-only credential file was not accepted")
	}
	alias := filepath.Join(dir, "alias")
	if err = os.Symlink(delivered, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QB_RECOVERY_KEY_FILE", alias)
	if _, err = r.decrypt(context.Background()); !errors.Is(err, ErrRecoveryKeyUnavailable) {
		t.Fatal("symlink runtime key accepted")
	}
}

func fixtureRecoveryCapture(tok *TokenSet) *ATSCapture {
	return &ATSCapture{Headers: map[string]string{"Authorization": "Intuit_APIKey fixture-new", "intuit-company-id": tok.RealmID}, Cookies: []Cookie{{Name: "new-only", Value: "fixture", Domain: "qbo.intuit.com", Path: "/"}}, Identity: Identity{Realm: tok.RealmID, Email: tok.Email, Company: "Fixture Company"}}
}

func TestRecoveryVaultEncryptionAndBinding(t *testing.T) {
	tok, secrets := recoveryFixture(t)
	r, err := loadRecovery()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(HomeDir(), recoveryFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{secrets.Username, secrets.Password, secrets.TOTP} {
		if bytes.Contains(b, []byte(v)) {
			t.Fatal("vault leaked secret plaintext")
		}
	}
	got, err := r.decrypt(context.Background())
	if err != nil || *got != secrets {
		t.Fatal("encrypted secrets did not round-trip")
	}
	status, err := LoginRecoveryStatus()
	if err != nil || !status.Enabled || !status.Bound || status.Provider != "key-file" {
		t.Fatal("enrollment status incorrect")
	}
	r.Email = "other@example.invalid"
	if _, err = r.decrypt(context.Background()); !errors.Is(err, errRecoveryInvalid) {
		t.Fatal("identity tamper accepted")
	}
	tok.SessionID = "other"
	if recoveryBound(r, tok) {
		t.Fatal("enrollment followed a new login")
	}
}

func TestRecoveryPrefersWarmSessionWithoutSecretResolution(t *testing.T) {
	tok, _ := recoveryFixture(t)
	previous := recoveryNativeKey
	recoveryNativeKey = func(context.Context, string, []byte) ([]byte, error) {
		t.Fatal("warm refresh resolved a key")
		return nil, nil
	}
	defer func() { recoveryNativeKey = previous }()
	r, _ := loadRecovery()
	r.Provider = "native"
	if err := writeRecovery(r); err != nil {
		t.Fatal(err)
	}
	if err := RemintATS(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, _ := Load()
	if next.SessionID != tok.SessionID || next.CredentialGeneration != 0 || next.AuditAuthorization == "" {
		t.Fatal("warm refresh replaced the login or discarded warm credentials")
	}
	r, _ = loadRecovery()
	if !r.LastAttempt.IsZero() {
		t.Fatal("warm session consumed a credential login attempt")
	}
}

func TestCredentialRecoveryFencesOldResponsesAndDropsOldKeys(t *testing.T) {
	tok, want := recoveryFixture(t)
	req := sessionTestRequest(t, tok)
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	full := 0
	recoveryCapture = func(_ context.Context, expected *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
		if secrets == nil {
			return nil, ErrRemintNeedsLogin
		}
		if *secrets != want {
			t.Fatal("resolved wrong credentials")
		}
		full++
		return fixtureRecoveryCapture(expected), nil
	}
	if err := RemintATS(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, _ := Load()
	if full != 1 || !SameSession(tok, next) || next.CredentialGeneration != 1 {
		t.Fatal("recovery did not preserve logical login and advance credentials")
	}
	if next.AuditAuthorization != "" || len(next.URIHostHeaders) != 0 || strings.Contains(CookieHeaderForURL(next.Cookies, req.URL, time.Now()), "old-only=") {
		t.Fatal("full login retained stale credential families")
	}
	if err := ObserveSessionResponse(req, &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"old-only=late; Path=/"}}}); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("late response crossed generation fence")
	}
	r, _ := loadRecovery()
	if r.State != "recovered" {
		t.Fatal("recovery status not committed")
	}
}

func TestRecoveryNeverResolvesCredentialsForUnknownFailures(t *testing.T) {
	for _, failure := range []error{errors.New("network unavailable"), ErrSessionChanged, ErrSessionIdentityUnverified, ErrRecoveryAttention} {
		t.Run(failure.Error(), func(t *testing.T) {
			tok, _ := recoveryFixture(t)
			recoveryCapture = func(context.Context, *TokenSet, *RecoverySecrets) (*ATSCapture, error) { return nil, failure }
			if err := RemintATS(context.Background()); !errors.Is(err, failure) {
				t.Fatal("changed the failure classification")
			}
			r, _ := loadRecovery()
			if !r.LastAttempt.IsZero() {
				t.Fatal("unknown failure attempted a credential login")
			}
			next, _ := Load()
			if next.Authorization != tok.Authorization {
				t.Fatal("failed recovery overwrote credentials")
			}
		})
	}
}

func TestRecoveryCancellationDisableLogoutAndIdentity(t *testing.T) {
	for _, scenario := range []string{"disable", "forget", "logout", "new-login", "wrong-company", "wrong-account", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			tok, _ := recoveryFixture(t)
			recoveryCapture = func(ctx context.Context, expected *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
				if secrets == nil {
					return nil, ErrRemintNeedsLogin
				}
				cap := fixtureRecoveryCapture(expected)
				switch scenario {
				case "disable":
					if err := DisableLoginRecovery(ctx, false); err != nil {
						t.Fatal(err)
					}
				case "forget":
					if err := DisableLoginRecovery(ctx, true); err != nil {
						t.Fatal(err)
					}
				case "logout":
					if err := Delete(); err != nil {
						t.Fatal(err)
					}
				case "new-login":
					if err := Save(&TokenSet{RealmID: "1", Email: expected.Email, Source: "managed-profile", Authorization: "Intuit_APIKey fixture-manual"}); err != nil {
						t.Fatal(err)
					}
				case "wrong-company":
					cap.Identity.Realm = "2"
				case "wrong-account":
					cap.Identity.Email = "other@example.invalid"
				case "cancel":
					return nil, context.Canceled
				}
				return cap, nil
			}
			err := RemintATS(context.Background())
			if err == nil {
				t.Fatal("unsafe recovery succeeded")
			}
			next, e := Load()
			if scenario == "logout" {
				if !errors.Is(e, ErrNoCredentials) {
					t.Fatal("recovery resurrected logout")
				}
			} else if scenario == "new-login" {
				if next.Authorization != "Intuit_APIKey fixture-manual" {
					t.Fatal("manual login overwritten")
				}
			} else if next.Authorization != tok.Authorization {
				t.Fatal("failed recovery changed credentials")
			}
		})
	}
}

func TestRecoveryAttemptsAreBounded(t *testing.T) {
	_, _ = recoveryFixture(t)
	full := 0
	recoveryCapture = func(context.Context, *TokenSet, *RecoverySecrets) (*ATSCapture, error) {
		return nil, ErrRemintNeedsLogin
	}
	base := recoveryCapture
	recoveryCapture = func(ctx context.Context, tok *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
		if secrets != nil {
			full++
			return nil, ErrRecoveryAttention
		}
		return base(ctx, tok, secrets)
	}
	if err := RemintATS(context.Background()); !errors.Is(err, ErrRecoveryAttention) {
		t.Fatal(err)
	}
	if err := RemintATS(context.Background()); !errors.Is(err, ErrRecoveryAttention) {
		t.Fatal("second login was not cooled down")
	}
	if full != 1 {
		t.Fatal("repeated credential login")
	}
}

func TestRecoveryFileAndHarnessSafety(t *testing.T) {
	_, _ = recoveryFixture(t)
	r, _ := loadRecovery()
	if err := os.Chmod(r.KeyFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.decrypt(context.Background()); !errors.Is(err, ErrRecoveryKeyUnavailable) {
		t.Fatal("public key file accepted")
	}
	if err := os.Chmod(r.KeyFile, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(r.KeyFile)
	if err := CreateRecoveryKeyFile(context.Background(), r.KeyFile); err == nil {
		t.Fatal("key overwrite accepted")
	}
	after, _ := os.ReadFile(r.KeyFile)
	if !bytes.Equal(before, after) {
		t.Fatal("key was overwritten")
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	if err := RemintATS(context.Background()); err == nil {
		t.Fatal("harness permitted renewal")
	}
	if err := DisableLoginRecovery(context.Background(), true); !errors.Is(err, ErrRecoveryDisabled) {
		t.Fatal("harness removed enrollment")
	}
}

func TestRecoveryKeyUnavailableDoesNotReachLogin(t *testing.T) {
	_, _ = recoveryFixture(t)
	r, _ := loadRecovery()
	if err := os.Remove(r.KeyFile); err != nil {
		t.Fatal(err)
	}
	recoveryCapture = func(_ context.Context, _ *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
		if secrets != nil {
			t.Fatal("login reached without its key")
		}
		return nil, ErrRemintNeedsLogin
	}
	if err := RemintATS(context.Background()); !errors.Is(err, ErrRecoveryKeyUnavailable) {
		t.Fatal("key failure not surfaced")
	}
}
