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

// itemReceiptServer answers warehouse GraphQL posts by operationName.
func itemReceiptServer(t *testing.T, byOp map[string]string) (*domServer, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		bodies = append(bodies, m)
		op, _ := m["operationName"].(string)
		body, ok := byOp[op]
		if !ok {
			t.Fatalf("unexpected operationName %q", op)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s, &bodies
}

const irSingleFetchResponse = `{"data":{"warehouseManagementItemReceipt":{
  "id":"1823000002","txnDate":"2026-09-16T00:00:00.000Z","referenceNo":"1002",
  "status":"RECEIVED","totalAmount":30.0,"memo":"","version":0,
  "vendor":{"id":"3","name":"CR-Test-Supplier","__typename":"x"},
  "currency":{"code":"AUD","currencyCode":"AUD","exchangeRate":1.0,"__typename":"x"},
  "lines":[{"id":"1","item":{"id":"3","__typename":"x"},"status":"RECEIVED",
    "lineOrder":1,"expectedQuantity":0,"receivedQuantity":3,"rejectedQuantity":0,
    "description":"","cost":10,"amount":30,
    "sourceLinks":[],"targetLinks":[],
    "customExtensions":{"dimensions":[],"__typename":"x"},"__typename":"x"}],
  "__typename":"x"}}}`

func TestItemReceiptGetByIDRoutesSingleFetch(t *testing.T) {
	saveUsableURIHost(t)
	srv, bodies := itemReceiptServer(t, map[string]string{
		"WarehouseManagementItemReceipt": irSingleFetchResponse,
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(context.Background(), "ItemReceipt", "1823000002", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 1 {
		t.Fatalf("res = %+v", res)
	}
	it := res.Items[0]
	if it.ID != "1823000002" || it.DocNumber != "1002" || it.Account != "CR-Test-Supplier" {
		t.Fatalf("item = %+v", it)
	}
	if got := (*bodies)[0]["operationName"]; got != "WarehouseManagementItemReceipt" {
		t.Fatalf("operationName = %v", got)
	}
}

func TestItemReceiptCreateInputShape(t *testing.T) {
	saveUsableURIHost(t)
	srv, bodies := itemReceiptServer(t, map[string]string{
		"CreateItemReceipt": `{"data":{"warehouseManagementCreateItemReceipt":{"itemReceipt":{"id":"9","referenceNo":"55","status":"RECEIVED","totalAmount":20,"version":0,"vendor":{"id":"3","name":"V"},"__typename":"x"},"__typename":"WarehouseManagement_CreateItemReceiptPayload"}}}`,
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayItemReceiptCreate(context.Background(), map[string]string{
		"vendor-id": "3", "item-id": "3", "qty": "2", "rate": "10",
		"ref-no": "55", "memo": "m", "date": "16/09/2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "9" || res.Op != "create" || res.Entity != "ItemReceipt" {
		t.Fatalf("res = %+v", res)
	}
	vars := (*bodies)[0]["variables"].(map[string]any)
	input := vars["input"].(map[string]any)
	if input["vendorId"] != "3" || input["referenceNo"] != "55" || input["memo"] != "m" {
		t.Fatalf("input = %v", input)
	}
	if input["txnDate"] != "2026-09-16T00:00:00.000Z" {
		t.Fatalf("txnDate = %v", input["txnDate"])
	}
	if input["totalAmount"] != 20.0 {
		t.Fatalf("totalAmount = %v", input["totalAmount"])
	}
	lines := input["lines"].([]any)
	line := lines[0].(map[string]any)
	if line["itemId"] != "3" || line["receivedQuantity"] != 2.0 || line["cost"] != 10.0 || line["amount"] != 20.0 {
		t.Fatalf("line = %v", line)
	}
	if _, hasCur := input["currency"]; hasCur {
		t.Fatalf("currency should be omitted by default: %v", input["currency"])
	}
}

func TestItemReceiptCreateRequiresFields(t *testing.T) {
	if _, err := ReplayItemReceiptCreate(context.Background(), map[string]string{}); err == nil {
		t.Fatal("expected vendor/item error")
	}
	if _, err := ReplayItemReceiptCreate(context.Background(), map[string]string{"vendor-id": "3", "item-id": "3"}); err == nil {
		t.Fatal("expected qty/rate error")
	}
}

func TestItemReceiptUpdateFetchesThenMutates(t *testing.T) {
	saveUsableURIHost(t)
	srv, bodies := itemReceiptServer(t, map[string]string{
		"WarehouseManagementItemReceipt": irSingleFetchResponse,
		"UpdateItemReceipt":              `{"data":{"warehouseManagementUpdateItemReceipt":{"itemReceipt":{"id":"1823000002","referenceNo":"1002","status":"RECEIVED","totalAmount":40,"version":1,"vendor":{"id":"3","name":"V"},"__typename":"x"},"__typename":"WarehouseManagement_UpdateItemReceiptPayload"}}}`,
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayItemReceiptUpdate(context.Background(), map[string]string{
		"id": "1823000002", "qty": "4", "memo": "edited",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "182300002" && res.Item.ID != "1823000002" {
		t.Fatalf("res = %+v", res)
	}
	if len(*bodies) != 2 {
		t.Fatalf("expected fetch+update, got %d calls", len(*bodies))
	}
	vars := (*bodies)[1]["variables"].(map[string]any)
	input := vars["input"].(map[string]any)
	if input["id"] != "1823000002" || input["memo"] != "edited" {
		t.Fatalf("input = %v", input)
	}
	if input["vendorId"] != "3" {
		t.Fatalf("vendorId = %v", input["vendorId"])
	}
	lines := input["lines"].([]any)
	line := lines[0].(map[string]any)
	if line["id"] != "1" || line["itemId"] != "3" || line["receivedQuantity"] != 4.0 || line["amount"] != 40.0 {
		t.Fatalf("line = %v", line)
	}
	if input["totalAmount"] != 40.0 {
		t.Fatalf("totalAmount = %v", input["totalAmount"])
	}
}

func TestItemReceiptUpdateNotFound(t *testing.T) {
	saveUsableURIHost(t)
	srv, _ := itemReceiptServer(t, map[string]string{
		"WarehouseManagementItemReceipt": `{"data":{"warehouseManagementItemReceipt":null}}`,
	})
	interceptHTTP(t, srv.URL)
	_, err := ReplayItemReceiptUpdate(context.Background(), map[string]string{"id": "999"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}
