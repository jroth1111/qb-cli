// P&L report intake for the owner-statement equation — the revenue side of
// `owner-net` that journals cannot carry. Feed it the JSON of a
// class-filtered report fetch:
//
//	qb reports profit-loss get --date-range 2026-06-01,2026-06-30 \
//	    --klass 3700000000000906241 --json | qb policy owner-net --pnl - ...
//
// For a landlord statement the owner-charged amounts sit on the income
// side: Client Property Revenue is the revenue base, Management Income is
// the fee actually charged, Cleaning Amenities Income the recharged
// cleaning. Expense-side rows are MSA's own cost accounting and are only
// used as cross-checks.
package policy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PnLReport is a normalised walk of a ProfitAndLoss report: leaf row name →
// summed amount, per top-level section ("Income", "Cost of Sales",
// "Expenses", "Other Income", "Other Expenses", ...).
type PnLReport struct {
	Report   string                        `json:"report"`
	Start    string                        `json:"start"`
	End      string                        `json:"end"`
	Basis    string                        `json:"basis"`
	Sections map[string]map[string]float64 `json:"sections"`
}

// Owner-charged income rows recognised for the landlord statement. Any
// other income row is reported as a note, never folded into the equation.
var pnlIncomeMap = map[string]string{
	"Client Property Revenue":   "revenue",
	"Management Income":         "management-fee",
	"Cleaning Amenities Income": "cleaning",
	"Parking Income":            "cost-recharge",
	"Internet Income":           "internet",
}

// ParsePnLReport decodes a `reports profit-loss get --json` payload —
// the wrapped {status, header, columns, rows} envelope or a bare v3 report
// object — into sectioned leaf totals.
func ParsePnLReport(data []byte) (*PnLReport, error) {
	var env map[string]any
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("P&L payload is not JSON: %w", err)
	}
	rep := &PnLReport{Sections: map[string]map[string]float64{}}
	if h, ok := env["header"].(map[string]any); ok {
		rep.Report, _ = h["ReportName"].(string)
		rep.Start, _ = h["StartPeriod"].(string)
		rep.End, _ = h["EndPeriod"].(string)
		rep.Basis, _ = h["ReportBasis"].(string)
	}
	rows := env["rows"]
	if rows == nil {
		rows = env["Rows"]
	}
	rmap, ok := rows.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("P&L payload has no rows object (got %T)", rows)
	}
	top, _ := rmap["Row"].([]any)
	if len(top) == 0 {
		return nil, fmt.Errorf("P&L rows.Row is empty or missing")
	}
	for _, t := range top {
		sec, _ := t.(map[string]any)
		if sec == nil {
			continue
		}
		name := sectionName(sec)
		if name == "" {
			continue
		}
		bag := rep.Sections[name]
		if bag == nil {
			bag = map[string]float64{}
			rep.Sections[name] = bag
		}
		walkPnLLeaves(sec, bag)
	}
	return rep, nil
}

// sectionName reads a top-level section's Header label.
func sectionName(row map[string]any) string {
	h, _ := row["Header"].(map[string]any)
	cd, _ := h["ColData"].([]any)
	if len(cd) == 0 {
		return ""
	}
	first, _ := cd[0].(map[string]any)
	v, _ := first["value"].(string)
	return strings.TrimSpace(v)
}

// walkPnLLeaves recurses a section, summing leaf (ColData-only) rows by
// name. Group headers and "Total …" summaries are not leaf rows.
func walkPnLLeaves(node map[string]any, bag map[string]float64) {
	if sub, ok := node["Rows"].(map[string]any); ok {
		if list, _ := sub["Row"].([]any); len(list) > 0 {
			for _, r := range list {
				if rm, ok := r.(map[string]any); ok {
					walkPnLLeaves(rm, bag)
				}
			}
			return
		}
	}
	cd, _ := node["ColData"].([]any)
	if len(cd) < 2 {
		return
	}
	name, _ := cd[0].(map[string]any)
	amt, _ := cd[len(cd)-1].(map[string]any)
	n, _ := name["value"].(string)
	v, _ := amt["value"].(string)
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || strings.TrimSpace(n) == "" {
		return
	}
	bag[strings.TrimSpace(n)] += f
}

// MergePnLEvidence folds a class-filtered P&L into an OwnerNetInput that
// may already carry flag-supplied or journal-derived values. P&L values
// fill empty fields; where both exist and disagree the difference is
// reported as a note — the income-side credit and the journal debit are the
// same charge on clean books, so a mismatch is a reconciliation signal.
func MergePnLEvidence(in OwnerNetInput, rep *PnLReport) (OwnerNetInput, []string) {
	var notes []string
	income := rep.Sections["Income"]
	if len(income) == 0 {
		return in, append(notes, "P&L has no Income section — nothing derived")
	}
	var unknown []string
	for name := range income {
		if _, ok := pnlIncomeMap[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		notes = append(notes, fmt.Sprintf("P&L income row %q (%.2f) is not a landlord-statement field — not folded in", name, income[name]))
	}

	// Revenue: flag-supplied wins; P&L fills an unset base.
	if rev, ok := income["Client Property Revenue"]; ok {
		switch {
		case in.Revenue == 0:
			in.Revenue = rev
			notes = append(notes, fmt.Sprintf("revenue %.2f derived from P&L Client Property Revenue", rev))
		case round2(in.Revenue-rev) != 0:
			notes = append(notes, fmt.Sprintf("--revenue %.2f differs from P&L Client Property Revenue %.2f — verify the report's --klass/--date-range", in.Revenue, rev))
		default:
			notes = append(notes, fmt.Sprintf("revenue %.2f confirmed by P&L Client Property Revenue", rev))
		}
	} else if in.Revenue == 0 {
		notes = append(notes, "P&L Income has no Client Property Revenue row — revenue still required via --revenue")
	}

	fill := func(field **float64, row, label string) {
		v, ok := income[row]
		if !ok {
			return
		}
		if *field == nil {
			fv := v
			*field = &fv
			notes = append(notes, fmt.Sprintf("%s %.2f derived from P&L %s", label, v, row))
		} else if d := round2(**field - v); d != 0 {
			notes = append(notes, fmt.Sprintf("%s: evidence %.2f differs from P&L %s %.2f by %+.2f — investigate before posting", label, **field, row, v, d))
		} else {
			notes = append(notes, fmt.Sprintf("%s %.2f confirmed by P&L %s", label, v, row))
		}
	}
	fill(&in.ManagementFee, "Management Income", "management fee")
	fill(&in.Cleaning, "Cleaning Amenities Income", "cleaning")
	fill(&in.Internet, "Internet Income", "internet")
	if v, ok := income["Parking Income"]; ok {
		in.PropertyCosts = append(in.PropertyCosts, v)
		notes = append(notes, fmt.Sprintf("parking recharge %.2f derived from P&L Parking Income", v))
	}

	// Expense-side sanity: MSA's own Management Fee expense should equal the
	// Management Income it charged owners when books are clean.
	exp := rep.Sections["Expenses"]
	if me, mok := exp["Management Fee"]; mok {
		if mi, iok := income["Management Income"]; iok {
			if d := round2(me - mi); d != 0 {
				notes = append(notes, fmt.Sprintf("P&L Management Fee expense %.2f differs from Management Income %.2f by %+.2f", me, mi, d))
			}
		}
	}
	return in, notes
}
