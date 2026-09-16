package cli

import (
	"errors"
	"strings"
	"testing"
)

// TestUnknownFlagExitsInputCode guards the contract: an unknown flag is a
// user-input mistake and must surface as ExitInputError (1), not
// ExitUnknownError (4). Regression for the Phase B output-regime audit.
func TestUnknownFlagExitsInputCode(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "list", "--bogus-flag"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if got := ClassifyExitCode(err); got != ExitInputError {
		t.Fatalf("classified exit = %d, want %d (input)", got, ExitInputError)
	}
}

// TestUntypedErrorClassifiesAsUnknown proves ClassifyExitCode maps a plain
// runtime error to ExitUnknownError (4) — the executable counterpart to the
// main.go mapping, so a regression that collapses untyped errors into the
// input code is caught here.
func TestUntypedErrorClassifiesAsUnknown(t *testing.T) {
	err := errors.New("genuinely unexpected runtime failure")
	if got := ClassifyExitCode(err); got != ExitUnknownError {
		t.Fatalf("ClassifyExitCode = %d, want %d (unknown)", got, ExitUnknownError)
	}
}

// TestShouldSuppressHonorsSilentFlag covers the suppression helper main.go
// uses: Silent ExitError suppresses stderr; non-silent does not.
func TestShouldSuppressHonorsSilentFlag(t *testing.T) {
	silent := &ExitError{Code: ExitInputError, Err: errors.New("x"), Silent: true}
	loud := &ExitError{Code: ExitInputError, Err: errors.New("x")}
	if !ShouldSuppress(silent) {
		t.Error("Silent=true should suppress")
	}
	if ShouldSuppress(loud) {
		t.Error("Silent=false should not suppress")
	}
	if ShouldSuppress(errors.New("plain")) {
		t.Error("plain error should never suppress")
	}
}

// TestCSVLineItemsRejectedBeforePlan guards the silent-$1 defect end to
// end: CSV rows were dropped and a $1 default line recorded instead. The
// rejection must fire before any plan is emitted, even under --dry-run.
func TestCSVLineItemsRejectedBeforePlan(t *testing.T) {
	_, _, err := runQB(t, "sales", "invoice", "create",
		"--customer", "1", "--invoice-date", "08/09/2026",
		"--line-items", "Services,1,7.77,GST", "--dry-run")
	if err == nil {
		t.Fatal("expected CSV line-items rejection, got nil error")
	}
	if got := ClassifyExitCode(err); got != ExitInputError {
		t.Fatalf("classified exit = %d, want %d (input)", got, ExitInputError)
	}
	if !strings.Contains(err.Error(), "JSON array") {
		t.Fatalf("error %q does not point at the JSON format", err.Error())
	}
}

// TestCreateWithoutRequiredLinesRejected guards the silent-$1 defect from
// the omission side: creates whose catalog marks line-items required must
// refuse when lines are absent, instead of recording a $1 default line.
func TestCreateWithoutRequiredLinesRejected(t *testing.T) {
	_, _, err := runQB(t, "sales", "invoice", "create",
		"--customer", "1", "--invoice-date", "08/09/2026", "--dry-run")
	if err == nil {
		t.Fatal("expected missing-lines rejection, got nil error")
	}
	if got := ClassifyExitCode(err); got != ExitInputError {
		t.Fatalf("classified exit = %d, want %d (input)", got, ExitInputError)
	}
	if !strings.Contains(err.Error(), "--line-items") {
		t.Fatalf("error %q does not name the missing flag", err.Error())
	}
}
