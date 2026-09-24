package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterPageUsesCapturedRangeAndKeepsLineIdentity(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("X-Range") != "items=300-599" {
			t.Errorf("wrong range/method: %s %s", r.Method, r.Header.Get("X-Range"))
		}
		_, _ = w.Write([]byte(`[{"txnId":123,"sequence":0,"memo":"original note"},{"txnId":123,"sequence":1,"memo":"second line"}]`))
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	res, err := ReplayRegisterPage(context.Background(), "204", 300, 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Transactions) != 2 || res.Transactions[0].Sequence != "0" || res.Transactions[1].Sequence != "1" || res.Transactions[0].Memo != "original note" {
		t.Fatalf("lost line identity: %+v", res)
	}
}

func TestRegisterTruncatedOrUnknownBodyFailsClosed(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"txnId":123}`))
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	res, err := ReplayRegisterPage(context.Background(), "204", 0, 10000)
	if res != nil || err == nil || !strings.Contains(err.Error(), "smaller --limit") || strings.Contains(err.Error(), "txnId") {
		t.Fatalf("truncated register accepted or leaked: result=%v error=%v", res, err)
	}
}

func TestRegisterPreservesBookedReferences(t *testing.T) {
	for _, body := range []string{
		`[{"txnId":123,"accountId":7,"accountName":"Supplies","lineAccountId":204,"klassId":12,"klass":"Property","txnTypeId":54,"txnTypeString":"Expense","authorization":"secret"}]`,
		`{"transactions":[{"txnId":"123","accountId":"7","accountName":"Supplies","lineAccountId":"204","klassId":"12","klass":"Property","txnTypeId":"54","txnTypeString":"Expense"}]}`,
	} {
		rows, ok := extractRegisterTxns([]byte(body))
		if !ok || len(rows) != 1 {
			t.Fatal("register shape rejected")
		}
		res := projectRegister(rows)
		got := res.Transactions[0]
		if got.Account != "Supplies" || got.Class != "Property" {
			t.Fatalf("lost labels: %+v", got)
		}
		if got.ID != "123" || got.AccountID != "7" || got.LineAccountID != "204" || got.ClassID != "12" || got.TxnTypeID != "54" || got.TxnType != "Expense" {
			t.Fatalf("lost register references: %+v", got)
		}
		encoded, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "authorization") {
			t.Fatal("raw fields leaked")
		}
	}
}

func TestRegisterMissingReferencesRemainUnknown(t *testing.T) {
	rows, ok := extractRegisterTxns([]byte(`[{"txnId":123,"klass":null,"klassId":null}]`))
	if !ok {
		t.Fatal("shape rejected")
	}
	encoded, err := json.Marshal(projectRegister(rows))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"account_id", "line_account_id", "class_id", "txn_type"} {
		if strings.Contains(string(encoded), key) {
			t.Fatalf("invented reference %s", key)
		}
	}
}
