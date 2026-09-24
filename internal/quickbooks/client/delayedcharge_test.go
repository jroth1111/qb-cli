package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The delayed charge/credit contracts: POST /api/v4/graphql running
// UpdateTransactions_Transaction_qbo with type ACTIVITY_CHARGE (txnTypeId 25,
// Referer /app/nonpostingcharge) or ACTIVITY_CREDIT (txnTypeId 13,
// /app/nonpostingcredit). itemLines carry ITEM_client_N ids and
// traits.item{quantity, rateType PER_QTY_RATE, rate, item{id,itemType,name}}.
// Captured on TC2; v3 has neither entity on this build.
func delayedServer(t *testing.T, gqlResp string) *domServer {
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
			switch {
			case strings.Contains(q, "from Customer"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Customer":[` +
					`{"Id":"1","DisplayName":"CR-Test-Customer-Edited"}]}}`))
			case strings.Contains(q, "from Item"):
				_, _ = w.Write([]byte(`{"QueryResponse":{"Item":[` +
					`{"Id":"1","Name":"Services","FullyQualifiedName":"Services","Type":"Service"}]}}`))
			default:
				_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func delayedOK(nodeID string) string {
	return `{"data":{"updateTransactions_Transaction":{"clientMutationId":"0",` +
		`"transactionsTransaction":{"id":"` + nodeID + `","entityVersion":"1"}}}}`
}

func TestDelayedChargeWireShape(t *testing.T) {
	srv := delayedServer(t, delayedOK("djQuMTo5MzQxNDU3NzY5NzU2ODU0OjgwMjcxZWRkOGE:191"))
	res, err := ReplayDelayedCharge(context.Background(), "charge",
		"CR-Test-Customer-Edited", "19/09/2026",
		`[{"item":"Services","qty":2,"rate":15,"description":"hold"}]`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Op != "create" || res.Entity != "DelayedCharge" || res.Item.ID != "191" || res.Item.Amount != 30 {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/api/v4/graphql") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	s := string(body)
	for _, want := range []string{
		"UpdateTransactions_Transaction_qbo",
		`"transactionType":"ACTIVITY_CHARGE"`,
		`"type":"ACTIVITY_CHARGE"`,
		`"txnTypeId":"25"`,
		`"txnDate":"2026-09-19"`,
		`"id":"ITEM_client_0"`,
		`"rateType":"PER_QTY_RATE"`,
		`"quantity":"2"`,
		`"rate":"15"`,
		`"amount":"30.00"`,
		`"itemType":"SERVICE"`,
		`"localId":"1"`,
		`"displayName":"CR-Test-Customer-Edited"`,
		`"acceptStatus":"PENDING"`,
		`"entityVersion":"0"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.1400s", want, s)
		}
	}
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
	if len(sent.Variables.Input.Txn.Lines.ItemLines) != 1 || len(sent.Variables.Input.Txn.Lines.AccountLines) != 0 {
		t.Fatalf("lines = %+v", sent.Variables.Input.Txn.Lines)
	}
}

func TestDelayedCreditWireShape(t *testing.T) {
	srv := delayedServer(t, delayedOK("djQuMTo5MzQxNDU3NzY5NzU2ODU0OjgwMjcxZWRkOGE:192"))
	res, err := ReplayDelayedCharge(context.Background(), "credit",
		"1", "2026-09-19",
		`[{"item":"1","qty":1,"rate":40}]`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Entity != "DelayedCredit" || res.Item.ID != "192" || res.Item.Amount != 40 {
		t.Fatalf("result = %+v", res)
	}
	_, _, _, body := srv.snap()
	s := string(body)
	for _, want := range []string{
		`"transactionType":"ACTIVITY_CREDIT"`,
		`"type":"ACTIVITY_CREDIT"`,
		`"txnTypeId":"13"`,
		`"amount":"40.00"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.1200s", want, s)
		}
	}
}

func TestDelayedChargeValidation(t *testing.T) {
	delayedServer(t, delayedOK("ns:1"))
	if _, err := ReplayDelayedCharge(context.Background(), "charge", "", "19/09/2026", `[{"item":"Services","rate":1}]`); err == nil || !strings.Contains(err.Error(), "--customer") {
		t.Fatalf("missing customer should fail, got %v", err)
	}
	if _, err := ReplayDelayedCharge(context.Background(), "charge", "1", "", `[{"item":"Services","rate":1}]`); err == nil || !strings.Contains(err.Error(), "-date") {
		t.Fatalf("missing date should fail, got %v", err)
	}
	if _, err := ReplayDelayedCharge(context.Background(), "charge", "1", "19/09/2026", ""); err == nil || !strings.Contains(err.Error(), "--line-items") {
		t.Fatalf("missing line-items should fail, got %v", err)
	}
	if _, err := ReplayDelayedCharge(context.Background(), "charge", "1", "19/09/2026", `[{"rate":5}]`); err == nil || !strings.Contains(err.Error(), "missing item") {
		t.Fatalf("line without item should fail, got %v", err)
	}
	if _, err := ReplayDelayedCharge(context.Background(), "charge", "1", "19/09/2026", `[{"item":"Services"}]`); err == nil || !strings.Contains(err.Error(), "rate or amount") {
		t.Fatalf("line without rate/amount should fail, got %v", err)
	}
	if _, err := ReplayDelayedCharge(context.Background(), "bogus", "1", "19/09/2026", `[{"item":"Services","rate":1}]`); err == nil || !strings.Contains(err.Error(), "charge or credit") {
		t.Fatalf("bad kind should fail, got %v", err)
	}
}

func TestDelayedChargeAmountDefaults(t *testing.T) {
	srv := delayedServer(t, delayedOK("ns:9"))
	res, err := ReplayDelayedCharge(context.Background(), "charge", "1", "19/09/2026",
		`[{"item":"Services","qty":3,"rate":10},{"item":"1","amount":7.5}]`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Item.Amount != 37.5 {
		t.Fatalf("amount = %v, want 37.5 (qty*rate + explicit)", res.Item.Amount)
	}
	_, _, _, body := srv.snap()
	s := string(body)
	if !strings.Contains(s, `"amount":"37.50"`) {
		t.Fatalf("header amount missing: %.800s", s)
	}
	if !strings.Contains(s, "ITEM_client_0") || !strings.Contains(s, "ITEM_client_1") {
		t.Fatalf("expected two ITEM_client_N lines: %.800s", s)
	}
}

func TestDelayedChargeGraphQLError(t *testing.T) {
	delayedServer(t, `{"errors":[{"message":"Transaction could not be saved"}]}`)
	_, err := ReplayDelayedCharge(context.Background(), "charge", "1", "19/09/2026", `[{"item":"Services","rate":1}]`)
	if err == nil || !strings.Contains(err.Error(), "could not be saved") {
		t.Fatalf("graphql error should surface, got %v", err)
	}
}
