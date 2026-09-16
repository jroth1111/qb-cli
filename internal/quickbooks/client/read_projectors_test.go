package client

// read_projectors_test.go pins the projector-fix-a-13/b-14 repairs: the
// count-only live read projectors (contract modified-envelopes,
// customFieldDefinitions, WASAllRules, GetItemReceipts, proposals,
// feedback-engagement, folio, GetSchedules, TPAR instances) must preserve
// one secret-free QueryItem per returned object or GraphQL node, keep
// their existing totalCount/note contracts, and stay explicit-empty on
// malformed bodies. GraphQL edges unwrap through .node; item receipts
// prefer the known nodes array and fall back to edge nodes. TPAR carries
// the instances data.rows subtree verbatim into ReportResult.Rows — never
// the whole envelope. Unmapped keys (authorization, cookies, tokens,
// nested blobs) never reach the envelope.
//
// Rejects: count-only projection that drops populated rows, echoing raw
// item JSON, and silent success on unparseable responses.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// --- contract (esign modified-envelopes, bare array) ------------------------

func TestProjectModifiedEnvelopesPreservesRows(t *testing.T) {
	res := projectModifiedEnvelopes([]byte(`[
		{"envelopeId":"env-1","name":"MSA 2026","status":"SIGNED","lastModifiedDate":"2026-09-01"},
		{"id":"env-2","subject":"W-9","status":"SENT","createdDate":"2026-08-20"},
		"not-an-object",
		42
	]`))
	if res.Entity != "Contract" {
		t.Fatalf("entity=%q, want Contract", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved rows", got)
	}
	if got := res.Counts["totalCount"]; got != 4 {
		t.Fatalf("counts.totalCount=%d, want 4 (raw element count)", got)
	}
	if len(res.Items) != 2 {
		t.Fatalf("len(items)=%d, want 2", len(res.Items))
	}
	it := res.Items[0]
	if it.ID != "env-1" || it.Name != "MSA 2026" || it.Type != "SIGNED" || it.Date != "2026-09-01" {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "env-2" || it.Name != "W-9" || it.Type != "SENT" || it.Date != "2026-08-20" {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectModifiedEnvelopesMalformedStaysUnparsed(t *testing.T) {
	res := projectModifiedEnvelopes([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
	if strings.Contains(res.Note, "not json") {
		t.Fatalf("note echoes the raw body: %q", res.Note)
	}
}

func TestProjectModifiedEnvelopesEmptyArray(t *testing.T) {
	res := projectModifiedEnvelopes([]byte(`[]`))
	if res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 || len(res.Items) != 0 {
		t.Fatalf("empty array must yield zero counts, got %+v", res)
	}
}

// --- custom field (GraphQL edges → nodes, deleted inverts Active) -----------

func TestProjectCustomFieldsPreservesNodes(t *testing.T) {
	res := projectCustomFieldsQueryCES([]byte(`{"data":{"customFieldDefinitions":{"edges":[
		{"node":{"id":"cf1","name":"Colourway","deleted":false}},
		{"node":{"id":"cf2","name":"Size Run","deleted":true,"__typename":"CustomFieldDefinition"}},
		{"cursor":"no-node"}
	]}}}`))
	if res.Entity != "CustomField" {
		t.Fatalf("entity=%q, want CustomField", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved nodes", got)
	}
	if got := res.Counts["totalCount"]; got != 3 {
		t.Fatalf("counts.totalCount=%d, want 3 (edge count)", got)
	}
	if len(res.Items) != 2 {
		t.Fatalf("len(items)=%d, want 2", len(res.Items))
	}
	it := res.Items[0]
	if it.ID != "cf1" || it.Name != "Colourway" || it.Type != "CustomField" {
		t.Fatalf("item0 = %+v", it)
	}
	if it.Active == nil || !*it.Active {
		t.Fatalf("cf1 deleted=false must project Active=true, got %+v", it.Active)
	}
	it = res.Items[1]
	if it.ID != "cf2" || it.Name != "Size Run" {
		t.Fatalf("item1 = %+v", it)
	}
	if it.Active == nil || *it.Active {
		t.Fatalf("cf2 deleted=true must project Active=false, got %+v", it.Active)
	}
}

func TestProjectCustomFieldsMalformedStaysUnparsed(t *testing.T) {
	res := projectCustomFieldsQueryCES([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
}

func TestProjectCustomFieldsErrorsKeepNote(t *testing.T) {
	res := projectCustomFieldsQueryCES([]byte(`{"errors":[{"message":"denied"}],"data":null}`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 {
		t.Fatalf("gql error body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "gql: denied") {
		t.Fatalf("note=%q must carry the gql error message", res.Note)
	}
}

// --- workflow (GraphQL edges → nodes) ----------------------------------------

func TestProjectWASAllRulesPreservesNodes(t *testing.T) {
	res := projectWASAllRules([]byte(`{"data":{"workflows":{"definitions":{"edges":[
		{"node":{"id":"w1","definitionKey":"key.approval","name":"appr","displayName":"Bill approval","status":"ACTIVE"}},
		{"node":{"id":"w2","name":"reminder","status":"PAUSED"}},
		{"cursor":"no-node"}
	]}}}}`))
	if res.Entity != "Workflow" {
		t.Fatalf("entity=%q, want Workflow", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved nodes", got)
	}
	if got := res.Counts["totalCount"]; got != 3 {
		t.Fatalf("counts.totalCount=%d, want 3 (edge count)", got)
	}
	it := res.Items[0]
	if it.ID != "w1" || it.Name != "Bill approval" || it.Type != "ACTIVE" {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "w2" || it.Name != "reminder" || it.Type != "PAUSED" {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectWASAllRulesMalformedStaysUnparsed(t *testing.T) {
	res := projectWASAllRules([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
}

// --- item receipt (known nodes, edge-node fallback) --------------------------

func TestProjectGetItemReceiptsPreservesNodes(t *testing.T) {
	res := projectGetItemReceipts([]byte(`{"data":{"warehouseManagementItemReceipts":{
		"__typename":"WarehouseManagement_ItemReceiptConnection",
		"totalCount":5,
		"nodes":[
			{"id":"ir1","txnDate":"2026-09-01","referenceNo":"RN-100","status":"RECEIVED","memo":"Dock A","vendor":{"id":"v9","name":"Acme Supplies"}},
			{"id":"ir2","txnDate":"2026-08-29","status":"OPEN"}
		],
		"edges":[{"node":{"id":"ir1"}},{"node":{"id":"ir2"}}]
	}}}`))
	if res.Entity != "ItemReceipt" {
		t.Fatalf("entity=%q, want ItemReceipt", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved nodes", got)
	}
	if got := res.Counts["totalCount"]; got != 5 {
		t.Fatalf("counts.totalCount=%d, want 5 (API field)", got)
	}
	it := res.Items[0]
	if it.ID != "ir1" || it.Date != "2026-09-01" || it.DocNumber != "RN-100" ||
		it.Type != "RECEIVED" || it.Name != "Dock A" || it.Account != "Acme Supplies" || it.AccountID != "v9" {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "ir2" || it.Date != "2026-08-29" || it.Type != "OPEN" {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectGetItemReceiptsFallsBackToEdgeNodes(t *testing.T) {
	res := projectGetItemReceipts([]byte(`{"data":{"warehouseManagementItemReceipts":{
		"__typename":"WarehouseManagement_ItemReceiptConnection",
		"totalCount":2,
		"edges":[{"cursor":"c1","node":{"id":"ir7"}},{"cursor":"c2"}]
	}}}`))
	if got := res.Counts["items"]; got != 1 {
		t.Fatalf("counts.items=%d, want 1 (only edge with a node object)", got)
	}
	if res.Items[0].ID != "ir7" {
		t.Fatalf("item0 = %+v, want id ir7", res.Items[0])
	}
	if got := res.Counts["totalCount"]; got != 2 {
		t.Fatalf("counts.totalCount=%d, want 2", got)
	}
}

func TestProjectGetItemReceiptsMalformedStaysUnparsed(t *testing.T) {
	res := projectGetItemReceipts([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
}

func TestProjectGetItemReceiptsErrorTypename(t *testing.T) {
	res := projectGetItemReceipts([]byte(`{"data":{"warehouseManagementItemReceipts":{
		"__typename":"WarehouseManagement_ItemReceiptError","message":"not allowed"}}}`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("error typename must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "WarehouseManagement_ItemReceiptError") {
		t.Fatalf("note=%q must carry the error typename", res.Note)
	}
}

// --- proposal (crm-proposal-svc, proposals[] + totalElements) ----------------

func TestProjectProposalsPreservesRows(t *testing.T) {
	res := projectProposals([]byte(`{"totalElements":5,"proposals":[
		{"proposalId":"p1","name":"Acme MSA","status":"SENT","updatedDate":"2026-09-01","totalAmount":1250.5},
		{"id":"p2","title":"Beta SOW","status":"DRAFT","createdDate":"2026-08-15"},
		"not-an-object",
		42
	]}`))
	if res.Entity != "Proposal" {
		t.Fatalf("entity=%q, want Proposal", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved rows", got)
	}
	if got := res.Counts["totalCount"]; got != 5 {
		t.Fatalf("counts.totalCount=%d, want 5 (API totalElements)", got)
	}
	if got := res.Counts["totalElements"]; got != 5 {
		t.Fatalf("counts.totalElements=%d, want 5", got)
	}
	it := res.Items[0]
	if it.ID != "p1" || it.Name != "Acme MSA" || it.Type != "SENT" || it.Date != "2026-09-01" || it.Amount != 1250.5 {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "p2" || it.Name != "Beta SOW" || it.Type != "DRAFT" || it.Date != "2026-08-15" {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectProposalsTotalCountFallsBackToArrayLen(t *testing.T) {
	res := projectProposals([]byte(`{"proposals":[{"id":"p9"}]}`))
	if res.Counts["items"] != 1 || res.Counts["totalCount"] != 1 {
		t.Fatalf("missing totalElements must fall back to array len, got %v", res.Counts)
	}
}

func TestProjectProposalsMalformedStaysUnparsed(t *testing.T) {
	res := projectProposals([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
	if strings.Contains(res.Note, "not json") {
		t.Fatalf("note echoes the raw body: %q", res.Note)
	}
}

func TestProjectProposalsEmpty(t *testing.T) {
	res := projectProposals([]byte(`{"proposals":[],"totalElements":0}`))
	if res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 || len(res.Items) != 0 {
		t.Fatalf("empty response must yield zero counts, got %+v", res)
	}
}

// --- review (mccrmmessaging responses[]) -------------------------------------

func TestProjectFeedbackEngagementPreservesRows(t *testing.T) {
	res := projectFeedbackEngagement([]byte(`{"responses":[
		{"id":"r1","comment":"Great","status":"SUBMITTED","submittedDate":"2026-09-01","rating":5},
		{"responseId":"r2","feedback":"Ok","status":"PENDING","createdDate":"2026-08-30","score":3},
		"not-an-object"
	]}`))
	if res.Entity != "Review" {
		t.Fatalf("entity=%q, want Review", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved rows", got)
	}
	if got := res.Counts["totalCount"]; got != 3 {
		t.Fatalf("counts.totalCount=%d, want 3 (raw element count)", got)
	}
	it := res.Items[0]
	if it.ID != "r1" || it.Name != "Great" || it.Type != "SUBMITTED" || it.Date != "2026-09-01" || it.Amount != 5 {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "r2" || it.Name != "Ok" || it.Type != "PENDING" || it.Amount != 3 {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectFeedbackEngagementMalformedStaysUnparsed(t *testing.T) {
	res := projectFeedbackEngagement([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
}

func TestProjectFeedbackEngagementEmpty(t *testing.T) {
	res := projectFeedbackEngagement([]byte(`{"responses":[]}`))
	if res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 || len(res.Items) != 0 {
		t.Fatalf("empty response must yield zero counts, got %+v", res)
	}
}

// --- folio (universalreportinsights, bare array) ------------------------------

func TestProjectFolioPreservesRows(t *testing.T) {
	res := projectFolio([]byte(`[
		{"id":"f1","title":"BAS summary","status":"READY","generatedDate":"2026-09-01"},
		{"folioId":"f2","name":"Cash runway","subType":"CRB_GROUP","createdDate":"2026-08-28"},
		"not-an-object"
	]`))
	if res.Entity != "Folio" {
		t.Fatalf("entity=%q, want Folio", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved rows", got)
	}
	if got := res.Counts["totalCount"]; got != 3 {
		t.Fatalf("counts.totalCount=%d, want 3 (raw element count)", got)
	}
	it := res.Items[0]
	if it.ID != "f1" || it.Name != "BAS summary" || it.Type != "READY" || it.Date != "2026-09-01" {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "f2" || it.Name != "Cash runway" || it.Type != "CRB_GROUP" || it.Date != "2026-08-28" {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectFolioMalformedStaysUnparsed(t *testing.T) {
	res := projectFolio([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
}

func TestProjectFolioEmptyArray(t *testing.T) {
	res := projectFolio([]byte(`[]`))
	if res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 || len(res.Items) != 0 {
		t.Fatalf("empty array must yield zero counts, got %+v", res)
	}
}

// --- prepaid (cboSchedules GraphQL edges → nodes) -----------------------------

func TestProjectGetSchedulesPreservesNodes(t *testing.T) {
	res := projectGetSchedules([]byte(`{"data":{"cboSchedules":{
		"totalCount":7,
		"edges":[
			{"cursor":"c1","node":{"id":"s1","status":"ACTIVE","amount":1200,"homeAmount":1200,"currency":"AUD","startDate":"2026-07-01","endDate":"2027-06-30","sourceTxnType":"BILL"}},
			{"cursor":"c2","node":{"id":"s2","status":"COMPLETED","amount":80,"startDate":"2026-08-01","endDate":"2026-08-31","sourceTxnType":"CHECK"}},
			{"cursor":"c3"}
		]
	}}}`))
	if res.Entity != "PrepaidSchedule" {
		t.Fatalf("entity=%q, want PrepaidSchedule", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved nodes", got)
	}
	if got := res.Counts["totalCount"]; got != 7 {
		t.Fatalf("counts.totalCount=%d, want 7 (API field)", got)
	}
	it := res.Items[0]
	if it.ID != "s1" || it.Type != "ACTIVE" || it.Amount != 1200 ||
		it.Date != "2026-07-01" || it.DocNumber != "2027-06-30" || it.Name != "BILL" {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "s2" || it.Type != "COMPLETED" || it.Name != "CHECK" || it.DocNumber != "2026-08-31" {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectGetSchedulesMalformedStaysUnparsed(t *testing.T) {
	res := projectGetSchedules([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
}

func TestProjectGetSchedulesErrorsKeepNote(t *testing.T) {
	res := projectGetSchedules([]byte(`{"errors":[{"message":"denied"}],"data":null}`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 {
		t.Fatalf("gql error body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "gql: denied") {
		t.Fatalf("note=%q must carry the gql error message", res.Note)
	}
}

func TestProjectGetSchedulesEmptyConnection(t *testing.T) {
	res := projectGetSchedules([]byte(`{"data":{"cboSchedules":{"edges":[],"totalCount":0}}}`))
	if res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 || len(res.Items) != 0 {
		t.Fatalf("empty connection must yield zero counts, got %+v", res)
	}
}

// --- TPAR (instances report: data.rows → ReportResult.Rows) -------------------

func TestProjectTPARInstancesReturnsRows(t *testing.T) {
	res, rows := projectTPARInstances([]byte(`{
		"status":"COMPLETE",
		"metadata":{"totalRows":2},
		"data":{"rows":[["Acme Pty","ABN 1","100.00"],["Beta Pty","ABN 2","200.00"]],"other":"dropped"}
	}`))
	if res.Entity != "TPAR" {
		t.Fatalf("entity=%q, want TPAR", res.Entity)
	}
	if res.Counts["items"] != 0 || res.Counts["totalRows"] != 2 {
		t.Fatalf("counts = %v, want items=0 totalRows=2", res.Counts)
	}
	if !strings.Contains(res.Note, "totalRows=2") || !strings.Contains(res.Note, "status=COMPLETE") {
		t.Fatalf("note=%q must keep totalRows/status", res.Note)
	}
	if string(rows) != `[["Acme Pty","ABN 1","100.00"],["Beta Pty","ABN 2","200.00"]]` {
		t.Fatalf("rows = %s, want data.rows verbatim", rows)
	}
}

func TestProjectTPARInstancesMalformedStaysUnparsed(t *testing.T) {
	res, rows := projectTPARInstances([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 || rows != nil {
		t.Fatalf("malformed body must yield zero items and no rows, got %+v rows=%s", res, rows)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
}

func TestProjectTPARInstancesEmpty(t *testing.T) {
	res, rows := projectTPARInstances([]byte(`{"status":"COMPLETE","metadata":{"totalRows":0},"data":{"rows":[]}}`))
	if res.Counts["items"] != 0 || res.Counts["totalRows"] != 0 || len(res.Items) != 0 {
		t.Fatalf("empty response must yield zero counts, got %+v", res)
	}
	if string(rows) != `[]` {
		t.Fatalf("rows = %s, want empty data.rows preserved", rows)
	}
}

// TestReplayTPARReportPreservesRows drives the report path through the
// transport seam: data.rows reaches ReportResult.Rows while header,
// totalRows, and status semantics stay put.
func TestReplayTPARReportPreservesRows(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{
		"status":"COMPLETE",
		"authorization":"Bearer tok-tpar","cookie":"sess-tpar",
		"metadata":{"totalRows":1,"token":"sekrit-meta"},
		"data":{"rows":[["Acme Pty","ABN 1","100.00"]],"request":{"headers":{"x-csrf-token":"sekrit-hdr"}}}
	}`)
	interceptHTTP(t, srv.URL)
	res, err := replayTPARReport(context.Background(), "2026-01-01", "2026-09-12")
	if err != nil {
		t.Fatalf("TPAR report: %v", err)
	}
	if res.Status != http.StatusOK || res.Report != "TAXABLE_PAYMENTS" {
		t.Fatalf("envelope = %+v", res)
	}
	if res.Counts["totalRows"] != 1 {
		t.Fatalf("counts = %v, want totalRows=1", res.Counts)
	}
	if res.Header["StartPeriod"] != "2026-01-01" || res.Header["EndPeriod"] != "2026-09-12" {
		t.Fatalf("header = %v, want StartPeriod/EndPeriod", res.Header)
	}
	if string(res.Rows) != `[["Acme Pty","ABN 1","100.00"]]` {
		t.Fatalf("rows = %s, want data.rows verbatim", res.Rows)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, s := range []string{"Bearer", "tok-", "sekrit-", "sess", "authorization", "cookie", "x-csrf-token", "request", "headers"} {
		if strings.Contains(string(raw), s) {
			t.Fatalf("report envelope leaks envelope-level field %q: %s", s, raw)
		}
	}
	calls, meth, path, _ := srv.snap()
	if calls != 1 || meth != http.MethodPost || !strings.Contains(path, "/v1/reports/qbo/") {
		t.Fatalf("call = %d %s %s, want single POST /v1/reports/qbo/", calls, meth, path)
	}
}

// --- no-secret projection across every repaired projector --------------------

// TestReadProjectorsNeverEchoSecrets marshals each repaired projector's
// envelope and asserts credential-shaped and arbitrary nested payload in
// raw rows cannot leak past the QueryItem surface.
func TestReadProjectorsNeverEchoSecrets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		project func([]byte) *QueryResult
		body    string
	}{
		{"contract", projectModifiedEnvelopes, `[{"envelopeId":"e1","authorization":"Bearer tok-contract","cookie":"sess=abc","nested":{"token":"sekrit-contract"}}]`},
		{"customfield", projectCustomFieldsQueryCES, `{"data":{"customFieldDefinitions":{"edges":[{"node":{"id":"cf1","name":"x","authorization":"Bearer tok-cf","accessToken":"sekrit-cf","headers":{"a":"b"}}}]}}}`},
		{"workflow", projectWASAllRules, `{"data":{"workflows":{"definitions":{"edges":[{"node":{"id":"w1","name":"x","authorization":"Bearer tok-wf","sessionToken":"sekrit-wf","cookies":["c=1"]}}]}}}}`},
		{"itemreceipt", projectGetItemReceipts, `{"data":{"warehouseManagementItemReceipts":{"totalCount":1,"nodes":[{"id":"ir1","authorization":"Bearer tok-ir","set-cookie":"sess-ir","private":{"token":"sekrit-ir"}}]}}}`},
		{"proposal", projectProposals, `{"totalElements":1,"proposals":[{"proposalId":"p1","authorization":"Bearer tok-pp","cookie":"sess-pp","customer":{"token":"sekrit-pp"}}]}`},
		{"review", projectFeedbackEngagement, `{"responses":[{"id":"r1","authorization":"Bearer tok-rv","set-cookie":"sess-rv","device":{"token":"sekrit-rv"}}]}`},
		{"folio", projectFolio, `[{"id":"f1","authorization":"Bearer tok-fo","sessionToken":"sekrit-fo","layout":{"a":"b"}}]`},
		{"prepaid", projectGetSchedules, `{"data":{"cboSchedules":{"totalCount":1,"edges":[{"node":{"id":"s1","authorization":"Bearer tok-ps","accessToken":"sekrit-ps","links":{"token":"sekrit-lnk"}}}]}}}`},
	} {
		res := tc.project([]byte(tc.body))
		if len(res.Items) != 1 {
			t.Fatalf("%s: len(items)=%d, want 1", tc.name, len(res.Items))
		}
		raw, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		for _, s := range []string{"Bearer", "tok-", "sekrit-", "sess", "authorization", "cookie", "headers", "sessionToken", "accessToken", "nested", "private"} {
			if strings.Contains(string(raw), s) {
				t.Fatalf("%s: envelope leaks unmapped field %q: %s", tc.name, s, raw)
			}
		}
	}
}

// TestReplayContractProjectsEnvelopeRows drives the repaired projector
// through the full client path on the package-standard transport seam:
// the esign modified-envelopes GET must come back as populated QueryItem
// rows, not a bare count.
func TestReplayContractProjectsEnvelopeRows(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `[{"envelopeId":"env-1","name":"MSA","status":"SIGNED"},{"envelopeId":"env-2"}]`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(context.Background(), "Contract", "", "", 20)
	if err != nil {
		t.Fatalf("Contract replay: %v", err)
	}
	if res.Status != http.StatusOK || res.Entity != "Contract" {
		t.Fatalf("envelope = %+v", res)
	}
	if len(res.Items) != 2 || res.Items[0].ID != "env-1" || res.Items[0].Name != "MSA" {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Counts["items"] != 2 || res.Counts["totalCount"] != 2 {
		t.Fatalf("counts = %v", res.Counts)
	}
	calls, meth, path, _ := srv.snap()
	if calls != 1 || meth != http.MethodGet || !strings.Contains(path, "/v1/modified-envelopes") {
		t.Fatalf("call = %d %s %s, want single GET /v1/modified-envelopes", calls, meth, path)
	}
}
