package policy

import (
	"strconv"
	"testing"
)

func f64(v float64) *float64 { return &v }
func f(v float64) string     { return strconv.FormatFloat(v, 'f', -1, 64) }

func findingsByCheck(r CheckResult, check string) []Finding {
	var out []Finding
	for _, fnd := range r.Findings {
		if fnd.Check == check {
			out = append(out, fnd)
		}
	}
	return out
}

func hasSeverity(r CheckResult, sev string) bool {
	for _, fnd := range r.Findings {
		if fnd.Severity == sev {
			return true
		}
	}
	return false
}

// feeEntry builds a v3 management-fee journal: Dr Management Fee /
// Cr Management Income with the property class on both lines.
func feeEntry(amount float64, classID, className string) string {
	line := func(posting, acctID, acctName string) string {
		return `{"Amount":` + f(amount) + `,"DetailType":"JournalEntryLineDetail","Description":"Management Fee (20.9%)",` +
			`"JournalEntryLineDetail":{"PostingType":"` + posting + `",` +
			`"AccountRef":{"value":"` + acctID + `","name":"` + acctName + `"},` +
			`"ClassRef":{"value":"` + classID + `","name":"` + className + `"},"Description":"Management Fee (20.9%)"}}`
	}
	return `{"TxnDate":"2026-05-31","Line":[` +
		line("Debit", "89", AcctMgmtFeeDebit) + `,` +
		line("Credit", "1", AcctMgmtIncome) + `]}`
}

// TestCheckManagementFeePass asserts a fully-evidenced fee journal passes:
// balanced pair, correct accounts, class parity, revenue recompute.
func TestCheckManagementFeePass(t *testing.T) {
	inputs, err := ParseCheckInputs([]byte(feeEntry(647.90, "3700000000000906240", "10 Toorak Melody")))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := CheckJournal(inputs[0], CheckOpts{Revenue: f64(3100)})
	if !res.OK || res.Verdict != "pass" {
		t.Fatalf("verdict = %s, want pass; findings = %+v", res.Verdict, res.Findings)
	}
	if res.Kind != "management-fee" || res.DetectedKind != "management-fee" {
		t.Errorf("kind = %q/%q", res.Kind, res.DetectedKind)
	}
	if !res.Balanced || res.TotalDebit != 647.90 {
		t.Errorf("balance = %+v", res)
	}
}

// TestCheckManagementFeeUnverifiedWithoutRevenue keeps the falsifier: no
// revenue base → the fee cannot be verified → unverified, not pass.
func TestCheckManagementFeeUnverifiedWithoutRevenue(t *testing.T) {
	inputs, _ := ParseCheckInputs([]byte(feeEntry(647.90, "3700000000000906240", "10 Toorak Melody")))
	res := CheckJournal(inputs[0], CheckOpts{})
	if res.Verdict != "unverified" {
		t.Fatalf("verdict = %s, want unverified; findings = %+v", res.Verdict, res.Findings)
	}
	if !hasSeverity(res, "unresolved") {
		t.Error("expected an unresolved finding for missing revenue")
	}
}

// TestCheckManagementFeeMismatch asserts a wrong fee amount fails.
func TestCheckManagementFeeMismatch(t *testing.T) {
	inputs, _ := ParseCheckInputs([]byte(feeEntry(500, "3700000000000906240", "10 Toorak Melody")))
	res := CheckJournal(inputs[0], CheckOpts{Revenue: f64(3100)})
	if res.Verdict != "fail" || res.OK {
		t.Fatalf("verdict = %s, want fail; findings = %+v", res.Verdict, res.Findings)
	}
}

// TestCheckClassParity asserts different classes across the pair fail, and a
// missing class fails (the rule requires it, not merely prefers it).
func TestCheckClassParity(t *testing.T) {
	mixed := `{"Line":[
		{"Amount":647.9,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"` + AcctMgmtFeeDebit + `"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}},
		{"Amount":647.9,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"` + AcctMgmtIncome + `"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}}]}`
	inputs, _ := ParseCheckInputs([]byte(mixed))
	res := CheckJournal(inputs[0], CheckOpts{Revenue: f64(3100)})
	if res.Verdict != "fail" {
		t.Fatalf("mixed classes: verdict = %s, want fail", res.Verdict)
	}

	bare := `{"Line":[
		{"Amount":647.9,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"` + AcctMgmtFeeDebit + `"}}},
		{"Amount":647.9,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"` + AcctMgmtIncome + `"}}}]}` //nolint
	inputs, _ = ParseCheckInputs([]byte(bare))
	res = CheckJournal(inputs[0], CheckOpts{Revenue: f64(3100)})
	if res.Verdict != "fail" {
		t.Fatalf("missing class: verdict = %s, want fail; findings = %+v", res.Verdict, res.Findings)
	}
}

// TestCheckMixedEntryDecomposesToPairs covers the real MSA convention: one
// journal entry carrying two balanced pairs (fee + cleaning, same class).
// Each pair is checked against its own template; without evidence the
// verdict is unverified, never fail — and never a silent pass.
func TestCheckMixedEntryDecomposesToPairs(t *testing.T) {
	payload := `{"TxnDate":"2026-06-30","Line":[
	  {"Amount":503.84,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"Management Income"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":503.84,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"Client Property Expenses:Short-Term Business Operating Expenses:Management Fee"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":210,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"71","name":"Client Property Expenses:Short-Term Business Operating Expenses:Commercial Cleaning Services"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":210,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"151","name":"Cleaning Amenities Income"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}}
	]}`
	inputs, err := ParseCheckInputs([]byte(payload))
	if err != nil || len(inputs) != 1 {
		t.Fatalf("parse: %v %v", err, inputs)
	}
	res := CheckJournal(inputs[0], CheckOpts{Company: testCompany})
	if res.Verdict != "unverified" {
		t.Fatalf("mixed entry without evidence: verdict = %s, want unverified; findings=%+v", res.Verdict, res.Findings)
	}
	if len(res.Pairs) != 2 {
		t.Fatalf("pairs = %v", res.Pairs)
	}
	got := map[string]bool{res.Pairs[0]: true, res.Pairs[1]: true}
	if !got["management-fee"] || !got["cleaning"] {
		t.Fatalf("pair kinds = %v", res.Pairs)
	}
	// With evidence supplied the same entry must reach pass.
	rev, exp := 2410.72, 210.0
	res = CheckJournal(inputs[0], CheckOpts{Company: testCompany, Revenue: &rev, ExpectedAmount: &exp})
	if res.Verdict != "pass" {
		t.Fatalf("mixed entry with evidence: verdict = %s, want pass; findings=%+v", res.Verdict, res.Findings)
	}
}

// TestCheckMixedEntryUndecomposable keeps the error for mixed lines that do
// not form clean Dr/Cr pairs.
func TestCheckMixedEntryUndecomposable(t *testing.T) {
	payload := `{"Line":[
	  {"Amount":100,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"Client Property Expenses:Short-Term Business Operating Expenses:Management Fee"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":60,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"71","name":"Client Property Expenses:Short-Term Business Operating Expenses:Commercial Cleaning Services"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":80,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"Management Income"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":80,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"151","name":"Cleaning Amenities Income"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}}
	]}`
	inputs, err := ParseCheckInputs([]byte(payload))
	if err != nil || len(inputs) != 1 {
		t.Fatalf("parse: %v", err)
	}
	res := CheckJournal(inputs[0], CheckOpts{Company: testCompany})
	if res.Verdict != "fail" {
		t.Fatalf("non-pairable mixed entry: verdict = %s, want fail", res.Verdict)
	}
}

// TestCheckUnbalanced fails a non-balancing pair.
func TestCheckUnbalanced(t *testing.T) {
	entry := `{"Line":[
		{"Amount":100,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"` + AcctMgmtFeeDebit + `"}}},
		{"Amount":90,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"` + AcctMgmtIncome + `"}}}]}`
	inputs, _ := ParseCheckInputs([]byte(entry))
	res := CheckJournal(inputs[0], CheckOpts{Revenue: f64(478.47)})
	if res.Verdict != "fail" || res.Balanced {
		t.Fatalf("verdict = %s balanced=%v, want fail", res.Verdict, res.Balanced)
	}
}

// TestCheckItemsFormat asserts the create-shape items parse and check, with
// account names supplied via the *-name metadata keys.
func TestCheckItemsFormat(t *testing.T) {
	items := `[{"date":"31/05/2026","amount":647.90,"from-account":"89","to-account":"1",
		"from-account-name":"` + AcctMgmtFeeDebit + `","to-account-name":"` + AcctMgmtIncome + `",
		"class":"3700000000000906240","revenue":3100}]`
	inputs, err := ParseCheckInputs([]byte(items))
	if err != nil {
		t.Fatalf("parse items: %v", err)
	}
	res := CheckJournal(inputs[0], CheckOpts{})
	if res.Kind != "management-fee" {
		t.Fatalf("kind = %q, want management-fee; findings = %+v", res.Kind, res.Findings)
	}
	if res.Verdict != "pass" {
		t.Fatalf("verdict = %s, want pass; findings = %+v", res.Verdict, res.Findings)
	}
}

// TestCheckCleaningCarryForward asserts the carry-forward rule: prior approved
// amount must be supplied; mismatch fails; conflict stays visible.
func TestCheckCleaningCarryForward(t *testing.T) {
	cleaning := `{"Line":[
		{"Amount":210,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"71","name":"` + AcctCleaningDebit + `"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
		{"Amount":210,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"151","name":"` + AcctCleaningIncome + `"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}}]}`
	inputs, _ := ParseCheckInputs([]byte(cleaning))

	res := CheckJournal(inputs[0], CheckOpts{})
	if res.Verdict != "unverified" {
		t.Fatalf("no expected amount: verdict = %s, want unverified", res.Verdict)
	}
	res = CheckJournal(inputs[0], CheckOpts{ExpectedAmount: f64(210)})
	if res.Verdict != "pass" {
		t.Fatalf("matching amount: verdict = %s, want pass; findings = %+v", res.Verdict, res.Findings)
	}
	res = CheckJournal(inputs[0], CheckOpts{ExpectedAmount: f64(440)})
	if res.Verdict != "fail" {
		t.Fatalf("conflicting amount: verdict = %s, want fail", res.Verdict)
	}
}

// TestCheckOwnerDistribution asserts the separation rule: crediting an income
// account is an error.
func TestCheckOwnerDistribution(t *testing.T) {
	dist := `{"Line":[
		{"Amount":5000,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"88","name":"` + AcctOwnerDist + `"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}},
		{"Amount":5000,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"` + AcctMgmtIncome + `"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}}]}`
	inputs, _ := ParseCheckInputs([]byte(dist))
	res := CheckJournal(inputs[0], CheckOpts{})
	if res.Verdict != "fail" {
		t.Fatalf("verdict = %s, want fail (income-credited distribution); findings = %+v", res.Verdict, res.Findings)
	}
}

// TestCheckCostRecharge asserts margin evidence is required: unresolved
// without supplier cost, margin reported with it.
func TestCheckCostRecharge(t *testing.T) {
	recharge := `{"Line":[
		{"Amount":300,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"230","name":"Client Property Expenses:Short-Term Business Operating Expenses:Parking"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
		{"Amount":300,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"` + AcctMgmtIncome + `"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}}]}`
	inputs, _ := ParseCheckInputs([]byte(recharge))
	res := CheckJournal(inputs[0], CheckOpts{})
	if res.Verdict != "unverified" {
		t.Fatalf("no supplier cost: verdict = %s, want unverified", res.Verdict)
	}
	res = CheckJournal(inputs[0], CheckOpts{SupplierCost: f64(220)})
	if res.Verdict != "pass" {
		t.Fatalf("supplier cost given: verdict = %s, want pass; findings = %+v", res.Verdict, res.Findings)
	}
}

// TestCheckUnknownKind asserts uninferrable journals are unverified, not
// silently passed.
func TestCheckUnknownKind(t *testing.T) {
	other := `{"Line":[
		{"Amount":50,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"999","name":"Depreciation"}}},
		{"Amount":50,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"998","name":"Accumulated Depreciation"}}}]}`
	inputs, _ := ParseCheckInputs([]byte(other))
	res := CheckJournal(inputs[0], CheckOpts{})
	if res.Verdict != "unverified" {
		t.Fatalf("verdict = %s, want unverified for unknown accounts", res.Verdict)
	}
}

// TestParseCheckInputsNegatives covers malformed payloads and envelope unwrap.
func TestParseCheckInputsNegatives(t *testing.T) {
	for _, raw := range []string{"", "not json", "[]", `{"foo":1}`, `[{"amount":0,"from-account":"1","to-account":"2"}]`, `[{"amount":5,"from-account":"","to-account":"2"}]`} {
		if _, err := ParseCheckInputs([]byte(raw)); err == nil {
			t.Errorf("expected parse error for %q", raw)
		}
	}
	// QueryResponse envelope unwrap.
	wrapped := `{"QueryResponse":{"JournalEntry":[` + feeEntry(647.90, "3700000000000906240", "10 Toorak Melody") + `]}}`
	inputs, err := ParseCheckInputs([]byte(wrapped))
	if err != nil || len(inputs) != 1 {
		t.Fatalf("QueryResponse unwrap: %v", err)
	}
}
