package client

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExpenseMemoUpdatePreservesAccountingLines(t *testing.T) {
	line := []any{map[string]any{
		"Id": "1", "Amount": 32.0, "Description": "Original line",
		"AccountBasedExpenseLineDetail": map[string]any{
			"AccountRef": map[string]any{"value": "3"},
			"ClassRef":   map[string]any{"value": "7"},
			"TaxCodeRef": map[string]any{"value": "NON"},
		},
	}}
	existing := map[string]any{
		"Id": "123", "SyncToken": "2", "TxnDate": "2025-10-27",
		"TotalAmt": 32.0, "PrivateNote": "Normalized memo",
		"AccountRef":  map[string]any{"value": "204"},
		"PaymentType": "CreditCard", "Line": line,
	}
	memo := "Original  memo    spacing"
	body, err := buildUpdateBody("Purchase", map[string]string{"memo": memo}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["sparse"] != true || body["PrivateNote"] != memo || body["SyncToken"] != "2" {
		t.Fatalf("unexpected memo update: %#v", body)
	}
	for _, key := range []string{"Line", "AccountRef", "PaymentType"} {
		if !reflect.DeepEqual(body[key], existing[key]) {
			t.Fatalf("%s changed", key)
		}
	}
	for _, key := range []string{"TotalAmt", "TxnDate"} {
		if _, changed := body[key]; changed {
			t.Fatalf("memo-only update supplies %s", key)
		}
	}
	if existing["PrivateNote"] != "Normalized memo" {
		t.Fatal("mutated the input record")
	}
}

func TestExpenseUpdateHonorsExplicitLinesWithMemo(t *testing.T) {
	for _, alias := range []string{"line-items", "lines", "items"} {
		t.Run(alias, func(t *testing.T) {
			original := []any{map[string]any{
				"Id": "1", "Amount": 1.97,
				"DetailType": "AccountBasedExpenseLineDetail",
				"AccountBasedExpenseLineDetail": map[string]any{
					"AccountRef": map[string]any{"value": "11"},
					"ClassRef":   map[string]any{"value": "7"},
					"TaxCodeRef": map[string]any{"value": "NON"},
				},
			}}
			existing := map[string]any{"Id": "123", "SyncToken": "4", "AccountRef": map[string]any{"value": "204"}, "PaymentType": "CreditCard", "Line": original, "PrivateNote": "old"}
			raw := `[{"Id":"1","Amount":1.97,"DetailType":"AccountBasedExpenseLineDetail","AccountBasedExpenseLineDetail":{"AccountRef":{"value":"154"},"ClassRef":{"value":"7"},"TaxCodeRef":{"value":"NON"}}}]`
			body, err := buildUpdateBody("Purchase", map[string]string{alias: raw, "memo": "new"}, existing)
			if err != nil {
				t.Fatal(err)
			}
			var wanted []map[string]any
			if err := json.Unmarshal([]byte(raw), &wanted); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(body["Line"], wanted) || body["PrivateNote"] != "new" {
				t.Fatalf("requested lines or memo lost: %#v", body)
			}
			for _, key := range []string{"AccountRef", "PaymentType", "SyncToken"} {
				if !reflect.DeepEqual(body[key], existing[key]) {
					t.Fatalf("%s changed", key)
				}
			}
			if existing["PrivateNote"] != "old" || !reflect.DeepEqual(existing["Line"], original) {
				t.Fatal("mutated existing entity")
			}
			if original[0].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)["AccountRef"].(map[string]any)["value"] != "11" {
				t.Fatal("mutated original category")
			}
		})
	}
}

func TestExpenseUpdateRejectsInvalidExplicitLines(t *testing.T) {
	for _, raw := range []string{"[]", "[{", "account,amount\n154,1.97"} {
		_, err := buildUpdateBody("Purchase", map[string]string{"line-items": raw}, map[string]any{"Id": "123", "SyncToken": "1"})
		if err == nil {
			t.Fatalf("accepted invalid explicit lines %q", raw)
		}
	}
}

// collectFlags lands unset bool flags as "false"; explicit --line-items must
// not trip the single-line billable/project write that assumes one line.
func TestExpenseUpdateLineItemsIgnoresDefaultBillable(t *testing.T) {
	existing := map[string]any{
		"Id": "31767", "SyncToken": "1", "PaymentType": "CreditCard",
		"AccountRef": map[string]any{"value": "204"},
		"Line": []any{map[string]any{
			"Id": "1", "Amount": 10.02, "DetailType": "AccountBasedExpenseLineDetail",
			"AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "203"}},
		}},
	}
	raw := `[{"Id":"1","Amount":4.70,"DetailType":"AccountBasedExpenseLineDetail","AccountBasedExpenseLineDetail":{"AccountRef":{"value":"203"},"ClassRef":{"value":"3700000000001534406"}}},{"Id":"2","Amount":5.32,"DetailType":"AccountBasedExpenseLineDetail","AccountBasedExpenseLineDetail":{"AccountRef":{"value":"203"},"ClassRef":{"value":"3700000000001477086"}}}]`
	body, err := buildUpdateBody("Purchase", map[string]string{"line-items": raw, "billable": "false", "project": ""}, existing)
	if err != nil {
		t.Fatalf("multi-line --line-items rejected by default billable flag: %v", err)
	}
	var wanted []map[string]any
	if err := json.Unmarshal([]byte(raw), &wanted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body["Line"], wanted) {
		t.Fatalf("supplied lines replaced by synth-flag write: %#v", body["Line"])
	}
}
