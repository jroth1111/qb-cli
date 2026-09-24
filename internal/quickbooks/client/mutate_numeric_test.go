package client

// mutate_numeric_test.go pins numeric flag handling in the native v3
// mutation request builders:
//   - an explicit finite zero must reach the request body unchanged; it must
//     never coerce into the omitted-flag defaults (1 or 0.01),
//   - invalid or non-finite input must fail before transport instead of
//     silently defaulting,
//   - omitted flags keep their existing defaults,
//   - builders only judge flags they actually consume: the raw --lines JSON
//     path never touches --amount, and entities that ignore a flag ignore
//     its invalid values too.
//
// Builder tests need no credentials or network. The wire test uses the
// package-standard httptest loopback via saveUsable + interceptHTTP.

import (
	"context"
	"maps"
	"net/http"
	"strings"
	"testing"
)

// numericField reads a numeric request field whether the builder stored it
// as float64 (amounts) or int (DueDays), or it arrived json-decoded on the
// wire.
func numericField(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case nil:
		t.Fatal("field absent")
	default:
		t.Fatalf("field not numeric: %T %v", v, v)
	}
	return 0
}

func firstCreateLine(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	lines, ok := body["Line"].([]map[string]any)
	if !ok || len(lines) == 0 {
		t.Fatalf("Line = %v", body["Line"])
	}
	return lines[0]
}

func createLineAmount(t *testing.T, body map[string]any) float64 {
	t.Helper()
	return numericField(t, firstCreateLine(t, body)["Amount"])
}

func totalAmt(t *testing.T, body map[string]any) float64 {
	t.Helper()
	return numericField(t, body["TotalAmt"])
}

// TestMutateNumericCreatePreservesExplicitZero drives create bodies with an
// explicit "0" numeric flag and asserts the wire field stays 0. Every entry
// regressed if a `value == 0 -> default` coercion returned.
func TestMutateNumericCreatePreservesExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		entity string
		flags  map[string]string
		field  func(t *testing.T, body map[string]any) float64
	}{
		{"invoice line amount", "Invoice",
			map[string]string{"customer": "1", "amount": "0", "item": "3"}, createLineAmount},
		{"estimate line amount", "Estimate",
			map[string]string{"customer": "1", "amount": "0", "item": "3"}, createLineAmount},
		{"sales receipt line amount", "SalesReceipt",
			map[string]string{"customer": "1", "amount": "0", "item": "3", "deposit-to": "4"}, createLineAmount},
		{"credit memo line amount", "CreditMemo",
			map[string]string{"customer": "1", "amount": "0", "item": "3"}, createLineAmount},
		{"payment total", "Payment",
			map[string]string{"customer": "1", "amount": "0"}, totalAmt},
		{"payment linked line", "Payment",
			map[string]string{"customer": "1", "amount": "0", "invoice-id": "9"}, createLineAmount},
		{"bill line amount", "Bill",
			map[string]string{"supplier": "2", "amount": "0"}, createLineAmount},
		{"vendor credit line amount", "VendorCredit",
			map[string]string{"supplier": "2", "amount": "0"}, createLineAmount},
		{"purchase order line amount", "PurchaseOrder",
			map[string]string{"supplier": "2", "amount": "0"}, createLineAmount},
		{"item receipt line amount", "ItemReceipt",
			map[string]string{"supplier": "2", "amount": "0"}, createLineAmount},
		{"bill payment total", "BillPayment",
			map[string]string{"supplier": "2", "bill-id": "10", "account": "48", "amount": "0"}, totalAmt},
		{"bill payment line", "BillPayment",
			map[string]string{"supplier": "2", "bill-id": "10", "account": "48", "amount": "0"}, createLineAmount},
		{"purchase line amount", "Purchase",
			map[string]string{"account": "48", "category-account": "7", "amount": "0"}, createLineAmount},
		{"transfer amount", "Transfer",
			map[string]string{"from-account": "1", "to-account": "2", "amount": "0"},
			func(t *testing.T, b map[string]any) float64 { return numericField(t, b["Amount"]) }},
		{"deposit line amount", "Deposit",
			map[string]string{"account": "4", "amount": "0"}, createLineAmount},
		{"budget detail amount", "Budget",
			map[string]string{"name": "B", "amount": "0"},
			func(t *testing.T, b map[string]any) float64 {
				return numericField(t, b["BudgetDetail"].([]map[string]any)[0]["Amount"])
			}},
		{"recurring line amount", "RecurringTransaction",
			map[string]string{"customer": "1", "amount": "0"},
			func(t *testing.T, b map[string]any) float64 {
				return createLineAmount(t, b["Invoice"].(map[string]any))
			}},
		{"item qty on hand", "Item",
			map[string]string{"name": "I", "type": "Inventory", "qty": "0"},
			func(t *testing.T, b map[string]any) float64 { return numericField(t, b["QtyOnHand"]) }},
		{"inventory adjust qty", "InventoryAdjustment",
			map[string]string{"item": "3", "account": "81", "qty": "0"}, qtyDiff},
		{"inventory adjust amount fallback", "InventoryAdjustment",
			map[string]string{"item": "3", "account": "81", "amount": "0"}, qtyDiff},
		{"term due days", "Term",
			map[string]string{"name": "T", "due-days": "0"},
			func(t *testing.T, b map[string]any) float64 { return numericField(t, b["DueDays"]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := buildCreateBody(tc.entity, tc.flags)
			if err != nil {
				t.Fatalf("buildCreateBody: %v", err)
			}
			if got := tc.field(t, body); got != 0 {
				t.Fatalf("explicit zero coerced to %v (body %v)", got, body)
			}
		})
	}
}

func qtyDiff(t *testing.T, body map[string]any) float64 {
	t.Helper()
	line := firstCreateLine(t, body)
	return numericField(t, line["ItemAdjustmentLineDetail"].(map[string]any)["QtyDiff"])
}

// TestMutateNumericOmittedDefaultsUnchanged pins the pre-existing defaults
// when the numeric flag is absent (or present-but-empty, as collectFlags
// emits for unset flags). These must not move while explicit zero is fixed.
func TestMutateNumericOmittedDefaultsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		entity string
		flags  map[string]string
		field  func(t *testing.T, body map[string]any) float64
		want   float64
	}{
		{"invoice line default", "Invoice",
			map[string]string{"customer": "1", "item": "3"}, createLineAmount, 1},
		{"invoice line empty flag", "Invoice",
			map[string]string{"customer": "1", "item": "3", "amount": ""}, createLineAmount, 1},
		{"payment total default", "Payment",
			map[string]string{"customer": "1"}, totalAmt, 0.01},
		{"bill line default", "Bill",
			map[string]string{"supplier": "2"}, createLineAmount, 1},
		{"bill payment total default", "BillPayment",
			map[string]string{"supplier": "2", "bill-id": "10", "account": "48"}, totalAmt, 0.01},
		{"purchase line default", "Purchase",
			map[string]string{"account": "48", "category-account": "7"}, createLineAmount, 1},
		{"transfer omitted stays zero", "Transfer",
			map[string]string{"from-account": "1", "to-account": "2"},
			func(t *testing.T, b map[string]any) float64 { return numericField(t, b["Amount"]) }, 0},
		{"deposit omitted stays zero", "Deposit",
			map[string]string{"account": "4"}, createLineAmount, 0},
		{"budget detail default", "Budget",
			map[string]string{"name": "B"},
			func(t *testing.T, b map[string]any) float64 {
				return numericField(t, b["BudgetDetail"].([]map[string]any)[0]["Amount"])
			}, 1},
		{"recurring line default", "RecurringTransaction",
			map[string]string{"customer": "1"},
			func(t *testing.T, b map[string]any) float64 {
				return createLineAmount(t, b["Invoice"].(map[string]any))
			}, 1},
		{"item qty default", "Item",
			map[string]string{"name": "I", "type": "Inventory"},
			func(t *testing.T, b map[string]any) float64 { return numericField(t, b["QtyOnHand"]) }, 1},
		{"inventory adjust default", "InventoryAdjustment",
			map[string]string{"item": "3", "account": "81"}, qtyDiff, 1},
		{"inventory adjust amount fallback", "InventoryAdjustment",
			map[string]string{"item": "3", "account": "81", "amount": "3"}, qtyDiff, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := buildCreateBody(tc.entity, tc.flags)
			if err != nil {
				t.Fatalf("buildCreateBody: %v", err)
			}
			if got := tc.field(t, body); got != tc.want {
				t.Fatalf("omitted-flag default changed: got %v, want %v (body %v)", got, tc.want, body)
			}
		})
	}

	// Omitted numeric fields must stay absent, not appear as 0.
	body, err := buildCreateBody("Term", map[string]string{"name": "T"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["DueDays"]; ok {
		t.Fatalf("DueDays appeared without --due-days: %v", body["DueDays"])
	}
	body, err = buildUpdateBody("Invoice", map[string]string{}, map[string]any{"Id": "9", "SyncToken": "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body["TotalAmt"]; ok {
		t.Fatalf("TotalAmt appeared without --amount: %v", body["TotalAmt"])
	}
}

// TestMutateNumericUpdatePreservesExplicitZero pins sparse-update bodies: an
// explicit --amount 0 must restate TotalAmt (and the first line) as 0 rather
// than being treated as "not provided".
func TestMutateNumericUpdatePreservesExplicitZero(t *testing.T) {
	existing := map[string]any{
		"Id": "9", "SyncToken": "0", "TotalAmt": 10.0,
		"Line": []any{map[string]any{"Amount": 10.0, "DetailType": "SalesItemLineDetail"}},
	}
	body, err := buildUpdateBody("Invoice", map[string]string{"amount": "0"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if got := numericField(t, body["TotalAmt"]); got != 0 {
		t.Fatalf("update --amount 0 coerced/dropped: TotalAmt = %v", body["TotalAmt"])
	}
	lines := body["Line"].([]any)
	if got := numericField(t, lines[0].(map[string]any)["Amount"]); got != 0 {
		t.Fatalf("update --amount 0 line restated to %v", lines[0])
	}

	trExisting := map[string]any{
		"Id": "5", "SyncToken": "1", "Amount": 5.0,
		"FromAccountRef": map[string]any{"value": "1"},
		"ToAccountRef":   map[string]any{"value": "2"},
	}
	body, err = buildUpdateBody("Transfer", map[string]string{"amount": "0"}, trExisting)
	if err != nil {
		t.Fatal(err)
	}
	if got := numericField(t, body["Amount"]); got != 0 {
		t.Fatalf("transfer update --amount 0 dropped: Amount = %v", body["Amount"])
	}
}

// TestMutateNumericRejectsInvalidAndNonFinite drives every builder path that
// consumes a numeric flag with malformed and non-finite values. Each must
// return an error naming the flag and a nil body — before any transport.
func TestMutateNumericRejectsInvalidAndNonFinite(t *testing.T) {
	invalid := []string{"abc", "NaN", "nan", "Inf", "+Inf", "-Inf", "1e309", "-1e309", "$5", "1,000", "1.2.3", "12x"}
	creates := []struct {
		name   string
		entity string
		flag   string
		flags  map[string]string
	}{
		{"invoice amount", "Invoice", "amount", map[string]string{"customer": "1"}},
		{"payment amount", "Payment", "amount", map[string]string{"customer": "1"}},
		{"bill amount", "Bill", "amount", map[string]string{"supplier": "2"}},
		{"bill payment amount", "BillPayment", "amount", map[string]string{"supplier": "2", "bill-id": "10", "account": "48"}},
		{"purchase amount", "Purchase", "amount", map[string]string{"account": "48", "category-account": "7"}},
		{"transfer amount", "Transfer", "amount", map[string]string{"from-account": "1", "to-account": "2"}},
		{"deposit amount", "Deposit", "amount", map[string]string{"account": "4"}},
		{"journal entry amount", "JournalEntry", "amount", map[string]string{"from-account": "1", "to-account": "2"}},
		{"budget amount", "Budget", "amount", map[string]string{"name": "B"}},
		{"recurring amount", "RecurringTransaction", "amount", map[string]string{"customer": "1"}},
		{"item qty", "Item", "qty", map[string]string{"name": "I", "type": "Inventory"}},
		{"inventory adjust qty", "InventoryAdjustment", "qty", map[string]string{"item": "3", "account": "81"}},
		{"inventory adjust amount", "InventoryAdjustment", "amount", map[string]string{"item": "3", "account": "81"}},
		{"term due-days", "Term", "due-days", map[string]string{"name": "T"}},
	}
	for _, c := range creates {
		for _, v := range invalid {
			t.Run(c.name+"/"+v, func(t *testing.T) {
				flags := map[string]string{c.flag: v}
				maps.Copy(flags, c.flags)
				body, err := buildCreateBody(c.entity, flags)
				if err == nil || body != nil {
					t.Fatalf("--%s %q returned body=%v err=%v; want pre-transport rejection", c.flag, v, body, err)
				}
				if !strings.Contains(err.Error(), "--"+c.flag) {
					t.Fatalf("error %q must name --%s", err, c.flag)
				}
			})
		}
	}

	// Updates: --amount is consumed generically, so it must be judged for
	// every entity including JournalEntry (whose stricter non-zero rule
	// still applies to finite zeros below).
	for _, v := range invalid {
		for _, entity := range []string{"Invoice", "Bill", "Transfer", "JournalEntry"} {
			t.Run("update/"+entity+"/"+v, func(t *testing.T) {
				body, err := buildUpdateBody(entity, map[string]string{"amount": v},
					map[string]any{"Id": "9", "SyncToken": "0"})
				if err == nil || body != nil {
					t.Fatalf("update --amount %q returned body=%v err=%v; want pre-transport rejection", v, body, err)
				}
			})
		}
	}
}

// TestMutateNumericJournalEntryZeroStillRejected pins the domain rule that a
// journal entry cannot carry a zero amount: explicit --amount 0 errors
// loudly (never coerces to a default, never posts).
func TestMutateNumericJournalEntryZeroStillRejected(t *testing.T) {
	if _, err := buildCreateBody("JournalEntry", map[string]string{
		"from-account": "1", "to-account": "2", "amount": "0",
	}); err == nil {
		t.Fatal("journal create --amount 0 must be rejected")
	}
	if _, err := buildUpdateBody("JournalEntry", map[string]string{"amount": "0"},
		map[string]any{"Id": "7", "SyncToken": "0"}); err == nil {
		t.Fatal("journal update --amount 0 must be rejected")
	}
}

// TestMutateNumericScopeOnlyConsumingPaths pins the boundary: a numeric flag
// is only judged where the builder actually consumes it. The raw --lines
// JSON path bypasses --amount entirely, and entities that ignore a flag
// keep ignoring its invalid values.
func TestMutateNumericScopeOnlyConsumingPaths(t *testing.T) {
	jeLines := `[{"Amount":5,"DetailType":"JournalEntryLineDetail","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"1"}}}]`
	body, err := buildCreateBody("JournalEntry", map[string]string{"lines": jeLines, "amount": "abc"})
	if err != nil {
		t.Fatalf("raw --lines path must not consume --amount: %v", err)
	}
	if got := numericField(t, body["Line"].([]map[string]any)[0]["Amount"]); got != 5 {
		t.Fatalf("raw line JSON altered: %v", body["Line"])
	}

	// Customer create never reads --amount; an invalid value stays ignored.
	if _, err := buildCreateBody("Customer", map[string]string{"name": "X", "amount": "abc"}); err != nil {
		t.Fatalf("non-consuming entity must ignore --amount: %v", err)
	}
	// Service items never read --qty.
	if _, err := buildCreateBody("Item", map[string]string{"name": "I", "type": "Service", "qty": "abc"}); err != nil {
		t.Fatalf("non-inventory item must ignore --qty: %v", err)
	}
}

// TestMutateNumericWireContract proves the fix end-to-end through
// ReplayMutate against the loopback fake v3 server: explicit zeros land in
// the POST body, invalid input dies before any POST (updates may still GET
// the row first for its SyncToken, but must never POST).
func TestMutateNumericWireContract(t *testing.T) {
	t.Run("create explicit zero posts zero", func(t *testing.T) {
		saveUsable(t)
		srv := newExpServer(t, "", `{"Invoice":{"Id":"200","SyncToken":"0","TotalAmt":0}}`)
		interceptHTTP(t, srv.URL)

		res, err := ReplayMutate(context.Background(), "Invoice", "create", "", map[string]string{
			"customer": "1", "amount": "0", "item": "3",
		})
		if err != nil {
			t.Fatalf("invoice create: %v", err)
		}
		if res.Status != http.StatusOK {
			t.Fatalf("envelope = %+v", res)
		}
		expAssertV3Post(t, srv, "invoice", "")
		body := expBodyMap(t, srv.lastBody)
		if got := numericField(t, body["Line"].([]any)[0].(map[string]any)["Amount"]); got != 0 {
			t.Fatalf("wire Line[0].Amount = %v, want explicit 0", got)
		}
	})

	t.Run("payment create explicit zero posts zero total", func(t *testing.T) {
		saveUsable(t)
		srv := newExpServer(t, "", `{"Payment":{"Id":"300","SyncToken":"0","TotalAmt":0}}`)
		interceptHTTP(t, srv.URL)

		if _, err := ReplayMutate(context.Background(), "Payment", "create", "", map[string]string{
			"customer": "1", "amount": "0",
		}); err != nil {
			t.Fatalf("payment create: %v", err)
		}
		body := expBodyMap(t, srv.lastBody)
		if got := numericField(t, body["TotalAmt"]); got != 0 {
			t.Fatalf("wire TotalAmt = %v, want explicit 0", got)
		}
	})

	t.Run("create invalid fails before any request", func(t *testing.T) {
		saveUsable(t)
		srv := newExpServer(t, "", `{}`)
		interceptHTTP(t, srv.URL)

		for _, v := range []string{"abc", "NaN", "Inf", "1e309"} {
			_, err := ReplayMutate(context.Background(), "Invoice", "create", "", map[string]string{
				"customer": "1", "amount": v,
			})
			if err == nil || !strings.Contains(err.Error(), "--amount") {
				t.Fatalf("--amount %q: err = %v, want finite-number rejection", v, err)
			}
		}
		if gets, posts := srv.counts(); gets != 0 || posts != 0 {
			t.Fatalf("invalid input must not reach transport; gets=%d posts=%d", gets, posts)
		}
	})

	t.Run("update explicit zero posts zero", func(t *testing.T) {
		saveUsable(t)
		srv := newExpServer(t, expBillExisting, expBillExisting)
		interceptHTTP(t, srv.URL)

		if _, err := ReplayMutate(context.Background(), "Bill", "update", "46", map[string]string{
			"amount": "0",
		}); err != nil {
			t.Fatalf("bill update: %v", err)
		}
		gets, posts := srv.counts()
		if gets != 2 || posts != 1 {
			t.Fatalf("update must GET, POST once, then GET readback; gets=%d posts=%d", gets, posts)
		}
		body := expBodyMap(t, srv.lastBody)
		if got := numericField(t, body["TotalAmt"]); got != 0 {
			t.Fatalf("wire TotalAmt = %v, want explicit 0", got)
		}
		if got := numericField(t, body["Line"].([]any)[0].(map[string]any)["Amount"]); got != 0 {
			t.Fatalf("wire Line[0].Amount = %v, want explicit 0", got)
		}
	})

	t.Run("update invalid fetches but never posts", func(t *testing.T) {
		saveUsable(t)
		srv := newExpServer(t, expBillExisting, expBillExisting)
		interceptHTTP(t, srv.URL)

		_, err := ReplayMutate(context.Background(), "Bill", "update", "46", map[string]string{
			"amount": "bogus",
		})
		if err == nil || !strings.Contains(err.Error(), "--amount") {
			t.Fatalf("err = %v, want finite-number rejection", err)
		}
		gets, posts := srv.counts()
		if gets != 1 || posts != 0 {
			t.Fatalf("invalid update must fail before POST; gets=%d posts=%d", gets, posts)
		}
	})
}
