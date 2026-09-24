package client

import "testing"

func TestRecategoriseExplicitlyPreservesExistingMemo(t *testing.T) {
	for _, memo := range []string{"PayPal 4029357733 [Card: Jacob]", ""} {
		existing := map[string]any{
			"Id": "25819", "SyncToken": "3", "PrivateNote": memo,
			"PaymentType": "CreditCard", "AccountRef": map[string]any{"value": "204"},
			"Line": []any{map[string]any{
				"Id": "1", "Amount": 0.49,
				"AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "11"}},
			}},
		}
		body, err := buildRecategoriseBody(existing, "56", "", "")
		if err != nil {
			t.Fatal(err)
		}
		got, present := body["PrivateNote"]
		if !present || got != memo {
			t.Fatalf("memo omitted or changed: %#v", body)
		}
		if existing["PrivateNote"] != memo {
			t.Fatal("input memo changed")
		}
	}
}
