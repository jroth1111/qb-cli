package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// poFixture is a two-line purchase order in v3 shape.
const poPartialFixture = `{"PurchaseOrder":{
  "Id":"135","SyncToken":"2","DocNumber":"PO-1",
  "VendorRef":{"value":"3","name":"CR-Test-Supplier"},
  "Line":[
    {"Id":"1","DetailType":"ItemBasedExpenseLineDetail","Amount":50,
     "ItemBasedExpenseLineDetail":{"ItemRef":{"value":"3"},"Qty":5,"UnitPrice":10}},
    {"Id":"2","DetailType":"ItemBasedExpenseLineDetail","Amount":20,
     "ItemBasedExpenseLineDetail":{"ItemRef":{"value":"4"},"Qty":2,"UnitPrice":10}}
  ]}}`

// poPartialServer answers the v3 PO fetch and the warehouse GraphQL post.
func poPartialServer(t *testing.T, gqlByOp map[string]string) (*domServer, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/purchaseorder/") {
			_, _ = w.Write([]byte(poPartialFixture))
			return
		}
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		bodies = append(bodies, m)
		op, _ := m["operationName"].(string)
		body, ok := gqlByOp[op]
		if !ok {
			t.Fatalf("unexpected operationName %q", op)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s, &bodies
}

func TestPurchaseOrderPartialBuildsReceiveInput(t *testing.T) {
	saveUsableURIHost(t)
	srv, bodies := poPartialServer(t, map[string]string{
		"CommerceReceiveInventory": `{"data":{"commerceReceiveInventory":{"id":"7","movementType":"EXPECT","version":0,"sourceTransaction":{"txnId":"135","txnType":"PURCHASE_ORDER","version":2},"lines":[],"__typename":"Commerce_InventoryMovement"}}}`,
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayPurchaseOrderPartial(context.Background(), map[string]string{
		"id":    "135",
		"items": `[{"item-id":"3","qty":2}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "7" || res.Entity != "InventoryMovement" {
		t.Fatalf("res = %+v", res)
	}
	vars := (*bodies)[0]["variables"].(map[string]any)
	input := vars["input"].(map[string]any)
	src := input["sourceTransaction"].(map[string]any)
	if src["txnId"] != "135" || src["txnType"] != "PURCHASE_ORDER" || src["version"] != 2.0 {
		t.Fatalf("sourceTransaction = %v", src)
	}
	if src["vendorId"] != "3" {
		t.Fatalf("vendorId = %v", src["vendorId"])
	}
	lines := input["lines"].([]any)
	if len(lines) != 1 {
		t.Fatalf("lines = %v", lines)
	}
	line := lines[0].(map[string]any)
	if line["itemId"] != "3" || line["quantity"] != 2.0 ||
		line["sourceTransactionLineId"] != "1" || line["lineOrder"] != 1.0 {
		t.Fatalf("line = %v", line)
	}
}

func TestPurchaseOrderPartialLineIDMatch(t *testing.T) {
	saveUsableURIHost(t)
	srv, bodies := poPartialServer(t, map[string]string{
		"CommerceReceiveInventory": `{"data":{"commerceReceiveInventory":{"id":"8","movementType":"EXPECT","version":0,"__typename":"Commerce_InventoryMovement"}}}`,
	})
	interceptHTTP(t, srv.URL)
	_, err := ReplayPurchaseOrderPartial(context.Background(), map[string]string{
		"id":    "135",
		"items": `[{"line-id":"2","qty":1}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	vars := (*bodies)[0]["variables"].(map[string]any)
	line := vars["input"].(map[string]any)["lines"].([]any)[0].(map[string]any)
	if line["itemId"] != "4" || line["sourceTransactionLineId"] != "2" || line["lineOrder"] != 2.0 {
		t.Fatalf("line = %v", line)
	}
}

func TestPurchaseOrderPartialRequiresFields(t *testing.T) {
	if _, err := ReplayPurchaseOrderPartial(context.Background(), map[string]string{}); err == nil {
		t.Fatal("expected missing id error")
	}
	if _, err := ReplayPurchaseOrderPartial(context.Background(), map[string]string{"id": "135"}); err == nil {
		t.Fatal("expected missing items error")
	}
	if _, err := ReplayPurchaseOrderPartial(context.Background(), map[string]string{
		"id": "135", "items": `[{"item-id":"3","qty":0}]`,
	}); err == nil {
		t.Fatal("expected non-positive qty error")
	}
}

func TestPurchaseOrderPartialUnknownLine(t *testing.T) {
	saveUsableURIHost(t)
	srv, _ := poPartialServer(t, map[string]string{})
	interceptHTTP(t, srv.URL)
	_, err := ReplayPurchaseOrderPartial(context.Background(), map[string]string{
		"id":    "135",
		"items": `[{"item-id":"999","qty":1}]`,
	})
	if err == nil || !strings.Contains(err.Error(), "matches no item line") {
		t.Fatalf("err = %v", err)
	}
}
