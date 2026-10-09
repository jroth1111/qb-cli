package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
)

type authStatusTransport func(*http.Request) (*http.Response, error)

func (f authStatusTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestLiveAuthStatusIsOneJSONDocument(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		status   int
		liveness string
		verified bool
	}{
		{name: "live", status: http.StatusOK, liveness: "live", verified: true},
		{name: "stale", status: http.StatusUnauthorized, liveness: "stale"},
		{name: "server-error", status: http.StatusServiceUnavailable, liveness: "unknown"},
		{name: "packet-loss", liveness: "unknown"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv("QB_HOME", t.TempDir())
			previous := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = previous })
			http.DefaultClient = &http.Client{Transport: authStatusTransport(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet || req.URL.Host != "qbo.intuit.com" || !strings.Contains(req.URL.Path, "/query") {
					t.Fatal("unexpected auth verification request")
				}
				if scenario.status == 0 {
					return nil, errors.New("fixture packet loss")
				}
				return &http.Response{StatusCode: scenario.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"QueryResponse":{"CompanyInfo":[{"Id":"123","CompanyName":"Fixture Company"}]}}`)), Request: req}, nil
			})}
			var output, diagnostics bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetOut(&output)
			cmd.SetErr(&diagnostics)
			tok := &auth.TokenSet{RealmID: "123", Email: "person@example.invalid", Source: "fixture", Authorization: "Intuit_APIKey fixture"}
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			tok, err := auth.Load()
			if err != nil {
				t.Fatal(err)
			}
			err = runStatusLive(cmd, &rootFlags{asJSON: true}, tok, auth.Status{OK: true})
			if (err == nil) != scenario.verified {
				t.Fatal("wrong live-verification result", err)
			}
			var status struct {
				OK       bool   `json:"ok"`
				Liveness string `json:"liveness"`
				Verified bool   `json:"verified"`
			}
			decoder := json.NewDecoder(&output)
			if err := decoder.Decode(&status); err != nil {
				t.Fatal("invalid status JSON", err)
			}
			if status.OK != scenario.verified || status.Verified != scenario.verified || status.Liveness != scenario.liveness {
				t.Fatal("machine status confused stale, unknown and verified", status)
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				t.Fatal("stdout contains data after the status JSON", err)
			}
		})
	}
}

func TestLiveAuthStatusRetainsHumanLiveness(t *testing.T) {
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	printStatusLiveness(cmd, &rootFlags{}, auth.Status{OK: true}, authLivenessStale)
	if !strings.Contains(output.String(), "liveness: STALE") {
		t.Fatal("human status lost its actionable liveness message")
	}
}
