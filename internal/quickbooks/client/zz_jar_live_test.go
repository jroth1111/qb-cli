//go:build live

package client

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// TestLiveJarHost runs the ego capture against QB_JAR_PAGE (an authenticated
// /app/* URL that fires the target service host) and merges the harvested
// per-host Intuit_APIKey headers into TokenSet.URIHostHeaders. One-off jar
// top-up for hosts a page owns that /app/banking never reaches.
func TestLiveJarHost(t *testing.T) {
	page := os.Getenv("QB_JAR_PAGE")
	if page == "" {
		t.Skip("set QB_JAR_PAGE to an authenticated /app/* URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cap, err := auth.CaptureATSFromEgo(ctx, "", page)
	if err != nil {
		t.Fatal(err)
	}
	for host := range cap.HostHeaders {
		t.Logf("captured host %s", host)
	}
	tok, err := auth.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := tok.ApplyATSCapture(cap); err != nil {
		t.Fatal(err)
	}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
	t.Log("merged into jar")
}

// TestLiveTaxGQL posts QB_PROBE_GQL (a JSON {query,variables} body) to the
// taxconfig GraphQL endpoint through the captured tax SPA transport
// (wrap-ATS + tax Referer). One-off op-shape probes; caller picks read or
// mutation documents deliberately.
func TestLiveTaxGQL(t *testing.T) {
	raw := os.Getenv("QB_PROBE_GQL")
	if raw == "" {
		t.Skip("set QB_PROBE_GQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, st, err := execTaxGraphQL(ctx, &RequestPlan{URL: gstConfigURL, Body: json.RawMessage(raw)}, true)
	if out := os.Getenv("QB_PROBE_OUT"); out != "" {
		if werr := os.WriteFile(out, body, 0o600); werr != nil {
			t.Fatal(werr)
		}
	}
	t.Logf("status=%d body=%s", st, string(body))
	if err != nil {
		t.Fatal(err)
	}
}
