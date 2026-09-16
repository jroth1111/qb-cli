package client

// accounting_deep_test.go covers accounting_deep.go using the
// package-standard DefaultTransport swap (interceptHTTP): production
// qbo.intuit.com and *.api.intuit.com URLs are routed to local httptest
// servers, nothing reaches the real network.
//
// Adequacy: happy-path tests assert method, endpoint path, request body
// (operation name + variables) and response projection; validation tests
// reject empty ids, empty bulk lists, unbalanced opening-balance lines and
// empty schedule input BEFORE any dial; no-creds tests prove fast auth
// failure. Together they kill: (1) a handler that never sends, (2) one that
// sends to the wrong endpoint/op, (3) one that sends a malformed body,
// (4) one that accepts invalid or unbalanced input, (5) one that dials
// without credentials, and (6) a batch projection that hides faults.
//
// Completed coverage adds: budget list empty envelope + no-creds, journal
// bulk delete abort when the SyncToken fetch fails, class/location v3
// update/delete round-trips (sparse update + operation=delete), deferred
// revenue no-schedule shape, prepaid schedule create entity typing, and
// the opening-balance debit==credit invariant under float accumulation.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// acctDeepServer records requests per host+path so one server can serve
// every accounting-deep endpoint. It is safe for the sequential calls these
// tests make; the mutex guards the recording fields.
type acctDeepServer struct {
	t *testing.T

	mu       sync.Mutex
	lastURL  string
	lastMeth string
	lastBody []byte

	// lastQuery keeps the raw query string (operation=delete, minorversion,
	// …); read it via callsFull() when a test asserts on query params.
	lastQuery string

	status map[string]int    // "METHOD /path" -> status
	bodies map[string]string // "METHOD /path" -> response body
}

func newAcctDeepServer(t *testing.T, status map[string]int, bodies map[string]string) *acctDeepServer {
	return &acctDeepServer{t: t, status: status, bodies: bodies}
}

// lookup resolves a request by METHOD + last path segment against the
// registered keys ("POST /journalentry" matches
// /api/v3/company/{realm}/journalentry). Unknown paths get 404 with an
// empty body, mirroring production's routing failure mode.
func (s *acctDeepServer) lookup(method, path string) (string, int) {
	path = strings.SplitN(path, "?", 2)[0] // drop query string
	segs := strings.Split(strings.Trim(path, "/"), "/")
	// Scan segments right-to-left so the entity segment wins over generic
	// prefixes (api, v3, company, realm): "/api/v3/company/R/journalentry/77"
	// matches "GET /journalentry" (not GET /77), and ".../batch" matches
	// "POST /batch".
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(segs) - 1; i >= 0; i-- {
		if b, ok := s.bodies[method+" /"+segs[i]]; ok {
			return b, http.StatusOK
		}
	}
	return "", http.StatusNotFound
}

func (s *acctDeepServer) handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.lastQuery = r.URL.RawQuery
		s.lastURL = r.URL.Path
		s.lastMeth = r.Method
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		_ = r.Body.Close()
		s.lastBody = b
		s.mu.Unlock()
		body, st := s.lookup(r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_, _ = w.Write([]byte(body))
	}
	// Intercepted requests keep their FULL production path
	// (/api/v3/company/{realm}/journalentry, /graphql, ...), so a single
	// catch-all handler matches by METHOD + trailing segment instead of
	// relying on ServeMux prefix rules.
	mux.HandleFunc("/", handle)
	return mux
}

func (s *acctDeepServer) start(t *testing.T) string {
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

// calls returns the last recorded method/URL/body under the lock.
func (s *acctDeepServer) calls() (string, string, []byte) {
	m, u, _, b := s.callsFull()
	return m, u, b
}

// callsFull also reports the raw query string of the last request.
func (s *acctDeepServer) callsFull() (string, string, string, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastMeth, s.lastURL, s.lastQuery, s.lastBody
}

// gqlEnvelope wraps data as a GraphQL response.
func gqlEnvelope(data string) string {
	return fmt.Sprintf(`{"data":%s}`, data)
}

// --- Budget list/get --------------------------------------------------------

// saveUsableAudit extends saveUsable with the audit-ui Intuit_APIKey that
// postBudgetGraphQL (budgeting.api transport) requires, and swaps the
// budget transport seam to a stdlib POST so the request is routed by the
// DefaultTransport intercept (the surf Chrome client bypasses it).
func saveUsableAudit(t *testing.T) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{
		Authorization:      "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		AuditAuthorization: "Intuit_APIKey intuit_apikey=audit,intuit_apikey_version=1.0",
		RealmID:            "12345",
	}); err != nil {
		t.Fatal(err)
	}
	orig := postBudgetGraphQLFn
	postBudgetGraphQLFn = func(ac *apiClient, ctx context.Context, body []byte) (*http.Response, error) {
		return ac.doStdlibJSON(ctx, http.MethodPost, budgetGraphQLURL, body, "")
	}
	t.Cleanup(func() { postBudgetGraphQLFn = orig })
}

func TestReplayBudgetListPostsCapturedCurrentUIQuery(t *testing.T) {
	saveUsableAudit(t)
	fs := newAcctDeepServer(t, nil, map[string]string{"POST /graphql": gqlEnvelope(`{"businessPlanningBudgets":{"edges":[
  {"node":{"budgetId":"B1","budgetName":"FY27 P&L","budgetType":"PROFIT_AND_LOSS","startDate":"2026-07-01","viewSettings":{"archived":false}}},
  {"node":{"budgetId":9007199254740993,"budgetName":"FY27 BS","budgetType":"BALANCE_SHEET","startDate":"2026-08-01"}}]}}`)})
	interceptHTTP(t, fs.start(t))
	res, err := ReplayBudgetList(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Entity != "Budget" || len(res.Items) != 2 || res.Counts["items"] != 2 {
		t.Fatalf("envelope = %+v", res)
	}
	first := res.Items[0]
	if first.ID != "B1" || first.Name != "FY27 P&L" || first.Type != "PROFIT_AND_LOSS" || first.Date != "2026-07-01" || first.Active == nil || !*first.Active {
		t.Fatalf("item0 = %+v", first)
	}
	second := res.Items[1]
	if second.ID != "9007199254740993" || second.Name != "FY27 BS" || second.Type != "BALANCE_SHEET" || second.Date != "2026-08-01" || second.Active != nil {
		t.Fatalf("numeric ID or missing archived flag misrepresented: %+v", second)
	}
	if !strings.Contains(res.Note, "fetchAllBudgets") {
		t.Fatalf("note = %q", res.Note)
	}
	method, path, body := fs.calls()
	if method != http.MethodPost || !strings.HasSuffix(path, "/graphql") {
		t.Fatalf("call = %s %s", method, path)
	}
	var sent struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
		Query         string         `json:"query"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.OperationName != "fetchAllBudgets" {
		t.Fatalf("operation = %q", sent.OperationName)
	}
	var wantVariables map[string]any
	if err := json.Unmarshal([]byte(`{"last":null,"first":2,"after":null,"before":null,"sortBy":"LAST_MODIFIED","sortDirection":"DESC","budgetFilters":{"archived":false,"budgetType":["PROFIT_AND_LOSS","BALANCE_SHEET"]}}`), &wantVariables); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sent.Variables, wantVariables) {
		t.Fatalf("variables = %#v; want %#v", sent.Variables, wantVariables)
	}
	// Pin the independently captured document, not the implementation constant.
	const wantQuery = `query fetchAllBudgets($first: Int, $after: String, $last: Int, $before: String, $sortBy: BusinessPlanning_SortByColumn, $sortDirection: BusinessPlanning_SortDirection, $budgetFilters: BusinessPlanning_BudgetFilters) {
  businessPlanningBudgets(first: $first, after: $after, last: $last, before: $before, sortBy: $sortBy, sortDirection: $sortDirection, budgetFilters: $budgetFilters) {
    pageInfo {
      hasPreviousPage
      hasNextPage
      startCursor
      endCursor
      __typename
    }
    edges {
      cursor
      node {
        budgetId
        budgetName
        budgetType
        startDate
        endDate
        linkedEntityId
        intervalType
        syncToken
        secondaryListType
        dimensionDefId
        budgetMetaData {
          createdBy
          createdAt
          lastUpdatedBy
          updatedAt
          __typename
        }
        viewSettings {
          archived
          __typename
        }
        approvalDetails {
          approvalStatus
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}
`
	if sent.Query != wantQuery {
		t.Fatalf("query differs from current UI capture:\n%s", sent.Query)
	}
}

func TestReplayBudgetGetUsesNativeQuery(t *testing.T) {
	saveUsable(t)
	original := postBudgetGraphQLFn
	postBudgetGraphQLFn = func(*apiClient, context.Context, []byte) (*http.Response, error) {
		t.Error("budget get must not use budgeting GraphQL")
		return nil, errors.New("unexpected budget GraphQL call")
	}
	t.Cleanup(func() { postBudgetGraphQLFn = original })
	fs := newAcctDeepServer(t, nil, map[string]string{"GET /query": `{"QueryResponse":{"Budget":[{"Id":"B9","Name":"Test budget"}]}}`})
	interceptHTTP(t, fs.start(t))
	res, err := ReplayBudgetGet(context.Background(), "B9")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Entity != "Budget" || len(res.Items) != 1 || res.Items[0].ID != "B9" || res.Items[0].Name != "Test budget" {
		t.Fatalf("envelope = %+v", res)
	}
	method, path, query, body := fs.callsFull()
	if method != http.MethodGet || !strings.HasSuffix(path, "/query") || len(body) != 0 {
		t.Fatalf("call = %s %s %s", method, path, body)
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	statement := values.Get("query")
	if !strings.Contains(statement, "from Budget") || !strings.Contains(statement, "Id = 'B9'") || !strings.Contains(statement, "maxresults 20") {
		t.Fatalf("native query = %q", statement)
	}
}

func TestReplayBudgetListEmptyEnvelope(t *testing.T) {
	saveUsableAudit(t)
	fs := newAcctDeepServer(t, nil, map[string]string{"POST /graphql": gqlEnvelope(`{"businessPlanningBudgets":{"edges":[]}}`)})
	interceptHTTP(t, fs.start(t))
	res, err := ReplayBudgetList(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Entity != "Budget" || res.Items == nil || len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("explicit empty collection must project to zero items: %+v", res)
	}
}

func TestReplayBudgetListLimitClamps(t *testing.T) {
	saveUsableAudit(t)
	fs := newAcctDeepServer(t, nil, map[string]string{"POST /graphql": gqlEnvelope(`{"businessPlanningBudgets":{"edges":[]}}`)})
	interceptHTTP(t, fs.start(t))
	for _, tc := range []struct{ input, want int }{{0, 20}, {-5, 20}, {1000, 100}, {5, 5}} {
		if _, err := ReplayBudgetList(context.Background(), tc.input); err != nil {
			t.Fatalf("limit %d: %v", tc.input, err)
		}
		_, _, body := fs.calls()
		var sent struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatal(err)
		}
		if sent.Variables["first"] != float64(tc.want) {
			t.Fatalf("limit %d sent first=%v, want %d", tc.input, sent.Variables["first"], tc.want)
		}
	}
}

func TestReplayBudgetListBoundsOversizedResponse(t *testing.T) {
	saveUsableAudit(t)
	fs := newAcctDeepServer(t, nil, map[string]string{"POST /graphql": gqlEnvelope(`{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":"B1"}},{"node":{"budgetId":"B2"}}]}}`)})
	interceptHTTP(t, fs.start(t))
	res, err := ReplayBudgetList(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "B1" || res.Counts["items"] != 1 {
		t.Fatalf("limit not honored: %+v", res)
	}
}

func TestReplayBudgetListRejectsInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		message string
	}{
		{"http error", 500, `{"error":"upstream failed"}`, ""},
		{"http error with data", 403, `{"data":{"businessPlanningBudgets":{"edges":[]}}}`, ""},
		{"invalid json", 200, `{`, ""},
		{"trailing json", 200, `{} {}`, ""},
		{"html", 200, `<html>Sign in</html>`, ""},
		{"graphql unknown type", 200, `{"errors":[{"message":"UnknownType DataAccess_BudgetFilter"}]}`, "UnknownType"},
		{"graphql partial data", 200, `{"data":{"businessPlanningBudgets":{"edges":[]}},"errors":[{"message":"resolver failed"}]}`, "resolver failed"},
		{"graphql empty error", 200, `{"data":{"businessPlanningBudgets":{"edges":[]}},"errors":[{}]}`, "GraphQL errors"},
		{"graphql null error", 200, `{"data":{"businessPlanningBudgets":{"edges":[]}},"errors":[null]}`, "GraphQL errors"},
		{"malformed errors", 200, `{"data":{"businessPlanningBudgets":{"edges":[]}},"errors":{}}`, ""},
		{"null envelope", 200, `null`, ""},
		{"missing data", 200, `{}`, ""},
		{"null data", 200, `{"data":null}`, ""},
		{"array data", 200, `{"data":[]}`, ""},
		{"missing collection", 200, `{"data":{}}`, ""},
		{"obsolete collection", 200, `{"data":{"dataAccessBudgets":{"edges":[]}}}`, ""},
		{"null collection", 200, `{"data":{"businessPlanningBudgets":null}}`, ""},
		{"malformed collection", 200, `{"data":{"businessPlanningBudgets":[]}}`, ""},
		{"missing edges", 200, `{"data":{"businessPlanningBudgets":{}}}`, ""},
		{"null edges", 200, `{"data":{"businessPlanningBudgets":{"edges":null}}}`, ""},
		{"malformed edges", 200, `{"data":{"businessPlanningBudgets":{"edges":{}}}}`, ""},
		{"missing node", 200, `{"data":{"businessPlanningBudgets":{"edges":[{}]}}}`, ""},
		{"null node", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":null}]}}}`, ""},
		{"missing identity", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{}}]}}}`, ""},
		{"null identity", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":null}}]}}}`, ""},
		{"empty identity", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":" "}}]}}}`, ""},
		{"object identity", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":{}}}]}}}`, ""},
		{"invalid name", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":"B1","budgetName":[]}}]}}}`, ""},
		{"invalid date", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":"B1","startDate":{}}}]}}}`, ""},
		{"invalid archived flag", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":"B1","viewSettings":{"archived":"false"}}}]}}}`, ""},
		{"invalid edge past limit", 200, `{"data":{"businessPlanningBudgets":{"edges":[{"node":{"budgetId":"B1"}},{"node":null}]}}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveUsableAudit(t)
			postBudgetGraphQLFn = func(*apiClient, context.Context, []byte) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			}
			res, err := ReplayBudgetList(context.Background(), 1)
			if err == nil || res != nil {
				t.Fatalf("invalid response returned success: result=%+v err=%v", res, err)
			}
			if tc.message != "" && !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error = %v, want %q", err, tc.message)
			}
			if tc.status != 200 {
				var replay *ReplayError
				if !errors.As(err, &replay) || replay.Status != tc.status {
					t.Fatalf("HTTP status lost: %v", err)
				}
			}
		})
	}
}

func TestReplayBudgetListPropagatesTransportFailure(t *testing.T) {
	saveUsableAudit(t)
	failure := errors.New("budget transport failed")
	postBudgetGraphQLFn = func(*apiClient, context.Context, []byte) (*http.Response, error) { return nil, failure }
	res, err := ReplayBudgetList(context.Background(), 20)
	if res != nil || !errors.Is(err, failure) {
		t.Fatalf("result=%+v error=%v", res, err)
	}
}

func TestReplayBudgetGetRejectsEmptyID(t *testing.T) {
	if _, err := ReplayBudgetGet(context.Background(), "  "); !errors.Is(err, ErrEmptyBudgetID) {
		t.Fatalf("err=%v, want ErrEmptyBudgetID", err)
	}
}

// --- Class / Location dimension reads ---------------------------------------

func TestReplayDimensionListClassAndLocation(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /graphql": http.StatusOK},
		map[string]string{"POST /graphql": gqlEnvelope(`{"dataAccessKlasses":{"edges":[
			{"node":{"id":"K1","name":"Sales","fullName":"Sales","active":true,"level":1}}],"totalCount":4}}`)},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDimensionList(context.Background(), "class", 20)
	if err != nil {
		t.Fatalf("class list: %v", err)
	}
	if res.Entity != "Class" || len(res.Items) != 1 || res.Items[0].ID != "K1" || res.Items[0].Name != "Sales" {
		t.Fatalf("envelope = %+v", res)
	}
	if res.Counts["totalCount"] != 4 {
		t.Fatalf("counts = %v", res.Counts)
	}
	active := res.Items[0].Active
	if active == nil || !*active {
		t.Fatalf("active flag not projected: %+v", res.Items[0])
	}
	_, _, body := fs.calls()
	var sent struct {
		OperationName string `json:"operationName"`
		Query         string `json:"query"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("body = %s", body)
	}
	if sent.OperationName != "GetKlasses" {
		t.Fatalf("operationName = %q", sent.OperationName)
	}

	// Same server serves departments: root field switches with dimension.
	fs.bodies["POST /graphql"] = gqlEnvelope(`{"dataAccessDepartments":{"edges":[
		{"node":{"id":"D2","name":"North","fullName":"North","active":false,"level":1}}],"totalCount":7}}`)
	res, err = ReplayDimensionList(context.Background(), "location", 10)
	if err != nil {
		t.Fatalf("location list: %v", err)
	}
	if res.Entity != "Location" || len(res.Items) != 1 || res.Items[0].ID != "D2" {
		t.Fatalf("envelope = %+v", res)
	}
	inactive := res.Items[0].Active
	if inactive == nil || *inactive {
		t.Fatalf("active=false must be distinguishable from unset: %+v", res.Items[0])
	}
	if !strings.Contains(res.Note, "GetDepartments") || !strings.Contains(res.Note, "totalCount=7") {
		t.Fatalf("note = %q", res.Note)
	}
}

func TestReplayDimensionListRejectsUnknownDimension(t *testing.T) {
	noCredsDir(t) // also proves validation precedes credential loading
	if _, err := ReplayDimensionList(context.Background(), "warehouse", 5); err == nil ||
		strings.Contains(err.Error(), "credentials") {
		t.Fatalf("err = %v, want unknown-dimension rejection before creds", err)
	}
}

// --- Class / Location v3 update + delete (cli deep wiring) --------------------

// TestClassLocationMutateUpdateFetchesThenPatches pins the class/location
// update round-trip the CLI wiring uses (ReplayMutate on "Class"/"Department"):
// GET the row for its SyncToken, then POST a sparse update carrying that
// token plus only the changed fields.
func TestClassLocationMutateUpdateFetchesThenPatches(t *testing.T) {
	saveUsable(t)
	for _, tc := range []struct {
		entity string // ReplayMutate entity
		path   string // v3 path segment served by the fake
		id     string
		name   string
	}{
		{"Class", "class", "K1", "Sales Renamed"},
		{"Department", "department", "D2", "North Renamed"},
	} {
		fs := newAcctDeepServer(t,
			map[string]int{
				"GET /" + tc.path:  http.StatusOK,
				"POST /" + tc.path: http.StatusOK,
			},
			map[string]string{
				"GET /" + tc.path:  fmt.Sprintf(`{%q:{"Id":%q,"SyncToken":"4","Name":"Old","Active":true}}`, tc.entity, tc.id),
				"POST /" + tc.path: fmt.Sprintf(`{%q:{"Id":%q,"SyncToken":"5","Name":%q,"Active":true}}`, tc.entity, tc.id, tc.name),
			},
		)
		interceptHTTP(t, fs.start(t))

		res, err := ReplayMutate(context.Background(), tc.entity, "update", "", map[string]string{"id": tc.id, "name": tc.name})
		if err != nil {
			t.Fatalf("%s update: %v", tc.entity, err)
		}
		if res.Op != "update" || res.Entity != tc.entity || res.Item.ID != tc.id || res.Item.Name != tc.name {
			t.Fatalf("%s envelope = %+v", tc.entity, res)
		}
		method, url, query, body := fs.callsFull()
		if method != http.MethodPost || !strings.Contains(url, "/api/v3/company/") || !strings.Contains(url, "/"+tc.path) {
			t.Fatalf("%s last call = %s %s?%s", tc.entity, method, url, query)
		}
		if !strings.Contains(query, "minorversion=73") {
			t.Fatalf("%s url missing minorversion: %s?%s", tc.entity, url, query)
		}
		var sent map[string]any
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatalf("%s body = %s", tc.entity, body)
		}
		if sent["Id"] != tc.id || sent["SyncToken"] != "4" {
			t.Fatalf("%s sparse body lost fetched identity/SyncToken: %v", tc.entity, sent)
		}
		if sent["sparse"] != true {
			t.Fatalf("%s update must be sparse: %v", tc.entity, sent)
		}
		if sent["Name"] != tc.name {
			t.Fatalf("%s rename not applied: %v", tc.entity, sent)
		}
	}
}

// TestClassLocationMutateDeleteSendsOperationAndSyncToken pins the delete
// contract for class/location: fetch first, then POST Id+SyncToken with
// operation=delete on the query string.
func TestClassLocationMutateDeleteSendsOperationAndSyncToken(t *testing.T) {
	saveUsable(t)
	for _, tc := range []struct {
		entity, path, id string
	}{
		{"Class", "class", "K9"},
		{"Department", "department", "D8"},
	} {
		fs := newAcctDeepServer(t,
			map[string]int{
				"GET /" + tc.path:  http.StatusOK,
				"POST /" + tc.path: http.StatusOK,
			},
			map[string]string{
				"GET /" + tc.path:  fmt.Sprintf(`{%q:{"Id":%q,"SyncToken":"7","Name":"Gone"}}`, tc.entity, tc.id),
				"POST /" + tc.path: `{}`,
			},
		)
		interceptHTTP(t, fs.start(t))

		res, err := ReplayMutate(context.Background(), tc.entity, "delete", tc.id, nil)
		if err != nil {
			t.Fatalf("%s delete: %v", tc.entity, err)
		}
		if res.Op != "delete" || res.Entity != tc.entity {
			t.Fatalf("%s envelope = %+v", tc.entity, res)
		}
		method, url, query, body := fs.callsFull()
		if method != http.MethodPost {
			t.Fatalf("%s last call = %s", tc.entity, method)
		}
		if !strings.Contains(query, "operation=delete") {
			t.Fatalf("%s url missing operation=delete: %s?%s", tc.entity, url, query)
		}
		if !strings.Contains(query, "minorversion=73") {
			t.Fatalf("%s url missing minorversion: %s?%s", tc.entity, url, query)
		}
		var sent map[string]any
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatalf("%s body = %s", tc.entity, body)
		}
		if sent["Id"] != tc.id || sent["SyncToken"] != "7" {
			t.Fatalf("%s delete body must carry fetched Id+SyncToken: %v", tc.entity, sent)
		}
		if len(sent) != 2 {
			t.Fatalf("%s delete body must be minimal {Id,SyncToken}: %v", tc.entity, sent)
		}
	}
}

// TestClassLocationMutateRejectsMissingID proves both verbs fail on a blank
// id with ErrMissingMutateID (the CLI maps this to exitInput before any
// write path runs).
func TestClassLocationMutateRejectsMissingID(t *testing.T) {
	saveUsable(t)
	for _, entity := range []string{"Class", "Department"} {
		if _, err := ReplayMutate(context.Background(), entity, "update", "", nil); !errors.Is(err, ErrMissingMutateID) {
			t.Errorf("%s update: err = %v, want ErrMissingMutateID", entity, err)
		}
		if _, err := ReplayMutate(context.Background(), entity, "delete", "", nil); !errors.Is(err, ErrMissingMutateID) {
			t.Errorf("%s delete: err = %v, want ErrMissingMutateID", entity, err)
		}
	}
}

// --- Journal bulk create/delete ----------------------------------------------

func TestReplayJournalBulkCreatePostsBatchRequest(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /batch": http.StatusOK},
		map[string]string{"POST /batch": `{"BatchItemResponse":[
			{"bId":"0","JournalEntry":{"Id":"JE-1","TxnDate":"2026-08-25"}},
			{"bId":"1","fault":{"type":"ValidationFault"}}]}`},
	)
	interceptHTTP(t, fs.start(t))

	items := []JournalBulkItem{
		{Amount: 100, FromAccount: "84", ToAccount: "7", Memo: "bulk one"},
		{Date: "25/08/2026", Amount: 55.5, FromAccount: "204", ToAccount: "93"},
	}
	res, err := ReplayJournalBulkCreate(context.Background(), items)
	if err != nil {
		t.Fatalf("ReplayJournalBulkCreate: %v", err)
	}
	if res.Entity != "JournalEntry" || res.Status != http.StatusOK {
		t.Fatalf("envelope = %+v", res)
	}
	if len(res.Items) != 2 || res.Items[0].ID != "JE-1" {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Items[1].ID != "1" || !strings.HasPrefix(res.Items[1].Name, "fault:") {
		t.Fatalf("fault row mis-projected: %+v", res.Items[1])
	}
	if res.Counts["faults"] != 1 {
		t.Fatalf("counts = %v (fault must be counted, not hidden)", res.Counts)
	}
	method, url, body := fs.calls()
	if method != http.MethodPost || !strings.HasSuffix(url, "/batch") {
		t.Fatalf("call = %s %s", method, url)
	}
	var sent struct {
		BatchItemRequest []struct {
			BID          string `json:"bId"`
			Operation    string `json:"operation"`
			JournalEntry struct {
				TxnDate string           `json:"TxnDate"`
				Line    []map[string]any `json:"Line"`
				Priv    string           `json:"PrivateNote"`
			} `json:"JournalEntry"`
		} `json:"BatchItemRequest"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("body = %s", body)
	}
	if len(sent.BatchItemRequest) != 2 {
		t.Fatalf("batch rows = %d, want 2", len(sent.BatchItemRequest))
	}
	row0 := sent.BatchItemRequest[0]
	if row0.Operation != "create" || len(row0.JournalEntry.Line) != 2 {
		t.Fatalf("row0 = %+v", row0)
	}
	if row0.JournalEntry.Priv != "bulk one" {
		t.Fatalf("memo lost: %+v", row0.JournalEntry)
	}
	// dd/MM/yyyy normalisation on the second item's date.
	if got := sent.BatchItemRequest[1].JournalEntry.TxnDate; got != "2026-08-25" {
		t.Fatalf("row1 date = %q, want 2026-08-25", got)
	}
}

// TestReplayJournalBulkDeleteAbortsWhenFetchFails pins the delete contract:
// if any SyncToken prefetch fails, the batch must NOT be posted (a v3
// delete without a fresh SyncToken is a guaranteed fault, so half-sending
// the batch would be worse than not sending it).
func TestReplayJournalBulkDeleteAbortsWhenFetchFails(t *testing.T) {
	saveUsable(t)
	// GET /journalentry returns 404: the prefetch fails.
	fs := newAcctDeepServer(t,
		map[string]int{"GET /journalentry": http.StatusNotFound},
		map[string]string{},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayJournalBulkDelete(context.Background(), []string{"77", "78"})
	if err == nil {
		t.Fatalf("expected prefetch failure to surface, got %+v", res)
	}
	if !strings.Contains(err.Error(), "journal bulk delete item") {
		t.Fatalf("err = %v, want item-scoped prefetch failure", err)
	}
	method, url, _ := fs.calls()
	if method != http.MethodGet || !strings.Contains(url, "/journalentry/") {
		t.Fatalf("last call = %s %s, want the failing prefetch only (no batch POST)", method, url)
	}
}

// TestReplayJournalBulkDeleteRejectsEmptyIDs pins the empty-ids sentinel
// firing before any dial.
func TestReplayJournalBulkDeleteRejectsEmptyIDs(t *testing.T) {
	noCredsDir(t) // validation must precede credential loading too
	start := time.Now()
	if _, err := ReplayJournalBulkDelete(context.Background(), []string{" ", ""}); !errors.Is(err, ErrEmptyJournalIDs) {
		t.Fatalf("err = %v, want ErrEmptyJournalIDs", err)
	}
	assertFast(t, time.Since(start))
}

func TestReplayJournalBulkDeleteFetchesSyncTokensFirst(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{
			"GET /journalentry": http.StatusOK,
			"POST /batch":       http.StatusOK,
		},
		map[string]string{
			"GET /journalentry": `{"JournalEntry":{"Id":"77","SyncToken":"3"}}`,
			"POST /batch":       `{"BatchItemResponse":[{"bId":"0","JournalEntry":{"Id":"77"}}]}`,
		},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayJournalBulkDelete(context.Background(), []string{" 77 ", ""})
	if err != nil {
		t.Fatalf("ReplayJournalBulkDelete: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %+v", res.Items)
	}
	method, _, _ := fs.calls()
	if method != http.MethodPost {
		t.Fatalf("last call = %s, batch POST expected last", method)
	}
	// The delete body must carry the fetched SyncToken.
	_, _, body := fs.calls()
	if !strings.Contains(string(body), `"SyncToken":"3"`) || !strings.Contains(string(body), `"operation":"delete"`) {
		t.Fatalf("delete body missing fetched SyncToken/operation: %s", body)
	}
}

func TestParseJournalBulkItemsRejectsBadItems(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"not json", `{amount`},
		{"empty array", `[]`},
		{"zero amount", `[{"amount":0,"from-account":"84","to-account":"7"}]`},
		{"missing account", `[{"amount":5,"to-account":"7"}]`},
		{"same account both sides", `[{"amount":5,"from-account":"7","to-account":"7"}]`},
	}
	for _, tc := range cases {
		if _, err := ParseJournalBulkItems(tc.raw); err == nil {
			t.Errorf("%s: accepted (%s)", tc.name, tc.raw)
		}
	}
}

// --- Opening balance ----------------------------------------------------------

func TestReplayOpeningBalanceCreateBooksBalancedJournal(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /journalentry": http.StatusOK},
		map[string]string{"POST /journalentry": `{"JournalEntry":{"Id":"OB-1","SyncToken":"0","TxnDate":"2026-07-01"}}`},
	)
	interceptHTTP(t, fs.start(t))

	lines := []TrialBalanceLine{
		{Account: "84", Debit: 1000},
		{Account: "7", Credit: 400},
		{Account: "3", Credit: 600},
	}
	res, err := ReplayOpeningBalanceCreate(context.Background(), "01/07/2026", lines)
	if err != nil {
		t.Fatalf("ReplayOpeningBalanceCreate: %v", err)
	}
	if res.Op != "create" || res.Entity != "JournalEntry" || res.Item.ID != "OB-1" {
		t.Fatalf("envelope = %+v", res)
	}
	_, _, body := fs.calls()
	var sent struct {
		TxnDate string           `json:"TxnDate"`
		Line    []map[string]any `json:"Line"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("body = %s", body)
	}
	if sent.TxnDate != "2026-07-01" {
		t.Fatalf("TxnDate = %q", sent.TxnDate)
	}
	var debits, credits float64
	for _, l := range sent.Line {
		detail := l["JournalEntryLineDetail"].(map[string]any)
		amt := l["Amount"].(float64)
		switch detail["PostingType"] {
		case "Debit":
			debits += amt
		case "Credit":
			credits += amt
		default:
			t.Fatalf("posting type %v", detail["PostingType"])
		}
	}
	if debits != 1000 || credits != 1000 {
		t.Fatalf("booked debits=%v credits=%v, want 1000/1000", debits, credits)
	}
}

func TestParseOpeningBalanceLinesRejectsUnbalanced(t *testing.T) {
	if _, err := ParseOpeningBalanceLines(`[{"account":"84","debit":100}]`); !errors.Is(err, ErrUnbalancedLines) {
		t.Fatalf("unbalanced single side: err = %v", err)
	}
	if _, err := ParseOpeningBalanceLines(`[{"account":"84","debit":100},{"account":"7","credit":90}]`); !errors.Is(err, ErrUnbalancedLines) {
		t.Fatalf("unbalanced totals: err = %v", err)
	}
	if _, err := ParseOpeningBalanceLines(`[{"account":"84","debit":50,"credit":50}]`); err == nil {
		t.Fatal("both-sides line accepted")
	}
	if _, err := ParseOpeningBalanceLines(`[{"credit":30}]`); err == nil {
		t.Fatal("missing account accepted")
	}
	if _, err := ParseOpeningBalanceLines(`[]`); err == nil {
		t.Fatal("empty lines accepted")
	}
	balanced := `[{"account":"84","debit":100},{"account":"7","credit":100}]`
	if _, err := ParseOpeningBalanceLines(balanced); err != nil {
		t.Fatalf("balanced rejected: %v", err)
	}
}

// --- Deferred revenue / prepaid schedules -------------------------------------

func TestReplayDeferredScheduleGetProjectsSchedule(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /graphql": http.StatusOK},
		map[string]string{"POST /graphql": gqlEnvelope(`{"accountingDeferredRecognitionSchedulesBySourceEntity":[{
			"id":"S-1","deferralType":"Prepaid","postingStatus":"POSTED","serviceStartDate":"2026-07-01",
			"recognitionFrequency":"MONTHLY",
			"remainingAmount":{"value":88.5,"currency":"AUD"},
			"scheduleLines":[{"sequence":1},{"sequence":2}]}]}`)},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDeferredScheduleGet(context.Background(), "TX-42", true)
	if err != nil {
		t.Fatalf("ReplayDeferredScheduleGet: %v", err)
	}
	if res.Entity != "PrepaidSchedule" || len(res.Items) != 1 {
		t.Fatalf("envelope = %+v", res)
	}
	it := res.Items[0]
	if it.ID != "S-1" || it.Name != "Prepaid" || it.Amount != 88.5 || it.Account != "AUD" {
		t.Fatalf("item = %+v", it)
	}
	if res.Counts["scheduleLines"] != 2 {
		t.Fatalf("counts = %v", res.Counts)
	}
	_, _, body := fs.calls()
	var sent struct {
		OperationName string `json:"operationName"`
		Variables     struct {
			ID string `json:"id"`
		} `json:"variables"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("body = %s", body)
	}
	if sent.OperationName != "GET_SCHEDULE" || sent.Variables.ID != "TX-42" {
		t.Fatalf("sent = %+v", sent)
	}
	if !strings.Contains(sent.Query, "fragment scheduleDetails on Accounting_DeferredRecognitionSchedule") {
		t.Fatal("get must inline the captured scheduleDetails fragment")
	}
}

// TestReplayDeferredScheduleGetNoSchedule pins the null-schedule shape: a
// source transaction with no schedule must return a 200 DeferredRevenue-
// Schedule envelope with zero items (never an error, never a synthetic row),
// with the prepaid=false entity name.
func TestReplayDeferredScheduleGetNoSchedule(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /graphql": http.StatusOK},
		map[string]string{"POST /graphql": gqlEnvelope(`{"accountingDeferredRecognitionSchedulesBySourceEntity":null}`)},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDeferredScheduleGet(context.Background(), "TX-7", false)
	if err != nil {
		t.Fatalf("ReplayDeferredScheduleGet: %v", err)
	}
	if res.Status != http.StatusOK || res.Entity != "DeferredRevenueSchedule" {
		t.Fatalf("envelope = %+v", res)
	}
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("no-schedule must project to zero items: %+v", res)
	}
	if !strings.Contains(res.Note, "no schedule for source entity") {
		t.Fatalf("note = %q", res.Note)
	}
}

// TestParseOpeningBalanceLinesFloatAccumulation pins the debit==credit
// invariant's float behaviour: 0.1 summed ten times is
// 0.9999999999999999 in float64, so a genuinely balanced trial balance of
// ten 10-cent debits against one $1 credit is (correctly, per the strict
// contract) rejected — the error must wrap ErrUnbalancedLines and name the
// mismatched totals so the operator can fix the lines. A validator that
// silently accepted drifted sums would book unbalanced journals.
func TestParseOpeningBalanceLinesFloatAccumulation(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[`)
	for i := range 10 {
		fmt.Fprintf(&b, `{"account":"A%d","debit":0.1},`, i)
	}
	fmt.Fprintf(&b, `{"account":"B0","credit":1.0}]`)
	_, err := ParseOpeningBalanceLines(b.String())
	if !errors.Is(err, ErrUnbalancedLines) {
		t.Fatalf("drifted sums: err = %v, want ErrUnbalancedLines", err)
	}
	if !strings.Contains(err.Error(), "debits 1.00 != credits 1.00") {
		t.Fatalf("error must name both totals: %v", err)
	}

	// Same drift on both sides balances exactly and must be accepted:
	// eleven 0.1 debits vs eleven 0.1 credits.
	b.Reset()
	b.WriteString(`[`)
	for i := range 11 {
		fmt.Fprintf(&b, `{"account":"A%d","debit":0.1},`, i)
	}
	for i := range 10 {
		fmt.Fprintf(&b, `{"account":"B%d","credit":0.1},`, i)
	}
	b.WriteString(`{"account":"B10","credit":0.1}]`)
	lines, err := ParseOpeningBalanceLines(b.String())
	if err != nil {
		t.Fatalf("identically-accumulated sides rejected: %v", err)
	}
	if len(lines) != 22 {
		t.Fatalf("lines = %d, want 22", len(lines))
	}
}

// TestReplayDeferredScheduleCreatePrepaidEntityAndPassthrough covers the
// prepaid create path: success (no gql errors) must yield a MutateResult
// typed PrepaidSchedule/op=create, and the caller-supplied --data input
// must ride through to variables.input verbatim — nothing invented.
func TestReplayDeferredScheduleCreatePrepaidEntityAndPassthrough(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /graphql": http.StatusOK},
		map[string]string{"POST /graphql": gqlEnvelope(`{"accountingUpdateDeferredRecognitionSchedule":{"clientMutationId":"m1"}}`)},
	)
	interceptHTTP(t, fs.start(t))

	in := map[string]any{"sourceEntityId": "TX-99", "schedule": map[string]any{"recognitionFrequency": "MONTHLY"}}
	res, err := ReplayDeferredScheduleCreate(context.Background(), in, true)
	if err != nil {
		t.Fatalf("ReplayDeferredScheduleCreate: %v", err)
	}
	if res.Op != "create" || res.Entity != "PrepaidSchedule" || res.Item.Type != "PrepaidSchedule" {
		t.Fatalf("envelope = %+v", res)
	}
	if !strings.Contains(res.Note, "UpdateDeferredRecognitionScheduleOp") {
		t.Fatalf("note = %q", res.Note)
	}
	method, url, body := fs.calls()
	if method != http.MethodPost || !strings.HasSuffix(url, "/graphql") {
		t.Fatalf("call = %s %s", method, url)
	}
	var sent struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
		Query         string         `json:"query"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("body = %s", body)
	}
	if sent.OperationName != "UpdateDeferredRecognitionScheduleOp" {
		t.Fatalf("operationName = %q", sent.OperationName)
	}
	gotIn, ok := sent.Variables["input"].(map[string]any)
	if !ok || gotIn["sourceEntityId"] != "TX-99" {
		t.Fatalf("input not passed through verbatim: %+v", sent.Variables)
	}
	if _, ok := gotIn["schedule"].(map[string]any); !ok {
		t.Fatalf("nested schedule input lost: %+v", gotIn)
	}
	if !strings.Contains(sent.Query, "mutation UpdateDeferredRecognitionScheduleOp") {
		t.Fatalf("query = %q", sent.Query)
	}
}

func TestReplayDeferredScheduleCreateRequiresInputAndReportsGqlErrors(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /graphql": http.StatusOK},
		map[string]string{"POST /graphql": `{"errors":[{"message":"Invalid input"}],"data":null}`},
	)
	interceptHTTP(t, fs.start(t))

	if _, err := ReplayDeferredScheduleCreate(context.Background(), map[string]any{}, false); !errors.Is(err, ErrEmptyScheduleInput) {
		t.Fatalf("empty input: err = %v", err)
	}
	_, err := ReplayDeferredScheduleCreate(context.Background(), map[string]any{"schedule": map[string]any{}}, false)
	if err == nil || !strings.Contains(err.Error(), "gql: Invalid input") {
		t.Fatalf("gql error must surface: %v", err)
	}
	_, _, body := fs.calls()
	if !strings.Contains(string(body), "UpdateDeferredRecognitionScheduleOp") {
		t.Fatalf("mutation not posted: %s", body)
	}
}

func TestReplayDeferredSchedulePreviewRejectsEmptyInput(t *testing.T) {
	if _, err := ReplayDeferredSchedulePreview(context.Background(), map[string]any{}, false); !errors.Is(err, ErrEmptyScheduleInput) {
		t.Fatalf("err = %v, want ErrEmptyScheduleInput", err)
	}
}

func TestParseScheduleInputRejectsNonObjectAndEmpty(t *testing.T) {
	if _, err := ParseScheduleInput(`[]`); err == nil {
		t.Fatal("array accepted")
	}
	if _, err := ParseScheduleInput(`{}`); !errors.Is(err, ErrEmptyScheduleInput) {
		t.Fatalf("empty object: err = %v", err)
	}
	if _, err := ParseScheduleInput(`{"a":1}`); err != nil {
		t.Fatalf("valid object rejected: %v", err)
	}
}

// --- Transport fallback seam ---------------------------------------------------

// TestPostAccountingGraphQLFallsBackOn403 pins the 403→fallback ladder:
// when the ATS-header POST draws a 403 the request is retried through the
// uri-host transport, which succeeds here.
func TestPostAccountingGraphQLFallsBackOn403(t *testing.T) {
	saveUsable(t)
	fs := newAcctDeepServer(t,
		map[string]int{"POST /graphql": http.StatusOK},
		map[string]string{"POST /graphql": gqlEnvelope(`{"ok":true}`)},
	)
	interceptHTTP(t, fs.start(t))
	origFallback := accountingURIFallback
	accountingURIFallback = func(ac *apiClient, ctx context.Context, method, rawURL string, body []byte) (*http.Response, error) {
		return ac.doStdlibJSON(ctx, method, rawURL, body, "")
	}
	t.Cleanup(func() { accountingURIFallback = origFallback })

	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ac.postAccountingGraphQL(context.Background(), coaCoreGraphQLURL, dimensionsReferer, []byte(`{}`))
	if err != nil {
		t.Fatalf("postAccountingGraphQL: %v", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, _ := readBody(resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"ok":true`) {
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
}

// --- No credentials: fast-fail before any dial ---------------------------------

func TestAccountingDeepNoCredsFastFail(t *testing.T) {
	noCredsDir(t)
	ctx := context.Background()
	tests := []struct {
		name string
		fn   func() error
	}{
		{"budget list", func() error { _, err := ReplayBudgetList(ctx, 5); return err }},
		{"budget get", func() error { _, err := ReplayBudgetGet(ctx, "B1"); return err }},
		{"class list", func() error { _, err := ReplayDimensionList(ctx, "class", 5); return err }},
		{"location list", func() error { _, err := ReplayDimensionList(ctx, "location", 5); return err }},
		{"journal bulk create", func() error {
			_, err := ReplayJournalBulkCreate(ctx, []JournalBulkItem{{Amount: 1, FromAccount: "84", ToAccount: "7"}})
			return err
		}},
		{"journal bulk delete", func() error { _, err := ReplayJournalBulkDelete(ctx, []string{"77"}); return err }},
		{"opening balance create", func() error {
			_, err := ReplayOpeningBalanceCreate(ctx, "", []TrialBalanceLine{{Account: "84", Debit: 1}, {Account: "7", Credit: 1}})
			return err
		}},
		{"deferred schedule get", func() error { _, err := ReplayDeferredScheduleGet(ctx, "TX1", false); return err }},
		{"deferred schedule create", func() error {
			_, err := ReplayDeferredScheduleCreate(ctx, map[string]any{"k": "v"}, true)
			return err
		}},
		{"deferred schedule preview", func() error {
			_, err := ReplayDeferredSchedulePreview(ctx, map[string]any{"k": "v"}, true)
			return err
		}},
	}
	for _, tt := range tests {
		start := time.Now()
		err := tt.fn()
		assertFast(t, time.Since(start))
		if !errors.Is(err, ErrNoCredentials) {
			t.Errorf("%s: got %v, want ErrNoCredentials", tt.name, err)
		}
	}
}
