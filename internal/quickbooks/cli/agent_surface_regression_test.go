package cli

import (
	"strings"
	"testing"
)

func TestAgentCatalogIncludesExpensePreconditions(t *testing.T) {
	params := map[string]bool{}
	for _, p := range paramsFor("QBO.EXPENSES.EXPENSE_RECATEGORISE") {
		params[p.Name] = true
	}
	root := NewRootCommand()
	cmd, _, err := root.Find([]string{"expenses", "expense", "update", "recategorise"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"class-id", "expected-sync-token", "expected-category-id", "expected-class-id"} {
		if !params[name] || cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing agent/runtime parity for %s", name)
		}
	}
}

func TestUnsupportedWorkflowTemplateDoesNotCreateBlankRule(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	_, err := runCoverageRoot(t, "advanced", "workflows", "create", "--name", "Test", "--template", "unknown", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "--template is not supported") {
		t.Fatalf("err=%v", err)
	}
}

func TestCreditCardCreditUsesItsActualLineSchema(t *testing.T) {
	for _, p := range paramsFor("QBO.SALES.CREDIT_CARD_CREDIT_CREATE") {
		if p.Name == "line-items" {
			if !strings.Contains(p.Help, "{account, amount") || strings.Contains(p.Help, "v3 line") {
				t.Fatal(p.Help)
			}
			return
		}
	}
	t.Fatal("missing line-items")
}

func TestReceiptAliasesNeverTargetAttachments(t *testing.T) {
	for _, id := range []string{"QBO.EXPENSES.RECEIPT_DELETE", "QBO.EXPENSES.RECEIPT_READ", "QBO.EXPENSES.RECEIPT_SEARCH"} {
		e, ok := catalogByID(id)
		if !ok || e.Mode != modeBlocked {
			t.Errorf("unsafe receipt alias %s", id)
		}
		if _, ok := v3ByID[id]; ok {
			t.Errorf("receipt mapped to v3 %s", id)
		}
		if _, ok := v3MutateByID[id]; ok {
			t.Errorf("receipt mutation mapped to v3 %s", id)
		}
	}
}

func TestReminderDefaultsToNoDialPreview(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	out, err := runCoverageRoot(t, "sales", "invoice", "run", "remind", "--id", "7", "--json")
	if err != nil || !strings.Contains(out, `"dry_run": true`) {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func TestReminderRefusesHarnessSend(t *testing.T) {
	for _, env := range []string{"PRINTING_PRESS_VERIFY", "PRINTING_PRESS_DOGFOOD"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			t.Setenv(env, "1")
			_, err := runCoverageRoot(t, "sales", "invoice", "run", "remind", "--id", "7", "--send")
			if err == nil || (!strings.Contains(err.Error(), "harness") && !strings.Contains(err.Error(), "blocked before submission")) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
