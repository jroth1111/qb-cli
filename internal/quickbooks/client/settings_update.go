package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Company settings edits ride settingsfacade.api.intuit.com — captured live on
// Test Company 2 (2026-09-17): the Settings drawer posts
// updateQbAppFoundationQbSettings there (not the qbo v4 gateway the catalog
// recorded). The update requires the target subtree's entityVersion for
// optimistic concurrency — STALE_STATE_ERROR without it — so the replay reads
// the current version, injects it, then posts the captured document.

const settingsUpdateDoc = `mutation UpdateSettings($with: String!, $updates: [UpdateQbAppFoundationQbSettingsInput!]!) {
  updateQbAppFoundationQbSettings(with: $with, updates: $updates) {
    finance { accounting { accountingCore { accountingCoreSettings { entityVersion } } } }
    identity { company { profile { entityVersion } } }
  }
}`

const settingsVersionDoc = `query {
  qbAppFoundationQbSettings {
    finance { accounting { accountingCore { accountingCoreSettings { entityVersion } } } }
  }
}`

// ReplaySettingsUpdate applies a patch to accountingCoreSettings on the
// settingsfacade host. patch holds leaf fields (e.g. {"accountNumbersEnabled":
// true}); the current entityVersion is fetched and injected automatically.
func ReplaySettingsUpdate(ctx context.Context, patch map[string]any) (*MutateResult, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("settings update requires --patch '<json object>' of accountingCoreSettings leaf fields")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	// Fetch the live entityVersion for optimistic concurrency.
	payload, err := json.Marshal(map[string]any{"query": settingsVersionDoc, "variables": map[string]any{}})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, "https://settingsfacade.api.intuit.com/graphql", "settingsfacade.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("settingsfacade version read: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("settingsfacade version read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var ver struct {
		Data struct {
			S struct {
				F struct {
					A struct {
						C struct {
							S struct {
								EntityVersion string `json:"entityVersion"`
							} `json:"accountingCoreSettings"`
						} `json:"accountingCore"`
					} `json:"accounting"`
				} `json:"finance"`
			} `json:"qbAppFoundationQbSettings"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &ver); err != nil {
		return nil, fmt.Errorf("settingsfacade: could not read accountingCoreSettings entityVersion")
	}
	if len(ver.Errors) > 0 && ver.Errors[0].Message != "" {
		return nil, fmt.Errorf("settingsfacade version read: %s", ver.Errors[0].Message)
	}
	if ver.Data.S.F.A.C.S.EntityVersion == "" {
		return nil, fmt.Errorf("settingsfacade: could not read accountingCoreSettings entityVersion")
	}
	// Assemble the captured envelope: leaf patch + entityVersion at the
	// accountingCoreSettings node.
	core := map[string]any{"entityVersion": ver.Data.S.F.A.C.S.EntityVersion}
	for k, v := range patch {
		if k == "entityVersion" {
			continue // never trust a caller-supplied version
		}
		core[k] = v
	}
	vars := map[string]any{
		"with": "",
		"updates": []any{
			map[string]any{
				"companySettings": map[string]any{
					"finance": map[string]any{
						"accounting": map[string]any{
							"accountingCore": map[string]any{
								"accountingCoreSettings": core,
							},
						},
					},
				},
			},
		},
	}
	payload, err = json.Marshal(map[string]any{"operationName": "UpdateSettings", "variables": vars, "query": settingsUpdateDoc})
	if err != nil {
		return nil, err
	}
	resp, err = ac.doURIHost(ctx, http.MethodPost, "https://settingsfacade.api.intuit.com/graphql", "settingsfacade.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("settingsfacade UpdateSettings: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err = readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading settingsfacade UpdateSettings: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			U struct {
				F struct {
					A struct {
						C struct {
							S struct {
								EntityVersion string `json:"entityVersion"`
							} `json:"accountingCoreSettings"`
						} `json:"accountingCore"`
					} `json:"accounting"`
				} `json:"finance"`
			} `json:"updateQbAppFoundationQbSettings"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("settingsfacade UpdateSettings: malformed response: %w", err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("settingsfacade UpdateSettings: %s", wrap.Errors[0].Message)
	}
	if strings.TrimSpace(wrap.Data.U.F.A.C.S.EntityVersion) == "" {
		return nil, fmt.Errorf("settingsfacade UpdateSettings outcome unknown: missing accountingCoreSettings receipt; inspect before retrying")
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "update",
		Entity: "CompanySettings",
		Item:   QueryItem{Type: "CompanySettings"},
		Note:   fmt.Sprintf("settingsfacade UpdateSettings entityVersion %s -> %s", ver.Data.S.F.A.C.S.EntityVersion, wrap.Data.U.F.A.C.S.EntityVersion),
	}, nil
}

// PlannedSettingsUpdateURL reports the mutation host for dry-run plans.
func PlannedSettingsUpdateURL() string { return "https://settingsfacade.api.intuit.com/graphql" }
