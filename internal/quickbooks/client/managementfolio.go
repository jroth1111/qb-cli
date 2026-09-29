package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Captured 2026-08-19 on /app/managementreports (open-39 follow-up).
// GET universalreportinsights /v1/folio?locale=en-au (no subType) → 200 array items=3.
// Distinct from custom Folio (subType=CRB_GROUP). Not BalanceSheet stand-in.
const managementFolioURL = "https://universalreportinsights.api.intuit.com/v1/folio?locale=en-au"
const managementFolioHost = "universalreportinsights.api.intuit.com"

func PlannedManagementFolioURL() string { return managementFolioURL }

func replayManagementFolio(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if id = strings.TrimSpace(id); id != "" {
		// Single-folio read: GET /v1/folio/{id} returns the full folio —
		// cover page, TOC and every report page. Each REPORT page carries
		// reportToken = the classic memorized-report id (mem_rpt_id) it
		// executes, plus its own reportDateMacro/startDate/endDate which
		// override the saved report's period at render time. The list
		// projection drops folioDataRequest, so detail carries the raw object.
		u := "https://" + managementFolioHost + "/v1/folio/" + url.PathEscape(id) + "?locale=en-au"
		resp, err := ac.doURIHost(ctx, http.MethodGet, u, managementFolioHost, nil)
		if err != nil {
			return nil, fmt.Errorf("management folio GET /v1/folio/%s: %w", id, err)
		}
		defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
		raw, err := readBody(resp)
		if err != nil {
			return nil, fmt.Errorf("reading management folio %s: %w", id, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("malformed folio %.200s", raw)
		}
		items := folioPageItems(obj)
		resolveFolioPageNames(ctx, items)
		note := "universalreportinsights GET /v1/folio/" + id
		if mod, _ := obj["lastModifiedDate"].(string); mod != "" {
			note += " | stored dates are the last-saved config, not proof of the generated period"
		}
		return &QueryResult{
			Entity: "ManagementFolio",
			Counts: map[string]int{"items": len(items), "pages": len(items), "totalCount": 1},
			Items:  items,
			Detail: obj,
			Note:   note,
		}, nil
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, managementFolioURL, managementFolioHost, nil)
	if err != nil {
		return nil, fmt.Errorf("management folio GET /v1/folio: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
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

// folioReadPage is one folioDataRequest.pages[] entry for the read path —
// superset of the mutate-side folioPage: pages returned by GET carry
// startDate/endDate and the resolved cover fields.
type folioReadPage struct {
	PageType        string `json:"pageType"`
	Type            string `json:"type"`
	Title           string `json:"title"`
	PageID          string `json:"pageId"`
	ReportToken     string `json:"reportToken"`
	ReportDateMacro string `json:"reportDateMacro"`
	StartDate       string `json:"startDate"`
	EndDate         string `json:"endDate"`
	PreparedBy      string `json:"preparedBy"`
	PreparedDate    string `json:"preparedDate"`
	ReportPeriod    string `json:"reportPeriod"`
}

// folioPageItems projects a folio object into one QueryItem per ordered page.
// REPORT/URI_REPORT pages expose reportToken (a memorized-report id) as ID so
// it can feed `reports memorized run --id`; the page title (not the registry
// name) is what renders in the statement TOC.
func folioPageItems(obj map[string]any) []QueryItem {
	fdr, _ := obj["folioDataRequest"].(map[string]any)
	pages, _ := fdr["pages"].([]any)
	items := make([]QueryItem, 0, len(pages))
	for i, raw := range pages {
		b, _ := json.Marshal(raw)
		var p folioReadPage
		if json.Unmarshal(b, &p) != nil {
			continue
		}
		it := QueryItem{
			ID:      fmt.Sprintf("p%d", i),
			Name:    p.Title,
			Type:    p.Type,
			SubType: p.ReportDateMacro,
		}
		if p.ReportToken != "" {
			it.ID = p.ReportToken
			it.DocNumber = p.ReportToken
		} else if p.PageID != "" {
			it.ID = p.PageID
		}
		if p.PageType == "page" && p.Type == "COVER_PAGE" {
			it.SubType = p.PreparedBy
		}
		if p.StartDate != "" || p.EndDate != "" {
			it.Date = p.StartDate + "→" + p.EndDate
		}
		items = append(items, it)
	}
	return items
}

// resolveFolioPageNames fills FullName with the registry name for each page's
// reportToken — memorized ids ("42") via the MEMORIZED registry, sbg: ids via
// the builder registry. Best-effort: a missing definition leaves FullName
// empty rather than failing the read (a folio can reference deleted reports).
func resolveFolioPageNames(ctx context.Context, items []QueryItem) {
	need := map[string][]int{}
	for i, it := range items {
		if it.DocNumber != "" {
			need[it.DocNumber] = append(need[it.DocNumber], i)
		}
	}
	if len(need) == 0 {
		return
	}
	resolve := func(reg *QueryResult) {
		if reg == nil {
			return
		}
		for _, it := range reg.Items {
			for _, key := range []string{it.DocNumber, it.ID} {
				for _, idx := range need[key] {
					items[idx].FullName = it.Name
				}
			}
		}
	}
	if mem, err := replayMemorizedReports(ctx, "", "", 0); err == nil {
		resolve(mem)
	}
	if saved, err := replaySavedReports(ctx, "", "", 0); err == nil {
		resolve(saved)
	}
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
		if limit > 0 && len(items) >= limit {
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
