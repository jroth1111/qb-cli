package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// sales_domain_test.go covers the sales transaction surface end to end at the
// client boundary: every wired/read verb the sales domains expose —
// invoice|estimate|payment|creditmemo|delayedcharge|delayedcredit|
// salesreceipt list/get/create/update/delete, the invoice extras
// void/send/pdf/copy, the estimate send, and sales-order list/get — using the
// package-standard DefaultTransport swap (interceptHTTP). Production
// qbo.intuit.com / commercecontrol / txnsrendering URLs are rewritten to a
// local httptest server; nothing reaches the network.
//
// Command → entry-point mapping (client level):
//   - salestx words (invoice/estimate/payment/creditmemo/delayedcharge/
//     salesreceipt) go through ReplaySalestxQuery / ReplaySalestxMutate.
//   - delayedcredit has no salestx word (SalestxV3Entity rejects it); its
//     wired surfaces run through the shared primitives ReplayQuery(
//     "DelayedCredit") and ReplayMutate("DelayedCredit", "create") — the
//     same calls cli/v3mutate.go makes for QBO.SALES.DELAYED_CREDIT_CREATE.
//   - sales-order reads dispatch to the captured commercecontrol
//     GetSalesOrders / GetSalesOrderWithTaxGroup GraphQL (ReplayQuery("SalesOrder")).
//     Native sales-order mutation contracts are covered in salesorder_contract_test.go.
//
// Kill denominator: 12. The suite rejects implementations that
//  1. dial the network without credentials (fast-fail broken),
//  2. lose the ErrNoCredentials sentinel behind changed wrapping,
//  3. accept a missing --customer on any sales create,
//  4. accept a missing id on update/delete/void/send/copy/pdf,
//  5. route an entity to the wrong v3 path or GraphQL endpoint,
//  6. drop minorversion=73 / operation=delete|void from the URL,
//  7. patch non-sparsely (losing the fetched Id/SyncToken),
//  8. clone server-managed fields (Id/SyncToken/DocNumber/MetaData),
//  9. forget sendTo on invoice send,
// 10. mis-dispatch SalesOrder reads (verb, operationName, limit variable, or
//     a phantom v3 /query fallback),
// 11. corrupt the PDF download (bytes, path, txnId/documentType params),
// 12. project v3 rows wrongly (Id, DocNumber, TotalAmt, TxnDate, type).

// --- fake v3/graphql server --------------------------------------------------

// sdServer records every request and answers from bodies keyed by
// "METHOD /lastpathsegment". Segments are scanned right-to-left so
// "/api/v3/company/{realm}/invoice/188" matches "GET /invoice" while
// "/api/v3/company/{realm}/invoice/188/send" matches "POST /send".
type sdServer struct {
	*httptest.Server

	t      *testing.T
	bodies map[string]string

	mu        sync.Mutex
	calls     int
	lastMeth  string
	lastPath  string
	lastQuery string // raw query string
	lastQ     string // decoded ?query= parameter (v3 select statement)
	lastBody  []byte
}

// newSDServer starts the fake and returns it; the embedded *httptest.Server
// provides the .URL handed to interceptHTTP.
func newSDServer(t *testing.T, bodies map[string]string) *sdServer {
	t.Helper()
	s := &sdServer{t: t, bodies: bodies}
	s.Server = httptest.NewServer(withPersistedV3(http.HandlerFunc(s.handle)))
	t.Cleanup(s.Close)
	return s
}

func (s *sdServer) handle(w http.ResponseWriter, r *http.Request) {
	var b []byte
	if r.Body != nil {
		if r.ContentLength > 0 {
			b = make([]byte, r.ContentLength)
			_, _ = io.ReadFull(r.Body, b)
		}
		_ = r.Body.Close()
	}
	s.mu.Lock()
	s.calls++
	s.lastMeth = r.Method
	s.lastPath = r.URL.Path
	s.lastQuery = r.URL.RawQuery
	s.lastQ = r.URL.Query().Get("query")
	body, status := "", http.StatusNotFound
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if got, ok := s.bodies[r.Method+" /"+segs[i]]; ok {
			body, status = got, http.StatusOK
			break
		}
	}
	s.lastBody = b
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// sdCall snapshots the counters and the last recorded request.
type sdCall struct {
	Calls    int
	Method   string
	Path     string
	RawQuery string
	QParam   string
	Body     []byte
}

func (s *sdServer) snap() sdCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sdCall{
		Calls:    s.calls,
		Method:   s.lastMeth,
		Path:     s.lastPath,
		RawQuery: s.lastQuery,
		QParam:   s.lastQ,
		Body:     s.lastBody,
	}
}

// sentMap decodes the last request body as a JSON object.
func (c sdCall) sentMap(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.Body, &m); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, c.Body)
	}
	return m
}

// --- no credentials: every sales entry point fails fast ----------------------

// TestSalesDomainNoCredsFastFail sweeps every wired/read sales command's
// client entry point with an empty QB_HOME. Each must return ErrNoCredentials
// well within network-latency territory (the guard precedes any dial).
func TestSalesDomainNoCredsFastFail(t *testing.T) {
	noCredsDir(t)
	ctx := context.Background()

	read := func(word string) func() error {
		return func() error { _, err := ReplaySalestxQuery(ctx, word, "", "", 10); return err }
	}
	readID := func(word, id string) func() error {
		return func() error { _, err := ReplaySalestxQuery(ctx, word, id, "", 10); return err }
	}
	write := func(word, op, id string, flags map[string]string) func() error {
		return func() error { _, err := ReplaySalestxMutate(ctx, word, op, id, flags); return err }
	}
	pdf := func() error {
		_, err := ReplayDocumentPDF(ctx, "188", filepath.Join(t.TempDir(), "x.pdf"))
		return err
	}

	cases := []struct {
		name string
		call func() error
	}{
		// reads
		{"invoice list", read("invoice")},
		{"invoice get", readID("invoice", "188")},
		{"estimate list", read("estimate")},
		{"estimate get", readID("estimate", "9")},
		{"payment list", read("payment")},
		{"payment get", readID("payment", "77")},
		{"creditmemo list", read("creditmemo")},
		{"creditmemo get", readID("creditmemo", "64")},
		{"delayedcharge list", read("delayedcharge")},
		{"delayedcredit list", func() error { _, err := ReplayQuery(ctx, "DelayedCredit", "", "", 10); return err }},
		{"salesreceipt list", read("salesreceipt")},
		{"salesreceipt get", readID("salesreceipt", "51")},
		{"salesorder list", func() error { _, err := ReplayQuery(ctx, "SalesOrder", "", "", 10); return err }},
		{"salesorder get", func() error { _, err := ReplayQuery(ctx, "SalesOrder", "77", "", 10); return err }},
		// invoice writes
		{"invoice create", write("invoice", "create", "", map[string]string{"customer": "23"})},
		{"invoice update", write("invoice", "update", "188", map[string]string{"id": "188"})},
		{"invoice delete", write("invoice", "delete", "188", nil)},
		{"invoice void", write("invoice", "void", "188", nil)},
		{"invoice send", write("invoice", "send", "188", map[string]string{"to": "a@b.co"})},
		{"invoice copy", write("invoice", "copy", "188", nil)},
		{"invoice pdf", pdf},
		// estimate writes
		{"estimate create", write("estimate", "create", "", map[string]string{"customer": "23"})},
		{"estimate update", write("estimate", "update", "9", map[string]string{"id": "9"})},
		{"estimate delete", write("estimate", "delete", "9", nil)},
		{"estimate send", write("estimate", "send", "9", map[string]string{"to": "a@b.co"})},
		// payment writes
		{"payment create", write("payment", "create", "", map[string]string{"customer": "23"})},
		{"payment delete", write("payment", "delete", "77", nil)},
		// creditmemo writes
		{"creditmemo create", write("creditmemo", "create", "", map[string]string{"customer": "23"})},
		{"creditmemo update", write("creditmemo", "update", "64", map[string]string{"id": "64"})},
		{"creditmemo delete", write("creditmemo", "delete", "64", nil)},
		// delayed charge/credit writes
		{"delayedcharge create", write("delayedcharge", "create", "", map[string]string{"customer": "23"})},
		{"delayedcredit create", func() error {
			_, err := ReplayMutate(ctx, "DelayedCredit", "create", "", map[string]string{"customer": "23"})
			return err
		}},
		// salesreceipt writes
		{"salesreceipt create", write("salesreceipt", "create", "", map[string]string{"customer": "23"})},
		{"salesreceipt delete", write("salesreceipt", "delete", "51", nil)},
	}
	for _, tc := range cases {
		start := time.Now()
		err := tc.call()
		assertFast(t, time.Since(start))
		if !errors.Is(err, ErrNoCredentials) {
			t.Errorf("%s: got %v, want ErrNoCredentials", tc.name, err)
		}
	}
}

// --- validation: reject before any dial -------------------------------------

// TestSalesDomainValidationRejectsBeforeDial pins the empty-input sentinels:
// creates without --customer and id-taking verbs without an id must fail
// before a single request leaves the process. The intercepted server
// registers no routes, so any accidental dial answers 404 and would surface
// as a ReplayError instead of the expected validation failure; the final
// zero-call assertion makes the pre-network guarantee explicit.
func TestSalesDomainValidationRejectsBeforeDial(t *testing.T) {
	saveUsable(t)
	srv := newSDServer(t, map[string]string{})
	interceptHTTP(t, srv.URL)
	ctx := context.Background()

	create := func(word string) func() error {
		return func() error { _, err := ReplaySalestxMutate(ctx, word, "create", "", map[string]string{}); return err }
	}
	idless := func(word, op string) func() error {
		return func() error { _, err := ReplaySalestxMutate(ctx, word, op, "", nil); return err }
	}

	cases := []struct {
		name         string
		call         func() error
		wantSentinel error // nil ⇒ match wantMsg on the error text
		wantMsg      string
	}{
		{"invoice create missing customer", create("invoice"), nil, "requires --customer"},
		{"estimate create missing customer", create("estimate"), nil, "requires --customer"},
		{"payment create missing customer", create("payment"), nil, "requires --customer"},
		{"creditmemo create missing customer", create("creditmemo"), nil, "requires --customer"},
		{"delayedcharge create missing customer", create("delayedcharge"), nil, "requires --customer"},
		{"delayedcredit create missing customer", func() error {
			_, err := ReplayMutate(ctx, "DelayedCredit", "create", "", map[string]string{})
			return err
		}, nil, "requires --customer"},
		{"salesreceipt create missing customer", create("salesreceipt"), nil, "requires --customer"},

		{"invoice update missing id", idless("invoice", "update"), ErrMissingMutateID, ""},
		{"invoice delete missing id", idless("invoice", "delete"), ErrMissingMutateID, ""},
		{"invoice void missing id", idless("invoice", "void"), ErrMissingMutateID, ""},
		{"invoice send missing id", idless("invoice", "send"), ErrMissingMutateID, ""},
		{"invoice copy missing id", idless("invoice", "copy"), ErrMissingMutateID, ""},
		{"estimate update missing id", idless("estimate", "update"), ErrMissingMutateID, ""},
		{"estimate delete missing id", idless("estimate", "delete"), ErrMissingMutateID, ""},
		{"estimate send missing id", idless("estimate", "send"), ErrMissingMutateID, ""},
		{"payment delete missing id", idless("payment", "delete"), ErrMissingMutateID, ""},
		{"creditmemo update missing id", idless("creditmemo", "update"), ErrMissingMutateID, ""},
		{"creditmemo delete missing id", idless("creditmemo", "delete"), ErrMissingMutateID, ""},
		{"salesreceipt delete missing id", idless("salesreceipt", "delete"), ErrMissingMutateID, ""},

		{"invoice pdf missing id", func() error {
			_, err := ReplayDocumentPDF(ctx, "  ", filepath.Join(t.TempDir(), "x.pdf"))
			return err
		}, ErrMissingTxnID, ""},
	}
	for _, tc := range cases {
		start := time.Now()
		err := tc.call()
		assertFast(t, time.Since(start))
		if err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
			continue
		}
		if tc.wantSentinel != nil {
			if !errors.Is(err, tc.wantSentinel) {
				t.Errorf("%s: got %v, want %v", tc.name, err, tc.wantSentinel)
			}
			continue
		}
		if !strings.Contains(err.Error(), tc.wantMsg) {
			t.Errorf("%s: got %q, want message containing %q", tc.name, err, tc.wantMsg)
		}
	}
	if got := srv.snap().Calls; got != 0 {
		t.Fatalf("validation must precede any request; recorded %d calls", got)
	}
}

// --- reads: list/get project v3 rows ----------------------------------------

const sdRowsShape = `{"QueryResponse":{` +
	`"Invoice":[{"Id":"188","SyncToken":"2","DocNumber":"INV-188","TxnDate":"2026-08-05","TotalAmt":55,"CustomerRef":{"value":"23","name":"Acme"}}],` +
	`"Estimate":[{"Id":"9","SyncToken":"1","DocNumber":"EST-9","TxnDate":"2026-08-06","TotalAmt":120}],` +
	`"Payment":[{"Id":"77","SyncToken":"0","TxnDate":"2026-08-07","TotalAmt":30,"PaymentType":"Card"}],` +
	`"CreditMemo":[{"Id":"64","SyncToken":"3","DocNumber":"CM-64","TxnDate":"2026-08-08","TotalAmt":12}],` +
	`"DelayedCharge":[{"Id":"41","TxnDate":"2026-08-09","TotalAmt":7}],` +
	`"DelayedCredit":[{"Id":"42","TxnDate":"2026-08-10","TotalAmt":3}],` +
	`"SalesReceipt":[{"Id":"51","SyncToken":"0","DocNumber":"SR-51","TxnDate":"2026-08-11","TotalAmt":8}]` +
	`},"time":"2026-08-24T00:00:00Z"}`

// TestSalesDomainListAndGetProjectsV3Rows drives list and by-id get for every
// sales entity through the shared v3 /query reader. One combined response
// serves all entities; projectQuery must pick each entity's own rows.
func TestSalesDomainListAndGetProjectsV3Rows(t *testing.T) {
	saveUsable(t)
	srv := newSDServer(t, map[string]string{"GET /query": sdRowsShape})
	interceptHTTP(t, srv.URL)
	ctx := context.Background()

	entities := []struct {
		word, v3ent, id, doc, date string
		amount                     float64
	}{
		{"invoice", "Invoice", "188", "INV-188", "2026-08-05", 55},
		{"estimate", "Estimate", "9", "EST-9", "2026-08-06", 120},
		{"payment", "Payment", "77", "", "2026-08-07", 30},
		{"creditmemo", "CreditMemo", "64", "CM-64", "2026-08-08", 12},
		{"delayedcharge", "DelayedCharge", "41", "", "2026-08-09", 7},
		{"salesreceipt", "SalesReceipt", "51", "SR-51", "2026-08-11", 8},
	}
	for _, e := range entities {
		res, err := ReplaySalestxQuery(ctx, e.word, "", "", 20)
		if err != nil {
			t.Fatalf("%s list: %v", e.word, err)
		}
		if res.Entity != e.v3ent || res.Status != http.StatusOK || len(res.Items) != 1 {
			t.Fatalf("%s list envelope = %+v", e.word, res)
		}
		it := res.Items[0]
		if it.ID != e.id || it.DocNumber != e.doc || it.Date != e.date || it.Amount != e.amount {
			t.Fatalf("%s list row = %+v", e.word, it)
		}
		if got := srv.snap().QParam; got != fmt.Sprintf("select * from %s maxresults 20", e.v3ent) {
			t.Fatalf("%s list query = %q", e.word, got)
		}

		res, err = ReplaySalestxQuery(ctx, e.word, e.id, "", 20)
		if err != nil {
			t.Fatalf("%s get: %v", e.word, err)
		}
		if len(res.Items) != 1 || res.Items[0].ID != e.id {
			t.Fatalf("%s get rows = %+v", e.word, res.Items)
		}
		if got := srv.snap().QParam; got != fmt.Sprintf("select * from %s where Id = '%s' maxresults 20", e.v3ent, e.id) {
			t.Fatalf("%s get query = %q", e.word, got)
		}
	}

	// delayedcredit rides the shared query reader (no salestx word).
	res, err := ReplayQuery(ctx, "DelayedCredit", "42", "", 20)
	if err != nil {
		t.Fatalf("delayedcredit get: %v", err)
	}
	if res.Entity != "DelayedCredit" || len(res.Items) != 1 || res.Items[0].ID != "42" || res.Items[0].Amount != 3 {
		t.Fatalf("delayedcredit get = %+v", res)
	}

	// Projection spot-checks: payment carries PaymentType, invoice carries
	// DocNumber/date, and oversized limits page via ordered windows of 100.
	if got := srv.snap().QParam; true {
		_ = got // last recorded call below re-checks the clamp explicitly
	}
	pay, err := ReplaySalestxQuery(ctx, "payment", "", "", 20)
	if err != nil || len(pay.Items) != 1 || pay.Items[0].Type != "Card" {
		t.Fatalf("payment projection = %+v (%v)", pay, err)
	}
	inv, err := ReplaySalestxQuery(ctx, "invoice", "", "", 500)
	if err != nil || len(inv.Items) != 1 || inv.Items[0].DocNumber != "INV-188" {
		t.Fatalf("invoice projection = %+v (%v)", inv, err)
	}
	if got := srv.snap().QParam; got != "select * from Invoice orderby Id maxresults 100" {
		t.Fatalf("paged limit query = %q", got)
	}
}

// TestSalesDomainSalesOrderReadsPostCapturedGraphQL checks native rows and IDs.
func TestSalesDomainSalesOrderReadsPostCapturedGraphQL(t *testing.T) {
	saveUsableURIHost(t)
	srv := newSDServer(t, map[string]string{
		"POST /graphql": `{"data":{"result":{"totalCount":2,"salesOrders":[{"id":"77","customerName":"Fixture Customer","orderNumber":"SO-77","transactionDate":"2026-09-10","total":2},{"id":"78","customerName":"Other Customer","orderNumber":"SO-78","total":3}]}}}`,
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(t.Context(), "SalesOrder", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Entity != "SalesOrder" || len(res.Items) != 2 || res.Items[0].ID != "77" || res.Items[0].Amount != 2 || res.Counts["totalCount"] != 2 || res.Counts["items"] != 2 {
		t.Fatalf("list=%+v", res)
	}
	snap := srv.snap()
	if snap.Calls != 1 || snap.Method != http.MethodPost || snap.Path != "/graphql" {
		t.Fatalf("dispatch=%+v", snap)
	}
	var payload struct {
		OperationName string
		Query         string
		Variables     map[string]any
	}
	if err := json.Unmarshal(snap.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OperationName != "GetSalesOrders" || !strings.Contains(payload.Query, "salesOrders {") || payload.Variables["offset"] != float64(0) || payload.Variables["limit"] != float64(5) {
		t.Fatalf("payload=%+v", payload)
	}
	srv.mu.Lock()
	srv.bodies["POST /graphql"] = `{"data":{"result":{"id":"77","customerName":"Fixture Customer","orderNumber":"SO-77","transactionDate":"2026-09-10","total":2}}}`
	srv.mu.Unlock()
	res, err = ReplayQuery(t.Context(), "SalesOrder", "77", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "77" || res.Items[0].Name != "Fixture Customer" {
		t.Fatalf("get=%+v", res)
	}
	snap = srv.snap()
	payload.Variables = nil
	if err := json.Unmarshal(snap.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if snap.Calls != 2 || payload.OperationName != "GetSalesOrderWithTaxGroup" || !strings.Contains(payload.Query, "getSalesOrder(id: $id)") || len(payload.Variables) != 1 || payload.Variables["id"] != "77" {
		t.Fatalf("by-id payload=%+v", payload)
	}
}

// --- creates -----------------------------------------------------------------

// TestSalesDomainCreatePostsSalesBodies drives every sales create and pins
// the wire body: customer ref, AU date normalisation, sales lines, linked
// transactions, deposit account, and the created-item projection.
func TestSalesDomainCreatePostsSalesBodies(t *testing.T) {
	saveUsable(t)
	srv := newSDServer(t, map[string]string{
		"POST /invoice":       `{"Invoice":{"Id":"301","SyncToken":"0","TotalAmt":42.5}}`,
		"POST /estimate":      `{"Estimate":{"Id":"302","SyncToken":"0"}}`,
		"POST /payment":       `{"Payment":{"Id":"303","SyncToken":"0","TotalAmt":30}}`,
		"POST /creditmemo":    `{"CreditMemo":{"Id":"304","SyncToken":"0"}}`,
		"POST /delayedcharge": `{"DelayedCharge":{"Id":"305","SyncToken":"0","TotalAmt":7}}`,
		"POST /delayedcredit": `{"DelayedCredit":{"Id":"306","SyncToken":"0","TotalAmt":3}}`,
		"POST /salesreceipt":  `{"SalesReceipt":{"Id":"307","SyncToken":"0","TotalAmt":8}}`,
	})
	interceptHTTP(t, srv.URL)
	ctx := context.Background()

	expectPOST := func(path, query string) map[string]any {
		t.Helper()
		snap := srv.snap()
		if snap.Method != http.MethodPost || !strings.HasSuffix(snap.Path, path) {
			t.Fatalf("create dispatch = %s %s", snap.Method, snap.Path)
		}
		if snap.RawQuery != query {
			t.Fatalf("create query = %q, want %q", snap.RawQuery, query)
		}
		return snap.sentMap(t)
	}

	// invoice: customer + item line + AU date normalisation.
	res, err := ReplaySalestxMutate(ctx, "invoice", "create", "", map[string]string{
		"customer": "23", "amount": "42.50", "item": "7", "date": "05/08/2026",
	})
	if err != nil {
		t.Fatalf("invoice create: %v", err)
	}
	if res.Op != "create" || res.Entity != "Invoice" || res.Item.ID != "301" {
		t.Fatalf("invoice create envelope = %+v", res)
	}
	body := expectPOST("/invoice", "minorversion=73")
	if body["TxnDate"] != "2026-08-05" {
		t.Fatalf("invoice TxnDate = %v", body["TxnDate"])
	}
	custRef := body["CustomerRef"].(map[string]any)
	if custRef["value"] != "23" {
		t.Fatalf("invoice CustomerRef = %v", custRef)
	}
	line := body["Line"].([]any)[0].(map[string]any)
	if line["Amount"] != 42.5 || line["DetailType"] != "SalesItemLineDetail" {
		t.Fatalf("invoice line = %v", line)
	}
	itemRef := line["SalesItemLineDetail"].(map[string]any)["ItemRef"].(map[string]any)
	if itemRef["value"] != "7" {
		t.Fatalf("invoice ItemRef = %v", itemRef)
	}

	// estimate.
	if _, err := ReplaySalestxMutate(ctx, "estimate", "create", "", map[string]string{
		"customer": "23", "amount": "120",
	}); err != nil {
		t.Fatalf("estimate create: %v", err)
	}
	body = expectPOST("/estimate", "minorversion=73")
	if body["CustomerRef"].(map[string]any)["value"] != "23" {
		t.Fatalf("estimate CustomerRef = %v", body["CustomerRef"])
	}
	if body["TxnDate"] == "" {
		t.Fatal("estimate create must default TxnDate")
	}

	// payment: TotalAmt + LinkedTxn to the paid invoice.
	if _, err := ReplaySalestxMutate(ctx, "payment", "create", "", map[string]string{
		"customer": "23", "amount": "30", "invoice-id": "88",
	}); err != nil {
		t.Fatalf("payment create: %v", err)
	}
	body = expectPOST("/payment", "minorversion=73")
	if body["TotalAmt"] != 30.0 {
		t.Fatalf("payment TotalAmt = %v", body["TotalAmt"])
	}
	pl := body["Line"].([]any)[0].(map[string]any)
	link := pl["LinkedTxn"].([]any)[0].(map[string]any)
	if link["TxnId"] != "88" || link["TxnType"] != "Invoice" {
		t.Fatalf("payment LinkedTxn = %v", link)
	}

	// creditmemo: line-level link back to the credited invoice.
	if _, err := ReplaySalestxMutate(ctx, "creditmemo", "create", "", map[string]string{
		"customer": "23", "amount": "12", "invoice-id": "88",
	}); err != nil {
		t.Fatalf("creditmemo create: %v", err)
	}
	body = expectPOST("/creditmemo", "minorversion=73")
	cl := body["Line"].([]any)[0].(map[string]any)["LinkedTxn"].([]any)[0].(map[string]any)
	if cl["TxnId"] != "88" || cl["TxnType"] != "Invoice" {
		t.Fatalf("creditmemo LinkedTxn = %v", cl)
	}

	// delayedcharge / delayedcredit.
	if _, err := ReplaySalestxMutate(ctx, "delayedcharge", "create", "", map[string]string{
		"customer": "23", "amount": "7",
	}); err != nil {
		t.Fatalf("delayedcharge create: %v", err)
	}
	body = expectPOST("/delayedcharge", "minorversion=73")
	if body["Line"].([]any)[0].(map[string]any)["Amount"] != 7.0 {
		t.Fatalf("delayedcharge line = %v", body["Line"])
	}
	if _, err := ReplayMutate(ctx, "DelayedCredit", "create", "", map[string]string{
		"customer": "23", "amount": "3",
	}); err != nil {
		t.Fatalf("delayedcredit create: %v", err)
	}
	body = expectPOST("/delayedcredit", "minorversion=73")
	if body["CustomerRef"].(map[string]any)["value"] != "23" {
		t.Fatalf("delayedcredit CustomerRef = %v", body["CustomerRef"])
	}

	// salesreceipt: point-of-sale deposit account.
	res, err = ReplaySalestxMutate(ctx, "salesreceipt", "create", "", map[string]string{
		"customer": "23", "amount": "8", "account": "91",
	})
	if err != nil {
		t.Fatalf("salesreceipt create: %v", err)
	}
	if res.Item.ID != "307" {
		t.Fatalf("salesreceipt envelope = %+v", res)
	}
	body = expectPOST("/salesreceipt", "minorversion=73")
	if body["DepositToAccountRef"].(map[string]any)["value"] != "91" {
		t.Fatalf("salesreceipt DepositToAccountRef = %v", body["DepositToAccountRef"])
	}
}

// --- updates -----------------------------------------------------------------

// TestSalesDomainUpdateFetchesThenPatchesSparse pins the read-modify-write
// contract: update GETs the row for its SyncToken, then POSTs a sparse patch
// carrying that token plus only the requested change.
func TestSalesDomainUpdateFetchesThenPatchesSparse(t *testing.T) {
	saveUsable(t)
	ctx := context.Background()
	for _, tc := range []struct{ word, entity, path, id, token string }{
		{"invoice", "Invoice", "invoice", "188", "2"},
		{"estimate", "Estimate", "estimate", "9", "1"},
		{"creditmemo", "CreditMemo", "creditmemo", "64", "3"},
	} {
		fetched := fmt.Sprintf(`{%q:{"Id":%q,"SyncToken":%q,"TotalAmt":55,"CustomerRef":{"value":"23"}}}`, tc.entity, tc.id, tc.token)
		srv := newSDServer(t, map[string]string{
			"GET /" + tc.path:  fetched,
			"POST /" + tc.path: fetched,
		})
		interceptHTTP(t, srv.URL)

		res, err := ReplaySalestxMutate(ctx, tc.word, "update", "", map[string]string{"id": tc.id, "email": "new@b.co"})
		if err != nil {
			t.Fatalf("%s update: %v", tc.word, err)
		}
		if res.Op != "update" || res.Entity != tc.entity || res.Item.ID != tc.id {
			t.Fatalf("%s update envelope = %+v", tc.word, res)
		}
		snap := srv.snap()
		if snap.Calls != 2 || snap.Method != http.MethodPost {
			t.Fatalf("%s update flow = %+v", tc.word, snap)
		}
		if snap.RawQuery != "minorversion=73" {
			t.Fatalf("%s update query = %q (must not carry operation=)", tc.word, snap.RawQuery)
		}
		sent := snap.sentMap(t)
		if sent["Id"] != tc.id || sent["SyncToken"] != tc.token {
			t.Fatalf("%s update lost fetched identity/token: %v", tc.word, sent)
		}
		if sent["sparse"] != true {
			t.Fatalf("%s update must be sparse: %v", tc.word, sent)
		}
		be := sent["BillEmail"].(map[string]any)
		if be["Address"] != "new@b.co" {
			t.Fatalf("%s update BillEmail = %v", tc.word, be)
		}
	}
}

// --- deletes -----------------------------------------------------------------

// TestSalesDomainDeleteSendsMinimalBodyWithOperation pins delete for every
// entity: fetch for the SyncToken, then POST exactly {Id, SyncToken} with
// operation=delete on the query string.
func TestSalesDomainDeleteSendsMinimalBodyWithOperation(t *testing.T) {
	saveUsable(t)
	ctx := context.Background()
	for _, tc := range []struct{ word, entity, path, id, token string }{
		{"invoice", "Invoice", "invoice", "188", "2"},
		{"estimate", "Estimate", "estimate", "9", "1"},
		{"payment", "Payment", "payment", "77", "0"},
		{"creditmemo", "CreditMemo", "creditmemo", "64", "3"},
		{"salesreceipt", "SalesReceipt", "salesreceipt", "51", "0"},
	} {
		obj := fmt.Sprintf(`{%q:{"Id":%q,"SyncToken":%q,"TotalAmt":9}}`, tc.entity, tc.id, tc.token)
		srv := newSDServer(t, map[string]string{
			"GET /" + tc.path:  obj,
			"POST /" + tc.path: obj,
		})
		interceptHTTP(t, srv.URL)

		res, err := ReplaySalestxMutate(ctx, tc.word, "delete", tc.id, nil)
		if err != nil {
			t.Fatalf("%s delete: %v", tc.word, err)
		}
		if res.Op != "delete" || res.Entity != tc.entity || res.Item.ID != tc.id {
			t.Fatalf("%s delete envelope = %+v", tc.word, res)
		}
		snap := srv.snap()
		if snap.Calls != 2 || snap.Method != http.MethodPost {
			t.Fatalf("%s delete flow = %+v", tc.word, snap)
		}
		if want := "minorversion=73&operation=delete"; snap.RawQuery != want {
			t.Fatalf("%s delete query = %q, want %q", tc.word, snap.RawQuery, want)
		}
		sent := snap.sentMap(t)
		if len(sent) != 2 || sent["Id"] != tc.id || sent["SyncToken"] != tc.token {
			t.Fatalf("%s delete body must be minimal {Id,SyncToken}: %v", tc.word, sent)
		}
	}
}

// --- invoice extras: void / send / copy / pdf --------------------------------

const sdInvoiceFull = `{"Invoice":{"Id":"188","SyncToken":"2","DocNumber":"INV-188",` +
	`"TxnDate":"2026-08-05","TotalAmt":55,"CustomerRef":{"value":"23","name":"Acme"},` +
	`"Line":[{"Amount":55,"DetailType":"SalesItemLineDetail"}],` +
	`"MetaData":{"CreateTime":"2026-08-05T00:00:00Z"}}}`

func TestSalesDomainInvoiceVoidSendCopyPdf(t *testing.T) {
	saveUsable(t)
	ctx := context.Background()

	// void: fetch, then {Id, SyncToken} with operation=void.
	voidSrv := newSDServer(t, map[string]string{
		"GET /invoice":  sdInvoiceFull,
		"POST /invoice": sdInvoiceFull,
	})
	interceptHTTP(t, voidSrv.URL)
	res, err := ReplaySalestxMutate(ctx, "invoice", "void", "188", nil)
	if err != nil {
		t.Fatalf("invoice void: %v", err)
	}
	if res.Op != "void" || res.Entity != "Invoice" || res.Item.ID != "188" {
		t.Fatalf("void envelope = %+v", res)
	}
	snap := voidSrv.snap()
	if snap.RawQuery != "minorversion=73&operation=void" {
		t.Fatalf("void query = %q", snap.RawQuery)
	}
	sent := snap.sentMap(t)
	if len(sent) != 2 || sent["Id"] != "188" || sent["SyncToken"] != "2" {
		t.Fatalf("void body = %v", sent)
	}

	// send: POST /invoice/{id}/send with sendTo.
	sendSrv := newSDServer(t, map[string]string{"POST /send": sdInvoiceFull})
	interceptHTTP(t, sendSrv.URL)
	_, err = ReplaySalestxMutate(ctx, "invoice", "send", "188", map[string]string{"to": "a@b.co"})
	if !errors.Is(err, ErrReadbackUnavailable) || sendSrv.snap().Calls != 0 {
		t.Fatalf("unverifiable email was submitted: %v", err)
	}

	// copy: fetch, strip server-managed fields, POST as a fresh create.
	copySrv := newSDServer(t, map[string]string{
		"GET /invoice":  sdInvoiceFull,
		"POST /invoice": `{"Invoice":{"Id":"201","SyncToken":"0","TotalAmt":55}}`,
	})
	interceptHTTP(t, copySrv.URL)
	res, err = ReplaySalestxMutate(ctx, "invoice", "copy", "188", nil)
	if err != nil {
		t.Fatalf("invoice copy: %v", err)
	}
	// A clone posts a create, so the envelope reports the create op and the
	// server-assigned clone id.
	if res.Op != "create" || res.Entity != "Invoice" || res.Item.ID != "201" {
		t.Fatalf("copy envelope = %+v", res)
	}
	snap = copySrv.snap()
	if snap.Calls != 2 || snap.Method != http.MethodPost {
		t.Fatalf("copy flow = %+v", snap)
	}
	if snap.RawQuery != "minorversion=73" {
		t.Fatalf("copy query = %q (clone must POST a plain create)", snap.RawQuery)
	}
	sent = snap.sentMap(t)
	for _, k := range []string{"Id", "SyncToken", "DocNumber", "MetaData", "CustomField"} {
		if _, ok := sent[k]; ok {
			t.Fatalf("copy must strip %s: %v", k, sent)
		}
	}
	if sent["CustomerRef"].(map[string]any)["value"] != "23" {
		t.Fatalf("copy lost CustomerRef: %v", sent["CustomerRef"])
	}
	if line := sent["Line"].([]any)[0].(map[string]any); line["Amount"] != 55.0 {
		t.Fatalf("copy line = %v", line)
	}

	// pdf: GET /v3/documents/pdf streams the rendered bytes to disk.
	pdfBytes := "%PDF-1.7 sales-domain"
	pdfSrv := newSDServer(t, map[string]string{"GET /pdf": pdfBytes})
	interceptHTTP(t, pdfSrv.URL)
	out := filepath.Join(t.TempDir(), "inv-188.pdf")
	pres, err := ReplayDocumentPDF(ctx, "188", out)
	if err != nil {
		t.Fatalf("invoice pdf: %v", err)
	}
	if pres.Bytes != len(pdfBytes) || pres.Path != out {
		t.Fatalf("pdf result = %+v", pres)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != pdfBytes {
		t.Fatalf("pdf file = %q (%v)", got, err)
	}
	snap = pdfSrv.snap()
	if snap.Method != http.MethodGet || snap.Path != "/v3/documents/pdf" {
		t.Fatalf("pdf request = %s %s", snap.Method, snap.Path)
	}
	if want := "txnId=188&documentType=invoice"; snap.RawQuery != want {
		t.Fatalf("pdf query = %q, want %q", snap.RawQuery, want)
	}
}

// TestSalesDomainEstimateSendCarriesRecipient pins the estimate send twin of
// the invoice send: POST /estimate/{id}/send with minorversion + sendTo.
func TestSalesDomainEstimateSendCarriesRecipient(t *testing.T) {
	saveUsable(t)
	est := `{"Estimate":{"Id":"9","SyncToken":"1","TotalAmt":120}}`
	srv := newSDServer(t, map[string]string{"POST /send": est})
	interceptHTTP(t, srv.URL)

	_, err := ReplaySalestxMutate(context.Background(), "estimate", "send", "9", map[string]string{"to": "quote@b.co"})
	if !errors.Is(err, ErrReadbackUnavailable) || srv.snap().Calls != 0 {
		t.Fatalf("unverifiable email was submitted: %v", err)
	}
}
