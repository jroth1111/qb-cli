package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The items-import contract: v3 queries resolve accounts/categories, then a
// single qbbulkaction bulkActionOnProductList mutation posts the products
// (ASYNC) and transactionBulkActionList polls to terminal status.
func itemsImportServer(t *testing.T) (*domServer, *[]byte) {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
	createBody := &[]byte{}
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
		case strings.Contains(r.URL.Path, "/v3/company/"):
			dec, _ := url.QueryUnescape(r.URL.RawQuery)
			hay := dec + string(b)
			switch {
			case strings.Contains(hay, "Item"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Item":[{"Id":"8","Name":"Service","Type":"Category"}]}}`))
			default:
				name := "Sales of Product Income"
				if m := regexp.MustCompile(`like '%([^']+)%'`).FindStringSubmatch(hay); len(m) == 2 {
					name = m[1]
				}
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"52","Name":"` + name + `"}]}}`))
			}
		case strings.Contains(r.URL.Path, "/v4/graphql"):
			if strings.Contains(string(b), "transactionBulkActionList") {
				_, _ = w.Write([]byte(`{"data":{"company":{"transactionBulkActionList":{"edges":[{"node":{"id":"ba-1","status":"COMPLETED","totalRequests":1,"totalSuccessful":1,"totalUnsuccessful":0}}]}}}}`))
			} else {
				s.mu.Lock()
				*createBody = b
				s.mu.Unlock()
				_, _ = w.Write([]byte(`{"data":{"Qbshared_Bulk_ProductBulkAction_bulkActionOnProductList":{"clientMutationId":"1","qbsharedBulkProductBulkActionProductListInput":{"id":"ba-1","status":"IN_PROGRESS","successEntities":[],"failedEntities":[],"inProgressEntities":[{}]}}}}`))
			}
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s, createBody
}

func writeItemsCSV(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "items.csv")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestItemsImportWireShape(t *testing.T) {
	srv, createBody := itemsImportServer(t)
	path := writeItemsCSV(t,
		"Product/Service Name,Type,SKU,Sales Description,Sales Price / Rate,Income Account,Purchase Description,Purchase Cost,Expense Account,Category\n"+
			"Test Svc,Service,SKU-9,desc,75,Sales of Product Income,pdesc,30,Cost of sales,Service\n")
	res, err := ReplayItemsImport(context.Background(), path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Op != "items import" || res.Item.Name != "1 rows" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, _ := srv.snap()
	if meth != http.MethodPost {
		t.Fatalf("method = %s", meth)
	}
	if !strings.Contains(pathq, "/v4/graphql") {
		t.Fatalf("path = %s", pathq)
	}
	body := *createBody
	var env struct {
		Query     string `json:"query"`
		Variables struct {
			Input struct {
				List struct {
					Mode     string `json:"mode"`
					Name     string `json:"name"`
					Action   string `json:"action"`
					Products []struct {
						Product map[string]any `json:"product"`
					} `json:"products"`
				} `json:"qbsharedBulkProductBulkActionProductListInput"`
			} `json:"input_0"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if !strings.Contains(env.Query, "bulkActionOnProductList") {
		t.Fatalf("query = %s", env.Query[:80])
	}
	l := env.Variables.Input.List
	if l.Mode != "ASYNC" || l.Action != "CREATE" || !strings.HasPrefix(l.Name, "Products-") {
		t.Fatalf("list input = %+v", l)
	}
	if len(l.Products) != 1 {
		t.Fatalf("products = %d", len(l.Products))
	}
	p := l.Products[0].Product
	if p["name"] != "Test Svc" || p["type"] != "SERVICE" || p["source"] != "PURCHASED" || p["categoryId"] != "8" {
		t.Fatalf("product = %v", p)
	}
	ref := p["accountingReference"].(map[string]any)
	if ref["salesAccountId"] != "52" || ref["purchaseAccountId"] != "52" || ref["salesDescription"] != "desc" || ref["purchaseDescription"] != "pdesc" {
		t.Fatalf("accountingReference = %v", ref)
	}
	if p["defaultVariantPrice"] != float64(75) || p["defaultVariantCost"] != float64(30) {
		t.Fatalf("variants = %v", p["variants"])
	}
	v := p["variants"].([]any)[0].(map[string]any)
	if v["sku"] != "SKU-9" || v["price"] != float64(75) || v["cost"] != float64(30) {
		t.Fatalf("variant = %v", v)
	}
}

func TestItemsImportValidation(t *testing.T) {
	_, _ = itemsImportServer(t)
	if _, err := ReplayItemsImport(context.Background(), ""); err == nil {
		t.Fatal("expected error for missing --file")
	}
	if _, err := ReplayItemsImport(context.Background(), writeItemsCSV(t, "Type\nService\n")); err == nil ||
		!strings.Contains(err.Error(), "Product/Service name column is required") {
		t.Fatalf("name err = %v", err)
	}
	if _, err := ReplayItemsImport(context.Background(), writeItemsCSV(t, "Name\nX\n")); err == nil ||
		!strings.Contains(err.Error(), "Item type column is required") {
		t.Fatalf("type err = %v", err)
	}
	if _, err := ReplayItemsImport(context.Background(), writeItemsCSV(t, "Name,Type\nX,Inventory\n")); err == nil ||
		!strings.Contains(err.Error(), "inventory") {
		t.Fatalf("inventory err = %v", err)
	}
	if _, err := ReplayItemsImport(context.Background(), writeItemsCSV(t, "Name,Type\nX,Bogus\n")); err == nil ||
		!strings.Contains(err.Error(), "unknown item type") {
		t.Fatalf("bogus err = %v", err)
	}
}

func TestItemsImportTypeNormalization(t *testing.T) {
	for in, want := range map[string]string{
		"Service": "SERVICE", "services": "SERVICE", "Non-inventory": "NON_INVENTORY", "non inventory": "NON_INVENTORY",
	} {
		got, err := normalizeItemType(in)
		if err != nil || got != want {
			t.Fatalf("normalizeItemType(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestWaitBulkActionTimeoutIdentifiesAction(t *testing.T) {
	_, _ = itemsImportServer(t)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = waitBulkAction(context.Background(), ac, "missing-action", nil, 0)
	if err == nil || !strings.Contains(err.Error(), "missing-action") || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err = %v", err)
	}
}

func TestItemsImportPollFailureIsUnknownOutcome(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/v3/company/"):
			_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[{"Id":"52","Name":"Sales of Product Income"}]}}`))
		case strings.Contains(string(body), "transactionBulkActionList"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"poll unavailable"}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"Qbshared_Bulk_ProductBulkAction_bulkActionOnProductList":{"qbsharedBulkProductBulkActionProductListInput":{"id":"ba-unknown","status":"IN_PROGRESS"}}}}`))
		}
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	path := writeItemsCSV(t, "Name,Type\nTest Svc,Service\n")
	if _, err := ReplayItemsImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "outcome unknown") || !strings.Contains(err.Error(), "ba-unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestItemsImportRequiresBulkActionID(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/v4/graphql") {
			_, _ = w.Write([]byte(`{"data":{"Qbshared_Bulk_ProductBulkAction_bulkActionOnProductList":{"qbsharedBulkProductBulkActionProductListInput":{"status":"IN_PROGRESS"}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	path := writeItemsCSV(t, "Name,Type\nTest Svc,Service\n")
	if _, err := ReplayItemsImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "outcome unknown") || !strings.Contains(err.Error(), "action id missing") {
		t.Fatalf("err = %v", err)
	}
}
