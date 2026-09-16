package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

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
