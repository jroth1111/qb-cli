package client

import "testing"

func TestRecategoriseClassNoneClearsSelectedLineOnly(t *testing.T) {
	existing := map[string]any{
		"Id": "123", "SyncToken": "4", "PaymentType": "CreditCard",
		"AccountRef":  map[string]any{"value": "204"},
		"PrivateNote": "original bank text [Card: Jacob]",
		"Line": []any{
			map[string]any{"Id": "1", "Amount": 22.0, "AccountBasedExpenseLineDetail": map[string]any{
				"AccountRef": map[string]any{"value": "90"},
				"ClassRef":   map[string]any{"value": "parent"},
				"TaxCodeRef": map[string]any{"value": "NON"},
			}},
			map[string]any{"Id": "2", "Amount": 11.0, "AccountBasedExpenseLineDetail": map[string]any{
				"AccountRef": map[string]any{"value": "37"},
				"ClassRef":   map[string]any{"value": "maggie"},
				"TaxCodeRef": map[string]any{"value": "NON"},
			}},
		},
	}
	body, err := buildRecategoriseBodyWithClass(existing, "97", "none", "", "1")
	if err != nil {
		t.Fatal(err)
	}
	lines := body["Line"].([]any)
	first := lines[0].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)
	second := lines[1].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)
	if _, present := first["ClassRef"]; present {
		t.Fatalf("ClassRef was not cleared: %#v", first)
	}
	if expenseRefValue(first["AccountRef"]) != "97" || expenseRefValue(second["AccountRef"]) != "37" || expenseRefValue(second["ClassRef"]) != "maggie" {
		t.Fatalf("wrong line changed: %#v %#v", first, second)
	}
	if body["PaymentType"] != "CreditCard" || expenseRefValue(body["AccountRef"]) != "204" || body["PrivateNote"] != existing["PrivateNote"] {
		t.Fatalf("funding or memo changed: %#v", body)
	}
	original := existing["Line"].([]any)[0].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)
	if expenseRefValue(original["ClassRef"]) != "parent" || expenseRefValue(original["AccountRef"]) != "90" {
		t.Fatal("input Purchase was mutated")
	}
}
