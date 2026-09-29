package client

import "testing"

// depositWithLine assembles the v3 Deposit shape the scanner reads: a line
// posting to acct through DepositLineDetail with the given description.
func depositWithLine(id, acct, desc string, amt float64) map[string]any {
	return map[string]any{
		"Id": id, "TxnDate": "2026-05-14",
		"Line": []any{map[string]any{
			"Id": "1", "Amount": amt, "Description": desc,
			"DepositLineDetail": map[string]any{
				"AccountRef": map[string]any{"value": acct},
				"ClassRef":   map[string]any{"name": "10 Toorak Melody"},
			},
		}},
	}
}

// The scanner must catch MSA-semantics lines inside the owner revenue
// account — including US "canceled" spellings and negative amounts — while
// leaving ordinary stay revenue and other accounts untouched.
func TestScanAttributionFindings(t *testing.T) {
	entities := []map[string]any{
		depositWithLine("100", "47", "Cancelled booking payout — resolution 12345", 620.00),
		depositWithLine("101", "47", "Airbnb claim for guest damage", 150.00),
		depositWithLine("102", "47", "Guest canceled reservation, partial payout", 300.00),
		depositWithLine("103", "47", "Refund of accommodation", -80.00),
		depositWithLine("104", "47", "Accommodation — Airbnb stay", 1500.00),
		depositWithLine("105", "46", "Cancelled fee but on a different account", 50.00),
		depositWithLine("106", "47", "Natalie Lock Accommodation 5th-20th Dec (including cleaning fee)", 3614.00),
	}
	out := ScanAttributionFindings("Deposit", entities, "47")
	if len(out) != 5 {
		t.Fatalf("findings=%d, want 5: %+v", len(out), out)
	}
	byID := map[string]AttributionFinding{}
	for _, f := range out {
		byID[f.EntityID] = f
	}
	if byID["100"].Severity != "msa" || byID["101"].Severity != "msa" || byID["102"].Severity != "msa" {
		t.Fatalf("cancellation/claim semantics not flagged msa: %+v", out)
	}
	if byID["103"].Severity != "review" || byID["103"].Direction != "owner_underpaid" {
		t.Fatalf("refund line misclassified: %+v", byID["103"])
	}
	if byID["106"].Severity != "review" {
		t.Fatalf("mixed accommodation+cleaning-fee line must demote to review: %+v", byID["106"])
	}
	if byID["100"].Direction != "owner_overpaid" || byID["100"].Class != "10 Toorak Melody" {
		t.Fatalf("positive line direction/class wrong: %+v", byID["100"])
	}
}

// SalesItemLineDetail on invoices is the second detail shape; ItemAccountRef
// must resolve when AccountRef is absent.
func TestScanAttributionFindingsInvoiceShape(t *testing.T) {
	entities := []map[string]any{
		{"Id": "7", "TxnDate": "2026-05-01", "Line": []any{
			map[string]any{"Id": "1", "Amount": 90.0, "Description": "Break lease charge",
				"SalesItemLineDetail": map[string]any{
					"ItemAccountRef": map[string]any{"value": "47"},
				}},
		}},
	}
	out := ScanAttributionFindings("Invoice", entities, "47")
	if len(out) != 1 || out[0].Account != "47" || out[0].Severity != "msa" {
		t.Fatalf("invoice line not found: %+v", out)
	}
}
