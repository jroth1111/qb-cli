package auth

import (
	"errors"
	"testing"
)

func TestHasAuditAuthorizationUsesAuditSpecificCredential(t *testing.T) {
	const auditKey = "Intuit_APIKey intuit_apikey=audit-only,intuit_apikey_version=1.0"
	if (&TokenSet{Authorization: auditKey}).HasAuditAuthorization() {
		t.Fatal("banking ATS authorization must not count as audit authorization")
	}
	if !(&TokenSet{AuditAuthorization: auditKey}).HasAuditAuthorization() {
		t.Fatal("dedicated audit authorization was not detected")
	}
	if !(&TokenSet{URIHostHeaders: map[string]map[string]string{"audit.api.intuit.com": {"Authorization": auditKey}}}).HasAuditAuthorization() {
		t.Fatal("captured audit host header was not detected")
	}
}

// TestApplyATSCaptureAuditMerge verifies the AuditAuthorization merge rule:
// a non-empty audit key overwrites the previous one; an empty audit key
// preserves any existing AuditAuthorization so a banking-only remint never
// wipes a known-good audit key.
func TestApplyATSCaptureAuditMerge(t *testing.T) {
	const bankingKey = "Intuit_APIKey intuit_apikey=banking,intuit_apikey_version=1.0"
	const oldAuditKey = "Intuit_APIKey intuit_apikey=old-audit,intuit_apikey_version=1.0"
	const newAuditKey = "Intuit_APIKey intuit_apikey=new-audit,intuit_apikey_version=1.0"

	t.Run("new audit key overwrites previous", func(t *testing.T) {
		tok := &TokenSet{
			Authorization:      bankingKey,
			AuditAuthorization: oldAuditKey,
		}
		err := tok.ApplyATSCapture(&ATSCapture{
			Headers:            map[string]string{"Authorization": bankingKey},
			AuditAuthorization: newAuditKey,
		})
		if err != nil {
			t.Fatal(err)
		}
		if tok.AuditAuthorization != newAuditKey {
			t.Fatalf("audit key: got len=%d, want len=%d (new should overwrite)", len(tok.AuditAuthorization), len(newAuditKey))
		}
		if tok.Authorization != bankingKey {
			t.Fatal("banking Authorization must not change")
		}
	})

	t.Run("empty audit key preserves existing", func(t *testing.T) {
		tok := &TokenSet{
			Authorization:      bankingKey,
			AuditAuthorization: oldAuditKey,
		}
		err := tok.ApplyATSCapture(&ATSCapture{
			Headers:            map[string]string{"Authorization": bankingKey},
			AuditAuthorization: "",
		})
		if err != nil {
			t.Fatal(err)
		}
		if tok.AuditAuthorization != oldAuditKey {
			t.Fatalf("audit key: got len=%d, want len=%d (empty must preserve existing)", len(tok.AuditAuthorization), len(oldAuditKey))
		}
		if tok.Authorization != bankingKey {
			t.Fatal("banking Authorization must not change")
		}
	})

	t.Run("first capture with no audit key leaves empty", func(t *testing.T) {
		tok := &TokenSet{}
		err := tok.ApplyATSCapture(&ATSCapture{
			Headers:            map[string]string{"Authorization": bankingKey},
			AuditAuthorization: "",
		})
		if err != nil {
			t.Fatal(err)
		}
		if tok.AuditAuthorization != "" {
			t.Fatalf("audit key should be empty, got len=%d", len(tok.AuditAuthorization))
		}
		if !tok.HasATSAuthorization() {
			t.Fatal("banking ATS authorization must be set")
		}
	})
}

// TestApplyATSCaptureEmptyATSAuthorizationErrors is the negative case: an
// empty ATS banking Authorization must still return ErrNoATSAuthorization,
// even when an audit key is present. The audit key alone is not a valid
// banking session.
func TestApplyATSCaptureEmptyATSAuthorizationErrors(t *testing.T) {
	const auditKey = "Intuit_APIKey intuit_apikey=audit-only,intuit_apikey_version=1.0"

	tok := &TokenSet{
		Authorization:      "Intuit_APIKey intuit_apikey=existing,intuit_apikey_version=1.0",
		AuditAuthorization: auditKey,
	}
	err := tok.ApplyATSCapture(&ATSCapture{
		Headers:            map[string]string{"Accept": "*/*"},
		AuditAuthorization: auditKey,
	})
	if !errors.Is(err, ErrNoATSAuthorization) {
		t.Fatalf("got %v, want ErrNoATSAuthorization", err)
	}
	// Existing banking key must not be wiped by a failed capture.
	if tok.Authorization == "" {
		t.Fatal("existing banking Authorization must survive failed capture")
	}
}

// TestApplyAndSaveCapturePreservesAuditKey verifies that saveCaptureForAuditTest
// does not wipe an existing AuditAuthorization when the new capture lacks one.
func TestApplyAndSaveCapturePreservesAuditKey(t *testing.T) {
	setQBHome(t)
	const bankingKey = "Intuit_APIKey intuit_apikey=banking,intuit_apikey_version=1.0"
	const oldAuditKey = "Intuit_APIKey intuit_apikey=old-audit,intuit_apikey_version=1.0"
	const newBankingKey = "Intuit_APIKey intuit_apikey=banking2,intuit_apikey_version=1.0"

	if err := Save(&TokenSet{
		Version: CurrentVersion,
		RealmID: "1", Email: "person@example.test",
		Authorization:      bankingKey,
		AuditAuthorization: oldAuditKey,
		Cookies:            []Cookie{{Name: "qbo.ticket", Value: "v", Domain: ".qbo.intuit.com", Path: "/"}},
	}); err != nil {
		t.Fatal(err)
	}
	// New capture has a fresh banking key but NO audit key.
	if err := saveCaptureForAuditTest(&ATSCapture{
		Headers:            map[string]string{"Authorization": newBankingKey},
		AuditAuthorization: "",
	}, "relay-session"); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Authorization != newBankingKey {
		t.Fatalf("banking key: got len=%d, want len=%d", len(got.Authorization), len(newBankingKey))
	}
	if got.AuditAuthorization != oldAuditKey {
		t.Fatalf("audit key: got len=%d, want len=%d (must preserve existing)", len(got.AuditAuthorization), len(oldAuditKey))
	}
}

// TestApplyAndSaveCaptureOverwritesAuditKey verifies that a new non-empty
// audit key overwrites the previous one through the full save/load cycle.
func TestApplyAndSaveCaptureOverwritesAuditKey(t *testing.T) {
	setQBHome(t)
	const bankingKey = "Intuit_APIKey intuit_apikey=banking,intuit_apikey_version=1.0"
	const oldAuditKey = "Intuit_APIKey intuit_apikey=old-audit,intuit_apikey_version=1.0"
	const newAuditKey = "Intuit_APIKey intuit_apikey=new-audit,intuit_apikey_version=1.0"

	if err := Save(&TokenSet{
		Version: CurrentVersion,
		RealmID: "1", Email: "person@example.test",
		Authorization:      bankingKey,
		AuditAuthorization: oldAuditKey,
		Cookies:            []Cookie{{Name: "qbo.ticket", Value: "v", Domain: ".qbo.intuit.com", Path: "/"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveCaptureForAuditTest(&ATSCapture{
		Headers:            map[string]string{"Authorization": bankingKey},
		AuditAuthorization: newAuditKey,
	}, "relay-session"); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AuditAuthorization != newAuditKey {
		t.Fatalf("audit key: got len=%d, want len=%d (new should overwrite)", len(got.AuditAuthorization), len(newAuditKey))
	}
}

// TestRedactAuthKey verifies the redaction helper never leaks key material.
func TestRedactAuthKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "none"},
		{"whitespace", "   ", "none"},
		{"intuit key", "Intuit_APIKey intuit_apikey=secret,intuit_apikey_version=1.0", "Intuit_APIKey(len=60)"},
		{"other auth", "Bearer abc123", "auth(len=13)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redactAuthKey(c.in)
			if got != c.want {
				t.Fatalf("redactAuthKey: got %q, want %q", got, c.want)
			}
			// Never include the input secret in the redacted output.
			if c.in != "" && c.in != "   " && containsStr(got, c.in) {
				t.Fatalf("redacted output leaked input: %q contains %q", got, c.in)
			}
		})
	}
}

func containsStr(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func saveCaptureForAuditTest(cap *ATSCapture, source string) error {
	expected, err := Load()
	if err != nil {
		return err
	}
	cap.Identity = Identity{Realm: expected.RealmID, Email: expected.Email}
	return SaveRenewedCapture(expected, cap, source)
}
