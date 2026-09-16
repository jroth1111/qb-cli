package client

import "testing"

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
