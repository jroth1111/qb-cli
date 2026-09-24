package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The time-invoice contract: v3 query for Billable TimeActivity rows, a v4
// node read per charge (links.targets = consumed), then one
// UpdateTransactions_Transaction_qbo per customer whose item lines consume
// the charge through links.sources{id,sourceLine,sourceTxn}.
func timeInvoiceServer(t *testing.T, taRows []string, consumed map[string]bool) (*domServer, *int) {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
	muts := 0
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
		case strings.Contains(r.URL.Path, "/v4/graphql"):
			body := string(b)
			if strings.Contains(body, "node(id:") {
				// charge consumption check — links:null when unconsumed
				id := regexp.MustCompile(`"id":\s*"[^"]*:([0-9-]+)"`).FindStringSubmatch(body)
				charge := ""
				if len(id) > 1 {
					charge = id[1]
				}
				if consumed[charge] {
					_, _ = w.Write([]byte(`{"data":{"node":{"id":"ns:` + charge + `","type":"ACTIVITY_TIME","links":{"targets":{"edges":[{"node":{"id":"link:1","targetTxn":{"id":"ns:9","type":"SALE_INVOICE"}}}]}}}}}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":{"node":{"id":"ns:` + charge + `","type":"ACTIVITY_TIME","links":null}}}`))
				return
			}
			if strings.Contains(body, "updateTransactions_Transaction") {
				muts++
				_, _ = w.Write([]byte(`{"data":{"updateTransactions_Transaction":{"clientMutationId":"0","transactionsTransaction":{"id":"ns:173","entityVersion":"0"}}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{}}`))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			if strings.Contains(q, "TimeActivity") {
				_, _ = w.Write([]byte(`{"QueryResponse":{"TimeActivity":[` + strings.Join(taRows, ",") + `]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s, &muts
}

var taRow = `{"Id":"1073741832","SyncToken":"0","TxnDate":"2026-09-21","NameOf":"Vendor",` +
	`"VendorRef":{"name":"CR-Test-Supplier","value":"3"},` +
	`"CustomerRef":{"name":"CR-Test-Customer-Edited","value":"1"},` +
	`"ItemRef":{"name":"Services","value":"1"},"TimeChargeId":"171",` +
	`"BillableStatus":"Billable","HourlyRate":120,"Hours":4,"Minutes":30,"Description":"QBCli v4 link test"}`

func TestTimeInvoiceWireShape(t *testing.T) {
	srv, muts := timeInvoiceServer(t, []string{taRow}, nil)
	res, err := ReplayTimeInvoice(context.Background(), "1", "")
	if err != nil {
		t.Fatalf("time-invoice: %v", err)
	}
	if res.Op != "create" || res.Entity != "Invoice" || res.Item.ID != "173" {
		t.Fatalf("result = %+v", res)
	}
	if *muts != 1 {
		t.Fatalf("expected 1 invoice mutation, got %d", *muts)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/v4/graphql") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	s := string(body)
	for _, want := range []string{
		`updateTransactions_Transaction`,
		`"transactionType":"SALE_INVOICE"`,
		`":-1"`,                  // draft sentinel id
		`"id":"ITEM_client_0"`,   // client line id
		`"id":"171"`,             // charge id
		`"type":"ACTIVITY_TIME"`, // source txn type
		`"sourceLine":{"amount":"540.00","sequence":"1","totalAmountConsumed":"0.00"}`,
		`"rateType":"HOURLY_RATE"`,
		`"quantity":"4.5"`,
		`"rate":"120"`,
		`"serviceDate":"2026-09-21"`,
		`"vendor":{"id":"3"}`,
		`"contact":{"id":"1"}`,
		`"txnTypeId":"4"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.1500s", want, s)
		}
	}
}

func TestTimeInvoiceSkipsConsumed(t *testing.T) {
	consumedRow := strings.Replace(taRow, `"Id":"1073741832"`, `"Id":"1073741831"`, 1)
	consumedRow = strings.Replace(consumedRow, `"TimeChargeId":"171"`, `"TimeChargeId":"162"`, 1)
	srv, muts := timeInvoiceServer(t, []string{consumedRow, taRow}, map[string]bool{"162": true})
	_, err := ReplayTimeInvoice(context.Background(), "1", "")
	if err != nil {
		t.Fatalf("time-invoice: %v", err)
	}
	if *muts != 1 {
		t.Fatalf("expected 1 invoice, got %d", *muts)
	}
	_, _, _, body := srv.snap()
	s := string(body)
	if strings.Contains(s, `"id":"162"`) {
		t.Fatalf("consumed charge 162 was invoiced: %.800s", s)
	}
	if !strings.Contains(s, `"id":"171"`) {
		t.Fatalf("fresh charge 171 missing: %.800s", s)
	}
}

func TestTimeInvoiceAllConsumed(t *testing.T) {
	timeInvoiceServer(t, []string{taRow}, map[string]bool{"171": true})
	_, err := ReplayTimeInvoice(context.Background(), "1", "")
	if err == nil || !strings.Contains(err.Error(), "already linked") {
		t.Fatalf("expected all-consumed error, got %v", err)
	}
}

func TestTimeInvoiceSweepGroupsByCustomer(t *testing.T) {
	other := strings.Replace(taRow, `"Id":"1073741832"`, `"Id":"1073741833"`, 1)
	other = strings.Replace(other, `"TimeChargeId":"171"`, `"TimeChargeId":"172"`, 1)
	other = strings.Replace(other, `"value":"1"}`, `"value":"7"}`, 1) // customer 7
	_, muts := timeInvoiceServer(t, []string{taRow, other}, nil)
	res, err := ReplayTimeInvoice(context.Background(), "", "")
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if *muts != 2 {
		t.Fatalf("expected 2 invoices (one per customer), got %d", *muts)
	}
	if !strings.Contains(res.Note, "2 invoice(s)") || !strings.Contains(res.Note, "2 time") {
		t.Fatalf("note = %q", res.Note)
	}
}

func TestTimeInvoiceValidation(t *testing.T) {
	timeInvoiceServer(t, []string{taRow}, nil)
	if _, err := ReplayTimeInvoice(context.Background(), "1", "true"); err == nil || !strings.Contains(err.Error(), "--auto") {
		t.Fatalf("--auto must fail loudly, got %v", err)
	}
	timeInvoiceServer(t, nil, nil)
	if _, err := ReplayTimeInvoice(context.Background(), "1", ""); err == nil || !strings.Contains(err.Error(), "no unbilled") {
		t.Fatalf("expected no-unbilled error, got %v", err)
	}
}
