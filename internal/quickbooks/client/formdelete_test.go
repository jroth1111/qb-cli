package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The form-delete contract: POST /api/v4/graphql running
// deleteSavedTransaction__transactions_experiences_qbo — an
// updateTransactions_Transaction with transactionsTransaction{id, type,
// deleted:true, header:{closeBooksPassword:"*"}}. The node is read first via
// `query c($id: ID!)` for existence + the real v4 type (v3 Bill is
// PURCHASE_BILL). Captured on TC2 (expense form More → Delete, Purchase 184).
func formDeleteServer(t *testing.T, nodeType, mutResp string) *domServer {
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
		case strings.Contains(r.URL.Path, "/api/v4/graphql"):
			if strings.Contains(string(b), "deleteSavedTransaction") {
				_, _ = w.Write([]byte(mutResp))
				return
			}
			if nodeType == "" {
				_, _ = w.Write([]byte(`{"data":{"node":null}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"node":{"id":"ns:1","type":"` + nodeType + `"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func formDelOK(nodeID string) string {
	return `{"data":{"updateTransactions_Transaction":{"clientMutationId":"0",` +
		`"transactionsTransaction":{"id":"` + nodeID + `","type":"PURCHASE","deleted":true,` +
		`"traits":{"payment":{"paymentType":null}},"__typename":"Transactions_Transaction"}}}}`
}

func TestFormDeleteWireShape(t *testing.T) {
	srv := formDeleteServer(t, "PURCHASE", formDelOK("djQuMTo5MzQxNDU3NzY5NzU2ODU0OjgwMjcxZWRkOGE:187"))
	res, err := ReplayFormDelete(context.Background(), "187", "PURCHASE")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Op != "delete" || res.Entity != "Purchase" || res.Item.ID != "187" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/api/v4/graphql") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	s := string(body)
	// The test harness realm is 12345, so the node id uses the derived
	// namespace, not the TC2 constant.
	wantID := "djQuMToxMjM0NTo4MDI3MWVkZDhh:187"
	for _, want := range []string{
		"deleteSavedTransaction__transactions_experiences_qbo",
		`"deleted":true`,
		`"type":"PURCHASE"`,
		`"id":"` + wantID + `"`,
		`"closeBooksPassword":"*"`,
		`"clientMutationId":"0"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.800s", want, s)
		}
	}
}

func TestFormDeleteBillType(t *testing.T) {
	// v3 Bill is PURCHASE_BILL in v4 — the node read supplies the real type.
	srv := formDeleteServer(t, "PURCHASE_BILL", formDelOK("ns:185"))
	res, err := ReplayFormDelete(context.Background(), "185", "PURCHASE_BILL")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Entity != "Bill" {
		t.Fatalf("entity = %q, want Bill", res.Entity)
	}
	_, _, _, body := srv.snap()
	if !strings.Contains(string(body), `"type":"PURCHASE_BILL"`) {
		t.Fatalf("delete must carry the node's real type: %.400s", body)
	}
}

func TestFormDeleteValidation(t *testing.T) {
	formDeleteServer(t, "PURCHASE", formDelOK("ns:1"))
	if _, err := ReplayFormDelete(context.Background(), "", "PURCHASE"); err == nil || !strings.Contains(err.Error(), "--id") {
		t.Fatalf("missing id should fail, got %v", err)
	}
	// Wrong-type surface: bill handed to the expense delete.
	if _, err := ReplayFormDelete(context.Background(), "1", "PURCHASE_BILL"); err == nil || !strings.Contains(err.Error(), "not PURCHASE_BILL") {
		t.Fatalf("type mismatch should fail, got %v", err)
	}
}

func TestFormDeleteMissingNode(t *testing.T) {
	formDeleteServer(t, "", formDelOK("ns:1"))
	_, err := ReplayFormDelete(context.Background(), "999", "PURCHASE")
	if err == nil || !strings.Contains(err.Error(), "no v4 transaction node") {
		t.Fatalf("missing node should fail, got %v", err)
	}
}

func TestFormDeleteGraphQLErrors(t *testing.T) {
	formDeleteServer(t, "PURCHASE", `{"errors":[{"message":"cannot delete reconciled transaction"}]}`)
	_, err := ReplayFormDelete(context.Background(), "187", "PURCHASE")
	if err == nil || !strings.Contains(err.Error(), "cannot delete reconciled") {
		t.Fatalf("graphql error should surface, got %v", err)
	}
	// The server echoes deleted:true on nonexistent nodes — but a false
	// deleted flag must also be rejected, not reported as success.
	formDeleteServer(t, "PURCHASE", `{"data":{"updateTransactions_Transaction":{"transactionsTransaction":{"id":"ns:9","type":"PURCHASE","deleted":false}}}}`)
	if _, err := ReplayFormDelete(context.Background(), "9", "PURCHASE"); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("deleted:false should fail, got %v", err)
	}
}
