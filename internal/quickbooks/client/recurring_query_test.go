package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// recurringQueryServer answers v3 /query?query=… against a fixed population
// of wrapped RecurringTransaction rows. whereQueries counts how many requests
// carried a WHERE clause so tests can prove the walk never ran.
func recurringQueryServer(t *testing.T, rows []string, rejectWhere bool) (*httptest.Server, *int) {
	t.Helper()
	var whereQueries int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		qs, _ := url.QueryUnescape(r.URL.Query().Get("query"))
		if strings.Contains(strings.ToLower(qs), "where") {
			whereQueries++
			if rejectWhere {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"Fault":{"Error":[{"Message":"where not supported on RecurringTransaction"}]}}`)
				return
			}
			// where Id = 'N' — match the live API: return the wrapped row.
			var id string
			if i := strings.Index(qs, "Id = '"); i >= 0 {
				id = strings.SplitN(qs[i+len("Id = '"):], "'", 2)[0]
			}
			for _, row := range rows {
				if strings.Contains(row, `"Id":"`+id+`"`) {
					fmt.Fprintf(w, `{"QueryResponse":{"RecurringTransaction":[%s]}}`, row)
					return
				}
			}
			fmt.Fprint(w, `{"QueryResponse":{}}`)
			return
		}
		// Paged walk fallback.
		fmt.Fprintf(w, `{"QueryResponse":{"RecurringTransaction":[%s]}}`, strings.Join(rows, ","))
	}))
	t.Cleanup(srv.Close)
	return srv, &whereQueries
}

// `where Id` on RecurringTransaction — verified live (2026-09-29, AU build).
// The targeted query must unwrap the nested entity and skip the paged walk.
func TestFetchRecurringByIDTargetedQuery(t *testing.T) {
	saveUsable(t)
	srv, whereQueries := recurringQueryServer(t,
		[]string{
			`{"Invoice":{"Id":"817","DocNumber":"R-817"}}`,
			`{"JournalEntry":{"Id":"22493"}}`,
		}, false)
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	obj, err := fetchRecurringByID(context.Background(), ac, "817")
	if err != nil {
		t.Fatal(err)
	}
	if obj["DocNumber"] != "R-817" || obj["Id"] != "817" {
		t.Fatalf("expected unwrapped invoice row: %v", obj)
	}
	if *whereQueries != 1 {
		t.Fatalf("expected exactly one where-query, got %d", *whereQueries)
	}
}

// A successful where-query returning no rows is a definitive not-found —
// the paged walk must not run on top of it.
func TestFetchRecurringByIDAbsentNoWalk(t *testing.T) {
	saveUsable(t)
	srv, whereQueries := recurringQueryServer(t,
		[]string{`{"Invoice":{"Id":"817"}}`}, false)
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fetchRecurringByID(context.Background(), ac, "999"); err == nil {
		t.Fatal("absent id returned a row")
	}
	if *whereQueries != 1 {
		t.Fatalf("walk ran after a definitive empty where-query (whereQueries=%d)", *whereQueries)
	}
}

// Builds that reject `where` on RecurringTransaction fall back to the paged
// walk and still resolve the wrapped entity.
func TestFetchRecurringByIDWalkFallback(t *testing.T) {
	saveUsable(t)
	srv, whereQueries := recurringQueryServer(t,
		[]string{`{"Invoice":{"Id":"817","DocNumber":"R-817"}}`}, true)
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	obj, err := fetchRecurringByID(context.Background(), ac, "817")
	if err != nil {
		t.Fatal(err)
	}
	if obj["DocNumber"] != "R-817" {
		t.Fatalf("walk fallback lost the entity: %v", obj)
	}
	if *whereQueries != 1 {
		t.Fatalf("expected the where-query attempt, got %d", *whereQueries)
	}
}
