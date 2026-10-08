package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPinnedMatchDateWindow(t *testing.T) {
	for _, tc := range []struct{ name, ledger, low, high string }{
		{"default", "", "2019-10-03", "2020-01-31"},
		{"inside", "2020-01-15", "2019-10-03", "2020-01-31"},
		{"earlier", "2019-01-01", "2019-01-01", "2020-01-31"},
		{"later", "2020-04-01", "2019-10-03", "2020-04-01"},
		{"invalid", "not-a-date", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("QB_HTTP_TRANSPORT", "")
			saveUsable(t)
			row := map[string]any{"id": "1:ofx", "qboAccountId": "204", "amount": -10.0, "olbTxnDate": "2020-01-01T10:00:00+10:00", "origDescription": "fixture"}
			reads := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				if r.Method != http.MethodGet || r.URL.Query().Get("lowDate") != tc.low || r.URL.Query().Get("highDate") != tc.high {
					t.Errorf("unexpected eligibility lookup: %s %s", r.Method, r.URL)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"olbTxn": row, "homeCurrencyMatchingTxns": []any{map[string]any{"qboTxnId": "10", "txnTypeId": "54", "qboTxnSeqId": "0", "txnSyncToken": "1", "amount": -10.0}}})
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			_, err = buildLiveMatchBodyWithLedgerDate(context.Background(), ac, "204", []map[string]any{row}, []MatchTxn{{TxnID: "10"}}, tc.ledger)
			if tc.name == "invalid" {
				if err == nil || reads != 0 {
					t.Fatalf("invalid date dialed: reads=%d err=%v", reads, err)
				}
			} else if err != nil || reads != 1 {
				t.Fatalf("reads=%d err=%v", reads, err)
			}
		})
	}
}
