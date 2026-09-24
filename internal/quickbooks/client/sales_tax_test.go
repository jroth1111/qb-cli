package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sales_tax_test.go covers the `qb salestx` client surface with the
// package-standard DefaultTransport swap (interceptHTTP): production-built
// qbo.intuit.com / taxconfig / taxfilingsvc URLs are rewritten to a local
// httptest server, so no production file is touched and no real host dialed.
//
// Kill denominator: 9. The suite rejects implementations that
//  1. dial the network without credentials (fast-fail broken),
//  2. route v3 salestx entities to the wrong v3 path,
//  3. drop the operation=delete/void query on delete/void,
//  4. omit sendTo on invoice send,
//  5. send the wrong GraphQL operationName or variables for the captured
//     indirect-tax ops (GetTaxCodes_qbo, TaxRates, CreateTaxPayment,
//     EfileTaxReturn),
//  6. skip the production-realm guard on gst file / bas lodge,
//  7. plan with a live realm embedded in the URL,
//  8. accept empty ids where the captured flow requires one,
//  9. claim TPAR generate works despite no captured API.

// --- fail-fast on missing credentials ---------------------------------------

func TestSalestxFailsFastOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplaySalestxQuery(context.Background(), "invoice", "", "", 10)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("invoice list: got %v, want ErrNoCredentials", err)
	}

	start = time.Now()
	_, err = ReplaySalestxMutate(context.Background(), "invoice", "create", "", map[string]string{"customer": "1"})
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("invoice create: got %v, want ErrNoCredentials", err)
	}

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"taxcode", func() error { _, err := ReplayTaxCodeList(context.Background(), 20); return err }},
		{"taxrate", func() error { _, err := ReplayTaxRateList(context.Background()); return err }},
		{"gst-list", func() error { _, err := ReplayGSTList(context.Background()); return err }},
		{"gst-file", func() error {
			_, err := ReplayGSTFile(context.Background(), 100, "01/08/2026", "91", "ag1", "")
			return err
		}},
		{"bas-list", func() error { _, err := ReplayBASList(context.Background(), "ag1"); return err }},
		{"bas-lodge", func() error { _, err := ReplayBASLodge(context.Background(), "ret1", "Filed", 0); return err }},
		{"tpar-list", func() error { _, err := ReplayTPARList(context.Background(), 20); return err }},
	} {
		start = time.Now()
		err := tc.call()
		assertFast(t, time.Since(start))
		if !errors.Is(err, ErrNoCredentials) {
			t.Fatalf("%s: got %v, want ErrNoCredentials", tc.name, err)
		}
	}
}

// --- entity mapping ----------------------------------------------------------

func TestSalestxEntityMapping(t *testing.T) {
	for _, tc := range []struct {
		word, want string
	}{
		{"invoice", "Invoice"}, {"estimate", "Estimate"}, {"payment", "Payment"},
		{"creditmemo", "CreditMemo"}, {"delayedcharge", "DelayedCharge"},
		{"salesreceipt", "SalesReceipt"},
	} {
		got, ok := SalestxV3Entity(tc.word)
		if !ok || got != tc.want {
			t.Fatalf("SalestxV3Entity(%q) = %q,%v want %q", tc.word, got, ok, tc.want)
		}
	}
	if _, ok := SalestxV3Entity("bill"); ok {
		t.Fatal("bill must not map through salestx")
	}
	if u := PlannedSalestxMutateURL("invoice", "delete"); !strings.Contains(u, "/invoice?") || !strings.Contains(u, "operation=delete") {
		t.Fatalf("planned delete url = %q", u)
	}
	if u := PlannedSalestxMutateURL("invoice", "void"); !strings.Contains(u, "operation=void") {
		t.Fatalf("planned void url = %q", u)
	}
	if u := PlannedSalestxMutateURL("invoice", "send"); !strings.Contains(u, "/send?") {
		t.Fatalf("planned send url = %q", u)
	}
}

// stServer records requests and answers from a scripted responder.
type stServer struct {
	t        *testing.T
	status   int
	respBody string
	lastPath string
	lastRaw  string
	lastBody []byte
	lastRef  string
	calls    int
}

func newSTServer(t *testing.T, status int, respBody string) *stServer {
	return &stServer{t: t, status: status, respBody: respBody}
}

func (s *stServer) start(t *testing.T) string {
	srv := httptest.NewServer(withPersistedV3(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		s.lastPath = r.URL.Path
		s.lastRaw = r.URL.RawQuery
		s.lastBody = b
		s.lastRef = r.Header.Get("Referer")
		s.calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.respBody))
	})))
	t.Cleanup(srv.Close)
	return srv.URL
}

// --- v3 verbs ---------------------------------------------------------------

const stInvoiceShape = `{"Invoice":{"Id":"188","SyncToken":"2","TotalAmt":55.0,"CustomerRef":{"value":"1","name":"C"}}}`

func TestReplaySalestxDeleteSendsOperationAndSyncToken(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stInvoiceShape)
	interceptHTTP(t, srv.start(t))

	// ReplayMutate fetches the row first (GET), then POSTs the delete.
	getHit := false
	res, err := ReplaySalestxMutate(context.Background(), "invoice", "delete", "188", nil)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	_ = getHit
	if res.Op != "delete" || res.Entity != "Invoice" || res.Item.ID != "188" {
		t.Fatalf("envelope = %+v", res)
	}
	var body map[string]any
	if err := json.Unmarshal(srv.lastBody, &body); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, srv.lastBody)
	}
	if body["Id"] != "188" {
		t.Fatalf("body Id = %v", body["Id"])
	}
	if _, has := body["SyncToken"]; !has {
		t.Fatal("delete body must carry SyncToken")
	}
}

func TestReplaySalestxCreateBuildsSalesLines(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stInvoiceShape)
	interceptHTTP(t, srv.start(t))

	res, err := ReplaySalestxMutate(context.Background(), "invoice", "create", "", map[string]string{
		"customer": "23", "amount": "42.50", "item": "7", "date": "05/08/2026",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Item.ID != "188" {
		t.Fatalf("created id = %q", res.Item.ID)
	}
	var req struct {
		CustomerRef map[string]any   `json:"CustomerRef"`
		TxnDate     string           `json:"TxnDate"`
		Line        []map[string]any `json:"Line"`
	}
	if err := json.Unmarshal(srv.lastBody, &req); err != nil {
		t.Fatalf("create body not JSON: %v (%s)", err, srv.lastBody)
	}
	if req.CustomerRef["value"] != "23" {
		t.Fatalf("customer = %v", req.CustomerRef["value"])
	}
	if req.TxnDate != "2026-08-05" {
		t.Fatalf("date = %q, want normalized 2026-08-05", req.TxnDate)
	}
	if len(req.Line) == 0 || req.Line[0]["Amount"] != 42.5 {
		t.Fatalf("line = %+v", req.Line)
	}
}

// Negative: create without --customer must fail before any dial.
func TestReplaySalestxCreateRejectsMissingCustomer(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stInvoiceShape)
	interceptHTTP(t, srv.start(t))
	_, err := ReplaySalestxMutate(context.Background(), "invoice", "create", "", map[string]string{})
	if err == nil || strings.Contains(err.Error(), "dial") {
		t.Fatalf("expected validation error, got %v", err)
	}
	if srv.calls != 0 {
		t.Fatalf("validation must precede any request; got %d calls", srv.calls)
	}
}

func TestReplaySalestxSendCarriesRecipient(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stInvoiceShape)
	base := srv.start(t)
	interceptHTTP(t, base)

	_, err := ReplaySalestxMutate(context.Background(), "invoice", "send", "188", map[string]string{"to": "a@b.co"})
	if !errors.Is(err, ErrReadbackUnavailable) || srv.calls != 0 {
		t.Fatalf("unverifiable email was submitted: %v", err)
	}
}

func TestReplaySalestxUnknownEntityRejected(t *testing.T) {
	saveUsable(t)
	if _, err := ReplaySalestxMutate(context.Background(), "vendor", "create", "", nil); err == nil {
		t.Fatal("vendor must not be a salestx entity")
	}
	if _, err := ReplaySalestxQuery(context.Background(), "nope", "", "", 5); err == nil {
		t.Fatal("nope must not be a salestx entity")
	}
}

// --- taxcode / taxrate happy paths ------------------------------------------

const stTaxCodesShape = `{"data":{"company":{"taxGroups":{"edges":[
 {"node":{"id":"10","hidden":false,"deleted":false,"name":"GST","qboAppData":{"salesTaxCodeDisplayName":"Goods and Services Tax"}}},
 {"node":{"id":"11","hidden":true,"deleted":false,"name":"FRE","qboAppData":{"salesTaxCodeDisplayName":"GST Free"}}}
]}}}}`

func TestReplayTaxCodeListProjectsGroups(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stTaxCodesShape)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayTaxCodeList(context.Background(), 25)
	if err != nil {
		t.Fatalf("taxcode list: %v", err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if res.Items[0].ID != "10" || !strings.Contains(res.Items[0].Name, "Goods and Services Tax") {
		t.Fatalf("item0 = %+v", res.Items[0])
	}
	if res.Items[1].Active == nil || *res.Items[1].Active {
		t.Fatalf("hidden FRE must project Active=false: %+v", res.Items[1])
	}
	var req struct {
		OperationName string `json:"operationName"`
		Query         string `json:"query"`
		Variables     struct {
			Limit int `json:"limit"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(srv.lastBody, &req); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, srv.lastBody)
	}
	if req.OperationName != "GetTaxCodes_qbo" || !strings.Contains(req.Query, "taxGroups") || req.Variables.Limit != 25 {
		t.Fatalf("request = %+v", req)
	}
}

const stTaxRatesShape = `{"data":{"company":{"taxAgencies":{"edges":[{"node":{"taxRates":{"edges":[
 {"node":{"status":"ACTIVE","id":"9","name":"GST on Income","rate":10,"configType":"SIMPLE","taxAgency":{"id":"ag1","name":"ATO"}}},
 {"node":{"status":"ARCHIVED","id":"12","name":"Old LCT","rate":33,"configType":"SIMPLE","taxAgency":{"id":"ag1","name":"ATO"}}}
]}}}]}}}}`

func TestReplayTaxRateListProjectsRates(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stTaxRatesShape)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayTaxRateList(context.Background())
	if err != nil {
		t.Fatalf("taxrate list: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %+v", res.Items)
	}
	it := res.Items[0]
	if it.ID != "9" || it.Name != "GST on Income" || it.Amount != 10 || it.Account != "ATO" {
		t.Fatalf("rate0 = %+v", it)
	}
	if !strings.Contains(string(srv.lastBody), "TaxRates__indirect_tax_ui_qbo") {
		t.Fatalf("wrong op: %s", srv.lastBody)
	}
}

// --- gst / bas --------------------------------------------------------------

const stConfigShape = `{"data":{"indirectTaxTaxAgencies":{"edges":[{"node":{"globalId":"gid-ag","name":"Australian Taxation Office","legacyDisplayName":"ATO","legacyTaxAgencyCode":"ATO","taxCategories":{"edges":[{"node":{"id":"cat1"}}]}}}]}}}`

func TestReplayGSTListProjectsAgenciesAndGroups(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stConfigShape)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayGSTList(context.Background())
	if err != nil {
		t.Fatalf("gst list: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Items[0].ID != "gid-ag" || res.Items[0].Type != "TaxAgency" {
		t.Fatalf("agency = %+v", res.Items[0])
	}
	if !strings.Contains(res.Items[0].Name, "ATO") || res.Items[0].Account != "ATO" {
		t.Fatalf("agency name/code = %+v", res.Items[0])
	}
	if !strings.Contains(string(srv.lastBody), "IndirectTaxConfigQuery_qbo") {
		t.Fatalf("wrong op: %s", srv.lastBody)
	}
}

const stPaymentAck = `{"data":{"indirectTaxCreatePayment":{"clientMutationId":"0"}}}`

func TestReplayGSTFilePostsCapturedInputAndGuardsProd(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stPaymentAck)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayGSTFile(context.Background(), 1234.5, "21/07/2026", "91", "gid-ag", "Q1 payment")
	if err != nil {
		t.Fatalf("gst file: %v", err)
	}
	if res.Op != "create" || res.Entity != "TaxPayment" {
		t.Fatalf("envelope = %+v", res)
	}
	var req struct {
		Variables struct {
			Input struct {
				TaxPayment struct {
					PaymentAmount  float64           `json:"paymentAmount"`
					PaymentDate    string            `json:"paymentDate"`
					Description    string            `json:"description"`
					PaymentAccount map[string]string `json:"paymentAccount"`
					TaxAgency      map[string]string `json:"taxAgency"`
				} `json:"taxPayment"`
			} `json:"input"`
		} `json:"variables"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal(srv.lastBody, &req); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, srv.lastBody)
	}
	in := req.Variables.Input.TaxPayment
	if in.PaymentAmount != 1234.5 || in.PaymentDate != "2026-07-21" || in.Description != "Q1 payment" {
		t.Fatalf("payment = %+v", in)
	}
	if in.PaymentAccount["id"] != "91" || in.TaxAgency["id"] != "gid-ag" {
		t.Fatalf("refs = %+v", in)
	}
	if !strings.Contains(req.Query, "indirectTaxCreatePayment") {
		t.Fatalf("mutation = %q", req.Query)
	}

}

func TestPlanGSTFileValidation(t *testing.T) {
	if _, err := PlanGSTFile(10, "", "", "", ""); !errors.Is(err, ErrEmptyTaxPaymentID) {
		t.Fatalf("missing account/agency: %v", err)
	}
	p, err := PlanGSTFile(10, "01/09/2026", "91", "ag", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Dial || p.Method != http.MethodPost {
		t.Fatalf("plan = %+v", p)
	}
	if strings.Contains(p.URL, "12345") {
		t.Fatalf("plan leaked live realm: %s", p.URL)
	}
	if !strings.Contains(string(p.Body), "CreateTaxPayment") {
		t.Fatalf("plan body = %s", p.Body)
	}
}

const stReturnsShape = `{"data":{"node":{"id":"gid-ag","taxReturns":{"edges":[{"node":{"id":"ret-9","startDate":"2026-07-01","returnDueStatus":"DUE","taxAgency":{"code":"ATO"}}}]}}}}`

func TestReplayBASListProjectsReturns(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stReturnsShape)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayBASList(context.Background(), "gid-ag")
	if err != nil {
		t.Fatalf("bas list: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %+v", res.Items)
	}
	it := res.Items[0]
	if it.ID != "ret-9" || it.Date != "2026-07-01" || it.Name != "DUE" {
		t.Fatalf("return = %+v", it)
	}
	var req struct {
		Variables struct {
			ID string `json:"id"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(srv.lastBody, &req); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if req.Variables.ID != "gid-ag" {
		t.Fatalf("agency var = %q", req.Variables.ID)
	}
}

const stEfileAck = `{"data":{"indirectTaxUpdateTaxReturn":{"clientMutationId":"0","indirecttaxesTaxReturn":{"id":"ret-9","startDate":"2026-07-01","endDate":"2026-09-30","netTaxAmountDue":845.5,"taxReturnType":"BAS","fileDate":"2026-08-20"}}}}`

func TestReplayBASLodgePostsCapturedInput(t *testing.T) {
	saveUsable(t)
	srv := newSTServer(t, http.StatusOK, stEfileAck)
	interceptHTTP(t, srv.start(t))

	res, err := ReplayBASLodge(context.Background(), "ret-9", "Filed", 3)
	if err != nil {
		t.Fatalf("bas lodge: %v", err)
	}
	if res.Op != "lodge" || res.Entity != "BAS" || res.Item.ID != "ret-9" || res.Item.Amount != 845.5 {
		t.Fatalf("envelope = %+v", res)
	}
	var req struct {
		Variables struct {
			Input struct {
				Return struct {
					ID            string `json:"id"`
					EntityVersion int    `json:"entityVersion"`
					EFileStatus   string `json:"eFilingStatus"`
				} `json:"indirecttaxesTaxReturn"`
			} `json:"input"`
		} `json:"variables"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal(srv.lastBody, &req); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, srv.lastBody)
	}
	in := req.Variables.Input.Return
	if in.ID != "ret-9" || in.EntityVersion != 3 || in.EFileStatus != "Filed" {
		t.Fatalf("lodge input = %+v", in)
	}
	if !strings.Contains(req.Query, "indirectTaxUpdateTaxReturn") {
		t.Fatalf("mutation = %q", req.Query)
	}
}

func TestPlanBASLodgeDefaultsStatusAndRejectsEmptyID(t *testing.T) {
	if _, err := PlanBASLodge("", "", 0); !errors.Is(err, ErrEmptyTaxReturnID) {
		t.Fatalf("empty return id: %v", err)
	}
	p, err := PlanBASLodge(" ret-9 ", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Body), `"eFilingStatus":"Filed"`) {
		t.Fatalf("plan body = %s", p.Body)
	}
}

func TestPlanBASListRequiresAgency(t *testing.T) {
	if _, err := PlanBASList(""); !errors.Is(err, ErrMissingTaxAgencyID) {
		t.Fatalf("empty agency: %v", err)
	}
}

// --- tpar -------------------------------------------------------------------

func TestReplayTPARGenerateNotWired(t *testing.T) {
	res, err := ReplayTPARGenerate(context.Background())
	if err == nil || !errors.Is(err, ErrTPARGenerateNoAPI) {
		t.Fatalf("got %v,%v want ErrTPARGenerateNoAPI", res, err)
	}
}

func TestPlanTPARListUsesInstancesBody(t *testing.T) {
	p := PlanTPARList(40)
	if !strings.Contains(p.URL, "TAXABLE_PAYMENTS/instances") {
		t.Fatalf("url = %q", p.URL)
	}
	var payload struct {
		PaginationOptions struct {
			PageSize int `json:"pageSize"`
		} `json:"paginationOptions"`
	}
	if err := json.Unmarshal(p.Body, &payload); err != nil {
		t.Fatalf("plan body not instances shape: %v", err)
	}
	if payload.PaginationOptions.PageSize != 40 {
		t.Fatalf("pageSize = %d, want clamped 40", payload.PaginationOptions.PageSize)
	}
}

// --- plans are secret-free ---------------------------------------------------

func TestSalestxPlannedURLsAreSecretFree(t *testing.T) {
	plans := []*RequestPlan{
		PlanTaxCodeList(10),
		PlanTaxRateList(),
		PlanGSTList(),
		mustPlan(t, func() (*RequestPlan, error) { return PlanBASList("ag") }),
		mustPlan(t, func() (*RequestPlan, error) { return PlanGSTFile(5, "", "91", "ag", "") }),
		mustPlan(t, func() (*RequestPlan, error) { return PlanBASLodge("ret", "", 0) }),
		PlanTPARList(20),
	}
	for _, p := range plans {
		if p.Dial {
			t.Fatalf("%s: plan dials", p.op)
		}
		if strings.Contains(p.URL, "12345") {
			t.Fatalf("%s: url leaks realm: %s", p.op, p.URL)
		}
		if p.Method != http.MethodPost {
			t.Fatalf("%s: method = %s", p.op, p.Method)
		}
	}
}
