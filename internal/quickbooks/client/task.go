package client

// TASKS_CREATE / TASKS_EDIT / TASKS_DELETE — QBO task CRUD on
// smallbusiness.api.intuit.com/graphql, reverse-engineered from the tasks-ui
// plugin bundle and live-proven on TC2 2026-09-19 (create→1802244811,
// rename, delete success:true, list back to 0).
//
// The task-management-service requires namespace context the GraphQL input
// does NOT carry: the gateway reads it from request headers the tasks-ui
// client injects — intuit_target_domain/intuit_target_usecase plus the
// plugin's own Intuit_APIKey and originating assetalias. Without them every
// mutation fails NAMESPACE_SHOULD_NOT_BE_EMPTY ("Domain and usecase values
// must be provided") regardless of the input namespace field.
//
// Create requires name, status ("Open"), assignee (identity profile id, not
// the folio createdUserId), and an RFC3339 dueDate. Update takes id:Long!
// plus mutable fields — version is not required. Delete takes id:Long! and
// returns {task, success}.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const taskGQLURL = "https://smallbusiness.api.intuit.com/graphql"
const taskGQLHost = "smallbusiness.api.intuit.com"

// PlannedTaskURL is the dry-run URL for task mutations.
func PlannedTaskURL() string { return taskGQLURL }

// taskHeaders returns the tasks-ui request-header overrides the
// task-management-service requires on mutations (captured from the tasks-ui
// bundle: targetDomain/targetUsecase + the plugin apikey and assetalias).
func taskHeaders() map[string]string {
	return map[string]string{
		"intuit_target_domain":          "QBO",
		"intuit_target_usecase":         "TASK_MANAGER",
		"intuit-plugin-id":              "tasks-ui",
		"intuit_originating_assetalias": "Intuit.practice.project.tasksui",
		"Authorization":                 "Intuit_APIKey intuit_apikey=prdakyresBSLk02axlIe7sb1iNZAzFLfZDXm4waB,intuit_apikey_version=1.0",
	}
}

// taskAssigneeID resolves the session user's identity profile id — the
// assignee value taskManagementCreateTask requires ("Assignee Id must not be
// null"). The folio createdUserId is a different id space and is rejected.
func taskAssigneeID(ctx context.Context, ac *apiClient) (string, error) {
	doc := `query Users($input: Identity_UserFilterInput!, $pagination: PaginationInput!) {
        identityUsers(input: $input, pagination: $pagination) {
            edges { node { profile { id } } }
        }
    }`
	body, _ := json.Marshal(map[string]any{
		"query": doc,
		"variables": map[string]any{
			"input":      map[string]any{"accountFilter": map[string]any{"accountId": ac.realm}},
			"pagination": map[string]any{"first": 50},
		},
	})
	resp, err := ac.doURIHost(ctx, http.MethodPost, "https://identity.api.intuit.com/v2/graphql", "identity.api.intuit.com", body)
	if err != nil {
		return "", fmt.Errorf("resolving assignee identity: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out struct {
		Data struct {
			IdentityUsers struct {
				Edges []struct {
					Node struct {
						Profile struct {
							ID string `json:"id"`
						} `json:"profile"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"identityUsers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("malformed identityUsers %.200s", raw)
	}
	profiles := map[string]struct{}{}
	for _, e := range out.Data.IdentityUsers.Edges {
		if id := strings.TrimSpace(e.Node.Profile.ID); id != "" {
			profiles[id] = struct{}{}
		}
	}
	if len(profiles) == 1 {
		for id := range profiles {
			return id, nil
		}
	}
	if len(profiles) > 1 {
		return "", fmt.Errorf("multiple identity profiles found for this company; pass --assignee explicitly")
	}
	return "", fmt.Errorf("no identity profile found for this company — pass --assignee explicitly")
}

// taskGQL posts one task-management document with the required header
// overrides and returns the decoded data envelope.
func taskGQL(ctx context.Context, ac *apiClient, doc string) (json.RawMessage, []byte, error) {
	body, _ := json.Marshal(map[string]any{"query": doc})
	resp, err := ac.doURIHost(ctx, http.MethodPost, taskGQLURL, taskGQLHost, body, taskHeaders())
	if err != nil {
		return nil, nil, fmt.Errorf("task mutation: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, raw, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, raw, fmt.Errorf("malformed task response %.200s", raw)
	}
	if len(env.Errors) > 0 {
		return nil, raw, fmt.Errorf("%s", env.Errors[0].Message)
	}
	return env.Data, raw, nil
}

// taskNode is the task projection used for MutateResult items.
type taskNode struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Assignee string `json:"assignee"`
	DueDate  string `json:"dueDate"`
}

// taskResult projects a mutation data envelope onto MutateResult.
func taskResult(data json.RawMessage, op, expectedID string) (*MutateResult, error) {
	item := QueryItem{Type: "Task"}
	var wrap struct {
		Create *struct {
			Task *taskNode `json:"task"`
		} `json:"taskManagementCreateTask"`
		Update *struct {
			Task *taskNode `json:"task"`
		} `json:"taskManagementUpdateTask"`
		Delete *struct {
			Task    *taskNode `json:"task"`
			Success bool      `json:"success"`
		} `json:"taskManagementDeleteTask"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("task %s: malformed receipt: %w", op, err)
	}
	var t *taskNode
	switch op {
	case "create":
		if wrap.Create != nil {
			t = wrap.Create.Task
		}
	case "update":
		if wrap.Update != nil {
			t = wrap.Update.Task
		}
	case "delete":
		if wrap.Delete != nil {
			t = wrap.Delete.Task
		}
	}
	if t == nil || t.ID == 0 {
		return nil, fmt.Errorf("task %s outcome unknown: missing task receipt; inspect before retrying", op)
	}
	item.ID = fmt.Sprintf("%d", t.ID)
	item.Name = t.Name
	if t.Status != "" {
		item.Type = "Task (" + t.Status + ")"
	}
	if expectedID != "" && item.ID != expectedID {
		return nil, fmt.Errorf("task %s outcome unknown: receipt id %q does not match requested id %q; inspect before retrying", op, item.ID, expectedID)
	}
	if op == "delete" && (wrap.Delete == nil || !wrap.Delete.Success) {
		return nil, fmt.Errorf("task delete outcome unknown: success receipt is false; inspect before retrying")
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     op,
		Entity: "Task",
		Item:   item,
		Note:   "task-management-service " + op,
	}, nil
}

// ReplayTaskMutate creates, updates, or deletes a QBO task.
//
//	create --name X --due 2026-09-30 [--assignee <identity-profile-id>]
//	update --id 1802244811 --name X
//	delete --id 1802244811
func ReplayTaskMutate(ctx context.Context, op, id, name, due, assignee string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	due = strings.TrimSpace(due)
	assignee = strings.TrimSpace(assignee)
	switch op {
	case "create":
		if name == "" {
			return nil, fmt.Errorf("tasks create requires --name")
		}
		if due == "" {
			return nil, fmt.Errorf("tasks create requires --due (YYYY-MM-DD or RFC3339) — the service rejects null dueDate")
		}
		if _, err := taskDue(due); err != nil {
			return nil, err
		}
	case "update":
		if id == "" {
			return nil, fmt.Errorf("tasks update requires --id (numeric task id)")
		}
		if name == "" {
			return nil, fmt.Errorf("tasks update requires --name (the only field proven mutable)")
		}
	case "delete":
		if id == "" {
			return nil, fmt.Errorf("tasks delete requires --id (numeric task id)")
		}
	default:
		return nil, fmt.Errorf("unknown tasks op %q", op)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	switch op {
	case "create":
		if assignee == "" {
			assignee, err = taskAssigneeID(ctx, ac)
			if err != nil {
				return nil, err
			}
		}
		dueRFC, _ := taskDue(due)
		doc := fmt.Sprintf(`mutation { taskManagementCreateTask(input:{name:%s,status:"Open",assignee:"%s",dueDate:"%s"}) { task { id name status assignee dueDate } } }`,
			gqlString(name), assignee, dueRFC)
		data, _, err := taskGQL(ctx, ac, doc)
		if err != nil {
			return nil, err
		}
		return taskResult(data, "create", "")
	case "update":
		doc := fmt.Sprintf(`mutation { taskManagementUpdateTask(input:{id:%s,name:%s}) { task { id name status } } }`,
			id, gqlString(name))
		data, _, err := taskGQL(ctx, ac, doc)
		if err != nil {
			return nil, err
		}
		return taskResult(data, "update", id)
	case "delete":
		doc := fmt.Sprintf(`mutation { taskManagementDeleteTask(input:{id:%s}) { task { id name } success } }`, id)
		data, _, err := taskGQL(ctx, ac, doc)
		if err != nil {
			return nil, err
		}
		return taskResult(data, "delete", id)
	}
	return nil, fmt.Errorf("unknown tasks op %q", op)
}

// taskDue normalises a due date to RFC3339 — accepts YYYY-MM-DD or a full
// RFC3339 timestamp.
func taskDue(v string) (string, error) {
	if _, err := time.Parse(time.RFC3339, v); err == nil {
		return v, nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("invalid --due %q — use YYYY-MM-DD or RFC3339", v)
}

// gqlString renders a Go string as a GraphQL string literal.
func gqlString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
