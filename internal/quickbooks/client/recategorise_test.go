package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recatServer answers the v3 purchase GET and POST.
func recatServer(t *testing.T, purchaseBody string) (*domServer, *map[string]any) {
	t.Helper()
	var lastPost map[string]any
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(purchaseBody))
			return
		}
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		_ = json.Unmarshal(b, &lastPost)
		_, _ = w.Write([]byte(`{"Purchase":{"Id":"51","SyncToken":"3"}}`))
	}))
	t.Cleanup(s.Close)
	return s, &lastPost
}

const recatPurchaseFixture = `{"Purchase":{
  "Id":"51","SyncToken":"1","PaymentType":"Cash",
  "EntityRef":{"value":"3","type":"Vendor"},
  "Line":[
    {"Id":"1","Amount":25,"DetailType":"AccountBasedExpenseLineDetail",
     "AccountBasedExpenseLineDetail":{"AccountRef":{"value":"8"}}},
    {"Id":"2","Amount":15,"DetailType":"AccountBasedExpenseLineDetail",
     "AccountBasedExpenseLineDetail":{"AccountRef":{"value":"8"}}}
  ]}}`

func TestRepairCreditCardTypeRequiresVerifiedFundingAccount(t *testing.T) {
	for _, tc := range []struct {
		name, payment, accountType, accountID string
		ok                                    bool
	}{
		{"supported", "Check", "Credit Card", "204", true},
		{"bank account", "Check", "Bank", "204", false},
		{"wrong account", "Check", "Credit Card", "209", false},
		{"cash payment", "Cash", "Credit Card", "204", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			existing := map[string]any{"PaymentType": tc.payment, "AccountRef": map[string]any{"value": "204"}}
			body := map[string]any{"PaymentType": tc.payment}
			err := repairCreditCardType(existing, body, map[string]any{"Id": tc.accountID, "AccountType": tc.accountType})
			if (err == nil) != tc.ok {
				t.Fatalf("error = %v", err)
			}
			want := tc.payment
			if tc.ok {
				want = "CreditCard"
			}
			if body["PaymentType"] != want || existing["PaymentType"] != tc.payment {
				t.Fatal("unexpected type mutation")
			}
		})
	}
}

func TestRecategoriseSwapsAccountRef(t *testing.T) {
	saveUsable(t)
	srv, lastPost := recatServer(t, recatPurchaseFixture)
	interceptHTTP(t, srv.URL)

	res, err := ReplayExpenseRecategorise(context.Background(), map[string]string{
		"id": "51", "category-id": "9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "recategorise" || res.Item.ID != "51" {
		t.Fatalf("result = %+v", res)
	}
	body := *lastPost
	if body["sparse"] != true || body["Id"] != "51" || body["SyncToken"] != "1" {
		t.Fatalf("body head = %#v", body)
	}
	lines, _ := body["Line"].([]any)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", body["Line"])
	}
	for _, lv := range lines {
		l, _ := lv.(map[string]any)
		d, _ := l["AccountBasedExpenseLineDetail"].(map[string]any)
		ref, _ := d["AccountRef"].(map[string]any)
		if ref["value"] != "9" {
			t.Fatalf("line %v accountRef = %#v", l["Id"], ref)
		}
	}
}

func TestRecategoriseRejectsMissingOrWrongReceipt(t *testing.T) {
	for _, reply := range []string{`{"Fault":{"type":"ValidationFault"}}`, `{"Purchase":{"Id":"999"}}`} {
		t.Run(reply, func(t *testing.T) {
			saveUsable(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(recatPurchaseFixture))
				} else {
					_, _ = w.Write([]byte(reply))
				}
			}))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			result, err := ReplayExpenseRecategorise(context.Background(), map[string]string{"id": "51", "category-id": "9"})
			if err == nil || result != nil || !strings.Contains(err.Error(), "receipt") {
				t.Fatalf("result=%v err=%v", result, err)
			}
		})
	}
}

func TestRecategoriseLineIDScopesUpdate(t *testing.T) {
	saveUsable(t)
	srv, lastPost := recatServer(t, recatPurchaseFixture)
	interceptHTTP(t, srv.URL)

	_, err := ReplayExpenseRecategorise(context.Background(), map[string]string{
		"id": "51", "category-id": "9", "line-id": "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	lines, _ := (*lastPost)["Line"].([]any)
	l0, _ := lines[0].(map[string]any)
	d0, _ := l0["AccountBasedExpenseLineDetail"].(map[string]any)
	if d0["AccountRef"].(map[string]any)["value"] != "8" {
		t.Fatalf("line 1 changed under --line-id 2: %#v", d0)
	}
	l1, _ := lines[1].(map[string]any)
	d1, _ := l1["AccountBasedExpenseLineDetail"].(map[string]any)
	if d1["AccountRef"].(map[string]any)["value"] != "9" {
		t.Fatalf("line 2 not swapped: %#v", d1)
	}
}

func TestRecategorisePayeeMovesEntityRef(t *testing.T) {
	saveUsable(t)
	srv, lastPost := recatServer(t, recatPurchaseFixture)
	interceptHTTP(t, srv.URL)

	_, err := ReplayExpenseRecategorise(context.Background(), map[string]string{
		"id": "51", "payee-id": "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := (*lastPost)["EntityRef"].(map[string]any)
	if ref["value"] != "7" || ref["type"] != "Vendor" {
		t.Fatalf("EntityRef = %#v", ref)
	}
	if _, has := (*lastPost)["Line"]; has {
		t.Fatal("payee-only recategorise sent lines")
	}
}

func TestRecategoriseValidation(t *testing.T) {
	saveUsable(t)
	srv, lastPost := recatServer(t, recatPurchaseFixture)
	interceptHTTP(t, srv.URL)

	if _, err := ReplayExpenseRecategorise(context.Background(), map[string]string{"category-id": "9"}); err == nil {
		t.Fatal("missing --id accepted")
	}
	if _, err := ReplayExpenseRecategorise(context.Background(), map[string]string{"id": "51"}); err == nil {
		t.Fatal("no category/payee accepted")
	}
	if _, err := ReplayExpenseRecategorise(context.Background(), map[string]string{"id": "51", "category-id": "9", "line-id": "99"}); err == nil || !strings.Contains(err.Error(), "matches no") {
		t.Fatalf("unmatched --line-id accepted: %v", err)
	}
	if *lastPost != nil {
		t.Fatal("validation failures still posted")
	}
}

func TestRecategoriseClassPreservesAccountAndOtherLines(t *testing.T) {
	saveUsable(t)
	srv, lastPost := recatServer(t, recatPurchaseFixture)
	interceptHTTP(t, srv.URL)
	_, err := ReplayExpenseRecategorise(context.Background(), map[string]string{
		"id": "51", "line-id": "2", "class-id": "1303",
		"expected-sync-token": "1", "expected-category-id": "8", "expected-class-id": "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := (*lastPost)["Line"].([]any)
	first := lines[0].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)
	second := lines[1].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)
	if first["ClassRef"] != nil || expenseRefValue(first["AccountRef"]) != "8" {
		t.Fatalf("other line changed: %#v", first)
	}
	if expenseRefValue(second["ClassRef"]) != "1303" || expenseRefValue(second["AccountRef"]) != "8" {
		t.Fatalf("selected line wrong: %#v", second)
	}
}

func TestRecategoriseClassRejectsStalePreconditionsBeforePost(t *testing.T) {
	for _, tc := range []map[string]string{
		{"id": "51", "line-id": "2", "class-id": "1303", "expected-sync-token": "0"},
		{"id": "51", "line-id": "2", "class-id": "1303", "expected-category-id": "203"},
		{"id": "51", "line-id": "2", "class-id": "1303", "expected-class-id": "1303"},
		{"id": "51", "class-id": "1303"},
	} {
		saveUsable(t)
		srv, lastPost := recatServer(t, recatPurchaseFixture)
		interceptHTTP(t, srv.URL)
		if _, err := ReplayExpenseRecategorise(context.Background(), tc); err == nil {
			t.Fatalf("accepted stale or ambiguous flags: %#v", tc)
		}
		if *lastPost != nil {
			t.Fatalf("posted despite validation failure: %#v", tc)
		}
	}
}
