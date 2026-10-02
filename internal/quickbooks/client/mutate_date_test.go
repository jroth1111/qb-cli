package client

import (
	"strings"
	"testing"
)

func TestPublishedTransactionDatesReachAPI(t *testing.T) {
	for _, tc := range []struct{ entity, flag string }{{"Invoice", "invoice-date"}, {"Bill", "bill-date"}, {"Estimate", "quote-date"}, {"Purchase", "purchase-date"}, {"SalesReceipt", "txn-date"}} {
		flags := map[string]string{tc.flag: "10/09/2026", "customer": "1", "supplier": "2", "account": "3", "amount": "1", "due-date": "20/09/2026"}
		body, err := buildCreateBody(tc.entity, flags)
		if err != nil {
			t.Fatal(err)
		}
		if body["TxnDate"] != "2026-09-10" {
			t.Errorf("%s ignored --%s: %v", tc.entity, tc.flag, body["TxnDate"])
		}
		if (tc.entity == "Invoice" || tc.entity == "Bill") && body["DueDate"] != "2026-09-20" {
			t.Error("due date ignored")
		}
		updated, err := buildUpdateBody(tc.entity, flags, map[string]any{"Id": "7", "SyncToken": "0"})
		if err != nil || updated["TxnDate"] != "2026-09-10" {
			t.Errorf("update date ignored: %v %v", updated, err)
		}
	}
}

// expense create declares --payee (catalog name), --txn-date and --memo;
// the builder must land all three — a dropped payee posts a payee-less
// Purchase, and a missing date/memo forces prior-period fixes into the UI.
func TestExpenseCreateCarriesPayeeDateMemo(t *testing.T) {
	body, err := buildCreateBody("Purchase", map[string]string{
		"payee": "193", "payment-account": "204", "account": "203",
		"amount": "38.70", "txn-date": "21/05/2019",
		"memo":         "TPG acct 5978391 — U203 — rcpt R155881648",
		"payment-type": "CreditCard",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["EntityRef"].(map[string]any)["value"] != "193" {
		t.Error("--payee dropped; expense would post with no payee")
	}
	if body["TxnDate"] != "2019-05-21" {
		t.Errorf("--txn-date ignored: %v", body["TxnDate"])
	}
	if body["PrivateNote"] != "TPG acct 5978391 — U203 — rcpt R155881648" {
		t.Errorf("--memo ignored: %v", body["PrivateNote"])
	}
	if body["PaymentType"] != "CreditCard" {
		t.Errorf("--payment-type ignored: %v", body["PaymentType"])
	}
}

func TestBillEditCarriesMemoAndDate(t *testing.T) {
	existing := map[string]any{"Id": "6927", "SyncToken": "0",
		"VendorRef": map[string]any{"value": "193"},
		"Line":      []any{map[string]any{"Amount": 299.95}}}
	body, err := buildUpdateBody("Bill", map[string]string{
		"txn-date": "15/03/2020", "memo": "TPG consolidated", "due-date": "29/03/2020",
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["PrivateNote"] != "TPG consolidated" || body["TxnDate"] != "2020-03-15" || body["DueDate"] != "2020-03-29" {
		t.Errorf("bill edit fields dropped: %v", body)
	}
}

func TestExpensePaymentAccountWinsOverCategoryAlias(t *testing.T) {
	body, err := buildCreateBody("Purchase", map[string]string{"payment-account": "10", "account": "20", "amount": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if body["AccountRef"].(map[string]any)["value"] != "10" {
		t.Fatal("payment account alias ignored")
	}
	line := body["Line"].([]map[string]any)[0]
	if line["AccountBasedExpenseLineDetail"].(map[string]any)["AccountRef"].(map[string]any)["value"] != "20" {
		t.Fatal("category alias ignored")
	}
}

func TestPaymentAmountUpdateKeepsAllocationConsistent(t *testing.T) {
	existing := map[string]any{"Id": "9", "SyncToken": "0", "CustomerRef": map[string]any{"value": "1"}, "Line": []any{map[string]any{"Amount": 1.0, "LinkedTxn": []any{map[string]any{"TxnId": "2", "TxnType": "Invoice"}}}}}
	body, err := buildUpdateBody("Payment", map[string]string{"amount": "0.5"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["TotalAmt"] != 0.5 || body["Line"].([]any)[0].(map[string]any)["Amount"] != 0.5 {
		t.Fatal("payment total and allocation differ")
	}
	existing["Line"] = append(existing["Line"].([]any), map[string]any{"Amount": 1.0})
	if _, err := buildUpdateBody("Payment", map[string]string{"amount": "0.5"}, existing); err == nil {
		t.Fatal("ambiguous multi-allocation update accepted")
	}
}

func TestPaymentDepositToAliasReachesAPI(t *testing.T) {
	body, err := buildCreateBody("Payment", map[string]string{"customer": "1", "amount": "1", "deposit-to": "67", "invoice-id": "82"})
	if err != nil {
		t.Fatal(err)
	}
	if body["DepositToAccountRef"].(map[string]any)["value"] != "67" {
		t.Fatal("deposit-to account ignored")
	}
}

func TestJournalAmountUpdatesBothSides(t *testing.T) {
	line := func(kind string) any {
		return map[string]any{"Amount": 1.0, "JournalEntryLineDetail": map[string]any{"PostingType": kind}}
	}
	existing := map[string]any{"Id": "1", "SyncToken": "0", "Line": []any{line("Debit"), line("Credit")}}
	body, err := buildUpdateBody("JournalEntry", map[string]string{"amount": "2"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range body["Line"].([]any) {
		if l.(map[string]any)["Amount"] != 2.0 {
			t.Fatal("journal became unbalanced")
		}
	}
	for _, l := range existing["Line"].([]any) {
		if l.(map[string]any)["Amount"] != 1.0 {
			t.Fatal("input lines mutated")
		}
	}
}

func TestTimeActivityDoesNotInventRate(t *testing.T) {
	for _, rate := range []string{"", "0"} {
		body, err := buildCreateBody("TimeActivity", map[string]string{"employee": "1", "hours": "1", "rate": rate, "billable": "false"})
		if err != nil {
			t.Fatal(err)
		}
		if body["HourlyRate"] != 0.0 || body["BillableStatus"] != "NotBillable" {
			t.Fatal("zero-rate non-billable entry changed")
		}
	}
}

// --- update-path dead flags wired -----------------------------------------

// cheque edit declares --payee; the Purchase update path must move EntityRef
// while preserving the stored entity type (Customer/Employee/Vendor).
func TestPurchaseEditMovesPayee(t *testing.T) {
	existing := map[string]any{"Id": "5", "SyncToken": "0",
		"AccountRef": map[string]any{"value": "204"}, "PaymentType": "Check",
		"EntityRef": map[string]any{"value": "1", "type": "Vendor"},
		"Line":      []any{map[string]any{"Amount": 10.0}}}
	body, err := buildUpdateBody("Purchase", map[string]string{"payee": "77"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	ref := body["EntityRef"].(map[string]any)
	if ref["value"] != "77" || ref["type"] != "Vendor" {
		t.Fatalf("payee move lost the entity type: %v", ref)
	}
	if existing["EntityRef"].(map[string]any)["value"] != "1" {
		t.Fatal("update mutated the existing document")
	}
	// payee-less document gains a Vendor-typed ref
	noPayee := map[string]any{"Id": "5", "SyncToken": "0", "Line": []any{map[string]any{"Amount": 1.0}}}
	body, err = buildUpdateBody("Purchase", map[string]string{"payee": "77"}, noPayee)
	if err != nil {
		t.Fatal(err)
	}
	if body["EntityRef"].(map[string]any)["type"] != "Vendor" {
		t.Fatal("new payee must default to Vendor type")
	}
}

// bill edit declares --supplier; the value must move VendorRef.
func TestBillEditMovesSupplier(t *testing.T) {
	existing := map[string]any{"Id": "9", "SyncToken": "0",
		"VendorRef": map[string]any{"value": "193"},
		"Line":      []any{map[string]any{"Amount": 1.0}}}
	for _, flag := range []string{"supplier", "vendor"} {
		body, err := buildUpdateBody("Bill", map[string]string{flag: "42"}, existing)
		if err != nil {
			t.Fatalf("--%s: %v", flag, err)
		}
		if body["VendorRef"].(map[string]any)["value"] != "42" {
			t.Fatalf("--%s dropped: %v", flag, body["VendorRef"])
		}
	}
}

// transfer edit declares --from-account/--to-account; they were copied
// but never applied, so an account move posted nothing.
func TestTransferEditMovesAccounts(t *testing.T) {
	existing := map[string]any{"Id": "3", "SyncToken": "0",
		"FromAccountRef": map[string]any{"value": "93"},
		"ToAccountRef":   map[string]any{"value": "209"},
		"Amount":         61.86}
	body, err := buildUpdateBody("Transfer", map[string]string{"from-account": "4", "to-account": "12"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["FromAccountRef"].(map[string]any)["value"] != "4" || body["ToAccountRef"].(map[string]any)["value"] != "12" {
		t.Fatalf("transfer account move dropped: %v", body)
	}
}

// deposit edit: --account moves the deposit target; --from-account retargets
// a single line's source; multi-line deposits must refuse ambiguous moves.
func TestDepositEditMovesTargetAndSource(t *testing.T) {
	line := map[string]any{"Amount": 5.0, "DepositLineDetail": map[string]any{"AccountRef": map[string]any{"value": "50"}}}
	existing := map[string]any{"Id": "7", "SyncToken": "0",
		"DepositToAccountRef": map[string]any{"value": "93"},
		"Line":                []any{line}}
	body, err := buildUpdateBody("Deposit", map[string]string{"account": "4", "from-account": "60"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["DepositToAccountRef"].(map[string]any)["value"] != "4" {
		t.Fatal("--account did not move DepositToAccountRef")
	}
	got := body["Line"].([]any)[0].(map[string]any)["DepositLineDetail"].(map[string]any)["AccountRef"].(map[string]any)["value"]
	if got != "60" {
		t.Fatal("--from-account did not retarget the source account")
	}
	// multi-line deposit + --from-account must error
	multi := map[string]any{"Id": "7", "SyncToken": "0", "Line": []any{line, line}}
	if _, err := buildUpdateBody("Deposit", map[string]string{"from-account": "60"}, multi); err == nil {
		t.Fatal("ambiguous multi-line deposit account move accepted")
	}
}

// journal edit declares --from-account/--to-account; they must land on the
// debit/credit halves respectively.
func TestJournalEditMovesPostingAccounts(t *testing.T) {
	line := func(kind string) any {
		return map[string]any{"Amount": 1.0, "JournalEntryLineDetail": map[string]any{"PostingType": kind, "AccountRef": map[string]any{"value": "1"}}}
	}
	existing := map[string]any{"Id": "1", "SyncToken": "0", "Line": []any{line("Debit"), line("Credit")}}
	body, err := buildUpdateBody("JournalEntry", map[string]string{"from-account": "10", "to-account": "20"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range body["Line"].([]any) {
		d := l.(map[string]any)["JournalEntryLineDetail"].(map[string]any)
		switch d["PostingType"] {
		case "Debit":
			if d["AccountRef"].(map[string]any)["value"] != "10" {
				t.Fatal("debit side did not take --from-account")
			}
		case "Credit":
			if d["AccountRef"].(map[string]any)["value"] != "20" {
				t.Fatal("credit side did not take --to-account")
			}
		}
	}
	// existing must be untouched (deep copy)
	if existing["Line"].([]any)[0].(map[string]any)["JournalEntryLineDetail"].(map[string]any)["AccountRef"].(map[string]any)["value"] != "1" {
		t.Fatal("journal edit mutated the existing document")
	}
}

func TestJournalEditAmountPreservesLineEdits(t *testing.T) {
	existing := map[string]any{"Id": "1", "SyncToken": "0", "TxnDate": "2026-08-21", "TotalAmt": 0.0, "Line": []any{
		map[string]any{"Id": "0", "Amount": 37.5, "Description": "old", "JournalEntryLineDetail": map[string]any{"PostingType": "Debit", "AccountRef": map[string]any{"value": "76"}, "ClassRef": map[string]any{"value": "unit-14"}, "Entity": map[string]any{"Type": "Vendor", "EntityRef": map[string]any{"value": "193"}}}},
		map[string]any{"Id": "1", "Amount": 37.5, "JournalEntryLineDetail": map[string]any{"PostingType": "Credit", "AccountRef": map[string]any{"value": "166"}, "ClassRef": map[string]any{"value": "unit-14"}}},
	}}
	for _, explicit := range []bool{false, true} {
		flags := map[string]string{"amount": "50", "from-account": "78", "to-account": "166"}
		if explicit {
			flags["line-items"] = `[{"Id":"0","Amount":37.5,"Description":"new","JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"78"},"ClassRef":{"value":"unit-14"},"Entity":null}},{"Id":"1","Amount":37.5,"Description":"new","JournalEntryLineDetail":{"PostingType":"Credit","AccountRef":{"value":"166"},"ClassRef":{"value":"unit-14"}}}]`
		}
		body, err := buildUpdateBody("JournalEntry", flags, existing)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := body["TotalAmt"]; ok {
			t.Fatal("journal update submitted the derived TotalAmt")
		}
		for _, value := range body["Line"].([]any) {
			line := value.(map[string]any)
			detail := line["JournalEntryLineDetail"].(map[string]any)
			want := "166"
			if detail["PostingType"] == "Debit" {
				want = "78"
			}
			if line["Amount"] != 50.0 || detail["AccountRef"].(map[string]any)["value"] != want || detail["ClassRef"].(map[string]any)["value"] != "unit-14" {
				t.Fatalf("explicit=%v amount discarded line edit: %+v", explicit, line)
			}
			if explicit && (line["Description"] != "new" || detail["PostingType"] == "Debit" && detail["Entity"] != nil) {
				t.Fatalf("explicit line description/entity clear lost: %+v", line)
			}
		}
	}
	old := existing["Line"].([]any)[0].(map[string]any)
	if old["Amount"] != 37.5 || old["JournalEntryLineDetail"].(map[string]any)["AccountRef"].(map[string]any)["value"] != "76" {
		t.Fatal("journal edit mutated pre-state")
	}
}

// journal create declared --items-json which was never consumed; the alias
// must work and malformed input must error instead of falling through.
func TestJournalCreateConsumesItemsJSONAndRejectsMalformed(t *testing.T) {
	body, err := buildCreateBody("JournalEntry", map[string]string{
		"items-json": `[{"Amount":5,"JournalEntryLineDetail":{"PostingType":"Debit","AccountRef":{"value":"1"}}}]`,
		"date":       "01/03/2020",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(body["Line"].([]map[string]any)) != 1 {
		t.Fatal("--items-json lines dropped")
	}
	if _, err := buildCreateBody("JournalEntry", map[string]string{"items-json": "not-json"}); err == nil {
		t.Fatal("malformed --items-json fell through to a misleading error")
	}
	if _, err := buildCreateBody("JournalEntry", map[string]string{"items-json": "[]"}); err == nil {
		t.Fatal("empty --items-json accepted")
	}
}

// --- declared-but-dead create fields wired --------------------------------

// item create declared sku/prices/supplier/reorder-point which were never
// written to the Item body.
func TestItemCreateCarriesDeclaredFields(t *testing.T) {
	body, err := buildCreateBody("Item", map[string]string{
		"name": "Cleaning", "type": "Service", "income-account": "50",
		"sku": "CLN-1", "sales-price": "45.00", "purchase-cost": "30.50",
		"preferred-supplier": "19", "reorder-point": "4", "description": "Unit clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["Sku"] != "CLN-1" || body["UnitPrice"] != 45.0 || body["PurchaseCost"] != 30.5 {
		t.Fatalf("item fields dropped: %v", body)
	}
	if body["PrefVendorRef"].(map[string]any)["value"] != "19" || body["ReorderPoint"] != 4.0 {
		t.Fatalf("supplier/reorder-point dropped: %v", body)
	}
	if _, err := buildCreateBody("Item", map[string]string{"name": "x", "sales-price": "abc"}); err == nil {
		t.Fatal("non-numeric --sales-price accepted")
	}
}

// inventory item create: initial-qty + inventory-asset-account were declared
// but never read.
func TestInventoryItemCreateCarriesQtyAndAssetAccount(t *testing.T) {
	body, err := buildCreateBody("Item", map[string]string{
		"name": "Stock", "type": "Inventory", "income-account": "50",
		"initial-qty": "12", "inventory-asset-account": "61",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["QtyOnHand"] != 12.0 {
		t.Fatalf("--initial-qty dropped: %v", body["QtyOnHand"])
	}
	if body["AssetAccountRef"].(map[string]any)["value"] != "61" {
		t.Fatalf("--inventory-asset-account dropped: %v", body["AssetAccountRef"])
	}
}

// item update edit declared starting-value/reorder-point/preferred-supplier
// — all dead until now.
func TestItemEditCarriesDeclaredFields(t *testing.T) {
	existing := map[string]any{"Id": "4", "SyncToken": "0", "Type": "Inventory",
		"IncomeAccountRef": map[string]any{"value": "50"}}
	body, err := buildUpdateBody("Item", map[string]string{
		"starting-value": "20", "reorder-point": "5", "preferred-supplier": "19",
		"sales-price": "99.95", "sku": "S-1", "income-account": "51",
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["QtyOnHand"] != 20.0 || body["TrackQtyOnHand"] != true {
		t.Fatalf("--starting-value dropped: %v", body)
	}
	if body["ReorderPoint"] != 5.0 || body["PrefVendorRef"].(map[string]any)["value"] != "19" {
		t.Fatalf("reorder/preferred-supplier dropped: %v", body)
	}
	if body["UnitPrice"] != 99.95 || body["Sku"] != "S-1" {
		t.Fatalf("price/sku dropped: %v", body)
	}
	if body["IncomeAccountRef"].(map[string]any)["value"] != "51" {
		t.Fatalf("--income-account dropped: %v", body["IncomeAccountRef"])
	}
}

// supplier create declared address/supplier-id/currency; all were dead.
func TestSupplierCreateCarriesDeclaredFields(t *testing.T) {
	body, err := buildCreateBody("Vendor", map[string]string{
		"name": "TPG", "address": "1 Telstra Way", "city": "Melbourne",
		"state": "VIC", "postcode": "3000", "supplier-id": "V-442",
		"currency": "aud", "phone": "1300", "email": "ap@tpg.com.au",
	})
	if err != nil {
		t.Fatal(err)
	}
	addr := body["BillAddr"].(map[string]any)
	if addr["Line1"] != "1 Telstra Way" || addr["City"] != "Melbourne" || addr["PostalCode"] != "3000" {
		t.Fatalf("supplier address dropped: %v", addr)
	}
	if body["AcctNum"] != "V-442" {
		t.Fatal("--supplier-id dropped")
	}
	if body["CurrencyRef"].(map[string]any)["value"] != "AUD" {
		t.Fatal("--currency dropped")
	}
	if body["PrimaryPhone"].(map[string]any)["FreeFormNumber"] != "1300" {
		t.Fatal("--phone dropped")
	}
}

// customer create declared phone + currency; both were dead.
func TestCustomerCreateCarriesPhoneCurrency(t *testing.T) {
	body, err := buildCreateBody("Customer", map[string]string{"name": "J", "phone": "0400", "currency": "usd"})
	if err != nil {
		t.Fatal(err)
	}
	if body["PrimaryPhone"].(map[string]any)["FreeFormNumber"] != "0400" {
		t.Fatal("--phone dropped")
	}
	if body["CurrencyRef"].(map[string]any)["value"] != "USD" {
		t.Fatal("--currency dropped")
	}
}

// supplier/customer edits must apply the same contact fields sparsely.
func TestContactFieldsApplyOnUpdate(t *testing.T) {
	existing := map[string]any{"Id": "2", "SyncToken": "0"}
	body, err := buildUpdateBody("Vendor", map[string]string{"phone": "999", "postcode": "3001"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["PrimaryPhone"].(map[string]any)["FreeFormNumber"] != "999" {
		t.Fatal("--phone dropped on vendor update")
	}
	if body["BillAddr"].(map[string]any)["PostalCode"] != "3001" {
		t.Fatal("--postcode dropped on vendor update")
	}
}

// --- cross-cutting guards --------------------------------------------------

// --amount disagreeing with --line-items must be refused, not posted.
func TestValidateLineTotalRejectsMismatch(t *testing.T) {
	flags := map[string]string{"amount": "10.00",
		"line-items": `[{"Amount":4},{"Amount":5}]`}
	if err := validateLineTotal("Purchase", map[string]any{"Line": []map[string]any{{"Amount": 4.0}, {"Amount": 5.0}}}, flags); err == nil {
		t.Fatal("9.00 lines vs 10.00 amount accepted")
	}
	ok := map[string]any{"Line": []map[string]any{{"Amount": 4.0}, {"Amount": 6.0}}}
	if err := validateLineTotal("Purchase", ok, flags); err != nil {
		t.Fatalf("matching total rejected: %v", err)
	}
	// No supplied lines → no check (synthesised line matches by construction).
	if err := validateLineTotal("Purchase", map[string]any{}, map[string]string{"amount": "10"}); err != nil {
		t.Fatalf("lineless create rejected: %v", err)
	}
}

// A sparse update that carries no actual field change must not POST — it
// would bump SyncToken and report a fake success.
func TestBodyHasChangesDetectsNoop(t *testing.T) {
	existing := map[string]any{"Id": "1", "SyncToken": "0", "TxnDate": "2020-03-15",
		"Line": []any{map[string]any{"Amount": 5.0}}}
	noop := map[string]any{"Id": "1", "SyncToken": "0", "sparse": true, "TxnDate": "2020-03-15",
		"Line": []any{map[string]any{"Amount": 5.0}}}
	if bodyHasChanges(noop, existing) {
		t.Fatal("identical body flagged as changed")
	}
	changed := map[string]any{"Id": "1", "SyncToken": "0", "sparse": true, "TxnDate": "2020-03-16"}
	if !bodyHasChanges(changed, existing) {
		t.Fatal("date change not detected")
	}
}

// copyExisting must deep-copy: mutating the body must not corrupt the
// fetched document used for later comparison/readback.
func TestCopyExistingDeepCopies(t *testing.T) {
	existing := map[string]any{"Line": []any{map[string]any{"Amount": 1.0, "DepositLineDetail": map[string]any{"AccountRef": map[string]any{"value": "5"}}}}}
	out := map[string]any{}
	copyExisting(out, existing, "Line")
	if err := applySoleLineAccount(out, "DepositLineDetail", "9"); err != nil {
		t.Fatal(err)
	}
	got := existing["Line"].([]any)[0].(map[string]any)["DepositLineDetail"].(map[string]any)["AccountRef"].(map[string]any)["value"]
	if got != "5" {
		t.Fatal("copyExisting aliased the existing document's lines")
	}
}

// The bracket-annotation guard must protect Bills as well as Purchases:
// a memo that drops an existing annotation is rejected, and one that
// retains it lands.
func TestMemoGuardExtendsToBill(t *testing.T) {
	existing := map[string]any{"Id": "6927", "SyncToken": "0",
		"PrivateNote": "[Card: Coles] March lump",
		"VendorRef":   map[string]any{"value": "193"},
		"Line":        []any{map[string]any{"Amount": 299.95}}}
	if _, err := buildUpdateBody("Bill", map[string]string{"memo": "TPG consolidated"}, existing); err == nil {
		t.Fatal("memo that strips a bracketed annotation was accepted")
	}
	body, err := buildUpdateBody("Bill", map[string]string{"memo": "[Card: Coles] TPG consolidated"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := body["PrivateNote"].(string); !strings.Contains(note, "[Card: Coles]") {
		t.Fatalf("bracketed annotation stripped on bill memo: %q", note)
	}
}
