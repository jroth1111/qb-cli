package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
)

// gqlOpsRuns executes `qb gql ops [substr]` in-process.
func gqlOpsRun(t *testing.T, args ...string) string {
	t.Helper()
	root := newRootCmd(&rootFlags{})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs(append([]string{"gql", "ops"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("gql ops %v: %v", args, err)
	}
	return root.OutOrStdout().(*bytes.Buffer).String()
}

// TestGqlOpsListsOffline proves the listing path needs no relay: it runs
// with QB_RELAY_URL pointing at a dead port. A regression that dials the
// network here hangs/fails instead of printing.
func TestGqlOpsListsOffline(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	out := gqlOpsRun(t)
	if !strings.Contains(out, "GetBills") {
		t.Fatalf("ops output missing GetBills:\n%s", out[:min(len(out), 400)])
	}
}

func TestGqlOpsFilterJSON(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	var buf bytes.Buffer
	root := newRootCmd(&rootFlags{})
	root.SetArgs([]string{"gql", "ops", "Bills", "--json"})
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, buf.String())
	}
	found := false
	for _, r := range rows {
		if r["name"] == "GetBills" {
			found = true
			if r["kind"] != "query" || r["endpoint"] == "" || r["module"] == nil {
				t.Fatalf("GetBills row incomplete: %v", r)
			}
		}
	}
	if !found {
		t.Fatal("Bills JSON filter missing GetBills")
	}
}

func TestParseVarsCoercion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want any
	}{
		{"first=10", 10},
		{"after=CUR", "CUR"},
		{"flag=true", true},
		{"empty=", nil},
		{"obj={\"a\":1}", map[string]any{"a": float64(1)}},
		{"arr=[1,2]", []any{float64(1), float64(2)}},
		{"ratio=1.5", 1.5},
	}
	got, err := parseVars(func() []string {
		s := make([]string, len(cases))
		for i, c := range cases {
			s[i] = c.in
		}
		return s
	}())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		name := strings.SplitN(tc.in, "=", 2)[0]
		gj, _ := json.Marshal(got[name])
		wj, _ := json.Marshal(tc.want)
		if !bytesEqual(gj, wj) {
			t.Errorf("%s: got %s want %s", name, gj, wj)
		}
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseVarsRejectsMalformed(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"noequals", "=novalue"} {
		if _, err := parseVars([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// TestGqlQueryUnknownVarFailsFast proves the CLI validates before any
// network attempt — dead relay env proves no dial happens on this path.
func TestGqlQueryUnknownVarFailsFast(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	root := newRootCmd(&rootFlags{})
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"gql", "query", "GetBills", "--vars", "bogus=1"})
	err := root.Execute()
	if err == nil {
		t.Fatal("unknown var accepted")
	}
	if strings.Contains(err.Error(), "relay") {
		t.Fatalf("validation should precede transport: %v", err)
	}
}

// TestGqlQueryKindMismatch rejects running mutations under query and back.
func TestGqlQueryKindMismatch(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	for _, tc := range []struct{ verb, op string }{
		{"query", "BankingDisconnectOlbAccounts"},
		{"mutate", "AccountingAccounts"},
	} {
		root := newRootCmd(&rootFlags{})
		root.SetOut(new(bytes.Buffer))
		root.SetErr(new(bytes.Buffer))
		root.SetArgs([]string{"gql", tc.verb, tc.op})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "qb gql "+otherVerb(tc.verb)) {
			t.Errorf("%s %s: wrong error: %v", tc.verb, tc.op, err)
		}
	}
}

func otherVerb(v string) string {
	if v == "query" {
		return "mutate"
	}
	return "query"
}

func TestGqlTreeRegistered(t *testing.T) {
	t.Parallel()
	root := newRootCmd(&rootFlags{})
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
		if c.Name() == "gql" {
			sub := map[string]bool{}
			for _, s := range c.Commands() {
				sub[s.Name()] = true
			}
			for _, want := range []string{"ops", "query", "mutate", "walk"} {
				if !sub[want] {
					t.Errorf("gql missing subcommand %s", want)
				}
			}
		}
	}
	if !names["gql"] {
		t.Fatal("gql not registered on root")
	}
}

func TestGqlWalkRejectsMutation(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	root := newRootCmd(&rootFlags{})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"gql", "walk", "BankingDisconnectOlbAccounts"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "queries only") {
		t.Fatalf("walk of mutation: %v", err)
	}
}

func TestEndpointHostTrim(t *testing.T) {
	t.Parallel()
	if got := endpointHost(gql.EndpointWarehouse); got != "warehouse-management-svc.api.intuit.com/graphql" {
		t.Fatalf("got %q", got)
	}
}

// TestGqlNamedWalkRegistered proves bills/tasks/items hang off `qb gql
// walk` with their date flags bound; a regression dropping one fails here.
func TestGqlNamedWalkRegistered(t *testing.T) {
	t.Parallel()
	root := newRootCmd(&rootFlags{})
	gqlCmd, _, err := root.Find([]string{"gql"})
	if err != nil {
		t.Fatal(err)
	}
	walkCmd, _, err := gqlCmd.Find([]string{"walk"})
	if err != nil || walkCmd.Name() != "walk" {
		t.Fatal("gql missing walk command")
	}
	for _, name := range []string{"bills", "tasks", "items"} {
		sub, _, err := walkCmd.Find([]string{name})
		if err != nil || sub.Name() != name {
			t.Errorf("gql walk missing subcommand %s", name)
			continue
		}
		if name != "items" {
			if sub.Flags().Lookup("from") == nil || sub.Flags().Lookup("to") == nil {
				t.Errorf("%s missing --from/--to flags", name)
			}
		}
	}
}

// TestGqlWalkBillsBadDateFailsFast pins input validation: a malformed
// --from is rejected before any relay dial (dead-port env would hang a
// regression that reaches the network).
func TestGqlWalkBillsBadDateFailsFast(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	root := newRootCmd(&rootFlags{})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"gql", "walk", "bills", "--from", "13/45/9999"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--from") {
		t.Fatalf("bad --from accepted: %v", err)
	}
}

// TestGqlWalkBillsReversedWindowFailsFast: from after to is user error.
func TestGqlWalkBillsReversedWindowFailsFast(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	root := newRootCmd(&rootFlags{})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"gql", "walk", "bills", "--from", "31/12/2024", "--to", "01/01/2024"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "after --to") {
		t.Fatalf("reversed window accepted: %v", err)
	}
}
