package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
)

// QBO Projects ride the accountant-workflow service — the /app/projects page
// lists company.projects (Work_Project) and creates via createWork_Project.
// The v3 "project" endpoint does not exist ("Operation project is not
// supported"), and v3 Customer Job=true creates a sub-customer with
// IsProject=false — a false alias. Contracts live-proven on Test Company 2
// (2026-09-17): project 813760022 created, renamed, entityVersion-locked.

const accountantWorkflowURL = "https://accountantworkflow.api.intuit.com/v4/graphql"

const workProjectFields = `id name description dueDate status type entityVersion client { id }`

func workflowMutation(ctx context.Context, opName, doc string, input map[string]any) (map[string]any, error) {
	// Captured catalog documents carry literal `\t` artifacts that the server
	// rejects as GraphQL syntax errors — normalize them out.
	doc = strings.ReplaceAll(doc, `\t`, " ")
	payload, err := json.Marshal(map[string]any{
		"operationName": opName,
		"variables":     map[string]any{"input": input},
		"query":         doc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, accountantWorkflowURL, "accountantworkflow.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", opName, err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", opName, err)
	}
	var out struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s parse: %w", opName, err)
	}
	if len(out.Errors) > 0 {
		return nil, &ReplayError{Status: resp.StatusCode, Message: out.Errors[0].Message}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	if out.Data == nil {
		return nil, fmt.Errorf("%s outcome unknown: missing data receipt; inspect before retrying", opName)
	}
	return out.Data, nil
}

// ReplayProjectCreate runs CreateProject_workflow — createWork_Project with
// {clientMutationId, workProject:{name, client:{id}, dueDate?, description?}}.
func ReplayProjectCreate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	name := strings.TrimSpace(firstFlag(flags, "name"))
	if name == "" {
		return nil, fmt.Errorf("project create requires --name")
	}
	custID := strings.TrimSpace(firstFlag(flags, "customer", "customer-id", "client-id"))
	if custID == "" {
		return nil, fmt.Errorf("project create requires --customer (v3 customer id)")
	}
	if _, err := strconv.Atoi(custID); err != nil {
		return nil, fmt.Errorf("--customer must be a numeric v3 customer id")
	}
	wp := map[string]any{"name": name, "client": map[string]any{"id": custID}}
	if v := strings.TrimSpace(firstFlag(flags, "due-date")); v != "" {
		wp["dueDate"] = v
	}
	if v := strings.TrimSpace(firstFlag(flags, "description", "memo", "notes")); v != "" {
		wp["description"] = v
	}
	op, err := gql.Lookup("CreateProject_workflow")
	if err != nil {
		return nil, err
	}
	input := map[string]any{
		"clientMutationId": fmt.Sprintf("qb-%d", time.Now().UnixNano()),
		"workProject":      wp,
	}
	data, err := workflowMutation(ctx, op.Name, op.Document, input)
	if err != nil {
		return nil, err
	}
	node := digMap(data, "createWork_Project", "workProjectEdge", "node")
	if strings.TrimSpace(strOf(node["id"])) == "" {
		return nil, fmt.Errorf("project create outcome unknown: missing createWork_Project receipt; inspect before retrying")
	}
	return &MutateResult{
		Status: http.StatusOK, Op: "create", Entity: "Work_Project",
		Note: "accountantworkflow createWork_Project",
		Item: QueryItem{ID: strOf(node["id"]), Name: strOf(node["name"])},
	}, nil
}

// ReplayProjectUpdate runs UpdateProject_workflow — updateWork_Project with
// {clientMutationId, workProject:{id, name?/description?/dueDate?/status?}}.
func ReplayProjectUpdate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	id := strings.TrimSpace(firstFlag(flags, "id"))
	if id == "" {
		return nil, fmt.Errorf("project update requires --id (Work_Project global id from project search)")
	}
	wp := map[string]any{"id": id}
	touched := false
	for _, pair := range [][2]string{
		{"name", "name"}, {"description", "description"}, {"memo", "description"}, {"notes", "description"},
		{"due-date", "dueDate"}, {"status", "status"}, {"entity-version", "entityVersion"},
	} {
		if v := strings.TrimSpace(firstFlag(flags, pair[0])); v != "" {
			if _, exists := wp[pair[1]]; !exists {
				wp[pair[1]] = v
				if pair[1] != "entityVersion" {
					touched = true
				}
			}
		}
	}
	if !touched {
		return nil, fmt.Errorf("project update needs a field to change (--name, --description, --due-date, --status)")
	}
	op, err := gql.Lookup("UpdateProject_workflow")
	if err != nil {
		return nil, err
	}
	input := map[string]any{
		"clientMutationId": fmt.Sprintf("qb-%d", time.Now().UnixNano()),
		"workProject":      wp,
	}
	data, err := workflowMutation(ctx, op.Name, op.Document, input)
	if err != nil {
		return nil, err
	}
	node := digMap(data, "updateWork_Project", "workProject")
	if got := strings.TrimSpace(strOf(node["id"])); got == "" || got != id {
		return nil, fmt.Errorf("project update outcome unknown: missing or mismatched updateWork_Project receipt; inspect before retrying")
	}
	return &MutateResult{
		Status: http.StatusOK, Op: "update", Entity: "Work_Project",
		Note: "accountantworkflow updateWork_Project",
		Item: QueryItem{ID: strOf(node["id"]), Name: strOf(node["name"])},
	}, nil
}

// ReplayProjectList runs the captured getJobs_workflow projection —
// company.projects(first, filterBy:"recurringProject=NULL && deleted='false'",
// orderBy:"name"). --id filters client-side on the Work_Project global id.
func ReplayProjectList(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 99999 // unbounded ask; service applies its own ceiling
	}
	doc := `query getJobs_workflow($pageSize: Int, $after: String, $filter: String, $order: String) { company { projects(first: $pageSize, after: $after, filterBy: $filter, orderBy: $order) { edges { node { ` + workProjectFields + ` } } } } }`
	payload, err := json.Marshal(map[string]any{
		"operationName": "getJobs_workflow",
		"variables": map[string]any{
			"pageSize": limit, "filter": "recurringProject=NULL && deleted='false'", "order": "name",
		},
		"query": doc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, accountantWorkflowURL, "accountantworkflow.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("getJobs_workflow: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("getJobs_workflow: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res, err := projectEdgeNodes("Work_Project", "getJobs_workflow @ accountantworkflow", raw,
		[]string{"data", "company", "projects", "edges"}, query, limit, resp.StatusCode)
	if err != nil {
		return nil, err
	}
	if id = strings.TrimSpace(id); id != "" {
		// Work_Project ids are global ids ending :<numericId> — match either.
		var keep []QueryItem
		for _, it := range res.Items {
			if it.ID == id || strings.HasSuffix(it.ID, ":"+id) {
				keep = append(keep, it)
			}
		}
		res.Items = keep
		res.Counts["items"] = len(keep)
	}
	return res, nil
}

func digMap(m map[string]any, path ...string) map[string]any {
	cur := m
	for _, k := range path {
		nx, ok := cur[k].(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = nx
	}
	return cur
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}
