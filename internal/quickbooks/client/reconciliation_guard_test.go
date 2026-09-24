package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMatchRegisterCensusFindsOldRecordPastTenPages(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("X-Range"), "items=%d-%d", &start, &end); err != nil || end-start != 299 {
			t.Errorf("wrong register range %q", r.Header.Get("X-Range"))
			return
		}
		if start == 3300 {
			_, _ = w.Write([]byte(`[{"txnId":"16688","sequence":0,"lineAccountId":204,"clearState":2}]`))
			return
		}
		if start > 3300 {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[`))
		for i := range 300 {
			if i > 0 {
				_, _ = w.Write([]byte(`,`))
			}
			_, _ = fmt.Fprintf(w, `{"txnId":"%d","sequence":0}`, start+i+1)
		}
		_, _ = w.Write([]byte(`]`))
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := registerRowsForAccount(context.Background(), ac, "204")
	if err != nil || len(rows) != 3301 || findRegisterRow(rows, "16688") == nil {
		t.Fatalf("old target lost after ten pages: rows=%d err=%v", len(rows), err)
	}
}

func TestRegisterClearStateProjection(t *testing.T) {
	for _, value := range []string{`0`, `1`, `2`, `"RECONCILED"`, `null`} {
		rows, ok := extractRegisterTxns([]byte(`[{"txnId":7,"clearState":` + value + `}]`))
		if !ok {
			t.Fatal(value)
		}
		got := projectRegister(rows).Transactions[0].ClearState
		want := strings.Trim(value, `"`)
		if value == "null" {
			want = ""
		}
		if got != want {
			t.Fatalf("%s => %q", value, got)
		}
	}
}

func TestUnpostReconciliationRefusesBeforePOST(t *testing.T) {
	for _, state := range []string{`0`, `2`, `3`, `null`, `"RECONCILED"`} {
		t.Run(state, func(t *testing.T) {
			saveUsable(t)
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					posts++
					t.Error("unsafe POST")
					return
				}
				if strings.Contains(r.URL.Path, "/getTransactions") {
					_, _ = w.Write([]byte(`{"items":[{"id":"9:ofx","olbTxnId":"9","qboAccountId":"44","matchedQboTxns":[{"qboTxnId":"7","txnFdmName":"Purchase","clearState":"CLEARED"}]}]}`))
				} else if r.Header.Get("X-Range") == "items=0-299" {
					_, _ = w.Write([]byte(`[{"txnId":"7","lineAccountId":"44","clearState":` + state + `}]`))
				} else {
					_, _ = w.Write([]byte(`[]`))
				}
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			err := ReplayUnpost(context.Background(), "44", []string{"9"})
			if !errors.Is(err, ErrReconciliationUnsafe) || posts != 0 {
				t.Fatalf("posts=%d err=%v", posts, err)
			}
		})
	}
}
