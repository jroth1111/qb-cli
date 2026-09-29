package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountQueryProjectsHierarchy(t *testing.T) {
	items := projectQuery("Account", []byte(`{"QueryResponse":{"Account":[{"Id":"171","Name":"Miscellaneous Property Expense","FullyQualifiedName":"Client Property Expenses:Short-Term Business Operating Expenses:Miscellaneous Property Expense","ParentRef":{"value":"62","name":"Short-Term Business Operating Expenses"},"Active":true}]}}`))
	if len(items) != 1 || items[0].ID != "171" || items[0].FullName != "Client Property Expenses:Short-Term Business Operating Expenses:Miscellaneous Property Expense" || items[0].ParentID != "62" || items[0].Parent != "Short-Term Business Operating Expenses" {
		t.Fatalf("account hierarchy projection = %+v", items)
	}
}

func TestQueryRejectsFalseEmptySuccess(t *testing.T) {
	for _, body := range []string{`<IntuitResponse/>`, `<html>login</html>`, `{}`, `{"Fault":{"type":"ValidationFault"}}`, `{"QueryResponse":null}`, `{"QueryResponse":{"Customer":"error"}}`} {
		t.Run(body, func(t *testing.T) {
			saveUsable(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			if _, err := ReplayQuery(context.Background(), "Customer", "", "", 5); err == nil {
				t.Fatal("invalid response reported as an empty successful query")
			}
		})
	}
	if err := validateQueryEnvelope("Customer", []byte(`{"QueryResponse":{}}`)); err != nil {
		t.Fatal(err)
	}
	if err := validateQueryEnvelope("Customer", []byte(`{"QueryResponse":{"Customer":[]}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestQueryBadRequestFails(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"Fault":{"type":"ValidationFault"}}`)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	if _, err := ReplayQuery(context.Background(), "Customer", "", "", 5); err == nil {
		t.Fatal("HTTP 400 returned success")
	}
}

func TestDepartmentIDReadUsesNativeResource(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v3/company/12345/department/42" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"Department":{"Id":"42","Name":"Fixture","Active":false}}`)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	result, err := ReplayQuery(context.Background(), "Department", "42", "", 1, "all")
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != "42" || result.Items[0].Active == nil || *result.Items[0].Active {
		t.Fatalf("native Department read: %+v, %v", result, err)
	}
	if got := PlannedQueryURL("Department", "42"); got != "https://qbo.intuit.com/api/v3/company/{realm}/department/42?minorversion=73" {
		t.Fatalf("plan drifted: %s", got)
	}
}

func TestPurchaseIDReadIncludesFullLineAndSyncToken(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v3/company/12345/purchase/51" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"Purchase":{"Id":"51","SyncToken":"7","TotalAmt":25,"Line":[{"Id":"1","Amount":25,"AccountBasedExpenseLineDetail":{"AccountRef":{"value":"115"},"ClassRef":{"value":"1303"},"TaxCodeRef":{"value":"NON"}}}]}}`)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	result, err := ReplayQuery(context.Background(), "Purchase", "51", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "51" || result.Detail["SyncToken"] != "7" {
		t.Fatalf("missing purchase detail: %+v", result)
	}
	lines, _ := result.Detail["Line"].([]any)
	if len(lines) != 1 {
		t.Fatalf("missing lines: %+v", result.Detail)
	}
	detail := lines[0].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)
	if expenseRefValue(detail["ClassRef"]) != "1303" || expenseRefValue(detail["AccountRef"]) != "115" || expenseRefValue(detail["TaxCodeRef"]) != "NON" {
		t.Fatalf("missing line fields: %+v", detail)
	}
}
