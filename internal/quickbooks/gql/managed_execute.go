package gql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

var fetchManaged = auth.FetchManaged

func executeManaged(ctx context.Context, tok *auth.TokenSet, req Request) (*Response, error) {
	if req.Op == nil {
		return nil, fmt.Errorf("gql: missing operation")
	}
	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = req.Op.Endpoint
	}
	if endpoint == "" {
		endpoint = EndpointDefault
	}
	variables := req.Variables
	if variables == nil {
		variables = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{"operationName": req.Op.Name, "variables": variables, "query": req.Op.Document})
	if err != nil {
		return nil, fmt.Errorf("gql: encoding request")
	}
	headers := authHeadersFor(endpoint, req.Op.Name)
	if headers == nil {
		headers = map[string]string{}
	}
	headers["content-type"] = "application/json"
	headers["accept"] = "application/json"
	result, err := fetchManaged(ctx, tok, endpoint, "POST", headers, payload)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Status < 100 || result.Status > 599 {
		return nil, errors.New("managed browser returned no valid HTTP response")
	}
	response := &Response{Status: result.Status, Body: json.RawMessage(result.Body)}
	var envelope struct {
		Errors []GraphQLError `json:"errors"`
	}
	if json.Unmarshal(response.Body, &envelope) == nil {
		response.Errors = envelope.Errors
	}
	return response, nil
}

func fetchManagedSession(ctx context.Context, tok *auth.TokenSet, endpoint, method string, headers map[string]string, body []byte) (*Response, error) {
	result, err := fetchManaged(ctx, tok, endpoint, method, headers, body)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Status < 100 || result.Status > 599 {
		return nil, errors.New("managed fetch returned no valid HTTP response")
	}
	return &Response{Status: result.Status, Body: json.RawMessage(result.Body)}, nil
}

func expectsJSONResponse(headers map[string]string) bool {
	for key, value := range headers {
		if strings.EqualFold(key, "accept") && (strings.Contains(strings.ToLower(value), "application/json") || strings.Contains(strings.ToLower(value), "+json")) {
			return true
		}
	}
	return false
}

func refreshedBrowserHeaders(old, fresh *auth.TokenSet, endpoint string, headers map[string]string) map[string]string {
	updated := maps.Clone(headers)
	oldAuth, newAuth := authHeadersForToken(old, endpoint), authHeadersForToken(fresh, endpoint)
	for key, value := range headers {
		for oldKey, oldValue := range oldAuth {
			if strings.EqualFold(key, oldKey) && value == oldValue {
				if next := newAuth[oldKey]; next != "" {
					updated[key] = next
				} else {
					delete(updated, key)
				}
			}
		}
	}
	return updated
}
