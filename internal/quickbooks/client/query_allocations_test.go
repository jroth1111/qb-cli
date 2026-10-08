package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSingleFinancialQueryPreservesRealAllocationLines(t *testing.T) {
	for _, entity := range []string{"Payment", "Invoice"} {
		t.Run(entity, func(t *testing.T) {
			saveUsable(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Fatal("read performed a write")
				}
				fmt.Fprintf(w, `{"QueryResponse":{"%s":[{"Id":"42","TotalAmt":123,"Line":[{"Amount":123,"LinkedTxn":[{"TxnId":"81","TxnType":"Invoice"}]}]}]}}`, entity)
			}))
			defer server.Close()
			interceptHTTP(t, server.URL)
			result, err := ReplayQuery(context.Background(), entity, "42", "", 0)
			if err != nil || result.Detail == nil || result.Detail["Line"] == nil {
				t.Fatalf("allocation discarded: %+v %v", result, err)
			}
			lines := result.Detail["Line"].([]any)
			link := lines[0].(map[string]any)["LinkedTxn"].([]any)[0].(map[string]any)
			if link["TxnId"] != "81" {
				t.Fatal("real relationship changed")
			}
		})
	}
}

func TestSinglePaymentQueryRejectsWrongIdentity(t *testing.T) {
	saveUsable(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"QueryResponse":{"Payment":[{"Id":"99","Line":[]}]}}`)
	}))
	defer server.Close()
	interceptHTTP(t, server.URL)
	if _, err := ReplayQuery(context.Background(), "Payment", "42", "", 0); err == nil {
		t.Fatal("wrong payment identity accepted")
	}
}
