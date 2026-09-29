// customobjects.go implements the custom-object definition reader and the
// cost-group client against the two GraphQL services the QBO frontend uses
// for the /app/customobjects and /app/cost-groups pages.
//
// Endpoint evidence (/tmp/qbo-cap/deobfuscated, capture 2026-08):
//
//   - Cost groups: plugin.intuitcdn.net__cost-groups-ui__137.…/9538.js pins
//     PROD to https://cost-group-svc.api.intuit.com/graphql (env map E2E/QA/
//     PERF siblings -e2e/-qal/-prf). Module 4634.js carries the four captured
//     operations reproduced verbatim below: GetCostGroupsWithFilter,
//     CreateCostGroup, UpdateCostGroup, ToggleCostGroupDeletion, plus the
//     exact variable shapes (filter.deleted tri-state, orderBy NAME_ASC,
//     create input {name, description, type:"ITEM"}, update input
//     {id, name, description, version}, toggle input {id, version, deleted}).
//     Deletion is a toggle: deleted=true deletes, deleted=false re-activates;
//     version is optimistic-concurrency (SPA defaults it to 0 when absent).
//
//   - Custom objects: products-and-services-core 15126.js captures query
//     GetCustomObjectDefinitions over appFoundationsCustomObjectDefinitions;
//     qbo-contacts-v2 26746.js executes it with service: COS_SERVICE
//     ("cos-service"). No cos-service gqlUrl literal appears in any captured
//     bundle (the SPA resolves it through its runtime service registry), so
//     requests go to the qbo.intuit.com app-foundations GraphQL gateway this
//     CLI already drives (api/v4/graphql; live-proven 200 in .audit
//     LIVE_TEST_REPORT). [INFERENCE] flagged on that routing choice.
//
// No captured API exists for creating/updating/deleting custom-object
// DEFINITIONS (no appFoundations*CustomObject* mutation appears in any
// bundle), so those CLI verbs stay not-wired stubs; only list/get are wired
// here. Cost groups are fully wired: every op below is captured.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const costGroupsGraphQLURL = "https://cost-group-svc.api.intuit.com/graphql"
const costGroupsReferer = "https://qbo.intuit.com/app/cost-groups"

// customObjectsGraphQLURL routes the cos-service operation through the v4
// app-foundations gateway. [INFERENCE] — see file-header evidence note.
const customObjectsGraphQLURL = "https://qbo.intuit.com/api/v4/graphql"
const customObjectsReferer = "https://qbo.intuit.com/app/customobjects"

// PlannedCostGroupsURL is the secret-free dry-run URL (no realm segment —
// the service scopes by session cookie, not by URL).
func PlannedCostGroupsURL() string { return costGroupsGraphQLURL }

// PlannedCustomObjectsURL is the secret-free dry-run URL.
func PlannedCustomObjectsURL() string { return customObjectsGraphQLURL }

// Empty-input sentinels. Like ErrEmptyExcludeIDs they fire during planning,
// before any session load or dial, so callers cannot mistake a no-op for
// success.
var (
	ErrEmptyCostGroupName  = errors.New("cost group requires a non-empty name")
	ErrEmptyCostGroupID    = errors.New("cost group requires a non-empty id")
	ErrEmptyCustomObjectID = errors.New("custom object requires a non-empty id")
)

// CostGroup is one ProjectManagement_CostGroupPayload.costGroup row.
type CostGroup struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
	Version     int    `json:"version,omitempty"`
}

// CostGroupListResult is the secret-free envelope returned by
// ReplayCostGroupList. The captured query selects no totalCount, so Count is
// the only population figure reported.
type CostGroupListResult struct {
	Status int         `json:"status"`
	Count  int         `json:"count"`
	Groups []CostGroup `json:"groups"`
}

// CustomObjectAppliesTo is one appliesTo entry of a custom-object definition
// (which entity/scope/attribute the object extends).
type CustomObjectAppliesTo struct {
	Entity    string `json:"entity,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Attribute string `json:"attribute,omitempty"`
	Active    bool   `json:"active,omitempty"`
}

// CustomObjectDefinition is one appFoundationsCustomObjectDefinitions edge
// node.
type CustomObjectDefinition struct {
	ID        string                  `json:"id,omitempty"`
	Label     string                  `json:"label,omitempty"`
	Active    bool                    `json:"active,omitempty"`
	AppliesTo []CustomObjectAppliesTo `json:"appliesTo,omitempty"`
}

// CustomObjectDefinitionResult is the envelope returned by
// ReplayCustomObjectList.
type CustomObjectDefinitionResult struct {
	Status      int                      `json:"status"`
	Count       int                      `json:"count"`
	Definitions []CustomObjectDefinition `json:"definitions"`
}

// --- captured operations -----------------------------------------------------

const costGroupsListQuery = `query GetCostGroupsWithFilter($first: Int, $after: String, $before: String, $last: Int, $filter: ProjectManagement_CostGroupFilter!, $orderBy: [ProjectManagement_CostGroupSort!]) {
  projectManagementGetCostGroupWithFilter(first: $first, after: $after, before: $before, last: $last, costGroupFilter: $filter, orderBy: $orderBy) {
      edges {
        node {
          __typename
          ... on ProjectManagement_CostGroupPayload {
            costGroup {
              id
              name
              description
              type
              deleted
              version
            }
          }
        }
        cursor
      }
      pageInfo {
        hasNextPage
        hasPreviousPage
        startCursor
        endCursor
      }
    }
  }`

const costGroupCreateMutation = `mutation CreateCostGroup($input: ProjectManagement_CostGroupCreateInput!) {
  projectManagementCreateCostGroup(createCostGroupInput: $input) {
    ... on ProjectManagement_CostGroupPayload {
      costGroup {
        id
        name
        description
        type
        deleted
        version
      }
    }
    ... on ProjectManagement_CostGroupError {
      code
      message
    }
  }
}`

const costGroupUpdateMutation = `mutation UpdateCostGroup($input: ProjectManagement_CostGroupUpdateInput!) {
  projectManagementUpdateCostGroup(updateCostGroupInput: $input) {
    ... on ProjectManagement_CostGroupPayload {
      costGroup {
        id
        name
        description
        type
        deleted
        version
      }
    }
    ... on ProjectManagement_CostGroupError {
      code
      message
    }
  }
}`

const costGroupToggleMutation = `mutation ToggleCostGroupDeletion($input: ProjectManagement_CostGroupToggleDeletionInput!) {
  projectManagementToggleCostGroupDeletion(toggleCostGroupDeletionInput: $input) {
    ... on ProjectManagement_CostGroupPayload {
      costGroup {
        id
        name
        description
        type
        deleted
        version
      }
    }
    ... on ProjectManagement_CostGroupError {
      code
      message
    }
  }
}`

const customObjectDefinitionsQuery = `query GetCustomObjectDefinitions(
  $filterBy: AppFoundations_CustomObjectDefinitionFilterBy
) {
  appFoundationsCustomObjectDefinitions(filterBy: $filterBy) {
    edges {
      node {
        id
        label
        active
        appliesTo {
          entity
          scope
          attribute
          active
        }
      }
    }
  }
}`

// --- plans -------------------------------------------------------------------

// cgNote/coNote stamp every plan; writePlan prints them on --dry-run.
const (
	cgNote = "cost-group-svc graphql; not sent"
	coNote = "cos-service appFoundations via v4 gateway; not sent"
)

// cgNewPlan stamps a secret-free plan for one cost-group-svc call.
func cgNewPlan(op string, body []byte) *RequestPlan {
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: http.MethodPost,
		URL:    costGroupsGraphQLURL,
		Body:   json.RawMessage(body),
		Note:   cgNote,
		op:     op,
	}
}

// coNewPlan stamps a secret-free plan for one appFoundations call.
func coNewPlan(op string, body []byte) *RequestPlan {
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: http.MethodPost,
		URL:    customObjectsGraphQLURL,
		Body:   json.RawMessage(body),
		Note:   coNote,
		op:     op,
	}
}

// costGroupFilter builds the captured filter: ALL sends deleted=null (no
// constraint), ACTIVE false, INACTIVE true — always with the group type, which
// the schema demands NonNull (ProjectManagement_CostGroupType; SPA default
// ITEM). Unknown statuses fall back to ACTIVE, matching the SPA default branch.
func costGroupFilter(status, typ string) map[string]any {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		typ = "ITEM"
	}
	m := map[string]any{"type": typ}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "all":
		m["deleted"] = nil
	case "inactive", "deleted":
		m["deleted"] = true
	default: // "active" and anything unrecognised
		m["deleted"] = false
	}
	return m
}

// clampFirst normalizes the requested page size; the service enforces its
// own ceiling, so no client-side cap is applied here. A caller limit <= 0
// asks for everything — the service decides how much it returns.
func clampFirst(limit int) int {
	if limit < 1 {
		return 99999
	}
	return limit
}

// PlanCostGroupList builds the GetCostGroupsWithFilter POST.
func PlanCostGroupList(limit int, status, typ string) *RequestPlan {
	body, err := json.Marshal(map[string]any{
		"operationName": "GetCostGroupsWithFilter",
		"variables": map[string]any{
			"first":   clampFirst(limit),
			"after":   nil,
			"before":  nil,
			"last":    nil,
			"filter":  costGroupFilter(status, typ),
			"orderBy": []string{"NAME_ASC"},
		},
		"query": costGroupsListQuery,
	})
	if err != nil {
		// Marshalling a map[string]any of scalars cannot fail; guarded for
		// signature symmetry with the validating planners.
		return cgNewPlan("costgroups.list", []byte("{}"))
	}
	return cgNewPlan("costgroups.list", body)
}

// PlanCostGroupCreate builds the CreateCostGroup POST. An empty name is
// rejected (the UI enforces the same rule); type defaults to ITEM, the SPA
// default.
func PlanCostGroupCreate(name, description, typ string) (*RequestPlan, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyCostGroupName
	}
	if strings.TrimSpace(typ) == "" {
		typ = "ITEM"
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "CreateCostGroup",
		"variables": map[string]any{
			"input": map[string]any{
				"name":        name,
				"description": description,
				"type":        strings.TrimSpace(typ),
			},
		},
		"query": costGroupCreateMutation,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding cost group create: %w", err)
	}
	return cgNewPlan("costgroups.create", body), nil
}

// PlanCostGroupUpdate builds the UpdateCostGroup POST. Both id and name are
// required (the captured UI flow always sends both).
func PlanCostGroupUpdate(id, name, description string, version int) (*RequestPlan, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrEmptyCostGroupID
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyCostGroupName
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "UpdateCostGroup",
		"variables": map[string]any{
			"input": map[string]any{
				"id":          id,
				"name":        name,
				"description": description,
				"version":     version,
			},
		},
		"query": costGroupUpdateMutation,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding cost group update: %w", err)
	}
	return cgNewPlan("costgroups.update", body), nil
}

// PlanCostGroupDelete builds the ToggleCostGroupDeletion POST with
// deleted=true. Pass activate=true to send deleted=false instead (undo).
func PlanCostGroupDelete(id string, version int, activate bool) (*RequestPlan, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrEmptyCostGroupID
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "ToggleCostGroupDeletion",
		"variables": map[string]any{
			"input": map[string]any{
				"id":      id,
				"version": version,
				"deleted": !activate,
			},
		},
		"query": costGroupToggleMutation,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding cost group toggle: %w", err)
	}
	op := "costgroups.delete"
	if activate {
		op = "costgroups.activate"
	}
	return cgNewPlan(op, body), nil
}

// PlanCustomObjectList builds the GetCustomObjectDefinitions POST.
func PlanCustomObjectList() *RequestPlan {
	body, err := json.Marshal(map[string]any{
		"operationName": "GetCustomObjectDefinitions",
		"variables":     map[string]any{"filterBy": nil},
		"query":         customObjectDefinitionsQuery,
	})
	if err != nil {
		return coNewPlan("customobjects.list", []byte("{}"))
	}
	return coNewPlan("customobjects.list", body)
}

// PlanCustomObjectGet builds the same captured query; the id filter is
// applied client-side (the captured operation has no single-node selector).
// An empty id is rejected so callers cannot mistake an unfiltered page for a
// match.
func PlanCustomObjectGet(id string) (*RequestPlan, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrEmptyCustomObjectID
	}
	return coNewPlan("customobjects.get", planBodyOf(PlanCustomObjectList())), nil
}

func planBodyOf(p *RequestPlan) []byte { return []byte(p.Body) }

// --- executors ---------------------------------------------------------------

// postCostGroupGraphQLFn is the transport seam for cost-group-svc calls.
// Production routes through doURIHost: the host demands its own Intuit_APIKey
// (ATS keys 403 — proven TC2 2026-09-08 across header/auth/TLS/freshness
// hypotheses), sourced from the leftover-host capture map. extra is honored
// only by the test swap; doURIHost carries the per-host captured set.
var postCostGroupGraphQLFn = func(ctx context.Context, ac *apiClient, plan *RequestPlan, extra map[string]string) ([]byte, int, error) {
	resp, err := ac.doURIHost(ctx, http.MethodPost, plan.URL, hostOf(plan.URL), []byte(plan.Body))
	if err != nil {
		return nil, 0, err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", plan.op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return raw, resp.StatusCode, nil
}

// execPlannedGraphQL POSTs a plan to its service with the wrap-ATS header
// set (postJSONExtra) plus the SPA page Referer the GraphQL services expect.
// Writes additionally pass the production-realm guard.
func execPlannedGraphQL(ctx context.Context, plan *RequestPlan, referer string, write bool) ([]byte, int, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, 0, err
	}
	extra := map[string]string{
		"Referer":      referer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	if strings.Contains(plan.URL, "cost-group-svc") {
		return postCostGroupGraphQLFn(ctx, ac, plan, extra)
	}
	resp, err := ac.postJSONExtra(ctx, plan.URL, []byte(plan.Body), extra)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", plan.op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", plan.op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return raw, resp.StatusCode, nil
}

// --- projections -------------------------------------------------------------

// projectCostGroups decodes GetCostGroupWithFilter. Union nodes whose
// __typename is the error payload carry no costGroup and are skipped, matching
// the SPA's `.filter(t => t)`.
func projectCostGroups(raw []byte) ([]CostGroup, error) {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data *struct {
			Filtered *struct {
				Edges []struct {
					Node *struct {
						CostGroup *CostGroup `json:"costGroup"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"projectManagementGetCostGroupWithFilter"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("decoding cost groups: %w", err)
	}
	if len(wrap.Errors) > 0 {
		return nil, fmt.Errorf("cost-group-svc graphql: %s", wrap.Errors[0].Message)
	}
	if wrap.Data == nil || wrap.Data.Filtered == nil {
		return []CostGroup{}, nil
	}
	groups := []CostGroup{}
	for _, e := range wrap.Data.Filtered.Edges {
		if e.Node == nil || e.Node.CostGroup == nil || e.Node.CostGroup.ID == "" {
			continue
		}
		groups = append(groups, *e.Node.CostGroup)
	}
	return groups, nil
}

// projectCostGroupMutation decodes a create/update/toggle payload. A
// CostGroupError branch (code+message) surfaces as an error; a response with
// neither group nor error is reported honestly rather than invented.
func projectCostGroupMutation(raw []byte, field string) (*CostGroup, error) {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", field, err)
	}
	var payload struct {
		CostGroup *CostGroup `json:"costGroup"`
		Code      string     `json:"code"`
		Message   string     `json:"message"`
	}
	if msg, ok := wrap.Data[field]; ok {
		if err := json.Unmarshal(msg, &payload); err != nil {
			return nil, fmt.Errorf("decoding %s payload: %w", field, err)
		}
	}
	if payload.CostGroup != nil {
		return payload.CostGroup, nil
	}
	if payload.Message != "" {
		return nil, fmt.Errorf("cost-group-svc graphql: %s (%s)", payload.Message, payload.Code)
	}
	if len(wrap.Errors) > 0 {
		return nil, fmt.Errorf("cost-group-svc graphql: %s", wrap.Errors[0].Message)
	}
	return nil, fmt.Errorf("cost group not found in %s response", field)
}

// projectCustomObjectDefinitions decodes GetCustomObjectDefinitions.
func projectCustomObjectDefinitions(raw []byte) ([]CustomObjectDefinition, error) {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data *struct {
			Defs *struct {
				Edges []struct {
					Node *CustomObjectDefinition `json:"node"`
				} `json:"edges"`
			} `json:"appFoundationsCustomObjectDefinitions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("decoding custom object definitions: %w", err)
	}
	if len(wrap.Errors) > 0 {
		return nil, fmt.Errorf("appfoundations graphql: %s", wrap.Errors[0].Message)
	}
	if wrap.Data == nil || wrap.Data.Defs == nil {
		return []CustomObjectDefinition{}, nil
	}
	defs := []CustomObjectDefinition{}
	for _, e := range wrap.Data.Defs.Edges {
		if e.Node == nil || e.Node.ID == "" {
			continue
		}
		defs = append(defs, *e.Node)
	}
	return defs, nil
}

// --- replays -----------------------------------------------------------------

// ReplayCostGroupList runs GetCostGroupsWithFilter. Read-only. status is
// active (default) | inactive | all; typ defaults to ITEM like the SPA.
func ReplayCostGroupList(ctx context.Context, limit int, status, typ string) (*CostGroupListResult, error) {
	plan := PlanCostGroupList(limit, status, typ)
	raw, st, err := execPlannedGraphQL(ctx, plan, costGroupsReferer, false)
	if err != nil {
		return nil, err
	}
	groups, err := projectCostGroups(raw)
	if err != nil {
		return nil, err
	}
	return &CostGroupListResult{Status: st, Count: len(groups), Groups: groups}, nil
}

// ReplayCostGroupCreate posts CreateCostGroup and projects the created group.
func ReplayCostGroupCreate(ctx context.Context, name, description, typ string) (*MutateResult, error) {
	plan, err := PlanCostGroupCreate(name, description, typ)
	if err != nil {
		return nil, err
	}
	raw, st, err := execPlannedGraphQL(ctx, plan, costGroupsReferer, true)
	if err != nil {
		return nil, err
	}
	cg, err := projectCostGroupMutation(raw, "projectManagementCreateCostGroup")
	if err != nil {
		return nil, err
	}
	return mutateResult(st, "create", cg), nil
}

// ReplayCostGroupUpdate posts UpdateCostGroup. version is optimistic
// concurrency; the SPA sends the value previously read (0 when unknown).
func ReplayCostGroupUpdate(ctx context.Context, id, name, description string, version int) (*MutateResult, error) {
	plan, err := PlanCostGroupUpdate(id, name, description, version)
	if err != nil {
		return nil, err
	}
	raw, st, err := execPlannedGraphQL(ctx, plan, costGroupsReferer, true)
	if err != nil {
		return nil, err
	}
	cg, err := projectCostGroupMutation(raw, "projectManagementUpdateCostGroup")
	if err != nil {
		return nil, err
	}
	return mutateResult(st, "update", cg), nil
}

// ReplayCostGroupDelete toggles deletion on (deleted=true). activate undoes
// it. The toggle is soft: rows stay visible under the inactive filter.
func ReplayCostGroupDelete(ctx context.Context, id string, version int, activate bool) (*MutateResult, error) {
	plan, err := PlanCostGroupDelete(id, version, activate)
	if err != nil {
		return nil, err
	}
	raw, st, err := execPlannedGraphQL(ctx, plan, costGroupsReferer, true)
	if err != nil {
		return nil, err
	}
	cg, err := projectCostGroupMutation(raw, "projectManagementToggleCostGroupDeletion")
	if err != nil {
		return nil, err
	}
	op := "delete"
	if activate {
		op = "activate"
	}
	return mutateResult(st, op, cg), nil
}

func mutateResult(status int, op string, cg *CostGroup) *MutateResult {
	note := "cost-group-svc.api.intuit.com/graphql " + op
	return &MutateResult{
		Status: status,
		Op:     op,
		Entity: "CostGroup",
		Item:   QueryItem{ID: cg.ID, Name: cg.Name, Type: "CostGroup"},
		Note:   note,
	}
}

// ReplayCustomObjectList runs GetCustomObjectDefinitions. Read-only.
func ReplayCustomObjectList(ctx context.Context) (*CustomObjectDefinitionResult, error) {
	plan := PlanCustomObjectList()
	raw, st, err := execPlannedGraphQL(ctx, plan, customObjectsReferer, false)
	if err != nil {
		return nil, err
	}
	defs, err := projectCustomObjectDefinitions(raw)
	if err != nil {
		return nil, err
	}
	return &CustomObjectDefinitionResult{Status: st, Count: len(defs), Definitions: defs}, nil
}

// ReplayCustomObjectGet filters the captured list query down to one id.
func ReplayCustomObjectGet(ctx context.Context, id string) (*CustomObjectDefinition, error) {
	plan, err := PlanCustomObjectGet(id)
	if err != nil {
		return nil, err
	}
	raw, _, err := execPlannedGraphQL(ctx, plan, customObjectsReferer, false)
	if err != nil {
		return nil, err
	}
	defs, err := projectCustomObjectDefinitions(raw)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	for i := range defs {
		if defs[i].ID == id {
			return &defs[i], nil
		}
	}
	return nil, fmt.Errorf("custom object %s not found in %d definition(s)", id, len(defs))
}
