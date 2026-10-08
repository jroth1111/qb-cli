package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestUnboundedFeedCensusRestartsWithoutCombiningAttempts(t *testing.T) {
	for _, mode := range []string{"count-drift", "changed-duplicate", "identical-duplicate", "persistent-shortage"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("QB_HTTP_TRANSPORT", "")
			saveUsable(t)
			walks := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatal("census retry wrote")
				}
				start, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
				if start == 0 {
					walks++
				}
				rows := []any{}
				total := 1
				id := fmt.Sprintf("walk-%d:ofx", walks)
				amount := -10.0
				include := start == 0
				if mode == "persistent-shortage" || mode == "count-drift" && walks == 1 {
					total = 2
				}
				if (mode == "changed-duplicate" && walks == 1 || mode == "identical-duplicate") && start == 1 {
					include = true
					if mode == "changed-duplicate" {
						amount = -11
					}
				}
				if include {
					rows = append(rows, map[string]any{"id": id, "qboAccountId": "204", "amount": amount, "olbTxnDate": "2020-01-01T10:00:00+10:00", "origDescription": "fixture"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"items": rows, "totalTransactionsCount": total})
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			result, err := ReplayFeed(context.Background(), "204", "PENDING", 0)
			if mode == "persistent-shortage" {
				if !errors.Is(err, ErrIncomplete) || walks != 3 || result == nil || result.Complete {
					t.Fatalf("short census accepted or retried without bound: walks=%d result=%+v err=%v", walks, result, err)
				}
				return
			}
			wantWalks := 2
			if mode == "identical-duplicate" {
				wantWalks = 1
			}
			if err != nil || !result.Complete || len(result.Transactions) != 1 || walks != wantWalks || result.Transactions[0].ID != fmt.Sprintf("walk-%d:ofx", wantWalks) {
				t.Fatalf("attempts combined or duplicate mishandled: walks=%d result=%+v err=%v", walks, result, err)
			}
		})
	}
}

func TestFeedCensusAdvancesByReturnedWireRows(t *testing.T) {
	t.Setenv("QB_HTTP_TRANSPORT", "")
	saveUsable(t)
	fs := newFeedServer(t, "204", nil, func(start, size int) []rawTxn {
		if start >= 650 {
			return nil
		}
		return pageOf(min(size, 250, 650-start), func(i int) string { return "row-" + strconv.Itoa(start+i) })
	})
	total := 650
	fs.totalTxn = &total
	interceptHTTP(t, fs.URL)
	result, err := ReplayFeed(context.Background(), "204", "PENDING", 0)
	if err != nil || !result.Complete || len(result.Transactions) != total || result.Transactions[649].ID != "row-649" {
		t.Fatalf("capped response skipped records: result=%+v err=%v", result, err)
	}
	if _, calls := fs.stats(); calls != 4 {
		t.Fatalf("unexpected page calls=%d", calls)
	}
}

func TestFeedCensusCancellationRefusesNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReplayFeed(ctx, "204", "PENDING", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
