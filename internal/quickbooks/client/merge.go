package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// CUSTOMER_MERGE / SUPPLIER_MERGE — the real merge contract is the customer
// form's "Merge contacts" confirmation: after a rename collides, the UI posts
// an updateContact_qbo mutation carrying mergedTo instead of a new name.
//
//	POST https://qbo.intuit.com/api/v4/graphql
//	{"operationName":"updateContact_qbo","variables":{"input":{
//	  "clientMutationId":"<uuid>","networkContact":{
//	    "id":"<ns-id>:<dup>", "entityVersion":"0",
//	    "mergedTo":{"id":"<ns-id>:<keep>"},
//	    "profiles":{"customer":{"active":true,"currency":"AUD"}}}}},...}
//
// The v3 API rejects the rename shortcut (6240 Duplicate Name Exists) — merge
// only works through this v4 contract. Contact ids are Relay-namespaced
// ("djQu…:31"); the suffix is the v3 id and the prefix is resolved per record
// via findContact_qbo (filterBy displayName + profile type). Merging is not
// reversible in QBO — the dup's record folds into the keep record.
const (
	mergeGraphQLURL = "https://qbo.intuit.com/api/v4/graphql"
	mergeReferer    = "https://qbo.intuit.com/app/customerdetail"
)

const mergeFindQuery = `query findContact_qbo($filterBy: String) {
  company {
    contacts(first: 50, filterBy: $filterBy) {
      totalCount
      edges { node { id displayName externalIds { localId namespaceId } profiles { customer { currency } vendor { currency } } } }
    }
  }
}`

const mergeUpdateMutation = `mutation updateContact_qbo($input: UpdateNetwork_ContactInput!) {
  updateNetwork_Contact(input: $input) {
    clientMutationId
    networkContact { id displayName externalIds { localId } }
  }
}`

// PlannedMergeURL reports the mutation URL for dry-run plans.
func PlannedMergeURL() string { return mergeGraphQLURL }

// gqlPost sends one GraphQL operation through the impersonated transport and
// returns the decoded envelope.
func gqlPost(ctx context.Context, ac *apiClient, operation, query string, vars map[string]any, referer string) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": operation,
		"query":         query,
		"variables":     vars,
	})
	if err != nil {
		return nil, err
	}
	extra := map[string]string{
		"Referer":      referer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, mergeGraphQLURL, payload, extra)
	if err != nil {
		return nil, err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return raw, nil
}

// gqlError extracts the first GraphQL error message, if any.
func gqlError(raw []byte) string {
	var out struct {
		Errors []struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Errors) == 0 {
		return ""
	}
	if out.Errors[0].Code != "" {
		return out.Errors[0].Code + ": " + out.Errors[0].Message
	}
	return out.Errors[0].Message
}

// namespacedContactID resolves a display name to the Relay-namespaced contact
// id the v4 layer requires (e.g. "djQu…:31"). profile is "customer" or "vendor".
func namespacedContactID(ctx context.Context, ac *apiClient, displayName, profile string) (string, error) {
	filter := fmt.Sprintf("displayName='%s' and profiles.%s!=null",
		strings.ReplaceAll(displayName, "'", "\\'"), profile)
	raw, err := gqlPost(ctx, ac, "findContact_qbo", mergeFindQuery,
		map[string]any{"filterBy": filter}, mergeReferer)
	if err != nil {
		return "", fmt.Errorf("findContact %q: %w", displayName, err)
	}
	if msg := gqlError(raw); msg != "" {
		return "", fmt.Errorf("findContact %q: %s", displayName, msg)
	}
	var out struct {
		Data struct {
			Company struct {
				Contacts struct {
					Edges []struct {
						Node struct {
							ID          string `json:"id"`
							DisplayName string `json:"displayName"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"contacts"`
			} `json:"company"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("findContact parse: %w", err)
	}
	for _, e := range out.Data.Company.Contacts.Edges {
		if strings.EqualFold(e.Node.DisplayName, displayName) {
			return e.Node.ID, nil
		}
	}
	return "", fmt.Errorf("no %s contact named %q in the network graph", profile, displayName)
}

// ReplayMerge merges one customer or supplier into another via the captured
// mergedTo contract. keep/merge accept a display name or v3 id; entity is
// "Customer" or "Vendor". The merge record folds into keep — irreversible.
func ReplayMerge(ctx context.Context, entity, keep, merge string) (*MutateResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	profile := "customer"
	if strings.EqualFold(entity, "Vendor") {
		profile = "vendor"
	} else if !strings.EqualFold(entity, "Customer") {
		return nil, fmt.Errorf("merge supports Customer or Vendor, not %q", entity)
	}
	keep = strings.TrimSpace(keep)
	merge = strings.TrimSpace(merge)
	if keep == "" || merge == "" {
		return nil, fmt.Errorf("merge requires --keep <name|id> and --merge <name|id>")
	}
	if strings.EqualFold(keep, merge) {
		return nil, fmt.Errorf("keep and merge are the same record %q — nothing to merge", keep)
	}
	keepID, err := resolveNameID(ctx, entity, keep)
	if err != nil {
		return nil, fmt.Errorf("keep: %w", err)
	}
	mergeID, err := resolveNameID(ctx, entity, merge)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}
	if keepID == mergeID {
		return nil, fmt.Errorf("keep and merge resolve to the same record %s", keepID)
	}
	keepName, err := entityDisplayName(ctx, entity, keepID)
	if err != nil {
		return nil, fmt.Errorf("keep %s: %w", keepID, err)
	}
	mergeName, err := entityDisplayName(ctx, entity, mergeID)
	if err != nil {
		return nil, fmt.Errorf("merge %s: %w", mergeID, err)
	}
	keepNS, err := namespacedContactID(ctx, ac, keepName, profile)
	if err != nil {
		return nil, fmt.Errorf("keep contact: %w", err)
	}
	mergeNS, err := namespacedContactID(ctx, ac, mergeName, profile)
	if err != nil {
		return nil, fmt.Errorf("merge contact: %w", err)
	}
	raw, err := gqlPost(ctx, ac, "updateContact_qbo", mergeUpdateMutation,
		map[string]any{"input": map[string]any{
			"clientMutationId": "0",
			"networkContact": map[string]any{
				"id":            mergeNS,
				"entityVersion": "0",
				"mergedTo":      map[string]string{"id": keepNS},
				"profiles": map[string]any{
					profile: map[string]any{"active": true, "currency": "AUD"},
				},
			},
		}}, mergeReferer)
	if err != nil {
		return nil, fmt.Errorf("merge %s→%s: %w", mergeName, keepName, err)
	}
	if msg := gqlError(raw); msg != "" {
		return nil, fmt.Errorf("merge %s→%s: %s", mergeName, keepName, msg)
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "merge",
		Entity: entity,
		Item:   QueryItem{ID: keepID, Name: fmt.Sprintf("%s merged into %s", mergeName, keepName)},
	}, nil
}

// entityDisplayName fetches a record's DisplayName by v3 id. The where-Id
// query returns at most one row.
func entityDisplayName(ctx context.Context, entity, id string) (string, error) {
	res, err := ReplayQuery(ctx, entity, id, "", 1)
	if err != nil {
		return "", err
	}
	if len(res.Items) == 0 || res.Items[0].ID != id {
		return "", fmt.Errorf("no %s with id %s", entity, id)
	}
	return res.Items[0].Name, nil
}
