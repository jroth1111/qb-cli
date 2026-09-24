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

// phase2aServer answers v3 GETs with the given fixture and captures the POST body.
func phase2aServer(t *testing.T, getBody string) (*domServer, *map[string]any, *string) {
	t.Helper()
	var lastPost map[string]any
	var lastQuery string
	s := &domServer{t: t}
	s.Server = httptest.NewServer(withPersistedV3(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		lastQuery = r.URL.RawQuery
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(getBody))
			return
		}
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		_ = json.Unmarshal(b, &lastPost)
		entity := "Project"
		for name, path := range v3Path {
			if strings.HasSuffix(r.URL.Path, "/"+path) {
				entity = name
				break
			}
		}
		id := jsonNumberString(lastPost["Id"])
		if id == "" {
			id = "71"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{entity: map[string]any{"Id": id, "SyncToken": "1"}})
	})))
	t.Cleanup(s.Close)
	return s, &lastPost, &lastQuery
}

func TestProjectCreateBody(t *testing.T) {
	saveUsable(t)
	srv, lastPost, _ := phase2aServer(t, `{}`)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Project", "create", "", map[string]string{
		"customer": "9", "name": "Kitchen Reno",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Entity != "Project" {
		t.Fatalf("result = %+v", res)
	}
	body := *lastPost
	if body["Name"] != "Kitchen Reno" {
		t.Fatalf("Name = %v", body["Name"])
	}
	cust, _ := body["CustomerRef"].(map[string]any)
	if cust["value"] != "9" {
		t.Fatalf("CustomerRef = %v", body["CustomerRef"])
	}
	if _, ok := body["Line"]; ok {
		t.Fatal("project create must not emit Line")
	}
}

func TestProjectCreateRequiresCustomer(t *testing.T) {
	if _, err := ReplayMutate(context.Background(), "Project", "create", "", map[string]string{
		"name": "x",
	}); err == nil {
		t.Fatal("missing --customer must fail")
	}
}

func TestCreditCardCreditCreateBody(t *testing.T) {
	saveUsable(t)
	srv, lastPost, _ := phase2aServer(t, `{}`)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "CreditCardCredit", "create", "", map[string]string{
		"credit-card-account": "41",
		"amount":              "25",
		"category-account":    "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Entity != "CreditCardCredit" {
		t.Fatalf("result = %+v", res)
	}
	body := *lastPost
	acct, _ := body["AccountRef"].(map[string]any)
	if acct["value"] != "41" {
		t.Fatalf("AccountRef = %v", body["AccountRef"])
	}
	lines, _ := body["Line"].([]any)
	if len(lines) != 1 {
		t.Fatalf("Line = %v", body["Line"])
	}
	line, _ := lines[0].(map[string]any)
	if line["DetailType"] != "AccountBasedExpenseLineDetail" {
		t.Fatalf("line DetailType = %v", line["DetailType"])
	}
}

func TestCreditCardCreditRequiresAccount(t *testing.T) {
	if _, err := ReplayMutate(context.Background(), "CreditCardCredit", "create", "", map[string]string{
		"amount": "25",
	}); err == nil {
		t.Fatal("missing --credit-card-account must fail")
	}
}

func TestVoidableEntityMap(t *testing.T) {
	for in, want := range map[string]string{
		"invoice": "Invoice", "PAYMENT": "Payment", "journal": "JournalEntry",
	} {
		got, err := VoidableEntity(in)
		if err != nil || got != want {
			t.Fatalf("VoidableEntity(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "customer", "cheque", "estimate", "purchase-order"} {
		if _, err := VoidableEntity(in); err == nil {
			t.Fatalf("VoidableEntity(%q) must fail (not voidable on v3)", in)
		}
	}
}

func TestTxnVoidPostsOperationVoid(t *testing.T) {
	saveUsable(t)
	srv, lastPost, lastQuery := phase2aServer(t, `{"Invoice":{"Id":"88","SyncToken":"2"}}`)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Invoice", "void", "88", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "void" {
		t.Fatalf("op = %q", res.Op)
	}
	body := *lastPost
	if body["Id"] != "88" || body["SyncToken"] != "2" {
		t.Fatalf("void body = %v", body)
	}
	if q := *lastQuery; q == "" || !containsStr(q, "operation=void") {
		t.Fatalf("void query = %q", q)
	}
}
