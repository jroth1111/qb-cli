package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const customFieldGraphQLURL = "https://customextensions.api.intuit.com/graphql"
const customFieldHost = "customextensions.api.intuit.com"

const customFieldsQueryCES = `query CustomFieldsQueryCES {
  customFieldDefinitions(limit: 1000) {
    edges {
      node {
        id
        name
        deleted
        __typename
      }
      __typename
    }
    __typename
  }
}`

func PlannedCustomFieldURL() string { return customFieldGraphQLURL }

func replayCustomFieldsQueryCES(ctx context.Context, _, _ string, _ int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "CustomFieldsQueryCES",
		"variables":     map[string]any{},
		"query":         customFieldsQueryCES,
	})
	if err != nil {
		return nil, fmt.Errorf("custom-field CustomFieldsQueryCES: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, customFieldGraphQLURL, customFieldHost, payload)
	if err != nil {
		return nil, fmt.Errorf("custom-field CustomFieldsQueryCES: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading custom-field CustomFieldsQueryCES: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectCustomFieldsQueryCES(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectCustomFieldsQueryCES(body []byte) *QueryResult {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Defs struct {
				Edges []json.RawMessage `json:"edges"`
			} `json:"customFieldDefinitions"`
		} `json:"data"`
	}
	note := "customextensions CustomFieldsQueryCES"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "CustomField", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	n := len(wrap.Data.Defs.Edges)
	note = fmt.Sprintf("customextensions CustomFieldsQueryCES edges=%d", n)
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "CustomField",
		Counts: map[string]int{"items": 0, "totalCount": n},
		Items:  []QueryItem{},
		Note:   note,
	}
}
