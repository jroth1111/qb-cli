package client

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChequeCreateUsesNativeCheckPaymentType(t *testing.T) {
	saveUsable(t)
	var sent map[string]any
	var persisted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && persisted != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"Purchase": persisted})
			return
		}
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/purchase") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
		}
		persisted = map[string]any{}
		maps.Copy(persisted, sent)
		persisted["Id"], persisted["SyncToken"] = "8", "0"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Purchase":{"Id":"8","PaymentType":"Check","TotalAmt":1}}`))
	}))
	defer server.Close()
	interceptHTTP(t, server.URL)
	flags := map[string]string{"account": "67", "category-account": "7", "amount": "1"}
	result, err := ReplayMutate(context.Background(), "Cheque", "create", "", flags)
	if err != nil {
		t.Fatal(err)
	}
	if sent["PaymentType"] != "Check" || result.Item.Type != "Check" {
		t.Fatalf("sent=%v result=%+v", sent, result)
	}
	if flags["cheque"] != "" {
		t.Fatal("mutated caller flags")
	}
}

func TestChequeChangesRejectCashPurchasesBeforePosting(t *testing.T) {
	for _, op := range []string{"update", "delete"} {
		t.Run(op, func(t *testing.T) {
			saveUsable(t)
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"Purchase":{"Id":"8","SyncToken":"0","PaymentType":"Cash"}}`))
			}))
			defer server.Close()
			interceptHTTP(t, server.URL)
			_, err := ReplayMutate(context.Background(), "Cheque", op, "8", map[string]string{"amount": "2"})
			if err == nil || !strings.Contains(err.Error(), "not a cheque") || posts != 0 {
				t.Fatalf("err=%v posts=%d", err, posts)
			}
		})
	}
}

func TestChequeQueryAlwaysFiltersNativeType(t *testing.T) {
	for _, id := range []string{"", "8"} {
		query := buildQuery("Cheque", id, "", 20)
		if !strings.Contains(query, "from Purchase") || !strings.Contains(query, "PaymentType = 'Check'") {
			t.Fatal(query)
		}
	}
}
