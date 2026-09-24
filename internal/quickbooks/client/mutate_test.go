package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildCreateInvoiceRequiresCustomer(t *testing.T) {
	_, err := buildCreateBody("Invoice", map[string]string{"amount": "1"})
	if err == nil {
		t.Fatal("expected customer required")
	}
	body, err := buildCreateBody("Invoice", map[string]string{"customer": "1", "amount": "1.5", "item": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if body["CustomerRef"].(map[string]any)["value"] != "1" {
		t.Fatalf("%v", body)
	}
}

func TestBuildCreateCustomerRequiresName(t *testing.T) {
	_, err := buildCreateBody("Customer", map[string]string{})
	if err == nil {
		t.Fatal("expected name")
	}
}

func TestBuildCreateEmployeeSetsGivenName(t *testing.T) {
	_, err := buildCreateBody("Employee", map[string]string{})
	if err == nil {
		t.Fatal("expected name")
	}
	body, err := buildCreateBody("Employee", map[string]string{"name": "QB-CLI-EMP"})
	if err != nil {
		t.Fatal(err)
	}
	if body["DisplayName"] != "QB-CLI-EMP" {
		t.Fatalf("DisplayName=%v", body["DisplayName"])
	}
	if body["GivenName"] != "QB-CLI-EMP" {
		t.Fatalf("GivenName=%v", body["GivenName"])
	}
	if body["FamilyName"] != "CLI" {
		t.Fatalf("FamilyName=%v", body["FamilyName"])
	}
}

func TestBuildCreateTimeActivitySetsEmployeeAndCustomer(t *testing.T) {
	_, err := buildCreateBody("TimeActivity", map[string]string{})
	if err == nil {
		t.Fatal("expected employee required")
	}
	body, err := buildCreateBody("TimeActivity", map[string]string{
		"employee": "10", "hours": "2", "customer": "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["NameOf"] != "Employee" {
		t.Fatalf("NameOf=%v", body["NameOf"])
	}
	if body["EmployeeRef"].(map[string]any)["value"] != "10" {
		t.Fatalf("EmployeeRef=%v", body["EmployeeRef"])
	}
	if body["CustomerRef"].(map[string]any)["value"] != "1" {
		t.Fatalf("CustomerRef=%v", body["CustomerRef"])
	}
}

func TestBuildUpdateBillCopiesVendorRefAndLineAmount(t *testing.T) {
	existing := map[string]any{
		"Id":        "46",
		"SyncToken": "0",
		"VendorRef": map[string]any{"value": "2"},
		"Line": []any{
			map[string]any{"Amount": 0.11, "DetailType": "AccountBasedExpenseLineDetail"},
		},
	}
	body, err := buildUpdateBody("Bill", map[string]string{"amount": "0.12"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["VendorRef"].(map[string]any)["value"] != "2" {
		t.Fatalf("VendorRef=%v", body["VendorRef"])
	}
	if body["TotalAmt"] != 0.12 {
		t.Fatalf("TotalAmt=%v", body["TotalAmt"])
	}
	lines := body["Line"].([]any)
	if lines[0].(map[string]any)["Amount"] != 0.12 {
		t.Fatalf("Line amount=%v", lines[0])
	}
}

func TestBuildUpdateTimeActivitySetsHours(t *testing.T) {
	existing := map[string]any{
		"Id":          "1",
		"SyncToken":   "0",
		"NameOf":      "Employee",
		"EmployeeRef": map[string]any{"value": "10"},
		"Hours":       1,
	}
	body, err := buildUpdateBody("TimeActivity", map[string]string{"hours": "3"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["Hours"] != 3 {
		t.Fatalf("Hours=%v", body["Hours"])
	}
	if body["EmployeeRef"].(map[string]any)["value"] != "10" {
		t.Fatalf("EmployeeRef=%v", body["EmployeeRef"])
	}
}

func TestCopyExistingDoesNotOverwrite(t *testing.T) {
	out := map[string]any{"VendorRef": map[string]any{"value": "9"}}
	existing := map[string]any{"VendorRef": map[string]any{"value": "2"}, "Line": []any{}}
	copyExisting(out, existing, "VendorRef", "Line")
	if out["VendorRef"].(map[string]any)["value"] != "9" {
		t.Fatalf("overwrite VendorRef=%v", out["VendorRef"])
	}
	if out["Line"] == nil {
		t.Fatal("Line not copied")
	}
}

func TestBuildCreateBudgetSetsDates(t *testing.T) {
	body, err := buildCreateBody("Budget", map[string]string{
		"name": "B", "start-date": "01/07/2026", "type": "profit and loss", "amount": "1", "account": "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["StartDate"] != "2026-07-01" || body["EndDate"] != "2027-06-30" || body["BudgetType"] != "ProfitAndLoss" {
		t.Fatalf("%v", body)
	}
	det := body["BudgetDetail"].([]map[string]any)
	if det[0]["BudgetDate"] != "2026-07-01" {
		t.Fatalf("detail=%v", det)
	}
}

func TestNormalizeDateAU(t *testing.T) {
	if got := normalizeDate("18/08/2026"); got != "2026-08-18" {
		t.Fatalf("got %s", got)
	}
}

func TestPlannedMutateURLHasRealmToken(t *testing.T) {
	u := PlannedMutateURL("Invoice", "delete")
	if !containsAll(u, "{realm}", "invoice", "operation=delete") {
		t.Fatalf("%s", u)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsStr(s, p) {
			return false
		}
	}
	return true
}

func containsStr(s, p string) bool {
	return len(s) >= len(p) && (s == p || len(p) == 0 || stringIndex(s, p) >= 0)
}

func stringIndex(s, p string) int {
	for i := 0; i+len(p) <= len(s); i++ {
		if s[i:i+len(p)] == p {
			return i
		}
	}
	return -1
}

func TestBuildCreatePurchaseSplitsBankAndCategory(t *testing.T) {
	_, err := buildCreateBody("Purchase", map[string]string{"account": "48", "category-account": "48", "amount": "0.01"})
	if err == nil {
		t.Fatal("same bank and category must be rejected")
	}
	body, err := buildCreateBody("Purchase", map[string]string{
		"account": "48", "category-account": "7", "amount": "0.01", "payment-type": "credit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["PaymentType"] != "CreditCard" {
		t.Fatalf("type=%v", body["PaymentType"])
	}
	if body["AccountRef"].(map[string]any)["value"] != "48" {
		t.Fatalf("bank=%v", body["AccountRef"])
	}
	lines := body["Line"].([]map[string]any)
	det := lines[0]["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "7" {
		t.Fatalf("line=%v", lines)
	}
	body, err = buildCreateBody("Purchase", map[string]string{
		"account": "48", "amount": "0.07",
	})
	if err != nil {
		t.Fatal(err)
	}
	lines = body["Line"].([]map[string]any)
	det = lines[0]["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "7" {
		t.Fatalf("default category=%v", lines)
	}
}

// TestBuildCreatePurchaseAssetAccountLine asserts --asset-account wins the
// line account over the expense default (gift-certificate/prepaid-voucher
// semantics: cash side on the bank account, line on an Other Current Asset
// account — verified live on TC2, Purchase deleted via form delete).
func TestBuildCreatePurchaseAssetAccountLine(t *testing.T) {
	body, err := buildCreateBody("Purchase", map[string]string{
		"payment-account": "44", "asset-account": "80",
		"supplier": "3", "amount": "50", "date": "19/09/2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["PaymentType"] != "Cash" {
		t.Fatalf("PaymentType=%v", body["PaymentType"])
	}
	if body["AccountRef"].(map[string]any)["value"] != "44" {
		t.Fatalf("cash side=%v", body["AccountRef"])
	}
	if body["EntityRef"].(map[string]any)["value"] != "3" {
		t.Fatalf("supplier=%v", body["EntityRef"])
	}
	lines := body["Line"].([]map[string]any)
	det := lines[0]["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "80" {
		t.Fatalf("asset line=%v", lines)
	}
	if lines[0]["Amount"] != 50.0 {
		t.Fatalf("amount=%v", lines[0]["Amount"])
	}
	if body["TxnDate"] != "2026-09-19" {
		t.Fatalf("date=%v", body["TxnDate"])
	}
	// asset-account == payment-account must hit the same-account guard.
	if _, err := buildCreateBody("Purchase", map[string]string{
		"payment-account": "44", "asset-account": "44", "amount": "50",
	}); err == nil {
		t.Fatal("identical payment/asset accounts must be rejected")
	}
	// asset-account keeps precedence when both are given (the more
	// specific asset/prepaid intent wins over the generic category flag).
	body, err = buildCreateBody("Purchase", map[string]string{
		"payment-account": "44", "asset-account": "80",
		"category-account": "7", "amount": "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	lines = body["Line"].([]map[string]any)
	det = lines[0]["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "80" {
		t.Fatalf("asset precedence=%v", lines)
	}
}

// TestBuildCreateDelayedChargeHasCustomerRef asserts DelayedCharge create
// produces the same shape as Invoice: CustomerRef + sales Lines. It rejects:
//   - missing CustomerRef (would fail QBO validation),
//   - missing --customer returning nil error (silent bad input),
//   - missing Line array (malformed body).
func TestBuildCreateDelayedChargeHasCustomerRef(t *testing.T) {
	_, err := buildCreateBody("DelayedCharge", map[string]string{"amount": "1"})
	if err == nil {
		t.Fatal("expected customer required for delayed charge")
	}
	body, err := buildCreateBody("DelayedCharge", map[string]string{"customer": "1", "amount": "2.5", "item": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if body["CustomerRef"].(map[string]any)["value"] != "1" {
		t.Fatalf("CustomerRef=%v", body["CustomerRef"])
	}
	lines := body["Line"].([]map[string]any)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	det := lines[0]["SalesItemLineDetail"].(map[string]any)
	if det["ItemRef"].(map[string]any)["value"] != "3" {
		t.Fatalf("ItemRef=%v", det["ItemRef"])
	}
}

// TestBuildCreateDelayedCreditHasCustomerRef mirrors the delayed-charge test
// for DelayedCredit. It rejects missing --customer and verifies CustomerRef +
// sales Lines are present.
func TestBuildCreateDelayedCreditHasCustomerRef(t *testing.T) {
	_, err := buildCreateBody("DelayedCredit", map[string]string{"item": "1"})
	if err == nil {
		t.Fatal("expected customer required for delayed credit")
	}
	body, err := buildCreateBody("DelayedCredit", map[string]string{"customer": "5", "amount": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if body["CustomerRef"].(map[string]any)["value"] != "5" {
		t.Fatalf("CustomerRef=%v", body["CustomerRef"])
	}
	if body["Line"] == nil {
		t.Fatal("missing Line")
	}
}

// TestBuildCreateBillPaymentRejectsMissingBillID asserts BillPayment create
// requires --bill-id (or --id) to link the payment to a Bill. It rejects:
//   - missing --bill-id and --id returning nil error (would create an
//     unlinked payment that QBO rejects),
//   - missing --supplier returning nil error,
//   - missing --payment-account returning nil error.
func TestBuildCreateBillPaymentRejectsMissingBillID(t *testing.T) {
	_, err := buildCreateBody("BillPayment", map[string]string{"supplier": "2", "account": "48"})
	if err == nil {
		t.Fatal("expected bill-id required")
	}
	_, err = buildCreateBody("BillPayment", map[string]string{"bill-id": "10", "payment-account": "48"})
	if err == nil {
		t.Fatal("expected supplier required")
	}
	_, err = buildCreateBody("BillPayment", map[string]string{"supplier": "2", "bill-id": "10"})
	if err == nil {
		t.Fatal("expected account required")
	}
	body, err := buildCreateBody("BillPayment", map[string]string{
		"supplier": "2", "bill-id": "10", "account": "48", "amount": "0.01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["VendorRef"].(map[string]any)["value"] != "2" {
		t.Fatalf("VendorRef=%v", body["VendorRef"])
	}
	if body["PayType"] != "Check" {
		t.Fatalf("PayType=%v", body["PayType"])
	}
	lines := body["Line"].([]map[string]any)
	linked := lines[0]["LinkedTxn"].([]map[string]any)
	if linked[0]["TxnId"] != "10" || linked[0]["TxnType"] != "Bill" {
		t.Fatalf("LinkedTxn=%v", linked)
	}
}

// TestBuildCreateBillPaymentCreditCard asserts that --pay-type credit
// produces PayType=CreditCard and CreditCardPayment with CCAccountRef instead
// of CheckPayment. It rejects a regression that left CheckPayment on a
// credit-card payment.
func TestBuildCreateBillPaymentCreditCard(t *testing.T) {
	body, err := buildCreateBody("BillPayment", map[string]string{
		"supplier": "2", "bill-id": "10", "account": "49", "pay-type": "credit", "amount": "0.01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["PayType"] != "CreditCard" {
		t.Fatalf("PayType=%v", body["PayType"])
	}
	if body["CheckPayment"] != nil {
		t.Fatal("CheckPayment should be absent for credit-card payment")
	}
	cc := body["CreditCardPayment"].(map[string]any)
	if cc["CCAccountRef"].(map[string]any)["value"] != "49" {
		t.Fatalf("CCAccountRef=%v", cc["CCAccountRef"])
	}
}

// TestStripForCopyRemovesServerFields asserts stripForCopy removes Id,
// SyncToken, MetaData, and DocNumber from a fetched entity while preserving
// CustomerRef and Line. It rejects:
//   - leaving Id or SyncToken (would update instead of create),
//   - dropping CustomerRef or Line (would lose the clone's content),
//   - leaving LinkedTxn on lines (would link clone to original).
func TestStripForCopyRemovesServerFields(t *testing.T) {
	existing := map[string]any{
		"Id":          "123",
		"SyncToken":   "2",
		"MetaData":    map[string]any{"CreateTime": "2026-01-01"},
		"DocNumber":   "INV-100",
		"CustomerRef": map[string]any{"value": "1"},
		"Line": []map[string]any{
			{"Amount": 1.5, "LinkedTxn": []map[string]any{{"TxnId": "99", "TxnType": "Payment"}}},
		},
	}
	out := stripForCopy(existing)
	if out["Id"] != nil {
		t.Fatal("Id should be stripped")
	}
	if out["SyncToken"] != nil {
		t.Fatal("SyncToken should be stripped")
	}
	if out["MetaData"] != nil {
		t.Fatal("MetaData should be stripped")
	}
	if out["DocNumber"] != nil {
		t.Fatal("DocNumber should be stripped")
	}
	if out["CustomerRef"] == nil {
		t.Fatal("CustomerRef should be preserved")
	}
	lines := out["Line"].([]map[string]any)
	if lines[0]["LinkedTxn"] != nil {
		t.Fatal("LinkedTxn should be stripped from lines")
	}
	if lines[0]["Amount"] != 1.5 {
		t.Fatalf("Amount should be preserved, got %v", lines[0]["Amount"])
	}
}

// TestStripForCopyRejectsEmptyInput asserts stripForCopy on an empty or
// Id-only map returns a map without Id. This is the negative case: a
// regression that copied Id through would silently update instead of create.
func TestStripForCopyRejectsEmptyInput(t *testing.T) {
	out := stripForCopy(map[string]any{"Id": "999", "SyncToken": "0"})
	if out["Id"] != nil {
		t.Fatal("Id should be stripped even from minimal input")
	}
	if out["SyncToken"] != nil {
		t.Fatal("SyncToken should be stripped even from minimal input")
	}
	if len(out) != 0 {
		t.Fatalf("expected empty map, got %v", out)
	}
}

// TestPlannedMutateURLSendHasSendPath asserts the send op produces a URL
// with the /{id}/send path segment. It rejects a regression that used the
// generic create/update URL for send.
func TestPlannedMutateURLSendHasSendPath(t *testing.T) {
	u := PlannedMutateURL("Invoice", "send")
	if !containsAll(u, "{realm}", "invoice", "{id}", "send") {
		t.Fatalf("%s", u)
	}
}

// TestBuildCreatePurchaseCreditCardCredit asserts that the Purchase entity
// with --payment-type credit produces a CreditCard PaymentType, which is the
// shape CREDIT_CARD_CREDIT_CREATE uses. It rejects a regression that
// defaulted to Cash or dropped the AccountRef.
func TestBuildCreatePurchaseCreditCardCredit(t *testing.T) {
	body, err := buildCreateBody("Purchase", map[string]string{
		"account": "49", "category-account": "7", "amount": "0.01", "payment-type": "credit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["PaymentType"] != "CreditCard" {
		t.Fatalf("PaymentType=%v", body["PaymentType"])
	}
	if body["AccountRef"].(map[string]any)["value"] != "49" {
		t.Fatalf("AccountRef=%v", body["AccountRef"])
	}
	lines := body["Line"].([]map[string]any)
	det := lines[0]["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "7" {
		t.Fatalf("line account=%v", det["AccountRef"])
	}
}

func TestBuildCreateItemReceiptRequiresSupplier(t *testing.T) {
	_, err := buildCreateBody("ItemReceipt", map[string]string{"amount": "1", "account": "7"})
	if err == nil {
		t.Fatal("expected supplier required")
	}
	body, err := buildCreateBody("ItemReceipt", map[string]string{
		"supplier": "2", "amount": "1", "account": "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["VendorRef"].(map[string]any)["value"] != "2" {
		t.Fatalf("VendorRef=%v", body["VendorRef"])
	}
	if body["Line"] == nil {
		t.Fatal("missing Line")
	}
	if _, ok := v3Path["ItemReceipt"]; !ok {
		t.Fatal("ItemReceipt missing from v3Path")
	}
}

func TestBuildCreateInventoryAdjustmentRequiresItemAndAccount(t *testing.T) {
	_, err := buildCreateBody("InventoryAdjustment", map[string]string{"amount": "1"})
	if err == nil {
		t.Fatal("expected item and account")
	}
	body, err := buildCreateBody("InventoryAdjustment", map[string]string{
		"item": "3", "account": "81", "new-qty": "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["AdjustAccountRef"].(map[string]any)["value"] != "81" {
		t.Fatalf("AdjustAccountRef=%v", body["AdjustAccountRef"])
	}
	line := body["Line"].([]map[string]any)[0]
	det := line["ItemAdjustmentLineDetail"].(map[string]any)
	if det["ItemRef"].(map[string]any)["value"] != "3" {
		t.Fatalf("ItemRef=%v", det["ItemRef"])
	}
	if det["QtyDiff"] != 2.0 {
		t.Fatalf("QtyDiff=%v", det["QtyDiff"])
	}
	if body["DocNumber"] == "" {
		t.Fatal("DocNumber empty")
	}
}

func TestBuildCreateInvoiceSetsBillEmail(t *testing.T) {
	body, err := buildCreateBody("Invoice", map[string]string{
		"customer": "1", "amount": "1", "item": "3", "email": "nobody@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["BillEmail"].(map[string]any)["Address"] != "nobody@example.com" {
		t.Fatalf("BillEmail=%v", body["BillEmail"])
	}
}

func TestBuildCreateInventoryItemSetsQty(t *testing.T) {
	body, err := buildCreateBody("Item", map[string]string{
		"name": "QB-CLI-INV", "type": "Inventory",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["Type"] != "Inventory" {
		t.Fatalf("Type=%v", body["Type"])
	}
	if body["TrackQtyOnHand"] != true {
		t.Fatalf("TrackQtyOnHand=%v", body["TrackQtyOnHand"])
	}
	if body["AssetAccountRef"].(map[string]any)["value"] != "42" {
		t.Fatalf("AssetAccountRef=%v", body["AssetAccountRef"])
	}
}

func TestBuildCreateAccountSetsSubtype(t *testing.T) {
	body, err := buildCreateBody("Account", map[string]string{
		"name": "QB-CLI-COGS", "type": "Cost of Goods Sold", "subtype": "SuppliesMaterialsCogs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["AccountType"] != "Cost of Goods Sold" {
		t.Fatalf("AccountType=%v", body["AccountType"])
	}
	if body["AccountSubType"] != "SuppliesMaterialsCogs" {
		t.Fatalf("AccountSubType=%v", body["AccountSubType"])
	}
}

func TestBuildCreateCustomerSetsBillAddr(t *testing.T) {
	body, err := buildCreateBody("Customer", map[string]string{"name": "QB-CLI-ADDR"})
	if err != nil {
		t.Fatal(err)
	}
	addr, ok := body["BillAddr"].(map[string]any)
	if !ok {
		t.Fatalf("missing BillAddr: %v", body)
	}
	if addr["Line1"] != "1 Test Street" || addr["City"] != "Sydney" || addr["Country"] != "AU" {
		t.Fatalf("BillAddr=%v", addr)
	}
}

func TestBuildCreateInvoiceSetsBillAddrAndDiscount(t *testing.T) {
	body, err := buildCreateBody("Invoice", map[string]string{
		"customer": "1", "amount": "10", "item": "3", "discount": "1.5", "email": "a@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	addr, ok := body["BillAddr"].(map[string]any)
	if !ok || addr["Line1"] != "1 Test Street" {
		t.Fatalf("BillAddr=%v", body["BillAddr"])
	}
	if body["BillEmail"].(map[string]any)["Address"] != "a@example.com" {
		t.Fatalf("BillEmail=%v", body["BillEmail"])
	}
	lines := body["Line"].([]map[string]any)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[1]["DetailType"] != "DiscountLineDetail" {
		t.Fatalf("discount line=%v", lines[1])
	}
}

func TestBuildCreateCreditMemoLinksInvoice(t *testing.T) {
	body, err := buildCreateBody("CreditMemo", map[string]string{
		"customer": "1", "amount": "10", "item": "3", "invoice-id": "88",
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := body["Line"].([]map[string]any)
	linked := lines[0]["LinkedTxn"].([]map[string]any)
	if linked[0]["TxnId"] != "88" || linked[0]["TxnType"] != "Invoice" {
		t.Fatalf("LinkedTxn=%v", linked)
	}
}

func TestBuildCreateInvoiceLinksTimeActivity(t *testing.T) {
	body, err := buildCreateBody("Invoice", map[string]string{
		"customer": "1", "amount": "2", "item": "3", "time-activity": "12",
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := body["Line"].([]map[string]any)
	linked := lines[0]["LinkedTxn"].([]map[string]any)
	if linked[0]["TxnId"] != "12" || linked[0]["TxnType"] != "TimeActivity" {
		t.Fatalf("LinkedTxn=%v", linked)
	}
}

func TestFirstItemRefFromInvoiceLines(t *testing.T) {
	got := firstItemRef(map[string]any{
		"Line": []any{
			map[string]any{"DetailType": "SubTotalLineDetail"},
			map[string]any{
				"DetailType": "SalesItemLineDetail",
				"SalesItemLineDetail": map[string]any{
					"ItemRef": map[string]any{"value": "3"},
				},
			},
		},
	})
	if got != "3" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildUpdateInvoiceAppendsDiscount(t *testing.T) {
	existing := map[string]any{
		"Id": "9", "SyncToken": "0",
		"Line": []any{map[string]any{"Amount": 10.0, "DetailType": "SalesItemLineDetail"}},
	}
	body, err := buildUpdateBody("Invoice", map[string]string{"discount": "1.5"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	lines, ok := body["Line"].([]map[string]any)
	if !ok || len(lines) != 2 {
		t.Fatalf("Line=%T %v", body["Line"], body["Line"])
	}
	if lines[1]["DetailType"] != "DiscountLineDetail" {
		t.Fatalf("discount=%v", lines[1])
	}
}

func TestBuildCreateVendorSetsTPAR(t *testing.T) {
	body, err := buildCreateBody("Vendor", map[string]string{"name": "QB-CLI-TPAR-SUP", "tpar": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if body["Vendor1099"] != true {
		t.Fatalf("Vendor1099=%v", body["Vendor1099"])
	}
	body2, err := buildCreateBody("Vendor", map[string]string{"name": "QB-CLI-SUP"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := body2["Vendor1099"]; ok {
		t.Fatalf("Vendor1099 set without --tpar: %v", body2["Vendor1099"])
	}
}

func TestBuildUpdateVendorSetsTPAR(t *testing.T) {
	existing := map[string]any{"Id": "5", "SyncToken": "0"}
	body, err := buildUpdateBody("Vendor", map[string]string{"tpar": "true"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["Vendor1099"] != true {
		t.Fatalf("Vendor1099=%v", body["Vendor1099"])
	}
}

func TestBuildUpdateTimeActivityCarriesVendorRef(t *testing.T) {
	existing := map[string]any{
		"Id":        "1",
		"SyncToken": "0",
		"NameOf":    "Vendor",
		"VendorRef": map[string]any{"value": "3"},
		"Hours":     2,
	}
	body, err := buildUpdateBody("TimeActivity", map[string]string{"hours": "3"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["VendorRef"].(map[string]any)["value"] != "3" {
		t.Fatalf("VendorRef dropped from update: %v", body["VendorRef"])
	}
}

func TestBuildUpdateBillPaymentCarriesLines(t *testing.T) {
	existing := map[string]any{
		"Id": "81", "SyncToken": "0",
		"VendorRef": map[string]any{"value": "3"},
		"Line":      []any{map[string]any{"Amount": 11.0}},
		"TotalAmt":  11.0,
	}
	body, err := buildUpdateBody("BillPayment", map[string]string{"amount": "5"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["TotalAmt"] != 5.0 {
		t.Fatalf("TotalAmt=%v, want 5", body["TotalAmt"])
	}
}

// writeoffServer serves the three calls a write-off makes: GET the invoice,
// POST the credit memo, POST the $0 apply-payment. It records each request.
type writeoffServer struct {
	lastPaymentBody []byte
	calls           []string
}

func (s *writeoffServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/company/12345/invoice/125", func(w http.ResponseWriter, r *http.Request) {
		s.calls = append(s.calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Invoice":{"Id":"125","SyncToken":"0","Balance":10.0,"TotalAmt":10.0,"CustomerRef":{"value":"1"},"Line":[{"Amount":10.0,"DetailType":"SalesItemLineDetail","SalesItemLineDetail":{"ItemRef":{"value":"1"}}}]}}`))
	})
	mux.HandleFunc("/api/v3/company/12345/creditmemo", func(w http.ResponseWriter, r *http.Request) {
		s.calls = append(s.calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"CreditMemo":{"Id":"129","TotalAmt":10.0,"Balance":10.0,"CustomerRef":{"value":"1"}}}`))
	})
	mux.HandleFunc("/api/v3/company/12345/payment", func(w http.ResponseWriter, r *http.Request) {
		s.calls = append(s.calls, r.Method+" "+r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		s.lastPaymentBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Payment":{"Id":"130","TotalAmt":0}}`))
	})
	return mux
}

// A write-off must not stop at creating the credit memo: the invoice only
// settles once a $0 payment links the invoice to the new credit memo. This
// regresses the live bug where the credit memo was created unapplied and the
// invoice balance stayed open.
func TestWriteoffAppliesCreditMemoToInvoice(t *testing.T) {
	saveUsable(t)
	s := &writeoffServer{}
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Invoice", "writeoff", "125", map[string]string{})
	if err != nil {
		t.Fatalf("ReplayMutate writeoff: %v", err)
	}
	if res.Item.ID != "129" {
		t.Fatalf("credit memo id = %q, want 129", res.Item.ID)
	}
	var seenInvoiceGet, seenPayment bool
	for _, c := range s.calls {
		if c == "GET /api/v3/company/12345/invoice/125" {
			seenInvoiceGet = true
		}
		if c == "POST /api/v3/company/12345/payment" {
			seenPayment = true
		}
	}
	if !seenInvoiceGet || !seenPayment {
		t.Fatalf("calls = %v, want invoice GET + payment POST", s.calls)
	}
	var pay map[string]any
	if err := json.Unmarshal(s.lastPaymentBody, &pay); err != nil {
		t.Fatalf("payment body not JSON: %v", err)
	}
	if total, _ := pay["TotalAmt"].(float64); total != 0 {
		t.Fatalf("apply payment TotalAmt = %v, want 0", pay["TotalAmt"])
	}
	lines, _ := pay["Line"].([]any)
	if len(lines) != 2 {
		t.Fatalf("apply payment lines = %v, want 2", pay["Line"])
	}
	want := map[string]string{"125": "Invoice", "129": "CreditMemo"}
	for _, l := range lines {
		linked, _ := l.(map[string]any)["LinkedTxn"].([]any)
		if len(linked) != 1 {
			t.Fatalf("line missing LinkedTxn: %v", l)
		}
		lt := linked[0].(map[string]any)
		id, _ := lt["TxnId"].(string)
		wantType, ok := want[id]
		if !ok || lt["TxnType"] != wantType {
			t.Fatalf("unexpected LinkedTxn %v", lt)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("payment missing links for %v", want)
	}
}
