package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// entityServer returns v3-style {Entity: {...}} envelopes keyed on the last
// two path segments — /payment/7 -> {"Payment": {...}}.
func entityServer(t *testing.T, bodies map[string]map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		key := segs[len(segs)-2] + "/" + segs[len(segs)-1]
		row, ok := bodies[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		entity := ""
		for name, path := range v3Path {
			if path == segs[len(segs)-2] {
				entity = name
				break
			}
		}
		if entity == "" {
			t.Errorf("unknown v3 entity path %s", segs[len(segs)-2])
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"%s": %s}`, entity, mustJSON(t, row))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --payments must become LinkedTxn Payment lines carrying each payment's
// fetched TotalAmt — previously the flag was silently ignored and the
// deposit posted as an undeposited-funds-free manual line.
func TestDepositPaymentsBecomeLinkedTxn(t *testing.T) {
	saveUsable(t)
	srv := entityServer(t, map[string]map[string]any{
		"payment/11": {"Id": "11", "TotalAmt": 120.50},
		"payment/12": {"Id": "12", "TotalAmt": 30.25},
	})
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"DepositToAccountRef": map[string]any{"value": "4"}}
	got, err := linkDepositPayments(context.Background(), ac, body, map[string]string{"payments": "11,12"})
	if err != nil {
		t.Fatal(err)
	}
	lines, ok := got["Line"].([]map[string]any)
	if !ok || len(lines) != 2 {
		t.Fatalf("expected 2 linked payment lines: %v", got["Line"])
	}
	first := lines[0]["LinkedTxn"].([]map[string]any)[0]
	if first["TxnId"] != "11" || first["TxnType"] != "Payment" {
		t.Fatalf("LinkedTxn missing: %v", first)
	}
	if lines[0]["Amount"] != 120.5 || lines[1]["Amount"] != 30.25 {
		t.Fatalf("payment amounts not recovered: %v", lines)
	}
	// Unknown payment id must fail the whole create, not post a partial link.
	if _, err := linkDepositPayments(context.Background(), ac, map[string]any{}, map[string]string{"payments": "99"}); err == nil {
		t.Fatal("unknown payment id silently skipped")
	}
}

// An expense on a credit-card account with no --payment-type must infer
// CreditCard (that omission produced Cash-on-card docs); an explicit
// mismatched type must be refused.
func TestFundingTypeInferredAndVerified(t *testing.T) {
	saveUsable(t)
	srv := entityServer(t, map[string]map[string]any{
		"account/204": {"Id": "204", "AccountType": "Credit Card"},
		"account/93":  {"Id": "93", "AccountType": "Bank"},
	})
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	// Omitted type on a card account -> inferred CreditCard.
	body := map[string]any{"AccountRef": map[string]any{"value": "204"}, "PaymentType": "Cash"}
	got, err := reconcileFundingType(context.Background(), ac, body, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if got["PaymentType"] != "CreditCard" {
		t.Fatalf("card funding account left PaymentType=%v", got["PaymentType"])
	}
	// Explicit Cash on a card account -> refused.
	_, err = reconcileFundingType(context.Background(), ac,
		map[string]any{"AccountRef": map[string]any{"value": "204"}, "PaymentType": "Cash"},
		map[string]string{"payment-type": "Cash"})
	if err == nil {
		t.Fatal("explicit Cash on a credit-card account accepted")
	}
	// Explicit CreditCard on a bank account -> refused.
	_, err = reconcileFundingType(context.Background(), ac,
		map[string]any{"AccountRef": map[string]any{"value": "93"}, "PaymentType": "CreditCard"},
		map[string]string{"payment-type": "CreditCard"})
	if err == nil {
		t.Fatal("explicit CreditCard on a bank account accepted")
	}
	// Consistent pair passes untouched.
	got, err = reconcileFundingType(context.Background(), ac,
		map[string]any{"AccountRef": map[string]any{"value": "93"}, "PaymentType": "Check"},
		map[string]string{"payment-type": "Check"})
	if err != nil || got["PaymentType"] != "Check" {
		t.Fatalf("consistent bank+Check rejected: %v", err)
	}
}
