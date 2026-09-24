package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- pure projection: projectTxn ------------------------------------------

// TestProjectTxnOrigDescriptionPreference is table-driven over the
// OrigDescription preference rule in projectTxn:
//
//	desc := t.Description
//	if t.OrigDescription != "" && (desc == "" || len(orig) > len(desc)) {
//	    desc = t.OrigDescription
//	}
//
// It rejects implementations that always prefer OrigDescription, never prefer
// it, or prefer it on equal length (the comparison is strict >).
func TestProjectTxnOrigDescriptionPreference(t *testing.T) {
	cases := []struct {
		name string
		in   rawTxn
		want Transaction
	}{
		{
			name: "orig empty keeps description",
			in:   rawTxn{ID: "t1", Description: "Coffee", Amount: new(4.5), AcceptType: "MATCH"},
			want: Transaction{ID: "t1", Description: "Coffee", Amount: 4.5, AcceptType: "MATCH"},
		},
		{
			name: "description empty falls back to orig",
			in:   rawTxn{ID: "t2", OrigDescription: "COLES GROUP 0001", Amount: new(1.0)},
			want: Transaction{ID: "t2", Description: "COLES GROUP 0001", Amount: 1},
		},
		{
			name: "orig longer wins over description",
			in:   rawTxn{ID: "t3", Description: "Coffee", OrigDescription: "COLES GROUP 0001", Amount: new(2.0)},
			want: Transaction{ID: "t3", Description: "COLES GROUP 0001", Amount: 2},
		},
		{
			name: "orig shorter keeps description",
			in:   rawTxn{ID: "t4", Description: "COLES GROUP 0001", OrigDescription: "Coffee", Amount: new(3.0)},
			want: Transaction{ID: "t4", Description: "COLES GROUP 0001", Amount: 3},
		},
		{
			name: "equal length keeps description (strict >)",
			in:   rawTxn{ID: "t5", Description: "Coffee", OrigDescription: "Mocha!", Amount: new(0.0)},
			want: Transaction{ID: "t5", Description: "Coffee", Amount: 0},
		},
		{
			name: "nil amount projects to zero",
			in:   rawTxn{ID: "t6", Description: "Coffee"},
			want: Transaction{ID: "t6", Description: "Coffee", Amount: 0},
		},
		{
			name: "all fields mapped",
			in:   rawTxn{ID: "t7", OlbTxnID: "olb7", OlbTxnDate: "2026-08-23", Amount: new(9.99), Description: "D", AcceptType: "ACCEPT"},
			want: Transaction{ID: "t7", OLBTxnID: "olb7", Date: "2026-08-23", Amount: 9.99, Description: "D", AcceptType: "ACCEPT"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := projectTxn(tc.in)
			if got != tc.want {
				t.Fatalf("projectTxn = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// --- pure projection: projectPending ---------------------------------------

// TestProjectPendingCounts asserts projectPending maps items via projectTxn
// and reports counts["transactions"] always and counts["total"] only when
// TotalTransactionsCount is present. It rejects implementations that omit
// the transaction count, always set total, or set total to 0 when absent.
func TestProjectPendingCounts(t *testing.T) {
	items := []rawTxn{
		{ID: "a", Description: "A", Amount: new(1.0)},
		{ID: "b", Description: "B", Amount: new(2.0)},
	}
	cases := []struct {
		name        string
		data        *pendingData
		wantTxCount int
		wantTotal   int
		wantTotalOK bool
	}{
		{
			name:        "empty items no total",
			data:        &pendingData{Items: nil},
			wantTxCount: 0,
			wantTotalOK: false,
		},
		{
			name:        "two items with total",
			data:        &pendingData{Items: items, TotalTransactionsCount: new(5)},
			wantTxCount: 2,
			wantTotal:   5,
			wantTotalOK: true,
		},
		{
			name:        "two items total absent",
			data:        &pendingData{Items: items},
			wantTxCount: 2,
			wantTotalOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := projectPending(tc.data)
			if res.Status != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.Status)
			}
			if got := res.Counts["transactions"]; got != tc.wantTxCount {
				t.Fatalf("counts[transactions] = %d, want %d", got, tc.wantTxCount)
			}
			total, ok := res.Counts["total"]
			if ok != tc.wantTotalOK {
				t.Fatalf("counts[total] present = %v, want %v", ok, tc.wantTotalOK)
			}
			if ok && total != tc.wantTotal {
				t.Fatalf("counts[total] = %d, want %d", total, tc.wantTotal)
			}
			if len(res.Transactions) != tc.wantTxCount {
				t.Fatalf("len(transactions) = %d, want %d", len(res.Transactions), tc.wantTxCount)
			}
		})
	}
}

// --- sentinel: ErrIncomplete -----------------------------------------------

// TestErrIncompleteSentinel asserts ErrIncomplete is usable with errors.Is
// both bare and wrapped, and is distinct from ErrNoCredentials. It rejects
// a non-sentinel (plain string) error or a sentinel that collides with the
// credentials guard.
func TestErrIncompleteSentinel(t *testing.T) {
	if !errors.Is(ErrIncomplete, ErrIncomplete) {
		t.Fatal("errors.Is(ErrIncomplete, ErrIncomplete) = false")
	}
	wrapped := fmt.Errorf("walk: %w", ErrIncomplete)
	if !errors.Is(wrapped, ErrIncomplete) {
		t.Fatal("wrapped ErrIncomplete not detectable via errors.Is")
	}
	if errors.Is(ErrIncomplete, ErrNoCredentials) {
		t.Fatal("ErrIncomplete must not satisfy errors.Is for ErrNoCredentials")
	}
	if errors.Is(ErrNoCredentials, ErrIncomplete) {
		t.Fatal("ErrNoCredentials must not satisfy errors.Is for ErrIncomplete")
	}
}

// --- ReplayFeedComplete: missing-credentials fast-fail (negative) ----------

// TestReplayFeedCompleteFailsOnNoCredentials is the missing-credentials
// negative for ReplayFeedComplete. With QB_HOME pointed at an empty temp dir
// there are no saved credentials, so ReplayFeedComplete MUST fail with
// ErrNoCredentials before any network activity. It rejects:
//   - returning a nil error (silently succeeding with no credentials),
//   - a non-ErrNoCredentials error (guard bypassed -> wrong failure),
//   - a non-nil result (partial data returned without a session),
//   - taking long enough to imply a network call.
func TestReplayFeedCompleteFailsOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	res, err := ReplayFeedComplete(context.Background(), "", "PENDING", 5)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected ErrNoCredentials, got nil")
	}
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
	if res != nil {
		t.Fatalf("expected nil result on missing creds, got %+v", res)
	}
	assertFast(t, elapsed)
}

// --- HTTP interception seam -------------------------------------------------
//
// ReplayFeedComplete, fetchFeedPage, and expectedPending all build URLs from
// apiClient.baseURL(), which formats the const bankingBaseURL
// "https://qbo.intuit.com/ats/v1/company/%s/banking". The realm only fills
// the %s path slot, so no realm value can redirect the dial to a test
// server (attempted and rejected: the host is hardcoded). httptest.NewTLSServer
// would require a custom TLS config on the production http.Client, which would
// mean modifying http.go — not allowed here.
//
// The clean seam: doStdlib builds &http.Client{Timeout: httpTimeout} with a
// nil Transport, which falls back to http.DefaultTransport. Swapping
// http.DefaultTransport for a RoundTripper that rewrites every request's
// scheme/host to a local httptest server transparently routes the
// production-built qbo.intuit.com URLs to the test server. No production file
// is touched. The swap is saved/restored via t.Cleanup; no test in this
// package calls t.Parallel, so the global swap is safe.

type localTransport struct {
	scheme, host string
	rt           http.RoundTripper
}

func (l *localTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = l.scheme
	clone.URL.Host = l.host
	clone.Host = "" // derive Host header from the rewritten URL
	return l.rt.RoundTrip(clone)
}

// feedServer is a configurable getInitialData/getTransactions server. It
// records call counts and the last getTransactions query params so tests can
// assert on the walk's request shape.
type feedServer struct {
	*httptest.Server

	accountID string // qboAccountId reported by getInitialData
	numTxn    *int   // numTxnToReview; nil omits the field
	page      func(start, size int) []rawTxn

	mu              sync.Mutex
	initialCalls    int
	txCalls         int
	lastReviewState string
	lastAccountID   string
	lastStartIndex  int
	lastChunkSize   int
}

func newFeedServer(t *testing.T, accountID string, numTxn *int, page func(start, size int) []rawTxn) *feedServer {
	t.Helper()
	fs := &feedServer{accountID: accountID, numTxn: numTxn, page: page}
	fs.Server = httptest.NewServer(http.HandlerFunc(fs.handle))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *feedServer) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/getInitialData"):
		fs.mu.Lock()
		fs.initialCalls++
		fs.mu.Unlock()
		acc := map[string]any{"qboAccountId": fs.accountID}
		if fs.numTxn != nil {
			acc["numTxnToReview"] = *fs.numTxn
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": []any{acc}})
	case strings.HasSuffix(r.URL.Path, "/getTransactions"):
		q := r.URL.Query()
		start, _ := strconv.Atoi(q.Get("startIndex"))
		size, _ := strconv.Atoi(q.Get("chunkSize"))
		items := []rawTxn{}
		if fs.page != nil {
			items = fs.page(start, size)
		}
		fs.mu.Lock()
		fs.txCalls++
		fs.lastReviewState = q.Get("reviewState")
		fs.lastAccountID = q.Get("accountId")
		fs.lastStartIndex = start
		fs.lastChunkSize = size
		fs.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (fs *feedServer) stats() (initial, tx int) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.initialCalls, fs.txCalls
}

// interceptHTTP routes production qbo.intuit.com requests to rawurl by
// swapping http.DefaultTransport. Restored on cleanup.
func interceptHTTP(t *testing.T, rawurl string) {
	t.Helper()
	orig := http.DefaultTransport
	origImpersonated := impersonatedTransport
	u, err := url.Parse(rawurl)
	if err != nil {
		t.Fatal(err)
	}
	rt := &http.Transport{}
	http.DefaultTransport = &localTransport{scheme: u.Scheme, host: u.Host, rt: rt}
	impersonatedTransport = http.DefaultTransport
	t.Cleanup(func() {
		rt.CloseIdleConnections()
		http.DefaultTransport = orig
		impersonatedTransport = origImpersonated
	})
}

// pageOf builds n rawTxn rows whose ids come from idf. Used to synthesize
// full (300-row) and short pages for the walk.
func pageOf(n int, idf func(i int) string) []rawTxn {
	items := make([]rawTxn, n)
	for i := range n {
		items[i] = rawTxn{ID: idf(i), OlbTxnDate: "2026-08-23", Amount: new(float64(i)), Description: "row"}
	}
	return items
}

// --- expectedPending --------------------------------------------------------

// TestExpectedPending reads numTxnToReview for the requested account from
// getInitialData and returns -1 when the account is absent. It rejects
// implementations that return 0 instead of -1 for unknown accounts
// (0 would masquerade as "complete with zero pending").
func TestExpectedPending(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", new(7), nil)
	interceptHTTP(t, fs.URL)

	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if got := ac.expectedPending(context.Background(), "204"); got != 7 {
		t.Fatalf("expectedPending(204) = %d, want 7", got)
	}
	if initial, _ := fs.stats(); initial != 1 {
		t.Fatalf("getInitialData calls = %d, want 1", initial)
	}

	// Absent account: server only reports account 204.
	if got := ac.expectedPending(context.Background(), "999"); got != -1 {
		t.Fatalf("expectedPending(999) = %d, want -1 (absent account)", got)
	}
}

// TestExpectedPendingCountAbsent asserts -1 when the account is present but
// numTxnToReview is omitted. It rejects returning 0 for a present-but-
// countless account.
func TestExpectedPendingCountAbsent(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", nil, nil) // numTxnToReview omitted
	interceptHTTP(t, fs.URL)

	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if got := ac.expectedPending(context.Background(), "204"); got != -1 {
		t.Fatalf("expectedPending = %d, want -1 (count absent)", got)
	}
}

// --- fetchFeedPage ----------------------------------------------------------

// TestFetchFeedPageDecodesItemsAndQuery asserts fetchFeedPage decodes the
// items array and sends the documented query params (reviewState, accountId,
// startIndex, chunkSize). It rejects a swapped startIndex/chunkSize or a
// missing reviewState.
func TestFetchFeedPageDecodesItemsAndQuery(t *testing.T) {
	saveUsable(t)
	wantItems := pageOf(3, func(i int) string { return "row-" + strconv.Itoa(i) })
	fs := newFeedServer(t, "204", nil, func(start, size int) []rawTxn {
		if start == 0 {
			return wantItems
		}
		return nil
	})
	interceptHTTP(t, fs.URL)

	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	page, err := fetchFeedPage(context.Background(), ac, "204", "PENDING", 0, 300)
	if err != nil {
		t.Fatalf("fetchFeedPage: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(page.Items))
	}
	if page.Items[0].ID != "row-0" {
		t.Fatalf("first item id = %q, want row-0", page.Items[0].ID)
	}
	if _, tx := fs.stats(); tx != 1 {
		t.Fatalf("getTransactions calls = %d, want 1", tx)
	}
	fs.mu.Lock()
	if fs.lastReviewState != "PENDING" {
		t.Fatalf("reviewState = %q, want PENDING", fs.lastReviewState)
	}
	if fs.lastAccountID != "204" {
		t.Fatalf("accountId = %q, want 204", fs.lastAccountID)
	}
	if fs.lastStartIndex != 0 || fs.lastChunkSize != 300 {
		t.Fatalf("startIndex/chunkSize = %d/%d, want 0/300", fs.lastStartIndex, fs.lastChunkSize)
	}
	fs.mu.Unlock()
}

// TestFetchFeedPageNon200ReplayError asserts a non-200 response surfaces as a
// *ReplayError carrying the status. It rejects implementations that swallow
// non-200s or return a generic error.
func TestFetchFeedPageNon200ReplayError(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)

	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = fetchFeedPage(context.Background(), ac, "204", "PENDING", 0, 300)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	var re *ReplayError
	if !errors.As(err, &re) {
		t.Fatalf("expected *ReplayError, got %T: %v", err, err)
	}
	if re.Status != http.StatusInternalServerError {
		t.Fatalf("ReplayError.Status = %d, want 500", re.Status)
	}
}

// --- ReplayFeedComplete: full walk ------------------------------------------

func TestReplayFeedLargeLimitUsesPages(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", nil, func(start, size int) []rawTxn {
		if start >= 650 {
			return nil
		}
		n := min(size, 650-start)
		return pageOf(n, func(i int) string { return "row-" + strconv.Itoa(start+i) })
	})
	interceptHTTP(t, fs.URL)

	res, err := ReplayFeed(context.Background(), "204", "pending", 1200)
	if err != nil {
		t.Fatalf("ReplayFeed: %v", err)
	}
	if len(res.Transactions) != 650 || res.Counts["transactions"] != 650 {
		t.Fatalf("paged result = %d, want 650", len(res.Transactions))
	}
	if _, calls := fs.stats(); calls != 3 {
		t.Fatalf("getTransactions calls = %d, want 3", calls)
	}
	if res.Transactions[649].ID != "row-649" {
		t.Fatalf("last id = %q, want row-649", res.Transactions[649].ID)
	}
}

// TestReplayFeedCompleteWalkComplete asserts a short first page ends the walk
// and that the result is marked complete when unique ids == numTxnToReview.
// It rejects implementations that keep paging past a short page or that mark
// a matching count incomplete.
func TestReplayFeedCompleteWalkComplete(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", new(2), func(start, size int) []rawTxn {
		if start == 0 {
			return pageOf(2, func(i int) string { return "t" + strconv.Itoa(i) })
		}
		return nil
	})
	interceptHTTP(t, fs.URL)

	res, err := ReplayFeedComplete(context.Background(), "204", "PENDING", 40)
	if err != nil {
		t.Fatalf("ReplayFeedComplete: %v", err)
	}
	if !res.Complete {
		t.Fatalf("Complete = false, want true (fetched %d, expected %d)", len(res.Transactions), res.Expected)
	}
	if res.Expected != 2 {
		t.Fatalf("Expected = %d, want 2", res.Expected)
	}
	if len(res.Transactions) != 2 {
		t.Fatalf("len(transactions) = %d, want 2", len(res.Transactions))
	}
	if res.AccountID != "204" {
		t.Fatalf("AccountID = %q, want 204", res.AccountID)
	}
	if res.Counts["transactions"] != 2 {
		t.Fatalf("counts[transactions] = %d, want 2", res.Counts["transactions"])
	}
	if initial, tx := fs.stats(); initial != 1 || tx != 1 {
		t.Fatalf("calls: initial=%d tx=%d, want 1/1 (short page stops walk)", initial, tx)
	}
}

// TestReplayFeedCompleteWalkDedup asserts the walk deduplicates by transaction
// id. The server returns a full 300-row page where every row is one of two
// ids; the walk must collapse them to 2 unique rows and, with numTxnToReview=2,
// report complete. It rejects implementations that count raw rows (300) or
// that fail to break after the following short page.
func TestReplayFeedCompleteWalkDedup(t *testing.T) {
	saveUsable(t)
	dupPage := pageOf(300, func(i int) string {
		if i%2 == 0 {
			return "dup-a"
		}
		return "dup-b"
	})
	fs := newFeedServer(t, "204", new(2), func(start, size int) []rawTxn {
		if start == 0 {
			return dupPage
		}
		return nil // short page ends the walk
	})
	interceptHTTP(t, fs.URL)

	res, err := ReplayFeedComplete(context.Background(), "204", "PENDING", 40)
	if err != nil {
		t.Fatalf("ReplayFeedComplete: %v", err)
	}
	if len(res.Transactions) != 2 {
		t.Fatalf("len(transactions) = %d, want 2 after dedup", len(res.Transactions))
	}
	if !res.Complete {
		t.Fatalf("Complete = false, want true (2 unique == expected 2)")
	}
	if initial, tx := fs.stats(); initial != 1 || tx != 1 {
		t.Fatalf("calls: initial=%d tx=%d, want 1/1 (early-stop after oracle satisfied)", initial, tx)
	}
}

// TestReplayFeedCompleteWalkIncomplete is the mismatch negative: the server
// reports 5 pending but only 2 rows exist. The walk must return ErrIncomplete
// (usable via errors.Is) alongside a non-nil result carrying the partial
// evidence. It rejects implementations that silently return a "complete"
// partial result or that return a nil result with the error.
func TestReplayFeedCompleteWalkIncomplete(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", new(5), func(start, size int) []rawTxn {
		if start == 0 {
			return pageOf(2, func(i int) string { return "t" + strconv.Itoa(i) })
		}
		return nil
	})
	interceptHTTP(t, fs.URL)

	res, err := ReplayFeedComplete(context.Background(), "204", "PENDING", 40)
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v, want errors.Is ErrIncomplete", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result carrying partial evidence, got nil")
	}
	if res.Complete {
		t.Fatal("Complete = true, want false (2 != 5)")
	}
	if res.Expected != 5 {
		t.Fatalf("Expected = %d, want 5", res.Expected)
	}
	if len(res.Transactions) != 2 {
		t.Fatalf("len(transactions) = %d, want 2 partial", len(res.Transactions))
	}
}

// TestReplayFeedCompleteNormalizesInputs asserts accountID empty -> "204",
// reviewState is uppercased+trimmed -> "PENDING", and the request query
// reflects both. It rejects implementations that send an empty accountId or
// a lowercased/whitespace reviewState.
func TestReplayFeedCompleteNormalizesInputs(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", new(1), func(start, size int) []rawTxn {
		if start == 0 {
			return pageOf(1, func(i int) string { return "only" })
		}
		return nil
	})
	interceptHTTP(t, fs.URL)

	res, err := ReplayFeedComplete(context.Background(), "", " pending ", 40)
	if err != nil {
		t.Fatalf("ReplayFeedComplete: %v", err)
	}
	if res.AccountID != "204" {
		t.Fatalf("AccountID = %q, want 204 (empty defaulted)", res.AccountID)
	}
	fs.mu.Lock()
	if fs.lastAccountID != "204" {
		t.Fatalf("request accountId = %q, want 204", fs.lastAccountID)
	}
	if fs.lastReviewState != "PENDING" {
		t.Fatalf("request reviewState = %q, want PENDING", fs.lastReviewState)
	}
	fs.mu.Unlock()
}

// TestReplayFeedCompleteMaxPagesBounds asserts maxPages caps the walk: with a
// server that always returns full 300-row pages, maxPages=2 produces exactly
// 2 getTransactions calls and 600 unique rows. It rejects implementations
// that ignore maxPages and page until the hard 40 default.
func TestReplayFeedCompleteMaxPagesBounds(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", nil, func(start, size int) []rawTxn {
		// Always-full pages of unique ids so the walk never short-page-breaks.
		return pageOf(300, func(i int) string {
			return "txn-" + strconv.Itoa(start) + "-" + strconv.Itoa(i)
		})
	})
	interceptHTTP(t, fs.URL)

	res, err := ReplayFeedComplete(context.Background(), "204", "PENDING", 2)
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v, want wrapped ErrIncomplete (oracle unavailable)", err)
	}
	if _, tx := fs.stats(); tx != 2 {
		t.Fatalf("getTransactions calls = %d, want 2 (maxPages bound)", tx)
	}
	if len(res.Transactions) != 600 {
		t.Fatalf("len(transactions) = %d, want 600", len(res.Transactions))
	}
}

// TestReplayFeedCompleteMaxPagesDefault asserts maxPages<1 defaults to 40:
// with always-full pages, the walk makes exactly 40 getTransactions calls
// (12_000 unique rows). It rejects a default of 0 (no pages), 1, or an
// unbounded walk.
func TestReplayFeedCompleteMaxPagesDefault(t *testing.T) {
	saveUsable(t)
	fs := newFeedServer(t, "204", nil, func(start, size int) []rawTxn {
		return pageOf(300, func(i int) string {
			return "txn-" + strconv.Itoa(start) + "-" + strconv.Itoa(i)
		})
	})
	interceptHTTP(t, fs.URL)

	res, err := ReplayFeedComplete(context.Background(), "204", "PENDING", 0)
	if err == nil || !errors.Is(err, ErrIncomplete) {
		t.Fatalf("expected ErrIncomplete (unknown oracle), got err=%v", err)
	}
	if _, tx := fs.stats(); tx != 40 {
		t.Fatalf("getTransactions calls = %d, want 40 (maxPages<1 default)", tx)
	}
	if len(res.Transactions) != 12000 {
		t.Fatalf("len(transactions) = %d, want 12000", len(res.Transactions))
	}
	if res.Complete {
		t.Fatal("Complete = true with unknown oracle — must be false")
	}
}
