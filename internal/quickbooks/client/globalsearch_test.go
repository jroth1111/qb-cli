package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Regression tests for the captured ceressos global-search surface
// (GetTransactionGlobalSearchEntities — the /app/fullSearch transactions
// pane; live-proven on TC2 2026-09-19).

func TestGlobalSearchWireShape(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"transactions":{"data":[
		{"type":"SALE_INVOICE","id":"7","referenceNumber":"INV-1001","contact":{"id":"1","fullName":"CR-Test-Customer-Edited","type":"CUSTOMER"},"currency":{"currencyType":"AUD"},"date":"2026-08-23","amount":120,"transactionStatus":"OPEN","balance":50,"memo":"m"},
		{"type":"RECEIVED_PAYMENT","id":"9","contact":{"id":"1","fullName":"CR-Test-Customer-Edited","type":"CUSTOMER"},"date":"2026-08-23","amount":50,"transactionStatus":"CLOSED"}
	],"totalCount":2}}}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayGlobalSearch(context.Background(), "invoice", 5)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 2 || res.Counts["totalCount"] != 2 {
		t.Fatalf("got status=%d items=%d total=%v", res.Status, len(res.Items), res.Counts)
	}
	it := res.Items[0]
	if it.ID != "7" || it.Type != "SALE_INVOICE" || it.Name != "CR-Test-Customer-Edited" || it.DocNumber != "INV-1001" || it.Amount != 120 || it.Date != "2026-08-23" {
		t.Fatalf("row projection = %+v", it)
	}
	calls, meth, path, body := srv.snap()
	if calls != 1 || meth != http.MethodPost || !strings.Contains(path, "/graphql") {
		t.Fatalf("call = %s %s x%d", meth, path, calls)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	if !strings.Contains(m["query"].(string), "GetTransactionGlobalSearchEntities") {
		t.Error("query doc missing GetTransactionGlobalSearchEntities")
	}
	vars, _ := m["variables"].(map[string]any)
	if vars["search"] != "invoice" || vars["transactionLimit"] != float64(5) {
		t.Errorf("variables = %v", vars)
	}
	if vars["useNER"] != true || vars["withContact"] != true {
		t.Errorf("with*/NER flags = %v", vars)
	}
}

func TestGlobalSearchNameFallback(t *testing.T) {
	saveUsableURIHost(t)
	// Transactions without a contact project DocNumber as the row name.
	srv := newDomServer(t, http.StatusOK, `{"data":{"transactions":{"data":[
		{"type":"JOURNAL_ENTRY","id":"204","referenceNumber":"204","amount":0,"date":"2026-09-19"}
	],"totalCount":1}}}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayGlobalSearch(context.Background(), "journal", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "204" {
		t.Fatalf("fallback name = %+v", res.Items)
	}
}

func TestGlobalSearchValidation(t *testing.T) {
	if _, err := ReplayGlobalSearch(context.Background(), "  ", 5); err == nil {
		t.Error("empty query should error")
	}
}

func TestGlobalSearchGQLError(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"transactions":{"data":[],"totalCount":0}},"errors":[{"message":"search unavailable"}]}`)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayGlobalSearch(context.Background(), "x", 5); err == nil || !strings.Contains(err.Error(), "search unavailable") {
		t.Fatalf("gql error = %v", err)
	}
}

func TestGlobalSearchHTTPError(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusForbidden, `{"error":"forbidden"}`)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayGlobalSearch(context.Background(), "x", 5); err == nil {
		t.Error("403 should error")
	}
}
