package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func resolveRelayURL() string {
	if v := os.Getenv("QB_RELAY_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	if tok, err := Load(); err == nil && tok.RelayURL != "" {
		return strings.TrimRight(tok.RelayURL, "/")
	}
	return DefaultRelayURL
}

type relayPage struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

func listRelayPages(ctx context.Context, relayURL string) ([]relayPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayURL+"/json/list", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relay /json/list returned %s", resp.Status)
	}
	var tabs []relayPage
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, err
	}
	return tabs, nil
}
