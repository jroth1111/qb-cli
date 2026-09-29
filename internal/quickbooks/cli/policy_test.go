package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestPolicyDocsList asserts `qb policy docs` enumerates the embedded MSA
// reference documents.
func TestPolicyDocsList(t *testing.T) {
	out, _, err := runQB(t, "policy", "docs")
	if err != nil {
		t.Fatalf("policy docs: %v", err)
	}
	for _, id := range []string{"landlord-statement-journal-patterns", "chart-of-accounts", "policies"} {
		if !strings.Contains(out, id) {
			t.Errorf("docs output missing %q\n%s", id, out)
		}
	}
}

// TestPolicyDocsJSON asserts the JSON shape agents will parse.
func TestPolicyDocsJSON(t *testing.T) {
	out, _, err := runQB(t, "policy", "docs", "--json")
	if err != nil {
		t.Fatalf("policy docs --json: %v", err)
	}
	var docs []struct {
		ID    string `json:"id"`
		Bytes int    `json:"bytes"`
	}
	if err := json.Unmarshal([]byte(out), &docs); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	found := false
	for _, d := range docs {
		if d.ID == "policies" && d.Bytes > 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("policies doc absent or empty in JSON output")
	}
}

// TestPolicyDocPrint asserts `qb policy doc` streams document content.
func TestPolicyDocPrint(t *testing.T) {
	out, _, err := runQB(t, "policy", "doc", "landlord-statement-journal-patterns")
	if err != nil {
		t.Fatalf("policy doc: %v", err)
	}
	if !strings.Contains(out, "20.9%") {
		t.Errorf("doc output missing fee rule\n%s", out)
	}
}

// TestPolicyRules asserts the parsed rule registry is reachable from the CLI.
func TestPolicyRules(t *testing.T) {
	out, _, err := runQB(t, "policy", "rules", "--json")
	if err != nil {
		t.Fatalf("policy rules: %v", err)
	}
	var rules []struct {
		RuleID string   `json:"rule_id"`
		Rate   *float64 `json:"rate"`
	}
	if err := json.Unmarshal([]byte(out), &rules); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var feeRate *float64
	for _, r := range rules {
		if r.RuleID == "MSA.LANDLORD_MANAGEMENT_FEE" {
			feeRate = r.Rate
		}
	}
	if feeRate == nil || *feeRate != 0.209 {
		t.Errorf("management fee rate = %v, want 0.209", feeRate)
	}
}

// TestPolicyManagementFee asserts the calculator and --amount check path.
func TestPolicyManagementFee(t *testing.T) {
	out, _, err := runQB(t, "policy", "management-fee", "--revenue", "1000.00", "--class", "10 Toorak Melody", "--json")
	if err != nil {
		t.Fatalf("management-fee: %v", err)
	}
	var calc struct {
		Fee           float64 `json:"fee"`
		Rate          float64 `json:"rate"`
		DebitAccount  string  `json:"debit_account"`
		CreditAccount string  `json:"credit_account"`
	}
	if err := json.Unmarshal([]byte(out), &calc); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if calc.Fee != 209.00 || calc.Rate != 0.209 {
		t.Errorf("calc = %+v, want fee 209.00 rate 0.209", calc)
	}
	if calc.DebitAccount == "" || calc.CreditAccount != "Management Income" {
		t.Errorf("accounts wrong: %+v", calc)
	}

	// --amount match path: 3100 * 0.209 = 647.90 exactly.
	out, _, err = runQB(t, "policy", "management-fee", "--revenue", "3100", "--amount", "647.90", "--json")
	if err != nil {
		t.Fatalf("management-fee --amount: %v", err)
	}
	var check struct {
		Match      bool    `json:"match"`
		Difference float64 `json:"difference"`
	}
	if err := json.Unmarshal([]byte(out), &check); err != nil {
		t.Fatalf("decode check: %v\n%s", err, out)
	}
	if !check.Match || check.Difference != 0 {
		t.Errorf("check = %+v, want match true difference 0", check)
	}

	// Mismatch must be visible in output.
	out, _, err = runQB(t, "policy", "management-fee", "--revenue", "3100", "--amount", "999", "--json")
	if err != nil {
		t.Fatalf("management-fee mismatch: %v", err)
	}
	if !strings.Contains(out, `"match": false`) {
		t.Errorf("mismatch not reported\n%s", out)
	}
}

// TestPolicyNegativeCases keeps the suite's falsifiers: unknown doc, unknown
// rule, unknown template, missing required flag, unknown company.
func TestPolicyNegativeCases(t *testing.T) {
	for _, args := range [][]string{
		{"policy", "doc", "no-such-doc"},
		{"policy", "rule", "MSA.NOPE"},
		{"policy", "journal-template", "no-such-kind"},
		{"policy", "management-fee"}, // missing required --revenue
		{"policy", "docs", "--company", "no-such-company"},
	} {
		if _, _, err := runQB(t, args...); err == nil {
			t.Errorf("expected error for args %v", args)
		}
	}
}

// TestPolicyLeavesAreReadOnly asserts every policy leaf is annotated so the
// mutation gate classifies it read, never unclassified-wired.
func TestPolicyLeavesAreReadOnly(t *testing.T) {
	root := NewRootCommand()
	policyCmd := findSub(root, "policy")
	if policyCmd == nil {
		t.Fatal("root missing policy subcommand")
	}
	for _, leaf := range policyCmd.Commands() {
		if !leaf.Runnable() {
			continue
		}
		if leaf.Annotations["qb:read-only"] != "true" {
			t.Errorf("policy leaf %q missing qb:read-only annotation", leaf.Name())
		}
	}
}

// runQBInput executes the root command with an injected stdin reader.
func runQBInput(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

const feeItems = `[{"amount":647.90,"from-account":"89","to-account":"1","from-account-name":"Client Property Expenses:Short-Term Business Operating Expenses:Management Fee","to-account-name":"Management Income","class-name":"10 Toorak Melody","revenue":3100}]`

// TestCheckJournalItems asserts the create-shape path end to end: pass when
// revenue recomputes, non-zero when the amount cannot be verified.
func TestCheckJournalItems(t *testing.T) {
	out, _, err := runQB(t, "policy", "check-journal", "--items-json", feeItems, "--json")
	if err != nil {
		t.Fatalf("check-journal pass case: %v\n%s", err, out)
	}
	var res struct {
		Verdict string `json:"verdict"`
		OK      bool   `json:"ok"`
		Kind    string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if res.Verdict != "pass" || !res.OK || res.Kind != "management-fee" {
		t.Errorf("result = %+v, want pass/management-fee", res)
	}

	// Same payload without the revenue evidence must not exit zero.
	ungated := `[{"amount":647.90,"from-account":"89","to-account":"1","from-account-name":"Client Property Expenses:Short-Term Business Operating Expenses:Management Fee","to-account-name":"Management Income","class-name":"10 Toorak Melody"}]`
	if _, _, err := runQB(t, "policy", "check-journal", "--items-json", ungated, "--json"); err == nil {
		t.Error("unverified check must exit non-zero")
	}
}

// TestCheckJournalV3Stdin asserts a v3 JournalEntry on stdin is checked.
func TestCheckJournalV3Stdin(t *testing.T) {
	entry := `{"TxnDate":"2026-05-31","Line":[
		{"Amount":647.9,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"Client Property Expenses:Short-Term Business Operating Expenses:Management Fee"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}},
		{"Amount":647.9,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"Management Income"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}}]}`
	out, err := runQBInput(t, entry, "policy", "check-journal", "--revenue", "3100", "--json")
	if err != nil {
		t.Fatalf("v3 stdin check: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"verdict": "pass"`) {
		t.Errorf("expected pass verdict\n%s", out)
	}
}

// TestCheckJournalFail asserts a rule violation exits non-zero.
func TestCheckJournalFail(t *testing.T) {
	bad := `[{"amount":500,"from-account":"89","to-account":"1","from-account-name":"Client Property Expenses:Short-Term Business Operating Expenses:Management Fee","to-account-name":"Management Income","class-name":"10 Toorak Melody","revenue":3100}]`
	if _, _, err := runQB(t, "policy", "check-journal", "--items-json", bad); err == nil {
		t.Error("failing check must exit non-zero")
	}
}

// TestPolicyAccountsQuery asserts chart-of-accounts lookup works from the
// CLI: name substring and exact id both resolve.
func TestPolicyAccountsQuery(t *testing.T) {
	out, _, err := runQB(t, "policy", "accounts", "--query", "Management Income")
	if err != nil {
		t.Fatalf("accounts query: %v", err)
	}
	if !strings.Contains(out, "Management Income") {
		t.Errorf("accounts output missing account\n%s", out)
	}
	out, _, err = runQB(t, "policy", "accounts", "--query", "89", "--json")
	if err != nil {
		t.Fatalf("accounts id query: %v", err)
	}
	var matches []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &matches); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	found := false
	for _, m := range matches {
		if m.ID == "89" {
			found = true
		}
	}
	if !found {
		t.Errorf("account 89 not in results")
	}
}

// TestPolicyClassesQuery asserts class lookup resolves the landlord classes.
func TestPolicyClassesQuery(t *testing.T) {
	out, _, err := runQB(t, "policy", "classes", "--query", "Toorak")
	if err != nil {
		t.Fatalf("classes query: %v", err)
	}
	if !strings.Contains(out, "3700000000000906240") || !strings.Contains(out, "10 Toorak Melody") {
		t.Errorf("classes output missing Toorak class\n%s", out)
	}
}

// TestPolicyOwnerNet asserts the statement equation via CLI.
func TestPolicyOwnerNet(t *testing.T) {
	out, _, err := runQB(t, "policy", "owner-net", "--revenue", "3100", "--cleaning", "210", "--internet", "99", "--cost", "300", "--cost", "220", "--json")
	if err != nil {
		t.Fatalf("owner-net: %v", err)
	}
	var res struct {
		OwnerNet   float64  `json:"owner_net"`
		Unresolved []string `json:"unresolved"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if res.OwnerNet != 1623.10 {
		t.Errorf("owner net = %v, want 1623.10", res.OwnerNet)
	}
}

// TestPolicyCategorise asserts the signal-registry suggestion surface: a
// strong signature suggests, a known conflict refuses and exits nonzero.
func TestPolicyCategorise(t *testing.T) {
	out, _, err := runQB(t, "policy", "categorise", "TANGERINE telecom 2403", "--json")
	if err != nil {
		t.Fatalf("categorise: %v\n%s", err, out)
	}
	var res struct {
		Verdict           string `json:"verdict"`
		AccountCandidates []struct {
			Account string `json:"account"`
		} `json:"account_candidates"`
		ClassCandidates []struct {
			Class string `json:"class"`
		} `json:"class_candidates"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if res.Verdict != "suggested" || len(res.AccountCandidates) != 1 || res.AccountCandidates[0].Account != "MSA Property Expenses:Internet" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.ClassCandidates) == 0 || res.ClassCandidates[0].Class != "2403 Mainpoint" {
		t.Fatalf("class candidates = %+v", res.ClassCandidates)
	}

	// Conflict path: TPG must never auto-suggest, and exit must be nonzero.
	out, _, err = runQB(t, "policy", "categorise", "TPG monthly", "--json")
	if err == nil {
		t.Fatalf("conflict should exit nonzero:\n%s", out)
	}
	if !strings.Contains(out, `"conflict"`) {
		t.Fatalf("conflict verdict missing:\n%s", out)
	}
}

func TestPolicyOwnerNetPnL(t *testing.T) {
	pnl := `{"header":{"ReportName":"ProfitAndLoss","StartPeriod":"2026-06-01","EndPeriod":"2026-06-30"},
"rows":{"Row":[
 {"Header":{"ColData":[{"value":"Income"}]},"Rows":{"Row":[
   {"ColData":[{"value":"Client Property Revenue"},{"value":"2410.70"}]},
   {"ColData":[{"value":"Management Income"},{"value":"503.84"}]},
   {"ColData":[{"value":"Cleaning Amenities Income"},{"value":"210.00"}]}
 ]}},
 {"Header":{"ColData":[{"value":"Expenses"}]},"Rows":{"Row":[
   {"ColData":[{"value":"Marketing"},{"value":"40.00"}]}
 ]}}
]}}`

	// --pnl supplies the revenue side: --revenue is not required alongside it.
	out, err := runQBInput(t, pnl, "policy", "owner-net", "--class", "14 Agatha", "--pnl", "-", "--json")
	if err != nil {
		t.Fatalf("owner-net --pnl: %v\n%s", err, out)
	}
	var res struct {
		Revenue   float64 `json:"revenue"`
		OwnerNet  float64 `json:"owner_net"`
		Deduction []struct {
			Label  string  `json:"label"`
			Amount float64 `json:"amount"`
		} `json:"deductions"`
		PnLNotes []string `json:"pnl_notes"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if res.Revenue != 2410.70 {
		t.Fatalf("revenue = %v", res.Revenue)
	}
	// 2410.70 - 503.84 - 210.00 = 1696.86
	if res.OwnerNet != 1696.86 {
		t.Fatalf("owner_net = %v", res.OwnerNet)
	}
	joined := strings.Join(res.PnLNotes, "\n")
	if !strings.Contains(joined, "derived from P&L") {
		t.Fatalf("derivation notes missing: %v", res.PnLNotes)
	}

	// Neither --revenue nor --pnl is a usage error now.
	if _, _, err := runQB(t, "policy", "owner-net", "--class", "14 Agatha"); err == nil {
		t.Fatal("expected error when neither --revenue nor --pnl is supplied")
	}
}
