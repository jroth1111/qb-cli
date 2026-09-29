package client

import (
	"encoding/json"
	"testing"
)

// plDetailRows reproduces the live ProfitAndLossDetail shape: one top-level
// "Ordinary Income/Expenses" section whose Summary is the Net row, nested
// Income/Expenses group sections, leaf txn rows with trailing Balance cells.
func plDetailRows(t *testing.T, income, expense, net string, txns [][]string) json.RawMessage {
	cell := func(v string) map[string]any { return map[string]any{"value": v} }
	cells := func(vals ...string) []any {
		out := make([]any, len(vals))
		for i, v := range vals {
			out[i] = cell(v)
		}
		return out
	}
	var incomeRows []any
	for _, tx := range txns {
		incomeRows = append(incomeRows, map[string]any{"ColData": cells(tx...)})
	}
	tree := map[string]any{
		"Header":  map[string]any{"ColData": cells("Ordinary Income/Expenses")},
		"Summary": map[string]any{"ColData": cells("Net Income", "", "", "", "", "", "", net, "")},
		"Rows": map[string]any{"Row": []any{
			map[string]any{
				"Header":  map[string]any{"ColData": cells("Income")},
				"Summary": map[string]any{"ColData": cells("Total for Income", "", "", "", "", "", "", income, "")},
				"Rows":    map[string]any{"Row": incomeRows},
			},
			map[string]any{
				"Header":  map[string]any{"ColData": cells("Expenses")},
				"Summary": map[string]any{"ColData": cells("Total for Expenses", "", "", "", "", "", "", expense, "")},
			},
		}},
	}
	raw, err := json.Marshal(map[string]any{"Row": []any{tree}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestV3ReportTotalsDetailShape(t *testing.T) {
	rows := plDetailRows(t, "7448.06", "1556.64", "5891.42", [][]string{
		{"2026-05-25", "Deposit", "", "Airbnb", "10 Toorak Melody", "memo", "Bank", "7446.54", "7448.06"},
	})
	tot, err := v3ReportTotals(rows)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Income != 7448.06 || tot.Expenses != 1556.64 || tot.Net != 5891.42 {
		t.Fatalf("got %+v", tot)
	}
	if tot.Total != 5891.42 {
		t.Fatalf("Total = %v, want net", tot.Total)
	}
}

func TestV3ReportTotalsEmpty(t *testing.T) {
	for _, raw := range []string{`{}`, `{"Row":[]}`, `{"Row":null}`} {
		tot, err := v3ReportTotals(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if tot.Net != 0 || tot.Income != 0 {
			t.Fatalf("%s: got %+v", raw, tot)
		}
	}
}

func TestV3ReportTotalsNoNetFallback(t *testing.T) {
	// account-list-style page: sections, no net row → Total = Σ summaries
	raw := json.RawMessage(`{"Row":[
		{"Summary":{"ColData":[{"value":"Total for Owner Distribution"},{"value":"100.00"}]},"Rows":{"Row":[]}},
		{"Summary":{"ColData":[{"value":"Total for Other"},{"value":"50.25"}]},"Rows":{"Row":[]}}
	]}`)
	tot, err := v3ReportTotals(raw)
	if err != nil {
		t.Fatal(err)
	}
	if tot.Total != 150.25 {
		t.Fatalf("Total = %v, want 150.25", tot.Total)
	}
}

func TestV3TxnRowsAmountVsBalance(t *testing.T) {
	cols := []string{"Date", "Transaction Type", "No.", "Name", "Class", "Memo/Description", "Split", "Amount", "Balance"}
	rows := plDetailRows(t, "7448.06", "0", "7448.06", [][]string{
		{"2026-05-14", "Deposit", "", "Airbnb", "10 Toorak Melody", "m", "Bank", "1.52", "1.52"},
		{"2026-05-25", "Deposit", "", "Airbnb", "10 Toorak Melody", "m", "Bank", "7446.54", "7448.06"},
	})
	txns, err := v3TxnRows(cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != 2 || txns[0].amount != 1.52 || txns[1].amount != 7446.54 {
		t.Fatalf("got %+v", txns)
	}
	// no column metadata: same result via the tail heuristic
	txns, err = v3TxnRows(nil, rows)
	if err != nil {
		t.Fatal(err)
	}
	if txns[1].amount != 7446.54 {
		t.Fatalf("heuristic picked %v, want 7446.54", txns[1].amount)
	}
}

func TestV3TxnRowsDegenerate(t *testing.T) {
	cases := map[string]json.RawMessage{
		"empty":      json.RawMessage(`{}`),
		"no-date":    json.RawMessage(`{"Row":[{"ColData":[{"value":"Header"},{"value":"5.00"}]}]}`),
		"bad-amount": json.RawMessage(`{"Row":[{"ColData":[{"value":"2026-01-01"},{"value":"abc"}]}]}`),
	}
	for name, raw := range cases {
		if _, err := v3TxnRows(nil, raw); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestMonthRangeEdges(t *testing.T) {
	p, err := monthRange("2026-01", "2026-03")
	if err != nil || len(p) != 3 {
		t.Fatalf("got %v %v", p, err)
	}
	if p[0].Start != "2026-01-01" || p[0].End != "2026-01-31" || p[1].End != "2026-02-28" || p[2].End != "2026-03-31" {
		t.Fatalf("bounds: %+v", p)
	}
	if _, err := monthRange("2026-03", "2026-01"); err == nil {
		t.Fatal("reversed range accepted")
	}
	for _, bad := range []string{"", "2026", "01-2026", "2026-13", "2026-00", "abc"} {
		if _, err := monthRange(bad, "2026-12"); err == nil {
			t.Fatalf("from %q accepted", bad)
		}
	}
	if _, err := monthRange("2026-02", "2026-02"); err != nil {
		t.Fatal("single month rejected")
	}
	p, _ = monthRange("2024-02", "2024-02") // leap year boundary
	if p[0].End != "2024-02-29" {
		t.Fatalf("leap Feb end = %s", p[0].End)
	}
}

func TestFolioAuditClassifyAndCoverage(t *testing.T) {
	cls := func(p *folioAuditPage, c ...string) *folioAuditPage { p.Classes = c; return p }
	mk := func(kind string, classes ...string) *folioAuditPage {
		return &folioAuditPage{Title: "t-" + kind, Kind: kind, Classes: classes}
	}
	// classify() derives kind from macro + class count
	for _, tc := range []struct {
		macro   string
		classes []string
		want    string
	}{
		{"all", []string{"a"}, "alltime"},
		{"all", nil, "alltime"},                // macro wins even with 0 classes
		{"All", []string{"a", "b"}, "alltime"}, // case-insensitive
		{"custom", []string{"a", "b"}, "combined"},
		{"custom", []string{"a"}, "unit"},
		{"custom", nil, "company"},
		{"", []string{"a"}, "unit"},
	} {
		p := cls(&folioAuditPage{Macro: tc.macro}, tc.classes...)
		p.classify()
		if p.Kind != tc.want {
			t.Fatalf("macro=%q classes=%v → %s, want %s", tc.macro, tc.classes, p.Kind, tc.want)
		}
	}
	// coverage: unit-only account + combined-only account both flagged
	a := &FolioAudit{}
	u := mk("unit", "A")
	u.Accounts = []string{"1", "2"}
	cb := mk("combined", "A", "B")
	cb.Accounts = []string{"1", "2", "230"}
	a.Pages = []folioAuditPage{*u, *cb}
	a.checkCoverage()
	var sawA230, sawB, sawUA bool
	for _, w := range a.Warnings {
		if w == `account 230 is in combined page "t-combined" but not in unit page "t-unit" — a posting on that class there breaks the tie-out` {
			sawA230 = true
		}
		if w == `combined page "t-combined" covers class B with no unit page — combined total includes activity the detail pages never show` {
			sawB = true
		}
	}
	_ = sawUA
	if !sawA230 || !sawB {
		t.Fatalf("warnings: %v", a.Warnings)
	}
}

func TestReconcileDeltaBothDirections(t *testing.T) {
	a := &FolioAudit{Pages: []folioAuditPage{
		{Title: "M", Kind: "unit", Classes: []string{"A"}},
		{Title: "G", Kind: "unit", Classes: []string{"B"}},
		{Title: "C", Kind: "combined"},
	}}
	rec := func(label, title string, kind string, net float64) {
		a.record(label, &folioAuditPage{Title: title, Kind: kind}, &auditTotals{Net: net})
	}
	rec("2026-05", "M", "unit", 5891.42)
	rec("2026-05", "G", "unit", 1754.02)
	rec("2026-05", "C", "combined", 7645.44)
	rec("2026-06", "M", "unit", 5472.39)
	rec("2026-06", "C", "combined", 7114.26) // G missing → mismatch
	a.reconcile()
	if !a.Periods[0].OK || a.Periods[0].Delta != 0 {
		t.Fatalf("May should tie out: %+v", a.Periods[0])
	}
	if a.Periods[1].OK || a.Periods[1].Delta != 1641.87 {
		t.Fatalf("June should flag missing unit: %+v", a.Periods[1])
	}
	// position binds distributions to unit pages by class
	a.Distributions = []folioAuditDist{
		{Page: "dist", Class: "A", InRange: 5000},
		{Page: "dist", Class: "B", InRange: 100},
	}
	a.Position = nil
	a.reconcile()
	if a.Position["M"] != 6363.81 {
		t.Fatalf("position M = %v, want 6363.81", a.Position["M"]) // 11363.81 - 5000
	}
	if a.Position["G"] != 1654.02 {
		t.Fatalf("position G = %v, want 1654.02", a.Position["G"]) // 1754.02 - 100
	}
}

// Single-property folios carry detail + summary on ONE class — the two
// single-class pages cross-check each other, not sum, and there is no
// combined page to tie against.
func TestReconcileSinglePropertyFolio(t *testing.T) {
	a := &FolioAudit{Pages: []folioAuditPage{
		{Title: "Adeline Detailed Statement", Kind: "unit", Report: "ProfitAndLossDetail", Classes: []string{"A"}},
		{Title: "Adeline Summary Statement", Kind: "unit", Report: "ProfitAndLoss", Classes: []string{"A"}},
		{Title: "Adeline Owner Distributions", Kind: "alltime", Classes: []string{"A"}},
	}}
	rec := func(title string, net float64) {
		a.record("2024-12", &folioAuditPage{Title: title, Kind: "unit"}, &auditTotals{Net: net})
	}
	rec("Adeline Detailed Statement", 2402.43)
	rec("Adeline Summary Statement", 2402.43)
	a.reconcile()
	pr := a.Periods[0]
	if !pr.OK || pr.Delta != 0 {
		t.Fatalf("agreeing leaves should reconcile: %+v", pr)
	}
	if pr.UnitSum != 2402.43 {
		t.Fatalf("UnitSum counted class twice: %v", pr.UnitSum)
	}
	// divergent leaves → not ok + warning once
	a2 := &FolioAudit{Pages: []folioAuditPage{
		{Title: "det", Kind: "unit", Report: "ProfitAndLossDetail", Classes: []string{"A"}},
		{Title: "sum", Kind: "unit", Report: "ProfitAndLoss", Classes: []string{"A"}},
	}}
	rec2 := func(title string, net float64) {
		a2.record("2024-12", &folioAuditPage{Title: title, Kind: "unit"}, &auditTotals{Net: net})
	}
	rec2("det", 2402.43)
	rec2("sum", 2000.00)
	a2.reconcile()
	if a2.Periods[0].OK {
		t.Fatal("divergent leaves must not reconcile")
	}
	if len(a2.Warnings) != 1 {
		t.Fatalf("warnings: %v", a2.Warnings)
	}
	// position: one row per class (representative = detail), not per title
	a.Distributions = []folioAuditDist{{Page: "dist", Class: "A", InRange: 5592.31}}
	a.Position = nil
	a.reconcile()
	if len(a.Position) != 1 || a.Position["Adeline Detailed Statement"] != -3189.88 {
		t.Fatalf("position: %v", a.Position)
	}
}

func TestParseTxnDateFormats(t *testing.T) {
	for in, want := range map[string]string{
		"2026-05-14":       "2026-05-14",
		"14/05/2026":       "2026-05-14",
		" 2026-05-14 ":     "2026-05-14",
		"bogus":            "",
		"":                 "",
		"Total for Income": "",
	} {
		if got := parseTxnDate(in); got != want {
			t.Fatalf("parseTxnDate(%q) = %q, want %q", in, got, want)
		}
	}
}
