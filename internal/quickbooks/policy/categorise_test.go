package policy

import "testing"

// TestCategoriseStrongSignal asserts a dominant posted signature suggests
// its account with the audit evidence attached.
func TestCategoriseStrongSignal(t *testing.T) {
	res, err := Categorise(testCompany, "TANGERINE telecom monthly")
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "suggested" || len(res.AccountCandidates) != 1 {
		t.Fatalf("verdict=%s candidates=%+v", res.Verdict, res.AccountCandidates)
	}
	if res.AccountCandidates[0].Account != "MSA Property Expenses:Internet" {
		t.Fatalf("account = %q", res.AccountCandidates[0].Account)
	}
	if res.AccountCandidates[0].Confidence != "strong" {
		t.Fatalf("confidence = %q", res.AccountCandidates[0].Confidence)
	}
}

// TestCategoriseUnitTokenClass checks a unit number resolves to the property
// class from the live-refreshed class inventory.
func TestCategoriseUnitTokenClass(t *testing.T) {
	res, err := Categorise(testCompany, "TANGERINE telecom 10 Toorak")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range res.ClassCandidates {
		if c.Class == "10 Toorak Melody" && c.ID == "3700000000000906240" {
			found = true
		}
	}
	if !found {
		t.Fatalf("class candidates = %+v", res.ClassCandidates)
	}
}

// TestCategoriseConflictNeverSuggests is the falsifier: channel parties and
// spanning suppliers must surface alternatives, never an account verdict.
func TestCategoriseConflictNeverSuggests(t *testing.T) {
	for _, text := range []string{"TPG monthly", "AIRBNB SERVICE payout", "Pushkar cleaning"} {
		res, err := Categorise(testCompany, text)
		if err != nil {
			t.Fatal(err)
		}
		if res.Verdict != "conflict" {
			t.Fatalf("%q: verdict=%s, want conflict", text, res.Verdict)
		}
		if len(res.Conflicts) == 0 {
			t.Fatalf("%q: no conflict surfaced", text)
		}
	}
}

// TestCategoriseWeakSignalRequiresDiscriminator checks a <98% signal stays
// unresolved and carries its discriminator.
func TestCategoriseWeakSignalRequiresDiscriminator(t *testing.T) {
	res, err := Categorise(testCompany, "Chunming Liu clean")
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "unresolved" {
		t.Fatalf("verdict = %s, want unresolved", res.Verdict)
	}
	if len(res.AccountCandidates) != 1 || res.AccountCandidates[0].Confidence != "weak" || res.AccountCandidates[0].Discriminator == "" {
		t.Fatalf("candidates = %+v", res.AccountCandidates)
	}
}

// TestCategoriseSalaryPersonClass checks the salary signature suggests both
// the account and the person class resolved from the inventory.
func TestCategoriseSalaryPersonClass(t *testing.T) {
	res, err := Categorise(testCompany, "Maggie Salary June")
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "suggested" || res.AccountCandidates[0].Account != "Director Compensation" {
		t.Fatalf("verdict=%s %+v", res.Verdict, res.AccountCandidates)
	}
	found := false
	for _, c := range res.ClassCandidates {
		if c.Class == "Maggie" && c.ID != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("class candidates = %+v, want Maggie resolved with id", res.ClassCandidates)
	}
}

// TestCategoriseNoMatch covers the empty-evidence path.
func TestCategoriseNoMatch(t *testing.T) {
	res, err := Categorise(testCompany, "xyzzy unknown vendor")
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "unresolved" || len(res.AccountCandidates) != 0 {
		t.Fatalf("verdict=%s candidates=%+v", res.Verdict, res.AccountCandidates)
	}
}

// TestCategoriseKeynestInternational verifies the more specific intl-fee
// signature outranks the base-charge signal.
func TestCategoriseKeynestInternational(t *testing.T) {
	res, err := Categorise(testCompany, "INTERNATIONAL FEE KEYNEST PTY LTD")
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "suggested" || res.AccountCandidates[0].Account != "Bank charges" {
		t.Fatalf("verdict=%s candidates=%+v", res.Verdict, res.AccountCandidates)
	}
}
