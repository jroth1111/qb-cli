package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql/routes"
)

// gqlMutationFlagVerbs lists every named `qb gql mutate` verb and the catalog
// operation it must route to. Membership here is the acceptance contract for
// the top-8 list.
var gqlMutationFlagVerbs = []struct{ verb, op string }{
	{"create-account", "CreateAccount"},
	{"bank-disconnect", "BankingDisconnectOlbAccounts"},
	{"batch-update-products", "BatchUpdateProducts"},
	{"assign-dimensions", "AssignDimensionsForProducts"},
	{"cancel-reconcile", "CancelReconcile__integration_banking_reconcile_ui_qbo"},
	{"receive-inventory", "CommerceReceiveInventory"},
	{"consume-inventory", "CommerceConsumeInventory"},
	{"activate-products", "UpdateItemStatus"},
}

// runGqlMutate executes `qb gql mutate ...` in-process against a dead relay,
// so any attempt to dial the network fails loudly instead of passing.
func runGqlMutate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:1")
	root := newRootCmd(&rootFlags{})
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"gql", "mutate"}, args...))
	err := root.Execute()
	return buf.String(), err
}

// gqlMutateSub returns the live `mutate` command from a freshly built tree.
func gqlMutateSub(t *testing.T) *cobra.Command {
	t.Helper()
	root := newRootCmd(&rootFlags{})
	var mutate *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "gql" {
			for _, s := range c.Commands() {
				if s.Name() == "mutate" {
					mutate = s
				}
			}
		}
	}
	if mutate == nil {
		t.Fatal("qb gql mutate not registered")
	}
	return mutate
}

func TestGqlMutationFlagSubcommandsRegistered(t *testing.T) {
	t.Parallel()
	mutate := gqlMutateSub(t)
	have := map[string]*cobra.Command{}
	for _, sub := range mutate.Commands() {
		have[sub.Name()] = sub
	}
	for _, want := range gqlMutationFlagVerbs {
		sub, ok := have[want.verb]
		if !ok {
			t.Errorf("qb gql mutate missing named subcommand %q (routes to %s)", want.verb, want.op)
			continue
		}
		_ = sub
		// Every named verb must route to a real captured mutation with a
		// declared variable signature — a typo in the spec table would
		// otherwise fail only at runtime, mid-request.
		op, err := gql.Lookup(want.op)
		if err != nil {
			t.Errorf("%s routes to %q which is not in the catalog: %v", want.verb, want.op, err)
			continue
		}
		if op.Kind != gql.KindMutation {
			t.Errorf("%s routes to %q which is a %s", want.verb, want.op, op.Kind)
		}
		if sig, err := gql.LookupMutation(want.op); err != nil || sig == "" {
			t.Errorf("%s routes to %q without a variable signature: %q, %v", want.verb, want.op, sig, err)
		}
	}
}

// TestGqlMutationFlagRequiredFlagsRejected runs each verb with no flags at
// all: every spec must refuse locally (ExitInputError) before any transport.
// Rejects the failure mode where build() ships an empty variables map and
// lets the provider return PLT-2000.
func TestGqlMutationFlagRequiredFlagsRejected(t *testing.T) {
	for _, want := range gqlMutationFlagVerbs {
		_, err := runGqlMutate(t, want.verb)
		if err == nil {
			t.Errorf("%s accepted zero flags; required-flag validation missing", want.verb)
			continue
		}
		exitErr, ok := err.(*ExitError)
		if !ok || exitErr.Code != ExitInputError {
			t.Errorf("%s: expected ExitInputError, got %T (%v)", want.verb, err, err)
			continue
		}
		if strings.Contains(strings.ToLower(err.Error()), "relay") {
			t.Errorf("%s: validation should precede transport: %v", want.verb, err)
		}
	}
}

// TestGqlCreateAccountValidatesAndReachesTransport proves the happy path
// passes local validation and proceeds to the relay dial — on a dead relay
// that surfaces as ExitRelayError, never ExitInputError.
func TestGqlCreateAccountValidatesAndReachesTransport(t *testing.T) {
	out, err := runGqlMutate(t, "create-account",
		"--name", "Test Acct", "--subtype", "Checking", "--number", "1010", "--yes")
	if err == nil {
		t.Fatalf("create-account should fail on dead relay after building vars, got output: %s", out)
	}
	exitErr, ok := err.(*ExitError)
	if !ok || exitErr.Code != ExitRelayError {
		t.Fatalf("expected ExitRelayError from dead relay, got %T (%v)", err, err)
	}
}

// Negative fixture: blank --name must be refused even with other flags set,
// guarding a regression where build() silently drops empty strings and ships
// an input the provider rejects server-side.
func TestGqlCreateAccountRejectsBlankName(t *testing.T) {
	_, err := runGqlMutate(t, "create-account", "--name", "", "--subtype", "Savings")
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("blank --name not rejected: %v", err)
	}
}

// TestGqlCreateAccountLowerCamelInput pins the live-proven wire shape: the
// SPA posts CreateAccount with lower-camel Accounting_CreateAccountInput keys
// ({name, accountType, detailType, ...}) — the PascalCase V2 shape answers
// PLT-2000 on every captured host.
func TestGqlCreateAccountLowerCamelInput(t *testing.T) {
	spec := gqlMutationFlagSpecByVerb(t, "create-account")
	cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Set("name", "Petty Cash")
	cmd.Flags().Set("subtype", "cash and cash equivalents")
	cmd.Flags().Set("description", "drawer float")
	cmd.Flags().Set("number", "1010")
	cmd.Flags().Set("parent-id", "77")
	cmd.Flags().Set("currency", "AUD")
	vars, err := spec.build(cmd)
	if err != nil {
		t.Fatalf("valid create-account args rejected: %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if input == nil {
		t.Fatalf("envelope missing input: %#v", vars)
	}
	want := map[string]any{
		"name":            "Petty Cash",
		"accountType":     "BANK", // --type default, normalized to UPPER_SNAKE
		"detailType":      "CASH_AND_CASH_EQUIVALENTS",
		"description":     "drawer float",
		"number":          "1010",
		"parentAccountId": "77",
		"currency":        "AUD",
	}
	for k, v := range want {
		if input[k] != v {
			t.Errorf("input[%q] = %#v; want %#v (keys %#v)", k, input[k], v, input)
		}
	}
	for _, stale := range []string{"Name", "AccountType", "AccountSubType", "Description", "AcctNum", "ParentAccountId", "Currency"} {
		if _, ok := input[stale]; ok {
			t.Errorf("input carries stale PascalCase key %q: %#v", stale, input)
		}
	}
}

// TestGqlCreateAccountRouteOverride proves the verified coa-core route
// override is wired through the shared routes table into the catalog op the
// named verb executes — the defect was CreateAccountV2 dialing the v4
// gateway that answers PLT-2000.
func TestGqlCreateAccountRouteOverride(t *testing.T) {
	if endpoint, found := routes.Lookup("CreateAccount"); !found || endpoint != routes.EndpointCoaCore {
		t.Fatalf("routes.Lookup(CreateAccount) = (%q, %v); want (%q, true)",
			endpoint, found, routes.EndpointCoaCore)
	}
	if routes.EndpointCoaCore != "https://coa-core.api.intuit.com/graphql" {
		t.Fatalf("EndpointCoaCore = %q; want https://coa-core.api.intuit.com/graphql", routes.EndpointCoaCore)
	}
	op, err := gql.Lookup("CreateAccount")
	if err != nil {
		t.Fatalf("CreateAccount not in catalog: %v", err)
	}
	if op.Endpoint != routes.EndpointCoaCore {
		t.Fatalf("catalog CreateAccount endpoint = %q; want verified override %q", op.Endpoint, routes.EndpointCoaCore)
	}
	spec := gqlMutationFlagSpecByVerb(t, "create-account")
	if spec.opName != "CreateAccount" {
		t.Fatalf("create-account routes to %q; want CreateAccount", spec.opName)
	}
}

func TestEffectiveMutationEndpointUsesRequestOverride(t *testing.T) {
	op, err := gql.Lookup("CreateAccount")
	if err != nil {
		t.Fatal(err)
	}
	override := "https://example.invalid/graphql"
	if got := effectiveMutationEndpoint(gql.Request{Op: op, Endpoint: override}); got != override {
		t.Fatalf("effective endpoint = %q; want override %q", got, override)
	}
	if got := effectiveMutationEndpoint(gql.Request{Op: op}); got != op.Endpoint {
		t.Fatalf("effective endpoint without override = %q; want catalog endpoint %q", got, op.Endpoint)
	}
}

func TestGqlBankDisconnectTrimsDisplayIds(t *testing.T) {
	spec := gqlMutationFlagSpecByVerb(t, "bank-disconnect")
	cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Set("olb-account-id", "12345:bktdig:123456789")
	cmd.Flags().Set("olb-account-id", "555")
	vars, err := spec.build(cmd)
	if err != nil {
		t.Fatalf("valid olb ids rejected: %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if input == nil {
		t.Fatalf("envelope missing input: %#v", vars)
	}
	ids, _ := input["olbAccountIds"].([]string)
	if len(ids) != 2 || ids[0] != "123456789" || ids[1] != "555" {
		t.Fatalf("olbAccountIds = %#v; want [123456789 555] (display-id suffix kept)", input["olbAccountIds"])
	}
}

// TestGqlBankDisconnectRequiresIds rejects the plausible bug of treating an
// unset StringSlice as one empty string and sending {"olbAccountIds":[""]}.
func TestGqlBankDisconnectRequiresIds(t *testing.T) {
	spec := gqlMutationFlagSpecByVerb(t, "bank-disconnect")
	cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd.SetOut(new(bytes.Buffer))
	if _, err := spec.build(cmd); err == nil || !strings.Contains(err.Error(), "--olb-account-id") {
		t.Fatalf("missing ids accepted: %v", err)
	}
	cmd2 := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.Flags().Set("olb-account-id", "")
	if _, err := spec.build(cmd2); err == nil {
		t.Fatalf("blank id accepted: built %#v", func() any { v, _ := spec.build(cmd2); return v }())
	}
}

func TestGqlBatchUpdateProductsJSONArg(t *testing.T) {
	spec := gqlMutationFlagSpecByVerb(t, "batch-update-products")

	dir := t.TempDir()
	file := filepath.Join(dir, "rows.json")
	rows := []map[string]any{{"id": "72", "version": 0}}
	b, _ := json.Marshal(rows)
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Set("products-json", "@"+file)
	vars, err := spec.build(cmd)
	if err != nil {
		t.Fatalf("@file rows rejected: %v", err)
	}
	gotRows, ok := vars["products"].([]any)
	if !ok || len(gotRows) != 1 {
		t.Fatalf("products = %#v; want one row from file", vars["products"])
	}

	// Inline garbage must fail as input error, never reach transport.
	cmd2 := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd2.SetOut(new(bytes.Buffer))
	cmd2.Flags().Set("products-json", "{not json")
	err = cmd2.Execute()
	if err == nil {
		t.Fatal("malformed products JSON accepted")
	}
	if exitErr, ok := err.(*ExitError); !ok || exitErr.Code != ExitInputError {
		t.Fatalf("malformed JSON should tag ExitInputError, got %T (%v)", err, err)
	}

	// An empty array is structurally valid JSON but semantically useless:
	// batchUpdateProducts declares [Commerce_BatchUpdateProductInput!]!.
	cmd3 := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd3.SetOut(new(bytes.Buffer))
	cmd3.Flags().Set("products-json", "[]")
	if _, err := spec.build(cmd3); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty rows array accepted: %v", err)
	}
}

func TestGqlCancelReconcileEnvelope(t *testing.T) {
	spec := gqlMutationFlagSpecByVerb(t, "cancel-reconcile")
	cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Set("reconciliation-id", "12345:integration/Reconciliation:55")
	vars, err := spec.build(cmd)
	if err != nil {
		t.Fatalf("valid reconciliation id rejected: %v", err)
	}
	input, _ := vars["input_0"].(map[string]any)
	if input == nil {
		t.Fatalf("envelope must use $input_0 per captured signature: %#v", vars)
	}
	recon, _ := input["integrationReconciliation"].(map[string]any)
	if recon == nil || recon["id"] == "" || recon["reconcileAction"] == "" {
		t.Fatalf("integrationReconciliation incomplete: %#v", input)
	}
	if _, ok := input["pluginInfo"]; !ok {
		t.Fatalf("pluginInfo envelope layer missing (SPA always sends it): %#v", input)
	}

	// Missing id must be refused.
	cmd2 := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd2.SetOut(new(bytes.Buffer))
	if _, err := spec.build(cmd2); err == nil || !strings.Contains(err.Error(), "--reconciliation-id") {
		t.Fatalf("missing --reconciliation-id accepted: %v", err)
	}
}

func TestGqlCommerceInventoryEnvelope(t *testing.T) {
	for _, verb := range []string{"receive-inventory", "consume-inventory"} {
		spec := gqlMutationFlagSpecByVerb(t, verb)
		cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
		cmd.SetOut(new(bytes.Buffer))
		cmd.Flags().Set("txn-id", "198")
		cmd.Flags().Set("txn-type", "Bill")
		cmd.Flags().Set("source-version", "1")
		cmd.Flags().Set("txn-date", "2026-08-24")
		cmd.Flags().Set("lines-json", `[{"itemId":"72","quantity":3,"inventoryLocationId":"L1","sourceTransactionLineId":"9","lineOrder":1}]`)
		vars, err := spec.build(cmd)
		if err != nil {
			t.Fatalf("%s: valid args rejected: %v", verb, err)
		}
		input, _ := vars["input"].(map[string]any)
		src, _ := input["sourceTransaction"].(map[string]any)
		lines, _ := input["lines"].([]any)
		if src == nil || src["txnId"] != "198" || src["txnType"] != "Bill" || src["txnDate"] != "2026-08-24" {
			t.Fatalf("%s: sourceTransaction wrong: %#v", verb, src)
		}
		if src["version"] != 1 {
			t.Fatalf("%s: sourceTransaction.version = %#v; want 1 (NonNull Int!)", verb, src["version"])
		}
		if len(lines) != 1 {
			t.Fatalf("%s: lines not threaded through: %#v", verb, lines)
		}

		// Lines are mandatory — an inventory movement without rows would be
		// rejected by the warehouse service, so refuse locally instead.
		cmd2 := newGqlMutationFlagCmd(&rootFlags{}, spec)
		cmd2.SetOut(new(bytes.Buffer))
		cmd2.Flags().Set("txn-id", "198")
		cmd2.Flags().Set("txn-type", "Bill")
		cmd2.Flags().Set("source-version", "1")
		if _, err := spec.build(cmd2); err == nil || !strings.Contains(err.Error(), "lines-json") {
			t.Fatalf("%s: missing lines accepted: %v", verb, err)
		}

		// txnId without txnType mirrors the SPA's own guard ordering.
		cmd3 := newGqlMutationFlagCmd(&rootFlags{}, spec)
		cmd3.SetOut(new(bytes.Buffer))
		cmd3.Flags().Set("txn-id", "198")
		if _, err := spec.build(cmd3); err == nil || !strings.Contains(err.Error(), "--txn-type") {
			t.Fatalf("%s: missing --txn-type accepted: %v", verb, err)
		}
	}
}

// TestGqlCommerceInventorySourceVersion pins the live-proven contract: the
// warehouse service coerces a missing sourceTransaction.version to Null for
// a NonNull Int! and rejects the call. Missing, zero, and negative values
// must fail locally as input errors before any transport attempt.
func TestGqlCommerceInventorySourceVersion(t *testing.T) {
	for _, verb := range []string{"receive-inventory", "consume-inventory"} {
		spec := gqlMutationFlagSpecByVerb(t, verb)
		for _, bad := range []string{"", "0", "-2"} {
			cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
			cmd.SetOut(new(bytes.Buffer))
			cmd.Flags().Set("txn-id", "198")
			cmd.Flags().Set("txn-type", "Bill")
			cmd.Flags().Set("lines-json", `[{"itemId":"72","quantity":3,"sourceTransactionLineId":"9","lineOrder":1}]`)
			if bad != "" {
				cmd.Flags().Set("source-version", bad)
			}
			if _, err := spec.build(cmd); err == nil || !strings.Contains(err.Error(), "--source-version") {
				t.Fatalf("%s: --source-version %q accepted: %v", verb, bad, err)
			}
		}
	}
}

func TestGqlActivateProductsEntities(t *testing.T) {
	spec := gqlMutationFlagSpecByVerb(t, "activate-products")
	cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Set("entity-id", "72")
	cmd.Flags().Set("entity-id", "73")
	vars, err := spec.build(cmd)
	if err != nil {
		t.Fatalf("entity ids rejected: %v", err)
	}
	entities, _ := vars["entities"].([]any)
	if len(entities) != 2 {
		t.Fatalf("entities = %#v; want two entries", entities)
	}
	first, _ := entities[0].(map[string]any)
	if first["id"] != "72" || first["version"] != 0 || first["entityType"] != "INVENTORY" || first["actionType"] != "ACTIVATE" {
		t.Fatalf("entity[0] = %#v; want id=72 version=0 entityType=INVENTORY actionType=ACTIVATE defaults", first)
	}

	// Neither flag: refused.
	cmd2 := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd2.SetOut(new(bytes.Buffer))
	if _, err := spec.build(cmd2); err == nil || !strings.Contains(err.Error(), "--entity-id") {
		t.Fatalf("no-entity invocation accepted: %v", err)
	}
}

// gqlMutationFlagSpecByVerb fetches a production spec by its CLI verb so
// tests exercise exactly what ships, never a copy.
func gqlMutationFlagSpecByVerb(t *testing.T, verb string) gqlMutationFlagSpec {
	t.Helper()
	for _, spec := range gqlMutationFlagSpecTable() {
		if spec.use == verb {
			return spec
		}
	}
	t.Fatalf("spec for verb %q vanished from production table", verb)
	return gqlMutationFlagSpec{}
}

// TestGqlMutateNamedVerbDispatch proves `mutate <named>` routes to the flag
// command rather than being treated as an unknown catalog op name, while a
// genuine catalog name still takes the generic --vars path.
func TestGqlMutateNamedVerbDispatch(t *testing.T) {
	// A catalog mutation invoked by name must NOT be hijacked by dispatch:
	// CommerceConsumeInventory requires -$input, and the generic path must
	// say exactly that instead of running some named subcommand.
	_, err := runGqlMutate(t, "CommerceConsumeInventory")
	if err == nil || !strings.Contains(err.Error(), "-$input") {
		t.Fatalf("generic path should validate required vars: %v", err)
	}

	// Unknown garbage still reports unknown-mutation with suggestions.
	_, err = runGqlMutate(t, "NoSuchMutationAnywhere")
	if err == nil || !strings.Contains(err.Error(), "unknown GraphQL") {
		t.Fatalf("unknown mutation misreported: %v", err)
	}
}

// TestGqlActivateProductsUnknownEntityShape guards --entities-json decoding
// to a non-array (a stray object), which must fail as input error.
func TestGqlActivateProductsRejectsObjectJSON(t *testing.T) {
	spec := gqlMutationFlagSpecByVerb(t, "activate-products")
	cmd := newGqlMutationFlagCmd(&rootFlags{}, spec)
	cmd.SetOut(new(bytes.Buffer))
	cmd.Flags().Set("entities-json", `{"id":"72"}`)
	if _, err := spec.build(cmd); err == nil || !strings.Contains(err.Error(), "array") {
		t.Fatalf("object JSON accepted for entities array: %v", err)
	}
}

// Compile-time proof the verbs table stays aligned with the production
// spec list: a renamed or dropped verb breaks both this and registration.
func TestGqlMutationFlagVerbsCoverSpecs(t *testing.T) {
	t.Parallel()
	specs := gqlMutationFlagSpecTable()
	if len(specs) != len(gqlMutationFlagVerbs) {
		t.Fatalf("spec table has %d entries but tests pin %d; keep them in sync", len(specs), len(gqlMutationFlagVerbs))
	}
	for _, want := range gqlMutationFlagVerbs {
		found := false
		for _, spec := range specs {
			if spec.use == want.verb && spec.opName == want.op {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("verb/op pair %+v missing from production spec table", want)
		}
	}
}
