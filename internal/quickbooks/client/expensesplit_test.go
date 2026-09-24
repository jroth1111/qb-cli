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

// splitServer answers the v3 purchase GET, the account/tax-code queries, and
// the sparse-update POST.
func splitServer(t *testing.T) (*domServer, *map[string]any) {
	t.Helper()
	saveUsableURIHost(t)
	var lastPost map[string]any
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.calls++
		s.lastMeth = r.Method
		s.lastPath = r.URL.Path + "?" + r.URL.RawQuery
		s.lastBody = b
		s.mu.Unlock()
		if r.Method == http.MethodGet {
			q := r.URL.RawQuery
			switch {
			case strings.Contains(q, "TaxCode"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"TaxCode":[{"Id":"5","Name":"GST"}]}}`))
			case strings.Contains(q, "Account"):
				if strings.Contains(q, "Motor") {
					_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"15","Name":"Motor vehicle expenses"}]}}`))
				} else {
					_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"17","Name":"Office expenses"}]}}`))
				}
			case strings.Contains(r.URL.Path, "/purchase/"):
				_, _ = w.Write([]byte(splitPurchaseFixture))
			default:
				_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
			}
			return
		}
		_ = json.Unmarshal(b, &lastPost)
		_, _ = w.Write([]byte(`{"Purchase":{"Id":"158","SyncToken":"2","TotalAmt":30}}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s, &lastPost
}

const splitPurchaseFixture = `{"Purchase":{
  "Id":"158","SyncToken":"1","PaymentType":"CreditCard","TotalAmt":30,
  "AccountRef":{"value":"70"},"EntityRef":{"value":"3","type":"Vendor"},
  "TxnDate":"2026-09-18","CurrencyRef":{"value":"AUD"},
  "Line":[{"Id":"1","Amount":30,"DetailType":"AccountBasedExpenseLineDetail",
    "AccountBasedExpenseLineDetail":{"AccountRef":{"value":"17"}}}]}}`

func TestExpenseSplitReplacesLines(t *testing.T) {
	_, lastPost := splitServer(t)
	res, err := ReplayExpenseSplit(context.Background(), map[string]string{
		"id": "158",
		"splits": `[{"category-account":"Office expenses","amount":"20.00"},` +
			`{"category-account":"Motor vehicle expenses","amount":"10.00","gst-code":"GST"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "expense split" || res.Item.ID != "158" {
		t.Fatalf("result = %+v", res)
	}
	body := *lastPost
	if body["sparse"] != true || body["Id"] != "158" || body["SyncToken"] != "1" {
		t.Fatalf("body head = %#v", body)
	}
	if body["PaymentType"] != "CreditCard" {
		t.Fatalf("required fields not carried: %#v", body)
	}
	lines, _ := body["Line"].([]any)
	if len(lines) != 2 {
		t.Fatalf("lines = %#v", body["Line"])
	}
	l0, _ := lines[0].(map[string]any)
	d0, _ := l0["AccountBasedExpenseLineDetail"].(map[string]any)
	if d0["AccountRef"].(map[string]any)["value"] != "17" || l0["Amount"] != 20.0 {
		t.Fatalf("line 0 = %#v", l0)
	}
	l1, _ := lines[1].(map[string]any)
	d1, _ := l1["AccountBasedExpenseLineDetail"].(map[string]any)
	if d1["AccountRef"].(map[string]any)["value"] != "15" || l1["Amount"] != 10.0 {
		t.Fatalf("line 1 = %#v", l1)
	}
	if d1["TaxCodeRef"].(map[string]any)["value"] != "5" {
		t.Fatalf("tax ref = %#v", d1)
	}
}

func TestExpenseSplitSumValidation(t *testing.T) {
	_, lastPost := splitServer(t)
	_, err := ReplayExpenseSplit(context.Background(), map[string]string{
		"id":     "158",
		"splits": `[{"category-account":"17","amount":"20.00"},{"category-account":"15","amount":"5.00"}]`,
	})
	if err == nil || !strings.Contains(err.Error(), "does not equal") {
		t.Fatalf("sum mismatch accepted: %v", err)
	}
	if *lastPost != nil {
		t.Fatal("mismatched split still posted")
	}
}

func TestExpenseSplitValidation(t *testing.T) {
	splitServer(t)
	if _, err := ReplayExpenseSplit(context.Background(), map[string]string{}); err == nil ||
		!strings.Contains(err.Error(), "--id") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayExpenseSplit(context.Background(), map[string]string{"id": "158"}); err == nil ||
		!strings.Contains(err.Error(), "--splits") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayExpenseSplit(context.Background(), map[string]string{
		"id": "158", "splits": `not-json`,
	}); err == nil || !strings.Contains(err.Error(), "JSON array") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayExpenseSplit(context.Background(), map[string]string{
		"id": "158", "splits": `[{"amount":"30.00"}]`,
	}); err == nil || !strings.Contains(err.Error(), "category-account") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayExpenseSplit(context.Background(), map[string]string{
		"id": "158", "splits": `[{"category-account":"17","amount":"abc"}]`,
	}); err == nil || !strings.Contains(err.Error(), "bad amount") {
		t.Fatalf("err = %v", err)
	}
}
