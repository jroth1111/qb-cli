package client

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV3MutationRequiresIndependentIntendedState(t *testing.T) {
	for _, mode := range []string{"applied", "unchanged", "wrong identity", "changed funding", "read failure"} {
		t.Run(mode, func(t *testing.T) {
			saveUsable(t)
			posts, gets := 0, 0
			state := map[string]any{"Id": "51", "SyncToken": "1", "PrivateNote": "old", "PaymentType": "Cash", "AccountRef": map[string]any{"value": "44"}, "Line": []any{map[string]any{"Id": "1", "Amount": 10.0, "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "7"}}}}}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					var submitted map[string]any
					_ = json.NewDecoder(r.Body).Decode(&submitted)
					if mode != "unchanged" {
						maps.Copy(state, submitted)
						state["SyncToken"] = "2"
					}
					if mode == "changed funding" {
						state["AccountRef"] = map[string]any{"value": "209"}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Purchase": map[string]any{"Id": "51", "PrivateNote": "new"}})
					return
				}
				gets++
				if posts > 0 && mode == "read failure" {
					w.WriteHeader(503)
					return
				}
				if posts > 0 && mode == "wrong identity" {
					state["Id"] = "99"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"Purchase": state})
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			res, err := ReplayMutate(context.Background(), "Purchase", "update", "51", map[string]string{"memo": "new"})
			if mode == "applied" {
				if err != nil || !res.Verified {
					t.Fatalf("verified mutation failed: %v", err)
				}
			} else if !errors.Is(err, ErrMutationUnverified) {
				t.Fatalf("false verified success: %v", err)
			}
			if posts != 1 || gets != 2 {
				t.Fatalf("writes=%d reads=%d; must not replay mutation", posts, gets)
			}
		})
	}
}

func TestReadbackComparisonRejectsDroppedLinesAndClass(t *testing.T) {
	want := normalizedJSON(map[string]any{"Line": []any{map[string]any{"Id": "1", "Amount": 10, "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "7"}}}}})
	for _, got := range []any{map[string]any{"Line": []any{}}, map[string]any{"Line": []any{map[string]any{"Id": "1", "Amount": 10, "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "7"}, "ClassRef": map[string]any{"value": "wrong"}}}}}} {
		if expectedFields(want, normalizedJSON(got), "Purchase") == nil {
			t.Fatal("accepted unexpected accounting lines/Class")
		}
	}
}

func TestReadbackPreservesUnrequestedNestedFields(t *testing.T) {
	before := map[string]any{"BillAddr": map[string]any{"Line1": "old", "City": "Melbourne"}}
	request := map[string]any{"BillAddr": map[string]any{"Line1": "new"}}
	bad := map[string]any{"BillAddr": map[string]any{"Line1": "new"}}
	if preservedMapFields(before, request, bad, "Customer") == nil {
		t.Fatal("silently lost unrequested address field")
	}
	good := map[string]any{"BillAddr": map[string]any{"Line1": "new", "City": "Melbourne"}}
	if err := preservedMapFields(before, request, good, "Customer"); err != nil {
		t.Fatal(err)
	}
}
