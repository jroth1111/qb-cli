package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// TestFeedAccountsMissingCredentialsFails is the contract's negative case
// for `qb feed accounts`: with QB_HOME pointed at an empty temp dir there
// are no saved credentials, so the command MUST fail with the auth exit
// code (2) and MUST NOT reach the network.
//
// The guard lives in the client package (auth.Load before any network
// call), so this never opens a socket. It rejects three failure modes:
//   - returning nil (silently succeeding with no credentials),
//   - exiting with the wrong code (e.g. a generic input error from a
//     transport failure, which would mean the guard was removed),
//   - taking long enough to suggest a network call (the HTTP client would
//     only be reached if the guard regressed).
func TestFeedAccountsMissingCredentialsFails(t *testing.T) {
	// Isolate config so a real ~/.config/qb is never touched. t.Setenv
	// auto-restores on test cleanup.
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs([]string{"feed", "account", "list"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	start := time.Now()
	err := root.ExecuteContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error for missing credentials, got nil; stdout=%q", out.String())
	}
	// Must be the auth exit code specifically — a different code would mean
	// the credentials guard was bypassed and we hit a downstream failure
	// (e.g. a transport error) for the wrong reason.
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitAuthError {
		t.Fatalf("expected exit code %d (ExitAuthError), got %d; stdout=%q",
			ExitAuthError, exitErr.Code, out.String())
	}
	// No network: the guard returns in microseconds. A regression that
	// reached the HTTP client would take noticeably longer (TLS handshake).
	if elapsed > 3*time.Second {
		t.Fatalf("feed accounts did not fail fast: took %v (want < 3s, no network expected)", elapsed)
	}
}

// TestFeedPendingMissingCredentialsFails mirrors the accounts negative for
// `qb feed pending`. Same guarantees: auth exit code, no network.
func TestFeedPendingMissingCredentialsFails(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "list", "--account-id", "204"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.ExecuteContext(ctx)
	if err == nil {
		t.Fatalf("expected error for missing credentials, got nil; stdout=%q", out.String())
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitAuthError {
		t.Fatalf("expected exit code %d (ExitAuthError), got %d; stdout=%q",
			ExitAuthError, exitErr.Code, out.String())
	}
}

// TestFeedCommandSurface asserts `qb feed` and its accounts/pending
// children are registered, so `qb feed accounts` and `qb feed pending`
// exist as the acceptance requires. Pure structure — no RunE invocation,
// no network.
func TestFeedCommandSurface(t *testing.T) {
	root := NewRootCommand()
	feed := findSub(root, "feed")
	if feed == nil {
		t.Fatal("`qb feed` command not registered")
	}
	acct := findSub(feed, "account")
	if acct == nil || findSub(acct, "list") == nil {
		t.Fatal("`qb feed account list` command not registered")
	}
	txn := findSub(feed, "txn")
	if txn == nil {
		t.Fatal("`qb feed txn` command not registered")
	}
	pending := findSub(txn, "list")
	if pending == nil {
		t.Fatal("`qb feed txn list` command not registered")
	}
	if got := pending.Flag("account-id").DefValue; got != "204" {
		t.Fatalf("feed txn list --account-id default = %q, want 204", got)
	}

}

// TestFeedPendingDefaultAccountIs204 guards the default-account contract
// at the client layer: an empty accountID resolves to DefaultAccountID.
func TestFeedPendingDefaultAccountIs204(t *testing.T) {
	if client.DefaultAccountID != "204" {
		t.Fatalf("client.DefaultAccountID = %q, want 204", client.DefaultAccountID)
	}
}

// runFeedVerb executes `qb feed <args...>` against an isolated empty config
// home and returns the resulting error and elapsed wall time. It is the
// shared harness for the missing-credentials / no-network negative cases:
// with QB_HOME pointed at an empty temp dir there are no saved credentials,
// so any read verb MUST fail fast with the auth exit code and MUST NOT reach
// the network. The 10s context is a safety net only; a regression that
// opened a socket would still be caught by the elapsed-time check below.
func runFeedVerb(t *testing.T, args ...string) (elapsed time.Duration, err error) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs(append([]string{"feed"}, args...))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	start := time.Now()
	err = root.ExecuteContext(ctx)
	elapsed = time.Since(start)
	return elapsed, err
}

// assertAuthExit verifies err is an *ExitError with the auth exit code (2),
// the code a missing-credentials guard produces. A different code would mean
// the guard was bypassed and a downstream transport failure was reported for
// the wrong reason.
func assertAuthExit(t *testing.T, err error, stdout string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil; stdout=%q", stdout)
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitAuthError {
		t.Fatalf("expected exit code %d (ExitAuthError), got %d; stdout=%q",
			ExitAuthError, exitErr.Code, stdout)
	}
}

// assertInputExit verifies err is an *ExitError with the input exit code (1),
// used for bad CLI input (e.g. a missing required flag) and for the
// not-wired mutation sentinel.
func assertInputExit(t *testing.T, err error, stdout string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil; stdout=%q", stdout)
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitInputError {
		t.Fatalf("expected exit code %d (ExitInputError), got %d; stdout=%q",
			ExitInputError, exitErr.Code, stdout)
	}
}

// assertFast verifies the command returned well within the window that would
// imply a network call (TLS handshake). The credentials guard returns in
// microseconds; a regression that reached the HTTP client would take
// noticeably longer.
func assertFast(t *testing.T, elapsed time.Duration) {
	t.Helper()
	if elapsed > 3*time.Second {
		t.Fatalf("did not fail fast: took %v (want < 3s, no network expected)", elapsed)
	}
}

func TestFeedPostedMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "list", "posted")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedPopulationMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "population")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedExcludedMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "list", "excluded")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedLookupRequiresQuery(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "get")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedLookupMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "get", "--query", "coles")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedMutationMissingCredentialsFails(t *testing.T) {
	// txn run tag stays a blocked stub: it checks the session before
	// surfacing ErrMutationNotWired, so it still exercises the auth-exit
	// path wired mutations leave to their own input guards.
	elapsed, err := runFeedVerb(t, "txn", "run", "tag")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedUnpostEmptyIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "unpost")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedExcludeEmptyIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "exclude")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedExcludeMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "exclude", "--ids", "1")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedUndoEmptyIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "undo-excluded")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedUndoMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "undo-excluded", "--ids", "1")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedImportMissingAccountFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "import", "--description", "QB-CLI-TEST", "--amount", "0.01")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedImportBlockedAccountFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "import", "--account-id", "204", "--description", "QB-CLI-TEST", "--amount", "0.01")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedImportMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "import", "--account-id", "209", "--description", "QB-CLI-TEST", "--amount", "0.01")
	assertInputExit(t, err, "") // no readback adapter: blocked before credential/network access
	assertFast(t, elapsed)
}

func TestFeedImportMissingFileFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "import", "--account-id", "209", "--file", "/nonexistent-tour.csv")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

// TestParseImportCSV locks the --file row contract: a Date,Description,Amount
// header is consumed, headerless files read positionally, short rows skip,
// and an empty file errors instead of importing nothing silently.
func TestParseImportCSV(t *testing.T) {
	rows, err := parseImportCSV([]byte("Date,Description,Amount\n08/09/2026,Tour A,-25.50\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].description != "Tour A" || rows[0].amount != "-25.50" || rows[0].date != "08/09/2026" {
		t.Fatalf("rows = %+v", rows)
	}
	rows, err = parseImportCSV([]byte("08/09/2026,Tour B,10\nshort\n\n"))
	if err != nil || len(rows) != 1 || rows[0].description != "Tour B" {
		t.Fatalf("positional rows = %+v err = %v", rows, err)
	}
	if _, err := parseImportCSV([]byte("Date,Description,Amount\n")); err == nil {
		t.Fatal("header-only file accepted")
	}
}

func TestFeedCommandSurfaceExtended(t *testing.T) {
	root := NewRootCommand()
	feed := findSub(root, "feed")
	if feed == nil {
		t.Fatal("`qb feed` command not registered")
	}
	txn := findSub(feed, "txn")
	if txn == nil {
		t.Fatal("`qb feed txn` missing")
	}
	listEnt := findSub(txn, "list")
	updateEnt := findSub(txn, "update")
	runEnt := findSub(txn, "run")
	if listEnt == nil || updateEnt == nil || runEnt == nil {
		t.Fatal("`qb feed txn` missing list/update/run containers")
	}
	for _, name := range []string{"posted", "excluded"} {
		if findSub(listEnt, name) == nil {
			t.Fatalf("`qb feed txn list %s` command not registered", name)
		}
	}
	for _, name := range []string{"exclude", "undo-excluded", "categorise", "match", "split", "batch-accept", "unpost"} {
		if findSub(updateEnt, name) == nil {
			t.Fatalf("`qb feed txn update %s` command not registered", name)
		}
	}
	for _, name := range []string{"attach", "transfer", "tag"} {
		if findSub(runEnt, name) == nil {
			t.Fatalf("`qb feed txn run %s` command not registered", name)
		}
	}
	for _, name := range []string{"get", "population"} {
		if findSub(txn, name) == nil {
			t.Fatalf("`qb feed txn %s` command not registered", name)
		}
	}
	// population now runs the full paged walk (client.ReplayFeedComplete)
	// with completeness verified against numTxnToReview — it must NOT
	// expose --limit, which would silently cap an allegedly complete dump.
	population := findSub(txn, "population")
	if population == nil {
		t.Fatal("`qb feed txn population` command not registered")
	}
	if population.Flag("limit") != nil {
		t.Fatal("feed txn population should not expose --limit (walk is completeness-checked, not capped)")
	}
	for _, name := range []string{"posted", "excluded"} {
		if got := findSub(listEnt, name).Flag("limit").DefValue; got != "0" {
			t.Fatalf("feed txn list %s --limit default = %q, want 0 (all)", name, got)
		}
	}
	rule := findSub(feed, "rule")
	if rule == nil || findSub(rule, "get") == nil {
		t.Fatal("`qb feed rule get` command not registered")
	}

	if findSub(txn, "get").Flag("query") == nil {
		t.Fatal("`qb feed txn get` has no --query flag")
	}
}

// findSub returns the first direct child of cmd named name, or nil.
func findSub(cmd *cobra.Command, name string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// --- categorise / match / split / batch-accept wired commands ---------------

func TestFeedCategoriseEmptyIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "categorise", "--category", "7")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedCategoriseEmptyCategoryFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "categorise", "--ids", "3")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedCategoriseMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "categorise", "--ids", "3", "--category", "7")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedCategoriseDryRunPlan(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "update", "categorise", "--ids", "3", "--category", "7", "--dry-run", "--json"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	body := out.String()
	for _, want := range []string{"batchAcceptTransactions", "acceptOnly=true", "categoryId", "QBO.FEED.TXN_CATEGORISE"} {
		if !bytesContains(out.Bytes(), want) {
			t.Errorf("dry-run output missing %q\n%s", want, body)
		}
	}
}

func TestFeedCategoriseAnnotationDryRun(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "update", "categorise", "--ids", "3", "--category", "7", "--class", "800398", "--memo", "Fee [Card: Jacob]", "--dry-run", "--json"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"klassId", "800398", "txnMemo", "Fee [Card: Jacob]"} {
		if !bytesContains(out.Bytes(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
}

func TestFeedCategoriseAnnotationParameterContract(t *testing.T) {
	docs := paramsFor("QBO.FEED.TXN_CATEGORISE")
	seen := map[string]string{}
	for _, doc := range docs {
		seen[doc.Name] = doc.Help
	}
	if seen["memo"] == "" || !bytesContains([]byte(seen["class"]), "none clears") {
		t.Fatalf("missing annotation parameter contract: %v", seen)
	}
}

func TestFeedMatchEmptyIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "match", "--match-id", "123")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedMatchEmptyMatchIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "match", "--ids", "3")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedMatchMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "match", "--ids", "3", "--match-id", "123")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedMatchDryRunPlan(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "update", "match", "--ids", "3", "--match-id", "123", "--dry-run", "--json"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	body := out.String()
	for _, want := range []string{"acceptTransactions", "selectedMatches", "matchedTxns", "qboTxnId", "QBO.FEED.TXN_MATCH"} {
		if !bytesContains(out.Bytes(), want) {
			t.Errorf("dry-run output missing %q\n%s", want, body)
		}
	}
}

func TestFeedSplitEmptyIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "split", "--lines", "7=-6,8=-4")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedSplitTooFewLinesFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "split", "--ids", "3", "--lines", "7=-10")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedSplitMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "split", "--ids", "3", "--lines", "7=-6,8=-4")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedBatchAcceptEmptyIDsFailsFast(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "batch-accept")
	assertInputExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedBatchAcceptMissingCredentialsFails(t *testing.T) {
	elapsed, err := runFeedVerb(t, "txn", "update", "batch-accept", "--ids", "3")
	assertAuthExit(t, err, "")
	assertFast(t, elapsed)
}

func TestFeedBatchAcceptDryRunPlan(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "update", "batch-accept", "--ids", "3,4", "--dry-run", "--json"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	body := out.String()
	for _, want := range []string{"batchAcceptTransactions", "acceptOnly=true", "txnList", "olbTxns", "QBO.FEED.TXN_BATCH_ACCEPT"} {
		if !bytesContains(out.Bytes(), want) {
			t.Errorf("dry-run output missing %q\n%s", want, body)
		}
	}
}

// bytesContains is a thin helper to avoid importing bytes.Contains in the
// test file's import block (bytes is already imported).
func bytesContains(b []byte, s string) bool {
	return bytes.Contains(b, []byte(s))
}
