package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The merge contract: resolve keep+dup via v3 (name→id, id→displayName),
// resolve each to a Relay-namespaced contact id via findContact_qbo, then
// post updateContact_qbo with networkContact.mergedTo.id = keep.
func mergeServer(t *testing.T) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
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
		case strings.Contains(r.URL.Path, "/v4/graphql"):
			if strings.Contains(string(b), `"findContact_qbo"`) {
				m := regexp.MustCompile(`displayName='([^']+)'`).FindStringSubmatch(string(b))
				name := ""
				if len(m) > 1 {
					name = m[1]
				}
				resp := map[string]any{"data": map[string]any{"company": map[string]any{"contacts": map[string]any{
					"edges": []any{map[string]any{"node": map[string]any{
						"id": "ns:" + name, "displayName": name,
					}}},
				}}}}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"updateNetwork_Contact":{"clientMutationId":"0","networkContact":{"id":"ns:Keep","displayName":"Keep"}}}}`))
		case regexp.MustCompile(`/v3/company/\d+/customer/\d+`).MatchString(r.URL.Path):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_, _ = w.Write([]byte(`{"Customer":{"Id":"` + id + `","DisplayName":"Cust` + id + `"}}`))
		case regexp.MustCompile(`/v3/company/\d+/vendor/\d+`).MatchString(r.URL.Path):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_, _ = w.Write([]byte(`{"Vendor":{"Id":"` + id + `","DisplayName":"Vend` + id + `"}}`))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			names := map[string]string{"33": "CustKeep", "34": "CustDup", "35": "VendKeep", "36": "VendDup"}
			if idm := regexp.MustCompile(`Id = '(\d+)'`).FindStringSubmatch(q); len(idm) > 1 {
				id := idm[1]
				key := "Customer"
				if strings.Contains(q, "Vendor") {
					key = "Vendor"
				}
				if names[id] == "" {
					_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
					return
				}
				_, _ = w.Write([]byte(`{"QueryResponse":{"` + key + `":[{"Id":"` + id + `","DisplayName":"` + names[id] + `"}]}}`))
				return
			}
			if strings.Contains(q, "Vendor") {
				_, _ = w.Write([]byte(`{"QueryResponse":{"Vendor":[{"Id":"35","DisplayName":"VendKeep"},{"Id":"36","DisplayName":"VendDup"}]}}`))
			} else {
				_, _ = w.Write([]byte(`{"QueryResponse":{"Customer":[{"Id":"33","DisplayName":"CustKeep"},{"Id":"34","DisplayName":"CustDup"}]}}`))
			}
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestMergeWireShape(t *testing.T) {
	srv := mergeServer(t)
	res, err := ReplayMerge(context.Background(), "Customer", "CustKeep", "CustDup")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.Op != "merge" || res.Item.ID != "33" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/v4/graphql") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	s := string(body)
	for _, want := range []string{
		`"operationName":"updateContact_qbo"`,
		`"id":"ns:CustDup"`,
		`"mergedTo":{"id":"ns:CustKeep"}`,
		`"entityVersion":"0"`,
		`"customer"`, `"active":true`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.600s", want, s)
		}
	}
}

func TestMergeVendorProfile(t *testing.T) {
	srv := mergeServer(t)
	res, err := ReplayMerge(context.Background(), "Vendor", "VendKeep", "VendDup")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.Entity != "Vendor" || res.Item.ID != "35" {
		t.Fatalf("result = %+v", res)
	}
	_, _, _, body := srv.snap()
	s := string(body)
	if !strings.Contains(s, `"vendor"`) || strings.Contains(s, `"customer":{"active"`) {
		t.Fatalf("vendor profile wrong: %.400s", s)
	}
}

func TestMergeValidation(t *testing.T) {
	mergeServer(t)
	if _, err := ReplayMerge(context.Background(), "Customer", "", "x"); err == nil {
		t.Fatal("expected error for missing keep")
	}
	if _, err := ReplayMerge(context.Background(), "Customer", "Same", "same"); err == nil {
		t.Fatal("expected error for identical keep/merge")
	}
	if _, err := ReplayMerge(context.Background(), "Invoice", "a", "b"); err == nil {
		t.Fatal("expected error for unsupported entity")
	}
	if _, err := ReplayMerge(context.Background(), "Customer", "Nobody", "b"); err == nil {
		t.Fatal("expected error for unknown keep")
	}
}
