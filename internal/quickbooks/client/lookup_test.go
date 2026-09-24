package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupFindsOlderRowsAndSeparatesCoverageFromLimit(t *testing.T) {
	saveUsable(t)
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if q.Get("accountId") != "204" || q.Get("chunkSize") != "300" {
			t.Errorf("unexpected lookup scope: %v", q)
		}
		body := `{"items":[],"totalTransactionsCount":0}`
		if q.Get("reviewState") == "PENDING" {
			body = `{"items":[],"totalTransactionsCount":3}`
			switch q.Get("startIndex") {
			case "0":
				// A short page and underreported total must not end the walk.
				body = `{"items":[{"id":"new","qboAccountId":204,"origDescription":"Other"}],"totalTransactionsCount":1}`
			case "300":
				body = `{"items":[{"id":"older-1","qboAccountId":204,"origDescription":"WOOLWORTHS ONLINE"}],"totalTransactionsCount":2}`
			case "600":
				body = `{"items":[{"id":"older-2","qboAccountId":"204","origDescription":"WW Online","description":"Woolworths Online"}],"totalTransactionsCount":2}`
			}
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	result, err := ReplayLookup(context.Background(), "204", "woolworths", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Transactions) != 1 || result.Transactions[0].ID != "older-1" || result.Transactions[0].ReviewState != "PENDING" {
		t.Fatalf("older match lost: %+v", result)
	}
	c := result.Lookup
	if c == nil || !c.Complete || !c.Truncated || c.TotalMatches != 2 || c.RowsByState["PENDING"] != 3 || c.PagesByState["PENDING"] != 4 || requests != 6 {
		t.Fatalf("incorrect coverage: %+v; requests=%d", c, requests)
	}
}

func TestLookupRejectsIncompleteOrContradictoryPages(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"missing array", `{}`},
		{"null array", `{"items":null}`},
		{"missing terminal total", `{"items":[]}`},
		{"contradictory terminal total", `{"items":[],"totalTransactionsCount":1}`},
		{"wrong account", `{"items":[{"id":"x","qboAccountId":209}]}`},
		{"missing identity", `{"items":[{"qboAccountId":204}]}`},
		{"repeated page", `{"items":[{"id":"x","qboAccountId":204}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveUsable(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			result, err := ReplayLookup(context.Background(), "204", "absent", 1)
			if !errors.Is(err, ErrIncomplete) || result != nil {
				t.Fatalf("incomplete lookup must not report absence: result=%+v error=%v", result, err)
			}
		})
	}
}

func TestLookupDoesNotAcceptTheSameIDInDifferentStates(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := `{"items":[],"totalTransactionsCount":1}`
		if r.URL.Query().Get("startIndex") == "0" {
			body = `{"items":[{"id":"moving-row","qboAccountId":204}]}`
		}
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	if _, err := ReplayLookup(context.Background(), "204", "moving", 1); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("cross-state drift must fail: %v", err)
	}
}
