package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMappedMatchDryRunAndAgentParams(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	p := filepath.Join(t.TempDir(), "pairs.json")
	_ = os.WriteFile(p, []byte(`[{"olb_txn_id":"1","match_id":"10"},{"olb_txn_id":"2","match_id":"20"}]`), 0600)
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"feed", "txn", "update", "match", "--mapping-file", p, "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"dial": false`, `"qboTxnId": "10"`, `"qboTxnId": "20"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, out.String())
		}
	}
	params := paramsFor("QBO.FEED.TXN_MATCH")
	found := map[string]bool{}
	for _, p := range params {
		found[p.Name] = true
		if (p.Name == "ids" || p.Name == "match-id") && p.Required {
			t.Fatal("agent schema makes mapping-file unusable")
		}
	}
	if !found["mapping-file"] || !found["verify-evidence"] {
		t.Fatal("new CLI modes absent from agent catalog")
	}
}
