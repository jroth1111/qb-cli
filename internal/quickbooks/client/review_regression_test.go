package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewQueryAllRejectsIncompleteResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"wrong collection", 200, `{"QueryResponse":{"Customer":[{"Id":"1"}]}}`},
		{"malformed collection", 200, `{"QueryResponse":{"Deposit":"bad"}}`},
		{"null collection", 200, `{"QueryResponse":{"Deposit":null}}`},
		{"missing identity", 200, `{"QueryResponse":{"Deposit":[{}]}}`},
		{"http failure with query body", 403, `{"QueryResponse":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveUsable(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			if rows, err := v3QueryAll(t.Context(), ac, "deposit", "select * from deposit"); err == nil {
				t.Fatalf("invalid census accepted: %s", rows)
			}
		})
	}
}

func TestReviewQueryAllRepeatedPageFailsClosed(t *testing.T) {
	saveUsable(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		rows := make([]map[string]string, queryPageSize)
		for i := range rows {
			rows[i] = map[string]string{"Id": fmt.Sprint(i + 1)}
		}
		json.NewEncoder(w).Encode(map[string]any{"QueryResponse": map[string]any{"Deposit": rows}, "time": calls})
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := v3QueryAll(t.Context(), ac, "deposit", "select * from deposit"); err == nil {
		t.Fatalf("repeated page reported as a complete census: %d rows", len(rows))
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestReviewQuerySearchFallbackPagesToMatches(t *testing.T) {
	for _, entity := range []string{"Customer", "Estimate"} {
		t.Run(entity, func(t *testing.T) {
			saveUsable(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := strings.ToLower(r.URL.Query().Get("query"))
				if strings.Contains(q, "where") {
					if entity == "Customer" {
						w.WriteHeader(http.StatusBadRequest)
					}
					fmt.Fprint(w, `{"QueryResponse":{}}`)
					return
				}
				start, size := 1, 100
				if i := strings.Index(q, "startposition"); i >= 0 {
					fmt.Sscanf(q[i:], "startposition %d", &start)
				}
				if i := strings.Index(q, "maxresults"); i >= 0 {
					fmt.Sscanf(q[i:], "maxresults %d", &size)
				}
				rows := []map[string]string{}
				for i := start; i < start+size && i <= 135; i++ {
					name := "irrelevant"
					if i > 130 {
						name = "needle"
					}
					rows = append(rows, map[string]string{"Id": fmt.Sprint(i), "DisplayName": name, "DocNumber": name})
				}
				json.NewEncoder(w).Encode(map[string]any{"QueryResponse": map[string]any{entity: rows}})
			}))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			res, err := ReplayQuery(context.Background(), entity, "", "needle", 3)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Items) != 3 || res.Items[0].ID != "131" {
				t.Fatalf("search stopped before matching rows: %+v", res)
			}
		})
	}
}

func TestReviewFolioRequiresAllPagesAndAgreement(t *testing.T) {
	for _, mode := range []string{"missing unit", "missing combined", "disagreeing leaf", "company only"} {
		t.Run(mode, func(t *testing.T) {
			a := &FolioAudit{Pages: []folioAuditPage{
				{Title: "detail", Kind: "unit", Report: "ProfitAndLossDetail", Classes: []string{"A"}},
				{Title: "summary", Kind: "unit", Classes: []string{"A"}},
				{Title: "combined", Kind: "combined", Classes: []string{"A", "B"}},
				{Title: "other unit", Kind: "unit", Classes: []string{"B"}},
			}}
			for i := range a.Pages {
				p := &a.Pages[i]
				if mode == "missing unit" && p.Title == "summary" || mode == "missing combined" && p.Kind == "combined" {
					continue
				}
				net := 0.0
				if mode == "disagreeing leaf" && p.Title == "summary" {
					net = 10
				}
				a.record("2026-05", p, &auditTotals{Net: net})
			}
			if mode == "company only" {
				a = &FolioAudit{Pages: []folioAuditPage{{Title: "company", Kind: "company"}}}
				a.record("2026-05", &a.Pages[0], &auditTotals{})
			}
			a.reconcile()
			if a.Periods[0].OK {
				t.Fatalf("incomplete/disagreeing audit passed: %+v", a.Periods[0])
			}
		})
	}
}

func TestReviewFolioReconcileIsIdempotent(t *testing.T) {
	a := &FolioAudit{Pages: []folioAuditPage{{Title: "unit", Kind: "unit", Classes: []string{"A"}}}}
	a.record("2026-05", &a.Pages[0], &auditTotals{Net: 42})
	a.reconcile()
	a.reconcile()
	if a.Periods[0].UnitSum != 42 {
		t.Fatalf("repeated reconciliation double counted: %+v", a.Periods[0])
	}
}

func TestReviewFolioTotalsUseFinalNetIncome(t *testing.T) {
	rows := json.RawMessage(`{"Row":[
		{"Summary":{"ColData":[{"value":"Net Operating Income"},{"value":"100"}]}},
		{"Summary":{"ColData":[{"value":"Net Other Income"},{"value":"20"}]}},
		{"Summary":{"ColData":[{"value":"Net Income"},{"value":"120"}]}}
	]}`)
	tot, err := v3ReportTotals(rows)
	if err != nil || tot.Net != 120 {
		t.Fatalf("net income = %+v, %v", tot, err)
	}
}

func TestReviewFolioTxnRowsIncludesTopLevelLeaves(t *testing.T) {
	rows := json.RawMessage(`{"Row":[{"ColData":[{"value":"2026-05-01"},{"value":"25"}]}]}`)
	got, err := v3TxnRows([]string{"Date", "Amount"}, rows)
	if err != nil || len(got) != 1 || got[0].amount != 25 {
		t.Fatalf("top-level row lost: %+v, %v", got, err)
	}
}

func TestReviewCatalogReadsDoNotRequestZeroRows(t *testing.T) {
	for _, tc := range []struct{ entity, variable, body string }{
		{"Task", "first", `{"data":{"taskManagementTasks":{"edges":[]}}}`},
		{"DimensionDefinition", "firstCount", `{"data":{"appFoundationsCustomDimensionDefinitions":{"edges":[]}}}`},
		{"TaxJurisdiction", "DEFAULT_FIRST_COUNT", `{"data":{"company":{"taxJurisdictions":{"edges":[]}}}}`},
		{"InventoryLocation", "first", `{"data":{"commerceInventoryLocations":{"nodes":[]}}}`},
	} {
		t.Run(tc.entity, func(t *testing.T) {
			saveUsableURIHost(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Variables map[string]any `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if n, ok := req.Variables[tc.variable].(float64); !ok || n <= 0 {
					t.Errorf("unbounded list requested zero rows: %v", req.Variables)
				}
				fmt.Fprint(w, tc.body)
			}))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			if _, err := ReplayQuery(t.Context(), tc.entity, "", "", 0); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReviewCatalogReadsRejectFalseEmpty(t *testing.T) {
	for _, body := range []string{
		`{"errors":[{"message":"denied"}]}`,
		`{"data":{"taskManagementTasks":{"edges":[]}},"errors":[{}]}`,
		`{"data":{}}`, `{"data":{"taskManagementTasks":{"edges":null}}}`,
		`{"data":{"taskManagementTasks":{"edges":[{"node":null}]}}}`,
	} {
		t.Run(body, func(t *testing.T) {
			saveUsableURIHost(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			if res, err := ReplayQuery(t.Context(), "Task", "", "", 1); err == nil {
				t.Fatalf("false empty success: %+v", res)
			}
		})
	}
}
