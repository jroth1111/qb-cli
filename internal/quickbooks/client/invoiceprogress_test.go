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

// The progress-invoice contract: POST /api/v4/graphql running
// UpdateTransactions_Transaction_qbo with type SALE_INVOICE, txnTypeId "4",
// itemLines carrying qboAppData.detailType ESTIMATE_INVOICE_LINK +
// traits.item.progressTracking PERCENT, and links.sources at line and
// transaction level consuming the estimate. The estimate is read first via
// `query c($id: ID!) { node(id:$id) { ... on Transactions_Transaction ... } }`
// where lines.itemLines is a Relay connection (edges[].node). Captured on TC2
// (estimate 178 → invoice 179 at 50%).
const estNodeFixture = `{"data":{"node":{
  "id":"djQuMTo5MzQxNDU3NzY5NzU2ODU0OjgwMjcxZWRkOGE:178","type":"SALE_ESTIMATE",
  "header":{"amount":"200.00","txnDate":"2026-09-19","voided":false,
    "currencyInfo":{"symbol":"A$","homeAmount":null,"code":"AUD","exchangeRate":"1.00","name":"Australian Dollar"},
    "contact":{"id":"djQuMTo5MzQxNDU3NzY5NzU2ODU0OjlkNjk5ZTk2MDg:c1","displayName":"Cool Cars",
      "externalIds":[{"namespaceId":"intuit.qbo.name.id","localId":"1"}]}},
  "traits":{"balance":{"consumableBalance":"150.00","balance":"0","dueDate":null}},
  "lines":{"itemLines":{"edges":[{"node":{
    "id":"djQuMTo5MzQxNDU3NzY5NzU2ODU0OjM2MGY2MDlmNTU:178-1",
    "sequence":"1","amount":"200.00","description":"Progress test service",
    "contact":{"id":"djQuMTo5MzQxNDU3NzY5NzU2ODU0OjlkNjk5ZTk2MDg:c1",
      "externalIds":[{"namespaceId":"intuit.qbo.name.id","localId":"1"}]},
    "traits":{
      "item":{"rateType":"PER_QTY_RATE","item":{"id":"djQuMTo5MzQxNDU3NzY5NzU2ODU0OjExMmRlNzQ2OTk:1",
        "name":"Services","fullName":"Services","itemType":"SERVICE"}},
      "purchaseOrder":{"closed":null,"quantityReceived":null,"amountReceived":"50.00"},
      "billable":{"billableTo":null,"billable":null,"salesAmt":null}}
  }}]}},
  "links":{"targets":{"edges":[]}}
}}}`

func progOK(nodeID string) string {
	return `{"data":{"updateTransactions_Transaction":{"clientMutationId":"0",` +
		`"transactionsTransaction":{"id":"` + nodeID + `","entityVersion":"1"}}}}`
}

// progServer answers the estimate node read, the mutation, and the optional
// v3 DocNumber lookup. gqlRead/mutErrs override the defaults for error tests.
func progServer(t *testing.T, nodeResp, mutResp string) *domServer {
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
			if strings.Contains(string(b), "UpdateTransactions_Transaction") {
				_, _ = w.Write([]byte(mutResp))
				return
			}
			_, _ = w.Write([]byte(nodeResp))
		case strings.Contains(r.URL.Path, "/v3/company/"):
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			if strings.Contains(q, "Estimate") {
				_, _ = w.Write([]byte(`{"QueryResponse":{"Estimate":[{"Id":"178","DocNumber":"EST-9001"}]}}`))
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

func TestInvoiceProgressWireShape(t *testing.T) {
	srv := progServer(t, estNodeFixture, progOK("djQuMTo5MzQxNDU3NzY5NzU2ODU0OjgwMjcxZWRkOGE:180"))
	res, err := ReplayInvoiceProgress(context.Background(), "178", "25", "")
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	// 25% of the $200 line = $50; consumable 150 - 50 = 100 remaining.
	if res.Op != "progress" || res.Entity != "Invoice" || res.Item.ID != "180" || res.Item.Amount != 50 {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(res.Note, "25% of estimate 178") || !strings.Contains(res.Note, "remaining 100.00") {
		t.Fatalf("note = %q", res.Note)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "/api/v4/graphql") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	s := string(body)
	for _, want := range []string{
		"UpdateTransactions_Transaction_qbo",
		`"transactionType":"SALE_INVOICE"`,
		`"type":"SALE_INVOICE"`,
		`"txnTypeId":"4"`,
		`"detailType":"ESTIMATE_INVOICE_LINK"`,
		`"progressTracking":"PERCENT"`,
		`"id":"ITEM_client_0"`,
		`"amount":"50.00"`,
		`"description":"Progress test service"`,
		`"totalAmountConsumed":"100.00"`,
		`"consumableBalance":"100.00"`,
		`"type":"SALE_ESTIMATE"`,
		`"entityVersion":"0"`,
		`"displayName":"Cool Cars"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("body missing %s: %.1500s", want, s)
		}
	}
	// Draft id must be the transaction-namespace :-1 sentinel; link id must
	// embed estimate-line-invoice keying.
	var sent struct {
		Variables struct {
			Input struct {
				Txn struct {
					ID    string `json:"id"`
					Lines struct {
						ItemLines []map[string]any `json:"itemLines"`
					} `json:"lines"`
					Links struct {
						Sources []map[string]any `json:"sources"`
					} `json:"links"`
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
	if len(sent.Variables.Input.Txn.Lines.ItemLines) != 1 {
		t.Fatalf("itemLines = %+v", sent.Variables.Input.Txn.Lines.ItemLines)
	}
	if len(sent.Variables.Input.Txn.Links.Sources) != 1 {
		t.Fatalf("txn links.sources = %+v", sent.Variables.Input.Txn.Links.Sources)
	}
	src := sent.Variables.Input.Txn.Links.Sources[0]
	srcTxn, _ := src["sourceTxn"].(map[string]any)
	if srcTxn["type"] != "SALE_ESTIMATE" || !strings.HasSuffix(str(srcTxn, "id"), ":178") {
		t.Fatalf("sourceTxn = %+v", srcTxn)
	}
}

func TestInvoiceProgressTotalMode(t *testing.T) {
	srv := progServer(t, estNodeFixture, progOK("ns:181"))
	res, err := ReplayInvoiceProgress(context.Background(), "178", "", "")
	if err != nil {
		t.Fatalf("progress: %v", err)
	}
	// Default 100% bills the line remainder: 200 - 50 consumed = 150.
	if res.Item.Amount != 150 || !strings.Contains(res.Note, "remaining 0.00") {
		t.Fatalf("result = %+v", res)
	}
	_, _, _, body := srv.snap()
	if !strings.Contains(string(body), `"totalAmountConsumed":"200.00"`) {
		t.Fatalf("total mode should fully consume the line: %.400s", body)
	}
}

func TestInvoiceProgressIDAlias(t *testing.T) {
	srv := progServer(t, estNodeFixture, progOK("ns:182"))
	res, err := ReplayInvoiceProgress(context.Background(), "", "10", "178")
	if err != nil {
		t.Fatalf("alias: %v", err)
	}
	if res.Item.Amount != 20 { // 10% of 200
		t.Fatalf("amount = %v", res.Item.Amount)
	}
	_ = srv
}

func TestInvoiceProgressDocNumber(t *testing.T) {
	progServer(t, estNodeFixture, progOK("ns:183"))
	res, err := ReplayInvoiceProgress(context.Background(), "EST-9001", "25", "")
	if err != nil {
		t.Fatalf("doc-number resolution: %v", err)
	}
	if res.Item.ID != "183" {
		t.Fatalf("id = %q", res.Item.ID)
	}
}

func TestInvoiceProgressValidation(t *testing.T) {
	progServer(t, estNodeFixture, progOK("ns:1"))
	if _, err := ReplayInvoiceProgress(context.Background(), "", "25", ""); err == nil || !strings.Contains(err.Error(), "--estimate") {
		t.Fatalf("missing estimate should fail, got %v", err)
	}
	for _, bad := range []string{"0", "-5", "150", "abc", "%"} {
		if _, err := ReplayInvoiceProgress(context.Background(), "178", bad, ""); err == nil || !strings.Contains(err.Error(), "--percent") {
			t.Fatalf("percent %q should fail, got %v", bad, err)
		}
	}
}

func TestInvoiceProgressFullyConsumed(t *testing.T) {
	consumed := strings.Replace(estNodeFixture, `"consumableBalance":"150.00"`, `"consumableBalance":"0.00"`, 1)
	progServer(t, consumed, progOK("ns:1"))
	_, err := ReplayInvoiceProgress(context.Background(), "178", "25", "")
	if err == nil || !strings.Contains(err.Error(), "fully invoiced") {
		t.Fatalf("consumed estimate should fail, got %v", err)
	}
}

func TestInvoiceProgressExceedsRemaining(t *testing.T) {
	// Line is $200 with $150 consumed; 50% of full ($100) exceeds the $50 left.
	partial := strings.Replace(estNodeFixture, `"amountReceived":"50.00"`, `"amountReceived":"150.00"`, 1)
	progServer(t, partial, progOK("ns:1"))
	_, err := ReplayInvoiceProgress(context.Background(), "178", "50", "")
	if err == nil || !strings.Contains(err.Error(), "exceeds remaining billable") {
		t.Fatalf("over-billing should fail, got %v", err)
	}
}

func TestInvoiceProgressWrongType(t *testing.T) {
	inv := strings.Replace(estNodeFixture, `"type":"SALE_ESTIMATE"`, `"type":"SALE_INVOICE"`, 1)
	progServer(t, inv, progOK("ns:1"))
	_, err := ReplayInvoiceProgress(context.Background(), "178", "25", "")
	if err == nil || !strings.Contains(err.Error(), "not SALE_ESTIMATE") {
		t.Fatalf("non-estimate node should fail, got %v", err)
	}
}

func TestInvoiceProgressMissingNode(t *testing.T) {
	progServer(t, `{"data":{"node":null}}`, progOK("ns:1"))
	_, err := ReplayInvoiceProgress(context.Background(), "999", "25", "")
	if err == nil || !strings.Contains(err.Error(), "no v4 transaction node") {
		t.Fatalf("missing estimate should fail, got %v", err)
	}
}

func TestInvoiceProgressGraphQLErrors(t *testing.T) {
	// Read-side error surfaces with the estimate id.
	progServer(t, `{"errors":[{"message":"node read denied"}]}`, progOK("ns:1"))
	if _, err := ReplayInvoiceProgress(context.Background(), "178", "25", ""); err == nil || !strings.Contains(err.Error(), "node read denied") {
		t.Fatalf("read error should surface, got %v", err)
	}
	// Mutation-side error surfaces through the invoice-progress wrapper.
	progServer(t, estNodeFixture, `{"errors":[{"message":"Transaction could not be saved"}]}`)
	if _, err := ReplayInvoiceProgress(context.Background(), "178", "25", ""); err == nil || !strings.Contains(err.Error(), "could not be saved") {
		t.Fatalf("mutation error should surface, got %v", err)
	}
}
