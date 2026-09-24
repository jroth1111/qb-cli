package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWeeklyReviewRejectsUncorrelatedReclassifyReceipt(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `[]`, `{"items":[],"ok":true}`, `[{"txnId":999,"sequence":1,"result":"Success"}]`} {
		t.Run(raw, func(t *testing.T) {
			reclassifyServerResponse(t, raw)
			res, err := ReplayReclassify(context.Background(), "account", "17", "7", "2026-08-01", "2026-08-31", "accrual")
			if err == nil {
				t.Fatalf("uncorrelated receipt accepted for submitted IDs 13 and 5: result=%+v", res)
			}
		})
	}
}

func TestReclassifyRejectsInvalidScopeBeforeCredentials(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	for _, dates := range [][2]string{{"bad", "2026-09-20"}, {"2026-09-20", ""}, {"2026-09-21", "2026-09-20"}} {
		if _, err := ReplayReclassify(context.Background(), "account", "1", "2", dates[0], dates[1], "accrual"); err == nil || !strings.Contains(err.Error(), "valid ordered") {
			t.Errorf("invalid scope err=%v", err)
		}
	}
}

func TestReclassifySourceMustBeRecognizedAndEditable(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantError  bool
		count      int
	}{
		{"unknown", `{}`, true, 0},
		{"missing version", `{"items":[{"txnId":1,"sequence":1,"typeId":54,"editable":true}]}`, true, 0},
		{"not editable", `{"items":[{"txnId":1,"sequence":1,"editSequence":2,"typeId":54,"editable":false}]}`, false, 0},
		{"tax line", `{"items":[{"txnId":1,"sequence":1,"editSequence":2,"typeId":54,"editable":true,"isTaxLine":true}]}`, false, 0},
		{"empty", `{"items":[]}`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveUsableURIHost(t)
			srv := newDomServer(t, 200, tc.body)
			interceptHTTP(t, srv.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			rows, err := reclassifyTxnList(context.Background(), ac, "1", "2026-09-01", "2026-09-20", "accrual")
			if (err != nil) != tc.wantError || len(rows) != tc.count {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
		})
	}
}

func TestWeeklyReviewShortPageDoesNotProveAbsence(t *testing.T) {
	saveUsable(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		id := "other"
		if r.URL.Query().Get("startIndex") != "0" {
			id = "9"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": id + ":ofx", "olbTxnId": id, "qboAccountId": "44"}}, "totalTransactionsCount": 301})
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := stateRows(context.Background(), ac, "44", "PENDING", []string{"9"}, false)
	if err == nil && len(rows) == 0 {
		t.Fatalf("claimed absence after %d short nonterminal page(s)", calls)
	}
}
