package gql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

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
	rejected := result != nil && result.Status == http.StatusUnauthorized || errors.Is(err, auth.ErrRemintNeedsLogin)
	if rejected && method == http.MethodGet {
		rctx, cancel := context.WithTimeout(ctx, 100*time.Second)
		rerr := remintGQL(rctx)
		cancel()
		if rerr != nil {
			return nil, rerr
		}
		fresh, ferr := auth.Load()
		if ferr != nil || !auth.SameSession(tok, fresh) {
			return nil, auth.ErrSessionChanged
		}
		// Preserve caller-specific headers, replacing only credential fields
		// whose previous values match the session's captured values.
		updated := maps.Clone(headers)
		oldAuth, newAuth := authHeadersForToken(tok, endpoint), authHeadersFor(endpoint)
		for key, value := range headers {
			for oldKey, oldValue := range oldAuth {
				if strings.EqualFold(key, oldKey) && value == oldValue {
					if n := newAuth[oldKey]; n != "" {
						updated[key] = n
					} else {
						delete(updated, key)
					}
				}
			}
		}
		result, err = fetchManaged(ctx, fresh, endpoint, method, updated, body)
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("managed fetch returned no response")
	}
	return &Response{Status: result.Status, Body: json.RawMessage(result.Body)}, nil
}
