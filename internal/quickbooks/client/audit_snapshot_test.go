package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditSnapshotOptInPreservesEvidenceAfterFiltering(t *testing.T) {
	const snapshot = `{"id":"42","memo":"original fee","amount":1.9700,"sequence":9007199254740993,"lines":[{"ofxMatchInformation":{"ofxTransactionId":"17"}}]}`
	for _, include := range []bool{false, true} {
		t.Run(map[bool]string{false: "summary", true: "snapshot"}[include], func(t *testing.T) {
			saveUsableAudit(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["entityId"] != "42" {
					t.Errorf("missing server entity filter: %v", body["entityId"])
				}
				_, _ = w.Write([]byte(`{"logs":[{"what":{"qboAuditId":"a0","entity":{"ELEMENTID":"42","TRANSACTION":{"memo":"earlier"}}}},{"what":{"qboAuditId":"a1","entity":{"ELEMENTID":"42","CONTEXT_INFORMATION":"do not expose","TRANSACTION":` + snapshot + `}}}],"pageInfo":{"totalLogs":2,"startIndex":0,"size":50}}`))
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			result, err := ReplayAuditLogFiltered(context.Background(), AuditFilter{EntityID: "42", IncludeSnapshots: include, Query: "a1", Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Events) != 1 || result.Events[0].ID != "a1" || result.PageInfo.TotalLogs != 2 {
				t.Fatalf("filter or pagination changed: %+v", result)
			}
			got := string(result.Events[0].TransactionSnapshot)
			if include && got != snapshot {
				t.Fatalf("historical snapshot changed: %s", got)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if !include && strings.Contains(string(encoded), "transactionSnapshot") {
				t.Fatal("default output must omit snapshots")
			}
			if strings.Contains(string(encoded), "CONTEXT_INFORMATION") || strings.Contains(string(encoded), "do not expose") {
				t.Fatal("non-transaction audit context leaked")
			}
		})
	}
}

func TestAuditSnapshotAndEntityMismatchFailClosed(t *testing.T) {
	for _, tc := range []struct{ name, entity, snapshot string }{
		{"wrong entity", "99", `{}`},
		{"invalid snapshot", "42", `"unexpected"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saveUsable(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"logs":[{"what":{"entity":{"ELEMENTID":"` + tc.entity + `","TRANSACTION":` + tc.snapshot + `}}}]}`))
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			if _, err := ReplayAuditLogFiltered(context.Background(), AuditFilter{EntityID: "42", IncludeSnapshots: true}); err == nil {
				t.Fatal("invalid historical evidence must fail")
			}
		})
	}
}
