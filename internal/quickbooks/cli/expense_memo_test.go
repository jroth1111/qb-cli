package cli

import (
	"strings"
	"testing"
)

func TestExpenseMemoFlagReachesMutationInput(t *testing.T) {
	root := NewRootCommand()
	cmd, _, err := root.Find([]string{"expenses", "expense", "update", "edit"})
	if err != nil {
		t.Fatal(err)
	}
	memo := "Original  memo    spacing"
	if err := cmd.ParseFlags([]string{"--id", "123", "--memo", memo}); err != nil {
		t.Fatal(err)
	}
	if got := collectFlags(cmd)["memo"]; got != memo {
		t.Fatalf("memo=%q, want %q", got, memo)
	}
	if !strings.Contains(cmd.Long, "--memo") {
		t.Fatal("memo missing from catalog help")
	}
}

func TestUnpostHelpDisclosesAccountingDeletion(t *testing.T) {
	root := NewRootCommand()
	cmd, _, err := root.Find([]string{"feed", "txn", "update", "unpost"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"can delete the expense", "preserving Unmatch"} {
		if !strings.Contains(cmd.Long, want) {
			t.Fatalf("unpost help does not disclose %q", want)
		}
	}
}
