package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchDetailsDescriptionEncodingDoesNotPermitSemanticDrift(t *testing.T) {
	for _, description := range []string{"Transfer to A&#x2F;c", "Transfer to another account"} {
		t.Run(description, func(t *testing.T) {
			saveUsable(t)
			row := map[string]any{"id": "1:ofx", "olbTxnId": "1", "qboAccountId": "204", "amount": -10.0, "olbTxnDate": "2020-01-01T10:00:00+10:00", "origDescription": "Transfer to A/c"}
			live := shallowCopyMap(row)
			live["origDescription"] = description
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatal("description lookup wrote")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"olbTxn": live, "homeCurrencyMatchingTxns": []any{map[string]any{"qboTxnId": "10", "txnTypeId": "54", "qboTxnSeqId": "0", "txnSyncToken": "1", "amount": -10.0}}})
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			_, err = buildLiveMatchBody(context.Background(), ac, "204", []map[string]any{row}, []MatchTxn{{TxnID: "10"}})
			if (err == nil) != (description == "Transfer to A&#x2F;c") {
				t.Fatalf("description drift check err=%v", err)
			}
		})
	}
}
