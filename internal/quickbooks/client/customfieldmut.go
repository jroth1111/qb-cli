package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
)

// Custom-field definitions mutate through the spend-lists-xp
// createCommon/updateCommon_CustomFieldDefinition mutations on the
// smallbusiness GraphQL host (same schema family the CES read uses on
// customextensions). The mutations take a single typed input object.
const customFieldMutateHost = "smallbusiness.api.intuit.com"
const customFieldMutateURL = "https://smallbusiness.api.intuit.com/graphql"

// ReplayCustomFieldMutate executes createCommon_CustomFieldDefinition (op
// "create") or updateCommon_CustomFieldDefinition (op "update") with an
// input object assembled from flags: --name is required; --type maps to the
// CES data type; --entities optionally scopes associated entity types.
func ReplayCustomFieldMutate(ctx context.Context, op string, flags map[string]string) (*MutateResult, error) {
	name := strings.TrimSpace(flags["name"])
	opName := "CustomFieldDefinitionCreateMutation"
	if op == "update" {
		opName = "UpdateFieldDefinitionCreateMutation"
		if strings.TrimSpace(flags["id"]) == "" {
			return nil, fmt.Errorf("custom-field update requires --id")
		}
	}
	if name == "" && op == "create" {
		return nil, fmt.Errorf("custom-field create requires --name")
	}
	input := map[string]any{}
	if name != "" {
		input["name"] = name
	}
	if t := strings.TrimSpace(flags["type"]); t != "" {
		input["type"] = strings.ToUpper(t)
	}
	if id := strings.TrimSpace(flags["id"]); id != "" {
		input["id"] = id
	}
	if ents := strings.TrimSpace(flags["entities"]); ents != "" {
		parts := []string{}
		for e := range strings.SplitSeq(ents, ",") {
			if e = strings.TrimSpace(e); e != "" {
				parts = append(parts, strings.ToUpper(e))
			}
		}
		if len(parts) > 0 {
			input["associatedEntityTypes"] = parts
		}
	}
	gop, err := gql.Lookup(opName)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": gop.Name,
		"variables":     map[string]any{"input_0": input},
		"query":         gop.Document,
	})
	if err != nil {
		return nil, fmt.Errorf("custom-field %s: %w", opName, err)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, customFieldMutateURL, customFieldMutateHost, payload)
	if err != nil {
		return nil, fmt.Errorf("custom-field %s: %w", opName, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading custom-field %s: %w", opName, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(raw, &wrap)
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("custom-field %s: %s", opName, wrap.Errors[0].Message)
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     op,
		Entity: "CustomField",
		Item:   QueryItem{Type: "CustomField", Name: name},
		Note:   "smallbusiness " + opName,
	}, nil
}

// PlannedCustomFieldMutateURL reports the GraphQL host for dry-run plans.
func PlannedCustomFieldMutateURL() string { return customFieldMutateURL }
