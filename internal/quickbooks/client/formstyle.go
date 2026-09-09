package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Captured 2026-08-19 on /app/formstyles (open-39 follow-up).
// GET txnsrendering.api.intuit.com/v2/customizations?fetchRethinkTemplate=true
// → 200 list items=2 (id, printPrefName, templateType). Honest form-style list — not Preferences.
const formStyleURL = "https://txnsrendering.api.intuit.com/v2/customizations?fetchRethinkTemplate=true"
const formStyleHost = "txnsrendering.api.intuit.com"

func PlannedFormStyleURL() string { return formStyleURL }

func replayFormStyles(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
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
	resp, err := ac.doURIHost(ctx, http.MethodGet, formStyleURL, formStyleHost, nil)
	if err != nil {
		return nil, fmt.Errorf("form-style customizations: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading form-style customizations: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectFormStyles(raw, query, limit)
	res.Status = resp.StatusCode
	return res, nil
}

func projectFormStyles(body []byte, query string, limit int) *QueryResult {
	note := "txnsrendering GET /v2/customizations"
	var wrap struct {
		Customizations []struct {
			ID            string `json:"id"`
			PrintPrefName string `json:"printPrefName"`
			TemplateType  string `json:"templateType"`
			Active        *bool  `json:"active"`
		} `json:"customizations"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "FormStyle", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(wrap.Customizations))
	for _, c := range wrap.Customizations {
		it := QueryItem{ID: c.ID, Name: c.PrintPrefName, Type: c.TemplateType, Active: c.Active}
		if q != "" && !strings.Contains(strings.ToLower(it.ID+" "+it.Name+" "+it.Type), q) {
			continue
		}
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	note = fmt.Sprintf("%s n=%d items=%d", note, len(wrap.Customizations), len(items))
	return &QueryResult{
		Entity: "FormStyle",
		Counts: map[string]int{"items": len(items), "totalCount": len(wrap.Customizations)},
		Items:  items,
		Note:   note,
	}
}
