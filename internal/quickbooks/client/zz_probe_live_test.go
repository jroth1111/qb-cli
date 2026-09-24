//go:build live

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
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
	requireLiveProbeWrite(t)
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

// TestLiveNeoPost posts QB_PROBE_BODY to the neo path in QB_PROBE_NEO
// (relative to /api/neo/v1/company/{realm}) for one-off first-party probes.
func TestLiveNeoPost(t *testing.T) {
	requireLiveProbeWrite(t)
	path := os.Getenv("QB_PROBE_NEO")
	if path == "" {
		t.Skip("set QB_PROBE_NEO")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	u := "https://qbo.intuit.com/api/neo/v1/company/" + c.realm + "/" + path
	if strings.HasPrefix(path, "https://") {
		u = path // absolute first-party URL (e.g. /api/v4/graphql)
	}
	if err := validateIntuitDestination(u); err != nil {
		t.Fatal(err)
	}
	var resp *http.Response
	if os.Getenv("QB_PROBE_NEO_URIHOST") != "" {
		method := os.Getenv("QB_PROBE_NEO_METHOD")
		if method == "" {
			method = http.MethodPost
		}
		resp, err = c.doURIHost(ctx, method, u, "qbo.intuit.com", []byte(os.Getenv("QB_PROBE_BODY")))
	} else {
		resp, err = c.postJSON(ctx, u, []byte(os.Getenv("QB_PROBE_BODY")))
	}
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

// TestLiveGQLOp posts QB_PROBE_VARS (JSON object, may be empty) to the
// catalogued operation QB_PROBE_OP at its captured endpoint via doURIHost,
// writing the raw response to QB_PROBE_OUT. Read or mutation depends on the
// op — callers choose deliberately.
func TestLiveGQLOp(t *testing.T) {
	opName := os.Getenv("QB_PROBE_OP")
	if opName == "" {
		t.Skip("set QB_PROBE_OP")
	}
	op, err := gql.Lookup(opName)
	if err != nil {
		t.Fatal(err)
	}
	if op.Kind != "query" {
		requireLiveProbeWrite(t)
	}
	vars := map[string]any{}
	if raw := os.Getenv("QB_PROBE_VARS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &vars); err != nil {
			t.Fatal(err)
		}
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": op.Name,
		"variables":     vars,
		"query":         op.Document,
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url := op.Endpoint
	if u := os.Getenv("QB_PROBE_URL"); u != "" {
		url = u
	}
	resp, err := c.doURIHost(ctx, http.MethodPost, url, hostOf(url), payload)
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

// TestLiveURIGet GETs the absolute URL in QB_PROBE_URL via doURIHost against
// the URL's own host — one-off probes for captured REST paths on harvested
// api.intuit.com services.
func TestLiveURIGet(t *testing.T) {
	raw := os.Getenv("QB_PROBE_URL")
	if raw == "" {
		t.Skip("set QB_PROBE_URL")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.doURIHost(ctx, http.MethodGet, raw, hostOf(raw), nil)
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
	// An arbitrary document is not proven read-only by its HTTP verb.
	requireLiveProbeWrite(t)
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
	u := "https://" + host + "/graphql"
	if alt := os.Getenv("QB_PROBE_URL"); alt != "" {
		u = alt
	}
	resp, err := c.doURIHost(ctx, http.MethodPost, u, host, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d bytes=%d body=%.800s", resp.StatusCode, len(body), body)
	if out := os.Getenv("QB_PROBE_OUT"); out != "" {
		if err := os.WriteFile(out, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLiveURIDo performs QB_PROBE_METHOD (default GET) on QB_PROBE_URL with
// optional QB_PROBE_BODY via doURIHost, writing the raw response to
// QB_PROBE_OUT. Generic write/read probe for non-v3 service hosts.
func TestLiveURIDo(t *testing.T) {
	raw := os.Getenv("QB_PROBE_URL")
	if raw == "" {
		t.Skip("set QB_PROBE_URL")
	}
	method := os.Getenv("QB_PROBE_METHOD")
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		requireLiveProbeWrite(t)
	}
	var body []byte
	if b := os.Getenv("QB_PROBE_BODY"); b != "" {
		body = []byte(b)
	}
	var overrides map[string]string
	if h := os.Getenv("QB_PROBE_HEADERS"); h != "" {
		if err := json.Unmarshal([]byte(h), &overrides); err != nil {
			t.Fatalf("QB_PROBE_HEADERS: %v", err)
		}
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.doURIHost(ctx, method, raw, hostOf(raw), body, overrides)
	if err != nil {
		t.Fatal(err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	out, err := readBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d bytes=%d body=%.1200s", resp.StatusCode, len(out), out)
	if resp.StatusCode >= 400 {
		t.Logf("headers=%v", resp.Header)
	}
	if dst := os.Getenv("QB_PROBE_OUT"); dst != "" {
		if err := os.WriteFile(dst, out, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func requireLiveProbeWrite(t *testing.T) {
	t.Helper()
	if os.Getenv("QB_LIVE_WRITE") != "1" {
		t.Skip("live write probes require explicit QB_LIVE_WRITE=1")
	}
}
