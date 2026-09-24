package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExcludeEmptyResponseRequiresReadback(t *testing.T) {
	for _, stuck := range []bool{false, true} {
		t.Run(map[bool]string{false: "observed transition", true: "HTTP 200 but no change"}[stuck], func(t *testing.T) {
			saveUsable(t)
			row := feedRowFixture("9")
			row["qboAccountId"] = "44"
			ms := newMutationServer(t, []map[string]any{row}, nil)
			ms.suppressStateMove = stuck
			interceptHTTP(t, ms.URL)
			err := ReplayExclude(context.Background(), "44", []string{"9"})
			if stuck && !errors.Is(err, ErrStateVerification) {
				t.Fatalf("false success: %v", err)
			}
			if !stuck && err != nil {
				t.Fatal(err)
			}
			if ms.postCount() != 1 {
				t.Fatalf("mutation retried: %d posts", ms.postCount())
			}
		})
	}
}

func TestUnpostVerifiesReportedEntityDeletion(t *testing.T) {
	for _, stillExists := range []bool{false, true} {
		t.Run(map[bool]string{false: "deleted", true: "false deletion receipt"}[stillExists], func(t *testing.T) {
			saveUsable(t)
			posted := false
			posts := 0
			row := feedRowFixture("9")
			row["qboAccountId"] = "44"
			row["matchedQboTxns"] = []any{map[string]any{"qboTxnId": "7", "txnFdmName": "Purchase"}}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/getTransactions"):
					items := []map[string]any{}
					if (!posted && r.URL.Query().Get("reviewState") == "ACCEPTED") || (posted && r.URL.Query().Get("reviewState") == "PENDING") {
						items = append(items, row)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
				case strings.HasSuffix(r.URL.Path, "/query"):
					q := map[string]any{}
					if !posted || stillExists {
						q["Purchase"] = []map[string]any{{"Id": "7", "TotalAmt": 25.0}}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"QueryResponse": q})
				case strings.HasSuffix(r.URL.Path, "/undoTransactions"):
					posts++
					posted = true
					_ = json.NewEncoder(w).Encode(map[string]any{"olbtxns": map[string]any{"success": []any{map[string]any{"olbTxnId": "9", "deletedQboTxns": []any{map[string]any{"qboTxnId": "7"}}}}}})
				default:
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			err := ReplayUnpost(context.Background(), "44", []string{"9"})
			if stillExists && !errors.Is(err, ErrStateVerification) {
				t.Fatalf("false deletion accepted: %v", err)
			}
			if !stillExists && err != nil {
				t.Fatal(err)
			}
			if posts != 1 {
				t.Fatalf("posts=%d", posts)
			}
		})
	}
}

func TestStateAbsenceRejectsMissingItems(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stateRows(context.Background(), ac, "44", "PENDING", []string{"9"}, false); err == nil {
		t.Fatal("missing items treated as proved absence")
	}
}
