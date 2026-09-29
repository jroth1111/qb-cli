//go:build live

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLiveDumpEntities pages a raw v3 entity query and writes each page body
// to $QB_DUMP_DIR/<entity>-<start>.json. Read-only; used for offline audit
// joins. Requires QB_LIVE_PROBE=1 and QB_EXPECTED_COMPANY.
func TestLiveDumpEntities(t *testing.T) {
	if os.Getenv("QB_LIVE_PROBE") != "1" {
		t.Skip("explicit live dump only")
	}
	dir := os.Getenv("QB_DUMP_DIR")
	entity := os.Getenv("QB_DUMP_ENTITY")
	where := os.Getenv("QB_DUMP_WHERE")
	if dir == "" || entity == "" {
		t.Skip("QB_DUMP_DIR and QB_DUMP_ENTITY required")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if want := os.Getenv("QB_EXPECTED_COMPANY"); want == "" || c.tok.CompanyName != want {
		t.Fatal("expected company not verified")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	pageSize := 1000
	if v := os.Getenv("QB_DUMP_PAGESIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			pageSize = n
		}
	}
	page := 1
	for start := 1; ; start += pageSize {
		stmt := fmt.Sprintf("select * from %s", entity)
		if where != "" {
			stmt += " where " + where
		}
		stmt += fmt.Sprintf(" orderby Id startposition %d maxresults %d", start, pageSize)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/query?minorversion=73&query=%s",
			c.realm, url.QueryEscape(stmt))
		resp, err := c.getJSON(ctx, u, "")
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		body, err := readBody(resp)
		_ = drainAndClose(resp)
		cancel()
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		out := filepath.Join(dir, fmt.Sprintf("%s-%d.json", entity, start))
		if err := os.WriteFile(out, body, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("page %d start=%d bytes=%d → %s", page, start, len(body), out)
		var probe struct {
			QueryResponse map[string]json.RawMessage `json:"QueryResponse"`
		}
		if err := json.Unmarshal(body, &probe); err != nil {
			t.Fatalf("page %d: envelope: %v", page, err)
		}
		got := 0
		for k, v := range probe.QueryResponse {
			if strings.EqualFold(k, entity) {
				var objs []json.RawMessage
				if json.Unmarshal(v, &objs) == nil {
					got = len(objs)
				}
			}
		}
		if got < pageSize {
			t.Logf("done: last page had %d entities", got)
			break
		}
		page++
	}
}
