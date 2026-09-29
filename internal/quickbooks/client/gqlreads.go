package client

import (
	"context"
	"encoding/base64"
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
	vars := map[string]any{"firstCount": limit, "filterCriteria": map[string]any{}}
	return replayCatalogQuery(ctx, "GetCustomDimensionDefinitions", "", vars,
		"DimensionDefinition", query, limit,
		[]string{"data", "appFoundationsCustomDimensionDefinitions", "edges"})
}

// replayTasks runs TaskManagementTasks on the v4 gateway — the task-manager
// list behind the Tasks menu.
func replayTasks(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	vars := map[string]any{
		"first": limit,
		"after": 0,
		"filter": map[string]any{"namespace": map[string]any{
			"in": taskNamespaces(),
		}},
	}
	return replayCatalogQuery(ctx, "TaskManagementTasks", "", vars,
		"Task", query, limit, []string{"data", "taskManagementTasks", "edges"})
}

// taskNamespaces returns the captured QBO task-namespace filter set — the
// 16 useCase values the Tasks page sends in TaskManagement_TaskFilter.
func taskNamespaces() []any {
	cases := []string{
		"DTM_TASKS", "ADV_NTTF_SETUP_TASKS", "ADV_UPGRADER_SETUP_TASKS",
		"BASIC_BUSINESS_INFO_TASK", "APP_ONBOARDING_TASK", "IES_SETUP_TASKS",
		"BANKING_TASKS", "SPEND_TASKS", "BILL_PAY_TASKS", "CUSTOMER_HUB_TASKS",
		"PAYMENTS_TASKS", "INVOICING_TASKS", "INDIRECT_TAX", "QBL_TASKS",
		"QBL_FREE_SETUP_TASK", "MAILCHIMP_TASK",
	}
	out := make([]any, 0, len(cases))
	for _, uc := range cases {
		out = append(out, map[string]any{
			"domain":  map[string]any{"equals": "QBO"},
			"useCase": map[string]any{"equals": uc},
		})
	}
	return out
}

// replayTaxJurisdictions lists indirect-tax jurisdictions (the nexus
// surface). Proven live on TC2 — typed-empty edges (AU GST has no
// jurisdiction rows; nexus is a US sales-tax concept).
func replayTaxJurisdictions(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	return replayCatalogQuery(ctx, "TaxJurisdictions__indirect_tax_ui_qbo", "",
		map[string]any{"DEFAULT_FIRST_COUNT": limit}, "TaxJurisdiction", query, limit,
		[]string{"data", "company", "taxJurisdictions", "edges"})
}

// replayWarehouseLocations lists Commerce inventory locations on the
// warehouse-management-svc host. Proven live on TC2 — typed-empty nodes
// (no inventory-location records on this company).
func replayWarehouseLocations(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	return replayCatalogQuery(ctx, "CommerceInventoryLocations", "",
		map[string]any{"filter": map[string]any{}, "first": limit},
		"InventoryLocation", query, limit,
		[]string{"data", "commerceInventoryLocations", "nodes"})
}

// replayCatalogQuery posts the catalogued document for opName via doURIHost
// and projects the edge-node list at edgePath onto QueryItems. endpoint ""
// uses the op's catalog endpoint.
func replayCatalogQuery(ctx context.Context, opName, endpoint string, vars map[string]any, entity, query string, limit int, edgePath []string) (*QueryResult, error) {
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
			id:   []string{"id", "Id", "ID", "forecastId", "globalId", "budgetId", "assetId"},
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
		if limit > 0 && len(items) >= limit {
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

// reconciliationNodeID builds the Relay node id the banking-reconcile UI
// uses: base64("v4.1:<realm>:<typeHash>") + ":<accountId>". The typeHash
// cec4fce82f is the realm's Integration_Reconciliation type id — captured
// live on TC2 2026-09-17 (same hash resolves every bank account's
// reconciliation node on this realm).
func reconciliationNodeID(realm, accountID string) string {
	raw := "v4.1:" + realm + ":cec4fce82f"
	enc := base64.RawURLEncoding.EncodeToString([]byte(raw))
	return enc + ":" + accountID
}

const reconcileWrapperDoc = `query w($id: ID!) {
  node(id: $id) {
    id
    __typename
    ... on Integration_Reconciliation {
      inProgress
      statementEndingDate
      lastReconcileSession { id }
      adjustingEntryDefaultAccount { id }
      account { id name }
    }
  }
}`

// ReplayReconciliation reads the Integration_Reconciliation node for a bank
// account via qbo.intuit.com /api/v4/graphql — the live reconcile-session
// state (inProgress, statement ending date, last session). Proven on TC2:
// account 44 reports inProgress=true with a 2026-09-16 statement date.
func ReplayReconciliation(ctx context.Context, accountID, query string, limit int) (*QueryResult, error) {
	if accountID == "" {
		return nil, fmt.Errorf("reconcile read requires --id <account-id> (see accounting coa search for bank account ids)")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return apixGraphQL(ctx, ac, "Reconciliation", v4GraphQLURL,
		"qbo.intuit.com", reconcileWrapperDoc,
		map[string]any{"id": reconciliationNodeID(ac.realm, accountID)},
		[]string{"data", "node"}, "reconciliation node "+accountID, query, limit)
}

// ReplayQBOnlineGraphql posts to the alternate qbonline-aws gateway captured
// serving the banking-reconcile module. --op names a catalogued operation to
// route there; omitted, it runs the proven company-probe document.
func ReplayQBOnlineGraphql(ctx context.Context, op, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	doc := `query c { company { id __typename } }`
	vars := map[string]any{}
	note := "qbonline-aws company probe"
	if op != "" {
		cop, err := gql.Lookup(op)
		if err != nil {
			return nil, fmt.Errorf("qbonline-gateway op %q: %w", op, err)
		}
		doc = cop.Document
		note = "qbonline-aws " + cop.Name
	}
	return apixGraphQL(ctx, ac, "QBOnlineGraphql", "https://qbonline-aws.api.intuit.com/v4/graphql",
		qbonlineHeaderHost, doc, vars,
		[]string{"data", "company"}, note, query, limit)
}
