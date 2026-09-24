package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchRegisterClearingTransitions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		old, new int
		want     bool
	}{{"unchanged", 1, 1, true}, {"UI blank to cleared", 2, 1, true}, {"other state cannot be cleared", 0, 1, false}, {"cleared cannot be reset", 1, 2, false}} {
		t.Run(tc.name, func(t *testing.T) {
			a := map[string]any{"amount": 42.0, "clearState": float64(tc.old)}
			b := map[string]any{"amount": 42.0, "clearState": float64(tc.new)}
			if got := matchRegisterPreserved(a, b); got != tc.want {
				t.Fatalf("preserved=%v want=%v", got, tc.want)
			}
		})
	}
	before := map[string]any{"amount": 42.0, "clearState": 2.0, "accountId": 93.0}
	for _, key := range []string{"amount", "accountId", "date", "memo", "klassId"} {
		after := shallowCopyMap(before)
		after[key] = "changed"
		if matchRegisterPreserved(before, after) {
			t.Fatalf("accepted unrelated change to %s", key)
		}
	}
}

func TestMatchResponseRequiresDomainSuccess(t *testing.T) {
	rows := []map[string]any{{"id": "1:ofx"}}
	targets := []MatchTxn{{TxnID: "2"}}
	for _, body := range []string{`{"ok":true}`, `{"success":[]}`, `{"error":"rejected"}`, `{"success":[{"id":"1:ofx","qboAccount":{"accountId":"99"},"matchedQboTxns":[{"qboTxnId":"2"}]}]}`, `{"success":[{"id":"1:ofx","qboAccount":{"accountId":"204"},"matchedQboTxns":[{"qboTxnId":"3"}]}]}`} {
		if err := verifyMatchResponse([]byte(body), "204", rows, targets); !errors.Is(err, ErrMatchVerification) {
			t.Fatalf("accepted nonmatching success: %s (%v)", body, err)
		}
	}
	good := `{"success":[{"id":"1:ofx","qboAccount":{"accountId":"204"},"matchedQboTxns":[{"qboTxnId":"2"}]}]}`
	if err := verifyMatchResponse([]byte(good), "204", rows, targets); err != nil {
		t.Fatal(err)
	}
}

func TestMatchReadbackRejectsWrongPostedState(t *testing.T) {
	for _, field := range []string{"missing", "amount", "origDescription", "target", "account"} {
		t.Run(field, func(t *testing.T) {
			saveUsable(t)
			before := feedRowFixture("2")
			row := shallowCopyMap(before)
			row["matchedQboTxns"] = []any{map[string]any{"qboTxnId": "4"}}
			switch field {
			case "amount":
				row["amount"] = 999.0
			case "origDescription":
				row["origDescription"] = "other"
			case "target":
				row["matchedQboTxns"] = []any{map[string]any{"qboTxnId": "9"}}
			case "account":
				row["qboAccountId"] = "93"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("verification performed a write")
				}
				items := []map[string]any{row}
				if field == "missing" {
					items = nil
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			if err = verifyMatchReadbackOnce(context.Background(), ac, "204", []map[string]any{before}, nil, []MatchTxn{{TxnID: "4"}}); err == nil {
				t.Fatalf("accepted %s readback", field)
			}
		})
	}
}
