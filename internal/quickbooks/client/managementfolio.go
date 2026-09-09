package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Captured 2026-08-19 on /app/managementreports (open-39 follow-up).
// GET universalreportinsights /v1/folio?locale=en-au (no subType) → 200 array items=3.
// Distinct from custom Folio (subType=CRB_GROUP). Not BalanceSheet stand-in.
const managementFolioURL = "https://universalreportinsights.api.intuit.com/v1/folio?locale=en-au"
const managementFolioHost = "universalreportinsights.api.intuit.com"

func PlannedManagementFolioURL() string { return managementFolioURL }

func replayManagementFolio(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, managementFolioURL, managementFolioHost, nil)
	if err != nil {
		return nil, fmt.Errorf("management folio GET /v1/folio: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading management folio: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectManagementFolio(raw, query, limit)
	res.Status = resp.StatusCode
	return res, nil
}

func projectManagementFolio(body []byte, query string, limit int) *QueryResult {
	note := "universalreportinsights GET /v1/folio (management, no subType)"
	var arr []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if json.Unmarshal(body, &arr) != nil {
		return &QueryResult{Entity: "ManagementFolio", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(arr))
	for _, f := range arr {
		it := QueryItem{ID: f.ID, Name: f.Name}
		if q != "" && !strings.Contains(strings.ToLower(it.ID+" "+it.Name), q) {
			continue
		}
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	note = fmt.Sprintf("%s n=%d items=%d", note, len(arr), len(items))
	return &QueryResult{
		Entity: "ManagementFolio",
		Counts: map[string]int{"items": len(items), "totalCount": len(arr)},
		Items:  items,
		Note:   note,
	}
}
