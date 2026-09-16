//go:build live

package client

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// TestLiveRawQuery runs an arbitrary read-only v3 SQL query supplied via
// QB_PROBE_SQL and writes the raw response to QB_PROBE_OUT (or the log).
func TestLiveRawQuery(t *testing.T) {
	sql := os.Getenv("QB_PROBE_SQL")
	if sql == "" {
		t.Skip("set QB_PROBE_SQL")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.getJSON(ctx, "https://qbo.intuit.com/api/v3/company/"+c.realm+"/query?minorversion=73&query="+url.QueryEscape(sql), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d bytes=%d", resp.StatusCode, len(body))
	if out := os.Getenv("QB_PROBE_OUT"); out != "" {
		if err := os.WriteFile(out, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Log(string(body))
}

// TestLiveRawPost posts QB_PROBE_BODY to the v3 path in QB_PROBE_PATH for
// one-off endpoint verification; writes the raw response to QB_PROBE_OUT.
func TestLiveRawPost(t *testing.T) {
	path := os.Getenv("QB_PROBE_PATH")
	if path == "" {
		t.Skip("set QB_PROBE_PATH")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	u := "https://qbo.intuit.com/api/v3/company/" + c.realm + "/" + path + "?minorversion=73"
	resp, err := c.postJSON(ctx, u, []byte(os.Getenv("QB_PROBE_BODY")))
	if err != nil {
		t.Fatal(err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d bytes=%d", resp.StatusCode, len(body))
	if out := os.Getenv("QB_PROBE_OUT"); out != "" {
		if err := os.WriteFile(out, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Log(string(body))
}

// TestLiveGQLHost posts QB_PROBE_GQL (JSON {query,variables}) to the
// QB_PROBE_HOST api.intuit.com host via doURIHost for one-off host probes.
func TestLiveGQLHost(t *testing.T) {
	host := os.Getenv("QB_PROBE_HOST")
	raw := os.Getenv("QB_PROBE_GQL")
	if host == "" || raw == "" {
		t.Skip("set QB_PROBE_HOST and QB_PROBE_GQL")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.doURIHost(ctx, http.MethodPost, "https://"+host+"/graphql", host, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d bytes=%d body=%.800s", resp.StatusCode, len(body), body)
}
