package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitError_Error(t *testing.T) {
	err := &ExitError{Code: ExitSpecError, Err: fmt.Errorf("spec not found")}
	if err.Error() != "spec not found" {
		t.Errorf("got %q, want %q", err.Error(), "spec not found")
	}
}

func TestExitError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	err := &ExitError{Code: ExitInputError, Err: fmt.Errorf("wrapping: %w", inner)}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should find inner error through ExitError")
	}
}

func TestExitError_As(t *testing.T) {
	err := &ExitError{Code: ExitGenerationError, Err: fmt.Errorf("build failed")}
	wrapped := fmt.Errorf("command failed: %w", err)

	var exitErr *ExitError
	if !errors.As(wrapped, &exitErr) {
		t.Fatal("errors.As should extract ExitError from wrapped error")
	}
	if exitErr.Code != ExitGenerationError {
		t.Errorf("got code %d, want %d", exitErr.Code, ExitGenerationError)
	}
}

func TestExitCodes_Values(t *testing.T) {
	if ExitSuccess != 0 {
		t.Errorf("ExitSuccess = %d, want 0", ExitSuccess)
	}
	if ExitInputError != 1 {
		t.Errorf("ExitInputError = %d, want 1", ExitInputError)
	}
	if ExitSpecError != 2 {
		t.Errorf("ExitSpecError = %d, want 2", ExitSpecError)
	}
	if ExitGenerationError != 3 {
		t.Errorf("ExitGenerationError = %d, want 3", ExitGenerationError)
	}
	if ExitUnknownError != 4 {
		t.Errorf("ExitUnknownError = %d, want 4", ExitUnknownError)
	}
}
