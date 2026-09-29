//go:build live

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveFolioRenderProbe discovers whether the universalreportinsights
// folio service exposes a generate/render/export endpoint. Read-only: GET
// probes only — a real render POST is never attempted. Requires
// QB_LIVE_PROBE=1 and QB_EXPECTED_COMPANY.
//
// Result (2026-09-29, Mega Style Apartments): every subpath under
// /v1/folio/{id} — render, generate, preview, export, pdf, print, and a
// garbage control — returns the identical 400/errorCode 500004 route-miss.
// format=pdf / format=RENDERED params on the folio GET are ignored (plain
// folio JSON). Conclusion: this service is CRUD-only; statement rendering
// runs through a different pipeline, so stored-pin-vs-def-macro at
// generation time is not observable here — `management audit` replaying
// pages per explicit month remains the correct settlement.
func TestLiveFolioRenderProbe(t *testing.T) {
	if os.Getenv("QB_LIVE_PROBE") != "1" {
		t.Skip("explicit live probe only")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if want := os.Getenv("QB_EXPECTED_COMPANY"); want == "" || c.tok.CompanyName != want {
		t.Fatal("expected company not verified")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// List folios; pick the first FOLIO (management report) id.
	resp, err := c.doURIHost(ctx, http.MethodGet,
		"https://universalreportinsights.api.intuit.com/v1/folio?locale=en-au",
		"universalreportinsights.api.intuit.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("folio list: status=%d err=%v body=%.200s", resp.StatusCode, err, raw)
	}
	var list []map[string]any
	if json.Unmarshal(raw, &list) != nil {
		var wrap map[string]any
		if json.Unmarshal(raw, &wrap) != nil {
			t.Fatalf("folio list shape: %.300s", raw)
		}
		for _, v := range wrap {
			if arr, ok := v.([]any); ok {
				for _, e := range arr {
					if m, ok := e.(map[string]any); ok {
						list = append(list, m)
					}
				}
			}
		}
	}
	var id string
	for _, f := range list {
		if fmt.Sprint(f["reportKindEnum"]) == "FOLIO" && fmt.Sprint(f["active"]) != "false" {
			id = fmt.Sprint(f["id"])
			t.Logf("probe folio: id=%s name=%v", id, f["name"])
			break
		}
	}
	if id == "" {
		t.Fatalf("no FOLIO in list response: %.400s", raw)
	}

	// Candidate render/generate paths — GET only, log status + content-type.
	base := "https://universalreportinsights.api.intuit.com/v1/folio/" + id
	candidates := []string{
		base + "/render?locale=en-au",
		base + "/generate?locale=en-au",
		base + "/preview?locale=en-au",
		base + "/export?locale=en-au",
		base + "/pdf?locale=en-au",
		base + "/print?locale=en-au",
		base + "?locale=en-au&format=pdf",
		base + "?locale=en-au&format=RENDERED",
		// Control: a provably nonexistent subpath — if this also yields 400
		// (not 404), the subpath 400s above are route-misses, not real routes.
		base + "/definitely-not-a-real-path?locale=en-au",
		// Control: nonexistent folio id — distinguishes route-miss from
		// resource-miss semantics on this service.
		"https://universalreportinsights.api.intuit.com/v1/folio/sbg:00000000-0000-0000-0000-000000000000/render?locale=en-au",
	}
	for _, u := range candidates {
		r2, err := c.doURIHost(ctx, http.MethodGet, u, "universalreportinsights.api.intuit.com", nil)
		if err != nil {
			t.Logf("%-70s dial=%v", strings.TrimPrefix(u, base), err)
			continue
		}
		b, _ := readBody(r2)
		ct := r2.Header.Get("Content-Type")
		_ = drainAndClose(r2)
		t.Logf("%-70s → %d %s (%.80s)", strings.TrimPrefix(u, base), r2.StatusCode, ct, b)
	}
}
