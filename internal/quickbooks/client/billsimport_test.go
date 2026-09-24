package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bills-import contract: resolve supplier (create via lists/name/save
// when missing), resolve account via v3 query, then post the captured
// createTransactions_Transaction mutation per row.
func billsImportServer(t *testing.T, vendorFound bool) *domServer {
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
		case strings.Contains(r.URL.Path, "/lists/name/save"):
			_, _ = w.Write([]byte(`{"name":{"id":"99","displayName":"NewVendor"}}`))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			// v3 query service — entity name rides the query string/body
			if strings.Contains(r.URL.RawQuery+string(b), "Vendor") {
				if vendorFound {
					_, _ = w.Write([]byte(`{"QueryResponse":{"Vendor":[{"Id":"27","DisplayName":"QBCliBillSupp2"}]}}`))
				} else {
					_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
				}
			} else {
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"17","Name":"Office expenses"}]}}`))
			}
		case strings.Contains(r.URL.Path, "/v4/graphql"):
			_, _ = w.Write([]byte(`{"data":{"createTransactions_Transaction":{"clientMutationId":"0","transactionsTransactionEdge":{"node":{"id":"djQuMTo5:145"}}}}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func writeBillsCSV(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bills.csv")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBillsImportWireShape(t *testing.T) {
	srv := billsImportServer(t, true)
	path := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount\nQBCliBillSupp2,BN-4004,04/09/2026,18/09/2026,Office expenses,44.00\n")
	res, err := ReplayBillsImport(context.Background(), path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Op != "bills import" || res.Item.Name != "1 bills" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/v4/graphql") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	var doc struct {
		Query     string `json:"query"`
		Variables struct {
			Input struct {
				ClientMutationID string `json:"clientMutationId"`
				Txn              struct {
					Header struct {
						TxnDate string `json:"txnDate"`
						RefNo   string `json:"referenceNumber"`
						Contact struct {
							ID string `json:"id"`
						} `json:"contact"`
						TxnType string `json:"transactionType"`
						Curr    struct {
							Code         string `json:"code"`
							ExchangeRate string `json:"exchangeRate"`
						} `json:"currencyInfo"`
					} `json:"header"`
					Traits struct {
						Balance struct {
							DueDate string `json:"dueDate"`
						} `json:"balance"`
					} `json:"traits"`
					Lines struct {
						AccountLines []struct {
							Account struct {
								ID string `json:"id"`
							} `json:"account"`
							Amount string `json:"amount"`
						} `json:"accountLines"`
					} `json:"lines"`
					Type string `json:"type"`
				} `json:"transactionsTransaction"`
			} `json:"createTransactionInput"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("body: %v", err)
	}
	if !strings.Contains(doc.Query, "createTransactions_Transaction") {
		t.Fatalf("query missing mutation: %s", doc.Query[:80])
	}
	txn := doc.Variables.Input.Txn
	if doc.Variables.Input.ClientMutationID != "0" {
		t.Fatal("clientMutationId must be \"0\"")
	}
	if txn.Header.TxnDate != "2026-09-04" || txn.Traits.Balance.DueDate != "2026-09-18" {
		t.Fatalf("dates = %s / %s", txn.Header.TxnDate, txn.Traits.Balance.DueDate)
	}
	if txn.Header.RefNo != "BN-4004" || txn.Header.Contact.ID != "27" || txn.Header.TxnType != "PURCHASE_BILL" {
		t.Fatalf("header = %+v", txn.Header)
	}
	if txn.Header.Curr.Code != "AUD" || txn.Header.Curr.ExchangeRate != "1" {
		t.Fatalf("currencyInfo = %+v", txn.Header.Curr)
	}
	if len(txn.Lines.AccountLines) != 1 || txn.Lines.AccountLines[0].Account.ID != "17" || txn.Lines.AccountLines[0].Amount != "44.00" {
		t.Fatalf("lines = %+v", txn.Lines.AccountLines)
	}
	if txn.Type != "PURCHASE_BILL" {
		t.Fatalf("type = %s", txn.Type)
	}
}

func TestBillsImportCreatesMissingSupplier(t *testing.T) {
	srv := billsImportServer(t, false)
	path := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount\nNewVendor,BN-1,04/09/2026,18/09/2026,Office expenses,10.00\n")
	res, err := ReplayBillsImport(context.Background(), path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !strings.Contains(res.Note, "lists/name/save") {
		t.Fatalf("note = %s", res.Note)
	}
	// vendor query (miss) + name/save + account query + mutation = 4 calls.
	srv.mu.Lock()
	calls := srv.calls
	srv.mu.Unlock()
	if calls < 4 {
		t.Fatalf("calls = %d, want vendor query + name/save + account query + mutation", calls)
	}
}

func TestBillsImportLookupFailureDoesNotCreateSupplierOrBill(t *testing.T) {
	saveUsableURIHost(t)
	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v3/company/") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"Fault":{"Error":[{"Message":"lookup unavailable"}]}}`))
			return
		}
		if strings.Contains(r.URL.Path, "/lists/name/save") || strings.Contains(r.URL.Path, "/v4/graphql") {
			writes++
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	path := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount\nUnavailableVendor,BN-5,04/09/2026,18/09/2026,Office expenses,10\n")
	if _, err := ReplayBillsImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "supplier lookup") {
		t.Fatalf("err = %v", err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want zero after lookup failure", writes)
	}
}

func TestBillsImportCappedLookupDoesNotCreateSupplierOrBill(t *testing.T) {
	saveUsableURIHost(t)
	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v3/company/") {
			w.Header().Set("Content-Type", "application/json")
			items := strings.TrimSuffix(strings.Repeat(`{"Id":"1","DisplayName":"PotentiallyHiddenVendor-other"},`, 10), ",")
			_, _ = w.Write([]byte(`{"QueryResponse":{"Vendor":[` + items + `]}}`))
			return
		}
		if strings.Contains(r.URL.Path, "/lists/name/save") || strings.Contains(r.URL.Path, "/v4/graphql") {
			writes++
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	path := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount\nPotentiallyHiddenVendor,BN-6,04/09/2026,18/09/2026,Office expenses,10\n")
	if _, err := ReplayBillsImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "result limit") {
		t.Fatalf("err = %v", err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want zero after capped lookup", writes)
	}
}

func TestBillsImportBlankExactMatchIDDoesNotCreateSupplierOrBill(t *testing.T) {
	saveUsableURIHost(t)
	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v3/company/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"QueryResponse":{"Vendor":[{"Id":"","DisplayName":"NoIDVendor"}]}}`))
			return
		}
		if strings.Contains(r.URL.Path, "/lists/name/save") || strings.Contains(r.URL.Path, "/v4/graphql") {
			writes++
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	path := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount\nNoIDVendor,BN-7,04/09/2026,18/09/2026,Office expenses,10\n")
	if _, err := ReplayBillsImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "without an id") {
		t.Fatalf("err = %v", err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want zero after blank-id lookup", writes)
	}
}

func TestBillsImportValidation(t *testing.T) {
	billsImportServer(t, true)
	if _, err := ReplayBillsImport(context.Background(), ""); err == nil ||
		!strings.Contains(err.Error(), "--file") {
		t.Fatalf("err = %v", err)
	}
	path := writeBillsCSV(t, "Supplier,Amount\nX,10\n")
	if _, err := ReplayBillsImport(context.Background(), path); err == nil ||
		!strings.Contains(err.Error(), "missing required columns") {
		t.Fatalf("err = %v", err)
	}
	path2 := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount\nX,B1,04/09/2026,18/09/2026,Office expenses,10\n")
	// header only
	path3 := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount\n")
	if _, err := ReplayBillsImport(context.Background(), path3); err == nil ||
		!strings.Contains(err.Error(), "header row plus at least one data row") {
		t.Fatalf("err = %v", err)
	}
	_ = path2
}

func TestBillsImportRejectsForeignCurrency(t *testing.T) {
	billsImportServer(t, true)
	path := writeBillsCSV(t, "Supplier,Ref no.,Date,Due date,Account,Amount,Currency\nQBCliBillSupp2,B2,04/09/2026,18/09/2026,Office expenses,10,USD\n")
	if _, err := ReplayBillsImport(context.Background(), path); err == nil ||
		!strings.Contains(err.Error(), "home-currency") {
		t.Fatalf("err = %v", err)
	}
}

func TestBillDateISO(t *testing.T) {
	for in, want := range map[string]string{
		"04/09/2026": "2026-09-04",
		"4/9/2026":   "2026-09-04",
		"2026-09-04": "2026-09-04",
		"04-09-2026": "2026-09-04",
	} {
		got, err := billDateISO(in)
		if err != nil || got != want {
			t.Fatalf("billDateISO(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := billDateISO("not-a-date"); err == nil {
		t.Fatal("expected error")
	}
}
