package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Captured 2026-09-28 from POST https://qbo.intuit.com/api/v4/entities as the
// Custom reports page (/app/customreports) loads: reportDefinitions filtered
// to reportType MEMORIZED/MEMORIZEDGROUP is the saved-report registry the grid
// shows (144 defs on Mega Style Apartments). This is distinct from the
// universalreportinsights CRB_REPORT registry (reports saved list|get), which
// only covers modern builder reports.
const memorizedEntitiesURL = "https://qbo.intuit.com/api/v4/entities"
const memorizedReferer = "https://qbo.intuit.com/app/customreports"

// memorizedGraphQuery is the captured graphQuery verbatim — the full saved
// report definition incl. per-report groups (email schedules / grouped
// children), attributes, share scope, and the composite entity id whose
// ":N" suffix is the classic reportv2 mem_rpt_id.
const memorizedGraphQuery = `{
    company{
        reportDefinitions(first: 99999, filterBy:"reportType='MEMORIZED' and reportType='MEMORIZEDGROUP'") {
        edges {
            node{
            id
            reportName
            reportType
            entityVersion
            createdBy
            qboAppData {
                reportUrl
                reportMode
                reportDescription
            }
            recurInfo {
                name
            }
            attributes{
                name
                value
            }
            shareInfo{
                shareType
            }
            groups(first: 99999) {
                edges{
                node{
                    id
                    reportName
                    entityVersion
                    createdBy
                    attributes{
                    name
                    value
                    }
                    shareInfo{
                    shareType
                    }
                }
                }
            }
            }
        }
        }
    }
}`

// PlannedMemorizedURL is the dry-run URL for memorized-report reads.
func PlannedMemorizedURL() string {
	return memorizedEntitiesURL + " (reportDefinitions MEMORIZED graphQuery)"
}

// memorizedRunQuery is the captured graphQuery verbatim from
// /app/reportv2?mem_rpt_id=N: the classic report grid executes the saved
// report server-side via createReports_Report on the same v4/entities lane.
// The node returns reportOptions (resolved filter values), columns, and the
// hierarchical rows[].cells[] grid (indent/type/summary attributes on cells).
const memorizedRunQuery = `
  fragment reportsExecuteFragment on CreateReports_ReportPayload {
    reportsReportEdge{
      node {
        reportDefinition {
          reportName
          reportOptions {
            id
            name
            defaultValue
            value
            type
            group
            enabled
            listType
            displayName
            required
            allowedValues {
              name
              value
              attributes {
                name
                value
              }
            }
            attributes {
              name
              value
            }
          }
        }
        attributes {
          name
          value
        }
        createDateTime
        columns {
          value
          type
          attributes {
            name
            value
          }
          columns {
            value
            type
            attributes {
              name
              value
            }
          }
        }
        rows {
          id
          parentId
          cells {
            id
            link
            type
            displayValue
            value
            annotation {
              description {
                date
                added_by
              }
            }
            attributes {
              name
              value
            }
          }
        }
      }
    }
  }mutation createReports_Report($input_0: CreateReports_ReportInput!) {
    createReports_Report(input: $input_0) {
      clientMutationId,
      ...reportsExecuteFragment
    }
  }`

// memorizedDefIDPrefix is the composite-id scheme v4 uses for saved report
// definitions ("djQ::/reports/ReportDefinition:"). A bare mem_rpt_id resolves
// through reportOptions regardless; the composite id is sent verbatim.
const memorizedDefIDPrefix = "djQ6Oi9yZXBvcnRzL1JlcG9ydERlZmluaXRpb246"

// memorizedEdge is one reportDefinitions node — a saved custom report.
type memorizedEdge struct {
	Node *struct {
		ID            string `json:"id"`
		ReportName    string `json:"reportName"`
		ReportType    string `json:"reportType"`
		EntityVersion string `json:"entityVersion"`
		CreatedBy     string `json:"createdBy"`
		QboAppData    *struct {
			ReportURL         string `json:"reportUrl"`
			ReportMode        string `json:"reportMode"`
			ReportDescription string `json:"reportDescription"`
		} `json:"qboAppData"`
		RecurInfo *struct {
			Name string `json:"name"`
		} `json:"recurInfo"`
		Attributes []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"attributes"`
		ShareInfo *struct {
			ShareType string `json:"shareType"`
		} `json:"shareInfo"`
		Groups *struct {
			Edges []json.RawMessage `json:"edges"`
		} `json:"groups"`
	} `json:"node"`
}

// memorizedEdges unwraps the /api/v4/entities envelope: top-level array of
// {$type, data} where data may be an array OR an object keyed "0" (the
// reportDefinitions response uses the latter).
func memorizedEdges(body []byte) ([]memorizedEdge, error) {
	var outer []struct {
		Data   json.RawMessage   `json:"data"`
		Type   string            `json:"$type"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, fmt.Errorf("memorized reports envelope: %w", err)
	}
	type companyWrap struct {
		Company *struct {
			ReportDefinitions *struct {
				Edges *[]memorizedEdge `json:"edges"`
			} `json:"reportDefinitions"`
		} `json:"company"`
		Errors []json.RawMessage `json:"errors"`
	}
	var edges []memorizedEdge
	sawCollection := false
	collect := func(d companyWrap) error {
		if len(d.Errors) > 0 || d.Company == nil || d.Company.ReportDefinitions == nil || d.Company.ReportDefinitions.Edges == nil {
			return fmt.Errorf("memorized reports: missing reportDefinitions.edges or query error")
		}
		sawCollection = true
		for _, edge := range *d.Company.ReportDefinitions.Edges {
			if edge.Node == nil || edge.Node.ID == "" {
				return fmt.Errorf("memorized reports: report node lacks identity")
			}
			edges = append(edges, edge)
		}
		return nil
	}
	for _, row := range outer {
		if len(row.Errors) > 0 || row.Type != "" && row.Type != "/Result" {
			return nil, fmt.Errorf("memorized reports: query failed")
		}
		if len(row.Data) == 0 || string(row.Data) == "null" {
			return nil, fmt.Errorf("memorized reports: missing query data")
		}
		// data as array of {company:{...}}
		var arr []companyWrap
		if err := json.Unmarshal(row.Data, &arr); err == nil {
			for _, d := range arr {
				if err := collect(d); err != nil {
					return nil, err
				}
			}
			continue
		}
		// data as object keyed "0": {"0":{company:{...}}}
		var obj map[string]companyWrap
		if err := json.Unmarshal(row.Data, &obj); err == nil {
			for _, d := range obj {
				if err := collect(d); err != nil {
					return nil, err
				}
			}
		} else {
			return nil, fmt.Errorf("memorized reports: invalid query data")
		}
	}
	if !sawCollection {
		return nil, fmt.Errorf("memorized reports: no reportDefinitions.edges collection")
	}
	return edges, nil
}

// memorizedIDMatch reports whether a node id matches the caller's selector:
// the full composite id, or its ":N" suffix which is the classic mem_rpt_id.
func memorizedIDMatch(id, want string) bool {
	if id == want {
		return true
	}
	if i := strings.LastIndex(id, ":"); i >= 0 && i+1 < len(id) {
		return id[i+1:] == want
	}
	return false
}

// replayMemorizedReports backs `reports memorized list|get`
// (QBO.REPORTS.MEMORIZED_LIST / MEMORIZED_READ): the saved custom-report
// registry behind /app/customreports. list projects edges to items; get
// resolves a node by composite id or mem_rpt_id suffix and surfaces the full
// definition (attributes, recurInfo, share scope, child groups) under Detail.
func replayMemorizedReports(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal([]map[string]any{
		{"$type": "/Query", "graphQuery": memorizedGraphQuery, "variables": nil},
	})
	if err != nil {
		return nil, fmt.Errorf("memorized reports: %w", err)
	}
	resp, err := ac.postJSONExtra(ctx, memorizedEntitiesURL, payload, map[string]string{
		"Referer":      memorizedReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("memorized reports: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading memorized reports: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	edges, err := memorizedEdges(body)
	if err != nil {
		return nil, err
	}

	items := make([]QueryItem, 0, len(edges))
	var match map[string]any
	var matchedItem QueryItem
	sel := strings.TrimSpace(id)
	for _, e := range edges {
		n := e.Node
		if n == nil {
			continue
		}
		it := QueryItem{Type: "MemorizedReport", ID: n.ID, Name: n.ReportName, SubType: n.ReportType}
		if i := strings.LastIndex(n.ID, ":"); i >= 0 {
			it.DocNumber = n.ID[i+1:] // classic reportv2 mem_rpt_id
		}
		items = append(items, it)
		if sel != "" && memorizedIDMatch(n.ID, sel) {
			matchedItem = it
			var node map[string]any
			if raw, merr := json.Marshal(n); merr == nil {
				_ = json.Unmarshal(raw, &node)
			}
			match = node
		}
	}
	if sel != "" {
		if match == nil {
			return nil, &ReplayError{Status: 404, Message: "no memorized report with id " + sel}
		}
		return &QueryResult{
			Entity: "MemorizedReport",
			Counts: map[string]int{"items": 1, "totalCount": len(items)},
			Items:  []QueryItem{matchedItem},
			Detail: map[string]any{"report": match},
			Note:   "v4/entities reportDefinitions MEMORIZED (resolved by id/mem_rpt_id)",
		}, nil
	}
	if q := strings.ToLower(strings.TrimSpace(query)); q != "" {
		kept := items[:0]
		for _, it := range items {
			if strings.Contains(strings.ToLower(it.Name), q) {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return &QueryResult{
		Entity: "MemorizedReport",
		Counts: map[string]int{"items": len(items), "totalCount": len(edges)},
		Items:  items,
		Note:   fmt.Sprintf("v4/entities reportDefinitions MEMORIZED n=%d", len(edges)),
	}, nil
}

// MemorizedRunOptions selects the period/basis override applied after the
// saved definition's own options are resolved.
type MemorizedRunOptions struct {
	LowDate  string // YYYY-MM-DD
	HighDate string // YYYY-MM-DD
	Basis    string // "cash" or "accrual"; empty keeps the saved report's basis
}

// memorizedV3Report maps the classic report token a memorized definition
// resolves to onto the equivalent v3 ReportService report used for
// date/basis overrides.
var memorizedV3Report = map[string]string{
	"TX_DET_BY_ACCT": "ProfitAndLossDetail", // Transaction Detail by Category
	"PANDL":          "ProfitAndLoss",
	"PANDL_DET":      "ProfitAndLossDetail",
	"BAL_SHEET":      "BalanceSheet",
	"GEN_LEDGER":     "GeneralLedger",
	"TRIAL_BAL":      "TrialBalance",
}

// PlannedMemorizedRunURL is the dry-run URL for memorized-report execution.
func PlannedMemorizedRunURL() string {
	return memorizedEntitiesURL + " (createReports_Report graphQuery)"
}

// memorizedRunNode is the executed report payload — resolved reportOptions,
// column headers, and the row tree. Cells carry displayValue/value plus
// indent/summary/align attribute hints.
type memorizedRunNode struct {
	ReportDefinition struct {
		ReportName    string `json:"reportName"`
		ReportOptions []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"reportOptions"`
	} `json:"reportDefinition"`
	Attributes []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"attributes"`
	Columns json.RawMessage `json:"columns"`
	Rows    json.RawMessage `json:"rows"`
}

// ReplayMemorizedRun executes a saved memorized report (QBO.REPORTS.
// MEMORIZED_RUN): POST createReports_Report on v4/entities with the saved
// definition's mem_rpt_id plus optional date/basis overrides, exactly as the
// classic reportv2 grid does on page load. Read-only.
func ReplayMemorizedRun(ctx context.Context, ref string, opts MemorizedRunOptions) (*ReportResult, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("memorized run requires --id (composite id or mem_rpt_id)")
	}
	memID := ref
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		memID = ref[i+1:]
	}
	defID := ref
	if !strings.Contains(ref, ":") {
		defID = memorizedDefIDPrefix + ":" + ref
	}
	// Execution is bound to the stored definition: reportOptions here only
	// select which report runs (mem_rpt_id). The grid ignores inline
	// date/basis overrides, so overrides are applied afterwards through the
	// equivalent v3 report — the saved option set (token, account, class,
	// basis) is read back from the executed report's resolvedOptions.
	reportOptions := []map[string]string{
		{"id": "mem_rpt_id", "value": memID},
		{"id": "edited_sections", "value": "false"},
	}

	payload, err := json.Marshal([]map[string]any{{
		"$type":      "/Query",
		"graphQuery": memorizedRunQuery,
		"variables": map[string]any{
			"input_0": map[string]any{
				"clientMutationId": "0",
				"reportsReport": map[string]any{
					"reportDefinition": map[string]any{
						"id":            defID,
						"reportType":    "MEMORIZED",
						"reportOptions": reportOptions,
					},
				},
			},
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("memorized run: %w", err)
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, memorizedEntitiesURL, payload, map[string]string{
		"Referer":      "https://qbo.intuit.com/app/reportv2?mem_rpt_id=" + memID,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("memorized run: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading memorized run: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}

	node, err := memorizedRunNodeFromEnvelope(body)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, &ReplayError{Status: 404, Message: "memorized run returned no report node (id " + memID + ")"}
	}
	// The executed node only carries the generic report name — resolve the
	// saved definition's name from the registry for display.
	savedName := node.ReportDefinition.ReportName
	if reg, rerr := replayMemorizedReports(ctx, memID, "", 0); rerr == nil && len(reg.Items) == 1 && reg.Items[0].Name != "" {
		savedName = reg.Items[0].Name
	}

	var cols []string
	var rawCols []struct {
		Value   string `json:"value"`
		Columns []struct {
			Value string `json:"value"`
		} `json:"columns"`
	}
	if json.Unmarshal(node.Columns, &rawCols) == nil {
		for _, c := range rawCols {
			if len(c.Columns) > 0 {
				for _, sub := range c.Columns {
					cols = append(cols, sub.Value)
				}
				continue
			}
			cols = append(cols, c.Value)
		}
	}
	var rowCount int
	var rows []json.RawMessage
	if json.Unmarshal(node.Rows, &rows) == nil {
		rowCount = len(rows)
	}
	header := map[string]any{"definitionId": defID, "memRptId": memID}
	for _, a := range node.Attributes {
		header[a.Name] = a.Value
	}
	resolved := map[string]string{}
	for _, o := range node.ReportDefinition.ReportOptions {
		if o.Value != "" {
			resolved[o.ID] = o.Value
		}
	}
	if len(resolved) > 0 {
		header["resolvedOptions"] = resolved
	}
	if (opts.LowDate != "" && opts.HighDate != "") || opts.Basis != "" {
		return replayMemorizedOverride(ctx, savedName, resolved, opts)
	}
	return &ReportResult{
		Status:  resp.StatusCode,
		Report:  savedName,
		Counts:  map[string]int{"rows": rowCount, "columns": len(cols)},
		Header:  header,
		Columns: cols,
		Rows:    node.Rows,
		Note:    "v4/entities createReports_Report (executed memorized report)",
	}, nil
}

// memorizedRunNodeFromEnvelope unwraps [{$type:/Result, data:[{createReports_
// Report:{reportsReportEdge:{node}}}]] and returns the executed report node.
func memorizedRunNodeFromEnvelope(body []byte) (*memorizedRunNode, error) {
	var outer []struct {
		Data []struct {
			CreateReportsReport struct {
				ReportsReportEdge struct {
					Node *memorizedRunNode `json:"node"`
				} `json:"reportsReportEdge"`
			} `json:"createReports_Report"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, fmt.Errorf("memorized run envelope: %w", err)
	}
	for _, row := range outer {
		for _, d := range row.Data {
			if d.CreateReportsReport.ReportsReportEdge.Node != nil {
				return d.CreateReportsReport.ReportsReportEdge.Node, nil
			}
		}
	}
	return nil, nil
}

// auDate converts YYYY-MM-DD to the dd/MM/yyyy shape the classic report
// service expects (en-au locale).
func auDate(iso string) string {
	p := strings.SplitN(strings.TrimSpace(iso), "-", 3)
	if len(p) != 3 {
		return iso
	}
	return p[2] + "/" + p[1] + "/" + p[0]
}

// replayMemorizedOverride handles --date-range/--basis on `memorized run`:
// the executed report's resolvedOptions carry the saved filter set (token,
// account whitelist, class list, basis), which is replayed through the
// equivalent v3 report for the requested window.
func replayMemorizedOverride(ctx context.Context, savedName string, resolved map[string]string, opts MemorizedRunOptions) (*ReportResult, error) {
	v3name, ok := memorizedV3Report[resolved["token"]]
	if !ok {
		return nil, fmt.Errorf("memorized run override: saved report token %q has no v3 equivalent; run without --date-range/--basis for the saved period", resolved["token"])
	}
	start, end := opts.LowDate, opts.HighDate
	if start == "" || end == "" {
		// a basis-only override keeps the saved period
		start = isoDate(resolved["start_date"])
		end = isoDate(resolved["end_date"])
	}
	basis := "Cash"
	if resolved["accounting_method"] == "no" {
		basis = "Accrual"
	}
	if opts.Basis != "" {
		basis = map[string]string{"cash": "Cash", "accrual": "Accrual"}[strings.ToLower(opts.Basis)]
	}
	filters := map[string]string{}
	if v := dedupeCSV(resolved["class"]); v != "" {
		filters["klass"] = v
	}
	if v := resolved["account"]; v != "" {
		filters["account"] = v
	}
	if strings.Contains(resolved["group_by"], "Account/") {
		filters["group_by"] = "Account"
	}
	if v := resolved["sort_order"]; v != "" {
		filters["sort_order"] = v
	}
	res, err := ReplayReport(ctx, v3name, start, end, ReportOptions{AccountingMethod: basis, Filters: filters})
	if err != nil {
		return nil, err
	}
	res.Report = savedName
	if res.Note == "" {
		res.Note = "memorized filters replayed via v3 " + v3name
	}
	return res, nil
}

// dedupeCSV collapses "a,a,a" (the class list the resolver emits per account
// group) back to "a".
func dedupeCSV(s string) string {
	parts := strings.Split(s, ",")
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

// isoDate converts dd/MM/yyyy back to YYYY-MM-DD.
func isoDate(au string) string {
	p := strings.SplitN(strings.TrimSpace(au), "/", 3)
	if len(p) != 3 {
		return au
	}
	return p[2] + "-" + p[1] + "-" + p[0]
}
