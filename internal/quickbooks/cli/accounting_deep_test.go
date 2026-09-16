package cli

// accounting_deep_test.go pins the residual-audit-11 D1/D2 identity fixes:
// the deferredrevenue/prepaid get|create leaves and opening-balance create
// must carry their own catalog ID and mode in the dry-run plan, and their
// help must come from the matching catalog row — not a sibling's.
//
// Rejects: the deferredrevenue get leaf borrowing PREPAID_READ's
// help/identity, dry-run envelopes missing id/mode, and opening-balance
// create planning or documenting itself as OPENING_BALANCE_SET
// (the separate `opening-balance update` row).

import (
	"strings"
	"testing"
	"time"
)

// TestDeferredScheduleGetPlanIdentity proves each get leaf plans under its
// own catalog row with read mode and never dials.
func TestDeferredScheduleGetPlanIdentity(t *testing.T) {
	for _, tc := range []struct {
		leaf   string
		wantID string
	}{
		{"deferredrevenue", "QBO.ACCOUNTING.DEFERRED_RECOGNITION_GET"},
		{"prepaid", "QBO.ACCOUNTING.PREPAID_READ"},
	} {
		stdout, elapsed, err := runQB(t, "accounting", tc.leaf, "get", "--source-id", "42", "--dry-run", "--json")
		if elapsed > 2*time.Second {
			t.Fatalf("%s get --dry-run took %s — likely dialed", tc.leaf, elapsed)
		}
		if err != nil {
			t.Fatalf("%s get --dry-run: %v; stdout=%q", tc.leaf, err, stdout)
		}
		env := decodePlan(t, stdout)
		if !env.DryRun || env.Dial {
			t.Fatalf("%s get envelope %+v must be dry-run/dial=false", tc.leaf, env)
		}
		if env.ID != tc.wantID {
			t.Fatalf("%s get plan id=%q, want %s", tc.leaf, env.ID, tc.wantID)
		}
		if env.Mode != modeRead {
			t.Fatalf("%s get plan mode=%q, want read", tc.leaf, env.Mode)
		}
		if env.Command != "accounting "+tc.leaf+" get" {
			t.Fatalf("%s get plan command=%q", tc.leaf, env.Command)
		}
	}
}

// TestDeferredScheduleCreatePlanIdentity proves each create leaf plans under
// its own catalog row with wired mode and never dials.
func TestDeferredScheduleCreatePlanIdentity(t *testing.T) {
	for _, tc := range []struct {
		leaf   string
		wantID string
	}{
		{"deferredrevenue", "QBO.ACCOUNTING.DEFERRED_RECOGNITION_CREATE"},
		{"prepaid", "QBO.ACCOUNTING.PREPAID_CREATE"},
	} {
		stdout, elapsed, err := runQB(t, "accounting", tc.leaf, "create", "--data", `{"schedule":"x"}`, "--dry-run", "--json")
		if elapsed > 2*time.Second {
			t.Fatalf("%s create --dry-run took %s — likely dialed", tc.leaf, elapsed)
		}
		if err != nil {
			t.Fatalf("%s create --dry-run: %v; stdout=%q", tc.leaf, err, stdout)
		}
		env := decodePlan(t, stdout)
		if !env.DryRun || env.Dial {
			t.Fatalf("%s create envelope %+v must be dry-run/dial=false", tc.leaf, env)
		}
		if env.ID != tc.wantID {
			t.Fatalf("%s create plan id=%q, want %s", tc.leaf, env.ID, tc.wantID)
		}
		if env.Mode != modeWired {
			t.Fatalf("%s create plan mode=%q, want wired", tc.leaf, env.Mode)
		}
	}
}

// TestDeferredScheduleGetHelpUsesOwnCatalogID proves the deferredrevenue get
// leaf documents the DEFERRED_RECOGNITION_GET row (GET_SCHEDULE guidance)
// and no longer inherits PREPAID_READ's help text.
func TestDeferredScheduleGetHelpUsesOwnCatalogID(t *testing.T) {
	stdout, _, err := runQB(t, "accounting", "deferredrevenue", "get", "--help")
	if err != nil {
		t.Fatalf("deferredrevenue get --help: %v", err)
	}
	if !strings.Contains(stdout, "GET_SCHEDULE") {
		t.Fatalf("deferredrevenue get help lacks the DEFERRED_RECOGNITION_GET note: %s", stdout)
	}
	if strings.Contains(stdout, "no AU help article found") {
		t.Fatalf("deferredrevenue get help still carries PREPAID_READ text: %s", stdout)
	}
}

// TestOpeningBalanceCreatePlanIdentity proves the create leaf plans as
// OPENING_BALANCE_CREATE / "accounting opening-balance create" in wired
// mode — never the sibling OPENING_BALANCE_SET update row — and never dials.
func TestOpeningBalanceCreatePlanIdentity(t *testing.T) {
	stdout, elapsed, err := runQB(t, "accounting", "opening-balance", "create",
		"--date", "2026-09-01",
		"--lines", `[{"account":"1","debit":100},{"account":"2","credit":100}]`,
		"--dry-run", "--json")
	if elapsed > 2*time.Second {
		t.Fatalf("opening-balance create --dry-run took %s — likely dialed", elapsed)
	}
	if err != nil {
		t.Fatalf("opening-balance create --dry-run: %v; stdout=%q", err, stdout)
	}
	env := decodePlan(t, stdout)
	if !env.DryRun || env.Dial {
		t.Fatalf("envelope %+v must be dry-run/dial=false", env)
	}
	if env.ID != "QBO.ACCOUNTING.OPENING_BALANCE_CREATE" {
		t.Fatalf("plan id=%q, want QBO.ACCOUNTING.OPENING_BALANCE_CREATE", env.ID)
	}
	if env.Command != "accounting opening-balance create" {
		t.Fatalf("plan command=%q, want accounting opening-balance create", env.Command)
	}
	if env.Mode != modeWired {
		t.Fatalf("plan mode=%q, want wired", env.Mode)
	}
}

// TestOpeningBalanceCreateHelpUsesCreateRow proves the leaf's help is the
// CREATE row's (trial-balance wording, create usage), not the SET update
// row's Opening Balance Equity guidance.
func TestOpeningBalanceCreateHelpUsesCreateRow(t *testing.T) {
	stdout, _, err := runQB(t, "accounting", "opening-balance", "create", "--help")
	if err != nil {
		t.Fatalf("opening-balance create --help: %v", err)
	}
	if !strings.Contains(stdout, "trial balance") {
		t.Fatalf("create help lacks the OPENING_BALANCE_CREATE note: %s", stdout)
	}
	if !strings.Contains(stdout, "opening-balance create") {
		t.Fatalf("create help lacks create usage: %s", stdout)
	}
	if strings.Contains(stdout, "Opening Balance Equity") || strings.Contains(stdout, "start-of-tracking") {
		t.Fatalf("create help still carries OPENING_BALANCE_SET text: %s", stdout)
	}
}
