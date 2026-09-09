package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProjectCustomersOverviewEmpty(t *testing.T) {
	sum := []byte(`{"data":{"company":{"invoiceSummary":{"totalCount":0}}}}`)
	emptyTx := []byte(`{"data":{"company":{"transactions":{"edges":[]}}}}`)
	got := projectCustomersOverview(sum, emptyTx, emptyTx, 2)
	if got.Entity != "CustomersOverview" {
		t.Fatalf("entity=%q, want CustomersOverview (not Customer)", got.Entity)
	}
	if len(got.Items) != 0 {
		t.Fatalf("items=%+v, want empty", got.Items)
	}
	if got.Counts["totalCount"] != 0 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if strings.Contains(got.Note, "Customer") && !strings.Contains(got.Note, "invoiceSummary") {
		t.Fatalf("note leaked Customer stand-in: %q", got.Note)
	}
}

func TestProjectCustomersOverviewDoesNotReturnCustomerList(t *testing.T) {
	sum := []byte(`{"data":{"company":{"invoiceSummary":{"totalCount":5}}}}`)
	est := []byte(`{"data":{"company":{"transactions":{"edges":[{"node":{"id":"e1","type":"SALE_ESTIMATE","header":{"txnStatus":"OPEN","transactionType":"SALE_ESTIMATE"}}}]}}}}`)
	inv := []byte(`{"data":{"company":{"transactions":{"edges":[{"node":{"id":"i1","type":"SALE_INVOICE","header":{"txnStatus":"OPEN","transactionType":"SALE_INVOICE"}}}]}}}}`)
	got := projectCustomersOverview(sum, est, inv, 2)
	if got.Entity == "Customer" {
		t.Fatal("must not map to Customer")
	}
	if got.Entity != "CustomersOverview" {
		t.Fatalf("entity=%q", got.Entity)
	}
	if got.Counts["totalCount"] != 5 || got.Counts["openEstimates"] != 1 || got.Counts["openInvoices"] != 1 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items=%+v", got.Items)
	}
	for _, it := range got.Items {
		if it.Type == "Customer" || it.Name == "Customer" {
			t.Fatalf("projected Customer row: %+v", it)
		}
	}
	if !strings.Contains(got.Note, "invoiceSummary") || !strings.Contains(got.Note, "totalCount=5") {
		t.Fatalf("note=%q", got.Note)
	}
}

func TestProjectCustomersOverviewRespectsLimit(t *testing.T) {
	sum := []byte(`{"data":{"company":{"invoiceSummary":{"totalCount":5}}}}`)
	est := []byte(`{"data":{"company":{"transactions":{"edges":[{"node":{"id":"e1","type":"SALE_ESTIMATE","header":{"txnStatus":"OPEN"}}},{"node":{"id":"e2","type":"SALE_ESTIMATE","header":{"txnStatus":"OPEN"}}}]}}}}`)
	inv := []byte(`{"data":{"company":{"transactions":{"edges":[{"node":{"id":"i1","type":"SALE_INVOICE","header":{"txnStatus":"OPEN"}}}]}}}}`)
	got := projectCustomersOverview(sum, est, inv, 2)
	if len(got.Items) != 2 {
		t.Fatalf("limit 2 should cap items: %+v", got.Items)
	}
}

func TestReplayQueryCustomersOverviewNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayQuery(t.Context(), "CustomersOverview", "", "", 2)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestPlannedQueryURLCustomersOverview(t *testing.T) {
	got := PlannedQueryURL("CustomersOverview")
	if got != customersOverviewGraphQLURL {
		t.Fatalf("PlannedQueryURL(CustomersOverview)=%q", got)
	}
	cust := PlannedQueryURL("Customer")
	if !strings.Contains(cust, "/query?") || strings.Contains(cust, "v4/graphql") {
		t.Fatalf("Customer planned URL must stay v3: %s", cust)
	}
}

func TestCustomersOverviewPayloadMatchesCapture(t *testing.T) {
	if !strings.Contains(invoiceSummaryQuery, "invoiceSummary") {
		t.Fatal("captured invoiceSummary query missing")
	}
	if !strings.Contains(getTransactionsQuery, "getTransactions") {
		t.Fatal("captured getTransactions query missing")
	}
	if !strings.Contains(getTransactionsEstimateWith, "txnTypeFilter='estimate'") {
		t.Fatal("estimate with-clause missing")
	}
	if !strings.Contains(getTransactionsInvoiceWith, "txnTypeFilter='invoice'") {
		t.Fatal("invoice with-clause missing")
	}
	payload, err := json.Marshal(map[string]any{
		"query":     invoiceSummaryQuery,
		"variables": map[string]any{"orderBy": "status asc", "filterBy": invoiceSummaryFilterBy, "offset": 0, "limit": 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m["query"].(string), "invoiceSummary") {
		t.Fatal(m["query"])
	}
}
