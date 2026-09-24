package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The currency suite exercises the sbseggraphqlorch surface: a version read
// (GetBusinessTransactionExchangeRates) precedes the rate write
// (businessTransactionUpdateCurrencyExchangeRate), and delete rides
// businessTransactionDeletePreferredForeignCurrency — dispatch keys off the
// query text.

func currencyMutServer(t *testing.T) *domServer {
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
		case strings.Contains(string(b), "GetBusinessTransactionExchangeRates"):
			_, _ = w.Write([]byte(`{"data":{"businessTransactionCurrencyExchangeRates":{"edges":[
				{"node":{"fromCurrency":{"currencyCode":"USD"},"entityVersion":3}},
				{"node":{"fromCurrency":{"currencyCode":"GBP"},"entityVersion":1}}
			]}}}`))
		case strings.Contains(string(b), "businessTransactionUpdateCurrencyExchangeRate"):
			currencyCode := "USD"
			if strings.Contains(string(b), `"currencyCode":"NZD"`) {
				currencyCode = "NZD"
			}
			_, _ = w.Write([]byte(`{"data":{"businessTransactionUpdateCurrencyExchangeRate":{
				"currencyExchangeRate":{"exchangeRate":1.405185,"entityVersion":4,
					"fromCurrency":{"currencyCode":"` + currencyCode + `"}}}}}`))
		case strings.Contains(string(b), "businessTransactionDeletePreferredForeignCurrency"):
			_, _ = w.Write([]byte(`{"data":{"businessTransactionDeletePreferredForeignCurrency":{
				"currencyCode":"NZD"}}}`))
		default:
			_, _ = w.Write([]byte(`{"errors":[{"message":"unmatched query"}]}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestCurrencyRateSetSendsCapturedPayload(t *testing.T) {
	srv := currencyMutServer(t)
	res, err := ReplayCurrencyRateSet(context.Background(), "usd", "1.405185", "18/09/2026")
	if err != nil {
		t.Fatalf("rate set: %v", err)
	}
	if res.Op != "update" || res.Item.ID != "USD" || res.Item.Name != "USD 1.405185" {
		t.Fatalf("result = %+v", res)
	}
	vars := srv.sentMap()["variables"].(map[string]any)
	in := vars["input"].(map[string]any)
	if in["currencyCode"] != "USD" || in["exchangeRate"] != "1.405185" {
		t.Fatalf("input = %v", in)
	}
	if in["asOfDate"] != "2026-09-18" {
		t.Fatalf("asOfDate = %v", in["asOfDate"])
	}
	// repeat writes must carry the live entityVersion for the currency
	if in["entityVersion"] != float64(3) {
		t.Fatalf("entityVersion = %v", in["entityVersion"])
	}
}

func TestCurrencyRateSetVersionZeroForNewRow(t *testing.T) {
	srv := currencyMutServer(t)
	if _, err := ReplayCurrencyRateSet(context.Background(), "NZD", "0.8", "2026-09-18"); err != nil {
		t.Fatalf("rate set: %v", err)
	}
	in := srv.sentMap()["variables"].(map[string]any)["input"].(map[string]any)
	if in["entityVersion"] != float64(0) {
		t.Fatalf("entityVersion = %v", in["entityVersion"])
	}
	if in["asOfDate"] != "2026-09-18" {
		t.Fatalf("asOfDate passthrough = %v", in["asOfDate"])
	}
}

func TestCurrencyRateSetDefaultsDateToToday(t *testing.T) {
	srv := currencyMutServer(t)
	if _, err := ReplayCurrencyRateSet(context.Background(), "USD", "1.4", ""); err != nil {
		t.Fatalf("rate set: %v", err)
	}
	in := srv.sentMap()["variables"].(map[string]any)["input"].(map[string]any)
	asOf, _ := in["asOfDate"].(string)
	if len(asOf) != 10 || asOf[4] != '-' || asOf[7] != '-' {
		t.Fatalf("asOfDate = %q, want ISO yyyy-mm-dd", asOf)
	}
}

func TestCurrencyRateSetValidation(t *testing.T) {
	if _, err := ReplayCurrencyRateSet(context.Background(), "US", "1.4", ""); err == nil {
		t.Fatal("expected error for 2-letter code")
	}
	if _, err := ReplayCurrencyRateSet(context.Background(), "USD", "", ""); err == nil {
		t.Fatal("expected error for empty rate")
	}
}

func TestCurrencyRateSetSurfacesGraphQLError(t *testing.T) {
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
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(b), "GetBusinessTransactionExchangeRates") {
			_, _ = w.Write([]byte(`{"data":{"businessTransactionCurrencyExchangeRates":{"edges":[]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"errors":[{"message":"Validation failed"}]}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	if _, err := ReplayCurrencyRateSet(context.Background(), "USD", "9.9", ""); err == nil ||
		!strings.Contains(err.Error(), "Validation failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestCurrencyRateSetNon200(t *testing.T) {
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
		s.lastBody = b
		s.mu.Unlock()
		if strings.Contains(string(b), "GetBusinessTransactionExchangeRates") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"businessTransactionCurrencyExchangeRates":{"edges":[]}}}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	_, err := ReplayCurrencyRateSet(context.Background(), "USD", "1.4", "")
	var re *ReplayError
	if err == nil || !errors.As(err, &re) || re.Status != http.StatusBadGateway {
		t.Fatalf("err = %v", err)
	}
}

func TestCurrencyDeleteSendsCode(t *testing.T) {
	srv := currencyMutServer(t)
	res, err := ReplayCurrencyDelete(context.Background(), "nzd")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Op != "delete" || res.Item.ID != "NZD" {
		t.Fatalf("result = %+v", res)
	}
	in := srv.sentMap()["variables"].(map[string]any)["input"].(map[string]any)
	if in["currencyCode"] != "NZD" {
		t.Fatalf("input = %v", in)
	}
	if srv.calls != 1 {
		t.Fatalf("calls = %d, want 1 (delete needs no version read)", srv.calls)
	}
}

func TestCurrencyDeleteValidation(t *testing.T) {
	if _, err := ReplayCurrencyDelete(context.Background(), "  "); err == nil {
		t.Fatal("expected error for empty code")
	}
	if _, err := ReplayCurrencyDelete(context.Background(), "TOOLONG"); err == nil {
		t.Fatal("expected error for non-3-letter code")
	}
}

func TestCurrencyDeleteSurfacesGraphQLError(t *testing.T) {
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"currency has attached transactions"}]}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	if _, err := ReplayCurrencyDelete(context.Background(), "USD"); err == nil ||
		!strings.Contains(err.Error(), "attached transactions") {
		t.Fatalf("err = %v", err)
	}
}

func TestCurrencyDeleteRejectsMismatchedReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"businessTransactionDeletePreferredForeignCurrency":{"currencyCode":"AUD"}}}`)
	interceptHTTP(t, srv.URL)
	_, err := ReplayCurrencyDelete(context.Background(), "USD")
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}
