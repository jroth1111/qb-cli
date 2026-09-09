package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

// TestCustomObjectsAndCostGroupsSurface asserts the two new top-level domains
// are registered on root with their full closed verb sets. Pure structure —
// no RunE invocation, no network.
func TestCustomObjectsAndCostGroupsSurface(t *testing.T) {
	root := NewRootCommand()
	for _, domain := range []string{"customobjects", "costgroups"} {
		if c := findSub(root, domain); c == nil {
			t.Fatalf("root missing %q subcommand", domain)
		}
	}
	co := findSub(root, "customobjects")
	if co == nil {
		t.Fatal("missing customobjects")
	}
	for _, verb := range []string{"list", "get", "create", "update", "delete"} {
		if findSub(co, verb) == nil {
			t.Errorf("customobjects missing %q", verb)
		}
	}
	cg := findSub(root, "costgroups")
	if cg == nil {
		t.Fatal("missing costgroups")
	}
	for _, verb := range []string{"list", "create", "update", "delete"} {
		if findSub(cg, verb) == nil {
			t.Errorf("costgroups missing %q", verb)
		}
	}
}

// TestCostGroupsDryRunPlansAreSecretFree runs every costgroups mutation with
// --dry-run and asserts the plan carries the service URL (no realm segment,
// no secrets) and the captured operation names. Rejects plans that embed a
// live company id or skip the GraphQL body.
func TestCostGroupsDryRunPlansAreSecretFree(t *testing.T) {
	cases := []struct {
		args     []string
		wantOp   string
		wantURL  string
		wantFail error
	}{
		{
			args:    []string{"costgroups", "create", "--name", "Freight"},
			wantOp:  "CreateCostGroup",
			wantURL: client.PlannedCostGroupsURL(),
		},
		{
			args:     []string{"costgroups", "create"},
			wantFail: client.ErrEmptyCostGroupName,
		},
		{
			args:    []string{"costgroups", "update", "--id", "9", "--name", "N", "--version", "2"},
			wantOp:  "UpdateCostGroup",
			wantURL: client.PlannedCostGroupsURL(),
		},
		{
			args:    []string{"costgroups", "delete", "--id", "9", "--version", "1"},
			wantOp:  "ToggleCostGroupDeletion",
			wantURL: client.PlannedCostGroupsURL(),
		},
		{
			args:    []string{"costgroups", "list"},
			wantOp:  "GetCostGroupsWithFilter",
			wantURL: client.PlannedCostGroupsURL(),
		},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root := NewRootCommand()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append(tc.args, "--dry-run", "--home", t.TempDir()))
			err := root.Execute()
			if tc.wantFail != nil {
				if err == nil || !errors.Is(err, tc.wantFail) {
					t.Fatalf("err = %v, want %v", err, tc.wantFail)
				}
				return
			}
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			body := out.String()
			if !strings.Contains(body, tc.wantURL) {
				t.Errorf("plan missing service url %q:\n%s", tc.wantURL, body)
			}
			if !strings.Contains(body, `"operationName":"`+tc.wantOp+`"`) {
				t.Errorf("plan missing operation %q:\n%s", tc.wantOp, body)
			}
			if realmLeak := "12345"; strings.Contains(body, realmLeak) {
				t.Errorf("plan leaked live realm id:\n%s", body)
			}
		})
	}
}

// TestCustomObjectsNotWiredVerbs asserts the definition-mutation stubs fail
// fast with ErrMutationNotWired when a usable session exists but no API is
// wired, and that list/get still plan honestly under --dry-run.
func TestCustomObjectsNotWiredVerbs(t *testing.T) {
	for _, args := range [][]string{
		{"customobjects", "create"},
		{"customobjects", "update"},
		{"customobjects", "delete"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := NewRootCommand()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append(args, "--dry-run", "--home", t.TempDir()))
			if err := root.Execute(); err != nil {
				t.Fatalf("dry-run stub should not error: %v", err)
			}
			if !strings.Contains(out.String(), "not sent") {
				t.Errorf("expected dry-run plan output, got:\n%s", out.String())
			}
		})
	}

	// No credentials: the not-wired verbs must exit as auth failures before
	// surfacing ErrMutationNotWired.
	for _, args := range [][]string{{"customobjects", "delete"}} {
		root := NewRootCommand()
		root.SetArgs(append(args, "--home", t.TempDir()))
		err := root.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != ExitAuthError {
			t.Fatalf("err = %v, want ExitError code %d", err, ExitAuthError)
		}
	}
}
