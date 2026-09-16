package gql

import (
	"context"
	"errors"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// TestExecuteNoCompanyGate proves Execute applies no company gate: every
// session realm delegates to the ego executor. The stub proves delegation
// happened (instead of a company refusal) without launching a browser.
func TestExecuteNoCompanyGate(t *testing.T) {
	sentinel := errors.New("stub executor reached")
	orig := executeEgoFn
	executeEgoFn = func(context.Context, Request) (*Response, error) {
		return nil, sentinel
	}
	t.Cleanup(func() { executeEgoFn = orig })
	for _, realm := range []string{"12345", "99999"} {
		t.Setenv("QB_HOME", t.TempDir())
		if err := auth.Save(&auth.TokenSet{
			Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			RealmID:       realm,
		}); err != nil {
			t.Fatal(err)
		}
		op, err := Lookup("GetBills")
		if err != nil {
			t.Fatal(err)
		}
		// Delegation (sentinel), never a company refusal, never nil.
		if _, err := Execute(context.Background(), Request{Op: op}); !errors.Is(err, sentinel) {
			t.Fatalf("realm %s: err = %v, want stub delegation", realm, err)
		}
	}
}
