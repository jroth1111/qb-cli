package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// TestRealmPinAllowsAnySessionRealm proves construction applies no company
// gate: even the production-company session constructs (every command
// operates on the login session's company). There is no allowlist.
func TestRealmPinAllowsAnySessionRealm(t *testing.T) {
	for _, realm := range []string{"12345", "99999"} {
		saveRealm(t, realm)
		if _, err := newAPIClient(); err != nil {
			t.Fatalf("realm %s rejected: %v", realm, err)
		}
	}
}

// TestRealmPinAllowsSessionRealm proves the login session's own company
// passes construction: any non-production realm is accepted, so the CLI
// works with whatever company the user signed in as.
func TestRealmPinAllowsSessionRealm(t *testing.T) {
	saveRealm(t, "12345")
	if _, err := newAPIClient(); err != nil {
		t.Fatalf("session realm rejected: %v", err)
	}
}

func newCountingServer(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(nilHandler(calls))
	return srv
}

func nilHandler(calls *int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { *calls++ })
	return mux
}

// TestReplayFeedCompleteUnknownOracleNotComplete proves that when the server
// omits numTxnToReview (expected=-1), the walk is NOT silently marked
// complete — it returns ErrIncomplete with Complete=false.
func TestReplayFeedCompleteUnknownOracleNotComplete(t *testing.T) {
	saveRealm(t, "12345")

	// getInitialData omits numTxnToReview entirely; getTransactions returns 2 rows (short page)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getInitialData") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": []any{map[string]any{"qboAccountId": "44"}}, // no numTxnToReview key
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/getTransactions") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{"id": "a:ofx", "olbTxnId": "a", "amount": -1.0},
					map[string]any{"id": "b:ofx", "olbTxnId": "b", "amount": -2.0},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)

	res, err := ReplayFeedComplete(context.Background(), "44", "PENDING", 40)
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("err = %v, want wrapped ErrIncomplete", err)
	}
	if res.Complete {
		t.Error("Complete = true with unknown oracle — must be false")
	}
	if res.Expected != -1 {
		t.Errorf("Expected = %d, want -1", res.Expected)
	}
	if len(res.Transactions) != 2 {
		t.Errorf("Transactions = %d, want 2 (rows still returned)", len(res.Transactions))
	}
}

// TestReplayFeedCompleteZeroExpectedComplete verifies that expected==0 with
// zero fetched rows IS treated as verified-complete (not incomplete).
func TestReplayFeedCompleteZeroExpectedComplete(t *testing.T) {
	saveRealm(t, "12345")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getInitialData") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accounts": []any{map[string]any{"qboAccountId": "44", "numTxnToReview": 0}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)

	res, err := ReplayFeedComplete(context.Background(), "44", "PENDING", 40)
	if err != nil {
		t.Fatalf("err = %v, want nil for empty population", err)
	}
	if !res.Complete {
		t.Error("Complete = false for expected==0 && fetched==0")
	}
}

// saveRealm isolates QB_HOME and saves a TokenSet for the given realm.
func saveRealm(t *testing.T, realm string) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       realm,
		Cookies:       []auth.Cookie{{Name: "qbo.csrftoken", Value: "v"}},
	}); err != nil {
		t.Fatal(err)
	}
}
