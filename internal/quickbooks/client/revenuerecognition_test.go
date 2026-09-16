package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProjectDeferredRecognitionLinesEmpty(t *testing.T) {
	body := []byte(`{"data":{"accountingDeferredRecognitionTransactionLineDetails":{"pageInfo":{"hasNextPage":false,"hasPreviousPage":false,"startCursor":null,"endCursor":null,"__typename":"Common_PageInfo"},"edges":[],"__typename":"Accounting_DeferredRecognitionTransactionLineConnection"}}}`)
	got := projectDeferredRecognitionLines(body)
	if got.Entity != "RevenueRecognition" {
		t.Fatalf("entity=%q, want RevenueRecognition (not Invoice)", got.Entity)
	}
	if len(got.Items) != 0 {
		t.Fatalf("items=%+v, want empty (AU UI 0 deferred lines)", got.Items)
	}
	if got.Counts["edges"] != 0 || got.Counts["items"] != 0 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if !strings.Contains(got.Note, "sbseggraphqlorch") || !strings.Contains(got.Note, "edges=0") {
		t.Fatalf("note=%q", got.Note)
	}
	if strings.Contains(got.Note, "Invoice") {
		t.Fatalf("note leaked Invoice stand-in: %q", got.Note)
	}
}

func TestProjectDeferredRecognitionLinesDoesNotInventInvoiceIDs(t *testing.T) {
	body := []byte(`{"data":{"accountingDeferredRecognitionTransactionLineDetails":{"edges":[{"node":{"id":"d1","docNum":"RR-1","postingStatus":"POSTED","serviceStartDate":"2026-07-01","remainingAmount":{"value":12.5,"currency":"AUD"},"item":{"id":"9"},"sourceDetail":{"transaction":{"id":"tx1","type":"INVOICE"}}}}], "__typename":"Accounting_DeferredRecognitionTransactionLineConnection"}}}`)
	got := projectDeferredRecognitionLines(body)
	if got.Entity == "Invoice" {
		t.Fatal("must not map to Invoice")
	}
	for _, it := range got.Items {
		if it.ID == "115" || it.ID == "87" {
			t.Fatalf("invented Invoice leftover id: %+v", it)
		}
	}
	if got.Counts["edges"] != 1 || got.Counts["items"] != 1 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "d1" || got.Items[0].DocNumber != "RR-1" {
		t.Fatalf("items=%+v", got.Items)
	}
}

func TestProjectDeferredRecognitionLinesGQLError(t *testing.T) {
	body := []byte(`{"errors":[{"message":"Account setting does not exist"}],"data":{"accountingDeferredRecognitionTransactionLineDetails":null}}`)
	got := projectDeferredRecognitionLines(body)
	if got.Entity != "RevenueRecognition" || len(got.Items) != 0 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Note, "gql:") {
		t.Fatalf("note=%q", got.Note)
	}
}

// Live Common_MoneyAmountV2 serialises value as a string ("120.00") and
// scheduleLines/template can be null — a float64 field used to fail the
// whole decode and report "(unparsed)" with items=0.
func TestProjectDeferredRecognitionLinesStringMoneyValue(t *testing.T) {
	body := []byte(`{"data":{"accountingDeferredRecognitionTransactionLineDetails":{"edges":[{"node":{"id":"133","docNum":null,"postingStatus":"INPROGRESS","serviceStartDate":"2026-09-16","remainingAmount":{"value":"120.00","currency":"AUD"},"item":{"id":"1"},"scheduleLines":null,"template":null,"sourceDetail":{"transaction":{"id":"133","type":"INVOICE"}}}}], "__typename":"Accounting_DeferredRecognitionTransactionLineConnection"}}}`)
	got := projectDeferredRecognitionLines(body)
	if strings.Contains(got.Note, "unparsed") {
		t.Fatalf("string money value must not fail decode: %q", got.Note)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "133" || got.Items[0].Amount != 120 {
		t.Fatalf("items=%+v", got.Items)
	}
}

func TestReplayQueryRevenueRecognitionNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayQuery(t.Context(), "RevenueRecognition", "", "", 2)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestPlannedQueryURLRevenueRecognition(t *testing.T) {
	got := PlannedQueryURL("RevenueRecognition")
	if got != revenueRecognitionGraphQLURL {
		t.Fatalf("PlannedQueryURL(RevenueRecognition)=%q, want sbseggraphqlorch graphql", got)
	}
	inv := PlannedQueryURL("Invoice")
	if !strings.Contains(inv, "/query?") || strings.Contains(inv, "sbseggraphqlorch") {
		t.Fatalf("Invoice planned URL must stay v3: %s", inv)
	}
}

func TestDeferredRecognitionPayloadMatchesCapture(t *testing.T) {
	if !strings.Contains(accountingDeferredTransactionLineDetailsQuery, "accountingDeferredRecognitionTransactionLineDetails") {
		t.Fatal("captured deferred-line query missing")
	}
	if !strings.Contains(accountingDeferredTransactionLineDetailsQuery, "edges") {
		t.Fatal("captured query must request edges")
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "accountingDeferredTransactionLineDetailsOp",
		"variables":     map[string]any{"first": 10, "filter": map[string]any{}},
		"query":         accountingDeferredTransactionLineDetailsQuery,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	if m["operationName"] != "accountingDeferredTransactionLineDetailsOp" {
		t.Fatalf("%v", m["operationName"])
	}
}
