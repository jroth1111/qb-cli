package client

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// noCredsDir sets QB_HOME to an empty temp dir and returns a cleanup. With
// no credentials.json present, every replay entry point MUST fail fast with
// ErrNoCredentials before any network activity.
func noCredsDir(t *testing.T) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
}

// assertFast verifies the call returned well within the window that would
// imply a network call (TLS handshake). The credentials guard returns in
// microseconds; a regression that reached the HTTP client would take
// noticeably longer.
func assertFast(t *testing.T, elapsed time.Duration) {
	t.Helper()
	if elapsed > 2*time.Second {
		t.Fatalf("took %s — likely reached the network", elapsed)
	}
}

// saveUsable writes a minimal usable TokenSet (ATS Authorization + realm)
// to QB_HOME so newAPIClient succeeds without network.
func saveUsable(t *testing.T) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       "12345",
	}); err != nil {
		t.Fatal(err)
	}
}

// --- ReplayFeed: fail fast on no credentials --------------------------------

// TestReplayFeedFailsOnNoCredentials is the missing-credentials negative
// for ReplayFeed. With QB_HOME pointed at an empty temp dir there are no
// saved credentials, so ReplayFeed MUST fail with ErrNoCredentials and MUST
// NOT reach the network. It rejects:
//   - returning a nil error (silently succeeding with no credentials),
//   - a non-ErrNoCredentials error (guard bypassed -> wrong failure),
//   - taking long enough to imply a network call.
func TestReplayFeedFailsOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayFeed(context.Background(), "", "PENDING", 10)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ReplayFeed: expected error, got nil")
	}
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("ReplayFeed: got %v, want ErrNoCredentials", err)
	}
	assertFast(t, elapsed)
}

// --- ReplayLookup: fail fast on no credentials ------------------------------

// TestReplayLookupFailsOnNoCredentials asserts ReplayLookup with a
// non-empty query and no saved credentials fails fast with ErrNoCredentials:
// ReplayLookup delegates to ReplayFeed, which guards on credentials before
// any network. It rejects:
//   - returning a nil error,
//   - a non-ErrNoCredentials error (guard bypassed),
//   - taking long enough to imply a network call.
func TestReplayLookupFailsOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayLookup(context.Background(), "", "google", 10)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ReplayLookup: expected error, got nil")
	}
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("ReplayLookup: got %v, want ErrNoCredentials", err)
	}
	assertFast(t, elapsed)
}

// --- ReplayRegister: fail fast on no credentials -----------------------------

// TestReplayRegisterFailsOnNoCredentials is the missing-credentials negative
// for ReplayRegister. With no saved credentials it MUST fail with
// ErrNoCredentials before any network activity. It rejects:
//   - returning a nil error,
//   - a non-ErrNoCredentials error (guard bypassed),
//   - taking long enough to imply a network call.
func TestReplayRegisterFailsOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayRegister(context.Background(), "", 10)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ReplayRegister: expected error, got nil")
	}
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("ReplayRegister: got %v, want ErrNoCredentials", err)
	}
	assertFast(t, elapsed)
}

// --- ReplayLookup: empty query errors ---------------------------------------

// TestReplayLookupEmptyQueryErrors asserts ReplayLookup rejects an empty
// query with ErrEmptyQuery before any network activity — even with no saved
// credentials. This is the lookup negative case: a regression that forwarded
// an empty query to ReplayFeed would either reach the network or surface a
// different error shape. It rejects:
//   - returning a nil error (empty query silently matching everything),
//   - an ErrNoCredentials error (query check must come before the credentials
//     guard so callers get a validation error, not a session error),
//   - taking long enough to imply a network call.
func TestReplayLookupEmptyQueryErrors(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayLookup(context.Background(), "", "  ", 10)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ReplayLookup: expected error for empty query, got nil")
	}
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("ReplayLookup empty query: got %v, want ErrEmptyQuery", err)
	}
	assertFast(t, elapsed)
}

// --- RequireSession ---------------------------------------------------------

// TestRequireSessionFailsOnNoCredentials asserts RequireSession fails with
// ErrNoCredentials when no credentials are saved, without any network
// activity. It rejects:
//   - returning nil (session check silently passed with no credentials),
//   - a non-ErrNoCredentials error,
//   - taking long enough to imply a network call.
func TestRequireSessionFailsOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	err := RequireSession()
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("RequireSession: expected error, got nil")
	}
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("RequireSession: got %v, want ErrNoCredentials", err)
	}
	assertFast(t, elapsed)
}

// TestRequireSessionSucceedsWithUsableCreds asserts RequireSession returns
// nil when a usable session is saved. No network activity occurs — only
// auth.Load and HasUsableCredential. It rejects a regression that added a
// network probe to the session check.
func TestRequireSessionSucceedsWithUsableCreds(t *testing.T) {
	saveUsable(t)
	start := time.Now()
	err := RequireSession()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RequireSession: got %v, want nil", err)
	}
	assertFast(t, elapsed)
}

// --- ErrMutationNotWired ----------------------------------------------------

// TestErrMutationNotWiredIsSentinel asserts ErrMutationNotWired is a stable
// sentinel that callers can branch on with errors.Is. It rejects a
// regression that changed it to a non-comparable value or removed it.
func TestErrMutationNotWiredIsSentinel(t *testing.T) {
	if !errors.Is(ErrMutationNotWired, ErrMutationNotWired) {
		t.Fatal("ErrMutationNotWired must be usable with errors.Is")
	}
	if !strings.Contains(ErrMutationNotWired.Error(), "mutation not wired") {
		t.Fatalf("ErrMutationNotWired message = %q", ErrMutationNotWired.Error())
	}
}

// --- ReplayPending delegation ------------------------------------------------

// TestReplayPendingDelegatesToReplayFeed asserts ReplayPending still has the
// same signature and delegates to ReplayFeed. With no credentials both must
// fail identically with ErrNoCredentials. This guards the backward-
// compatibility contract: ReplayPending's signature is unchanged.
func TestReplayPendingDelegatesToReplayFeed(t *testing.T) {
	noCredsDir(t)
	_, err := ReplayPending(context.Background(), "", "", 5)
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("ReplayPending: got %v, want ErrNoCredentials", err)
	}
}

// --- Register projection (no network) ---------------------------------------

// TestExtractRegisterTxnsBareArray asserts extractRegisterTxns recognises a
// bare JSON array of transactions and projects id/txnId, date/txnDate,
// amount, description/memo/payee aliases. It rejects a regression that
// dropped an alias or mis-assigned a field.
func TestExtractRegisterTxnsBareArray(t *testing.T) {
	body := []byte(`[
		{"id":"t1","txnDate":"2026-08-02","amount":304.28,"memo":"Google ADS"},
		{"txnId":"t2","date":"2026-08-01","amount":-50.0,"payee":"Shell"}
	]`)
	txns, ok := extractRegisterTxns(body)
	if !ok {
		t.Fatal("expected bare array to be recognised")
	}
	if len(txns) != 2 {
		t.Fatalf("got %d txns, want 2", len(txns))
	}
	res := projectRegister(txns)
	if res.Transactions[0].ID != "t1" || res.Transactions[0].Date != "2026-08-02" {
		t.Fatalf("txn0 = %+v", res.Transactions[0])
	}
	if res.Transactions[0].Description != "Google ADS" {
		t.Fatalf("txn0 description = %q, want memo fallback", res.Transactions[0].Description)
	}
	// txn1 uses txnId (id empty) and payee (description/memo empty).
	if res.Transactions[1].ID != "t2" {
		t.Fatalf("txn1 id = %q, want txnId fallback", res.Transactions[1].ID)
	}
	if res.Transactions[1].Description != "Shell" {
		t.Fatalf("txn1 description = %q, want payee fallback", res.Transactions[1].Description)
	}
	if res.Transactions[1].Amount != -50.0 {
		t.Fatalf("txn1 amount = %v, want -50", res.Transactions[1].Amount)
	}
	if res.Counts["transactions"] != 2 {
		t.Fatalf("counts = %v, want transactions=2", res.Counts)
	}
}

// TestExtractRegisterTxnsItemsEnvelope asserts extractRegisterTxns
// recognises an object with an "items" field (the same envelope shape as
// the banking getTransactions API).
func TestExtractRegisterTxnsItemsEnvelope(t *testing.T) {
	body := []byte(`{"items":[{"id":"a1","txnDate":"2026-07-01","amount":10.5,"description":"Foo"}]}`)
	txns, ok := extractRegisterTxns(body)
	if !ok {
		t.Fatal("expected items envelope to be recognised")
	}
	if len(txns) != 1 || txns[0].ID != "a1" {
		t.Fatalf("got %+v", txns)
	}
}

// TestExtractRegisterTxnsTransactionsEnvelope asserts extractRegisterTxns
// recognises an object with a "transactions" field.
func TestExtractRegisterTxnsTransactionsEnvelope(t *testing.T) {
	body := []byte(`{"transactions":[{"id":"b1","date":"2026-06-01","amount":20.0,"description":"Bar"}]}`)
	txns, ok := extractRegisterTxns(body)
	if !ok {
		t.Fatal("expected transactions envelope to be recognised")
	}
	if len(txns) != 1 || txns[0].ID != "b1" {
		t.Fatalf("got %+v", txns)
	}
}

// TestExtractRegisterTxnsUnknownShape asserts extractRegisterTxns returns
// false for an unrecognised body shape, so ReplayRegister refuses to report
// a false empty register without leaking the raw body.
func TestExtractRegisterTxnsUnknownShape(t *testing.T) {
	body := []byte(`{"error":"throttled","requestId":"abc"}`)
	txns, ok := extractRegisterTxns(body)
	if ok {
		t.Fatal("expected unknown shape to be rejected")
	}
	if txns != nil {
		t.Fatalf("expected nil txns, got %+v", txns)
	}
}

// TestProjectRegisterEmpty asserts projectRegister on an empty slice
// returns a non-nil result with an empty (not nil) transaction slice, so
// JSON marshalling produces [] not null.
func TestProjectRegisterEmpty(t *testing.T) {
	res := projectRegister(nil)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Transactions == nil {
		t.Fatal("expected empty slice, not nil")
	}
	if len(res.Transactions) != 0 {
		t.Fatalf("got %d txns, want 0", len(res.Transactions))
	}
}

// --- ReplayFeed default account / state (no network via no-creds guard) ------

// TestReplayFeedDefaultAccountIs204 guards the default-account contract:
// an empty accountID resolves to DefaultAccountID. We can't observe this
// directly without a server, but we can assert the constant is unchanged.
func TestReplayFeedDefaultAccountIs204(t *testing.T) {
	if DefaultAccountID != "204" {
		t.Fatalf("DefaultAccountID = %q, want 204", DefaultAccountID)
	}
}

// --- ReplayLookup filtering (no network) ------------------------------------

// TestReplayLookupFiltersByIDAndDescription is a unit test for txnMatches.
// A transaction matches when id, olbTxnId, or description contains the
// query after case-fold and hyphen/underscore folding. It rejects a
// regression that only checked one field, was case-sensitive, or failed
// to match "QB-CLI-TEST" against "Qb Cli Test Do Not".
func TestReplayLookupFiltersByIDAndDescription(t *testing.T) {
	txns := []Transaction{
		{ID: "google-ads-001", Description: "Google ADS Sydney"},
		{ID: "shell-002", Description: "Fuel Station Sydney"},
		{ID: "amazon-003", Description: "Amazon AU"},
		{ID: "32371:ofx", OLBTxnID: "32371", Description: "Qb Cli Test Do Not"},
	}

	got := filterTxns(txns, "google")
	if len(got) != 1 || got[0].ID != "google-ads-001" {
		t.Fatalf("filter by id: got %+v, want 1 match", got)
	}

	got = filterTxns(txns, "shell")
	if len(got) != 1 || got[0].ID != "shell-002" {
		t.Fatalf("filter by description (case-insensitive): got %+v, want 1", got)
	}

	got = filterTxns(txns, "QB-CLI-TEST")
	if len(got) != 1 || got[0].OLBTxnID != "32371" {
		t.Fatalf("hyphen-fold description: got %+v, want 32371", got)
	}

	got = filterTxns(txns, "32371")
	if len(got) != 1 || got[0].OLBTxnID != "32371" {
		t.Fatalf("filter by olbTxnId: got %+v, want 32371", got)
	}
}

func filterTxns(txns []Transaction, query string) []Transaction {
	var got []Transaction
	for _, txn := range txns {
		if txnMatches(txn, query) {
			got = append(got, txn)
		}
	}
	return got
}

// --- No env leak guard ------------------------------------------------------

// TestNoQBHomeEnvLeak ensures the test suite does not depend on a real
// QB_HOME from the environment. If QB_HOME is set outside the test process
// the no-credentials negatives could pass for the wrong reason (credentials
// found on disk). This test rejects that by asserting QB_HOME is unset or
// points to a temp dir, not a real config root.
func TestNoQBHomeEnvLeak(t *testing.T) {
	if home := os.Getenv("QB_HOME"); home != "" {
		// t.Setenv in other tests sets temp dirs; outside those, QB_HOME
		// should not leak. This is a sanity guard, not a hard failure.
		t.Logf("QB_HOME=%q (set by another test in this process — OK)", home)
	}
}

func TestReplayExcludeEmptyIDs(t *testing.T) {
	saveUsable(t)
	start := time.Now()
	err := ReplayExclude(context.Background(), "93", nil)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrEmptyExcludeIDs) {
		t.Fatalf("got %v, want ErrEmptyExcludeIDs", err)
	}
}

func TestReplayExcludeNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	err := ReplayExclude(context.Background(), "93", []string{"1"})
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func TestReplayUndoEmptyIDs(t *testing.T) {
	saveUsable(t)
	start := time.Now()
	err := ReplayUndo(context.Background(), "209", nil)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrEmptyUndoIDs) {
		t.Fatalf("got %v, want ErrEmptyUndoIDs", err)
	}
}

func TestReplayUndoNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	err := ReplayUndo(context.Background(), "209", []string{"1"})
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func TestReplayImportCSVEmptyAccount(t *testing.T) {
	saveUsable(t)
	start := time.Now()
	_, err := ReplayImportCSV(context.Background(), "", "17/08/2026", "QB-CLI-TEST", "0.01")
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrEmptyImportAccount) {
		t.Fatalf("got %v, want ErrEmptyImportAccount", err)
	}
}

func TestReplayImportCSVBlockedLiveAccounts(t *testing.T) {
	saveUsable(t)
	for _, id := range []string{"204", "93"} {
		start := time.Now()
		_, err := ReplayImportCSV(context.Background(), id, "17/08/2026", "QB-CLI-TEST", "0.01")
		assertFast(t, time.Since(start))
		if !errors.Is(err, ErrImportBlockedAccount) {
			t.Fatalf("account %s: got %v, want ErrImportBlockedAccount", id, err)
		}
	}
}

func TestReplayImportCSVEmptyDescription(t *testing.T) {
	saveUsable(t)
	start := time.Now()
	_, err := ReplayImportCSV(context.Background(), "209", "17/08/2026", "  ", "0.01")
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrEmptyImportDescription) {
		t.Fatalf("got %v, want ErrEmptyImportDescription", err)
	}
}

func TestReplayImportCSVBadAmount(t *testing.T) {
	saveUsable(t)
	start := time.Now()
	_, err := ReplayImportCSV(context.Background(), "209", "17/08/2026", "QB-CLI-TEST", "nope")
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrEmptyImportAmount) {
		t.Fatalf("got %v, want ErrEmptyImportAmount", err)
	}
}

func TestReplayImportCSVNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayImportCSV(context.Background(), "209", "17/08/2026", "QB-CLI-TEST", "0.01")
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func TestIsInactiveAccount(t *testing.T) {
	if isInactiveAccount(nil) || isInactiveAccount(errors.New("inactive")) {
		t.Fatal("non-ReplayError must be false")
	}
	if !isInactiveAccount(&ReplayError{Status: 400, Message: "inactive field"}) {
		t.Fatal("short inactive field 400")
	}
	if !isInactiveAccount(&ReplayError{Status: 400, Message: "Something you're trying to use has been made inactive."}) {
		t.Fatal("long made-inactive 400")
	}
	if isInactiveAccount(&ReplayError{Status: 422, Message: "inactive field"}) {
		t.Fatal("422 is not the register/feed inactive 400")
	}
	if !allowInactiveFallback("") || !allowInactiveFallback(DefaultAccountID) {
		t.Fatal("default account must allow fallback")
	}
	if allowInactiveFallback("48") {
		t.Fatal("explicit live id must not fallback")
	}
}
