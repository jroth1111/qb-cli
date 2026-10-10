package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestSingleDepositQueryRetainsPostedRecord(t *testing.T) {
	saveUsable(t)
	want := map[string]any{
		"Id": "42", "SyncToken": "3", "TxnDate": "2021-12-23", "TotalAmt": 44.0,
		"DepositToAccountRef": map[string]any{"value": "204"},
		"PrivateNote":         "Original bank memo",
		"Line": []any{map[string]any{
			"Id": "1", "Amount": 44.0, "Description": "Original linen refund",
			"DetailType": "DepositLineDetail",
			"DepositLineDetail": map[string]any{
				"AccountRef": map[string]any{"value": "113"},
				"ClassRef":   map[string]any{"value": "7"},
				"TaxCodeRef": map[string]any{"value": "NON"},
			},
		}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("read performed a write")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"QueryResponse": map[string]any{"Deposit": []any{want}}})
	}))
	defer server.Close()
	interceptHTTP(t, server.URL)
	result, err := ReplayQuery(context.Background(), "Deposit", "42", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != http.StatusOK || len(result.Items) != 1 || !reflect.DeepEqual(result.Detail, want) {
		t.Fatalf("full posted record lost: %+v", result)
	}
}

func TestSingleDepositQueryRejectsMismatchedOrAmbiguousRecords(t *testing.T) {
	for _, ids := range [][]string{{"99"}, {"42", "99"}} {
		t.Run(ids[0], func(t *testing.T) {
			saveUsable(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var rows []any
				for _, id := range ids {
					rows = append(rows, map[string]any{"Id": id, "TotalAmt": 44})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"QueryResponse": map[string]any{"Deposit": rows}})
			}))
			defer server.Close()
			interceptHTTP(t, server.URL)
			if _, err := ReplayQuery(context.Background(), "Deposit", "42", "", 0); err == nil {
				t.Fatal("wrong or ambiguous deposit identity accepted")
			}
		})
	}
}
