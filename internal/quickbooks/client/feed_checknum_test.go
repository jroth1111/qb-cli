package client

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFeedProjectsAndSearchesChequeNumber(t *testing.T) {
	for _, label := range []string{"JacobCard", "MaggieCard", "Unverified", ""} {
		t.Run(label, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{"id": "example:ofx", "description": "Merchant", "checkNum": label})
			if err != nil {
				t.Fatal(err)
			}
			var raw rawTxn
			if err := json.Unmarshal(payload, &raw); err != nil {
				t.Fatal(err)
			}
			got := projectTxn(raw)
			if got.CheckNum != label {
				t.Fatalf("checkNum = %q, want %q", got.CheckNum, label)
			}
			if label != "" && !txnMatches(got, strings.ToLower(label[:3])) {
				t.Fatal("lookup dropped cheque-number label")
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if label != "" && fields["checkNum"] != label {
				t.Fatalf("JSON lost label: %s", encoded)
			}
			if txnMatches(got, "unknown-card") {
				t.Fatal("lookup matched an absent label")
			}
		})
	}
}
