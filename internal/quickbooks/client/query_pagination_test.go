package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pagingServer answers v3 /query?query=SELECT…STARTPOSITION n MAXRESULTS m
// from a synthetic dataset of `total` Customer rows.
func pagingServer(t *testing.T, total int) (*httptest.Server, *int64) {
	t.Helper()
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		q, _ := url.QueryUnescape(r.URL.Query().Get("query"))
		start, max := 1, 100
		lq := strings.ToLower(q)
		if i := strings.Index(lq, "startposition"); i >= 0 {
			fmt.Sscanf(lq[i:], "startposition %d", &start)
		}
		if i := strings.Index(lq, "maxresults"); i >= 0 {
			fmt.Sscanf(lq[i:], "maxresults %d", &max)
		}
		var rows []string
		for i := start - 1; i < start-1+max && i < total; i++ {
			rows = append(rows, fmt.Sprintf(`{"Id":"%d","DisplayName":"c%d"}`, i+1, i+1))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"QueryResponse":{"Customer":[%s]}}`, strings.Join(rows, ","))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestQueryPaginationTerminates(t *testing.T) {
	saveUsable(t)
	for _, tc := range []struct {
		name      string
		total     int
		limit     int
		wantItems int
		maxCalls  int64
	}{
		{"short page ends unbounded walk", 250, 0, 250, 3},
		{"empty result", 0, 0, 0, 1},
		{"exact page boundary", 200, 200, 200, 2},
		{"limit beyond 5000", 6000, 5500, 5500, 55},
		{"unbounded over 5000", 5200, 0, 5200, 53},
		{"limit smaller than page", 37, 37, 37, 1},
	} {
		srv, calls := pagingServer(t, tc.total)
		interceptHTTP(t, srv.URL)
		done := make(chan *QueryResult, 1)
		go func() {
			res, err := ReplayQuery(context.Background(), "Customer", "", "", tc.limit)
			if err != nil {
				t.Errorf("%s: %v", tc.name, err)
			}
			done <- res
		}()
		select {
		case res := <-done:
			if res == nil {
				t.Fatalf("%s: no result", tc.name)
			}
			if len(res.Items) != tc.wantItems {
				t.Errorf("%s: got %d items, want %d", tc.name, len(res.Items), tc.wantItems)
			}
			if atomic.LoadInt64(calls) > tc.maxCalls {
				t.Errorf("%s: %d calls, want ≤ %d", tc.name, *calls, tc.maxCalls)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: loop did not terminate", tc.name)
		}
		srv.Close()
	}
}

// A server that ignores STARTPOSITION and always returns the same full page
// must fail closed, not present the first page as a complete population.
func TestQueryPaginationNonAdvancingServer(t *testing.T) {
	saveUsable(t)
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		var rows []string
		for i := range 100 {
			rows = append(rows, fmt.Sprintf(`{"Id":"%d"}`, i+1))
		}
		fmt.Fprintf(w, `{"QueryResponse":{"Customer":[%s]}}`, strings.Join(rows, ","))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	res, err := ReplayQuery(ctx, "Customer", "", "", 0)
	if err == nil || res != nil || ctx.Err() != nil {
		t.Fatalf("repeated page must fail before deadline: result=%+v, err=%v", res, err)
	}
	if atomic.LoadInt64(&calls) != 2 {
		t.Fatalf("%d calls before stopping", calls)
	}
}

// Overlapping windows (different bytes, repeated ids) also terminate.
func TestQueryPaginationOverlapWindows(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.QueryUnescape(r.URL.Query().Get("query"))
		start := 1
		if i := strings.Index(strings.ToLower(q), "startposition"); i >= 0 {
			fmt.Sscanf(strings.ToLower(q)[i:], "startposition %d", &start)
		}
		// window advances by only 50 rows per request → ids overlap pages
		var rows []string
		for i := (start - 1) / 2; i < (start-1)/2+100; i++ {
			rows = append(rows, fmt.Sprintf(`{"Id":"%d"}`, i+1))
		}
		fmt.Fprintf(w, `{"QueryResponse":{"Customer":[%s]}}`, strings.Join(rows, ","))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(context.Background(), "Customer", "", "", 300)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, it := range res.Items {
		if seen[it.ID] {
			t.Fatalf("duplicate id %s in output", it.ID)
		}
		seen[it.ID] = true
	}
	if len(res.Items) != 300 {
		t.Fatalf("got %d items, want 300", len(res.Items))
	}
}
