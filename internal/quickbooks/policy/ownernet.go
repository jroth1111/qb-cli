// Owner-statement equation from landlord-statement-journal-patterns.md:
//
//	owner net = Client Property Revenue − management fee − cleaning
//	            − internet − direct or recharged property costs
//
// Owner distribution is reported separately and never folded into the
// equation.
package policy

import (
	"fmt"
	"io"
	"strings"
)

// OwnerNetInput carries the declared components of the owner-statement
// equation. Nil fields mark evidence the caller did not supply.
type OwnerNetInput struct {
	Revenue       float64
	ManagementFee *float64 // declared fee; absent → computed at 20.9%
	Cleaning      *float64 // prior month's approved amount; absent → unresolved
	Internet      *float64
	PropertyCosts []float64 // direct + recharged property costs, summed
	Distribution  *float64  // reported separately, never a deduction
	Class         string
}

// OwnerNetDeduction is one itemised deduction line.
type OwnerNetDeduction struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
	Source string  `json:"source"` // declared | computed | unresolved
	Detail string  `json:"detail,omitempty"`
}

// OwnerNetResult is the computed statement plus evidence markers.
type OwnerNetResult struct {
	Revenue      float64             `json:"revenue"`
	Deductions   []OwnerNetDeduction `json:"deductions"`
	OwnerNet     float64             `json:"owner_net"`
	Distribution *float64            `json:"distribution,omitempty"`
	Class        string              `json:"class,omitempty"`
	ClassWarning string              `json:"class_warning,omitempty"`
	Unresolved   []string            `json:"unresolved,omitempty"`
	JournalNotes []string            `json:"journal_notes,omitempty"`
	PnLNotes     []string            `json:"pnl_notes,omitempty"`
	Note         string              `json:"note"`
}

// OwnerNet computes the owner-statement equation. The management fee is
// recomputed at 20.9% unless declared; a declared fee is still compared
// against the computed value and the difference reported — per the rule that
// the fee is exactly 20.9% of Client Property Revenue, independently
// recompute before trusting.
func OwnerNet(in OwnerNetInput) (*OwnerNetResult, error) {
	if in.Revenue < 0 {
		return nil, fmt.Errorf("revenue must be non-negative, got %v", in.Revenue)
	}
	res := &OwnerNetResult{Revenue: in.Revenue, OwnerNet: in.Revenue, Class: in.Class, Distribution: in.Distribution}

	// Management fee.
	computed := round2(in.Revenue * 0.209)
	if in.ManagementFee == nil {
		res.Deductions = append(res.Deductions, OwnerNetDeduction{
			Label:  "management fee",
			Amount: computed,
			Source: "computed",
			Detail: "20.9% of revenue (MSA.LANDLORD_MANAGEMENT_FEE)",
		})
		res.OwnerNet -= computed
	} else {
		d := OwnerNetDeduction{Label: "management fee", Amount: *in.ManagementFee, Source: "declared"}
		if diff := round2(*in.ManagementFee - computed); diff != 0 {
			d.Detail = fmt.Sprintf("declared fee differs from revenue x 20.9%% by %+.2f — verify revenue base and rounding", diff)
		}
		res.Deductions = append(res.Deductions, d)
		res.OwnerNet -= *in.ManagementFee
	}

	// Cleaning: amount source is the prior month's approved fee.
	if in.Cleaning == nil {
		res.Deductions = append(res.Deductions, OwnerNetDeduction{
			Label:  "cleaning",
			Amount: 0,
			Source: "unresolved",
			Detail: "requires the prior month's approved cleaning fee (MSA.CLEANING_FEE_CARRY_FORWARD); unit 10 typically has none",
		})
		res.Unresolved = append(res.Unresolved, "cleaning: pass --cleaning with the prior month's approved amount, or accept 0 only with evidence")
	} else {
		res.Deductions = append(res.Deductions, OwnerNetDeduction{Label: "cleaning", Amount: *in.Cleaning, Source: "declared"})
		res.OwnerNet -= *in.Cleaning
	}

	// Internet.
	if in.Internet != nil {
		res.Deductions = append(res.Deductions, OwnerNetDeduction{Label: "internet", Amount: *in.Internet, Source: "declared"})
		res.OwnerNet -= *in.Internet
	}

	// Direct/recharged property costs.
	var costs float64
	for _, c := range in.PropertyCosts {
		costs += c
	}
	if costs != 0 {
		res.Deductions = append(res.Deductions, OwnerNetDeduction{Label: "direct/recharged property costs", Amount: round2(costs), Source: "declared", Detail: fmt.Sprintf("%d item(s)", len(in.PropertyCosts))})
		res.OwnerNet -= round2(costs)
	}

	res.OwnerNet = round2(res.OwnerNet)
	if in.Class != "" && !landlordClasses[in.Class] {
		res.ClassWarning = fmt.Sprintf("class %q is not one of the landlord-management properties these rules are evidenced for", in.Class)
	}
	res.Note = "Owner distribution is reported separately via Client Property Expenses:Owner Distribution and is never part of this equation; reconcile statement net to distributions independently."
	return res, nil
}

// DeriveOwnerNet builds the deduction side of the equation from posted
// journal entries for one property class — the self-evidencing path behind
// `owner-net --journals`. The reader may carry a JSON array, one object, or
// concatenated `journal get --id --json` envelopes; dedup by Id.
//
// Revenue is not a journal quantity — the caller still supplies it. Lines
// for other classes are skipped; unclassed pairs and unrecognised kinds are
// reported in notes, never silently included.
func DeriveOwnerNet(company, class string, r io.Reader) (OwnerNetInput, []string, error) {
	entries, err := decodeJournalStream(r)
	if err != nil {
		return OwnerNetInput{}, nil, err
	}
	want := strings.ToLower(strings.TrimSpace(class))
	// A --class flag may be the class name or the QBO id — resolve ids to
	// names so both spellings match.
	if n, ok := landlordClassIDs[class]; ok {
		want = strings.ToLower(n)
	}
	in := OwnerNetInput{Class: class}
	var notes []string
	fee, cleaning, dist := 0.0, 0.0, 0.0
	feeSeen, cleanSeen, distSeen := false, false, false

	for _, e := range entries {
		lines := entryLines(e)
		var pairs [][]CheckLine
		if len(lines) == 2 {
			pairs = [][]CheckLine{lines}
		} else {
			pairs = decomposePairs(company, lines)
		}
		if len(pairs) == 0 {
			notes = append(notes, fmt.Sprintf("journal %s skipped: lines do not decompose into balanced pairs", e.Id))
			continue
		}
		for _, p := range pairs {
			pc, _ := classIdentity(p[0])
			pcMatch := strings.ToLower(pc) == want || strings.ToLower(p[0].ClassID) == want
			if pc == "" || !pcMatch {
				notes = append(notes, fmt.Sprintf("journal %s pair skipped: class %q != %q", e.Id, pc, class))
				continue
			}
			switch inferKind(company, p) {
			case "management-fee":
				fee += p[0].Amount
				feeSeen = true
			case "cleaning":
				cleaning += p[0].Amount
				cleanSeen = true
			case "owner-distribution":
				dist += p[0].Amount
				distSeen = true
			case "cost-recharge":
				in.PropertyCosts = append(in.PropertyCosts, p[0].Amount)
			default:
				notes = append(notes, fmt.Sprintf("journal %s pair skipped: kind not recognised (%s / %s)", e.Id, accountLabel(p[0]), accountLabel(p[1])))
			}
		}
	}
	if feeSeen {
		in.ManagementFee = &fee
	}
	if cleanSeen {
		in.Cleaning = &cleaning
	}
	if distSeen {
		in.Distribution = &dist
	}
	return in, notes, nil
}
