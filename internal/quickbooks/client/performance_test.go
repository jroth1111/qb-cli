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

// REPORTS.PERFORMANCE_* — dashboardframework panels CRUD captured from
// /app/insights/dashboard. Live-proven on TC2 2026-09-19: POST panel→200,
// PUT full panel→200 rename, DELETE→200 active:false. Partial PUT bodies
// 500 server-side — update always sends the full panel.

// perfServer answers dashboard lookup, paneldefinitions, and panels calls.
type perfServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    int
	lastMeth string
	lastPath string
	lastBody []byte
}

func newPerfServer(t *testing.T) *perfServer {
	t.Helper()
	s := &perfServer{}
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
		switch {
		case strings.HasSuffix(r.URL.Path, "/system/paneldefinitions"):
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(`<List><item><id>Revenue</id><name>Revenue</name><defaultView>TREND_LINE</defaultView></item><item><id>Expenses</id><name>Expenses</name><defaultView>VERTICAL_BAR</defaultView></item></List>`))
		case strings.HasSuffix(r.URL.Path, "/dashboards"):
			_, _ = w.Write([]byte(`[{"id":"sbg:dash-1","name":"Performance dashboard"}]`))
		case strings.HasSuffix(r.URL.Path, "/panels") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[{"id":"sbg:p1","name":"Expenses by time","type":"METRIC","active":true,"customization":{}}]`))
		case strings.Contains(r.URL.Path, "/panels"):
			// create POST / PUT / DELETE / {id}
			if r.Method == http.MethodDelete {
				_, _ = w.Write([]byte(`{"id":"sbg:p9","name":"zz","type":"METRIC","active":false}`))
				return
			}
			if r.Method == http.MethodPut {
				var m map[string]any
				_ = json.Unmarshal(b, &m)
				m["id"] = "sbg:p9"
				out, _ := json.Marshal(m)
				_, _ = w.Write(out)
				return
			}
			// POST create
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			m["id"] = "sbg:p9"
			out, _ := json.Marshal(m)
			_, _ = w.Write(out)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *perfServer) snap() (calls int, meth, path string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.lastMeth, s.lastPath, s.lastBody
}

func TestPerformanceCreatePostsPanel(t *testing.T) {
	saveUsableURIHost(t)
	srv := newPerfServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayPerformanceMutate(context.Background(), "create", "", "My Chart", "revenue", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Item.ID != "sbg:p9" || res.Entity != "PerformanceChart" {
		t.Fatalf("res = %+v", res)
	}
	_, meth, path, body := srv.snap()
	if meth != http.MethodPost || !strings.HasSuffix(path, "/dashboards/sbg:dash-1/panels") {
		t.Fatalf("%s %s", meth, path)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	cust, _ := m["customization"].(map[string]any)
	data, _ := cust["data"].(map[string]any)
	view, _ := cust["view"].(map[string]any)
	if m["name"] != "My Chart" || m["type"] != "METRIC" || data["ref"] != "Revenue" || view["visualization"] != "TREND_LINE" {
		t.Fatalf("body = %v", m)
	}
}

func TestPerformanceCreateResolvesDefaultView(t *testing.T) {
	saveUsableURIHost(t)
	srv := newPerfServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayPerformanceMutate(context.Background(), "create", "", "X", "Expenses", ""); err != nil {
		t.Fatal(err)
	}
	_, _, _, body := srv.snap()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	cust, _ := m["customization"].(map[string]any)
	view, _ := cust["view"].(map[string]any)
	if view["visualization"] != "VERTICAL_BAR" {
		t.Fatalf("defaultView resolution failed: %v", view)
	}
}

func TestPerformanceUpdatePutsFullPanel(t *testing.T) {
	saveUsableURIHost(t)
	srv := newPerfServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayPerformanceMutate(context.Background(), "update", "sbg:p1", "Renamed", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.Name != "Renamed" {
		t.Fatalf("res = %+v", res)
	}
	_, meth, path, body := srv.snap()
	if meth != http.MethodPut || !strings.HasSuffix(path, "/panels/sbg:p1") {
		t.Fatalf("%s %s", meth, path)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	// full object: type+active must ride along, not just the name
	if m["name"] != "Renamed" || m["type"] != "METRIC" || m["active"] != true {
		t.Fatalf("PUT must send the full panel: %v", m)
	}
}

func TestPerformanceDeleteHitsPanel(t *testing.T) {
	saveUsableURIHost(t)
	srv := newPerfServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayPerformanceMutate(context.Background(), "delete", "sbg:p9", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.Type != "Chart (inactive)" {
		t.Fatalf("delete must project active:false: %+v", res.Item)
	}
	_, meth, path, _ := srv.snap()
	if meth != http.MethodDelete || !strings.HasSuffix(path, "/panels/sbg:p9") {
		t.Fatalf("%s %s", meth, path)
	}
}

func TestPerformanceValidation(t *testing.T) {
	saveUsableURIHost(t)
	srv := newPerfServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayPerformanceMutate(context.Background(), "create", "", "", "Revenue", ""); err == nil {
		t.Fatal("blank --name must fail")
	}
	if _, err := ReplayPerformanceMutate(context.Background(), "create", "", "X", "", ""); err == nil {
		t.Fatal("blank --metric must fail")
	}
	if _, err := ReplayPerformanceMutate(context.Background(), "update", " ", "x", "", ""); err == nil {
		t.Fatal("blank --id must fail")
	}
	if _, err := ReplayPerformanceMutate(context.Background(), "delete", "", "", "", ""); err == nil {
		t.Fatal("delete blank --id must fail")
	}
	if calls, _, _, _ := srv.snap(); calls != 0 {
		t.Fatalf("validation must precede HTTP, calls=%d", calls)
	}
	// unknown metric name surfaces the definition catalog
	_, err := ReplayPerformanceMutate(context.Background(), "create", "", "X", "BogusMetric", "")
	if err == nil || !strings.Contains(err.Error(), "BogusMetric") {
		t.Fatalf("unknown metric err = %v", err)
	}
}

func TestPlannedDashboardsURL(t *testing.T) {
	if !strings.Contains(PlannedDashboardsURL(), "Performance%20dashboard") {
		t.Fatalf("url = %s", PlannedDashboardsURL())
	}
}
