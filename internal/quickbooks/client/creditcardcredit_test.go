package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The credit-card-credit contract: POST /api/v4/graphql running
// UpdateTransactions_Transaction_qbo with type PURCHASE,
// header.transactionType CREDIT_CARD_CREDIT, qboAppData.txnTypeId "11",
// header.account as a namespaced Account node id, and accountLines[] with
// bare account ids + ACCOUNT_client_N line ids. Captured on TC2 after v3
// rejected creditcardcredit outright (500 unsupported).
func cccServer(t *testing.T, gqlResp string) *domServer {
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
			_, _ = w.Write([]byte(gqlResp))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			if strings.Contains(q, "Account") {
				_, _ = w.Write([]byte(`{"QueryResponse":{"Account":[` +
					`{"Id":"70","Name":"Rewards Card","AccountType":"Credit Card"},` +
					`{"Id":"17","Name":"Office expenses","AccountType":"Expense"},` +
					`{"Id":"44","Name":"Chequing","AccountType":"Bank"}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func cccOK(nodeID string) string {
	return `{"data":{"updateTransactions_Transaction":{"clientMutationId":"0",` +
		`"transactionsTransaction":{"id":"` + nodeID + `","entityVersion":"1"}}}}`
}

func TestCreditCardCreditWireShape(t *testing.T) {
	srv := cccServer(t, cccOK("djQuMTo5MzQxNDU3NzY5NzU2ODU0OjgwMjcxZWRkOGE:177"))
	res, err := ReplayCreditCardCredit(context.Background(),
		"70", "70", "19/09/2026", "",
		`[{"account":"17","amount":"33.00","description":"qb-cli ccc live test"}]`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Op != "create" || res.Entity != "CreditCardCredit" || res.Item.ID != "177" || res.Item.Amount != 33 {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/api/v4/graphql") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	s := string(body)
	wantAcct := base64.RawURLEncoding.EncodeToString([]byte("v4.1:12345:51ced8536c")) + ":70"
	for _, want := range []string{
		"UpdateTransactions_Transaction_qbo",
		`"transactionType":"CREDIT_CARD_CREDIT"`,
		`"type":"PURCHASE"`,
		`"txnTypeId":"11"`,
		`"paymentType":"CREDIT_CARD_CREDIT"`,
		`"id":"` + wantAcct + `"`,
		`"txnDate":"2026-09-19"`,
		`"id":"ACCOUNT_client_0"`,
		`"account":{"id":"17"}`,
		`"amount":"33.00"`,
		`"description":"qb-cli ccc live test"`,
		`"entityVersion":"0"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.1200s", want, s)
		}
	}
	// Relay draft id must use the transaction namespace with the -1 sentinel.
	var sent struct {
		Variables struct {
			Input struct {
				Txn struct {
					ID    string `json:"id"`
					Lines struct {
						ItemLines    []any `json:"itemLines"`
						AccountLines []any `json:"accountLines"`
					} `json:"lines"`
				} `json:"transactionsTransaction"`
			} `json:"input_0"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("sent payload: %v", err)
	}
	if !strings.HasSuffix(sent.Variables.Input.Txn.ID, ":-1") {
		t.Fatalf("draft id = %q, want :-1 sentinel", sent.Variables.Input.Txn.ID)
	}
	if len(sent.Variables.Input.Txn.Lines.AccountLines) != 1 || len(sent.Variables.Input.Txn.Lines.ItemLines) != 0 {
		t.Fatalf("lines = %+v", sent.Variables.Input.Txn.Lines)
	}
}

func TestCreditCardCreditResolvesNames(t *testing.T) {
	cccServer(t, cccOK("ns:188"))
	res, err := ReplayCreditCardCredit(context.Background(),
		"Rewards Card", "", "19/09/2026", "Office expenses",
		`[{"amount":"12.50","description":"cat fallback"}]`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Item.ID != "188" {
		t.Fatalf("id = %q", res.Item.ID)
	}
}

func TestCreditCardCreditValidation(t *testing.T) {
	cccServer(t, cccOK("ns:1"))
	if _, err := ReplayCreditCardCredit(context.Background(), "", "", "19/09/2026", "", `[{"account":"17","amount":1}]`); err == nil || !strings.Contains(err.Error(), "--credit-card-account") {
		t.Fatalf("missing card should fail, got %v", err)
	}
	if _, err := ReplayCreditCardCredit(context.Background(), "70", "", "", "", `[{"account":"17","amount":1}]`); err == nil || !strings.Contains(err.Error(), "--date") {
		t.Fatalf("missing date should fail, got %v", err)
	}
	if _, err := ReplayCreditCardCredit(context.Background(), "70", "", "19/09/2026", "", ""); err == nil || !strings.Contains(err.Error(), "--line-items") {
		t.Fatalf("missing line-items should fail, got %v", err)
	}
	if _, err := ReplayCreditCardCredit(context.Background(), "70", "", "19/09/2026", "", `[{"amount":5}]`); err == nil || !strings.Contains(err.Error(), "missing account") {
		t.Fatalf("line without account should fail, got %v", err)
	}
}

func TestCreditCardCreditAliasMismatch(t *testing.T) {
	cccServer(t, cccOK("ns:1"))
	_, err := ReplayCreditCardCredit(context.Background(),
		"70", "44", "19/09/2026", "", `[{"account":"17","amount":1}]`)
	if err == nil || !strings.Contains(err.Error(), "cash side") {
		t.Fatalf("mismatched --account should fail, got %v", err)
	}
	// Same account via name + id resolves equal and proceeds.
	if _, err := ReplayCreditCardCredit(context.Background(),
		"Rewards Card", "70", "19/09/2026", "", `[{"account":"17","amount":1}]`); err != nil {
		t.Fatalf("equivalent alias: %v", err)
	}
}

func TestCreditCardCreditGraphQLError(t *testing.T) {
	cccServer(t, `{"errors":[{"message":"Transaction could not be saved"}]}`)
	_, err := ReplayCreditCardCredit(context.Background(),
		"70", "", "19/09/2026", "", `[{"account":"17","amount":1}]`)
	if err == nil || !strings.Contains(err.Error(), "could not be saved") {
		t.Fatalf("graphql error should surface, got %v", err)
	}
}
