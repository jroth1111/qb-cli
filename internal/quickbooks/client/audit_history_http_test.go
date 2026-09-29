package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditHistoricalRequestAndPaginationMetadata(t *testing.T) {
	saveUsableAudit(t)
	var body map[string]any
	var authorization string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method=%s", r.Method)
		}
		authorization = r.Header.Get("Authorization")
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
	if authorization != "Intuit_APIKey intuit_apikey=audit,intuit_apikey_version=1.0" {
		t.Fatalf("audit authorization = %q, want audit-ui key rather than banking key", authorization)
	}
	entities, ok := body["entities"].([]any)
	if !ok || len(entities) != 64 {
		t.Fatalf("audit UI entity inventory = %T/%d, want 64 entities", body["entities"], len(entities))
	}
	if result.PageInfo == nil || result.PageInfo.TotalLogs != 96 || result.PageInfo.StartIndex != 50 || result.Events[0].EntityID != "t1" {
		t.Fatalf("lost pagination or lineage: %+v", result)
	}
}

func TestReplayAuditFailsClosedWithoutAuditUICredential(t *testing.T) {
	saveUsable(t)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	_, err := ReplayAuditLog(context.Background(), 20)
	if err == nil || !strings.Contains(err.Error(), "missing Audit Log UI authorization") {
		t.Fatalf("error = %v, want missing audit-ui authorization", err)
	}
	if calls != 0 {
		t.Fatalf("sent %d request(s) without audit-ui authorization", calls)
	}
}

func TestAuditUnknownResponseFails(t *testing.T) {
	saveUsableAudit(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"unexpected":"shape"}`)) }))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	if _, err := ReplayAuditLog(context.Background(), 20); err == nil {
		t.Fatal("unknown HTTP-200 response must not look like empty audit history")
	}
}

func TestAuditSearchPaginatesBeyondFirstPage(t *testing.T) {
	saveUsableAudit(t)
	var offsets []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		offset := int(body["offset"].(float64))
		offsets = append(offsets, offset)
		logs := make([]map[string]any, 0, 50)
		if offset == 0 {
			for i := range 50 {
				logs = append(logs, map[string]any{"what": map[string]any{"qboAuditId": fmt.Sprintf("login-%d", i), "eventType": "LOGIN", "defaultEventDescription": "Login", "entityType": "audit_info"}})
			}
		} else {
			logs = append(logs, map[string]any{"what": map[string]any{"qboAuditId": "kmart-event", "eventType": "ADD", "defaultEventDescription": "Kmart Burwood purchase", "entityType": "Purchase", "entity": map[string]any{"ELEMENTID": "purchase-1"}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"logs": logs, "pageInfo": map[string]any{"totalLogs": 51, "startIndex": offset, "size": len(logs)}})
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	result, err := ReplayAuditLogFiltered(context.Background(), AuditFilter{Limit: 10, Query: "Kmart Burwood", FromDate: "2023-01-01T00:00:00Z", ToDate: "2024-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 2 || offsets[0] != 0 || offsets[1] != 50 {
		t.Fatalf("searched offsets %v, want [0 50]", offsets)
	}
	if len(result.Events) != 1 || result.Events[0].ID != "kmart-event" || result.Events[0].EntityID != "purchase-1" {
		t.Fatalf("search results = %+v", result.Events)
	}
}
