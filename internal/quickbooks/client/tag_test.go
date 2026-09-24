package client

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// Tag reads ride tags.api.intuit.com GET tags[/{id}] — the qbo-tags-ui REST
// contract. Writes are retirement-gated server-side (403 NOT_PERMITTED on
// companies that never used tags), so only the read path is wired.

func TestTagReadListFiltersDeleted(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK,
		`[{"id":"1","tagName":"VIP","tagColor":"0077C5","typeId":"9","deleted":false},`+
			`{"id":"2","tagName":"Gone","tagColor":"6B6C72","typeId":"","deleted":true},`+
			`{"id":"3","tagName":"Wholesale","tagColor":"","typeId":"","deleted":false}]`)
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(context.Background(), "Tag", "", "", 20)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %+v, want 2 live tags (deleted filtered)", res.Items)
	}
	if res.Items[0].ID != "1" || res.Items[0].Name != "VIP" || res.Items[0].Type != "Tag" {
		t.Fatalf("item0 = %+v", res.Items[0])
	}
	if res.Items[0].AccountID != "9" {
		t.Fatalf("group id should surface: %+v", res.Items[0])
	}
	_, meth, path, _ := srv.snap()
	if meth != http.MethodGet || !strings.HasSuffix(path, "/tags") {
		t.Fatalf("call = %s %s", meth, path)
	}
}

func TestTagReadByID(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK,
		`{"id":"42","tagName":"Priority","tagColor":"2CA01C","typeId":"","deleted":false}`)
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	res, err := ReplayQuery(context.Background(), "Tag", "42", "", 20)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "42" || res.Items[0].Name != "Priority" {
		t.Fatalf("items = %+v", res.Items)
	}
	_, meth, path, _ := srv.snap()
	if meth != http.MethodGet || !strings.HasSuffix(path, "/tags/42") {
		t.Fatalf("call = %s %s, want GET …/tags/42", meth, path)
	}
}

func TestTagReadByIDNotFound(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusNotFound,
		`{"message":"Tag not found","errorCode":"TAG_NOT_FOUND"}`)
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	_, err := ReplayQuery(context.Background(), "Tag", "999", "", 20)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want 404 surfaced", err)
	}
}

func TestTagReadListError(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusForbidden,
		`{"message":"denied","errorCode":"NOT_PERMITTED"}`)
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	_, err := ReplayQuery(context.Background(), "Tag", "", "", 20)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want 403 surfaced", err)
	}
}

func TestPlannedTagURL(t *testing.T) {
	if got := PlannedQueryURL("Tag", "7"); !strings.Contains(got, "tags.api.intuit.com") {
		t.Fatalf("PlannedURL = %s", got)
	}
}
