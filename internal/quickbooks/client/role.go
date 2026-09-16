package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Captured 2026-08-19 on /app/usermgt (open-39 follow-up).
// POST roles.api.intuit.com/v1/graphql query roles/accountRoles → 200 list items=4.
// Honest custom-roles / role list — not Employee stand-in.
const roleGraphQLURL = "https://roles.api.intuit.com/v1/graphql"
const roleHost = "roles.api.intuit.com"

const roleQuery = `query roles($accountId: String!) {
  custom: accountRoles(
    filterBy: {accountId: $accountId, roleOfferingId: "Intuit.sbe.salsa.default"}
  ) {
    edges {
      node {
        id
        canonicalName
        name
        description
        roleType
        state
        __typename
      }
      __typename
    }
    __typename
  }
}`

func PlannedRoleURL() string { return roleGraphQLURL }

func replayAccountRoles(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
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
	payload, err := json.Marshal(map[string]any{
		"operationName": "roles",
		"variables":     map[string]any{"accountId": ac.realm},
		"query":         roleQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("roles accountRoles: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, roleGraphQLURL, roleHost, payload)
	if err != nil {
		return nil, fmt.Errorf("roles accountRoles: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading roles accountRoles: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectAccountRoles(raw, query, limit)
	res.Status = resp.StatusCode
	return res, nil
}

func projectAccountRoles(body []byte, query string, limit int) *QueryResult {
	note := "roles.api accountRoles"
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Custom struct {
				Edges []struct {
					Node struct {
						ID            string `json:"id"`
						CanonicalName string `json:"canonicalName"`
						Name          string `json:"name"`
						Description   string `json:"description"`
						RoleType      string `json:"roleType"`
						State         string `json:"state"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"custom"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "Role", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(wrap.Data.Custom.Edges))
	for _, e := range wrap.Data.Custom.Edges {
		n := e.Node
		it := QueryItem{ID: n.ID, Name: firstNonEmpty(n.Name, n.CanonicalName), Type: n.RoleType}
		if q != "" && !strings.Contains(strings.ToLower(it.ID+" "+it.Name+" "+it.Type+" "+n.Description), q) {
			continue
		}
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	note = fmt.Sprintf("%s edges=%d items=%d", note, len(wrap.Data.Custom.Edges), len(items))
	return &QueryResult{
		Entity: "Role",
		Counts: map[string]int{"items": len(items), "totalCount": len(wrap.Data.Custom.Edges)},
		Items:  items,
		Note:   note,
	}
}
