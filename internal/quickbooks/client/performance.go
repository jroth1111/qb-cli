package client

// PERFORMANCE_CREATE / PERFORMANCE_EDIT / PERFORMANCE_DELETE — Performance
// centre chart CRUD on dashboardframework.api.intuit.com, captured from
// /app/insights/dashboard (Reports > Performance centre):
//
//	GET    /v1/dashboards?name=Performance dashboard&sku=ADVANCED&locale=en-au
//	GET    /v1/dashboards/{id}/panels
//	POST   /v1/dashboards/{id}/panels          create chart (200)
//	PUT    /v1/dashboards/{id}/panels/{pid}    update chart — full object, not patch (200)
//	DELETE /v1/dashboards/{id}/panels/{pid}    soft-deactivate chart, active:false (200)
//
// Chart definitions come from universalreportinsights.api.intuit.com
// /v1/system/paneldefinitions?sku=ADVANCED (XML catalog: id, defaultData
// filters/groups, defaultView, creationMode QUICK_ADD/CUSTOMISE_ADD).
// Live-proven on TC2 2026-09-19: create {"name","type":"METRIC","active",
// "customization":{data:{ref:Revenue,request:def.defaultData},view}}→200
// (service assigns source.USER_CREATED); PUT full object→200 rename;
// DELETE→200 active:false. Partial PUT bodies 500 — always PUT the full panel.

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const dashFrameworkHost = "dashboardframework.api.intuit.com"
const insightsHost = "universalreportinsights.api.intuit.com"
const dashboardsURL = "https://dashboardframework.api.intuit.com/v1/dashboards"
const panelDefsURL = "https://universalreportinsights.api.intuit.com/v1/system/paneldefinitions?sku=ADVANCED"

// PlannedDashboardsURL is the dry-run URL for dashboard lookup.
func PlannedDashboardsURL() string {
	return dashboardsURL + "?name=Performance%20dashboard&sku=ADVANCED&locale=en-au"
}

// perfDashboardID resolves the company's "Performance dashboard" id.
func perfDashboardID(ctx context.Context, ac *apiClient) (string, error) {
	u := dashboardsURL + "?name=Performance%20dashboard&sku=ADVANCED&locale=en-au"
	resp, err := ac.doURIHost(ctx, http.MethodGet, u, dashFrameworkHost, nil)
	if err != nil {
		return "", fmt.Errorf("performance dashboard: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var arr []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return "", fmt.Errorf("malformed dashboards %.200s", raw)
	}
	for _, d := range arr {
		if d.ID != "" {
			return d.ID, nil
		}
	}
	return "", fmt.Errorf("no Performance dashboard found for this company")
}

// perfPanel is the dashboardframework panel shape (spelling preserved).
type perfPanel struct {
	ID            string          `json:"id,omitempty"`
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Active        bool            `json:"active"`
	PositionID    any             `json:"positionId"`
	Source        json.RawMessage `json:"source"`
	Customization json.RawMessage `json:"customization"`
	WidgetRef     any             `json:"widgetRef"`
	Props         any             `json:"props"`
}

// perfPanelDef is one paneldefinitions <item> (XML catalog entry).
type perfPanelDef struct {
	ID          string `xml:"id"`
	Name        string `xml:"name"`
	DefaultView string `xml:"defaultView"`
}

// perfDefinitions resolves the creatable chart-definition ids so --metric is
// validated against the real catalog, not guessed.
func perfDefinitions(ctx context.Context, ac *apiClient) ([]perfPanelDef, error) {
	resp, err := ac.doURIHost(ctx, http.MethodGet, panelDefsURL, insightsHost, nil)
	if err != nil {
		return nil, fmt.Errorf("panel definitions: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var list struct {
		Items []perfPanelDef `xml:"item"`
	}
	if err := xml.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("malformed paneldefinitions %.200s", raw)
	}
	return list.Items, nil
}

// perfPanelFetch returns the panels of the resolved dashboard.
func perfPanelFetch(ctx context.Context, ac *apiClient, dashID string) ([]perfPanel, error) {
	u := dashboardsURL + "/" + url.PathEscape(dashID) + "/panels"
	resp, err := ac.doURIHost(ctx, http.MethodGet, u, dashFrameworkHost, nil)
	if err != nil {
		return nil, err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var panels []perfPanel
	if err := json.Unmarshal(raw, &panels); err != nil {
		return nil, fmt.Errorf("malformed panels %.200s", raw)
	}
	return panels, nil
}

// perfRequest builds the customization.data.request from the definition
// defaults the UI posts: THIS_YEAR_TO_DATE + group by month. Proven live.
func perfRequest() map[string]any {
	return map[string]any{
		"filters": []map[string]any{{"on": "date", "operator": "datemacro", "value": "THIS_YEAR_TO_DATE"}},
		"groups":  []map[string]any{{"on": "time", "by": "month"}},
	}
}

// ReplayPerformanceMutate creates, updates, or deletes a Performance centre
// chart on the resolved dashboard.
//
//	create --name X --metric Revenue [--view TREND_LINE]
//	update --id sbg:... [--name X]
//	delete --id sbg:...
func ReplayPerformanceMutate(ctx context.Context, op, id, name, metric, view string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	metric = strings.TrimSpace(metric)
	view = strings.TrimSpace(view)
	switch op {
	case "create":
		if name == "" {
			return nil, fmt.Errorf("performance create requires --name")
		}
		if metric == "" {
			return nil, fmt.Errorf("performance create requires --metric (chart definition id, e.g. Revenue, Expenses, NetProfit)")
		}
	case "update":
		if id == "" {
			return nil, fmt.Errorf("performance update requires --id (panel id, e.g. sbg:...)")
		}
		if name == "" {
			return nil, fmt.Errorf("performance update requires --name (the only panel field proven mutable)")
		}
	case "delete":
		if id == "" {
			return nil, fmt.Errorf("performance delete requires --id (panel id, e.g. sbg:...)")
		}
	default:
		return nil, fmt.Errorf("unknown performance op %q", op)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	dash, err := perfDashboardID(ctx, ac)
	if err != nil {
		return nil, err
	}
	base := dashboardsURL + "/" + url.PathEscape(dash) + "/panels"
	switch op {
	case "create":
		defs, err := perfDefinitions(ctx, ac)
		if err != nil {
			return nil, err
		}
		var def *perfPanelDef
		for i := range defs {
			if strings.EqualFold(defs[i].ID, metric) || strings.EqualFold(defs[i].Name, metric) {
				def = &defs[i]
				break
			}
		}
		if def == nil {
			known := make([]string, 0, len(defs))
			for _, d := range defs {
				known = append(known, d.ID)
			}
			return nil, fmt.Errorf("unknown --metric %q; definitions: %s", metric, strings.Join(known, ", "))
		}
		if view == "" {
			view = def.DefaultView
		}
		body, _ := json.Marshal(map[string]any{
			"name":   name,
			"type":   "METRIC",
			"active": true,
			"customization": map[string]any{
				"data": map[string]any{"ref": def.ID, "request": perfRequest()},
				"view": map[string]any{"visualization": view},
			},
		})
		return perfDo(ctx, ac, op, http.MethodPost, base, body, "create "+name)
	case "update":
		panels, err := perfPanelFetch(ctx, ac, dash)
		if err != nil {
			return nil, err
		}
		var cur *perfPanel
		for i := range panels {
			if panels[i].ID == id {
				cur = &panels[i]
				break
			}
		}
		if cur == nil {
			return nil, fmt.Errorf("panel %q not on the Performance dashboard", id)
		}
		cur.Name = name
		body, _ := json.Marshal(cur)
		return perfDo(ctx, ac, op, http.MethodPut, base+"/"+url.PathEscape(id), body, "update "+id)
	case "delete":
		return perfDo(ctx, ac, op, http.MethodDelete, base+"/"+url.PathEscape(id), nil, "delete "+id)
	}
	return nil, fmt.Errorf("unknown performance op %q", op)
}

// perfDo executes one dashboardframework call and projects onto MutateResult.
// 200 responses carry the panel JSON; DELETE returns the deactivated panel.
func perfDo(ctx context.Context, ac *apiClient, op, method, url string, body []byte, note string) (*MutateResult, error) {
	resp, err := ac.doURIHost(ctx, method, url, dashFrameworkHost, body)
	if err != nil {
		return nil, fmt.Errorf("performance %s: %w", op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	item := QueryItem{Type: "Chart"}
	var p perfPanel
	if json.Unmarshal(raw, &p) == nil {
		item.ID = p.ID
		item.Name = p.Name
		if !p.Active {
			item.Type = "Chart (inactive)"
		}
	}
	var state struct {
		Active *bool `json:"active"`
	}
	_ = json.Unmarshal(raw, &state)
	if item.ID == "" || (op == "delete" && (state.Active == nil || *state.Active)) {
		return nil, fmt.Errorf("performance receipt missing identity or expected deletion; inspect before retrying")
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     op,
		Entity: "PerformanceChart",
		Item:   item,
		Note:   "dashboardframework " + note,
	}, nil
}
