package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
)

// Named-catalog-op reads: each helper posts a captured GraphQL operation
// through doURIHost against the host the capture proved.

// replayIDX runs the captured Connections query (v4 gateway) — the list of
// connected Intuit Data Exchange app connections for the company.
func replayIDX(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	return replayCatalogQuery(ctx, "Connections", "", nil, "IDX", query, limit,
		[]string{"data", "company", "connections", "edges"})
}

// replayIMSPref runs GetIMSPref against the commercecontrol host — the
// order-management/inventory preference read the sales commerce surface uses.
func replayIMSPref(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	res, err := replayCatalogQuery(ctx, "GetIMSPref", "https://commercecontrol.api.intuit.com/graphql",
		nil, "IMSPref", query, limit, []string{"data", "imsPref"})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// replayDimensionDefinitions runs GetCustomDimensionDefinitions on the v4
// gateway — custom dimension definitions (empty set is a valid read).
func replayDimensionDefinitions(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	vars := map[string]any{"firstCount": limit}
	return replayCatalogQuery(ctx, "GetCustomDimensionDefinitions", "", vars,
		"DimensionDefinition", query, limit,
		[]string{"data", "appFoundationsCustomDimensionDefinitions", "edges"})
}

// replayTasks runs TaskManagementTasks on the v4 gateway — the task-manager
// list behind the Tasks menu.
func replayTasks(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	vars := map[string]any{
		"first":  limit,
		"after":  0,
		"filter": map[string]any{},
	}
	return replayCatalogQuery(ctx, "TaskManagementTasks", "", vars,
		"Task", query, limit, []string{"data", "taskManagementTasks", "edges"})
}

// replayCatalogQuery posts the catalogued document for opName via doURIHost
// and projects the edge-node list at edgePath onto QueryItems. endpoint ""
// uses the op's catalog endpoint.
func replayCatalogQuery(ctx context.Context, opName, endpoint string, vars map[string]any, entity, query string, limit int, edgePath []string) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	op, err := gql.Lookup(opName)
	if err != nil {
		return nil, err
	}
	url := endpoint
	if url == "" {
		url = op.Endpoint
	}
	if vars == nil {
		vars = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": op.Name,
		"variables":     vars,
		"query":         op.Document,
	})
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", entity, opName, err)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, url, hostOf(url), payload)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", entity, opName, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading %s %s: %w", entity, opName, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return projectEdgeNodes(entity, opName+" @ "+hostOf(url), raw, edgePath, query, limit, resp.StatusCode), nil
}

// projectEdgeNodes walks edgePath (e.g. data→company→connections→edges),
// projects each edge's node onto QueryItem, and applies the substring filter.
func projectEdgeNodes(entity, note string, body []byte, edgePath []string, query string, limit, status int) *QueryResult {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return &QueryResult{Entity: entity, Status: status, Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	node := doc
	for _, key := range edgePath {
		m, ok := node.(map[string]any)
		if !ok {
			node = nil
			break
		}
		node = m[key]
	}
	edges, _ := node.([]any)
	if m, ok := node.(map[string]any); ok {
		// Object payloads (e.g. imsPref prefs) project as a single row.
		edges = []any{m}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(edges))
	for _, e := range edges {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		nm, ok := em["node"].(map[string]any)
		if !ok {
			nm = em // non-edge payloads: project the object itself
		}
		raw, _ := json.Marshal(nm)
		it := projectJSONItems(entity, []json.RawMessage{raw}, itemKeys{
			id:   []string{"id", "Id", "ID"},
			name: []string{"name", "Name", "label", "title", "displayName"},
		})
		if len(it) == 0 {
			continue
		}
		one := it[0]
		blob := strings.ToLower(string(raw))
		if q != "" && !strings.Contains(blob, q) {
			continue
		}
		items = append(items, one)
		if len(items) >= limit {
			break
		}
	}
	var errNote string
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &wrap) == nil && len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		errNote = "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: entity,
		Status: status,
		Counts: map[string]int{"items": len(items), "totalCount": len(edges)},
		Items:  items,
		Note:   fmt.Sprintf("%s edges=%d%s", note, len(edges), errNote),
	}
}
