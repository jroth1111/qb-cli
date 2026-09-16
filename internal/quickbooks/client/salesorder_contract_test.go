package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// Synthetic identifiers and the native field shapes from the UI captures.
const soContractGet = `{"data":{"result":{
 "id":"1490000001","version":1,"orderNumber":"FIXTURE-SO","orderStatus":"OPEN",
 "subtotal":1,"total":1,"totalTax":0,"totalDiscount":0,"totalShipping":0,
 "issuedAt":"2026-09-09T14:00:00Z","transactionDate":"2026-09-10","dueDate":"2026-09-20","term":"1000000004",
 "customerId":"10","customerName":"Fixture Customer","customerNote":"fixture sales order",
 "freeformBillingAddress":"Fixture billing address","freeformShippingAddress":"Fixture shipping address",
 "printTemplateId":"1000000002","currencyIso":"AUD",
 "currencyInfo":{"symbol":"A$","homeAmount":1,"code":"AUD","exchangeRate":1,"name":"Australian Dollar","currency":"AUD"},
 "attachments":[],"discount":{"amount":0,"percent":null,"isDiscountAfterTax":null},
 "salesOrderLineItems":[{"id":"638554","variantId":"7","description":"fixture sales order","position":0,
 "quantity":1,"total":1,"taxable":false,"type":"SERVICE","unitPrice":1,
 "baseQuantity":1,"baseRate":1,"unitConversionRatio":1,"totalDiscount":0,"totalTax":0,"taxGroup":{"id":null},
 "parentItemId":null,"parentLineItemId":null,"isDeleted":false,"quantityConsumedInPurchaseOrders":0}]
}}}`
const soContractLine = `[{"DetailType":"SalesItemLineDetail","Amount":1,"Description":"fixture sales order","SalesItemLineDetail":{"ItemRef":{"value":"7"},"Qty":1,"UnitPrice":1}}]`
const soContractUpdateLine = `[{"DetailType":"SalesItemLineDetail","Amount":2,"SalesItemLineDetail":{"ItemRef":{"value":"7"},"Qty":2,"UnitPrice":1}}]`

func soContractFlags() map[string]string {
	return map[string]string{"customer": "10", "date": "10/09/2026", "due-date": "20/09/2026", "term": "1000000004", "currency": "AUD", "print-template-id": "1000000002", "order-number": "FIXTURE-SO", "memo": "fixture sales order", "line-items": soContractLine}
}

type soContractRequest struct {
	OperationName string         `json:"operationName"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
}
type soContractTransport func(*http.Request) (*http.Response, error)

func (f soContractTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func soContractResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

// Replace the final impersonation boundary, leaving doURIHost's real header
// selection active. Unexpected calls fail locally instead of reaching any API.
func soContractMock(t *testing.T, fn func(*http.Request, soContractRequest) (int, string)) {
	t.Helper()
	saveUsable(t)
	tok, err := auth.Load()
	if err != nil {
		t.Fatal(err)
	}
	tok.URIHostHeaders = map[string]map[string]string{salesOrderHost: {"Authorization": "Intuit_APIKey intuit_apikey=service-fixture,intuit_apikey_version=1.0", "X-Contract": "native-service", "X-Supergraph-Version": "2"}}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
	oldURI, oldTransport := uriHostTransport, impersonatedTransport
	uriHostTransport = nil
	impersonatedTransport = soContractTransport(func(r *http.Request) (*http.Response, error) {
		var p soContractRequest
		if r.URL.Host == salesOrderHost {
			if r.Header.Get("X-Supergraph-Version") != "" {
				t.Error("inventory schema header leaked into sales-order request")
			}
			if r.URL.String() != salesOrderGraphQLURL || r.Method != http.MethodPost || r.Header.Get("X-Contract") != "native-service" || !strings.Contains(r.Header.Get("Authorization"), "service-fixture") {
				t.Errorf("wrong native transport or service headers")
			}
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.ToLower(p.Query), "invoice(input") {
				t.Fatal("invoice substitution")
			}
		} else if r.URL.Host != "qbo.intuit.com" || r.URL.Path != "/api/v3/company/12345/item/7" || r.Method != http.MethodGet {
			return nil, fmt.Errorf("unexpected offline request: %s %s", r.Method, r.URL.Path)
		}
		status, body := fn(r, p)
		return soContractResponse(status, body), nil
	})
	t.Cleanup(func() { uriHostTransport = oldURI; impersonatedTransport = oldTransport })
}

func soContractAck(op string) string {
	return `{"data":{"` + op + `":{"id":"1490000001","version":3,"orderNumber":"FIXTURE-SO","salesOrderLineItems":[{"id":"638554","position":0,"parentLineItemId":null,"parentItemId":null}]}}}`
}

func TestSalesOrderContractCreateResolvesRealItemAndPostsNative(t *testing.T) {
	calls := []string{}
	soContractMock(t, func(r *http.Request, p soContractRequest) (int, string) {
		calls = append(calls, p.OperationName)
		if r.Method == http.MethodGet {
			return 200, `{"Item":{"Id":"7","Type":"Service"}}`
		}
		if p.OperationName != "CreateSalesOrder" || !strings.Contains(p.Query, "CreateSalesOrderInput!") {
			t.Fatalf("wrong mutation: %+v", p)
		}
		order := p.Variables["order"].(map[string]any)
		for k, want := range map[string]any{"customerId": "10", "transactionDate": "2026-09-10", "dueDate": "2026-09-20", "term": "1000000004", "subtotal": float64(1), "total": float64(1), "totalTax": float64(0), "currencyIso": "AUD", "ignoreDuplicateOrderNum": false} {
			if order[k] != want {
				t.Errorf("%s=%v want %v", k, order[k], want)
			}
		}
		if _, exists := order["id"]; exists {
			t.Fatal("create invented order id")
		}
		line := order["lineItems"].([]any)[0].(map[string]any)
		for k, want := range map[string]any{"id": nil, "itemId": "7", "type": "SERVICE", "quantity": float64(1), "unitPrice": float64(1), "total": float64(1), "description": "fixture sales order", "taxable": false, "position": float64(0)} {
			if line[k] != want {
				t.Errorf("line %s=%v want %v", k, line[k], want)
			}
		}
		return 200, soContractAck("createSalesOrder")
	})
	res, err := ReplaySalesOrderMutate(t.Context(), "create", "", soContractFlags())
	if err != nil {
		t.Fatal(err)
	}
	if res.Entity != "SalesOrder" || res.Item.Type != "SalesOrder" || res.Item.ID != "1490000001" || res.Item.Amount != 1 || res.Item.Date != "2026-09-10" || len(calls) != 2 || calls[0] != "" || calls[1] != "CreateSalesOrder" {
		t.Fatalf("result=%+v calls=%v", res, calls)
	}
}

func TestSalesOrderContractUpdatePreservesNativeFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
		qty   float64
		note  string
	}{{"quantity", map[string]string{"line-items": soContractUpdateLine}, 2, "fixture sales order"}, {"sparse line", map[string]string{"line-items": `[{"Id":"638554","SalesItemLineDetail":{"Qty":2}}]`}, 2, "fixture sales order"}, {"note", map[string]string{"memo": "amended"}, 1, "amended"}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			soContractMock(t, func(_ *http.Request, p soContractRequest) (int, string) {
				calls++
				if calls == 1 {
					if p.OperationName != "GetSalesOrderWithTaxGroup" || p.Variables["id"] != "1490000001" {
						t.Fatalf("missing native get: %+v", p)
					}
					return 200, soContractGet
				}
				if p.OperationName != "UpdateSalesOrder" || !strings.Contains(p.Query, "UpdateSalesOrderInput!") {
					t.Fatalf("wrong update: %+v", p)
				}
				order := p.Variables["order"].(map[string]any)
				for k, want := range map[string]any{"id": "1490000001", "customerId": "10", "customerNote": tc.note, "term": "1000000004", "dueDate": "2026-09-20", "issuedAt": "2026-09-09T14:00:00Z", "transactionDate": "2026-09-10", "currencyIso": "AUD", "printTemplateId": "1000000002", "freeformBillingAddress": "Fixture billing address", "freeformShippingAddress": "Fixture shipping address", "total": tc.qty, "subtotal": tc.qty, "ignoreDuplicateOrderNum": true} {
					if order[k] != want {
						t.Errorf("%s=%v want %v", k, order[k], want)
					}
				}
				if order["currencyInfo"] == nil || order["attachments"] == nil {
					t.Error("lost omitted currency/attachments")
				}
				line := order["lineItems"].([]any)[0].(map[string]any)
				for k, want := range map[string]any{"id": "638554", "itemId": "7", "type": "SERVICE", "description": "fixture sales order", "quantity": tc.qty, "total": tc.qty, "unitPrice": float64(1), "baseQuantity": float64(1), "baseRate": float64(1), "unitConversionRatio": float64(1)} {
					if line[k] != want {
						t.Errorf("line %s=%v want %v", k, line[k], want)
					}
				}
				for _, key := range []string{"variantId", "totalDiscount", "isDeleted"} {
					if _, ok := line[key]; ok {
						t.Errorf("read-only native field forwarded: %s", key)
					}
				}
				return 200, soContractAck("updateSalesOrder")
			})
			res, err := ReplaySalesOrderMutate(t.Context(), "update", "1490000001", tc.flags)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || res.Item.Amount != tc.qty {
				t.Fatalf("result=%+v calls=%d", res, calls)
			}
		})
	}
}

func TestSalesOrderContractDeleteUsesNativeAcknowledgement(t *testing.T) {
	soContractMock(t, func(_ *http.Request, p soContractRequest) (int, string) {
		if p.OperationName != "DeleteSalesOrder" || p.Variables["id"] != "1490000001" || !strings.Contains(p.Query, "deletedSalesOrderId") {
			t.Fatalf("delete=%+v", p)
		}
		return 200, `{"data":{"deleteSalesOrder":{"deletedSalesOrderId":"1490000001"}}}`
	})
	res, err := ReplaySalesOrderMutate(t.Context(), "delete", "1490000001", nil)
	if err != nil || res.Entity != "SalesOrder" || res.Item.ID != "1490000001" {
		t.Fatalf("result=%+v err=%v", res, err)
	}
}

func TestSalesOrderContractPaginationAndSearch(t *testing.T) {
	for _, tc := range []struct {
		name, search                string
		limit, want, matched, calls int
	}{{"list", "", 101, 101, 0, 2}, {"name", "FIXTURE", 1, 1, 2, 2}, {"number", "so-late", 1, 1, 1, 2}, {"id", "ID-100", 1, 1, 1, 2}, {"none", "absent", 1, 0, 0, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			soContractMock(t, func(_ *http.Request, p soContractRequest) (int, string) {
				calls++
				if p.OperationName != "GetSalesOrders" || !strings.Contains(p.Query, "salesOrders {") {
					t.Fatalf("list=%+v", p)
				}
				offset, limit := int(p.Variables["offset"].(float64)), int(p.Variables["limit"].(float64))
				if offset != (calls-1)*100 {
					t.Fatalf("offset=%d call=%d", offset, calls)
				}
				if len(p.Variables["filter"].(map[string]any)) != 0 {
					t.Fatal("invented server-side filter")
				}
				rows := []map[string]any{}
				for i := offset; i < offset+limit && i < 102; i++ {
					name, number := "Other Customer", fmt.Sprintf("SO-%d", i)
					if i == 100 {
						name, number = "Fixture Customer", "SO-LATE"
					}
					if i == 101 {
						name = "Fixture Other"
					}
					rows = append(rows, map[string]any{"id": fmt.Sprintf("id-%d", i), "customerName": name, "orderNumber": number, "total": 1, "transactionDate": "2026-09-10"})
				}
				b, _ := json.Marshal(map[string]any{"data": map[string]any{"result": map[string]any{"salesOrders": rows, "totalCount": 102}}})
				return 200, string(b)
			})
			res, err := replayGetSalesOrders(t.Context(), "", tc.search, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Items) != tc.want || res.Counts["totalCount"] != 102 || calls != tc.calls || (tc.search != "" && res.Counts["matched"] != tc.matched) {
				t.Fatalf("result=%+v calls=%d", res, calls)
			}
		})
	}
}

func TestSalesOrderContractRejectsBeforePosting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]string)
	}{
		{"unsupported flag", func(f map[string]string) { f["amount"] = "1" }},
		{"unsupported empty flag", func(f map[string]string) { f["tax-code"] = "" }},
		{"conflicting aliases", func(f map[string]string) { f["txn-date"] = "2026-09-11" }},
		{"bad date", func(f map[string]string) { f["date"] = "31/02/2026" }},
		{"missing currency", func(f map[string]string) { delete(f, "currency") }},
		{"csv", func(f map[string]string) { f["line-items"] = "item,qty,amount\n7,1,1" }},
		{"empty lines", func(f map[string]string) { f["line-items"] = "[]" }},
		{"null line", func(f map[string]string) { f["line-items"] = "[null]" }},
		{"numeric string", func(f map[string]string) {
			f["line-items"] = strings.Replace(soContractLine, `"Qty":1`, `"Qty":"1"`, 1)
		}},
		{"negative", func(f map[string]string) { f["line-items"] = strings.Replace(soContractLine, `"Qty":1`, `"Qty":-1`, 1) }},
		{"zero quantity", func(f map[string]string) { f["line-items"] = strings.Replace(soContractLine, `"Qty":1`, `"Qty":0`, 1) }},
		{"overflow", func(f map[string]string) {
			f["line-items"] = strings.Replace(soContractLine, `"Qty":1`, `"Qty":1e999`, 1)
		}},
		{"precision", func(f map[string]string) {
			f["line-items"] = strings.Replace(soContractLine, `"Qty":1`, `"Qty":1.0000000000000001`, 1)
		}},
		{"tax field", func(f map[string]string) {
			f["line-items"] = strings.Replace(soContractLine, `"Qty":1`, `"Qty":1,"TaxCodeRef":{"value":"NON"}`, 1)
		}},
		{"ambiguous amount", func(f map[string]string) {
			f["line-items"] = strings.Replace(soContractLine, `"Amount":1`, `"Amount":2`, 1)
		}},
		{"unsupported detail", func(f map[string]string) {
			f["line-items"] = strings.ReplaceAll(soContractLine, "SalesItemLineDetail", "DiscountLineDetail")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			soContractMock(t, func(_ *http.Request, _ soContractRequest) (int, string) { calls++; return 500, `{}` })
			flags := soContractFlags()
			tc.change(flags)
			res, err := ReplaySalesOrderMutate(t.Context(), "create", "", flags)
			if err == nil || res != nil || calls != 0 {
				t.Fatalf("result=%+v err=%v calls=%d", res, err, calls)
			}
		})
	}
}

func TestSalesOrderContractRejectsUnsupportedExistingAndItemTypes(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"tax", strings.Replace(soContractGet, `"totalTax":0`, `"totalTax":1`, 1)},
		{"discount", strings.Replace(soContractGet, `"totalDiscount":0`, `"totalDiscount":1`, 1)},
		{"tax group", strings.Replace(soContractGet, `"taxGroup":{"id":null}`, `"taxGroup":{"id":"tax-1"}`, 1)},
		{"line identity", strings.Replace(soContractGet, `"id":"638554"`, `"id":""`, 1)},
		{"total mismatch", strings.Replace(soContractGet, `"subtotal":1`, `"subtotal":2`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			soContractMock(t, func(_ *http.Request, p soContractRequest) (int, string) {
				calls++
				if p.OperationName != "GetSalesOrderWithTaxGroup" {
					t.Error("posted unsupported update")
				}
				return 200, tc.body
			})
			res, err := ReplaySalesOrderMutate(t.Context(), "update", "1490000001", map[string]string{"memo": "changed"})
			if err == nil || res != nil || calls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", res, err, calls)
			}
		})
	}
	t.Run("unverified item type", func(t *testing.T) {
		calls := 0
		soContractMock(t, func(r *http.Request, _ soContractRequest) (int, string) {
			calls++
			if r.Method != http.MethodGet {
				t.Error("posted unsupported create")
			}
			return 200, `{"Item":{"Id":"7","Type":"Inventory"}}`
		})
		_, err := ReplaySalesOrderMutate(t.Context(), "create", "", soContractFlags())
		if err == nil || calls != 1 {
			t.Fatalf("err=%v calls=%d", err, calls)
		}
	})
}

func TestSalesOrderContractStrictResponseFailures(t *testing.T) {
	for _, tc := range []struct {
		name, op string
		status   int
		body     string
	}{
		{"HTTP", "list", 503, `{"error":"unavailable"}`},
		{"GraphQL", "list", 200, `{"errors":[{"message":"denied"}],"data":{"result":{"totalCount":0,"salesOrders":[]}}}`},
		{"malformed", "list", 200, `{`}, {"missing data", "list", 200, `{}`}, {"null result", "list", 200, `{"data":{"result":null}}`},
		{"count only", "list", 200, `{"data":{"result":{"totalCount":1}}}`},
		{"missing count", "list", 200, `{"data":{"result":{"salesOrders":[]}}}`},
		{"missing row id", "list", 200, `{"data":{"result":{"totalCount":1,"salesOrders":[{"total":1}]}}}`},
		{"bad count", "list", 200, `{"data":{"result":{"totalCount":1.5,"salesOrders":[]}}}`},
		{"missing row total", "get", 200, `{"data":{"result":{"id":"1490000001"}}}`},
		{"wrong get id", "get", 200, `{"data":{"result":{"id":"other","total":1}}}`},
		{"missing delete ack", "delete", 200, `{"data":{"deleteSalesOrder":{}}}`},
		{"wrong delete ack", "delete", 200, `{"data":{"deleteSalesOrder":{"deletedSalesOrderId":"other"}}}`},
		{"missing create ack", "create", 200, `{"data":{"createSalesOrder":{}}}`},
		{"missing version", "create", 200, strings.Replace(soContractAck("createSalesOrder"), `"version":3,`, "", 1)},
		{"missing line ack", "create", 200, `{"data":{"createSalesOrder":{"id":"1490000001","version":1,"orderNumber":"FIXTURE-SO"}}}`},
		{"wrong update id", "update", 200, strings.Replace(soContractAck("updateSalesOrder"), "1490000001", "other", 1)},
		{"wrong line id", "update", 200, strings.Replace(soContractAck("updateSalesOrder"), "638554", "other", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			soContractMock(t, func(r *http.Request, p soContractRequest) (int, string) {
				if r.Method == http.MethodGet {
					return 200, `{"Item":{"Id":"7","Type":"Service"}}`
				}
				if tc.op == "update" && p.OperationName == "GetSalesOrderWithTaxGroup" {
					return 200, soContractGet
				}
				return tc.status, tc.body
			})
			var err error
			switch tc.op {
			case "list":
				_, err = ReplayQuery(t.Context(), "SalesOrder", "", "", 1)
			case "get":
				_, err = ReplayQuery(t.Context(), "SalesOrder", "1490000001", "", 1)
			case "create":
				_, err = ReplaySalesOrderMutate(t.Context(), "create", "", soContractFlags())
			case "update":
				_, err = ReplaySalesOrderMutate(t.Context(), "update", "1490000001", map[string]string{"memo": "changed"})
			case "delete":
				_, err = ReplaySalesOrderMutate(t.Context(), "delete", "1490000001", nil)
			}
			if err == nil {
				t.Fatal("accepted failed response")
			}
			if tc.status != 200 {
				var replay *ReplayError
				if !errors.As(err, &replay) || replay.Status != tc.status {
					t.Fatalf("wrong HTTP error: %v", err)
				}
			}
		})
	}
}

func TestSalesOrderContractPaginationRejectsStallsAndChanges(t *testing.T) {
	for _, tc := range []struct{ name, page string }{
		{"stalled", `{"totalCount":2,"salesOrders":[]}`},
		{"repeated", `{"totalCount":2,"salesOrders":[{"id":"first","total":1}]}`},
		{"changed total", `{"totalCount":3,"salesOrders":[{"id":"second","total":1}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			soContractMock(t, func(_ *http.Request, _ soContractRequest) (int, string) {
				calls++
				if calls == 1 {
					return 200, `{"data":{"result":{"totalCount":2,"salesOrders":[{"id":"first","total":1}]}}}`
				}
				return 200, `{"data":{"result":` + tc.page + `}}`
			})
			_, err := ReplayQuery(t.Context(), "SalesOrder", "", "missing", 1)
			if err == nil || calls != 2 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestSalesOrderContractNoCredentialsAndCancelled(t *testing.T) {
	noCredsDir(t)
	_, err := ReplaySalesOrderMutate(t.Context(), "create", "", soContractFlags())
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err=%v", err)
	}
	_, err = ReplaySalesOrderMutate(t.Context(), "delete", "", nil)
	if !errors.Is(err, ErrMissingMutateID) {
		t.Fatalf("err=%v", err)
	}
	soContractMock(t, func(_ *http.Request, _ soContractRequest) (int, string) {
		t.Error("cancelled request dialed")
		return 500, `{}`
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = ReplayQuery(ctx, "SalesOrder", "", "query", 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}
