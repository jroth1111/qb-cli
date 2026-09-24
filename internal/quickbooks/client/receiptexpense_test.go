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

// The receipt-expense contract: list staged receipts via the receipts UI's
// /Query on stagetransactions, resolve payee + accounts via v3, then write the
// captured ACCEPTED_ADDED StageEntity back to the same endpoint.
const stageEntityID = "djQuMTo5MzQxNDU3NzY5NzU2ODU0OjA4OGFmYzQ3M2E:043f1a40-b361-11f1-aa32-4b634b437563"

func receiptExpenseServer(t *testing.T, staged string) *domServer {
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
		switch {
		case strings.Contains(r.URL.Path, "/stage/entities"):
			if strings.Contains(string(b), `"/Query"`) {
				_, _ = w.Write([]byte(staged))
				return
			}
			_, _ = w.Write([]byte(`[{"$type":"/Result","data":[{"$type":"/integration/StageEntity","id":"` + stageEntityID + `","state":"ACCEPTED_ADDED"}]}]`))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			q := r.URL.RawQuery + string(b)
			switch {
			case strings.Contains(q, "Vendor"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Vendor":[{"Id":"27","DisplayName":"TestSupp"}]}}`))
			case strings.Contains(q, "QBVerify"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"70","Name":"QBVerify Credit Card"}]}}`))
			default:
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"17","Name":"Office expenses"}]}}`))
			}
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

const stagedList = `[{"$type":"/Result","data":[{"$type":"/integration/StageEntity","id":"` + stageEntityID + `","state":"TO_BE_REVIEWED","transactionTrait":{"transactionSource":"RECEIPT_UPLOAD","receiptProcessingState":"PROCESSED","transaction":{"$type":"/transactions/Transaction","header":{"txnDate":"2026-09-18","amount":"42.50","referenceNumber":"R-9"}}}}],"pageInfo":{"limit":300,"nextOffset":1,"totalCount":1}}]`

func TestReceiptExpenseWireShape(t *testing.T) {
	srv := receiptExpenseServer(t, stagedList)
	res, err := ReplayReceiptExpenseCreate(context.Background(),
		stageEntityID, "TestSupp", "QBVerify Credit Card", "Office expenses:42.50")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Op != "receipt-expense create" || res.Entity != "Purchase" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/stage/entities") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	var doc []struct {
		Type  string `json:"$type"`
		ID    string `json:"id"`
		State string `json:"state"`
		Trait struct {
			Source string `json:"transactionSource"`
			Txn    struct {
				Header struct {
					TxnDate string `json:"txnDate"`
					RefNo   string `json:"referenceNumber"`
					Amount  string `json:"amount"`
					Account struct {
						ID string `json:"id"`
					} `json:"account"`
					Contact struct {
						ID string `json:"id"`
					} `json:"contact"`
				} `json:"header"`
				Lines struct {
					AccountLines []struct {
						Amount  string `json:"amount"`
						Account struct {
							ID string `json:"id"`
						} `json:"account"`
					} `json:"accountLines"`
				} `json:"lines"`
				Type string `json:"type"`
			} `json:"transaction"`
		} `json:"transactionTrait"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || len(doc) != 1 {
		t.Fatalf("body: %v (%d entities)", err, len(doc))
	}
	e := doc[0]
	if e.Type != "/integration/StageEntity" || e.ID != stageEntityID || e.State != "ACCEPTED_ADDED" {
		t.Fatalf("entity = %+v", e)
	}
	if e.Trait.Source != "RECEIPT_UPLOAD" || e.Trait.Txn.Type != "PURCHASE" {
		t.Fatalf("trait = %+v", e.Trait)
	}
	h := e.Trait.Txn.Header
	if h.TxnDate != "2026-09-18" || h.RefNo != "R-9" || h.Amount != "42.50" {
		t.Fatalf("header = %+v", h)
	}
	if h.Account.ID != "70" || h.Contact.ID != "27" {
		t.Fatalf("account/contact = %+v", h)
	}
	l := e.Trait.Txn.Lines.AccountLines
	if len(l) != 1 || l[0].Account.ID != "17" || l[0].Amount != "42.50" {
		t.Fatalf("lines = %+v", l)
	}
}

func TestReceiptExpenseSuffixMatch(t *testing.T) {
	receiptExpenseServer(t, stagedList)
	if _, err := ReplayReceiptExpenseCreate(context.Background(),
		"4b634b437563", "TestSupp", "QBVerify Credit Card", "Office expenses:10.00"); err != nil {
		t.Fatalf("suffix match: %v", err)
	}
}

func TestReceiptExpenseMultiLineSums(t *testing.T) {
	srv := receiptExpenseServer(t, stagedList)
	if _, err := ReplayReceiptExpenseCreate(context.Background(),
		stageEntityID, "TestSupp", "QBVerify Credit Card", "Office expenses:10.00,Office expenses:5.25"); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, _, body := srv.snap()
	if !strings.Contains(string(body), `"amount":"15.25"`) {
		t.Fatalf("header amount should sum lines: %.400s", body)
	}
}

func TestReceiptExpenseValidation(t *testing.T) {
	receiptExpenseServer(t, stagedList)
	if _, err := ReplayReceiptExpenseCreate(context.Background(), "", "TestSupp", "QBVerify Credit Card", "Office expenses:1"); err == nil ||
		!strings.Contains(err.Error(), "--receipt-id") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayReceiptExpenseCreate(context.Background(), "no-such-id", "TestSupp", "QBVerify Credit Card", "Office expenses:1"); err == nil ||
		!strings.Contains(err.Error(), "no staged receipt") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayReceiptExpenseCreate(context.Background(), stageEntityID, "Ghost", "QBVerify Credit Card", "Office expenses:1"); err == nil ||
		!strings.Contains(err.Error(), "payee") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayReceiptExpenseCreate(context.Background(), stageEntityID, "TestSupp", "QBVerify Credit Card", "Office expenses"); err == nil ||
		!strings.Contains(err.Error(), "amount") {
		t.Fatalf("err = %v", err)
	}
	if _, err := ReplayReceiptExpenseCreate(context.Background(), stageEntityID, "TestSupp", "QBVerify Credit Card", "Office expenses:abc"); err == nil ||
		!strings.Contains(err.Error(), "bad amount") {
		t.Fatalf("err = %v", err)
	}
}

func TestReceiptExpenseEmptyStageList(t *testing.T) {
	receiptExpenseServer(t, `[{"$type":"/Result","pageInfo":{"limit":300,"nextOffset":0,"totalCount":0}}]`)
	if _, err := ReplayReceiptExpenseCreate(context.Background(),
		stageEntityID, "TestSupp", "QBVerify Credit Card", "Office expenses:1"); err == nil ||
		!strings.Contains(err.Error(), "no staged receipt") {
		t.Fatalf("err = %v", err)
	}
}

func TestReceiptExpenseRequiresAcceptedAddedReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := &domServer{t: t}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/stage/entities") && strings.Contains(string(b), `"/Query"`):
			_, _ = w.Write([]byte(stagedList))
		case strings.Contains(r.URL.Path, "/stage/entities"):
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			q := r.URL.RawQuery + string(b)
			switch {
			case strings.Contains(q, "TestSupp"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Vendor":[{"Id":"27","DisplayName":"TestSupp"}]}}`))
			case strings.Contains(q, "QBVerify"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"70","Name":"QBVerify Credit Card"}]}}`))
			default:
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"17","Name":"Office expenses"}]}}`))
			}
		}
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayReceiptExpenseCreate(context.Background(), stageEntityID, "TestSupp", "QBVerify Credit Card", "Office expenses:42.50"); err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("err = %v", err)
	}
}
