package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// --- VerifyFeed -------------------------------------------------------------

// TestVerifyFeedComplete verifies the happy path: expected==fetched →
// Complete=true, no error, account id echoed.
func TestVerifyFeedComplete(t *testing.T) {
	noCredsDir(t)
	saveUsable(t)
	n := 3
	fs := newFeedServer(t, "44", &n, func(start, size int) []rawTxn {
		return pageOf(3, func(i int) string { return "id" + strconv.Itoa(i) })
	})
	interceptHTTP(t, fs.URL)

	res, err := VerifyFeed(context.Background(), "44")
	if err != nil {
		t.Fatalf("VerifyFeed: %v", err)
	}
	if !res.Complete {
		t.Errorf("Complete = false, want true")
	}
	if res.Expected != 3 || res.Fetched != 3 {
		t.Errorf("Expected=%d Fetched=%d, want 3/3", res.Expected, res.Fetched)
	}
	if res.AccountID != "44" {
		t.Errorf("AccountID = %q, want 44", res.AccountID)
	}
}

// TestVerifyFeedIncompleteReportsWithoutError is the negative case that
// distinguishes verify from population: a mismatch must be REPORTED
// (Complete=false) but must NOT be an error.
func TestVerifyFeedIncompleteReportsWithoutError(t *testing.T) {
	noCredsDir(t)
	saveUsable(t)
	n := 5
	fs := newFeedServer(t, "44", &n, func(start, size int) []rawTxn {
		return pageOf(2, func(i int) string { return "id" + strconv.Itoa(i) })
	})
	interceptHTTP(t, fs.URL)

	res, err := VerifyFeed(context.Background(), "44")
	if err != nil {
		t.Fatalf("verify must not error on mismatch; got %v", err)
	}
	if res.Complete {
		t.Error("Complete = true, want false for 2/5")
	}
	if res.Expected != 5 || res.Fetched != 2 {
		t.Errorf("Expected=%d Fetched=%d, want 5/2", res.Expected, res.Fetched)
	}
}

// TestVerifyFeedUnknownExpected rejects an oracle that treats a missing
// numTxnToReview as complete.
func TestVerifyFeedUnknownExpected(t *testing.T) {
	noCredsDir(t)
	saveUsable(t)
	fs := newFeedServer(t, "44", nil, func(start, size int) []rawTxn {
		return pageOf(1, func(i int) string { return "x" })
	})
	interceptHTTP(t, fs.URL)

	res, err := VerifyFeed(context.Background(), "44")
	if err != nil {
		t.Fatalf("VerifyFeed: %v", err)
	}
	if res.Expected != -1 {
		t.Errorf("Expected = %d, want -1 when server omits numTxnToReview", res.Expected)
	}
	if res.Complete {
		t.Error("Complete = true with unknown expected count")
	}
}

// TestVerifyFeedEmptyPopulation checks the zero-count edge: no rows, no walk
// pages needed beyond one short page, Complete=true.
func TestVerifyFeedEmptyPopulation(t *testing.T) {
	noCredsDir(t)
	saveUsable(t)
	zero := 0
	fs := newFeedServer(t, "44", &zero, func(start, size int) []rawTxn { return nil })
	interceptHTTP(t, fs.URL)

	res, err := VerifyFeed(context.Background(), "44")
	if err != nil {
		t.Fatalf("VerifyFeed: %v", err)
	}
	if !res.Complete || res.Fetched != 0 || res.Expected != 0 {
		t.Errorf("got Expected=%d Fetched=%d Complete=%v, want 0/0/true",
			res.Expected, res.Fetched, res.Complete)
	}
}

// TestVerifyFeedNoCredentials is the fast-fail negative: no saved creds must
// yield ErrNoCredentials before any network activity.
func TestVerifyFeedNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := VerifyFeed(context.Background(), "44")
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want wrapped ErrNoCredentials", err)
	}
	assertFast(t, time.Since(start))
}

// --- ReplayPopulationAll ----------------------------------------------------

// TestReplayPopulationAllSkipsZeroCount verifies zero-pending accounts are
// recorded (complete, empty) without a transaction walk, and walked accounts
// produce correct entries. AllComplete=true when everything matches.
func TestReplayPopulationAllMixedAccounts(t *testing.T) {
	noCredsDir(t)
	saveUsable(t)

	// getInitialData reports two accounts; route by accountId in the tx handler.
	var numA, numB int
	numA, numB = 2, 0
	fs := newFeedServerMulti(t, map[string]int{"204": numA, "45": numB})
	interceptHTTP(t, fs.URL)

	res, err := ReplayPopulationAll(context.Background(), 40)
	if err != nil {
		t.Fatalf("ReplayPopulationAll: %v", err)
	}
	if len(res.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(res.Accounts))
	}
	byID := map[string]AccountPopulation{}
	for _, a := range res.Accounts {
		byID[a.AccountID] = a
	}
	a, okA := byID["204"]
	b, okB := byID["45"]
	if !okA || !okB {
		t.Fatalf("missing manifest entries: %+v", res.Accounts)
	}
	if !a.Complete || a.Fetched != 2 || a.Expected != 2 {
		t.Errorf("account 204: got %+v", a)
	}
	if !b.Complete || b.Fetched != 0 {
		t.Errorf("zero-count account 45 should be complete/empty without walking: %+v", b)
	}
	if !res.AllComplete {
		t.Error("AllComplete = false, want true")
	}
}

// TestReplayPopulationAllIncompleteIsolatesFailure verifies a mismatch on one
// account marks only that account incomplete (AllComplete=false) while others
// remain complete — the manifest must never abort on the first bad account.
//
// Truncation is simulated by giving account 204 more rows than the page
// ceiling can fetch (maxPages=1 x 300 = 300 < 450 served), while account 45
// fits in one short page.
func TestReplayPopulationAllIncompleteIsolatesFailure(t *testing.T) {
	noCredsDir(t)
	saveUsable(t)
	fs := newFeedServerMulti(t, map[string]int{"204": 450, "45": 1})
	interceptHTTP(t, fs.URL)

	res, err := ReplayPopulationAll(context.Background(), 1)
	if err != nil {
		t.Fatalf("ReplayPopulationAll: %v", err)
	}
	if res.AllComplete {
		t.Error("AllComplete = true with a mismatched account")
	}
	byID := map[string]AccountPopulation{}
	for _, a := range res.Accounts {
		byID[a.AccountID] = a
	}
	a45, ok45 := byID["45"]
	if !ok45 || !a45.Complete {
		t.Errorf("account 45 should stay complete: %+v", a45)
	}
	a204 := byID["204"]
	if a204.Complete {
		t.Errorf("account 204 (server says 450, fetched %d) should be incomplete", a204.Fetched)
	}
	if a204.Fetched != 300 {
		t.Errorf("account 204 fetched = %d, want 300 (one full page at the ceiling)", a204.Fetched)
	}
}

// TestReplayPopulationAllNoCredentials fast-fails without credentials.
func TestReplayPopulationAllNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayPopulationAll(context.Background(), 40)
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v, want wrapped ErrNoCredentials", err)
	}
	assertFast(t, time.Since(start))
}

// newFeedServerMulti builds a feed server whose getInitialData lists several
// accounts (each with its own numTxnToReview) and whose getTransactions pages
// per-account: each account serves exactly its own count of unique ids.
func newFeedServerMulti(t *testing.T, counts map[string]int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case endsWith(r.URL.Path, "/getInitialData"):
			accs := make([]any, 0, len(counts))
			for id, n := range counts {
				accs = append(accs, map[string]any{"qboAccountId": id, "numTxnToReview": n})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"accounts": accs})
		case endsWith(r.URL.Path, "/getTransactions"):
			q := r.URL.Query()
			id := q.Get("accountId")
			total := counts[id]
			start, _ := strconv.Atoi(q.Get("startIndex"))
			size, _ := strconv.Atoi(q.Get("chunkSize"))
			items := make([]rawTxn, 0, size)
			for i := start; i < total && i < start+size; i++ {
				items = append(items, rawTxn{
					ID: id + "-id" + strconv.Itoa(i), OlbTxnID: id + "-olb" + strconv.Itoa(i),
					Description: "row", AcceptType: "ADD",
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
