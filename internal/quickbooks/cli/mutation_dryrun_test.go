package cli

// mutation_dryrun_test.go is the exhaustive dry-run audit: every wired
// mutation (catalog + inline inventory entries) is executed with --dry-run
// against a credential-free QB_HOME and must emit a secret-free plan
// envelope without dialing.
//
// Per entry the harness:
//   - resolves the command through the REAL root tree (same path a user
//     invocation takes)
//   - supplies representative values for every required flag (cobra
//     MarkFlagRequired + catalog paramDoc.Required) and every flag the
//     mutation contract says the command consumes — a consumed flag that
//     fails validation here exposes a contract bug
//   - asserts the envelope is dry_run:true, dial:false, carries a planned
//     method/URL or an explicit "no captured API" note
//   - asserts every probed flag is echoed in the envelope's flag map
//     (alias-aware: folded aliases like --category → category-id count)
//
// Commands whose required inputs cannot be probed by name shape live in
// dryRunProbeArgs — the override table is the audit record of which
// mutations need domain-shaped values (JSON payloads, olbTxnIds, RFC3339
// dates, live-blocked account ids).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Representative JSON payloads reused across the table.
const (
	dryRunSalesLine   = `[{"Amount":1,"DetailType":"SalesItemLineDetail","SalesItemLineDetail":{"ItemRef":{"value":"1"}}}]`
	dryRunExpenseLine = `[{"Amount":1,"DetailType":"AccountBasedExpenseLineDetail","AccountBasedExpenseLineDetail":{"AccountRef":{"value":"76"}}}]`
)

// dryRunProbeArgs carries per-command representative args that generic
// name-based probing cannot synthesize: required JSON payloads, feed
// olbTxnIds, gql relay selectors, and commands whose required values have a
// domain shape (CSV import refuses the live accounts 204/93 — 44 is the
// help-documented test account).
var dryRunProbeArgs = map[string][]string{
	// --- bank-feed subsystem ---
	"feed txn import":               {"--account-id", "44", "--description", "audit-probe", "--amount", "1.00"},
	"feed txn update exclude":       {"--ids", "olb-1"},
	"feed txn update undo-excluded": {"--ids", "olb-1"},
	"feed txn update unpost":        {"--ids", "olb-1"},
	"feed txn update batch-accept":  {"--ids", "olb-1"},
	"feed txn update categorise":    {"--ids", "olb-1", "--category", "7"},
	"feed txn update match":         {"--ids", "olb-1", "--match-id", "1"},
	"feed txn update split":         {"--ids", "olb-1", "--lines", "7=-6,8=-4"},
	"feed txn run transfer":         {"--ids", "olb-1", "--to-account-id", "2"},
	"feed txn run attach":           {"--id", "1", "--txn-type", "Invoice", "--note", "audit-probe"},
	"feed rec run":                  {"--account-id", "1", "--ending-date", "2024-06-30", "--adjust-account", "2"},
	// --- relay GraphQL mutations ---
	"gql mutate activate-products":     {"--entity-id", "1"},
	"gql mutate assign-dimensions":     {"--input-json", `{"productId":"1"}`},
	"gql mutate bank-disconnect":       {"--id", "1"},
	"gql mutate batch-update-products": {"--products-json", `[{"id":"1"}]`},
	"gql mutate cancel-reconcile":      {"--reconciliation-id", "1"},
	"gql mutate consume-inventory":     {"--txn-id", "1", "--txn-type", "Bill", "--lines-json", `[{"itemId":"1","qty":1}]`, "--source-version", "1"},
	"gql mutate receive-inventory":     {"--txn-id", "1", "--txn-type", "Bill", "--lines-json", `[{"itemId":"1","qty":1}]`, "--source-version", "1"},
	"gql mutate create-account":        {"--name", "AuditProbe", "--subtype", "Checking"},
	// --- report/forecast builders ---
	"reports custom create":      {"--name", "AuditProbe"},
	"reports forecast create":    {"--name", "AuditProbe", "--start", "2024-01-01", "--end", "2024-12-31"},
	"reports forecast delete":    {"--id", "1"},
	"reports forecast update":    {"--id", "1", "--name", "AuditProbe"},
	"reports management create":  {"--name", "AuditProbe"},
	"reports management update":  {"--id", "1", "--name", "AuditProbe"},
	"reports performance create": {"--name", "AuditProbe", "--metric", "net_income"},
	"reports performance delete": {"--id", "1"},
	"reports performance update": {"--id", "1", "--name", "AuditProbe"},
	"reports report create":      {"--name", "AuditProbe"},
	"reports report delete":      {"--id", "1"},
	"reports report update":      {"--id", "1", "--name", "AuditProbe"},
	// --- proposals / cost groups / tasks / workflows ---
	"customers proposal create":     {"--customer", "1"},
	"customers proposal update":     {"--id", "00000000-0000-0000-0000-000000000001"},
	"customers proposal delete":     {"--id", "00000000-0000-0000-0000-000000000001"},
	"customers proposal-svc create": {"--customer", "1"},
	"costgroups create":             {"--name", "AuditProbe"},
	"costgroups update":             {"--id", "1"},
	"costgroups delete":             {"--id", "1"},
	"advanced tasks create":         {"--name", "AuditProbe", "--due", "2024-06-30"},
	"advanced tasks update":         {"--id", "1", "--name", "AuditProbe"},
	"advanced tasks delete":         {"--id", "1"},
	"advanced workflows create":     {"--name", "AuditProbe"},
	"advanced workflows update":     {"--id", "1"},
	"advanced workflows delete":     {"--id", "1"},
	// --- dedicated accounting payloads ---
	"accounting deferredrevenue create": {"--data", `{"accountId":"1"}`},
	"accounting prepaid create":         {"--data", `{"accountId":"1"}`},
	"accounting journal create":         {"--items-json", `[{"date":"2024-06-30","amount":1,"from-account":"1","to-account":"2"}]`},
	"accounting journal delete":         {"--ids", "1"},
	"accounting opening-balance create": {"--date", "30/06/2024", "--lines", `[{"account":"1","debit":1},{"account":"2","credit":1}]`},
	"accounting reconcile create":       {"--account", "1", "--ending-balance", "1", "--ending-date", "30/06/2024"},
	// generic v3 void: entity selector + target
	"accounting txn update": {"--entity", "invoice", "--id", "1"},
	// dimension commands bind --id by hand (not cobra-required)
	"accounting class update":    {"--id", "1"},
	"accounting class delete":    {"--id", "1"},
	"accounting location update": {"--id", "1"},
	"accounting location delete": {"--id", "1"},
	// --- v3 creates whose RunE refuses a missing line array ---
	"sales invoice create":            {"--customer", "1", "--line-items", dryRunSalesLine},
	"sales estimate create":           {"--customer", "1", "--line-items", dryRunSalesLine},
	"sales credit-memo create":        {"--customer", "1", "--line-items", dryRunSalesLine},
	"sales receipt create":            {"--customer", "1", "--line-items", dryRunSalesLine},
	"sales refund-receipt create":     {"--customer", "1", "--line-items", dryRunSalesLine},
	"expenses expense create":         {"--line-items", dryRunExpenseLine},
	"expenses cheque create":          {"--line-items", dryRunExpenseLine},
	"expenses bill create":            {"--line-items", dryRunExpenseLine, "--supplier", "1"},
	"expenses vendor-credit create":   {"--line-items", dryRunExpenseLine, "--supplier", "1"},
	"inventory purchase-order create": {"--items", dryRunExpenseLine, "--supplier", "1"},
	// --- send ops bind --id as required ---
	"sales invoice run send":     {"--id", "1"},
	"sales estimate run send":    {"--id", "1"},
	"sales credit-memo run send": {"--id", "1"},
	"sales receipt run send":     {"--id", "1"},
	// --- misc dedicated paths ---
	"tax code create":                         {"--name", "AuditProbe", "--rates-json", `[{"TaxRateRef":{"value":"1"},"RateValue":1,"TaxAgencyRef":{"value":"1"},"TaxApplicableOn":"Sales"}]`},
	"advanced batch run":                      {"--file", `[{"method":"GET","url":"/companyinfo/1"}]`},
	"expenses expense update recategorise":    {"--id", "1"},
	"inventory purchase-order update partial": {"--id", "1", "--items", `[{"id":"1","qty":1}]`},
	"accounting project create":               {"--customer", "1", "--name", "AuditProbe"},
	// "@TMPFILE" is substituted with a real temp file the command can stat.
	"company attachable upload": {"--file", "@TMPFILE"},
}

// dryRunProbeValue maps a flag name to a representative value good enough to
// satisfy required-flag and shape validation under --dry-run. The second
// return is false for flags that must not be probed (globals, help).
func dryRunProbeValue(name string, f *pflag.Flag) (string, bool) {
	switch name {
	case "help", "json", "dry-run", "yes", "no-input", "quiet", "home",
		"config", "timeout", "relay-url", "audit-dir":
		return "", false
	}
	switch name {
	case "entity":
		return "invoice", true
	case "txn-type":
		return "Invoice", true
	case "mode":
		return "delete", true
	case "email", "to", "send-to":
		return "audit-probe@example.com", true
	case "file", "path":
		return "@TMPFILE", true
	case "out", "output":
		return "@TMPFILE", true
	case "line-items", "lines", "items", "items-json":
		return dryRunSalesLine, true
	case "data", "input-json", "patch", "payload":
		return "{}", true
	case "products-json", "entities-json", "rates-json", "splits", "payments":
		return "[]", true
	case "keep":
		return "1", true
	case "merge":
		return "2", true
	case "currency", "code":
		return "AUD", true
	case "name", "title", "description", "memo", "note", "notes", "purpose",
		"message", "customer-note", "payee-name", "query":
		return "AuditProbe", true
	case "type":
		return "Bank", true
	case "subtype", "sub-type":
		return "Checking", true
	case "metric":
		return "net_income", true
	case "template-type", "recur-type", "recurring-type":
		return "Unscheduled", true
	case "status":
		return "Active", true
	case "method":
		return "straight-line", true
	case "trip-type":
		return "business", true
	case "vehicle":
		return "audit-vehicle", true
	case "basis":
		return "accrual", true
	case "target":
		return "account", true
	case "format":
		return "pdf", true
	case "payment-type", "pay-type":
		return "Check", true
	case "period":
		return "1", true
	case "entity-type":
		return "Expense", true
	case "match":
		return "1", true
	case "when":
		return "2024-06-30", true
	}
	// Name-shape fallbacks: ids, dates, and numerics.
	switch {
	case name == "id" || strings.HasSuffix(name, "-id") || strings.HasSuffix(name, "-ids") || name == "ids":
		return "1", true
	case name == "date" || strings.HasSuffix(name, "-date") || name == "due" ||
		name == "start" || name == "end" || name == "when":
		return "2024-06-30", true
	case name == "address" || name == "line1":
		return "1 Probe St", true
	case name == "city":
		return "Melbourne", true
	case name == "state" || name == "postcode" || name == "postal":
		return "VIC", true
	case name == "phone":
		return "0300000000", true
	case name == "amount" || name == "rate" || name == "qty" || name == "quantity" ||
		name == "price" || name == "cost" || name == "initial-cost" || name == "salvage" ||
		name == "percent" || name == "discount" || name == "discount-percent" ||
		name == "hours" || name == "minutes" || name == "interval" || name == "interval-type" ||
		name == "days-in-advance" || name == "days-before" || name == "due-days" ||
		name == "due_days" || name == "duedays" || name == "fiscal-year" || name == "life-years" ||
		name == "useful-life" || name == "new-qty" || name == "initial-qty" || name == "counted-qty" ||
		name == "starting-value" || name == "distance" || name == "distance-km" || name == "fee" ||
		name == "ending-balance" || name == "reorder-point" || name == "entity-version" ||
		name == "balance" || name == "limit" || name == "source-version" || name == "exchange-rate" ||
		strings.HasSuffix(name, "-amount") || strings.HasSuffix(name, "-qty") || strings.HasSuffix(name, "-km"):
		return "1", true
	}
	// Anything else gets probed as a generic token rather than skipped — a
	// contract flag that needs a shaped value belongs in the table above.
	return "1", true
}

// dryRunArgs builds the full argv for one mutation entry: command words,
// override args, probed required flags, probed consumed flags, then the
// global --dry-run --json. It returns the argv and the flag→value map the
// run should echo back in the plan envelope.
func dryRunArgs(t *testing.T, e primitiveEntry, tmpFile string) ([]string, map[string]string) {
	t.Helper()
	probe := NewRootCommand()
	cmd := resolveMutationCommand(probe, e)
	if cmd == nil {
		t.Fatalf("%s (%q): wired mutation does not resolve to a real command", e.ID, e.Command)
	}

	args := append([]string{}, strings.Fields(e.Command)...)
	set := map[string]string{}

	put := func(name, val string) {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("%s: probe referenced unattached flag --%s", e.ID, name)
		}
		if val == "@TMPFILE" {
			val = tmpFile
		}
		if f.Value.Type() == "bool" {
			args = append(args, "--"+name+"="+val)
		} else {
			args = append(args, "--"+name, val)
		}
		set[name] = val
	}

	// Overrides first: they carry domain-shaped values that win over generic
	// name probing.
	covered := map[string]bool{}
	overrides := dryRunProbeArgs[e.Command]
	for i := 0; i < len(overrides); i += 2 {
		name := strings.TrimPrefix(overrides[i], "--")
		put(name, overrides[i+1])
		covered[name] = true
	}

	// cobra-required flags (MarkFlagRequired).
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f == nil || covered[f.Name] {
			return
		}
		if f.Annotations[cobra.BashCompOneRequiredFlag] == nil {
			return
		}
		if v, ok := dryRunProbeValue(f.Name, f); ok {
			put(f.Name, v)
		}
	})

	// Catalog-declared required params (enforced in RunE, not by cobra).
	meta := mutationMetaParams[e.ID]
	for _, p := range paramsFor(e.ID) {
		if !p.Required || covered[p.Name] {
			continue
		}
		if _, dead := meta[p.Name]; dead {
			continue // denied at run time — must not be probed
		}
		if f := cmd.Flags().Lookup(p.Name); f != nil {
			if v, ok := dryRunProbeValue(p.Name, f); ok {
				put(p.Name, v)
			}
		}
	}

	// Contract-consumed flags: probe every non-bool member so flag inclusion
	// is exercised end-to-end through ValidateMutationFlags (v3 path) or the
	// dedicated command's own flag map.
	kind, spec := classifyMutation(e, cmd)
	var consumed []string
	switch kind {
	case mutationV3:
		consumed, _ = client.MutationFlagsConsumed(spec.Entity, spec.Op)
	case mutationDedicated:
		consumed = dedicatedConsumed[e.ID]
	}
	for _, name := range consumed {
		if covered[name] || set[name] != "" {
			continue
		}
		f := cmd.Flags().Lookup(name)
		if f == nil || f.Value.Type() == "bool" {
			continue // unattached (alias-satisfied) or a bool modifier
		}
		if _, dead := meta[name]; dead {
			continue
		}
		if v, ok := dryRunProbeValue(name, f); ok {
			put(name, v)
		}
	}

	return append(args, "--dry-run", "--json"), set
}

// envFlagValue reads a flag back from the plan envelope, honoring folded
// aliases (a probe on --category surfaces as category-id after the command's
// alias folding).
func envFlagValue(env planEnvelope, name, want string) bool {
	if got, ok := env.Flags[name]; ok && strings.Contains(got, want) {
		return true
	}
	for _, group := range mutationAliasGroups {
		in := false
		for _, m := range group {
			if m == name {
				in = true
				break
			}
		}
		if !in {
			continue
		}
		for _, m := range group {
			if got, ok := env.Flags[m]; ok && strings.Contains(got, want) {
				return true
			}
		}
	}
	return false
}

func TestMutationDryRunExhaustive(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "probe.json")
	if err := os.WriteFile(tmpFile, []byte("[]"), 0600); err != nil {
		t.Fatalf("write probe file: %v", err)
	}

	var noPlan, noURL []string
	for _, e := range mutationEntries() {
		e := e
		t.Run(e.ID, func(t *testing.T) {
			args, set := dryRunArgs(t, e, tmpFile)
			stdout, elapsed, err := runQB(t, args...)
			if elapsed > 5*time.Second {
				t.Fatalf("%s: dry-run took %s — likely dialed", e.Command, elapsed)
			}
			if err != nil {
				t.Fatalf("%s: dry-run failed: %v\nstdout=%s\nargs=%s", e.Command, err, stdout, strings.Join(args, " "))
			}
			// Decode generically first: the gql relay mutations emit
			// {"dry_run","dial","operation","variables"} instead of the
			// planEnvelope shape — still a valid captured plan.
			var raw map[string]any
			if jerr := json.Unmarshal([]byte(stdout), &raw); jerr != nil {
				t.Fatalf("%s: dry-run output is not JSON: %v\nstdout=%s", e.Command, jerr, stdout)
			}
			if raw["dry_run"] != true || raw["dial"] != false {
				t.Fatalf("%s: envelope dry_run=%v dial=%v", e.Command, raw["dry_run"], raw["dial"])
			}
			env := decodePlan(t, stdout)
			if env.Command != "" && !strings.HasSuffix(env.Command, e.Command) {
				t.Errorf("%s: envelope command %q", e.Command, env.Command)
			}
			_, gqlOp := raw["operation"]
			if env.Method == "" && !gqlOp {
				if !strings.Contains(env.Note, "not sent") {
					t.Errorf("%s: no planned method and no uncaptured note: %+v", e.Command, env)
				}
				noPlan = append(noPlan, e.ID+" ("+e.Command+")")
			} else if env.Method != "" && env.URL == "" {
				t.Errorf("%s: method %s with no planned url", e.Command, env.Method)
			}
			if env.URL == "" {
				noURL = append(noURL, e.ID)
			}
			for name, want := range set {
				if env.Flags == nil {
					continue // envelopes without a flag map are audited above
				}
				if !envFlagValue(env, name, want) {
					t.Errorf("%s: set --%s=%q not echoed in plan flags", e.Command, name, want)
				}
			}
			for _, bad := range []string{"intuit_apikey=pr", "Authorization", "Bearer ", "access_token"} {
				if strings.Contains(stdout, bad) {
					t.Fatalf("%s: secret-like content in stdout: %s", e.Command, bad)
				}
			}
		})
	}
	t.Cleanup(func() {
		if t.Failed() {
			return
		}
		if len(noPlan) > 0 {
			t.Logf("commands with no planned method (uncaptured API): %s", strings.Join(noPlan, ", "))
		}
		if len(noURL) > 0 {
			t.Logf("commands with no planned url: %s", strings.Join(noURL, ", "))
		}
	})
}
