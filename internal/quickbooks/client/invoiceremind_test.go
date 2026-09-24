package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The remind contract: v3 fetch of the invoice (customer/doc/email), then
// POST /api/neo/v1/company/<realm>/purchsales/send with txnTypeId 4, txnId,
// and an EMAIL deliveryInfo carrying subject+body.
func remindServer(t *testing.T) *domServer {
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
		case strings.Contains(r.URL.Path, "/purchsales/send"):
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			switch {
			case strings.Contains(q, "Invoice"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Invoice":[{"Id":"7","DocNumber":"INV-7",` +
					`"CustomerRef":{"name":"CR-Test-Customer-Edited","value":"1"},` +
					`"BillEmail":{"Address":"cust@example.com"}}]}}`))
			case strings.Contains(q, "CompanyInfo"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"CompanyInfo":[{"CompanyName":"Test Company 2"}]}}`))
			default:
				_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
			}
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestInvoiceRemindWireShape(t *testing.T) {
	srv := remindServer(t)
	res, err := ReplayInvoiceReminder(context.Background(), "7", "", "")
	if !errors.Is(err, ErrReminderOutcomeUnknown) || res != nil {
		t.Fatalf("empty acknowledgement must report unknown delivery: res=%+v err=%v", res, err)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/purchsales/send") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	s := string(body)
	for _, want := range []string{
		`"txnTypeId":4`,
		`"txnId":"7"`,
		`"deliveryType":"EMAIL"`,
		`"deliveryAddress":"cust@example.com"`,
		`"deliveryEmailSubject":"Reminder: Your payment to Test Company 2 is due"`,
		`invoice INV-7 has not been paid`,
		`Dear CR-Test-Customer-Edited`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.900s", want, s)
		}
	}
}

func TestInvoiceRemindValidation(t *testing.T) {
	remindServer(t)
	if _, err := ReplayInvoiceReminder(context.Background(), "", "", ""); err == nil || !strings.Contains(err.Error(), "--id") {
		t.Fatalf("missing id should fail, got %v", err)
	}
	if _, err := ReplayInvoiceReminder(context.Background(), "7", "true", ""); err == nil || !strings.Contains(err.Error(), "--automatic") {
		t.Fatalf("--automatic must fail loudly, got %v", err)
	}
	if _, err := ReplayInvoiceReminder(context.Background(), "7", "", "override@x.com"); !errors.Is(err, ErrReminderOutcomeUnknown) {
		t.Fatalf("explicit recipient: %v", err)
	}
}
