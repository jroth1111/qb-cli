package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestFeedRejectsSilentlyDroppedIDs(t *testing.T) {
	for _, verb := range []string{"unpost", "undo-excluded", "exclude", "categorise", "match", "batch-accept", "split"} {
		t.Run(verb, func(t *testing.T) {
			root := NewRootCommand()
			root.SetArgs([]string{"--dry-run", "feed", "txn", "update", verb, "--ids", "101", "102"})
			root.SetOut(new(bytes.Buffer))
			root.SetErr(new(bytes.Buffer))
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "comma-separated") {
				t.Fatalf("silently dropped second ID: %v", err)
			}
		})
	}
}

func TestFeedIDForms(t *testing.T) {
	for _, tc := range []struct {
		args []string
		bad  bool
	}{
		{[]string{"--ids", "101,102"}, false},
		{[]string{"--ids", "101", "--ids", "102"}, false},
		{[]string{"--ids", "101 102"}, true},
	} {
		root := NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(new(bytes.Buffer))
		root.SetArgs(append([]string{"--dry-run", "--json", "feed", "txn", "update", "unpost"}, tc.args...))
		err := root.Execute()
		if tc.bad {
			if err == nil {
				t.Fatal("accepted whitespace list")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `"101"`) || !strings.Contains(out.String(), `"102"`) {
			t.Fatal("plan lost IDs")
		}
	}
}
