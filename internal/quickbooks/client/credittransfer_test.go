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

// The credit-transfer contract: POST /api/v3/company/{realm}/journalentry
// with two JournalEntryLineDetail legs — Debit A/R carrying the source
// customer entity, Credit A/R carrying the destination customer entity
// (Type:"Customer" title case; "CUSTOMER" is a live 2010). The AU doc'd
// workaround for moving a credit between customers, proven live on TC2
// (JE 198, deleted).
func transferServer(t *testing.T, postResp string) *domServer {
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
		if strings.Contains(r.URL.Path, "/query") {
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			switch {
			case strings.Contains(q, "from Customer"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Customer":[` +
					`{"Id":"1","DisplayName":"CustA"},` +
					`{"Id":"2","DisplayName":"CustB"}]}}`))
			case strings.Contains(q, "Accounts Receivable"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[` +
					`{"Id":"48","Name":"Accounts Receivable (A/R)"}]}}`))
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

func TestCreditTransferWireShape(t *testing.T) {
	srv := transferServer(t, `{"JournalEntry":{"Id":"198","SyncToken":"0"}}`)
	res, err := ReplayCreditTransfer(context.Background(), "CustA", "CustB", "20")
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if res.Op != "transfer" || res.Entity != "JournalEntry" || res.Item.ID != "198" || res.Item.Amount != 20 {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/journalentry") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	legs := bounceLegs(t, body)
	if len(legs) != 2 {
		t.Fatalf("legs = %d, want 2: %v", len(legs), legs)
	}
	check := func(l map[string]any, wantPost, wantCust string) {
		t.Helper()
		d, _ := l["JournalEntryLineDetail"].(map[string]any)
		if d["PostingType"] != wantPost {
			t.Fatalf("PostingType = %v, want %s", d["PostingType"], wantPost)
		}
		acct, _ := d["AccountRef"].(map[string]any)
		if acct["value"] != "48" {
			t.Fatalf("AccountRef = %v, want A/R 48", acct)
		}
		ent, _ := d["Entity"].(map[string]any)
		eref, _ := ent["EntityRef"].(map[string]any)
		if ent["Type"] != "Customer" || eref["value"] != wantCust {
			t.Fatalf("Entity = %v, want EntityRef %s Type Customer", d["Entity"], wantCust)
		}
		if l["Amount"] != 20.0 {
			t.Fatalf("Amount = %v, want 20", l["Amount"])
		}
	}
	check(legs[0], "Debit", "1")  // source consumed
	check(legs[1], "Credit", "2") // destination credited
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("sent body: %v", err)
	}
	if sent["TxnDate"] == "" || sent["TxnDate"] == nil {
		t.Fatalf("TxnDate missing: %v", sent)
	}
}

func TestCreditTransferNumericIDs(t *testing.T) {
	transferServer(t, `{"JournalEntry":{"Id":"9"}}`)
	res, err := ReplayCreditTransfer(context.Background(), "1", "2", "5.5")
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if res.Item.ID != "9" || res.Item.Amount != 5.5 {
		t.Fatalf("result = %+v", res)
	}
}

func TestCreditTransferValidation(t *testing.T) {
	transferServer(t, `{"JournalEntry":{"Id":"9"}}`)
	if _, err := ReplayCreditTransfer(context.Background(), "", "CustB", "10"); err == nil || !strings.Contains(err.Error(), "--from") {
		t.Fatalf("missing from should fail, got %v", err)
	}
	if _, err := ReplayCreditTransfer(context.Background(), "CustA", "", "10"); err == nil || !strings.Contains(err.Error(), "--to") {
		t.Fatalf("missing to should fail, got %v", err)
	}
	if _, err := ReplayCreditTransfer(context.Background(), "CustA", "custA", "10"); err == nil || !strings.Contains(err.Error(), "same customer") {
		t.Fatalf("same customer should fail, got %v", err)
	}
	if _, err := ReplayCreditTransfer(context.Background(), "CustA", "CustB", "abc"); err == nil || !strings.Contains(err.Error(), "--amount") {
		t.Fatalf("bad amount should fail, got %v", err)
	}
	if _, err := ReplayCreditTransfer(context.Background(), "CustA", "CustB", "0"); err == nil || !strings.Contains(err.Error(), "--amount") {
		t.Fatalf("zero amount should fail, got %v", err)
	}
	if _, err := ReplayCreditTransfer(context.Background(), "Nobody", "CustB", "10"); err == nil || !strings.Contains(err.Error(), "no Customer named") {
		t.Fatalf("unknown from should fail, got %v", err)
	}
}

func TestCreditTransferHTTPError(t *testing.T) {
	transferServer(t, "") // → 400 boom
	_, err := ReplayCreditTransfer(context.Background(), "CustA", "CustB", "10")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("HTTP error should surface, got %v", err)
	}
}
