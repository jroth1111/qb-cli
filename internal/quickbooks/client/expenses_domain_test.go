package client

// expenses_domain_test.go covers the expenses domain wired/read commands
// (bill, expense, cheque, mileage, bank-fee) using the package-standard
// DefaultTransport swap (interceptHTTP): production-built qbo.intuit.com v3
// company URLs are rewritten to a local httptest server, so no production
// file is touched and no real host dialed.
//
// The domain wires onto three v3 entities: Bill (expenses bill), Purchase
// (expense cash, cheque Check, bank-fee Cash) and TimeActivity (mileage).
// Every verb rides POST /api/v3/company/{realm}/{entity}?minorversion=73,
// with operation=delete|void appended for destructive ops and a GET of the
// row first whenever a SyncToken is required (update/delete/void).
//
// Kill denominator: 10. The suite rejects implementations that
//  1. dial the network without credentials (fast-fail broken),
//  2. route expenses entities to the wrong v3 path (or invent Cheque /
//     BankFee / Mileage entities that QBO does not have),
//  3. drop minorversion=73 or the operation=delete/void query parameter,
//  4. write blind — update/delete/void must GET the row for its SyncToken
//     before POSTing,
//  5. send a Bill create without VendorRef, normalized TxnDate, or an
//     AccountBasedExpenseLineDetail,
//  6. book expense / cheque / bank-fee as anything but a Purchase whose
//     payment type is Cash or Check against a bank/CC AccountRef,
//  7. accept a purchase whose category account equals its payment account
//     (self-referencing transfer masquerading as an expense),
//  8. record mileage outside the TimeActivity NameOf=Employee +
//     EmployeeRef shape (or without an employee),
//  9. embed the live realm in a planned URL instead of the {realm} token,
//  10. hide faults in query projection/filtering (wrong SELECT clause,
//      dropped rows, clamped limits ignored).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fixtures ---------------------------------------------------------------

const expBillRow = `{"Id":"160","SyncToken":"0","TxnDate":"2026-08-01","DocNumber":"QB-CLI-001","TotalAmt":12.5,"VendorRef":{"value":"2","name":"Acme Pty Ltd"}}`

const expBillShape = `{"Bill":` + expBillRow + `}`

// expBillExisting is the stored bill fetched before update/delete/void.
const expBillExisting = `{"Bill":{"Id":"46","SyncToken":"2","TxnDate":"2026-08-01","VendorRef":{"value":"2","name":"Acme Pty Ltd"},"CurrencyRef":{"value":"AUD"},"Line":[{"Amount":0.11,"DetailType":"AccountBasedExpenseLineDetail","AccountBasedExpenseLineDetail":{"AccountRef":{"value":"7"}}}]}}`

const expPurchaseRow = `{"Id":"177","SyncToken":"1","TxnDate":"2026-08-02","PaymentType":"Cash","TotalAmt":100,"AccountRef":{"value":"48"},"EntityRef":{"value":"5","name":"Acme Pty Ltd"}}`

const expPurchaseShape = `{"Purchase":` + expPurchaseRow + `}`

const expPurchaseExisting = `{"Purchase":{"Id":"177","SyncToken":"1","TxnDate":"2026-08-02","PaymentType":"Check","AccountRef":{"value":"48"},"EntityRef":{"value":"5","name":"Acme Pty Ltd"},"Line":[{"Amount":0.53,"DetailType":"AccountBasedExpenseLineDetail","AccountBasedExpenseLineDetail":{"AccountRef":{"value":"29"}}}]}}`

const expTimeExisting = `{"TimeActivity":{"Id":"551","SyncToken":"3","TxnDate":"2026-08-03","NameOf":"Employee","EmployeeRef":{"value":"11","name":"Jo"},"Hours":2,"HourlyRate":1}}`

const expBillQueryRows = `{"QueryResponse":{"Bill":[` + expBillRow + `,{"Id":"161","SyncToken":"0","TxnDate":"2026-08-02","DocNumber":"OTHER","TotalAmt":3}]}}`

const expPurchaseQueryRows = `{"QueryResponse":{"Purchase":[` + expPurchaseRow + `]}}`

const expTimeQueryRows = `{"QueryResponse":{"TimeActivity":[{"Id":"551","TxnDate":"2026-08-06","NameOf":"Employee","Hours":2}]}}`

// --- fake v3 company server ---------------------------------------------------

// expServer answers GETs (row fetch / query) and POSTs (mutations) with
// scripted bodies and records every request. Tests are sequential; the
// mutex guards the recording fields only.
type expServer struct {
	t *testing.T
	*httptest.Server

	mu            sync.Mutex
	getCalls      int
	postCalls     int
	lastMethod    string
	lastPath      string
	lastRawQuery  string
	lastBody      []byte
	lastPostPath  string
	lastPostQuery string
	persisted     map[string]any
	deleted       bool

	statusGet  int
	statusPost int
	getBody    string
	postBody   string
}

func newExpServer(t *testing.T, getBody, postBody string) *expServer {
	t.Helper()
	s := &expServer{
		t:          t,
		statusGet:  http.StatusOK,
		statusPost: http.StatusOK,
		getBody:    getBody,
		postBody:   postBody,
		persisted:  map[string]any{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *expServer) handle(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	status, body := s.statusPost, s.postBody
	s.mu.Lock()
	s.lastMethod = r.Method
	s.lastPath = r.URL.Path
	s.lastRawQuery = r.URL.RawQuery
	if r.Method == http.MethodGet {
		s.getCalls++
	} else {
		s.postCalls++
		s.lastBody = b // preserve the submitted wire body across readback GETs.
		s.lastPostPath = r.URL.Path
		s.lastPostQuery = r.URL.RawQuery
		var submitted map[string]any
		_ = json.Unmarshal(b, &submitted)
		var receipt map[string]map[string]any
		_ = json.Unmarshal([]byte(s.postBody), &receipt)
		for entity, row := range receipt {
			if r.URL.Query().Get("operation") == "delete" {
				s.deleted = true
				break
			}
			stored := map[string]any{}
			maps.Copy(stored, row)
			maps.Copy(stored, submitted)
			if r.URL.Query().Get("operation") == "void" {
				stored["status"] = "Voided"
				stored["TotalAmt"] = float64(0)
			}
			s.persisted[entity] = stored
			break
		}
	}
	if r.Method == http.MethodGet {
		status, body = s.statusGet, s.getBody
		if strings.HasSuffix(r.URL.Path, "/query") && s.deleted {
			entity := "Bill"
			if q, _ := url.ParseQuery(r.URL.RawQuery); q != nil {
				if fields := strings.Fields(q.Get("query")); len(fields) >= 4 {
					entity = fields[3]
				}
			}
			body = `{"QueryResponse":{"` + entity + `":[]}}`
		} else if !strings.HasSuffix(r.URL.Path, "/query") && !s.deleted {
			for entity, row := range s.persisted {
				encoded, _ := json.Marshal(map[string]any{entity: row})
				body = string(encoded)
				break
			}
		}
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// snapshot returns the recorded method, path, raw query and request body of
// the last call.
func (s *expServer) snapshot() (method, path, rawQuery string, reqBody []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return http.MethodPost, s.lastPostPath, s.lastPostQuery, append([]byte(nil), s.lastBody...)
}

// counts returns the number of GETs and POSTs seen so far.
func (s *expServer) counts() (gets, posts int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getCalls, s.postCalls
}

// expQueryOf parses the recorded raw query of the last call.
func expQueryOf(t *testing.T, rawQuery string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("query %q not parseable: %v", rawQuery, err)
	}
	return q
}

// expAssertV3Post pins the v3 company POST contract: METHOD POST, path
// /api/v3/company/{realm}/{entity}, minorversion=73, and — for destructive
// ops — the operation query parameter.
func expAssertV3Post(t *testing.T, s *expServer, entity, operation string) {
	t.Helper()
	method, path, rawQuery, _ := s.snapshot()
	if method != http.MethodPost {
		t.Fatalf("%s %s: method = %s, want POST", entity, operation, method)
	}
	wantPath := "/api/v3/company/12345/" + entity // session realm from saveUsable
	if path != wantPath {
		t.Fatalf("%s %s: path = %s, want %s", entity, operation, path, wantPath)
	}
	q := expQueryOf(t, rawQuery)
	if got := q.Get("minorversion"); got != "73" {
		t.Fatalf("%s %s: minorversion = %q, want 73 (%s)", entity, operation, got, rawQuery)
	}
	if got := q.Get("operation"); got != operation {
		t.Fatalf("%s %s: operation = %q, want %q (%s)", entity, operation, got, operation, rawQuery)
	}
}

// expBodyMap decodes the recorded request body.
func expBodyMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, body)
	}
	return m
}

// --- fail-fast on missing credentials -----------------------------------------

// TestExpensesDomainFailsFastOnNoCredentials is the missing-credentials
// negative for every expenses entry point: with an empty QB_HOME each must
// fail with ErrNoCredentials before any network activity.
func TestExpensesDomainFailsFastOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	calls := []struct {
		name string
		do   func() error
	}{
		{"bill-list", func() error { _, err := ReplayQuery(context.Background(), "Bill", "", "", 5); return err }},
		{"bill-get", func() error { _, err := ReplayQuery(context.Background(), "Bill", "46", "", 1); return err }},
		{"bill-create", func() error {
			_, err := ReplayMutate(context.Background(), "Bill", "create", "", map[string]string{"supplier": "2"})
			return err
		}},
		{"bill-delete", func() error { _, err := ReplayMutate(context.Background(), "Bill", "delete", "46", nil); return err }},
		{"expense-list", func() error { _, err := ReplayQuery(context.Background(), "Purchase", "", "", 5); return err }},
		{"expense-create", func() error {
			_, err := ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{"account": "48"})
			return err
		}},
		{"cheque-list", func() error { _, err := ReplayQuery(context.Background(), "Purchase", "177", "", 1); return err }},
		{"mileage-list", func() error { _, err := ReplayQuery(context.Background(), "TimeActivity", "", "", 5); return err }},
		{"mileage-create", func() error {
			_, err := ReplayMutate(context.Background(), "TimeActivity", "create", "", map[string]string{"employee": "11"})
			return err
		}},
		{"bankfee-create", func() error {
			_, err := ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{"account": "48", "amount": "2.5"})
			return err
		}},
	}
	for _, tc := range calls {
		start := time.Now()
		err := tc.do()
		assertFast(t, time.Since(start))
		if !errors.Is(err, ErrNoCredentials) {
			t.Fatalf("%s: got %v, want ErrNoCredentials", tc.name, err)
		}
	}
}

// --- entity routing -----------------------------------------------------------

// TestExpensesDomainRouteThroughThreeV3Entities pins the routing table: the
// expenses domain owns Bill, Purchase, and TimeActivity. Cheque, Expense,
// BankFee, and Mileage are CLI words, not v3 entities — an implementation
// that added them to the path table would bypass the real QBO schema.
func TestExpensesDomainRouteThroughThreeV3Entities(t *testing.T) {
	for _, ent := range []string{"Bill", "Purchase", "TimeActivity"} {
		path, ok := v3Path[ent]
		if !ok || path == "" {
			t.Fatalf("v3Path[%q] missing", ent)
		}
		if path != strings.ToLower(ent) {
			t.Fatalf("v3Path[%q] = %q, want lowercase segment", ent, path)
		}
	}
	for _, word := range []string{"Cheque", "Expense", "BankFee", "Mileage"} {
		if _, ok := v3Path[word]; ok {
			t.Fatalf("v3Path[%q] must not exist; CLI words ride Purchase/TimeActivity", word)
		}
	}

	// Cheque is a constrained Purchase operation, never a separate API entity.
	if PlannedMutateURL("Cheque", "create") != PlannedMutateURL("Purchase", "create") {
		t.Fatal("cheque plan must target the native purchase endpoint")
	}
	// Unknown-entity rejection still precedes credential loading.
	noCredsDir(t)
	if _, err := ReplayMutate(context.Background(), "BankFee", "create", "", nil); err == nil ||
		strings.Contains(err.Error(), "credentials") {
		t.Fatalf("ReplayMutate(BankFee) err = %v, want no-v3-path rejection before creds", err)
	}
}

// TestExpensesDomainPlannedURLsPinV3Contract pins dry-run URL shapes: v3
// company path, minorversion=73, operation on destructive verbs.
func TestExpensesDomainPlannedURLsPinV3Contract(t *testing.T) {
	cases := []struct{ entity, op, want string }{
		{"Bill", "create", "https://qbo.intuit.com/api/v3/company/{realm}/bill?minorversion=73"},
		{"Bill", "delete", "https://qbo.intuit.com/api/v3/company/{realm}/bill?minorversion=73&operation=delete"},
		{"Bill", "void", "https://qbo.intuit.com/api/v3/company/{realm}/bill?minorversion=73&operation=void"},
		{"Bill", "update", "https://qbo.intuit.com/api/v3/company/{realm}/bill?minorversion=73"},
		{"Purchase", "create", "https://qbo.intuit.com/api/v3/company/{realm}/purchase?minorversion=73"},
		{"Purchase", "delete", "https://qbo.intuit.com/api/v3/company/{realm}/purchase?minorversion=73&operation=delete"},
		{"TimeActivity", "create", "https://qbo.intuit.com/api/v3/company/{realm}/timeactivity?minorversion=73"},
		{"TimeActivity", "delete", "https://qbo.intuit.com/api/v3/company/{realm}/timeactivity?minorversion=73&operation=delete"},
	}
	for _, tc := range cases {
		if got := PlannedMutateURL(tc.entity, tc.op); got != tc.want {
			t.Fatalf("PlannedMutateURL(%s,%s) = %q, want %q", tc.entity, tc.op, got, tc.want)
		}
	}
	for _, ent := range []string{"Bill", "Purchase", "TimeActivity"} {
		u := PlannedQueryURL(ent)
		if !strings.HasPrefix(u, "https://qbo.intuit.com/api/v3/company/{realm}/query?minorversion=73&query=") {
			t.Fatalf("PlannedQueryURL(%s) = %q, want v3 company query", ent, u)
		}
		if !strings.Contains(u, url.QueryEscape("select * from "+ent+" maxresults 20")) {
			t.Fatalf("PlannedQueryURL(%s) = %q, want default select", ent, u)
		}
	}
}

// --- bill ---------------------------------------------------------------------

// TestExpensesBillCreatePostsSupplierAndExpenseLine drives `expenses bill
// create`: POST /bill carrying VendorRef, the dd/MM/yyyy date normalized to
// TxnDate, and an AccountBasedExpenseLineDetail against the category
// account. It rejects implementations that send the raw AU-style date, drop
// the supplier, or emit a sales-style line.
func TestExpensesBillCreatePostsSupplierAndExpenseLine(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, "", expBillShape)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Bill", "create", "", map[string]string{
		"supplier": "2", "amount": "12.50", "account": "7", "date": "05/08/2026",
	})
	if err != nil {
		t.Fatalf("bill create: %v", err)
	}
	expAssertV3Post(t, srv, "bill", "")
	if res.Status != http.StatusOK || res.Op != "create" || res.Entity != "Bill" {
		t.Fatalf("envelope = %+v", res)
	}
	if res.Item.ID != "160" || res.Item.DocNumber != "QB-CLI-001" {
		t.Fatalf("projected item = %+v", res.Item)
	}
	body := expBodyMap(t, srv.lastBody)
	if got := body["VendorRef"].(map[string]any)["value"]; got != "2" {
		t.Fatalf("VendorRef = %v", body["VendorRef"])
	}
	if body["TxnDate"] != "2026-08-05" {
		t.Fatalf("TxnDate = %v, want dd/MM/yyyy normalized", body["TxnDate"])
	}
	lines, _ := body["Line"].([]any)
	if len(lines) != 1 {
		t.Fatalf("Line = %v", body["Line"])
	}
	line := lines[0].(map[string]any)
	if line["DetailType"] != "AccountBasedExpenseLineDetail" || line["Amount"] != 12.5 {
		t.Fatalf("line = %v", line)
	}
	det := line["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "7" {
		t.Fatalf("line account = %v", det["AccountRef"])
	}
}

// TestExpensesBillCreateRejectsMissingSupplier: the negative create —
// validation must fail before any dial.
func TestExpensesBillCreateRejectsMissingSupplier(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, "", expBillShape)
	interceptHTTP(t, srv.URL)
	_, err := ReplayMutate(context.Background(), "Bill", "create", "", map[string]string{"amount": "1"})
	if err == nil || !strings.Contains(err.Error(), "--supplier") {
		t.Fatalf("err = %v, want supplier-required validation error", err)
	}
	if _, posts := srv.counts(); posts != 0 {
		t.Fatalf("validation must precede any request; got %d posts", posts)
	}
}

// TestExpensesBillUpdateFetchesThenSparsePatches drives `expenses bill
// update edit`: GET the row for its SyncToken, then POST a sparse update
// that copies VendorRef/Line/CurrencyRef and restates the amount on both
// TotalAmt and the first line. It rejects blind writes (no GET) and patches
// that lose the stored line detail.
func TestExpensesBillUpdateFetchesThenSparsePatches(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, expBillExisting, expBillExisting)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Bill", "update", "46", map[string]string{"amount": "0.52"})
	if err != nil {
		t.Fatalf("bill update: %v", err)
	}
	if res.Op != "update" || res.Entity != "Bill" || res.Item.ID != "46" {
		t.Fatalf("update envelope = %+v, want Bill 46 echoed from the POST response", res)
	}
	gets, posts := srv.counts()
	if gets != 2 || posts != 1 {
		t.Fatalf("update must GET the row, POST once, then independently read back; got %d gets, %d posts", gets, posts)
	}
	expAssertV3Post(t, srv, "bill", "")
	body := expBodyMap(t, srv.lastBody)
	if body["Id"] != "46" || body["SyncToken"] != "2" {
		t.Fatalf("patch must carry fetched Id+SyncToken: %v", body)
	}
	if body["sparse"] != true {
		t.Fatalf("update must be sparse: %v", body)
	}
	if body["TotalAmt"] != 0.52 {
		t.Fatalf("TotalAmt = %v", body["TotalAmt"])
	}
	lines, _ := body["Line"].([]any)
	if len(lines) != 1 || lines[0].(map[string]any)["Amount"] != 0.52 {
		t.Fatalf("Line = %v", body["Line"])
	}
	if _, ok := body["VendorRef"].(map[string]any); !ok {
		t.Fatalf("stored VendorRef must be copied: %v", body)
	}
}

// TestExpensesBillDeleteAndVoidCarryOperationQuery drives `expenses bill
// delete` and the void variant: both fetch the row first, then POST with
// Id+SyncToken and the matching operation query parameter.
func TestExpensesBillDeleteAndVoidCarryOperationQuery(t *testing.T) {
	for _, op := range []string{"delete", "void"} {
		t.Run(op, func(t *testing.T) {
			saveUsable(t)
			srv := newExpServer(t, expBillExisting, expBillExisting)
			interceptHTTP(t, srv.URL)

			res, err := ReplayMutate(context.Background(), "Bill", op, "46", nil)
			if err != nil {
				t.Fatalf("bill %s: %v", op, err)
			}
			if res.Op != op || res.Item.ID != "46" {
				t.Fatalf("envelope = %+v", res)
			}
			gets, posts := srv.counts()
			if gets != 2 || posts != 1 {
				t.Fatalf("%s must GET the row, POST once, then independently read back; got %d gets, %d posts", op, gets, posts)
			}
			expAssertV3Post(t, srv, "bill", op)
			body := expBodyMap(t, srv.lastBody)
			if body["Id"] != "46" || body["SyncToken"] != "2" {
				t.Fatalf("%s body must carry fetched Id+SyncToken: %v", op, body)
			}
		})
	}
}

// TestExpensesBillMutateRejectsMissingID: destructive verbs without an id
// fail with ErrMissingMutateID before any dial.
func TestExpensesBillMutateRejectsMissingID(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, expBillExisting, expBillShape)
	interceptHTTP(t, srv.URL)
	for _, op := range []string{"update", "delete", "void"} {
		_, err := ReplayMutate(context.Background(), "Bill", op, "", nil)
		if !errors.Is(err, ErrMissingMutateID) {
			t.Fatalf("%s without id: got %v, want ErrMissingMutateID", op, err)
		}
	}
	if gets, posts := srv.counts(); gets != 0 || posts != 0 {
		t.Fatalf("validation must precede any request; got %d gets, %d posts", gets, posts)
	}
}

// TestExpensesBillListGetSearchUseSanitizedSelect drives `expenses bill
// list/get/search`: the SELECT names the entity, a get narrows on
// Id = '...', a search narrows on the DocNumber LIKE clause (bills have no
// display name), results are filtered client-side, and the limit is clamped
// to the v3 ceiling.
func TestExpensesBillListGetSearchUseSanitizedSelect(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, expBillQueryRows, "")
	interceptHTTP(t, srv.URL)

	// list
	res, err := ReplayQuery(context.Background(), "Bill", "", "", 3)
	if err != nil {
		t.Fatalf("bill list: %v", err)
	}
	if res.Status != http.StatusOK || res.Entity != "Bill" || len(res.Items) != 2 {
		t.Fatalf("list envelope = %+v", res)
	}
	if res.Items[0].ID != "160" || res.Items[0].DocNumber != "QB-CLI-001" || res.Items[0].Amount != 12.5 || res.Items[0].Date != "2026-08-01" {
		t.Fatalf("item projection = %+v", res.Items[0])
	}
	q := expQueryOf(t, srv.lastRawQuery)
	if got := q.Get("query"); got != "select * from Bill maxresults 3" {
		t.Fatalf("list query = %q", got)
	}

	// get by id
	if _, err := ReplayQuery(context.Background(), "Bill", "46", "", 1); err != nil {
		t.Fatalf("bill get: %v", err)
	}
	q = expQueryOf(t, srv.lastRawQuery)
	if got := q.Get("query"); got != "select * from Bill where Id = '46' maxresults 1" {
		t.Fatalf("get query = %q", got)
	}

	// search: DocNumber LIKE + client-side filter keeps only the match
	res, err = ReplayQuery(context.Background(), "Bill", "", "QB-CLI", 20)
	if err != nil {
		t.Fatalf("bill search: %v", err)
	}
	q = expQueryOf(t, srv.lastRawQuery)
	if got := q.Get("query"); got != "select * from Bill where DocNumber like '%QB-CLI%' maxresults 20" {
		t.Fatalf("search query = %q", got)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "160" {
		t.Fatalf("search items = %+v, want only the QB-CLI row", res.Items)
	}

	// page: oversized limits page via ordered STARTPOSITION windows of 100
	if _, err := ReplayQuery(context.Background(), "Bill", "", "", 500); err != nil {
		t.Fatalf("bill list paged: %v", err)
	}
	q = expQueryOf(t, srv.lastRawQuery)
	if got := q.Get("query"); got != "select * from Bill orderby Id maxresults 100" {
		t.Fatalf("paged query = %q", got)
	}
}

// --- expense (Purchase Cash) ----------------------------------------------------

// TestExpensesExpenseCreateBooksCashPurchase drives `expenses expense
// create`: a Purchase with PaymentType=Cash, AccountRef on the paying
// bank/CC account, EntityRef on the payee, and the category line distinct
// from the payment account.
func TestExpensesExpenseCreateBooksCashPurchase(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, expPurchaseQueryRows, expPurchaseShape)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{
		"supplier":         "5",
		"amount":           "100",
		"account":          "48",
		"category-account": "29",
		"date":             "06/08/2026",
	})
	if err != nil {
		t.Fatalf("expense create: %v", err)
	}
	expAssertV3Post(t, srv, "purchase", "")
	if res.Entity != "Purchase" || res.Op != "create" || res.Item.ID != "177" {
		t.Fatalf("envelope = %+v", res)
	}
	if res.Item.Type != "Cash" {
		t.Fatalf("projected PaymentType = %q, want Cash", res.Item.Type)
	}
	body := expBodyMap(t, srv.lastBody)
	if body["PaymentType"] != "Cash" {
		t.Fatalf("PaymentType = %v, want Cash", body["PaymentType"])
	}
	if body["AccountRef"].(map[string]any)["value"] != "48" {
		t.Fatalf("payment AccountRef = %v", body["AccountRef"])
	}
	if body["EntityRef"].(map[string]any)["value"] != "5" {
		t.Fatalf("EntityRef = %v", body["EntityRef"])
	}
	if body["TxnDate"] != "2026-08-06" {
		t.Fatalf("TxnDate = %v", body["TxnDate"])
	}
	lines, _ := body["Line"].([]any)
	line := lines[0].(map[string]any)
	det := line["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "29" || line["Amount"] != 100.0 {
		t.Fatalf("category line = %v", line)
	}

	// `expenses expense list` reads the same Purchase entity back.
	list, err := ReplayQuery(context.Background(), "Purchase", "", "", 5)
	if err != nil {
		t.Fatalf("expense list: %v", err)
	}
	q := expQueryOf(t, srv.lastRawQuery)
	if got := q.Get("query"); got != "select * from Purchase maxresults 5" {
		t.Fatalf("expense list query = %q", got)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "177" || list.Items[0].Type != "Cash" {
		t.Fatalf("list items = %+v", list.Items)
	}
}

// TestExpensesPurchaseCreateRejectsBadAccounts: the shared expense /
// cheque / bank-fee builder refuses a purchase with no payment account and
// one whose category equals the payment account — both would book money
// moving from an account to itself — before any dial.
func TestExpensesPurchaseCreateRejectsBadAccounts(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, "", expPurchaseShape)
	interceptHTTP(t, srv.URL)

	_, err := ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{
		"category-account": "29", "amount": "1",
	})
	if err == nil || !strings.Contains(err.Error(), "requires --account") {
		t.Fatalf("no payment account: err = %v", err)
	}
	_, err = ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{
		"account": "48", "category-account": "48", "amount": "1",
	})
	if err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("identical accounts: err = %v", err)
	}
	if gets, posts := srv.counts(); gets != 0 || posts != 0 {
		t.Fatalf("validation must precede any request; got %d gets, %d posts", gets, posts)
	}
}

// --- cheque (Purchase Check) ----------------------------------------------------

// TestExpensesChequeCreateSetsCheckPaymentType drives `expenses cheque
// create`: the cheque is a Purchase whose payment mode flips PaymentType to
// Check (and credit-card mode to CreditCard) while keeping the bank
// AccountRef and category line.
func TestExpensesChequeCreateSetsCheckPaymentType(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, "", expPurchaseExisting)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{
		"supplier":         "5",
		"amount":           "0.53",
		"account":          "48",
		"category-account": "29",
		"mode":             "cheque",
	})
	if err != nil {
		t.Fatalf("cheque create: %v", err)
	}
	expAssertV3Post(t, srv, "purchase", "")
	body := expBodyMap(t, srv.lastBody)
	if body["PaymentType"] != "Check" {
		t.Fatalf("PaymentType = %v, want Check", body["PaymentType"])
	}
	if res.Item.Type != "Check" {
		t.Fatalf("projected type = %q, want Check", res.Item.Type)
	}

	// credit-card mode on the same builder
	if _, err := ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{
		"account": "48", "category-account": "29", "amount": "1", "mode": "credit-card",
	}); err != nil {
		t.Fatalf("credit-card expense: %v", err)
	}
	body = expBodyMap(t, srv.lastBody)
	if body["PaymentType"] != "CreditCard" {
		t.Fatalf("PaymentType = %v, want CreditCard", body["PaymentType"])
	}
}

// TestExpensesChequeUpdateKeepsPaymentTypeThenDelete drives `expenses
// cheque update edit` and `expenses cheque delete`: the sparse update
// preserves the stored PaymentType/AccountRef/EntityRef while restating the
// amount, and the delete rides operation=delete with the fetched SyncToken.
func TestExpensesChequeUpdateKeepsPaymentTypeThenDelete(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, expPurchaseExisting, expPurchaseExisting)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Purchase", "update", "177", map[string]string{"amount": "0.54"})
	if err != nil {
		t.Fatalf("cheque update: %v", err)
	}
	if res.Item.ID != "177" || res.Item.Type != "Check" {
		t.Fatalf("update envelope = %+v", res)
	}
	body := expBodyMap(t, srv.lastBody)
	if body["PaymentType"] != "Check" || body["Id"] != "177" || body["SyncToken"] != "1" || body["sparse"] != true {
		t.Fatalf("cheque patch = %v", body)
	}
	if body["TotalAmt"] != 0.54 {
		t.Fatalf("TotalAmt = %v", body["TotalAmt"])
	}

	del, err := ReplayMutate(context.Background(), "Purchase", "delete", "177", nil)
	if err != nil {
		t.Fatalf("cheque delete: %v", err)
	}
	if del.Op != "delete" {
		t.Fatalf("delete envelope = %+v", del)
	}
	expAssertV3Post(t, srv, "purchase", "delete")
	body = expBodyMap(t, srv.lastBody)
	if body["Id"] != "177" || body["SyncToken"] != "1" {
		t.Fatalf("delete body = %v", body)
	}
}

// --- mileage (TimeActivity) -----------------------------------------------------

// TestExpensesMileageCreateRecordsEmployeeTimeActivity drives `expenses
// mileage create`: a TimeActivity named Employee with the EmployeeRef,
// normalized trip date, and integer hours. Missing --employee must fail
// before any dial.
func TestExpensesMileageCreateRecordsEmployeeTimeActivity(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, "", expTimeExisting)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "TimeActivity", "create", "", map[string]string{
		"employee": "11", "hours": "2", "date": "06/08/2026",
	})
	if err != nil {
		t.Fatalf("mileage create: %v", err)
	}
	expAssertV3Post(t, srv, "timeactivity", "")
	if res.Entity != "TimeActivity" || res.Item.ID != "551" {
		t.Fatalf("envelope = %+v", res)
	}
	body := expBodyMap(t, srv.lastBody)
	if body["NameOf"] != "Employee" {
		t.Fatalf("NameOf = %v, want Employee", body["NameOf"])
	}
	if body["EmployeeRef"].(map[string]any)["value"] != "11" {
		t.Fatalf("EmployeeRef = %v", body["EmployeeRef"])
	}
	if body["TxnDate"] != "2026-08-06" || body["Hours"] != float64(2) {
		t.Fatalf("trip body = %v", body)
	}

	_, err = ReplayMutate(context.Background(), "TimeActivity", "create", "", map[string]string{"hours": "2"})
	if err == nil || !strings.Contains(err.Error(), "--employee") {
		t.Fatalf("missing employee: err = %v", err)
	}
	if gets, posts := srv.counts(); posts != 1 {
		t.Fatalf("negative must not dial; counts = %d/%d", gets, posts)
	}
}

// TestExpensesMileageUpdateHoursThenDelete drives `expenses mileage
// update` and `expenses mileage delete`: the sparse update copies the
// stored NameOf/EmployeeRef and bumps Hours; the delete rides
// operation=delete with the fetched SyncToken.
func TestExpensesMileageUpdateHoursThenDelete(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, expTimeExisting, expTimeExisting)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "TimeActivity", "update", "551", map[string]string{"hours": "3"})
	if err != nil {
		t.Fatalf("mileage update: %v", err)
	}
	if res.Item.ID != "551" {
		t.Fatalf("update envelope = %+v", res)
	}
	body := expBodyMap(t, srv.lastBody)
	if body["Hours"] != float64(3) || body["NameOf"] != "Employee" {
		t.Fatalf("trip patch = %v", body)
	}
	if body["EmployeeRef"].(map[string]any)["value"] != "11" {
		t.Fatalf("stored EmployeeRef must be copied: %v", body)
	}

	del, err := ReplayMutate(context.Background(), "TimeActivity", "delete", "551", nil)
	if err != nil {
		t.Fatalf("mileage delete: %v", err)
	}
	if del.Op != "delete" {
		t.Fatalf("delete envelope = %+v", del)
	}
	expAssertV3Post(t, srv, "timeactivity", "delete")
}

// TestExpensesMileageListAndReadProjectTrips drives `expenses mileage
// list/get`: SELECT from TimeActivity, with the get narrowed on Id.
func TestExpensesMileageListAndReadProjectTrips(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, expTimeQueryRows, "")
	interceptHTTP(t, srv.URL)

	res, err := ReplayQuery(context.Background(), "TimeActivity", "", "", 20)
	if err != nil {
		t.Fatalf("mileage list: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "551" || res.Items[0].Date != "2026-08-06" {
		t.Fatalf("items = %+v", res.Items)
	}
	q := expQueryOf(t, srv.lastRawQuery)
	if got := q.Get("query"); got != "select * from TimeActivity maxresults 20" {
		t.Fatalf("list query = %q", got)
	}

	if _, err := ReplayQuery(context.Background(), "TimeActivity", "551", "", 1); err != nil {
		t.Fatalf("mileage get: %v", err)
	}
	q = expQueryOf(t, srv.lastRawQuery)
	if got := q.Get("query"); got != "select * from TimeActivity where Id = '551' maxresults 1" {
		t.Fatalf("get query = %q", got)
	}
}

// --- bank fee (Purchase Cash against the bank account) --------------------------

// TestExpensesBankFeeCreateBooksDefaultCategoryLine drives `expenses
// bank-fee create`: a Cash Purchase debiting the charged bank account with
// the default expense category line and no payee EntityRef (the fee is not
// tied to a supplier).
func TestExpensesBankFeeCreateBooksDefaultCategoryLine(t *testing.T) {
	saveUsable(t)
	srv := newExpServer(t, "", expPurchaseShape)
	interceptHTTP(t, srv.URL)

	res, err := ReplayMutate(context.Background(), "Purchase", "create", "", map[string]string{
		"account": "48", "amount": "2.50", "date": "07/08/2026", "payee": "NAB FEE",
	})
	if err != nil {
		t.Fatalf("bank-fee create: %v", err)
	}
	expAssertV3Post(t, srv, "purchase", "")
	body := expBodyMap(t, srv.lastBody)
	if body["PaymentType"] != "Cash" {
		t.Fatalf("PaymentType = %v, want Cash", body["PaymentType"])
	}
	if body["AccountRef"].(map[string]any)["value"] != "48" {
		t.Fatalf("bank AccountRef = %v", body["AccountRef"])
	}
	if _, ok := body["EntityRef"]; ok {
		t.Fatalf("bank fee must not carry an EntityRef: %v", body)
	}
	lines, _ := body["Line"].([]any)
	det := lines[0].(map[string]any)["AccountBasedExpenseLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "7" {
		t.Fatalf("default category line = %v", lines)
	}
	_ = res
}

// --- planned URLs stay secret-free ----------------------------------------------

// TestExpensesDomainPlannedURLsAreSecretFree pins the dry-run contract:
// every planned expenses URL carries the {realm} token — never a resolved
// realm id, so plans can be printed and diffed without leaking the company.
func TestExpensesDomainPlannedURLsAreSecretFree(t *testing.T) {
	for _, ent := range []string{"Bill", "Purchase", "TimeActivity"} {
		for _, op := range []string{"create", "update", "delete", "void"} {
			u := PlannedMutateURL(ent, op)
			if !strings.Contains(u, "/api/v3/company/{realm}/") {
				t.Fatalf("planned %s %s url is not v3 company-scoped: %s", ent, op, u)
			}
			for seg := range strings.SplitSeq(u, "/") {
				if len(seg) == 16 && seg != "{realm}" && isAllDigits(seg) {
					t.Fatalf("planned %s %s url embeds a realm-like id: %s", ent, op, u)
				}
			}
		}
		if u := PlannedQueryURL(ent); !strings.Contains(u, "company/{realm}/query") {
			t.Fatalf("planned %s query url is not v3: %s", ent, u)
		}
	}
}

// isAllDigits reports whether s is non-empty and entirely digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
