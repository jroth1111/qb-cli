package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The cheque-bounce contract: POST /api/v3/company/{realm}/journalentry with
// JournalEntryLineDetail legs — Debit A/R carrying Entity{EntityRef: customer,
// Type:"Customer"} (title case; "CUSTOMER" is a live 2010), Credit the
// payment's DepositToAccountRef, plus optional bank-charges fee legs. No form
// exposes a bounce action and v3 rejects A/R in expense lines (400 6430) —
// the JE reversal is the AU doc'd procedure, proven live on TC2 (JE 196).
func bounceServer(t *testing.T, paymentRow string, postResp string) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/journalentry") {
			if postResp == "" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"Fault":{"Error":[{"code":"6000","Message":"boom"}],"type":"ValidationFault"}}`))
				return
			}
			_, _ = w.Write([]byte(postResp))
			return
		}
		if strings.Contains(r.URL.Path, "/v3/company/") && strings.Contains(r.URL.Path, "/query") {
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			switch {
			case strings.Contains(q, "from Payment"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Payment":[` + paymentRow + `]}}`))
			case strings.Contains(q, "Accounts Receivable"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[` +
					`{"Id":"48","Name":"Accounts Receivable (A/R)"}]}}`))
			case strings.Contains(q, "from Account"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[` +
					`{"Id":"30","Name":"Undeposited funds"},` +
					`{"Id":"9","Name":"Bank charges and fees"}]}}`))
			default:
				_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func bouncePaymentRow() string {
	return `{"Id":"9","TotalAmt":50,` +
		`"CustomerRef":{"value":"1","name":"CR-Test-Customer-Edited"},` +
		`"DepositToAccountRef":{"value":"30","name":"Undeposited funds"}}`
}

func bounceOK(id string) string {
	return `{"JournalEntry":{"Id":"` + id + `","SyncToken":"0","TxnDate":"2026-09-19"}}`
}

func bounceLegs(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var sent struct {
		TxnDate string           `json:"TxnDate"`
		Line    []map[string]any `json:"Line"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("sent JE body: %v", err)
	}
	return sent.Line
}

func TestChequeBounceWireShape(t *testing.T) {
	srv := bounceServer(t, bouncePaymentRow(), bounceOK("196"))
	res, err := ReplayChequeBounce(context.Background(), "9", "19/09/2026", "15")
	if err != nil {
		t.Fatalf("bounce: %v", err)
	}
	if res.Op != "bounce" || res.Entity != "JournalEntry" || res.Item.ID != "196" || res.Item.Amount != 65 {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/journalentry") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("sent body: %v", err)
	}
	if sent["TxnDate"] != "2026-09-19" {
		t.Fatalf("TxnDate = %v", sent["TxnDate"])
	}
	legs := bounceLegs(t, body)
	if len(legs) != 4 {
		t.Fatalf("legs = %d, want 4 (ar-debit, bank-credit, fee-debit, fee-credit): %v", len(legs), legs)
	}
	detail := func(l map[string]any) map[string]any {
		d, _ := l["JournalEntryLineDetail"].(map[string]any)
		return d
	}
	d0 := detail(legs[0])
	if d0["PostingType"] != "Debit" || legs[0]["Amount"] != 50.0 {
		t.Fatalf("leg0 = %v", legs[0])
	}
	acct, _ := d0["AccountRef"].(map[string]any)
	if acct["value"] != "48" {
		t.Fatalf("leg0 AccountRef = %v, want A/R 48", d0["AccountRef"])
	}
	ent, _ := d0["Entity"].(map[string]any)
	eref, _ := ent["EntityRef"].(map[string]any)
	if ent["Type"] != "Customer" || eref["value"] != "1" {
		t.Fatalf("leg0 Entity = %v, want {EntityRef:{value:1},Type:Customer}", d0["Entity"])
	}
	d1 := detail(legs[1])
	acct1, _ := d1["AccountRef"].(map[string]any)
	if d1["PostingType"] != "Credit" || acct1["value"] != "30" {
		t.Fatalf("leg1 = %v, want Credit deposit acct 30", legs[1])
	}
	d2 := detail(legs[2])
	acct2, _ := d2["AccountRef"].(map[string]any)
	if d2["PostingType"] != "Debit" || acct2["value"] != "9" || legs[2]["Amount"] != 15.0 {
		t.Fatalf("leg2 = %v, want Debit charges acct 9 amt 15", legs[2])
	}
	d3 := detail(legs[3])
	acct3, _ := d3["AccountRef"].(map[string]any)
	if d3["PostingType"] != "Credit" || acct3["value"] != "30" || legs[3]["Amount"] != 15.0 {
		t.Fatalf("leg3 = %v, want Credit deposit acct 30 amt 15", legs[3])
	}
}

func TestChequeBounceNoFee(t *testing.T) {
	srv := bounceServer(t, bouncePaymentRow(), bounceOK("9"))
	res, err := ReplayChequeBounce(context.Background(), "9", "2026-09-19", "")
	if err != nil {
		t.Fatalf("bounce: %v", err)
	}
	if res.Item.Amount != 50 {
		t.Fatalf("amount = %v, want 50", res.Item.Amount)
	}
	_, _, _, body := srv.snap()
	if legs := bounceLegs(t, body); len(legs) != 2 {
		t.Fatalf("legs = %d, want 2 without --fee", len(legs))
	}
}

func TestChequeBounceUndepositedFallback(t *testing.T) {
	// Payment without DepositToAccountRef falls back to the 'Undeposited
	// funds' account by name.
	srv := bounceServer(t,
		`{"Id":"9","TotalAmt":50,"CustomerRef":{"value":"1","name":"C"}}`,
		bounceOK("9"))
	if _, err := ReplayChequeBounce(context.Background(), "9", "", ""); err != nil {
		t.Fatalf("bounce: %v", err)
	}
	_, _, _, body := srv.snap()
	legs := bounceLegs(t, body)
	d1, _ := legs[1]["JournalEntryLineDetail"].(map[string]any)
	acct, _ := d1["AccountRef"].(map[string]any)
	if acct["value"] != "30" {
		t.Fatalf("credit leg AccountRef = %v, want undeposited-funds 30", acct)
	}
}

func TestChequeBounceValidation(t *testing.T) {
	bounceServer(t, bouncePaymentRow(), bounceOK("9"))
	if _, err := ReplayChequeBounce(context.Background(), "", "", ""); err == nil || !strings.Contains(err.Error(), "--id") {
		t.Fatalf("missing id should fail, got %v", err)
	}
	if _, err := ReplayChequeBounce(context.Background(), "9", "not-a-date", ""); err == nil || !strings.Contains(err.Error(), "--date") {
		t.Fatalf("bad date should fail, got %v", err)
	}
	if _, err := ReplayChequeBounce(context.Background(), "9", "", "abc"); err == nil || !strings.Contains(err.Error(), "--fee") {
		t.Fatalf("bad fee should fail, got %v", err)
	}
	if _, err := ReplayChequeBounce(context.Background(), "9", "", "-5"); err == nil || !strings.Contains(err.Error(), "--fee") {
		t.Fatalf("negative fee should fail, got %v", err)
	}
}

func TestChequeBounceMissingPayment(t *testing.T) {
	bounceServer(t, "", bounceOK("9")) // empty row → "Payment":[]
	_, err := ReplayChequeBounce(context.Background(), "77", "", "")
	if err == nil || !strings.Contains(err.Error(), "no payment with id 77") {
		t.Fatalf("missing payment should fail, got %v", err)
	}
}

func TestChequeBounceZeroPayment(t *testing.T) {
	bounceServer(t,
		`{"Id":"9","TotalAmt":0,"CustomerRef":{"value":"1"},"DepositToAccountRef":{"value":"30"}}`,
		bounceOK("9"))
	_, err := ReplayChequeBounce(context.Background(), "9", "", "")
	if err == nil || !strings.Contains(err.Error(), "nothing to reverse") {
		t.Fatalf("zero payment without fee should fail, got %v", err)
	}
}

func TestChequeBounceHTTPError(t *testing.T) {
	bounceServer(t, bouncePaymentRow(), "") // postResp empty → 400
	_, err := ReplayChequeBounce(context.Background(), "9", "", "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("HTTP error should surface, got %v", err)
	}
}
