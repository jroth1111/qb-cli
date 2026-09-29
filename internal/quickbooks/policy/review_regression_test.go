package policy

import (
	"fmt"
	"strings"
	"testing"
)

func TestReviewOwnerNetRejectsUnprovedJournalPairs(t *testing.T) {
	for _, tc := range []struct {
		name, creditClass string
		creditAmount      float64
	}{
		{"unbalanced", "14 Agatha", 90},
		{"different class", "10 Toorak Melody", 100},
		{"unclassed credit", "", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := fmt.Sprintf(`{"Id":"1","Line":[
				{"Amount":100,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"name":%q},"ClassRef":{"name":"14 Agatha"}}},
				{"Amount":%g,"JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"name":"Management Income"},"ClassRef":{"name":%q}}}
			]}`, AcctMgmtFeeDebit, tc.creditAmount, tc.creditClass)
			in, notes, err := DeriveOwnerNet(testCompany, "14 Agatha", strings.NewReader(entry))
			if err != nil {
				t.Fatal(err)
			}
			if in.ManagementFee != nil || len(notes) == 0 {
				t.Fatalf("unproved pair became a property deduction: %+v, %v", in, notes)
			}
		})
	}
}

func TestReviewJournalCannotIgnoreUnknownLineType(t *testing.T) {
	inputs, err := ParseCheckInputs([]byte(feeEntry(647.90, "3700000000000906240", "10 Toorak Melody")))
	if err != nil {
		t.Fatal(err)
	}
	in := inputs[0]
	extra := in.Lines[0]
	extra.PostingType = "unknown"
	in.Lines = append(in.Lines, extra)
	res := CheckJournal(in, CheckOpts{Revenue: f64(3100)})
	if res.OK || res.Verdict != "fail" {
		t.Fatalf("ignored invalid line: %+v", res)
	}
}
