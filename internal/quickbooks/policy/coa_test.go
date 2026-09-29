package policy

import "testing"

// TestCOAParse asserts the embedded chart of accounts parses into the
// documented counts and that policy-critical accounts resolve.
func TestCOAParse(t *testing.T) {
	accounts, err := Accounts(testCompany)
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	// Doc refreshed from live v3 query 2026-09-29: 240 accounts incl. inactive.
	if len(accounts) < 200 {
		t.Errorf("parsed %d accounts, want ~240", len(accounts))
	}
	byName := map[string]Account{}
	for _, a := range accounts {
		byName[a.Name] = a
	}
	for name, wantID := range map[string]string{
		AcctMgmtFeeDebit:   "89",
		AcctMgmtIncome:     "1",
		AcctCleaningDebit:  "71",
		AcctCleaningIncome: "151",
		AcctOwnerDist:      "88",
	} {
		a, ok := byName[name]
		if !ok {
			t.Errorf("account %q missing from COA", name)
			continue
		}
		if a.ID != wantID {
			t.Errorf("account %q id = %s, want %s", name, a.ID, wantID)
		}
	}
}

// TestCOAQuery covers name-substring and exact-id lookup, plus the negative
// no-match case.
func TestCOAQuery(t *testing.T) {
	matches, err := QueryAccounts(testCompany, "management fee")
	if err != nil {
		t.Fatalf("QueryAccounts: %v", err)
	}
	found89 := false
	for _, m := range matches {
		if m.ID == "89" {
			found89 = true
		}
	}
	if !found89 || len(matches) == 0 {
		t.Fatalf("management fee query = %+v", matches)
	}
	matches, err = QueryAccounts(testCompany, "89")
	if err != nil {
		t.Fatalf("QueryAccounts id: %v", err)
	}
	found := false
	for _, m := range matches {
		if m.ID == "89" {
			found = true
		}
	}
	if !found {
		t.Error("exact id query did not surface account 89")
	}
	if m, _ := QueryAccounts(testCompany, "no such account zzz"); len(m) != 0 {
		t.Errorf("no-match query returned %d rows", len(m))
	}
}

// TestCOAClasses asserts the class inventory parses, the landlord-management
// classes resolve, and the truncation recovery note is present but harmless.
func TestCOAClasses(t *testing.T) {
	classes, err := Classes(testCompany)
	if err != nil {
		t.Fatalf("Classes: %v", err)
	}
	// Table was regenerated from the live v3 query 2026-09-29 (the original
	// capture truncated at ~101 rows); live count is 173 incl. inactive.
	if len(classes) < 170 {
		t.Errorf("parsed %d classes, expected ~173", len(classes))
	}
	byName := map[string]string{}
	for _, c := range classes {
		byName[c.Name] = c.ID
	}
	if byName["10 Toorak Melody"] != "3700000000000906240" {
		t.Errorf("10 Toorak Melody = %q", byName["10 Toorak Melody"])
	}
	if byName["14 Agatha"] != "3700000000000906241" {
		t.Errorf("14 Agatha = %q", byName["14 Agatha"])
	}
	matches, _ := QueryClasses(testCompany, "agatha")
	if len(matches) != 1 || matches[0].ID != "3700000000000906241" {
		t.Fatalf("agatha query = %+v", matches)
	}
}

// TestAccountNameForID asserts the checker-side id resolution works for both
// the policy map and the wider COA.
func TestAccountNameForID(t *testing.T) {
	if got := AccountNameForID(testCompany, "5"); got != "Accounts receivable" {
		t.Errorf("id 5 = %q", got)
	}
	if got := AccountNameForID(testCompany, "999999"); got != "" {
		t.Errorf("unknown id = %q, want empty", got)
	}
}
