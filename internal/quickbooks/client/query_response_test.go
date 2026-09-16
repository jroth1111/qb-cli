package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
