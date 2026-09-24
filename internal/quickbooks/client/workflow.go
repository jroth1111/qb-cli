package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const workflowGraphQLURL = "https://workflowautomation.api.intuit.com/v4/graphql"
const workflowHost = "workflowautomation.api.intuit.com"

const wasAllRulesQuery = `query WASAllRules {
  workflows {
    definitions(filterBy: "template.category IN ('CUSTOM','HUB')") {
      edges {
        node {
          id
          definitionKey
          name
          displayName
          description
          status
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}`

func PlannedWorkflowURL() string { return workflowGraphQLURL }

func replayWASAllRules(ctx context.Context, _, _ string, _ int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "WASAllRules",
		"variables":     map[string]any{},
		"query":         wasAllRulesQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("workflow WASAllRules: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, workflowGraphQLURL, workflowHost, payload)
	if err != nil {
		return nil, fmt.Errorf("workflow WASAllRules: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading workflow WASAllRules: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectWASAllRules(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectWASAllRules(body []byte) *QueryResult {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Workflows struct {
				Definitions struct {
					Edges []json.RawMessage `json:"edges"`
				} `json:"definitions"`
			} `json:"workflows"`
		} `json:"data"`
	}
	note := "workflowautomation WASAllRules"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "Workflow", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	n := len(wrap.Data.Workflows.Definitions.Edges)
	note = fmt.Sprintf("workflowautomation WASAllRules edges=%d", n)
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	items := projectJSONItems("Workflow", gqlEdgeNodes(wrap.Data.Workflows.Definitions.Edges), workflowItemKeys)
	return &QueryResult{
		Entity: "Workflow",
		Counts: map[string]int{"items": len(items), "totalCount": n},
		Items:  items,
		Note:   note,
	}
}

// workflowItemKeys maps WASAllRules definition nodes onto QueryItem.
var workflowItemKeys = itemKeys{
	id:     []string{"id", "Id"},
	name:   []string{"displayName", "name", "Name", "definitionKey"},
	typeOf: []string{"status", "Status"},
}

// --- appconnect rule CRUD -----------------------------------------------------
//
// WORKFLOWS_CREATE / WORKFLOWS_EDIT — the workflows-page bundle compiles its
// mutations as Apollo @rest directives onto the iPaaS engine (captured from
// workflow-plugin chunk 9736):
//
//	createRule   POST   /api/v1/subscriptions/{sub}/workflows/
//	updateRule   PUT    /api/v1/subscriptions/{sub}/workflows/{id}
//	performRule  POST   /api/v1/subscriptions/{sub}/workflows/{id}?action=<verb>
//	deleteRule   DELETE /api/v1/subscriptions/{sub}/workflows/{id}
//
// The subscription id is the company's activated "Workflow AppConnect"
// (intuitAppId AP15231), resolved live from
// GET /api/v1/subscriptions?intuitAppId=AP15231&companyId={realm}.
// Auth: same workflow-plugin Intuit_APIKey as the GraphQL host. Proven live on
// TC2 2026-09-19: create {name}→201 id, PUT {name,description}→200 rename,
// DELETE→204; ?action=activate reaches real validation (IAC-002803 "Workflow
// is incomplete" on an unconfigured rule).
const appconnectHost = "api.appconnect.intuit.com"
const appconnectSubsURL = "https://api.appconnect.intuit.com/api/v1/subscriptions"

// PlannedWorkflowSubsURL is the dry-run URL for the subscription lookup.
func PlannedWorkflowSubsURL() string {
	return appconnectSubsURL + "?intuitAppId=AP15231&companyId={realm}"
}

// PlannedWorkflowsURL is the dry-run URL for the workflows collection.
func PlannedWorkflowsURL() string {
	return appconnectSubsURL + "/{subscription}/workflows/"
}

// workflowSubscription resolves the activated Workflow AppConnect
// subscription id for the current company.
func workflowSubscription(ctx context.Context, ac *apiClient) (string, error) {
	u := fmt.Sprintf("%s?intuitAppId=AP15231&companyId=%s", appconnectSubsURL, ac.realm)
	resp, err := ac.doURIHost(ctx, http.MethodGet, u, appconnectHost, nil)
	if err != nil {
		return "", fmt.Errorf("workflow subscription: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var subs []struct {
		ID          string `json:"id"`
		IntuitAppID string `json:"intuitAppId"`
		State       string `json:"state"`
	}
	if err := json.Unmarshal(raw, &subs); err != nil {
		return "", fmt.Errorf("malformed subscriptions %.200s", raw)
	}
	for _, s := range subs {
		if s.IntuitAppID == "AP15231" && s.State == "activated" && s.ID != "" {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("no activated Workflow AppConnect subscription (AP15231) for this company")
}

// ReplayWorkflowMutate creates or updates an appconnect workflow rule.
//
//	create --name X [--description Y]
//	update --id N [--name X] [--description Y] [--action activate|deactivate]
func ReplayWorkflowMutate(ctx context.Context, op, id, name, desc, action string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	desc = strings.TrimSpace(desc)
	action = strings.ToLower(strings.TrimSpace(action))
	switch op {
	case "create":
		if name == "" {
			return nil, fmt.Errorf("workflows create requires --name")
		}
	case "update":
		if id == "" {
			return nil, fmt.Errorf("workflows update requires --id (appconnect workflow id, e.g. from advanced workflows get)")
		}
		if action != "" && action != "activate" && action != "deactivate" {
			return nil, fmt.Errorf("--action must be activate or deactivate, got %q", action)
		}
		if action == "" && name == "" && desc == "" {
			return nil, fmt.Errorf("workflows update needs --name and/or --description, or --action activate|deactivate")
		}
	default:
		return nil, fmt.Errorf("unknown workflow op %q", op)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	sub, err := workflowSubscription(ctx, ac)
	if err != nil {
		return nil, err
	}
	base := appconnectSubsURL + "/" + sub + "/workflows/"
	if op == "create" {
		body, _ := json.Marshal(map[string]any{
			"name":        name,
			"description": desc,
			"track":       false,
		})
		return workflowDo(ctx, ac, op, http.MethodPost, base, body, "create "+name)
	}
	if action != "" {
		return workflowDo(ctx, ac, op, http.MethodPost, base+id+"?action="+action, nil, action+" "+id, id, action)
	}
	patch := map[string]any{"track": false}
	if name != "" {
		patch["name"] = name
	}
	if desc != "" {
		patch["description"] = desc
	}
	body, _ := json.Marshal(patch)
	return workflowDo(ctx, ac, op, http.MethodPut, base+id, body, "update "+id, id, "")
}

// workflowDo executes one appconnect call and projects the workflow onto
// MutateResult. 201/200 carry the workflow JSON; 204 carries nothing.
func workflowDo(ctx context.Context, ac *apiClient, op, method, url string, body []byte, note string, expected ...string) (*MutateResult, error) {
	resp, err := ac.doURIHost(ctx, method, url, appconnectHost, body)
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	item := QueryItem{Type: "Workflow"}
	var wf struct {
		ID          string          `json:"id"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Status      json.RawMessage `json:"status"`
	}
	var st string
	if json.Unmarshal(raw, &wf) == nil {
		item.ID = wf.ID
		item.Name = wf.Name
		if json.Unmarshal(wf.Status, &st) != nil {
			var so struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(wf.Status, &so) == nil {
				st = so.Status
			}
		}
		if st != "" {
			item.Type = "Workflow (" + st + ")"
		}
	}
	if item.ID == "" {
		return nil, fmt.Errorf("workflow receipt missing identity; inspect before retrying")
	}
	if len(expected) > 0 && item.ID != expected[0] {
		return nil, fmt.Errorf("workflow receipt identity mismatch; inspect before retrying")
	}
	if len(expected) > 1 && expected[1] != "" {
		want := map[string]string{"activate": "active", "deactivate": "inactive"}[expected[1]]
		if !strings.EqualFold(st, want) {
			return nil, fmt.Errorf("workflow status did not confirm %s; inspect before retrying", expected[1])
		}
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     op,
		Entity: "Workflow",
		Item:   item,
		Note:   "appconnect " + note,
	}, nil
}
