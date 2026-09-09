package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	defer drainAndClose(resp)
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
	return &QueryResult{
		Entity: "Workflow",
		Counts: map[string]int{"items": 0, "totalCount": n},
		Items:  []QueryItem{},
		Note:   note,
	}
}
