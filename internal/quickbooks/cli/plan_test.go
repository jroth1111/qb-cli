package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

// Kill denominator: 6.
// Rejects dry-run that: dials, skips validation, requires creds, leaks
// secrets, succeeds as a live stub, or leaves RULE_READ/AUDIT_LOG_READ blocked.

func runQB(t *testing.T, args ...string) (string, error, time.Duration) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	err := root.ExecuteContext(ctx)
	return out.String(), err, time.Since(start)
}

func decodePlan(t *testing.T, stdout string) planEnvelope {
	t.Helper()
	var env planEnvelope
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout)), &env); err != nil {
		t.Fatalf("decode plan: %v; stdout=%q", err, stdout)
	}
	return env
}

func TestDryRunExcludeEmitsPlanNoDial(t *testing.T) {
	stdout, err, elapsed := runQB(t, "feed", "txn", "update", "exclude", "--ids", "olb-1", "--dry-run", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("dry-run took %s — likely dialed", elapsed)
	}
	if err != nil {
		t.Fatalf("dry-run exclude: %v; stdout=%q", err, stdout)
	}
	env := decodePlan(t, stdout)
	if !env.DryRun || env.Dial {
		t.Fatalf("envelope %+v", env)
	}
	if env.Method != "POST" || !strings.Contains(env.URL, "excludeTransactions") {
		t.Fatalf("method/url %s %s", env.Method, env.URL)
	}
	if strings.Contains(stdout, "intuit_apikey") || strings.Contains(stdout, "Authorization") {
		t.Fatalf("secrets in stdout: %s", stdout)
	}
}

func TestDryRunExcludeEmptyIDsFails(t *testing.T) {
	_, err, elapsed := runQB(t, "feed", "txn", "update", "exclude", "--dry-run", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if err == nil || !errors.Is(err, client.ErrEmptyExcludeIDs) {
		t.Fatalf("got %v, want ErrEmptyExcludeIDs", err)
	}
}

func TestDryRunImportBlockedAccountFails(t *testing.T) {
	_, err, elapsed := runQB(t, "feed", "txn", "import", "--account-id", "204", "--description", "QB-CLI-TEST", "--amount", "0.01", "--dry-run", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if err == nil || !errors.Is(err, client.ErrImportBlockedAccount) {
		t.Fatalf("got %v, want ErrImportBlockedAccount", err)
	}
}

func TestDryRunStubInvoiceCreate(t *testing.T) {
	stdout, err, elapsed := runQB(t, "sales", "invoice", "create", "--customer", "1", "--line-items", "[{\"Amount\":1}]", "--dry-run", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if err != nil {
		t.Fatalf("dry-run create: %v; stdout=%q", err, stdout)
	}
	env := decodePlan(t, stdout)
	if env.Dial || !env.DryRun || env.Method != "POST" {
		t.Fatalf("envelope %+v", env)
	}
	if !strings.Contains(env.URL, "invoice") {
		t.Fatalf("url %q", env.URL)
	}
}

func TestDryRunStubFlagsAreIndependent(t *testing.T) {
	stdout, err, _ := runQB(t, "sales", "invoice", "create", "--customer", "Acme", "--line-items", "[{\"Amount\":1}]", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("dry-run stub: %v; stdout=%q", err, stdout)
	}
	env := decodePlan(t, stdout)
	if env.Flags["customer"] != "Acme" {
		t.Fatalf("customer=%q", env.Flags["customer"])
	}
	if env.Flags["line-items"] == "Acme" {
		t.Fatal("string flags shared a backing variable")
	}
}

func TestStubInvoiceCreateWithoutDryRunStillFails(t *testing.T) {
	_, err, elapsed := runQB(t, "sales", "invoice", "create", "--customer", "TEST", "--line-items", "[{\"Amount\":1}]", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if err == nil {
		t.Fatal("stub without --dry-run must not succeed")
	}
	if !errors.Is(err, client.ErrNoCredentials) && !errors.Is(err, client.ErrMutationNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestDryRunRegisterReadNoDial(t *testing.T) {
	stdout, err, elapsed := runQB(t, "accounting", "register", "get", "--dry-run", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if err != nil {
		t.Fatalf("dry-run register: %v; stdout=%q", err, stdout)
	}
	env := decodePlan(t, stdout)
	if env.Method != "GET" || !strings.Contains(env.URL, "register/transactions") || env.Dial {
		t.Fatalf("envelope %+v", env)
	}
}

func TestFeedRuleReadMissingCredentialsFails(t *testing.T) {
	_, err, elapsed := runQB(t, "feed", "rule", "get", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if !errors.Is(err, client.ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func TestInvoiceReadMissingCredentialsFails(t *testing.T) {
	_, err, elapsed := runQB(t, "sales", "invoice", "get", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if !errors.Is(err, client.ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func TestDryRunInvoiceReadNoDial(t *testing.T) {
	stdout, err, elapsed := runQB(t, "sales", "invoice", "get", "--dry-run", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if err != nil {
		t.Fatalf("%v %s", err, stdout)
	}
	env := decodePlan(t, stdout)
	if env.Dial || env.Method != "GET" || !strings.Contains(env.URL, "query") {
		t.Fatalf("%+v", env)
	}
}

func TestAuditLogReadMissingCredentialsFails(t *testing.T) {
	_, err, elapsed := runQB(t, "accounting", "audit-log", "get", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if !errors.Is(err, client.ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}
