package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestFeedLookupHelpMatchesSearchFields(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "get", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"cheque-number labels", "description or cheque number", "Pending, Posted and Excluded", "not a structured amount/date filter"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("help missing %q: %s", text, out.String())
		}
	}
}
