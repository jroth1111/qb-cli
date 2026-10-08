package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestMatchCensusesAdvanceByObservedRowsAndProveAbsence(t *testing.T) {
	t.Setenv("QB_CENSUS_PAGE_SIZE", "999")
	saveUsable(t)
	starts := map[string][]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		start, _ := strconv.Atoi(r.URL.Query().Get("startIndex"))
		kind := r.URL.Query().Get("reviewState")
		if strings.Contains(r.URL.Path, "/register/") {
			kind = "register"
			var end int
			_, _ = fmt.Sscanf(r.Header.Get("X-Range"), "items=%d-%d", &start, &end)
		}
		starts[kind] = append(starts[kind], start)
		rows := []map[string]any{}
		if start < 3 {
			// The server caps this response to one row regardless of request size.
			id := fmt.Sprint(start + 1)
			rows = append(rows, map[string]any{"id": id + ":ofx", "olbTxnId": id, "qboAccountId": "204", "txnId": id, "sequence": 0})
		}
		if kind == "register" {
			_ = json.NewEncoder(w).Encode(rows)
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
		}
	}))
	defer server.Close()
	interceptHTTP(t, server.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := registerRowsForAccount(context.Background(), ac, "204")
	if err != nil || len(rows) != 3 {
		t.Fatalf("register rows=%d err=%v", len(rows), err)
	}
	rows, err = stateRows(context.Background(), ac, "204", "PENDING", []string{"99"}, false)
	if err != nil || len(rows) != 0 {
		t.Fatalf("pending absence=%v err=%v", rows, err)
	}
	if err = mappedTargetsUnclaimed(context.Background(), ac, "204", []MatchMapping{{OlbTxnID: "99", MatchID: "99"}}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"register", "PENDING", "ACCEPTED"} {
		if fmt.Sprint(starts[kind]) != "[0 1 2 3]" {
			t.Fatalf("%s skipped capped rows: %v", kind, starts[kind])
		}
	}
}

func TestCensusSizeConfigIsBoundedAndFailsBeforeDial(t *testing.T) {
	saveUsable(t)
	for _, value := range []string{"0", "1000", "-1", "not-a-number"} {
		t.Setenv("QB_CENSUS_PAGE_SIZE", value)
		if _, err := newAPIClient(); err == nil {
			t.Fatalf("accepted invalid page capacity %q", value)
		}
	}
}
