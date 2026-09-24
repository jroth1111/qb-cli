package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// taxServer answers the two-step indirect-tax read: TaxAuthorities first
// (agency id), then node__indirect_tax_ui_qbo (taxReturns connection).
type taxServer struct {
	*httptest.Server

	mu          sync.Mutex
	agencyBody  []byte
	returnsBody []byte
}

func newTaxServer(t *testing.T) *taxServer {
	t.Helper()
	saveUsableURIHost(t)
	s := &taxServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		s.mu.Lock()
		switch {
		case strings.Contains(string(b), "taxAgencies"):
			s.agencyBody = b
		case strings.Contains(string(b), "taxReturns"):
			s.returnsBody = b
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(string(b), "taxAgencies"):
			_, _ = w.Write([]byte(`{"data":{"company":{"taxAgencies":{"edges":[
				{"node":{"id":"agency:1","code":"GapAgency1","displayName":"GapAgency1","__typename":"Indirecttaxes_TaxAgency"}}
			]}}}}`))
		case strings.Contains(string(b), "taxReturns"):
			_, _ = w.Write([]byte(`{"data":{"node":{"id":"agency:1","name":"GapAgency1",
				"taxReturns":{"edges":[{"node":{"id":"ret:1","taxReturnType":"OPEN",
				"startDate":"2026-01-01","endDate":"2026-03-31","upcomingFiling":true,
				"taxAgency":{"id":"agency:1","name":"GapAgency1","code":"GapAgency1"},
				"__typename":"Indirecttaxes_TaxReturn"}}],"__typename":"c"}}}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestTaxReturnsResolvesAgencyThenLists(t *testing.T) {
	s := newTaxServer(t)
	res, err := ReplayTaxReturns(context.Background(), "TaxReturn", "", "", 10)
	if err != nil {
		t.Fatalf("taxReturns: %v", err)
	}
	if res.Counts["items"] != 1 || res.Items[0].ID != "ret:1" {
		t.Fatalf("items = %+v", res.Items)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.agencyBody) == 0 || len(s.returnsBody) == 0 {
		t.Fatal("expected agency + returns calls")
	}
	var vars struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(s.returnsBody, &vars); err != nil {
		t.Fatal(err)
	}
	if vars.Variables["id"] != "agency:1" {
		t.Fatalf("node id = %v, want resolved agency:1", vars.Variables["id"])
	}
}

func TestTaxReturnsFilterExpressionSent(t *testing.T) {
	s := newTaxServer(t)
	if _, err := ReplayTaxReturns(context.Background(), "TaxReturn", "taxReturnType = 'OPEN'", "", 10); err != nil {
		t.Fatalf("taxReturns: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var m map[string]any
	if err := json.Unmarshal(s.returnsBody, &m); err != nil {
		t.Fatal(err)
	}
	vars := m["variables"].(map[string]any)
	if vars["taxReturnTypeFilterExpression"] != "taxReturnType = 'OPEN'" {
		t.Fatalf("filter = %v", vars["taxReturnTypeFilterExpression"])
	}
}

// forecastServer answers getAllBusinessForecasts on the planningforecasting host.
func forecastServer(t *testing.T, body string) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, body)
	interceptHTTP(t, srv.URL)
	return srv
}

func TestBusinessForecastsList(t *testing.T) {
	forecastServer(t, `{"data":{"getAllBusinessForecasts":{"edges":[
		{"cursor":"c1","node":{"forecastId":"f1","name":"FY27","forecastType":"Annual"}}
	],"totalCount":1}}}`)
	res, err := ReplayBusinessForecasts(context.Background(), "", "", 10)
	if err != nil {
		t.Fatalf("forecasts: %v", err)
	}
	if res.Counts["items"] != 1 || res.Items[0].ID != "f1" {
		t.Fatalf("items = %+v", res.Items)
	}
}

func TestBusinessForecastsIDFilter(t *testing.T) {
	forecastServer(t, `{"data":{"getAllBusinessForecasts":{"edges":[
		{"cursor":"c1","node":{"forecastId":"f1","name":"A"}},
		{"cursor":"c2","node":{"forecastId":"f2","name":"B"}}
	],"totalCount":2}}}`)
	res, err := ReplayBusinessForecasts(context.Background(), "f2", "", 10)
	if err != nil {
		t.Fatalf("forecasts: %v", err)
	}
	if res.Counts["items"] != 1 || res.Items[0].ID != "f2" {
		t.Fatalf("items = %+v", res.Items)
	}
}

// metricsServer answers POST /v1/metrics/<Name> with a dataFrames body.
func metricsServer(t *testing.T) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	srv := &domServer{t: t}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dataFrames":[{"columns":["month","value"],"rows":[["2026-08",100.0],["2026-09",150.0]]}]}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	return srv
}

func TestPerformanceMetricsSumsFrames(t *testing.T) {
	metricsServer(t)
	res, err := ReplayPerformanceMetrics(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if res.Counts["items"] != len(performanceMetrics) {
		t.Fatalf("items = %d, want %d", res.Counts["items"], len(performanceMetrics))
	}
	var rev *QueryItem
	for i := range res.Items {
		if res.Items[i].ID == "Revenue" {
			rev = &res.Items[i]
		}
	}
	if rev == nil || rev.Amount != 250 {
		t.Fatalf("Revenue = %+v, want amount 250", rev)
	}
}

func TestPerformanceMetricsQueryFilter(t *testing.T) {
	metricsServer(t)
	res, err := ReplayPerformanceMetrics(context.Background(), "ratio", 0)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if res.Counts["items"] != 2 {
		t.Fatalf("items = %d, want 2 (CurrentRatio+QuickRatio)", res.Counts["items"])
	}
}
