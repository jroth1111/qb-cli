package policy

import (
	"strings"
	"testing"
)

const testCompany = "mega-style-apartments"

// TestEmbeddedCompanyProfile asserts the recovered MSA profile is present
// with all nine reference documents.
func TestEmbeddedCompanyProfile(t *testing.T) {
	companies, err := Companies()
	if err != nil {
		t.Fatalf("Companies: %v", err)
	}
	if len(companies) != 1 || companies[0] != testCompany {
		t.Fatalf("unexpected companies: %v", companies)
	}
	docs, err := Docs(testCompany)
	if err != nil {
		t.Fatalf("Docs: %v", err)
	}
	want := map[string]bool{
		"README":                              false,
		"categorisation-decision-model":       false,
		"categorisation-patterns":             false,
		"categorisation-signals":              false,
		"chart-of-accounts":                   false,
		"expense-classification-policy":       false,
		"landlord-statement-journal-patterns": false,
		"policies":                            false,
		"property-operating-models":           false,
		"two-year-categorisation-audit":       false,
	}
	for _, d := range docs {
		if _, ok := want[d.ID]; !ok {
			t.Errorf("unexpected doc %q", d.ID)
			continue
		}
		want[d.ID] = true
		if d.Bytes == 0 {
			t.Errorf("doc %q is empty", d.ID)
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("missing doc %q", id)
		}
	}
}

// TestDocContent asserts the statement-patterns doc carries the rules the
// CLI surfaces: 20.9% fee, paired journals, owner distribution separation.
func TestDocContent(t *testing.T) {
	content, err := Doc(testCompany, "landlord-statement-journal-patterns")
	if err != nil {
		t.Fatalf("Doc: %v", err)
	}
	for _, needle := range []string{"20.9%", "Owner Distribution", "Management Income", "Cleaning Amenities Income"} {
		if !strings.Contains(content, needle) {
			t.Errorf("doc missing %q", needle)
		}
	}
}

func TestDocUnknown(t *testing.T) {
	if _, err := Doc(testCompany, "no-such-doc"); err == nil {
		t.Fatal("expected error for unknown doc")
	}
	if _, err := Docs("no-such-company"); err == nil {
		t.Fatal("expected error for unknown company")
	}
}

// TestRulesParse asserts policies.yaml parses into the four registered rules
// with the fee rate and temporal window intact.
func TestRulesParse(t *testing.T) {
	rules, err := Rules(testCompany)
	if err != nil {
		t.Fatalf("Rules: %v", err)
	}
	byID := map[string]Rule{}
	for _, r := range rules {
		byID[r.RuleID] = r
	}
	for _, id := range []string{
		"MSA.LANDLORD_MANAGEMENT_FEE",
		"MSA.CLEANING_FEE_CARRY_FORWARD",
		"MSA.COST_MARGIN_EVIDENCE",
		"MSA.AUDIT_WINDOW",
	} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing rule %q", id)
		}
	}
	fee := byID["MSA.LANDLORD_MANAGEMENT_FEE"]
	if fee.Rate == nil || *fee.Rate != 0.209 {
		t.Errorf("fee rate = %v, want 0.209", fee.Rate)
	}
	if len(fee.Property) != 2 {
		t.Errorf("fee property scope = %v", fee.Property)
	}
	window := byID["MSA.AUDIT_WINDOW"]
	if window.EffectiveFrom == nil || *window.EffectiveFrom != "2024-07-13" {
		t.Errorf("audit window start = %v", window.EffectiveFrom)
	}
	if _, err := RuleByID(testCompany, "MSA.NOPE"); err == nil {
		t.Fatal("expected error for unknown rule")
	}
}

// TestManagementFee covers the deterministic fee calculation, including a
// cent-rounding boundary and the negative cases.
func TestManagementFee(t *testing.T) {
	calc, err := ManagementFee(testCompany, 1000.00, "10 Toorak Melody")
	if err != nil {
		t.Fatalf("ManagementFee: %v", err)
	}
	if calc.Fee != 209.00 {
		t.Errorf("fee = %v, want 209.00", calc.Fee)
	}
	if calc.ClassWarning != "" {
		t.Errorf("unexpected warning: %s", calc.ClassWarning)
	}
	if calc.DebitAccount != "Client Property Expenses:Short-Term Business Operating Expenses:Management Fee" {
		t.Errorf("debit = %s", calc.DebitAccount)
	}
	if calc.CreditAccount != "Management Income" {
		t.Errorf("credit = %s", calc.CreditAccount)
	}

	// 11.96 * 0.209 = 2.49964 → 2.50 (rounds up past the cent).
	round, err := ManagementFee(testCompany, 11.96, "")
	if err != nil {
		t.Fatalf("ManagementFee rounding case: %v", err)
	}
	if round.Fee != 2.50 {
		t.Errorf("rounded fee = %v, want 2.50", round.Fee)
	}

	// Non-landlord class warns but still computes — no company gate.
	warn, err := ManagementFee(testCompany, 1000, "Arbitrage Unit")
	if err != nil {
		t.Fatalf("ManagementFee warn case: %v", err)
	}
	if warn.ClassWarning == "" {
		t.Error("expected class warning for non-landlord class")
	}
	if warn.Fee != 209.00 {
		t.Errorf("warn-case fee = %v, want 209.00", warn.Fee)
	}

	if _, err := ManagementFee(testCompany, -5, ""); err == nil {
		t.Fatal("expected error for negative revenue")
	}
	if _, err := ManagementFee("no-such-company", 1000, ""); err == nil {
		t.Fatal("expected error for unknown company")
	}
}

// TestJournalTemplates asserts the four paired-journal patterns exist with
// the documented account mappings and class requirements.
func TestJournalTemplates(t *testing.T) {
	tpls := JournalTemplates()
	byKind := map[string]JournalTemplate{}
	for _, tpl := range tpls {
		byKind[tpl.Kind] = tpl
	}
	mgmt, ok := byKind["management-fee"]
	if !ok {
		t.Fatal("missing management-fee template")
	}
	if !mgmt.ClassRequired {
		t.Error("management-fee template must require a property class")
	}
	clean, ok := byKind["cleaning"]
	if !ok || clean.CreditAccount != "Cleaning Amenities Income" {
		t.Errorf("cleaning template wrong: %+v", clean)
	}
	if _, err := JournalTemplateFor("no-such-kind"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}
