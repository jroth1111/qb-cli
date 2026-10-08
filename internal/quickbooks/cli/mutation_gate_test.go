package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/proof"
	"github.com/spf13/cobra"
)

func TestEveryCatalogMutationHasSuccessGate(t *testing.T) {
	root := NewRootCommand()
	count, available := 0, 0
	for _, e := range catalogPrimitives {
		if e.Mode != modeWired || e.Risk == "R0" {
			continue
		}
		cmd, _, err := root.Find(strings.Fields(e.Command))
		if err != nil || cmd.Annotations["qb:verification-gated"] != "true" {
			t.Errorf("unguarded mutation %s", e.ID)
		}
		count++
		if hasReadbackAdapter(cmd, e) {
			available++
		}
	}
	if count < 100 {
		t.Fatalf("unexpected mutation coverage %d", count)
	}
	t.Logf("mutation commands=%d adapters=%d blocked=%d", count, available, count-available)
}

func TestOperationalLeavesCannotEscapeClassification(t *testing.T) {
	root := NewRootCommand()
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() {
			path := strings.TrimPrefix(cmd.CommandPath(), "qb ")
			first := strings.Fields(path)[0]
			if _, ok := catalogByCommand(path); !ok && first != "qb" && first != "auth" && first != "login" && first != "doctor" && first != "actions" && first != "gql" && cmd.Annotations["qb:verification-gated"] != "true" && cmd.Annotations["qb:read-only"] != "true" {
				t.Errorf("uncatalogued operational leaf %s", path)
			}
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}

func TestMutationGateRejectsResponseClaimWithoutProof(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		flags := &rootFlags{asJSON: true}
		root := &cobra.Command{Use: "qb", SilenceUsage: true, SilenceErrors: true}
		group := &cobra.Command{Use: "advanced"}
		batch := &cobra.Command{Use: "batch"}
		leaf := &cobra.Command{Use: "run", RunE: func(cmd *cobra.Command, args []string) error {
			if confirmed {
				proof.Confirm(cmd.Context())
			}
			_, _ = cmd.OutOrStdout().Write([]byte(`{"ok":true,"verified":true}`))
			return nil
		}}
		batch.AddCommand(leaf)
		group.AddCommand(batch)
		root.AddCommand(group)
		installMutationGates(root, flags)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{"advanced", "batch", "run"})
		err := root.ExecuteContext(context.Background())
		if (err == nil) != confirmed {
			t.Fatalf("confirmed=%v err=%v", confirmed, err)
		}
		var obj map[string]any
		if json.Unmarshal(out.Bytes(), &obj) != nil || obj["verified"] != confirmed {
			t.Fatalf("false completion envelope: %s", out.String())
		}
	}
}

func TestRulePreviewDoesNotClaimMutationProof(t *testing.T) {
	for _, preview := range []string{"true", "false", "invalid"} {
		flags := &rootFlags{asJSON: true}
		root := &cobra.Command{Use: "qb", SilenceUsage: true, SilenceErrors: true}
		feed := &cobra.Command{Use: "feed"}
		rule := &cobra.Command{Use: "rule"}
		leaf := &cobra.Command{Use: "update", RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write([]byte(`{"matchingPopulationVerified":false}`))
			return err
		}}
		leaf.Flags().String("preview", "", "")
		rule.AddCommand(leaf)
		feed.AddCommand(rule)
		root.AddCommand(feed)
		installMutationGates(root, flags)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{"feed", "rule", "update", "--preview", preview})
		err := root.ExecuteContext(context.Background())
		if (err == nil) != (preview == "true") {
			t.Fatalf("preview=%s err=%v", preview, err)
		}
		if preview == "true" && strings.Contains(out.String(), `"verified":true`) {
			t.Fatal("preview falsely claimed a verified mutation")
		}
	}
}

func TestUnsupportedMutationNeverCallsHandler(t *testing.T) {
	root := &cobra.Command{Use: "qb"}
	g := &cobra.Command{Use: "accounting"}
	b := &cobra.Command{Use: "budget"}
	called := false
	b.AddCommand(&cobra.Command{Use: "create", RunE: func(*cobra.Command, []string) error { called = true; return nil }})
	g.AddCommand(b)
	root.AddCommand(g)
	installMutationGates(root, &rootFlags{})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"accounting", "budget", "create"})
	if err := root.Execute(); err == nil || called {
		t.Fatal("unsupported mutation reached handler")
	}
}
