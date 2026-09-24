package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuditHistoricalRequestAndPaginationMetadata(t *testing.T) {
	saveUsable(t)
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method=%s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"logs":[{"what":{"qboAuditId":"a1","entity":{"ELEMENTID":"t1"}}}],"pageInfo":{"totalLogs":96,"startIndex":50,"size":50}}`))
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	result, err := ReplayAuditLogFiltered(context.Background(), AuditFilter{Limit: 50, Offset: 50, FromDate: "2026-07-31T00:00:00+10:00", ToDate: "2026-07-31T23:59:59.999+10:00"})
	if err != nil {
		t.Fatal(err)
	}
	if body["offset"] != float64(50) || body["fromDate"] != "2026-07-30T14:00:00Z" || body["toDate"] != "2026-07-31T13:59:59.999Z" {
		t.Fatalf("wrong request fields: offset=%v from=%v to=%v", body["offset"], body["fromDate"], body["toDate"])
	}
	if result.PageInfo == nil || result.PageInfo.TotalLogs != 96 || result.PageInfo.StartIndex != 50 || result.Events[0].EntityID != "t1" {
		t.Fatalf("lost pagination or lineage: %+v", result)
	}
}

func TestAuditUnknownResponseFails(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"unexpected":"shape"}`)) }))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	if _, err := ReplayAuditLog(context.Background(), 20); err == nil {
		t.Fatal("unknown HTTP-200 response must not look like empty audit history")
	}
}
