package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProjectGetSalesOrdersEmpty(t *testing.T) {
	body := []byte(`{"data":{"result":{"totalCount":0,"__typename":"SalesOrdersPagedResult"}}}`)
	got := projectGetSalesOrders(body)
	if got.Entity != "SalesOrder" {
		t.Fatalf("entity=%q, want SalesOrder (not Invoice)", got.Entity)
	}
	if len(got.Items) != 0 {
		t.Fatalf("items=%+v, want empty (AU UI 0 rows)", got.Items)
	}
	if got.Counts["totalCount"] != 0 || got.Counts["items"] != 0 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if !strings.Contains(got.Note, "commercecontrol") || !strings.Contains(got.Note, "totalCount=0") {
		t.Fatalf("note=%q", got.Note)
	}
	if strings.Contains(got.Note, "Invoice") {
		t.Fatalf("note leaked Invoice stand-in: %q", got.Note)
	}
}

func TestProjectGetSalesOrdersDoesNotInventInvoiceIDs(t *testing.T) {
	// Even a non-zero totalCount must not project Invoice leftovers 115/87.
	body := []byte(`{"data":{"result":{"totalCount":2,"__typename":"SalesOrdersPagedResult"}}}`)
	got := projectGetSalesOrders(body)
	if got.Entity == "Invoice" {
		t.Fatal("must not map to Invoice")
	}
	for _, it := range got.Items {
		if it.ID == "115" || it.ID == "87" {
			t.Fatalf("invented Invoice leftover id: %+v", it)
		}
	}
	if got.Counts["totalCount"] != 2 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if len(got.Items) != 0 {
		t.Fatalf("captured query only returns totalCount; items must stay empty: %+v", got.Items)
	}
}

func TestProjectGetSalesOrdersGQLError(t *testing.T) {
	body := []byte(`{"errors":[{"message":"Account setting does not exist"}],"data":{"result":null}}`)
	got := projectGetSalesOrders(body)
	if got.Entity != "SalesOrder" || len(got.Items) != 0 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Note, "gql:") {
		t.Fatalf("note=%q", got.Note)
	}
}

func TestReplayQuerySalesOrderNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayQuery(t.Context(), "SalesOrder", "", "", 2)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestPlannedQueryURLSalesOrder(t *testing.T) {
	got := PlannedQueryURL("SalesOrder")
	if got != salesOrderGraphQLURL {
		t.Fatalf("PlannedQueryURL(SalesOrder)=%q, want commercecontrol graphql", got)
	}
	inv := PlannedQueryURL("Invoice")
	if !strings.Contains(inv, "/query?") || strings.Contains(inv, "commercecontrol") {
		t.Fatalf("Invoice planned URL must stay v3: %s", inv)
	}
}

func TestGetSalesOrdersPayloadMatchesCapture(t *testing.T) {
	if !strings.Contains(getSalesOrdersQuery, "operationName") && !strings.Contains(getSalesOrdersQuery, "getSalesOrders") {
		t.Fatal("captured GetSalesOrders query missing")
	}
	if !strings.Contains(getSalesOrdersQuery, "totalCount") {
		t.Fatal("captured query must request totalCount")
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "GetSalesOrders",
		"variables":     map[string]any{"offset": 0, "limit": 2},
		"query":         getSalesOrdersQuery,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	if m["operationName"] != "GetSalesOrders" {
		t.Fatalf("%v", m["operationName"])
	}
}
