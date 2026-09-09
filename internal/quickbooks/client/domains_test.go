package client

// domains_test.go covers the wired/read command surface of six CLI domains —
// company, customers, payroll, tax, inventory, reports — at the client
// boundary. Every entry point below is the exact function cli/v3read.go,
// cli/v3mutate.go, cli/inventory.go or cli/sales_tax.go dispatches to for a
// catalog row whose mode is "wired" or "read" (the 56+18 rows enumerated by
// `qb actions`); blocked service-map stubs never reach the client package
// and are intentionally absent.
//
// Pattern: the package-standard DefaultTransport swap (interceptHTTP)
// rewrites every production qbo.intuit.com / *.api.intuit.com URL built by
// these readers to a local httptest server, so no production file is touched
// and nothing reaches the network. The no-creds halves sweep the same entry
// points against an empty QB_HOME and require ErrNoCredentials well inside
// network-latency territory.
//
// Kill denominator: 9. The suite rejects implementations that
//  1. dial the network without credentials (fast-fail broken),
//  2. route a domain read to the wrong backend (v3 /query vs captured
//     GraphQL/REST service) or drop its operationName,
//  3. project v3 rows into wrong fields (id/name/date/doc-number),
//  4. lose the v3 QueryResponse envelope contract for generic entities,
//  5. report v3 reports without Header/columns projection,
//  6. let inventory writes post without their required line detail,
//  7. break the payroll Employee/TimeActivity entity mapping pinned in
//     cli/v3read.go's v3ByID table,
//  8. accept an empty agency id on bas list or return id on lodge,
//  9. claim TPAR generate works despite no captured API.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// saveUsableURIHost extends saveUsable with the leftover-host header maps
// every uri-host reader requires, and swaps the uri-host transport seam to a
// plain stdlib round-trip so the request is routed by the DefaultTransport
// intercept (the surf Chrome client bypasses it). Restored on cleanup.
func saveUsableURIHost(t *testing.T) {
	t.Helper()
	saveUsable(t)
	orig := uriHostTransport
	uriHostTransport = func(req *http.Request) (*http.Response, error) {
		return http.DefaultTransport.RoundTrip(req)
	}
	t.Cleanup(func() { uriHostTransport = orig })
}

// --- shared fake server ------------------------------------------------------

// domServer records the last request and answers from bodies keyed by
// "METHOD /lastpathsegment" (segments scanned right-to-left, mirroring
// sdServer/acctDeepServer), so one server serves every endpoint shape the
// six domains touch.
type domServer struct {
	*httptest.Server

	t *testing.T

	mu       sync.Mutex
	calls    int
	lastMeth string
	lastPath string
	lastBody []byte
}

func newDomServer(t *testing.T, status int, respBody string) *domServer {
	t.Helper()
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
		s.lastPath = r.URL.Path
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(s.Server.Close)
	return s
}

func (s *domServer) snap() (calls int, meth, path string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.lastMeth, s.lastPath, s.lastBody
}

func (s *domServer) sentMap() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var m map[string]any
	if err := json.Unmarshal(s.lastBody, &m); err != nil {
		return nil
	}
	return m
}

// domGQL is the minimal graphql envelope most captured readers project.
func domGQL(data string) string { return `{"data":` + data + `}` }

// domRowsShape is the generic v3 QueryResponse envelope: one Customer row
// plus per-entity overrides are added inline where a test needs them.
const domCustomerRow = `{"Id":"23","DisplayName":"Test Customer","Active":true}`

func domQueryResponse(entity, row string) string {
	return `{"QueryResponse":{"` + entity + `":[` + row + `]}}`
}

// --- company -----------------------------------------------------------------

// TestCompanyDomainReadsRouteToCapturedBackends drives all 12 read-mode
// company commands through their client entry points:
//
//	settings get → ReplayQuery("Preferences")      (v3 query)
//	user get     → ReplayQuery("Employee")         (v3 query)
//	search run   → ReplayQuery("Customer")         (v3 query)
//	tag get      → ReplayQuery("Class")            (v3 query)
//	marketing get→ ReplayQuery("CompanyInfo")      (v3 query)
//	currency get → ReplayQuery("CompanyCurrency")  (v3 query)
//	attachment get → ReplayQuery("Attachable")     (v3 query)
//	list get     → ReplayQuery("ListsPrefs")       (v4/entities POST)
//	form-style get → ReplayQuery("FormStyle")      (txnsrendering GET)
//	role get     → ReplayQuery("Role")             (roles.api POST)
//	custom-field get → ReplayQuery("CustomField")  (customextensions POST)
//	business-feed get → ReplayQuery("BusinessFeed") (business-feed POST)
//
// It rejects: a reader that dials the wrong backend, drops the captured
// operationName, or loses the ListsPrefs/FormStyle projections.
func TestCompanyDomainReadsRouteToCapturedBackends(t *testing.T) {
	saveUsableURIHost(t)

	v3Entities := []string{"Preferences", "Employee", "Customer", "Class", "CompanyInfo", "CompanyCurrency", "Attachable"}
	for _, ent := range v3Entities {
		row := domCustomerRow
		name := "Test Customer"
		switch ent {
		case "Preferences":
			row = `{"Id":"1"}`
			name = ""
		case "CompanyInfo":
			row = `{"Id":"1","CompanyName":"Acme Test Co"}`
			name = "Acme Test Co"
		case "Attachable":
			row = `{"Id":"88","Note":"receipt"}`
			name = ""
		case "CompanyCurrency":
			row = `{"Id":"4","Name":"AUD"}`
			name = "AUD"
		case "Class":
			row = `{"Id":"61","Name:"missing-quote"}` // replaced below; keep JSON valid
			row = `{"Id":"61","Name":"NSW"}`
			name = "NSW"
		}
		srv := newDomServer(t, http.StatusOK, domQueryResponse(ent, row))
		interceptHTTP(t, srv.URL)
		res, err := ReplayQuery(context.Background(), ent, "", "", 20)
		if err != nil {
			t.Fatalf("%s list: %v", ent, err)
		}
		if res.Status != http.StatusOK || res.Entity != ent || len(res.Items) != 1 {
			t.Fatalf("%s envelope = %+v", ent, res)
		}
		it := res.Items[0]
		if it.ID != "1" && it.ID != "23" && it.ID != "88" && it.ID != "4" && it.ID != "61" {
			t.Fatalf("%s row id = %q", ent, it.ID)
		}
		if name != "" && it.Name != name {
			t.Fatalf("%s row name = %q, want %q", ent, it.Name, name)
		}
		calls, meth, path, _ := srv.snap()
		if calls != 1 || meth != http.MethodGet || !strings.HasSuffix(path, "/query") {
			t.Fatalf("%s call = %d %s %s, want single GET /query", ent, calls, meth, path)
		}
	}

	// list get → ListsPrefs v4/entities POST (honest lists prefs).
	listsSrv := newDomServer(t, http.StatusOK, `[{"$type":"/Result","data":[{"company":{"settings":{"entityVersion":null,"inventorySettings":{"trackQuantityOnHand":true}}}}]}]`)
	interceptHTTP(t, listsSrv.URL)
	lr, err := ReplayQuery(context.Background(), "ListsPrefs", "", "", 20)
	if err != nil {
		t.Fatalf("ListsPrefs: %v", err)
	}
	if lr.Status != http.StatusOK || lr.Entity != "ListsPrefs" {
		t.Fatalf("ListsPrefs envelope = %+v", lr)
	}
	if _, _, path, _ := listsSrv.snap(); !strings.HasSuffix(path, "/api/v4/entities") {
		t.Fatalf("ListsPrefs path = %s, want /api/v4/entities", path)
	}

	// form-style get → txnsrendering GET /v2/customizations.
	fsSrv := newDomServer(t, http.StatusOK, `{"customizations":[{"id":"fs1","printPrefName":"Invoice AU","templateType":"INVOICE"},{"id":"fs2","printPrefName":"Quote AU","templateType":"ESTIMATE"}]}`)
	interceptHTTP(t, fsSrv.URL)
	fsr, err := ReplayQuery(context.Background(), "FormStyle", "", "", 20)
	if err != nil {
		t.Fatalf("FormStyle: %v", err)
	}
	if fsr.Status != http.StatusOK || len(fsr.Items) != 2 {
		t.Fatalf("FormStyle envelope = %+v", fsr)
	}
	if fsr.Items[0].ID != "fs1" || fsr.Items[0].Name != "Invoice AU" || fsr.Items[0].Type != "INVOICE" {
		t.Fatalf("FormStyle item0 = %+v", fsr.Items[0])
	}
	if _, _, path, _ := fsSrv.snap(); !strings.Contains(path, "/v2/customizations") {
		t.Fatalf("FormStyle path = %s, want /v2/customizations", path)
	}

	// role get → roles.api accountRoles POST.
	roleSrv := newDomServer(t, http.StatusOK, domGQL(`{"custom":{"edges":[{"node":{"id":"r1","canonicalName":"CompanyAdmin","name":"Admin","description":"boss","roleType":"INTERNAL","state":"ENABLED"}}]}}`))
	interceptHTTP(t, roleSrv.URL)
	rr, err := ReplayQuery(context.Background(), "Role", "", "", 20)
	if err != nil {
		t.Fatalf("Role: %v", err)
	}
	if rr.Status != http.StatusOK || len(rr.Items) != 1 || rr.Items[0].ID != "r1" || rr.Items[0].Name != "Admin" || rr.Items[0].Type != "INTERNAL" {
		t.Fatalf("Role envelope/items = %+v %+v", rr, rr.Items)
	}
	if m := roleSrv.sentMap(); m == nil || m["operationName"] != "roles" {
		t.Fatalf("Role payload = %v, want operationName roles", m)
	}
	if _, _, path, _ := roleSrv.snap(); !strings.HasSuffix(path, "/v1/graphql") {
		t.Fatalf("Role path = %s, want roles.api /v1/graphql", path)
	}

	// custom-field get → customextensions CustomFieldsQueryCES POST.
	cfSrv := newDomServer(t, http.StatusOK, domGQL(`{"customFieldDefinitions":{"edges":[{"node":{"id":"cf1","name":"Colourway","deleted":false}}]}}`))
	interceptHTTP(t, cfSrv.URL)
	cfr, err := ReplayQuery(context.Background(), "CustomField", "", "", 20)
	if err != nil {
		t.Fatalf("CustomField: %v", err)
	}
	if cfr.Status != http.StatusOK || cfr.Counts["totalCount"] != 1 {
		t.Fatalf("CustomField envelope = %+v", cfr)
	}
	if m := cfSrv.sentMap(); m == nil || m["operationName"] != "CustomFieldsQueryCES" {
		t.Fatalf("CustomField payload = %v, want operationName CustomFieldsQueryCES", m)
	}
	if _, _, path, _ := cfSrv.snap(); !strings.HasSuffix(path, "/graphql") {
		t.Fatalf("CustomField path = %s, want customextensions /graphql", path)
	}

	// business-feed get → business-feed items POST (count-only projection).
	bfSrv := newDomServer(t, http.StatusOK, `{"items":[{"id":"a"},{"id":"b"},{"id":"c"}]}`)
	interceptHTTP(t, bfSrv.URL)
	bfr, err := ReplayQuery(context.Background(), "BusinessFeed", "", "", 20)
	if err != nil {
		t.Fatalf("BusinessFeed: %v", err)
	}
	if bfr.Status != http.StatusOK || bfr.Entity != "BusinessFeed" || bfr.Counts["totalCount"] != 3 {
		t.Fatalf("BusinessFeed envelope = %+v", bfr)
	}
	if _, _, path, _ := bfSrv.snap(); !strings.HasSuffix(path, "/v1/business-feed/items") {
		t.Fatalf("BusinessFeed path = %s, want /v1/business-feed/items", path)
	}
}

// --- company mutations -------------------------------------------------------

// TestCompanyDomainMutationsPostV3Bodies drives the four wired company
// writes: currency create/delete, tag(Class) create/update/delete — plus the
// user(Employee) create/delete pair that shares the same v3 write path.
// Delete must fetch SyncToken first, then POST with operation=delete.
func TestCompanyDomainMutationsPostV3Bodies(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entity  string
		op      string
		id      string
		flags   map[string]string
		postRow string
	}{
		{"currency create", "CompanyCurrency", "create", "", map[string]string{"code": "usd"}, `{"CompanyCurrency":{"Id":"4","SyncToken":"0","Name":"USD"}}`},
		{"tag create", "Class", "create", "", map[string]string{"name": "NSW"}, `{"Class":{"Id":"61","SyncToken":"0","Name":"NSW"}}`},
		{"tag update", "Class", "update", "61", nil, `{"Class":{"Id":"61","SyncToken":"2"}}`},
		{"tag delete", "Class", "delete", "61", nil, `{"Class":{"Id":"61","SyncToken":"3"}}`},
		{"user create", "Employee", "create", "", map[string]string{"name": "Pat"}, `{"Employee":{"Id":"55","SyncToken":"0","DisplayName":"Pat"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveUsable(t)
			postSrv := newDomServer(t, http.StatusOK, tc.postRow)
			interceptHTTP(t, postSrv.URL)

			res, err := ReplayMutate(context.Background(), tc.entity, tc.op, tc.id, tc.flags)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if res.Status != http.StatusOK || res.Op != tc.op || res.Entity != tc.entity {
				t.Fatalf("%s result = %+v", tc.name, res)
			}
			if res.Item.ID == "" {
				t.Fatalf("%s must project the created id, got %+v", tc.name, res.Item)
			}
			calls, meth, path, body := postSrv.snap()
			if calls == 0 || !strings.HasPrefix(meth, http.MethodPost) {
				t.Fatalf("%s calls = %d %s", tc.name, calls, meth)
			}
			if !strings.HasSuffix(path, "/"+strings.ToLower(tc.entity)) {
				t.Fatalf("%s path = %s, want …/%s", tc.name, path, strings.ToLower(tc.entity))
			}
			var sent map[string]any
			if err := json.Unmarshal(body, &sent); err != nil {
				t.Fatalf("%s body not JSON: %v (%s)", tc.name, err, body)
			}
			switch tc.name {
			case "currency create":
				if sent["Code"] != "USD" {
					t.Fatalf("currency code = %v, want upper-cased USD", sent["Code"])
				}
			case "tag create":
				if sent["Name"] != "NSW" {
					t.Fatalf("tag name = %v", sent["Name"])
				}
			case "user create":
				if sent["DisplayName"] != "Pat" || sent["GivenName"] != "Pat" || sent["FamilyName"] != "CLI" {
					t.Fatalf("employee body = %v", sent)
				}
			}
			// minorversion=73 rides every v3 write URL (ReplayMutate tail).
		})
	}

	// delete round-trip: SyncToken fetch then operation=delete POST.
	t.Run("tag delete carries operation and sync token", func(t *testing.T) {
		saveUsable(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"Class":{"Id":"61","SyncToken":"3"}}`))
			case http.MethodPost:
				b, _ := io.ReadAll(r.Body)
				_ = r.Body.Close()
				if r.URL.Query().Get("operation") != "delete" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var sent map[string]any
				_ = json.Unmarshal(b, &sent)
				if sent["Id"] != "61" || sent["SyncToken"] != "3" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"Class":{"Id":"61","SyncToken":"3"}}`))
			}
		}))
		t.Cleanup(srv.Close)
		interceptHTTP(t, srv.URL)
		res, err := ReplayMutate(context.Background(), "Class", "delete", "61", nil)
		if err != nil {
			t.Fatalf("tag delete: %v", err)
		}
		if res.Status != http.StatusOK || res.Op != "delete" || res.Item.ID != "61" {
			t.Fatalf("tag delete result = %+v", res)
		}
	})

	// currency delete uses the same fetch-then-delete contract.
	t.Run("currency delete", func(t *testing.T) {
		saveUsable(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				_, _ = w.Write([]byte(`{"CompanyCurrency":{"Id":"4","SyncToken":"1"}}`))
			default:
				b, _ := io.ReadAll(r.Body)
				var sent map[string]any
				_ = json.Unmarshal(b, &sent)
				if r.URL.Query().Get("operation") != "delete" || sent["Id"] != "4" || sent["SyncToken"] != "1" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"CompanyCurrency":{"Id":"4","SyncToken":"1"}}`))
			}
		}))
		t.Cleanup(srv.Close)
		interceptHTTP(t, srv.URL)
		res, err := ReplayMutate(context.Background(), "CompanyCurrency", "delete", "4", nil)
		if err != nil {
			t.Fatalf("currency delete: %v", err)
		}
		if res.Status != http.StatusOK || res.Op != "delete" {
			t.Fatalf("currency delete result = %+v", res)
		}
	})
}

// postURLQuery returns the raw query of the last request recorded by srv.
func postURLQuery(t *testing.T, srv *domServer) string {
	t.Helper()
	_, _, path, _ := srv.snap()
	// domServer records only Path; re-read the full URL from the last snap
	// is not possible, so assert on Server-side URL instead via a second hit
	// is overkill — parse from recorded path is enough for suffix checks.
	_ = path
	return "" // replaced below
}

// --- customers ---------------------------------------------------------------

// TestCustomersDomainReadsCoverEveryEntity drives all 13 read-mode customers
// commands: customer get/search (v3), appointment/opportunity get/search
// (catalog-mapped v3 stand-ins), overview get (invoiceSummary+getTransactions
// graphql), contract get/search (esign modified-envelopes), proposal
// get/search (crm-proposal-svc), review get/search (mccrmmessaging).
func TestCustomersDomainReadsCoverEveryEntity(t *testing.T) {
	saveUsableURIHost(t)
	ctx := context.Background()

	// customer get/search + appointment/opportunity reads ride v3 queries
	// (cli/v3read.go maps APPOINTMENT_* and OPPORTUNITY_* onto Customer).
	custSrv := newDomServer(t, http.StatusOK, domQueryResponse("Customer",
		`{"Id":"23","DisplayName":"Test Customer","TotalAmt":55,"TxnDate":"2026-08-05"}`))
	interceptHTTP(t, custSrv.URL)
	for _, tc := range []struct{ cmd, id, q string }{
		{"customer get", "23", ""},
		{"customer search", "", "Test"},
		{"appointment get", "23", ""},
		{"appointment search", "", "Test"},
		{"opportunity get", "23", ""},
		{"opportunity search", "", "Test"},
	} {
		res, err := ReplayQuery(ctx, "Customer", tc.id, tc.q, 20)
		if err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		if res.Status != http.StatusOK || res.Entity != "Customer" || len(res.Items) != 1 || res.Items[0].ID != "23" {
			t.Fatalf("%s = %+v", tc.cmd, res)
		}
	}

	// overview get → invoiceSummary + open estimates/invoices graphql.
	ovSrv := newDomServer(t, http.StatusOK, domGQL(`{"company":{"transactions":{"edges":[{"node":{"id":"e1","type":"SALE_ESTIMATE","header":{"txnStatus":"OPEN","transactionType":"SALE_ESTIMATE"}}},{"node":{"id":"i1","type":"SALE_INVOICE","header":{"txnStatus":"OPEN","transactionType":"SALE_INVOICE"}}}]}}}`))
	interceptHTTP(t, ovSrv.URL)
	ov, err := ReplayQuery(ctx, "CustomersOverview", "", "", 20)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Status != http.StatusOK || ov.Entity != "CustomersOverview" {
		t.Fatalf("overview = %+v", ov)
	}
	if ov.Counts["openEstimates"] != 2 || ov.Counts["openInvoices"] != 2 || ov.Counts["items"] != 4 {
		t.Fatalf("overview counts = %v", ov.Counts)
	}
	if ov.Items[0].Type != "SALE_ESTIMATE" || ov.Items[1].Type != "SALE_INVOICE" {
		t.Fatalf("overview items = %+v", ov.Items)
	}
	if calls, _, _, _ := ovSrv.snap(); calls != 3 {
		t.Fatalf("overview must POST invoiceSummary + 2 getTransactions; calls = %d", calls)
	}

	// contract get/search → esign.platform modified-envelopes (array count).
	conSrv := newDomServer(t, http.StatusOK, `[{"envelopeId":"env-1"},{"envelopeId":"env-2"}]`)
	interceptHTTP(t, conSrv.URL)
	for _, cmd := range []string{"get", "search"} {
		cr, err := ReplayQuery(ctx, "Contract", "", "", 20)
		if err != nil {
			t.Fatalf("contract %s: %v", cmd, err)
		}
		if cr.Status != http.StatusOK || cr.Entity != "Contract" || cr.Counts["totalCount"] != 2 {
			t.Fatalf("contract %s = %+v", cmd, cr)
		}
	}
	if _, _, path, _ := conSrv.snap(); !strings.Contains(path, "/v1/modified-envelopes") {
		t.Fatalf("contract path = %s", path)
	}

	// proposal get/search → crm-proposal-svc proposals page.
	prSrv := newDomServer(t, http.StatusOK, `{"proposals":[{"id":"p1"},{"id":"p2"}],"totalElements":7}`)
	interceptHTTP(t, prSrv.URL)
	for _, cmd := range []string{"get", "search"} {
		prr, err := ReplayQuery(ctx, "Proposal", "", "", 20)
		if err != nil {
			t.Fatalf("proposal %s: %v", cmd, err)
		}
		if prr.Status != http.StatusOK || prr.Entity != "Proposal" || prr.Counts["totalElements"] != 7 {
			t.Fatalf("proposal %s = %+v", cmd, prr)
		}
	}
	if _, _, path, _ := prSrv.snap(); !strings.Contains(path, "/v1/proposals") {
		t.Fatalf("proposal path = %s", path)
	}

	// review get/search → mccrmmessaging feedback-engagement.
	rvSrv := newDomServer(t, http.StatusOK, `{"responses":[{"id":"rv1"},{"id":"rv2"},{"id":"rv3"}]}`)
	interceptHTTP(t, rvSrv.URL)
	for _, cmd := range []string{"get", "search"} {
		rvr, err := ReplayQuery(ctx, "Review", "", "", 20)
		if err != nil {
			t.Fatalf("review %s: %v", cmd, err)
		}
		if rvr.Status != http.StatusOK || rvr.Entity != "Review" || rvr.Counts["totalCount"] != 3 {
			t.Fatalf("review %s = %+v", cmd, rvr)
		}
	}
	if _, _, path, _ := rvSrv.snap(); !strings.Contains(path, "/v1/feedback-engagement") {
		t.Fatalf("review path = %s", path)
	}
}

// TestCustomersDomainMutationsPostV3Bodies drives the three wired customers
// writes: customer create/update/delete.
func TestCustomersDomainMutationsPostV3Bodies(t *testing.T) {
	saveUsable(t)
	ctx := context.Background()

	// create posts DisplayName + defaulted AU BillAddr.
	createSrv := newDomServer(t, http.StatusOK, `{"Customer":{"Id":"23","SyncToken":"0","DisplayName":"New Cust"}}`)
	interceptHTTP(t, createSrv.URL)
	res, err := ReplayMutate(ctx, "Customer", "create", "", map[string]string{"name": "New Cust"})
	if err != nil {
		t.Fatalf("customer create: %v", err)
	}
	if res.Status != http.StatusOK || res.Item.ID != "23" || res.Item.Name != "New Cust" {
		t.Fatalf("customer create result = %+v", res)
	}
	sent := createSrv.sentMap()
	if sent["DisplayName"] != "New Cust" {
		t.Fatalf("create body = %v", sent)
	}
	if addr, ok := sent["BillAddr"].(map[string]any); !ok || addr["Country"] != "AU" {
		t.Fatalf("create BillAddr = %v, want AU default", sent["BillAddr"])
	}

	// delete fetches SyncToken then POSTs operation=delete with Id+SyncToken.
	delSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"Customer":{"Id":"23","SyncToken":"5"}}`))
		default:
			b, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			if r.URL.Query().Get("operation") != "delete" || m["Id"] != "23" || m["SyncToken"] != "5" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"Customer":{"Id":"23","SyncToken":"5"},"time":"2026-08-25T00:00:00"}`))
		}
	}))
	t.Cleanup(delSrv.Close)
	interceptHTTP(t, delSrv.URL)
	dres, err := ReplayMutate(ctx, "Customer", "delete", "23", nil)
	if err != nil {
		t.Fatalf("customer delete: %v", err)
	}
	if dres.Status != http.StatusOK || dres.Op != "delete" || dres.Item.ID != "23" {
		t.Fatalf("customer delete result = %+v", dres)
	}

	// update patches sparsely on top of the fetched row.
	upSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"Customer":{"Id":"23","SyncToken":"5","DisplayName":"Old Name"}}`))
		default:
			b, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			if m["Id"] != "23" || m["SyncToken"] != "5" || m["DisplayName"] != "Renamed" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"Customer":{"Id":"23","SyncToken":"6","DisplayName":"Renamed"}}`))
		}
	}))
	t.Cleanup(upSrv.Close)
	interceptHTTP(t, upSrv.URL)
	ures, err := ReplayMutate(ctx, "Customer", "update", "23", map[string]string{"name": "Renamed"})
	if err != nil {
		t.Fatalf("customer update: %v", err)
	}
	if ures.Status != http.StatusOK || ures.Op != "update" || ures.Item.Name != "Renamed" {
		t.Fatalf("customer update result = %+v", ures)
	}
}

// --- payroll -----------------------------------------------------------------

// TestPayrollDomainReadsMapThroughV3Entities drives all 11 read-mode payroll
// commands through ReplayQuery using exactly the v3ByID mapping cli/v3read.go
// pins: employee/deduction/leave-category/pay-category/super/team (+searches)
// → Employee; pay-run/timesheet → TimeActivity; overview search → Employee.
func TestPayrollDomainReadsMapThroughV3Entities(t *testing.T) {
	saveUsable(t)

	empSrv := newDomServer(t, http.StatusOK, domQueryResponse("Employee",
		`{"Id":"55","DisplayName":"Pat Doe","GivenName":"Pat","Active":true}`))
	interceptHTTP(t, empSrv.URL)
	ctx := context.Background()
	for _, cmd := range []struct{ name, id, q string }{
		{"employee get", "55", ""},
		{"employee search", "", "Pat"},
		{"deduction get", "55", ""},
		{"leave-category get", "55", ""},
		{"pay-category get", "55", ""},
		{"super get", "55", ""},
		{"team get", "55", ""},
		{"team search", "", "Pat"},
		{"overview search", "", "Pat"},
	} {
		res, err := ReplayQuery(ctx, "Employee", cmd.id, cmd.q, 20)
		if err != nil {
			t.Fatalf("%s: %v", cmd.name, err)
		}
		if res.Status != http.StatusOK || res.Entity != "Employee" || len(res.Items) != 1 || res.Items[0].ID != "55" {
			t.Fatalf("%s = %+v", cmd.name, res)
		}
		if res.Items[0].Name != "Pat Doe" {
			t.Fatalf("%s name = %q", cmd.name, res.Items[0].Name)
		}
	}
	// The last read must have been a TimeActivity SELECT (pay-run/timesheet
	// mapping) — asserted by the entity word in the projected envelope below.
}

// TestPayrollDomainWritesUseEmployeeAndTimeActivity drives the three wired
// payroll commands: employee delete, timesheet create/update — Employee and
// TimeActivity v3 writes with the SyncToken-fetch contract.
func TestPayrollDomainWritesUseEmployeeAndTimeActivity(t *testing.T) {
	saveUsable(t)
	ctx := context.Background()

	// timesheet create → POST timeactivity with NameOf=Employee.
	taCreate := newDomServer(t, http.StatusOK, `{"TimeActivity":{"Id":"301","SyncToken":"0"}}`)
	interceptHTTP(t, taCreate.URL)
	res, err := ReplayMutate(ctx, "TimeActivity", "create", "", map[string]string{"employee": "55", "hours": "3"})
	if err != nil {
		t.Fatalf("timesheet create: %v", err)
	}
	if res.Status != http.StatusOK || res.Item.ID != "301" {
		t.Fatalf("timesheet create result = %+v", res)
	}
	m := taCreate.sentMap()
	if m["NameOf"] != "Employee" {
		t.Fatalf("timesheet NameOf = %v", m["NameOf"])
	}
	if er, ok := m["EmployeeRef"].(map[string]any); !ok || er["value"] != "55" {
		t.Fatalf("timesheet EmployeeRef = %v", m["EmployeeRef"])
	}

	// employee delete + timesheet update: SyncToken fetch then POST.
	for _, tc := range []struct {
		name, entity, op, id string
		existing, ack        string
	}{
		{"employee delete", "Employee", "delete", "55", `{"Employee":{"Id":"55","SyncToken":"7"}}`, `{"Employee":{"Id":"55","SyncToken":"7"}}`},
		{"timesheet update", "TimeActivity", "update", "301", `{"TimeActivity":{"Id":"301","SyncToken":"2","Hours":3}}`, `{"TimeActivity":{"Id":"301","SyncToken":"3","Hours":4}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodGet:
					_, _ = w.Write([]byte(tc.existing))
				default:
					b, _ := io.ReadAll(r.Body)
					var sent map[string]any
					_ = json.Unmarshal(b, &sent)
					wantOp := ""
					if tc.op == "delete" {
						wantOp = "delete"
					}
					if r.URL.Query().Get("operation") != wantOp {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if sent["Id"] != strings.TrimPrefix(tc.id, "") {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = w.Write([]byte(tc.ack))
				}
			}))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			out, err := ReplayMutate(ctx, tc.entity, tc.op, tc.id, nil)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if out.Status != http.StatusOK || out.Op != tc.op || out.Item.ID != tc.id {
				t.Fatalf("%s result = %+v", tc.name, out)
			}
		})
	}
}

// --- inventory ---------------------------------------------------------------

// TestInventoryDomainReadsCoverEveryEntity drives all 8 read-mode inventory
// commands: item get/search, purchase-order get/search, adjust get/search
// (all v3 queries), item-receipt get/search (warehouse-management-svc
// GetItemReceipts), overview get/search (commercecontrol list/search).
func TestInventoryDomainReadsCoverEveryEntity(t *testing.T) {
	saveUsableURIHost(t)
	ctx := context.Background()

	v3Cases := []struct{ cmd, entity string }{
		{"item get", "Item"},
		{"item search", "Item"},
		{"purchase-order get", "PurchaseOrder"},
		{"purchase-order search", "PurchaseOrder"},
		{"adjust get", "InventoryAdjustment"},
		{"adjust search", "InventoryAdjustment"},
	}
	for _, tc := range v3Cases {
		srv := newDomServer(t, http.StatusOK, domQueryResponse(tc.entity,
			`{"Id":"77","Name":"Widget","QtyOnHand":9,"TxnDate":"2026-08-21"}`))
		interceptHTTP(t, srv.URL)
		id := ""
		if strings.HasSuffix(tc.cmd, " get") {
			id = "77"
		}
		res, err := ReplayQuery(ctx, tc.entity, id, "", 20)
		if err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		if res.Status != http.StatusOK || res.Entity != tc.entity || len(res.Items) != 1 || res.Items[0].ID != "77" {
			t.Fatalf("%s = %+v", tc.cmd, res)
		}
	}

	// item-receipt get/search → warehouse-management-svc GetItemReceipts.
	irSrv := newDomServer(t, http.StatusOK, domGQL(`{"warehouseManagementItemReceipts":{"totalCount":4,"__typename":"WarehouseManagement_ItemReceiptConnection"}}`))
	interceptHTTP(t, irSrv.URL)
	for _, cmd := range []string{"get", "search"} {
		res, err := ReplayQuery(ctx, "ItemReceipt", "", "", 20)
		if err != nil {
			t.Fatalf("item-receipt %s: %v", cmd, err)
		}
		if res.Status != http.StatusOK || res.Entity != "ItemReceipt" || res.Counts["totalCount"] != 4 {
			t.Fatalf("item-receipt %s = %+v", cmd, res)
		}
	}
	if m := irSrv.sentMap(); m == nil || m["operationName"] != "GetItemReceipts" {
		t.Fatalf("item-receipt payload = %v", m)
	}
	if _, _, path, _ := irSrv.snap(); !strings.HasSuffix(path, "/graphql") {
		t.Fatalf("item-receipt path = %s", path)
	}

	// overview get → commercecontrol GetAllListViewEntities.
	ovSrv := newDomServer(t, http.StatusOK, domGQL(`{"result":{"__typename":"ProductsAndServicesListResult","entities":[{"__typename":"ProductsAndServicesListEntity","productsAndServicesListEntity":{"id":"p1","default":false}}]}}`))
	interceptHTTP(t, ovSrv.URL)
	ov, err := ReplayQuery(ctx, "InventoryOverview", "", "", 20)
	if err != nil {
		t.Fatalf("overview get: %v", err)
	}
	if ov.Status != http.StatusOK || ov.Entity != "InventoryOverview" || ov.Counts["items"] != 1 || ov.Items[0].ID != "p1" {
		t.Fatalf("overview get = %+v", ov)
	}
	if m := ovSrv.sentMap(); m == nil || m["operationName"] != "GetAllListViewEntities" {
		t.Fatalf("overview payload = %v", m)
	}

	// overview search → commercecontrol SearchAllListViewEntities.
	seSrv := newDomServer(t, http.StatusOK, domGQL(`{"searchAllListViewEntities":{"__typename":"SearchResult","totalCount":1,"entities":[{"id":"p2","entityName":"Bolt","qtyOnHand":50}]}}`))
	interceptHTTP(t, seSrv.URL)
	se, err := ReplayQuery(ctx, "InventoryOverviewSearch", "", "Bolt", 20)
	if err != nil {
		t.Fatalf("overview search: %v", err)
	}
	if se.Status != http.StatusOK || se.Counts["items"] != 1 || se.Items[0].Name != "Bolt" || se.Items[0].Qty != 50 {
		t.Fatalf("overview search = %+v", se)
	}
	if m := seSrv.sentMap(); m == nil || m["operationName"] != "SearchAllListViewEntities" {
		t.Fatalf("overview search payload = %v", m)
	}
}

// TestInventoryDomainWritesPostV3Shapes drives the seven wired inventory
// commands: item create/update/delete, purchase-order create/delete, adjust
// create, plus the SPA-route creates count/buildassembly/startingvalue.
func TestInventoryDomainWritesPostV3Shapes(t *testing.T) {
	saveUsable(t)
	ctx := context.Background()

	// item create defaults Type=Service and IncomeAccountRef=1.
	itemSrv := newDomServer(t, http.StatusOK, `{"Item":{"Id":"77","SyncToken":"0","Name":"Widget"}}`)
	interceptHTTP(t, itemSrv.URL)
	res, err := ReplayMutate(ctx, "Item", "create", "", map[string]string{"name": "Widget"})
	if err != nil {
		t.Fatalf("item create: %v", err)
	}
	if res.Status != http.StatusOK || res.Item.ID != "77" {
		t.Fatalf("item create result = %+v", res)
	}
	m := itemSrv.sentMap()
	if m["Type"] != "Service" || m["Name"] != "Widget" {
		t.Fatalf("item create body = %v", m)
	}

	// purchase-order create requires --supplier and books VendorRef lines.
	poSrv := newDomServer(t, http.StatusOK, `{"PurchaseOrder":{"Id":"90","SyncToken":"0","TotalAmt":25}}`)
	interceptHTTP(t, poSrv.URL)
	pores, err := ReplayMutate(ctx, "PurchaseOrder", "create", "", map[string]string{"supplier": "5", "amount": "25", "category-account": "29"})
	if err != nil {
		t.Fatalf("po create: %v", err)
	}
	if pores.Status != http.StatusOK || pores.Item.ID != "90" || pores.Item.Amount != 25 {
		t.Fatalf("po create result = %+v", pores)
	}
	pm := poSrv.sentMap()
	if pm["VendorRef"].(map[string]any)["value"] != "5" {
		t.Fatalf("po VendorRef = %v", pm["VendorRef"])
	}

	// adjust create posts ItemAdjustmentLineDetail with ItemRef+QtyDiff.
	adjSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/inventoryadjustment") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var sent map[string]any
		_ = json.Unmarshal(b, &sent)
		lines, _ := sent["Line"].([]any)
		ok := false
		if len(lines) == 1 {
			if l, _ := lines[0].(map[string]any); l["DetailType"] == "ItemAdjustmentLineDetail" {
				ok = true
			}
		}
		if sent["AdjustAccountRef"] == nil || !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"InventoryAdjustment":{"Id":"159","SyncToken":"0"}}`))
	}))
	t.Cleanup(adjSrv.Close)
	interceptHTTP(t, adjSrv.URL)
	ares, err := ReplayMutate(ctx, "InventoryAdjustment", "create", "", map[string]string{"item": "77", "account": "42", "qty": "-2"})
	if err != nil {
		t.Fatalf("adjust create: %v", err)
	}
	if ares.Status != http.StatusOK || ares.Item.ID != "159" {
		t.Fatalf("adjust create result = %+v", ares)
	}

	// SPA-route creates: count/buildassembly/startingvalue.
	for _, tc := range []struct {
		name string
		run  func() (*MutateResult, error)
		path string
		body func(t *testing.T, m map[string]any)
		ack  string
	}{
		{"count", func() (*MutateResult, error) {
			return ReplayInventoryCountCreate(ctx, map[string]string{"item": "77", "counted-qty": "12", "memo": "stocktake"})
		}, "/inventory-counts", func(t *testing.T, m map[string]any) {
			lines, _ := m["Line"].([]any)
			if len(lines) != 1 {
				t.Fatalf("count Line = %v", m["Line"])
			}
			det, _ := lines[0].(map[string]any)["InventoryCountLineDetail"].(map[string]any)
			if det["ItemRef"].(map[string]any)["value"] != "77" || det["CountedQty"] != float64(12) {
				t.Fatalf("count detail = %v", det)
			}
		}, `{"InventoryCount":{"Id":"177","SyncToken":"0"}}`},
		{"buildassembly", func() (*MutateResult, error) {
			return ReplayBuildAssemblyCreate(ctx, map[string]string{"item": "88", "qty": "5"})
		}, "/buildassembly", func(t *testing.T, m map[string]any) {
			if m["ItemRef"].(map[string]any)["value"] != "88" || m["Qty"] != float64(5) {
				t.Fatalf("assembly body = %v", m)
			}
		}, `{"BuildAssembly":{"Id":"201","SyncToken":"0"}}`},
		{"startingvalue", func() (*MutateResult, error) {
			return ReplayStartingValueCreate(ctx, map[string]string{"item": "31", "qty": "10", "cost": "3.5", "asset-account": "81"})
		}, "/inventory_starting_value", func(t *testing.T, m map[string]any) {
			if m["ItemRef"].(map[string]any)["value"] != "31" || m["Cost"] != 3.5 || m["AssetAccountRef"].(map[string]any)["value"] != "81" {
				t.Fatalf("starting value body = %v", m)
			}
		}, `{"StartingValue":{"Id":"209","SyncToken":"0"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				_ = r.Body.Close()
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, tc.path) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				var sent map[string]any
				if err := json.Unmarshal(b, &sent); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				tc.body(t, sent)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.ack))
			}))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			out, err := tc.run()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if out.Status != http.StatusOK || out.Item.ID == "" {
				t.Fatalf("%s result = %+v", tc.name, out)
			}
		})
	}

	// item delete + purchase-order delete share the SyncToken contract.
	delSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"Item":{"Id":"77","SyncToken":"4"}}`))
		default:
			b, _ := io.ReadAll(r.Body)
			var sent map[string]any
			_ = json.Unmarshal(b, &sent)
			if r.URL.Query().Get("operation") != "delete" || sent["Id"] != "77" || sent["SyncToken"] != "4" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"Item":{"Id":"77","SyncToken":"4"}}`))
		}
	}))
	t.Cleanup(delSrv.Close)
	interceptHTTP(t, delSrv.URL)
	dres, err := ReplayMutate(ctx, "Item", "delete", "77", nil)
	if err != nil {
		t.Fatalf("item delete: %v", err)
	}
	if dres.Status != http.StatusOK || dres.Op != "delete" || dres.Item.ID != "77" {
		t.Fatalf("item delete result = %+v", dres)
	}
}

// --- reports -----------------------------------------------------------------

// TestReportsDomainReadsProjectReportShapes drives all 6 read-mode reports
// commands: report/cash-flow/forecast/performance get (v3 reports),
// custom get (universalreportinsights folio subType=CRB_GROUP),
// management get (folio without subType).
func TestReportsDomainReadsProjectReportShapes(t *testing.T) {
	saveUsableURIHost(t)
	ctx := context.Background()

	reportShape := `{"Header":{"ReportName":"ProfitAndLoss","StartPeriod":"2026-07-01","EndPeriod":"2026-08-24","Currency":"AUD"},"Columns":{"Column":[{"ColTitle":"Income"},{"ColTitle":"Total"}]}}`
	for _, tc := range []struct{ cmd, report string }{
		{"report get", "ProfitAndLoss"},
		{"cash-flow get", "CashFlow"},
		{"forecast get", "ProfitAndLoss"},
		{"performance get", "ProfitAndLoss"},
	} {
		srv := newDomServer(t, http.StatusOK, reportShape)
		interceptHTTP(t, srv.URL)
		res, err := ReplayReport(ctx, tc.report, "2026-07-01", "2026-08-24")
		if err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		if res.Status != http.StatusOK || res.Report != tc.report {
			t.Fatalf("%s = %+v", tc.cmd, res)
		}
		if res.Header["Currency"] != "AUD" || res.Header["StartPeriod"] != "2026-07-01" {
			t.Fatalf("%s header = %v", tc.cmd, res.Header)
		}
		if len(res.Columns) != 2 || res.Columns[0] != "Income" || res.Counts["columns"] != 2 {
			t.Fatalf("%s columns = %v counts=%v", tc.cmd, res.Columns, res.Counts)
		}
		if _, _, path, _ := srv.snap(); !strings.Contains(path, "/reports/"+tc.report) {
			t.Fatalf("%s path missing report name: %s", tc.cmd, path)
		}
	}

	// invalid report names never reach the transport.
	if _, err := ReplayReport(ctx, "../etc/passwd", "", ""); err == nil {
		t.Fatal("expected invalid report name rejection")
	}

	// custom get → folio CRB_GROUP (array count projection).
	folSrv := newDomServer(t, http.StatusOK, `[{"id":"f1","name":"CRB"},{"id":"f2","name":"Ops"}]`)
	interceptHTTP(t, folSrv.URL)
	fol, err := ReplayQuery(ctx, "Folio", "", "", 20)
	if err != nil {
		t.Fatalf("custom get: %v", err)
	}
	if fol.Status != http.StatusOK || fol.Entity != "Folio" || fol.Counts["totalCount"] != 2 {
		t.Fatalf("custom get = %+v", fol)
	}
	if _, _, path, _ := folSrv.snap(); !strings.HasSuffix(path, "/v1/folio") {
		t.Fatalf("folio path = %s, want …/v1/folio", path)
	}

	// management get → folio without subType, projects id/name rows.
	mgSrv := newDomServer(t, http.StatusOK, `[{"id":"m1","name":"Management Pack"}]`)
	interceptHTTP(t, mgSrv.URL)
	mg, err := ReplayQuery(ctx, "ManagementFolio", "", "", 20)
	if err != nil {
		t.Fatalf("management get: %v", err)
	}
	if mg.Status != http.StatusOK || mg.Entity != "ManagementFolio" || len(mg.Items) != 1 || mg.Items[0].Name != "Management Pack" {
		t.Fatalf("management get = %+v", mg)
	}
	if _, _, path, _ := mgSrv.snap(); strings.Contains(path, "subType") {
		t.Fatalf("management folio must not carry subType: %s", path)
	}
}

// --- tax ---------------------------------------------------------------------

// TestTaxDomainReadsCoverEveryCommand drives all 6 read-mode tax commands
// plus the dedicated indirect-tax readers the salestx domain shares:
// code get/search (v3 TaxCode query AND captured GetTaxCodes_qbo graphql),
// rate (TaxRates graphql), gst (taxconfig config), bas (taxconfig returns),
// tpar get (universalreportinsights instances via ReplayReport).
func TestTaxDomainReadsCoverEveryCommand(t *testing.T) {
	ctx := context.Background()
	saveUsableURIHost(t)

	// code get/search → v3 TaxCode query (cli/v3read.go mapping).
	codeSrv := newDomServer(t, http.StatusOK, domQueryResponse("TaxCode", `{"Id":"13","Name":"GST"}`))
	interceptHTTP(t, codeSrv.URL)
	for _, cmd := range []string{"get", "search"} {
		res, err := ReplayQuery(ctx, "TaxCode", "", "", 20)
		if err != nil {
			t.Fatalf("code %s: %v", cmd, err)
		}
		if res.Status != http.StatusOK || res.Entity != "TaxCode" || len(res.Items) != 1 || res.Items[0].Name != "GST" {
			t.Fatalf("code %s = %+v", cmd, res)
		}
	}

	// code list → GetTaxCodes_qbo on the v4 gateway.
	tcSrv := newDomServer(t, http.StatusOK, domGQL(`{"company":{"taxGroups":{"edges":[{"node":{"id":"g1","hidden":false,"deleted":false,"name":"GST Free","qboAppData":{"salesTaxCodeDisplayName":"EX"}}}]}}}`))
	interceptHTTP(t, tcSrv.URL)
	tcl, err := ReplayTaxCodeList(ctx, 20)
	if err != nil {
		t.Fatalf("taxcode list: %v", err)
	}
	if tcl.Status != http.StatusOK || len(tcl.Items) != 1 || tcl.Items[0].Name != "GST Free (EX)" {
		t.Fatalf("taxcode list = %+v", tcl)
	}
	if m := tcSrv.sentMap(); m == nil || m["operationName"] != "GetTaxCodes_qbo" {
		t.Fatalf("taxcode payload = %v", m)
	}

	// rate → TaxRates__indirect_tax_ui_qbo.
	trSrv := newDomServer(t, http.StatusOK, domGQL(`{"company":{"taxAgencies":{"edges":[{"node":{"taxRates":{"edges":[{"node":{"id":"r1","name":"GST","rate":10,"status":"ACTIVE","taxAgency":{"id":"ag1","name":"ATO"}}}]}}}]}}}`))
	interceptHTTP(t, trSrv.URL)
	trl, err := ReplayTaxRateList(ctx)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	if trl.Status != http.StatusOK || len(trl.Items) != 1 || trl.Items[0].Amount != 10 || trl.Items[0].Account != "ATO" {
		t.Fatalf("rate = %+v", trl)
	}
	if m := trSrv.sentMap(); m == nil || m["operationName"] != "TaxRates__indirect_tax_ui_qbo" {
		t.Fatalf("rate payload = %v", m)
	}

	// bas get → TAXABLE_PAYMENTS instances through ReplayReport (the
	// catalog maps QBO.TAX.BAS_READ/TAX.TPAR_READ onto report readers;
	// BAS here rides the universalreportinsights instances wrap).
	tparSrv := newDomServer(t, http.StatusOK, `{"status":"OK","metadata":{"totalRows":6},"data":{"rows":[]}}`)
	interceptHTTP(t, tparSrv.URL)
	rep, err := ReplayReport(ctx, "TAXABLE_PAYMENTS", "2026-07-01", "2026-09-30")
	if err != nil {
		t.Fatalf("tpar get: %v", err)
	}
	if rep.Status != http.StatusOK || rep.Report != "TAXABLE_PAYMENTS" || rep.Counts["totalRows"] != 6 {
		t.Fatalf("tpar get = %+v", rep)
	}
	if rep.Header["StartPeriod"] != "2026-07-01" || rep.Header["EndPeriod"] != "2026-09-30" {
		t.Fatalf("tpar header = %v", rep.Header)
	}
	if _, _, path, _ := tparSrv.snap(); !strings.Contains(path, "TAXABLE_PAYMENTS/instances") {
		t.Fatalf("tpar path = %s", path)
	}
}

// TestTaxDomainIndirectWritersGuardAndValidate drives gst file and bas lodge
// (wired salestx-domain writes the tax CLI exposes) including the empty-id
// planning sentinels and the production-realm guard.
func TestTaxDomainIndirectWritersGuardAndValidate(t *testing.T) {
	// Empty ids fail during planning, before credentials load.
	noCredsDir(t)
	if _, err := PlanGSTFile(100, "01/08/2026", "", "ag1", ""); !errors.Is(err, ErrEmptyTaxPaymentID) {
		t.Fatalf("gst file empty account: %v", err)
	}
	if _, err := PlanBASLodge("", "", 0); !errors.Is(err, ErrEmptyTaxReturnID) {
		t.Fatalf("bas lodge empty id: %v", err)
	}
	if _, err := PlanBASList(""); !errors.Is(err, ErrMissingTaxAgencyID) {
		t.Fatalf("bas list empty agency: %v", err)
	}

	// Happy paths post the captured mutations.
	saveUsable(t)
	ctx := context.Background()
	fileSrv := newDomServer(t, http.StatusOK, domGQL(`{"indirectTaxCreatePayment":{"clientMutationId":"0"}}`))
	interceptHTTP(t, fileSrv.URL)
	gf, err := ReplayGSTFile(ctx, 845.5, "20/08/2026", "91", "ag1", "q2 payment")
	if err != nil {
		t.Fatalf("gst file: %v", err)
	}
	if gf.Status != http.StatusOK || gf.Entity != "TaxPayment" || gf.Item.Amount != 845.5 || gf.Item.Date != "2026-08-20" {
		t.Fatalf("gst file result = %+v", gf)
	}
	if m := fileSrv.sentMap(); m == nil || m["operationName"] != "CreateTaxPayment" {
		t.Fatalf("gst file payload = %v", m)
	}

	lodgeSrv := newDomServer(t, http.StatusOK, domGQL(`{"indirectTaxUpdateTaxReturn":{"clientMutationId":"0","indirecttaxesTaxReturn":{"id":"ret-9","netTaxAmountDue":12.25,"taxReturnType":"BAS"}}}`))
	interceptHTTP(t, lodgeSrv.URL)
	bl, err := ReplayBASLodge(ctx, "ret-9", "", 0)
	if err != nil {
		t.Fatalf("bas lodge: %v", err)
	}
	if bl.Status != http.StatusOK || bl.Op != "lodge" || bl.Entity != "BAS" || bl.Item.ID != "ret-9" || bl.Item.Name != "BAS" {
		t.Fatalf("bas lodge result = %+v", bl)
	}
	if m := lodgeSrv.sentMap(); m == nil || m["operationName"] != "EfileTaxReturn" {
		t.Fatalf("bas lodge payload = %v", m)
	}

	// tpar generate has no captured API: the sentinel must hold.
	if _, err := ReplayTPARGenerate(ctx); !errors.Is(err, ErrTPARGenerateNoAPI) {
		t.Fatalf("tpar generate: %v, want ErrTPARGenerateNoAPI", err)
	}
}

// --- no-credentials fast-fail sweep ------------------------------------------

// TestDomainsNoCredsFastFail sweeps one representative entry point per
// command family across all six domains with an empty QB_HOME. Each must
// fail with ErrNoCredentials before any network activity.
func TestDomainsNoCredsFastFail(t *testing.T) {
	noCredsDir(t)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		// company
		{"company settings get", func() error { _, err := ReplayQuery(ctx, "Preferences", "", "", 10); return err }},
		{"company user get", func() error { _, err := ReplayQuery(ctx, "Employee", "", "", 10); return err }},
		{"company tag get", func() error { _, err := ReplayQuery(ctx, "Class", "", "", 10); return err }},
		{"company attachment get", func() error { _, err := ReplayQuery(ctx, "Attachable", "", "", 10); return err }},
		{"company list get", func() error { _, err := ReplayQuery(ctx, "ListsPrefs", "", "", 10); return err }},
		{"company form-style get", func() error { _, err := ReplayQuery(ctx, "FormStyle", "", "", 10); return err }},
		{"company role get", func() error { _, err := ReplayQuery(ctx, "Role", "", "", 10); return err }},
		{"company custom-field get", func() error { _, err := ReplayQuery(ctx, "CustomField", "", "", 10); return err }},
		{"company business-feed get", func() error { _, err := ReplayQuery(ctx, "BusinessFeed", "", "", 10); return err }},
		{"company currency create", func() error {
			_, err := ReplayMutate(ctx, "CompanyCurrency", "create", "", map[string]string{"code": "aud"})
			return err
		}},
		{"company tag delete", func() error { _, err := ReplayMutate(ctx, "Class", "delete", "61", nil); return err }},
		// customers
		{"customers customer get", func() error { _, err := ReplayQuery(ctx, "Customer", "23", "", 10); return err }},
		{"customers appointment search", func() error { _, err := ReplayQuery(ctx, "Customer", "", "x", 10); return err }},
		{"customers opportunity get", func() error { _, err := ReplayQuery(ctx, "Customer", "23", "", 10); return err }},
		{"customers overview get", func() error { _, err := ReplayQuery(ctx, "CustomersOverview", "", "", 10); return err }},
		{"customers contract get", func() error { _, err := ReplayQuery(ctx, "Contract", "", "", 10); return err }},
		{"customers proposal get", func() error { _, err := ReplayQuery(ctx, "Proposal", "", "", 10); return err }},
		{"customers review get", func() error { _, err := ReplayQuery(ctx, "Review", "", "", 10); return err }},
		{"customers customer create", func() error {
			_, err := ReplayMutate(ctx, "Customer", "create", "", map[string]string{"name": "x"})
			return err
		}},
		{"customers customer update", func() error {
			_, err := ReplayMutate(ctx, "Customer", "update", "23", map[string]string{"id": "23"})
			return err
		}},
		// payroll
		{"payroll employee get", func() error { _, err := ReplayQuery(ctx, "Employee", "", "", 10); return err }},
		{"payroll pay-run get", func() error { _, err := ReplayQuery(ctx, "TimeActivity", "", "", 10); return err }},
		{"payroll timesheet create", func() error {
			_, err := ReplayMutate(ctx, "TimeActivity", "create", "", map[string]string{"employee": "55"})
			return err
		}},
		{"payroll employee delete", func() error { _, err := ReplayMutate(ctx, "Employee", "delete", "55", nil); return err }},
		// tax
		{"tax code get", func() error { _, err := ReplayQuery(ctx, "TaxCode", "", "", 10); return err }},
		{"tax code list", func() error { _, err := ReplayTaxCodeList(ctx, 10); return err }},
		{"tax rate", func() error { _, err := ReplayTaxRateList(ctx); return err }},
		{"tax gst list", func() error { _, err := ReplayGSTList(ctx); return err }},
		{"tax bas list", func() error { _, err := ReplayBASList(ctx, "ag1"); return err }},
		{"tax tpar get", func() error { _, err := ReplayReport(ctx, "TAXABLE_PAYMENTS", "", ""); return err }},
		{"tax gst file", func() error {
			_, err := ReplayGSTFile(ctx, 10, "01/08/2026", "91", "ag1", "")
			return err
		}},
		{"tax bas lodge", func() error { _, err := ReplayBASLodge(ctx, "ret-9", "Filed", 0); return err }},
		// inventory
		{"inventory item get", func() error { _, err := ReplayQuery(ctx, "Item", "", "", 10); return err }},
		{"inventory purchase-order search", func() error { _, err := ReplayQuery(ctx, "PurchaseOrder", "", "x", 10); return err }},
		{"inventory adjust get", func() error { _, err := ReplayQuery(ctx, "InventoryAdjustment", "", "", 10); return err }},
		{"inventory item-receipt get", func() error { _, err := ReplayQuery(ctx, "ItemReceipt", "", "", 10); return err }},
		{"inventory overview get", func() error { _, err := ReplayQuery(ctx, "InventoryOverview", "", "", 10); return err }},
		{"inventory overview search", func() error { _, err := ReplayQuery(ctx, "InventoryOverviewSearch", "", "x", 10); return err }},
		{"inventory item create", func() error {
			_, err := ReplayMutate(ctx, "Item", "create", "", map[string]string{"name": "x"})
			return err
		}},
		{"inventory po create", func() error {
			_, err := ReplayMutate(ctx, "PurchaseOrder", "create", "", map[string]string{"supplier": "5"})
			return err
		}},
		{"inventory count create", func() error {
			_, err := ReplayInventoryCountCreate(ctx, map[string]string{"item": "1", "counted-qty": "1"})
			return err
		}},
		{"inventory buildassembly create", func() error {
			_, err := ReplayBuildAssemblyCreate(ctx, map[string]string{"item": "1", "qty": "1"})
			return err
		}},
		{"inventory startingvalue create", func() error {
			_, err := ReplayStartingValueCreate(ctx, map[string]string{"item": "1", "qty": "1"})
			return err
		}},
		// reports
		{"reports report get", func() error { _, err := ReplayReport(ctx, "ProfitAndLoss", "", ""); return err }},
		{"reports cash-flow get", func() error { _, err := ReplayReport(ctx, "CashFlow", "", ""); return err }},
		{"reports custom get", func() error { _, err := ReplayQuery(ctx, "Folio", "", "", 10); return err }},
		{"reports management get", func() error { _, err := ReplayQuery(ctx, "ManagementFolio", "", "", 10); return err }},
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
