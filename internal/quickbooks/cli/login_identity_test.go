package cli

import (
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
	"io"
	"testing"
)

func TestWeeklyReviewLoginSeparatesCompanyCredentials(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	old := &auth.TokenSet{RealmID: "111", Email: "old@example.test", Authorization: "Intuit_APIKey intuit_apikey=synthetic-old,intuit_apikey_version=1.0", AuditAuthorization: "old-audit", URIHostHeaders: map[string]map[string]string{"service.api.intuit.com": {"Authorization": "old-service", "intuit-realm-id": "111"}}}
	if err := auth.Save(old); err != nil {
		t.Fatal(err)
	}
	next := &auth.TokenSet{RealmID: "222", Email: "new@example.test", Authorization: "Intuit_APIKey intuit_apikey=synthetic-new,intuit_apikey_version=1.0"}
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := persistLogin(cmd, &rootFlags{asJSON: true}, next); err != nil {
		t.Fatal(err)
	}
	saved, err := auth.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.AuditAuthorization != "" || len(saved.URIHostHeaders) != 0 {
		t.Fatal("new company inherited old identity service/audit credentials")
	}
}

func TestLoginCredentialReuseRequiresPositiveSameIdentity(t *testing.T) {
	known := &auth.TokenSet{RealmID: "111", Email: "person@example.test"}
	for _, tc := range []struct {
		realm, email string
		same         bool
	}{
		{"111", " Person@Example.Test ", true},
		{"222", "person@example.test", false},
		{"111", "other@example.test", false},
		{"111", "", false},
		{"", "person@example.test", false},
	} {
		if got := sameLoginIdentity(known, &auth.TokenSet{RealmID: tc.realm, Email: tc.email}); got != tc.same {
			t.Errorf("identity check got=%v want=%v", got, tc.same)
		}
	}
}
