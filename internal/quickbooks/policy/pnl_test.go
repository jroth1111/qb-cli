package policy

import (
	"strings"
	"testing"
)

const pnlFixture = `{
  "status": 200,
  "report": "ProfitAndLoss",
  "header": {"ReportName": "ProfitAndLoss", "StartPeriod": "2026-06-01", "EndPeriod": "2026-06-30", "ReportBasis": "Cash"},
  "columns": ["Total"],
  "rows": {"Row": [
    {"Header": {"ColData": [{"value": "Income"}]},
     "Rows": {"Row": [
       {"ColData": [{"value": "Cleaning Amenities Income", "id": "15"}, {"value": "210.00"}]},
       {"ColData": [{"value": "Client Property Revenue", "id": "1"}, {"value": "2410.70"}]},
       {"ColData": [{"value": "Management Income", "id": "88"}, {"value": "503.84"}]},
       {"ColData": [{"value": "MSA Property Revenue", "id": "4"}, {"value": "105016.87"}]}
     ]},
     "Summary": {"ColData": [{"value": "Total Income"}, {"value": "108141.41"}]}},
    {"Header": {"ColData": [{"value": "Expenses"}]},
     "Rows": {"Row": [
       {"Header": {"ColData": [{"value": "Client Property Expenses"}]},
        "Rows": {"Row": [
          {"Header": {"ColData": [{"value": "Short-Term Business Operating Expenses"}]},
           "Rows": {"Row": [
             {"ColData": [{"value": "Management Fee", "id": "89"}, {"value": "503.84"}]},
             {"ColData": [{"value": "Internet", "id": "99"}, {"value": "54.99"}]}
           ]}}
        ]}},
       {"ColData": [{"value": "Marketing", "id": "120"}, {"value": "40.00"}]}
     ]},
     "Summary": {"ColData": [{"value": "Total Expenses"}, {"value": "598.83"}]}}
  ]}
}`

func TestParsePnLReport(t *testing.T) {
	rep, err := ParsePnLReport([]byte(pnlFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rep.Report != "ProfitAndLoss" || rep.Start != "2026-06-01" || rep.End != "2026-06-30" {
		t.Fatalf("header fields not extracted: %+v", rep)
	}
	if got := rep.Sections["Income"]["Client Property Revenue"]; got != 2410.70 {
		t.Fatalf("revenue row = %v", got)
	}
	if got := rep.Sections["Income"]["Management Income"]; got != 503.84 {
		t.Fatalf("management income = %v", got)
	}
	// Nested leaf inside a sub-section of Expenses must surface.
	if got := rep.Sections["Expenses"]["Management Fee"]; got != 503.84 {
		t.Fatalf("nested Management Fee = %v", got)
	}
	if _, ok := rep.Sections["Income"]["Total Income"]; ok {
		t.Fatal("summary rows must not be parsed as leaf rows")
	}
}

func TestParsePnLReportRejectsBadInput(t *testing.T) {
	for _, bad := range []string{`not json`, `{"rows": {}}`, `{}`} {
		if _, err := ParsePnLReport([]byte(bad)); err == nil {
			t.Fatalf("expected parse error for %q", bad)
		}
	}
}

func TestMergePnLEvidenceFillsUnset(t *testing.T) {
	rep, err := ParsePnLReport([]byte(pnlFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	in, notes := MergePnLEvidence(OwnerNetInput{}, rep)
	if in.Revenue != 2410.70 {
		t.Fatalf("revenue = %v", in.Revenue)
	}
	if in.ManagementFee == nil || *in.ManagementFee != 503.84 {
		t.Fatalf("fee = %v", in.ManagementFee)
	}
	if in.Cleaning == nil || *in.Cleaning != 210.00 {
		t.Fatalf("cleaning = %v", in.Cleaning)
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "MSA Property Revenue") {
		t.Fatalf("unrecognised income row must be noted, got: %v", notes)
	}
	if strings.Contains(joined, "differs") {
		t.Fatalf("clean fixture should produce no diffs: %v", notes)
	}
}

func TestMergePnLEvidenceConfirmsAndFlagsDiffs(t *testing.T) {
	rep, _ := ParsePnLReport([]byte(pnlFixture))
	fee, cleaning := 503.84, 215.00 // cleaning mismatches the P&L
	in, notes := MergePnLEvidence(OwnerNetInput{
		Revenue:       2410.70,
		ManagementFee: &fee,
		Cleaning:      &cleaning,
	}, rep)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "confirmed by P&L Client Property Revenue") {
		t.Fatalf("matching revenue should confirm: %v", notes)
	}
	if !strings.Contains(joined, "management fee 503.84 confirmed") {
		t.Fatalf("matching fee should confirm: %v", notes)
	}
	if !strings.Contains(joined, "cleaning: evidence 215.00 differs") {
		t.Fatalf("mismatched cleaning must surface the diff: %v", notes)
	}
	if in.Cleaning == nil || *in.Cleaning != 215.00 {
		t.Fatal("existing evidence must not be overwritten by the P&L")
	}
}

func TestMergePnLEvidenceMissingRowsStayUnresolved(t *testing.T) {
	rep := &PnLReport{Sections: map[string]map[string]float64{
		"Income": {"Client Property Revenue": 1000},
	}}
	in, notes := MergePnLEvidence(OwnerNetInput{}, rep)
	if in.ManagementFee != nil || in.Cleaning != nil {
		t.Fatal("absent P&L rows must not fabricate evidence")
	}
	if in.Revenue != 1000 {
		t.Fatalf("revenue = %v", in.Revenue)
	}
	_ = notes
}
