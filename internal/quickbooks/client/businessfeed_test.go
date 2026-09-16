package client

// businessfeed_test.go pins the residual-audit-11 D3 fix:
// projectBusinessFeedItems must preserve the secret-free fields each raw
// item actually carries as QueryItem rows, report counts.items as the number
// preserved, and retain totalCount. Malformed bodies stay an explicit
// unparsed/empty result, and unmapped fields (headers, tokens, nested
// blobs) never reach the envelope.
//
// Rejects: count-only projection that drops populated rows, echoing raw
// item JSON, and silent success on unparseable responses.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProjectBusinessFeedItemsPreservesRows(t *testing.T) {
	res := projectBusinessFeedItems([]byte(`{"items":[
		{"id":"a","title":"Review 3 transactions","type":"BANK_TXN","date":"2026-09-01","amount":12.50},
		{"id":"b","name":"Mileage","createdDate":"2026-08-30"},
		"not-an-object",
		42
	],"totalCount":5}`))
	if res.Entity != "BusinessFeed" {
		t.Fatalf("entity=%q, want BusinessFeed", res.Entity)
	}
	if got := res.Counts["items"]; got != 2 {
		t.Fatalf("counts.items=%d, want 2 preserved rows", got)
	}
	if got := res.Counts["totalCount"]; got != 5 {
		t.Fatalf("counts.totalCount=%d, want 5", got)
	}
	if len(res.Items) != 2 {
		t.Fatalf("len(items)=%d, want 2", len(res.Items))
	}
	it := res.Items[0]
	if it.ID != "a" || it.Name != "Review 3 transactions" || it.Type != "BANK_TXN" || it.Date != "2026-09-01" || it.Amount != 12.5 {
		t.Fatalf("item0 = %+v", it)
	}
	it = res.Items[1]
	if it.ID != "b" || it.Name != "Mileage" || it.Date != "2026-08-30" {
		t.Fatalf("item1 = %+v", it)
	}
}

func TestProjectBusinessFeedItemsDefaultsTotalCountToLength(t *testing.T) {
	res := projectBusinessFeedItems([]byte(`{"items":[{"id":"a"},{"id":"b"},{"id":"c"}]}`))
	if res.Counts["totalCount"] != 3 || res.Counts["items"] != 3 {
		t.Fatalf("counts=%v, want items=3 totalCount=3", res.Counts)
	}
}

func TestProjectBusinessFeedItemsMalformedStaysUnparsed(t *testing.T) {
	res := projectBusinessFeedItems([]byte(`{not json`))
	if len(res.Items) != 0 || res.Counts["items"] != 0 {
		t.Fatalf("malformed body must yield zero items, got %+v", res)
	}
	if !strings.Contains(res.Note, "unparsed") {
		t.Fatalf("note=%q must mark the result unparsed", res.Note)
	}
	if strings.Contains(res.Note, "not json") {
		t.Fatalf("note echoes the raw body: %q", res.Note)
	}
}

func TestProjectBusinessFeedItemsEmptyArray(t *testing.T) {
	res := projectBusinessFeedItems([]byte(`{"items":[]}`))
	if res.Counts["items"] != 0 || res.Counts["totalCount"] != 0 || len(res.Items) != 0 {
		t.Fatalf("empty items must yield zero counts, got %+v", res)
	}
}

// TestProjectBusinessFeedItemsDropsUnmappedFields proves nothing outside the
// QueryItem surface survives projection — headers, tokens and arbitrary
// nested payloads in a raw item cannot leak into the envelope.
func TestProjectBusinessFeedItemsDropsUnmappedFields(t *testing.T) {
	res := projectBusinessFeedItems([]byte(`{"items":[{
		"id":"a","title":"ok",
		"authorization":"Bearer tok-123","intuit_apikey":"sekrit",
		"nested":{"session":"abc"},"headers":{"x":"y"}}]}`))
	if res.Items[0].ID != "a" || res.Items[0].Name != "ok" {
		t.Fatalf("mapped fields lost: %+v", res.Items[0])
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"tok-123", "sekrit", "session", "headers", "Bearer"} {
		if strings.Contains(string(raw), s) {
			t.Fatalf("envelope leaks unmapped field %q: %s", s, raw)
		}
	}
}

// TestReplayBusinessFeedItemsProjectsRows drives the full client path against
// the package-standard transport seam: the POST to /v1/business-feed/items
// must come back as populated QueryItem rows, not a bare count.
func TestReplayBusinessFeedItemsProjectsRows(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"items":[{"id":"a","title":"x","type":"BANK_TXN"},{"id":"b"}],"totalCount":2}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(context.Background(), "BusinessFeed", "", "", 20)
	if err != nil {
		t.Fatalf("BusinessFeed replay: %v", err)
	}
	if res.Status != http.StatusOK || res.Entity != "BusinessFeed" {
		t.Fatalf("envelope = %+v", res)
	}
	if len(res.Items) != 2 || res.Items[0].ID != "a" || res.Items[0].Name != "x" || res.Items[0].Type != "BANK_TXN" {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Counts["items"] != 2 || res.Counts["totalCount"] != 2 {
		t.Fatalf("counts = %v", res.Counts)
	}
	calls, meth, path, _ := srv.snap()
	if calls != 1 || meth != http.MethodPost || !strings.HasSuffix(path, "/v1/business-feed/items") {
		t.Fatalf("call = %d %s %s, want single POST /v1/business-feed/items", calls, meth, path)
	}
}
