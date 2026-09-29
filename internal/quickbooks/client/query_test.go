package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildQuerySanitizesQuotes(t *testing.T) {
	got := buildQuery("Customer", "", "Acme';--", 20)
	if strings.Contains(got, ";") || strings.Contains(got, "'--") {
		t.Fatalf("unsanitized SQL: %s", got)
	}
	if !strings.Contains(got, "Acme") || !strings.Contains(got, "where") {
		t.Fatalf("query = %s", got)
	}
	idq := buildQuery("Customer", "1';drop", "", 5)
	if strings.Contains(idq, ";") {
		t.Fatalf("id query unsanitized: %s", idq)
	}

}

func TestProjectQueryCustomerArray(t *testing.T) {
	body := []byte(`{"QueryResponse":{"Customer":[{"Id":"7","DisplayName":"Acme","Balance":12.5,"Active":true}]}}`)
	got := projectQuery("Customer", body)
	if len(got) != 1 || got[0].ID != "7" || got[0].Name != "Acme" || got[0].Amount != 12.5 {
		t.Fatalf("%+v", got)
	}
}

func TestProjectQueryEmpty(t *testing.T) {
	got := projectQuery("Estimate", []byte(`{"QueryResponse":{},"time":"x"}`))
	if len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestEstimateSearchRetriesUnfilteredAfterFalseEmpty(t *testing.T) {
	saveUsable(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = io.WriteString(w, `{"QueryResponse":{"Estimate":[]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"QueryResponse":{"Estimate":[{"Id":"111","TxnDate":"2026-09-14","TotalAmt":1}]}}`)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(t.Context(), "Estimate", "", "111", 20)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(res.Items) != 1 || res.Items[0].ID != "111" {
		t.Fatalf("calls=%d result=%+v, want bounded unfiltered fallback", calls, res)
	}
}

// TestReplayQueryPagesStartPosition verifies that limits above one page
// drive ordered STARTPOSITION paging and merge the windows.
func TestReplayQueryPagesStartPosition(t *testing.T) {
	saveUsable(t)
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		write := func(n, base int) {
			var rows strings.Builder
			rows.WriteString(`{"QueryResponse":{"Class":[`)
			for i := range n {
				if i > 0 {
					rows.WriteString(",")
				}
				fmt.Fprintf(&rows, `{"Id":"%d","Name":"C%d","Active":true}`, base+i, base+i)
			}
			rows.WriteString(`]}}`)
			_, _ = io.WriteString(w, rows.String())
		}
		if strings.Contains(q, "startposition 101") {
			write(25, 100)
			return
		}
		write(100, 0)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(t.Context(), "Class", "", "", 250)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 125 {
		t.Fatalf("items=%d, want 125 across two pages", len(res.Items))
	}
	if len(queries) != 2 {
		t.Fatalf("queries=%v", queries)
	}
	if !strings.Contains(queries[0], "orderby Id") || strings.Contains(queries[0], "startposition") {
		t.Fatalf("page 1 query = %q, want ordered first window", queries[0])
	}
	if !strings.Contains(queries[1], "startposition 101") || !strings.Contains(queries[1], "orderby Id") {
		t.Fatalf("page 2 query = %q", queries[1])
	}
}

// TestReplayQuerySinglePageKeepsStatementShape verifies limits within one
// page emit the historic un-ordered statement with no STARTPOSITION.
func TestReplayQuerySinglePageKeepsStatementShape(t *testing.T) {
	saveUsable(t)
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("query")
		_, _ = io.WriteString(w, `{"QueryResponse":{"Class":[{"Id":"7","Name":"C","Active":true}]}}`)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	if _, err := ReplayQuery(t.Context(), "Class", "", "", 20); err != nil {
		t.Fatal(err)
	}
	if got != "select * from Class maxresults 20" {
		t.Fatalf("query = %q", got)
	}
}

func TestProjectQueryRecurringUnwrapsInvoice(t *testing.T) {
	body := []byte(`{"QueryResponse":{"RecurringTransaction":[{"Invoice":{"Id":"88","TotalAmt":1,"RecurringInfo":{"Name":"QB-CLI-TEST-REC"}}}]}}`)
	got := projectQuery("RecurringTransaction", body)
	if len(got) != 1 || got[0].ID != "88" || got[0].Name != "QB-CLI-TEST-REC" {
		t.Fatalf("%+v", got)
	}
}

func TestProjectQueryRecurringRejectsEmptyInvoice(t *testing.T) {
	got := projectQuery("RecurringTransaction", []byte(`{"QueryResponse":{"RecurringTransaction":[{}]}}`))
	if len(got) != 1 || got[0].ID != "" {
		t.Fatalf("empty nest should stay empty id: %+v", got)
	}
}

func TestSanitizeTokenStripsInjection(t *testing.T) {
	if sanitizeToken("a'; DELETE FROM Customer") != "a DELETE FROM Customer" {
		t.Fatalf("%q", sanitizeToken("a'; DELETE FROM Customer"))
	}
}

func TestReplayQueryNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayQuery(t.Context(), "Customer", "", "", 5)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestV3QueryableRejectsDelayedCharge(t *testing.T) {
	if V3Queryable("DelayedCharge") {
		t.Fatal("DelayedCharge must not be a v3 query context")
	}
	for _, entity := range []string{"Invoice", "Estimate", "Payment", "CreditMemo", "SalesReceipt", "DelayedCredit"} {
		if !V3Queryable(entity) {
			t.Errorf("%s must stay queryable", entity)
		}
	}
}

func TestReplayReportNoCreds(t *testing.T) {
	noCredsDir(t)
	_, err := ReplayReport(t.Context(), "ProfitAndLoss", "", "")
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestReplayReportRejectsBadName(t *testing.T) {
	_, err := ReplayReport(t.Context(), "../etc/passwd", "", "")
	if err == nil {
		t.Fatal("expected invalid report name")
	}
}

func TestProjectQueryAccountType(t *testing.T) {
	body := []byte(`{"QueryResponse":{"Account":[{"Id":"40","Name":"Freight","AccountType":"Cost of Goods Sold","AccountSubType":"SuppliesMaterialsCogs"}]}}`)
	got := projectQuery("Account", body)
	if len(got) != 1 || got[0].ID != "40" || got[0].Type != "Cost of Goods Sold" {
		t.Fatalf("%+v", got)
	}
}

func TestProjectQueryItemTypeNotOverwrittenByMissingPaymentType(t *testing.T) {
	body := []byte(`{"QueryResponse":{"Item":[{"Id":"3","Name":"Widget","Type":"Inventory"}]}}`)
	got := projectQuery("Item", body)
	if len(got) != 1 || got[0].Type != "Inventory" {
		t.Fatalf("%+v", got)
	}
}

func TestProjectQueryInventoryAdjustmentAccount(t *testing.T) {
	body := []byte(`{"QueryResponse":{"InventoryAdjustment":[{"Id":"159","DocNumber":"QB-ADJ-172351","TxnDate":"2026-08-18","AdjustAccountRef":{"value":"60","name":"Inventory Shrinkage"}}]}}`)
	got := projectQuery("InventoryAdjustment", body)
	if len(got) != 1 || got[0].ID != "159" || got[0].DocNumber != "QB-ADJ-172351" || got[0].Account != "Inventory Shrinkage" || got[0].AccountID != "60" {
		t.Fatalf("%+v", got)
	}
}

// The AU schema returns credit-card payments under CreditCardPaymentTxn;
// the read must resolve that collection key rather than reporting empty.
func TestProjectQueryCreditCardPaymentTxnAlias(t *testing.T) {
	if got := queryResponseEntity("CreditCardPayment"); got != "CreditCardPaymentTxn" {
		t.Fatalf("alias=%q", got)
	}
	body := []byte(`{"QueryResponse":{"CreditCardPaymentTxn":[{"Id":"120","Amount":10,"TxnDate":"2026-09-16","CreditCardAccountRef":{"value":"70","name":"QBVerify Credit Card"}}]}}`)
	if err := validateQueryEnvelope(queryResponseEntity("CreditCardPayment"), body); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got := projectQuery(queryResponseEntity("CreditCardPayment"), body)
	if len(got) != 1 || got[0].ID != "120" || got[0].Amount != 10 {
		t.Fatalf("%+v", got)
	}
}

func TestProjectQueryAccountRefFallback(t *testing.T) {
	body := []byte(`{"QueryResponse":{"Purchase":[{"Id":"1","AccountRef":{"value":"48","name":"Cash"}}]}}`)
	got := projectQuery("Purchase", body)
	if len(got) != 1 || got[0].Account != "Cash" || got[0].AccountID != "48" {
		t.Fatalf("%+v", got)
	}
}

func TestProjectQueryNoAccountWhenRefMissing(t *testing.T) {
	body := []byte(`{"QueryResponse":{"InventoryAdjustment":[{"Id":"159","DocNumber":"QB-ADJ-172351"}]}}`)
	got := projectQuery("InventoryAdjustment", body)
	if len(got) != 1 || got[0].Account != "" || got[0].AccountID != "" {
		t.Fatalf("invented account: %+v", got)
	}
}

func TestProjectQueryInventoryAdjustmentLine(t *testing.T) {
	body := []byte(`{"QueryResponse":{"InventoryAdjustment":[{"Id":"159","DocNumber":"QB-ADJ-172351","TxnDate":"2026-08-18","AdjustAccountRef":{"value":"60","name":"Inventory Shrinkage"},"Line":[{"DetailType":"ItemAdjustmentLineDetail","ItemAdjustmentLineDetail":{"ItemRef":{"value":"11","name":"QB-CLI-INV"},"QtyDiff":1}}]}]}}`)
	got := projectQuery("InventoryAdjustment", body)
	if len(got) != 1 || got[0].ID != "159" || got[0].Item != "QB-CLI-INV" || got[0].ItemID != "11" || got[0].Qty != 1 || got[0].Amount != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestProjectQueryNoLineFieldsWhenMissing(t *testing.T) {
	body := []byte(`{"QueryResponse":{"InventoryAdjustment":[{"Id":"159","DocNumber":"QB-ADJ-172351","AdjustAccountRef":{"value":"60","name":"Inventory Shrinkage"}}]}}`)
	got := projectQuery("InventoryAdjustment", body)
	if len(got) != 1 || got[0].Item != "" || got[0].ItemID != "" || got[0].Qty != 0 || got[0].Amount != 0 {
		t.Fatalf("invented line fields: %+v", got)
	}
}

func TestBuildQueryIdIncludesInactiveEmployee(t *testing.T) {
	got := buildQuery("Employee", "37", "", 20)
	if !strings.Contains(got, "Id = '37'") {
		t.Fatalf("query = %s", got)
	}
	if !strings.Contains(got, "Active IN (true, false)") {
		t.Fatalf("--id Employee must include inactive (QBO defaults Active=true): %s", got)
	}
	inv := buildQuery("Invoice", "1", "", 20)
	if strings.Contains(strings.ToLower(inv), "active") {
		t.Fatalf("Invoice --id must not add Active: %s", inv)
	}
}

func TestBuildQueryActiveFalse(t *testing.T) {
	got := buildQuery("Employee", "37", "", 20, "false")
	if !strings.Contains(got, "Id = '37'") || !strings.Contains(got, "Active = false") {
		t.Fatalf("query = %s", got)
	}
	all := buildQuery("Employee", "37", "", 20, "all")
	if !strings.Contains(all, "Active IN (true, false)") {
		t.Fatalf("query = %s", all)
	}
	junk := buildQuery("Employee", "37", "", 20, "';drop")
	if strings.Contains(junk, "drop") || strings.Contains(junk, ";") {
		t.Fatalf("unknown active must not inject: %s", junk)
	}
}

func TestBuildQueryEntitySearchFields(t *testing.T) {
	cases := map[string]string{
		"ExchangeRate": "SourceCurrencyCode",
		"CustomerType": "Name",
	}
	for entity, field := range cases {
		q := buildQuery(entity, "", "USD", 20)
		if !strings.Contains(q, field+" like '%USD%'") {
			t.Errorf("%s search = %q, want %s LIKE", entity, q, field)
		}
	}
}

func TestProjectQueryFallsBackToSourceCurrencyCode(t *testing.T) {
	body := []byte(`{"QueryResponse":{"ExchangeRate":[{"SourceCurrencyCode":"USD","TargetCurrencyCode":"AUD","Rate":1.3}]}}`)
	items := projectQuery("ExchangeRate", body)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Name != "USD" {
		t.Errorf("Name = %q, want USD", items[0].Name)
	}
	if got := filterItems(items, "usd"); len(got) != 1 {
		t.Error("client filter must match the projected currency code")
	}
}

func TestAttachableNameAndSearch(t *testing.T) {
	items := projectQuery("Attachable", []byte(`{"QueryResponse":{"Attachable":[{"Id":"7","FileName":"proof.txt"}]}}`))
	if len(items) != 1 || items[0].Name != "proof.txt" {
		t.Fatal("attachment filename discarded")
	}
	if !strings.Contains(buildQuery("Attachable", "", "proof", 5, ""), "FileName like") {
		t.Fatal("attachment search uses wrong field")
	}
	body, err := buildUpdateBody("Attachable", map[string]string{"name": "renamed.txt"}, map[string]any{"Id": "7", "SyncToken": "0"})
	if err != nil || body["FileName"] != "renamed.txt" {
		t.Fatal("attachment rename did not target FileName")
	}
}
