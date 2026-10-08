package auth

import (
	"context"
	"errors"
	"testing"
)

func TestRecoveryRebindPreservesSecretsAndRequiresIndependentSameIdentity(t *testing.T) {
	for _, scenario := range []string{"ok", "other-company", "other-principal", "no-proof", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			tok, secrets := recoveryFixture(t)
			before, _ := loadRecovery()
			tok.Source, tok.EgoSpace, tok.EgoTargetID, tok.SessionID = "ego-existing", "52", "p1", "new-fixture-session"
			if scenario == "other-company" {
				tok.RealmID = "other"
			}
			if scenario == "other-principal" {
				tok.Email = "other@example.invalid"
			}
			if err := Save(tok); err != nil {
				t.Fatal(err)
			}
			if scenario == "disabled" {
				if err := DisableLoginRecovery(context.Background(), false); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			recoveryCapture = func(_ context.Context, expected *TokenSet, entered *RecoverySecrets) (*ATSCapture, error) {
				calls++
				if entered != nil {
					t.Fatal("rebind submitted credentials")
				}
				cap := fixtureRecoveryCapture(expected)
				if scenario != "no-proof" {
					cap.Evidence = &LoginEvidence{APIProof: true, IdentityVerified: true}
				}
				return cap, nil
			}
			err := RebindLoginRecovery(context.Background())
			if scenario != "ok" {
				if err == nil {
					t.Fatal("unsafe rebind accepted")
				}
				after, _ := loadRecovery()
				if after.SessionID != before.SessionID || string(after.Ciphertext) != string(before.Ciphertext) {
					t.Fatal("failed rebind changed encryption")
				}
				if scenario != "no-proof" && calls != 0 {
					t.Fatal("wrong/disabled identity contacted browser")
				}
				if scenario == "no-proof" && !errors.Is(err, ErrSessionIdentityUnverified) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after, _ := loadRecovery()
			current, _ := Load()
			if !recoveryBound(after, current) || after.ID != before.ID {
				t.Fatal("binding/key identity changed incorrectly")
			}
			decoded, err := after.decrypt(context.Background())
			if err != nil || *decoded != secrets {
				t.Fatal("rebind lost secret contents")
			}
		})
	}
}

func TestEgoRecoveryFencesCredentialGenerationAndPreservesPinnedSource(t *testing.T) {
	tok, _ := recoveryFixture(t)
	tok.Source, tok.EgoSpace, tok.EgoTargetID = "ego-existing", "52", "p1"
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	tok, _ = Load()
	calls := 0
	recoveryCapture = func(_ context.Context, expected *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
		if secrets == nil {
			return nil, ErrRemintNeedsLogin
		}
		calls++
		return fixtureRecoveryCapture(expected), nil
	}
	if err := RemintATS(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := Load()
	if calls != 1 || after.Source != "ego-existing" || after.EgoSpace != "52" || after.EgoTargetID != "p1" || after.CredentialGeneration != tok.CredentialGeneration+1 {
		t.Fatal("recovery switched source or did not fence old headers")
	}
}

func TestWarmRecoveryVerificationRequiresActualProof(t *testing.T) {
	for _, proved := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "proved"}[proved], func(t *testing.T) {
			recoveryFixture(t)
			recoveryCapture = func(_ context.Context, expected *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
				if secrets != nil {
					t.Fatal("warm verification resolved credentials")
				}
				cap := fixtureRecoveryCapture(expected)
				if proved {
					cap.Evidence = &LoginEvidence{APIProof: true, IdentityVerified: true}
				}
				return cap, nil
			}
			result, err := VerifyLoginRecovery(context.Background(), false)
			if proved && (err != nil || !result.OK || !result.IndependentReadback || !result.IdentityPreserved) {
				t.Fatal("valid warm proof rejected")
			}
			if !proved && (err == nil || result.OK) {
				t.Fatal("missing warm proof claimed success")
			}
		})
	}
}
