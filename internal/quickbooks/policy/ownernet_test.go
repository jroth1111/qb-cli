package policy

import (
	"strings"
	"testing"
)

// TestOwnerNetComputedFee asserts the default path: fee computed at 20.9%,
// cleaning unresolved without the prior-approved amount.
func TestOwnerNetComputedFee(t *testing.T) {
	res, err := OwnerNet(OwnerNetInput{Revenue: 3100, Class: "14 Agatha"})
	if err != nil {
		t.Fatalf("OwnerNet: %v", err)
	}
	if res.OwnerNet != 2452.10 {
		t.Errorf("owner net = %.2f, want 2452.10 (3100 - 647.90)", res.OwnerNet)
	}
	var fee *OwnerNetDeduction
	for i := range res.Deductions {
		if res.Deductions[i].Label == "management fee" {
			fee = &res.Deductions[i]
		}
	}
	if fee == nil || fee.Amount != 647.90 || fee.Source != "computed" {
		t.Errorf("fee deduction = %+v", fee)
	}
	if len(res.Unresolved) == 0 {
		t.Error("cleaning must be unresolved without a declared amount")
	}
}

// TestOwnerNetFull asserts the complete equation: 3100 − 647.90 − 210 − 99 −
// (300+220) = 1933.10, with distribution kept separate.
func TestOwnerNetFull(t *testing.T) {
	res, err := OwnerNet(OwnerNetInput{
		Revenue:       3100,
		ManagementFee: new(647.90),
		Cleaning:      f64(210),
		Internet:      f64(99),
		PropertyCosts: []float64{300, 220},
		Distribution:  f64(1500),
		Class:         "14 Agatha",
	})
	if err != nil {
		t.Fatalf("OwnerNet: %v", err)
	}
	if res.OwnerNet != 1623.10 {
		t.Errorf("owner net = %.2f, want 1623.10", res.OwnerNet)
	}
	if res.Distribution == nil || *res.Distribution != 1500 {
		t.Errorf("distribution = %v, want 1500", res.Distribution)
	}
	if len(res.Unresolved) != 0 {
		t.Errorf("unresolved = %v, want none", res.Unresolved)
	}
}

// TestOwnerNetDeclaredFeeMismatch asserts a declared fee that is not 20.9%
// surfaces the difference instead of silently passing.
func TestOwnerNetDeclaredFeeMismatch(t *testing.T) {
	res, err := OwnerNet(OwnerNetInput{Revenue: 3100, ManagementFee: f64(600)})
	if err != nil {
		t.Fatalf("OwnerNet: %v", err)
	}
	var fee *OwnerNetDeduction
	for i := range res.Deductions {
		if res.Deductions[i].Label == "management fee" {
			fee = &res.Deductions[i]
		}
	}
	if fee == nil || fee.Detail == "" {
		t.Fatalf("fee mismatch detail missing: %+v", fee)
	}
}

// TestOwnerNetNegative covers the negative cases.
func TestOwnerNetNegative(t *testing.T) {
	if _, err := OwnerNet(OwnerNetInput{Revenue: -1}); err == nil {
		t.Error("expected error for negative revenue")
	}
	res, err := OwnerNet(OwnerNetInput{Revenue: 100, Class: "Not A Property"})
	if err != nil {
		t.Fatalf("OwnerNet: %v", err)
	}
	if res.ClassWarning == "" {
		t.Error("expected class warning for non-landlord class")
	}
}

// TestDeriveOwnerNet covers the journal-derivation path: class scoping,
// pair decomposition, concatenated-envelope streams, and honest notes.
func TestDeriveOwnerNet(t *testing.T) {
	// Two concatenated `journal get --id --json` envelopes: an Agatha
	// fee+cleaning combined entry, then a Toorak fee pair.
	stream := `{"status":200,"detail":{"Id":"29633","TxnDate":"2026-06-30","Line":[
	  {"Amount":503.84,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"Management Income"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":503.84,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"` + AcctMgmtFeeDebit + `"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":210,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"71","name":"` + AcctCleaningDebit + `"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}},
	  {"Amount":210,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"151","name":"Cleaning Amenities Income"},"ClassRef":{"value":"3700000000000906241","name":"14 Agatha"}}}]}}
{"status":200,"detail":{"Id":"29630","TxnDate":"2026-06-30","Line":[
	  {"Amount":1445.93,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"Management Income"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}},
	  {"Amount":1445.93,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"` + AcctMgmtFeeDebit + `"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}}]}}`
	in, notes, err := DeriveOwnerNet(testCompany, "14 Agatha", strings.NewReader(stream))
	if err != nil {
		t.Fatalf("DeriveOwnerNet: %v", err)
	}
	if in.ManagementFee == nil || *in.ManagementFee != 503.84 {
		t.Fatalf("fee = %v", in.ManagementFee)
	}
	if in.Cleaning == nil || *in.Cleaning != 210 {
		t.Fatalf("cleaning = %v", in.Cleaning)
	}
	if len(notes) == 0 {
		t.Error("expected a skip note for the Toorak pair")
	}
	// Class-id spelling must scope identically.
	in2, _, err := DeriveOwnerNet(testCompany, "3700000000000906241", strings.NewReader(stream))
	if err != nil || in2.ManagementFee == nil || *in2.ManagementFee != 503.84 {
		t.Fatalf("class-id scoping: %v %+v", err, in2.ManagementFee)
	}
}

// TestDeriveOwnerNetDedup asserts a repeated journal id is counted once.
func TestDeriveOwnerNetDedup(t *testing.T) {
	entry := `{"Id":"28848","TxnDate":"2026-02-28","Line":[
	  {"Amount":1637.19,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"1","name":"Management Income"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}},
	  {"Amount":1637.19,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"89","name":"` + AcctMgmtFeeDebit + `"},"ClassRef":{"value":"3700000000000906240","name":"10 Toorak Melody"}}}]}`
	in, _, err := DeriveOwnerNet(testCompany, "10 Toorak Melody", strings.NewReader(entry+"\n"+entry))
	if err != nil {
		t.Fatalf("DeriveOwnerNet: %v", err)
	}
	if in.ManagementFee == nil || *in.ManagementFee != 1637.19 {
		t.Fatalf("deduped fee = %v, want 1637.19", in.ManagementFee)
	}
}
