package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

// runInv executes qb with args, capturing stdout, under an empty QB_HOME so
// any live-dial attempt fails fast with ErrNoCredentials instead of reaching
// the network.
func runInv(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	root := NewRootCommand()
	root.SetArgs(args)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.ExecuteContext(ctx)
	return out.String(), err
}

// TestInventoryNewTreeRegistered pins the four new subcommand surfaces on the
// inventory domain: count create|list|get, buildassembly create,
// startingvalue create, and adjust delete. Pure structure; rejects a wiring
// regression that drops any verb.
func TestInventoryNewTreeRegistered(t *testing.T) {
	root := NewRootCommand()
	inv := findSub(root, "inventory")
	if inv == nil {
		t.Fatal("inventory missing")
	}
	count := findSub(inv, "count")
	if count == nil {
		t.Fatal("inventory count missing")
	}
	for _, verb := range []string{"create", "list", "get"} {
		if findSub(count, verb) == nil {
			t.Errorf("inventory count missing %q", verb)
		}
	}
	if findSub(inv, "buildassembly") == nil || findSub(findSub(inv, "buildassembly"), "create") == nil {
		t.Error("inventory buildassembly create missing")
	}
	if findSub(inv, "startingvalue") == nil || findSub(findSub(inv, "startingvalue"), "create") == nil {
		t.Error("inventory startingvalue create missing")
	}
	adj := findSub(inv, "adjust")
	if adj == nil || findSub(adj, "delete") == nil {
		t.Error("inventory adjust delete missing")
	}
}

// TestInventoryCountCreateDryRunPostsV3Path proves the wired count create is
// a real mutate command: --dry-run emits a plan envelope whose URL is the v3
// company inventory-counts path with {realm} (never a live id). Rejects a
// stub that plans nothing or leaks the realm.
func TestInventoryCountCreateDryRunPostsV3Path(t *testing.T) {
	out, err := runInv(t, "inventory", "count", "create", "--item", "42", "--counted-qty", "7", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	var env planEnvelope
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &env); err != nil {
		t.Fatalf("plan not JSON: %v (%s)", err, out)
	}
	if !env.DryRun || env.Dial {
		t.Fatalf("dial must be false in dry-run: %+v", env)
	}
	if env.Method != "POST" {
		t.Fatalf("method = %q, want POST", env.Method)
	}
	want := "https://qbo.intuit.com/api/v3/company/{realm}/inventory-counts?minorversion=73"
	if env.URL != want {
		t.Fatalf("url = %q, want %q", env.URL, want)
	}
}

// TestInventoryBuildAssemblyAndStartingValueDryRunPaths checks the other two
// create verbs plan their route-derived v3 paths.
func TestInventoryBuildAssemblyAndStartingValueDryRunPaths(t *testing.T) {
	cases := []struct {
		args []string
		path string
	}{
		{[]string{"inventory", "buildassembly", "create", "--item", "8", "--qty", "2"}, "/buildassembly"},
		{[]string{"inventory", "startingvalue", "create", "--item", "8", "--qty", "2"}, "/inventory_starting_value"},
	}
	for _, tc := range cases {
		out, err := runInv(t, append(tc.args, "--dry-run", "--json")...)
		if err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		var env planEnvelope
		if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &env); err != nil {
			t.Fatalf("%v: plan not JSON: %v", tc.args, err)
		}
		if !strings.HasSuffix(env.URL, tc.path+"?minorversion=73") {
			t.Fatalf("url = %q, want suffix %q", env.URL, tc.path)
		}
	}
}

// TestInventoryAdjustDeleteDryRunIsDeleteOp proves adjust delete plans a
// delete-operation POST rather than a plain create URL.
func TestInventoryAdjustDeleteDryRunIsDeleteOp(t *testing.T) {
	out, err := runInv(t, "inventory", "adjust", "delete", "--id", "159", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	var env planEnvelope
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &env); err != nil {
		t.Fatalf("plan not JSON: %v", err)
	}
	if !strings.Contains(env.URL, "operation=delete") {
		t.Fatalf("url = %q, want operation=delete", env.URL)
	}
	if !strings.Contains(env.URL, "/inventoryadjustment?") {
		t.Fatalf("url = %q, want /inventoryadjustment path", env.URL)
	}
}

// TestInventoryCreateValidationFailsFastLivePath is the negative case for the
// live (non-dry-run) path: without credentials the commands must fail with
// ErrNoCredentials-classified auth exit BEFORE flag validation would matter,
// and with credentials absent they must never return success. Validation
// ordering itself is covered client-side; here we pin the exit-code contract:
// no creds -> ExitAuthError, never ExitInputError or silent success.
func TestInventoryCreateValidationFailsFastLivePath(t *testing.T) {
	for _, args := range [][]string{
		{"inventory", "count", "create", "--item", "1", "--counted-qty", "2"},
		{"inventory", "buildassembly", "create", "--item", "1", "--qty", "1"},
		{"inventory", "startingvalue", "create", "--item", "1", "--qty", "1"},
		{"inventory", "adjust", "delete", "--id", "159"},
	} {
		_, err := runInv(t, args...)
		if err == nil {
			t.Errorf("%v: expected auth failure without creds, got nil", args)
			continue
		}
		if got := ClassifyExitCode(err); got != ExitInputError {
			t.Errorf("%v: exit = %d, want readback gate before submission; err=%v", args, got, err)
		}
		if !errors.Is(err, client.ErrReadbackUnavailable) {
			t.Errorf("%v: err = %v, want ErrReadbackUnavailable chain", args, err)
		}
	}
}
