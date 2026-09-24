package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The reclassify suite exercises the qbo.intuit.com datarequest surface:
// account resolution rides the v3 query service, txn lines come from
// getTxnData (a JS-literal body, not JSON), and the write posts
// reclassifyTxn/reclassify — dispatch keys off the URL path.

func reclassifyServer(t *testing.T) *domServer {
	t.Helper()
	return reclassifyServerResponse(t, `[{"txnId":13,"sequence":1,"result":"Success"},{"txnId":5,"sequence":1,"result":"Success"}]`)
}

func reclassifyServerResponse(t *testing.T, mutationResponse string) *domServer {
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
		s.lastPath = r.URL.Path
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/v3/company/") && strings.Contains(r.URL.Path, "/query"):
			_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[
				{"Id":"7","Name":"Accounting and bookkeeping","AccountType":"Expense"},
				{"Id":"17","Name":"Office expenses","AccountType":"Expense"}
			]},"time":"2026-09-18T00:00:00Z"}`))
		case strings.Contains(r.URL.Path, "getTxnData"):
			_, _ = w.Write([]byte(`{
  "items" : [ {
    "txnId" : 13, "isTaxLine" : false, "sequence" : 1, "editSequence" : 5,
    "date" : new Date(2026,7,23), "type" : "Supplier Credit",
    "amount" : qbo.amount.moneyFromLocalizedString('-30.00'),
    "name" : "CR-Test-Supplier", "account" : "Office expenses",
    "accountId" : 17, "typeId" : 12, "editable" : true
  }, {
    "txnId" : 5, "isTaxLine" : false, "sequence" : 1, "editSequence" : 5,
    "date" : new Date(2026,7,22), "type" : "Expense",
    "amount" : qbo.amount.moneyFromLocalizedString('15.00'),
    "name" : "CR-Test-Supplier", "account" : "Office expenses",
    "accountId" : 17, "typeId" : 54, "editable" : true
  } ]
}`))
		case strings.Contains(r.URL.Path, "reclassifyTxn/reclassify"):
			_, _ = w.Write([]byte(mutationResponse))
		default:
			_, _ = w.Write([]byte(`{"error":[{"code":"SRV-999","message":"unmatched"}]}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestReclassifyHTTP200LineFailureIsNotSuccess(t *testing.T) {
	reclassifyServerResponse(t, `[{"txnId":13,"sequence":1,"result":"FailedChangeNotAllowed","additionalMessage":"Check your account details before you continue."},{"txnId":5,"sequence":1,"result":"Success"}]`)
	result, err := ReplayReclassify(context.Background(), "account", "17", "7", "2026-08-01", "2026-08-31", "accrual")
	if result != nil || err == nil || !strings.Contains(err.Error(), "FailedChangeNotAllowed") || !strings.Contains(err.Error(), "readbacks before retry") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestReclassifyReceiptFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"line success", `[{"txnId":1,"sequence":1,"result":"Success"}]`, false},
		{"partial failure", `[{"txnId":1,"result":"Success"},{"txnId":2,"result":"FailedChangeNotAllowed"}]`, true},
		{"nested failure", `{"items":[{"txnId":1,"result":"Failed"}]}`, true},
		{"unknown result", `[{"txnId":1,"result":"Pending"}]`, true},
		{"blank result", `[{"txnId":1,"result":""}]`, true},
		{"non-string result", `[{"txnId":1,"result":false}]`, true},
		{"invalid receipt", `<html>sign in</html>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := reclassifyReceiptFailure([]byte(tc.body)); (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%v", err, tc.wantError)
			}
		})
	}
}

func TestReclassifyBuildsTxnInfosFromJSLiteral(t *testing.T) {
	srv := reclassifyServer(t)
	res, err := ReplayReclassify(context.Background(), "account",
		"Office expenses", "Accounting and bookkeeping", "01/08/2026", "31/08/2026", "")
	if err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	if res.Op != "reclassify" || res.Item.ID != "7" {
		t.Fatalf("result = %+v", res)
	}
	vars := srv.sentMap()
	infos, ok := vars["txnInfos"].([]any)
	if !ok || len(infos) != 2 {
		t.Fatalf("txnInfos = %v", vars)
	}
	first := infos[0].(map[string]any)
	if first["txnId"] != float64(13) || first["editSequence"] != float64(5) ||
		first["sequence"] != float64(1) || first["txnType"] != float64(12) {
		t.Fatalf("txnInfo = %v", first)
	}
	second := infos[1].(map[string]any)
	if second["txnId"] != float64(5) || second["txnType"] != float64(54) {
		t.Fatalf("txnInfo = %v", second)
	}
	if vars["accountId"] != "7" {
		t.Fatalf("target accountId = %v", vars["accountId"])
	}
}

func TestReclassifyEmptySourceIsNoop(t *testing.T) {
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/query") {
			_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[
				{"Id":"7","Name":"Accounting and bookkeeping"},
				{"Id":"17","Name":"Office expenses"}]}}`))
			return
		}
		if strings.Contains(r.URL.Path, "reclassifyTxn/reclassify") {
			t.Error("must not post a write when the txn list is empty")
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	res, err := ReplayReclassify(context.Background(), "account",
		"Office expenses", "Accounting and bookkeeping", "01/08/2026", "31/08/2026", "")
	if err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	if !strings.Contains(res.Note, "no editable transactions") {
		t.Fatalf("result = %+v", res)
	}
}

func TestReclassifyRejectsNonAccountTarget(t *testing.T) {
	for _, target := range []string{"class", "location", "gst-code", "bogus"} {
		if _, err := ReplayReclassify(context.Background(), target, "x", "y", "", "", ""); err == nil ||
			!strings.Contains(err.Error(), "only 'account' is wired") {
			t.Fatalf("target %q: err = %v", target, err)
		}
	}
}

func TestReclassifyRequiresAccounts(t *testing.T) {
	if _, err := ReplayReclassify(context.Background(), "", "a", "b", "", "", ""); err == nil {
		t.Fatal("expected error for missing target")
	}
}

func TestReclassifySameAccountRejected(t *testing.T) {
	reclassifyServer(t)
	if _, err := ReplayReclassify(context.Background(), "account",
		"Office expenses", "Office expenses", "", "", ""); err == nil ||
		!strings.Contains(err.Error(), "same account") {
		t.Fatalf("err = %v", err)
	}
}

func TestReclassifyUnknownAccount(t *testing.T) {
	reclassifyServer(t)
	if _, err := ReplayReclassify(context.Background(), "account",
		"No Such Account", "Office expenses", "", "", ""); err == nil ||
		!strings.Contains(err.Error(), "no account named") {
		t.Fatalf("err = %v", err)
	}
}

func TestReclassifyNumericIDsSkipLookup(t *testing.T) {
	srv := reclassifyServer(t)
	res, err := ReplayReclassify(context.Background(), "account", "17", "7", "2026-08-01", "2026-08-31", "cash")
	if err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	if res.Item.ID != "7" {
		t.Fatalf("result = %+v", res)
	}
	vars := srv.sentMap()
	if vars["accountId"] != "7" {
		t.Fatalf("accountId = %v", vars["accountId"])
	}
}
