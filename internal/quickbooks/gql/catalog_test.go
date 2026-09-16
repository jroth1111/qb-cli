package gql

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestLookupKnownOperations(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"GetBills", "BankingDisconnectOlbAccounts", "AccountingAccounts"} {
		op, err := Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		if op.Name != name {
			t.Fatalf("Lookup(%q) returned %q", name, op.Name)
		}
		if op.Document == "" {
			t.Fatalf("%s has empty document", name)
		}
		if op.Endpoint == "" {
			t.Fatalf("%s has no endpoint", name)
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	t.Parallel()
	_, err := Lookup("NoSuchOperationZzz")
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
	var nf *ErrNotFound
	if _, ok := err.(*ErrNotFound); !ok && !strings.Contains(err.Error(), "unknown GraphQL operation") {
		t.Fatalf("wrong error type: %T (%v)", err, err)
	}
	_ = nf
	if !strings.Contains(err.Error(), "qb gql ops") {
		t.Fatalf("error should point at `qb gql ops`: %v", err)
	}
}

func TestOpsFilterAndSort(t *testing.T) {
	t.Parallel()
	all := Ops("")
	if len(all) < 100 {
		t.Fatalf("expected full catalog >= 100 ops, got %d", len(all))
	}
	bills := Ops("bills") // case-insensitive substring
	if len(bills) == 0 {
		t.Fatal("expected Bills filter to match GetBills")
	}
	found := false
	for i, op := range bills {
		if i > 0 && strings.ToLower(bills[i-1].Name) > strings.ToLower(op.Name) {
			t.Fatalf("Ops not sorted at %d", i)
		}
		if op.Name == "GetBills" {
			found = true
		}
	}
	if !found {
		t.Fatal("filter 'bills' did not return GetBills")
	}
	if got := Count(); got != len(all) {
		t.Fatalf("Count()=%d but Ops(\"\")=%d", got, len(all))
	}
}

// Negative fixture: an op whose document declares only $input must reject
// unknown variables and accept declared ones. This rejects the failure mode
// where the gateway silently forwards typos to the server.
func TestValidateVarsRejectsUnknown(t *testing.T) {
	t.Parallel()
	op, err := Lookup("BankingDisconnectOlbAccounts")
	if err != nil {
		t.Fatal(err)
	}
	if err := op.ValidateVars(map[string]any{"input": map[string]any{}}); err != nil {
		t.Fatalf("declared var rejected: %v", err)
	}
	err = op.ValidateVars(map[string]any{"olbAccountId": "10"})
	if err == nil {
		t.Fatal("unknown var accepted — validation is broken")
	}
	if !strings.Contains(err.Error(), "$olbAccountId") || !strings.Contains(err.Error(), "does not declare") {
		t.Fatalf("error should name the bad var: %v", err)
	}
	if !strings.Contains(err.Error(), "declared variables: $input") {
		t.Fatalf("error should list declared variables: %v", err)
	}
}

func TestValidateVarsEmptyIsNoop(t *testing.T) {
	t.Parallel()
	op := &Op{Name: "X", Document: "query X { f }"}
	if err := op.ValidateVars(nil); err != nil {
		t.Fatalf("nil vars on no-var op: %v", err)
	}
	if err := op.ValidateVars(nil); err != nil {
		return
	}
}

func TestDeclaredVarsOrder(t *testing.T) {
	t.Parallel()
	op, err := Lookup("GetBills")
	if err != nil {
		t.Fatal(err)
	}
	got := op.DeclaredVars()
	want := []string{"first", "after", "orderBy", "filter"}
	if len(got) != len(want) {
		t.Fatalf("DeclaredVars=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DeclaredVars[%d]=%q want %q (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestDeclaredVarsNewlineCatalog(t *testing.T) {
	op, err := Lookup("GetContacts")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(op.DeclaredVars(), ","); got != "first,offset,filter" {
		t.Fatalf("DeclaredVars = %q", got)
	}
	if err := op.ValidateVars(map[string]any{"offset": 100, "filter": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if got := op.DetectWalkStyle(); got != WalkOffset {
		t.Fatalf("style = %q, want offset", got)
	}
}

func TestDeclaredVarsWhitespaceAndSelectionArguments(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{"query Q($first \t: Int\n$offset: Int) { rows }", "first,offset"},
		{"query Q { rows(first: $notDeclared) { id } }", ""},
	} {
		op := &Op{Kind: KindQuery, Document: tc.doc}
		if got := strings.Join(op.DeclaredVars(), ","); got != tc.want {
			t.Errorf("DeclaredVars(%q) = %q, want %q", tc.doc, got, tc.want)
		}
	}
}

// DetectWalkStyle must classify from the document's own variable names.
// Each style branch below rejects a distinct misclassification bug.
func TestDetectWalkStyle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		doc  string
		want WalkStyle
	}{
		{"query A($first: Int!, $after: String) { c(first: $first, after: $after) { pageInfo { hasNextPage endCursor } } }", WalkCursor},
		{"query B($first: Int!, $offset: Int!) { c(first: $first, offset: $offset) { nodes { id } } }", WalkOffset},
		{"query C($limit: Int!) { c(limit: $limit) { id } }", WalkSingle},
		{"mutation D($input: X!) { d(input: $input) { id } }", WalkSingle},
	}
	for _, tc := range cases {
		op := &Op{Name: "T", Kind: KindQuery, Document: tc.doc}
		if got := op.DetectWalkStyle(); got != tc.want {
			t.Errorf("DetectWalkStyle(%q)=%q want %q", tc.doc, got, tc.want)
		}
	}
}

func TestExtractNodesCursorPage(t *testing.T) {
	t.Parallel()
	body := `{"data":{"commerceBills":{"pageInfo":{"hasNextPage":true,"endCursor":"CUR1"},
		"edges":[{"node":{"id":"1"}},{"node":{"id":"2"}}]}}}`
	nodes, hasNext, cursor, err := extractNodes(json.RawMessage(body))
	if err != nil {
		t.Fatal(err)
	}
	if !hasNext || cursor != "CUR1" || len(nodes) != 2 {
		t.Fatalf("nodes=%v hasNext=%v cursor=%q", nodes, hasNext, cursor)
	}
	var first struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(nodes[0], &first); err != nil || first.ID != "1" {
		t.Fatalf("node 0 decode: %v (%+v)", err, first)
	}
}

func TestExtractNodesNoConnection(t *testing.T) {
	t.Parallel()
	nodes, hasNext, cursor, err := extractNodes(json.RawMessage(`{"data":{"company":{"id":"x"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 || hasNext || cursor != "" {
		t.Fatalf("expected empty walk state, got %d nodes", len(nodes))
	}
}

func TestResponseDataDecodes(t *testing.T) {
	t.Parallel()
	resp := &Response{Status: 200, Body: json.RawMessage(`{"data":{"bill":{"id":"9"}},"errors":null}`)}
	var out struct {
		Bill struct {
			ID string `json:"id"`
		} `json:"bill"`
	}
	if err := resp.Data(&out); err != nil {
		t.Fatal(err)
	}
	if out.Bill.ID != "9" {
		t.Fatalf("got %q", out.Bill.ID)
	}
}

// BrokenTemplate documents must still be listed (discoverable) even though
// they refuse execution — the flag exists so callers can short-circuit.
func TestBrokenTemplatesAreFlaggedNotDropped(t *testing.T) {
	t.Parallel()
	flagged := 0
	for _, op := range Ops("") {
		if op.BrokenTemplate {
			flagged++
			if !strings.Contains(op.Document, ".concat(") && !strings.ContainsAny(op.Document, "\n") {
				t.Fatalf("%s flagged broken without artifact", op.Name)
			}
		}
	}
	if flagged == 0 {
		t.Fatal("corpus contains known broken templates; expected some flagged")
	}
}

// templateHoleRe mirrors the generator's hole detector: any surviving
// `${var}` in a shipped document is a verbatim-to-server syntax error.
var templateHoleRe = regexp.MustCompile(`\$\{[A-Za-z_]\w*\}`)

// Unflagged documents must contain no webpack template holes: ego_execute
// sends Document verbatim, so a surviving `${N}` would reach the server as
// a GraphQL syntax error (as happened with CommerceReceiveInventory, fixed
// by inlining the live expenses-forms-ui fragment via gen).
func TestUnflaggedDocsHaveNoTemplateHoles(t *testing.T) {
	t.Parallel()
	for _, op := range Ops("") {
		if !op.BrokenTemplate && templateHoleRe.MatchString(op.Document) {
			t.Errorf("%s has a template hole but BrokenTemplate=false", op.Name)
		}
	}
}
