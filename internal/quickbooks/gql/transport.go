package gql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// ErrNeedsLogin mirrors auth.ErrRemintNeedsLogin: the relay is unreachable
// or no authenticated qbo.intuit.com tab exists, so a session-bound GraphQL
// POST cannot be executed. The user must run `qb auth login` first.
var ErrNeedsLogin = errors.New("no authenticated qbo.intuit.com tab; run `qb auth login`")

// DefaultTimeout bounds a single in-page fetch.
const DefaultTimeout = 30 * time.Second

// FitTransactionsEndpoint is the prod FIT GraphQL host the SPA routes
// banking-feed mutations to (endpointType "fitGql" → fitransactions), even
// when the capture corpus recorded them against qbo.intuit.com/api/v4.
const FitTransactionsEndpoint = "https://fitransactions.api.intuit.com/graphql"

// Request is one GraphQL POST.
type Request struct {
	Op        *Op
	Variables map[string]any
	// Endpoint overrides op.Endpoint when non-empty (used by tests).
	Endpoint string
}

// Response carries both transport and application outcomes distinctly:
// Status/Body describe the HTTP exchange; Errors holds the GraphQL
// `errors` array from a 200-with-errors response.
type Response struct {
	Status int             `json:"status"`           // HTTP status from the in-page fetch
	Body   json.RawMessage `json:"body"`             // raw response body
	Errors []GraphQLError  `json:"errors,omitempty"` // GraphQL errors array, if any
}

// GraphQLError is one entry of a GraphQL response's errors array.
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

func (e GraphQLError) Error() string { return e.Message }

// Data decodes the response body's data field into v.
func (r *Response) Data(v any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &envelope); err != nil {
		return fmt.Errorf("gql: response body is not a GraphQL envelope: %w", err)
	}
	if len(envelope.Data) == 0 {
		return errors.New("gql: response has no data field")
	}
	if v == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, v)
}

// Execute posts req inside the authenticated browser context via the
// ego-browser QBO space. It never replays saved headers offline — those 401.
// (The OMP-relay CDP path was retired: CONTRACT.md forbids Target.* CDP and
// the relay is gone; ego helpers are the sanctioned browser channel.)
func Execute(ctx context.Context, req Request) (*Response, error) {
	// No company gate: the session realm rules. Credential validation
	// happens in the ego execution path.
	return executeEgoFn(ctx, req)
}

// executeEgoFn runs the ego-browser execution. Tests replace it so Execute
// is exercised without a browser.
var executeEgoFn = ExecuteEgo

// RelayURL resolves the CDP relay endpoint: $QB_RELAY_URL or the default
// loopback port used by the OMP relay.
func RelayURL() string {
	if v := strings.TrimRight(os.Getenv("QB_RELAY_URL"), "/"); v != "" {
		return v
	}
	return auth.DefaultRelayURL
}

// authHeadersFor returns the per-host auth headers observed on live browser
// traffic. Cookies travel automatically via credentials:'include'; these are
// the additional headers the SPA's clients attach.
func authHeadersFor(endpoint string, operations ...string) map[string]string {
	tok, err := auth.Load()
	if err != nil || tok == nil {
		return nil
	}
	h := map[string]string{}
	if u, err := url.Parse(endpoint); err == nil {
		if captured := tok.URIHostHeaders[u.Hostname()]; len(captured) > 0 {
			// A captured service request defines its own CORS/auth surface.
			// Do not carry banking-only headers into a different service.
			for key, value := range captured {
				switch name := strings.ToLower(key); name {
				case "authorization", "apikey", "authtype", "csrftoken", "x-csrf-token", "intuit_appid":
					if value != "" {
						h[name] = value
					}
				}
			}
			if len(operations) > 0 && operations[0] == "TaskManagementTasks" {
				for _, key := range []string{"intuit_target_domain", "intuit_target_usecase", "intuit_originating_assetalias", "intuit-plugin-id"} {
					if value := captured[key]; value != "" {
						h[key] = value
					}
				}
			}
			if tok.RealmID != "" {
				h["intuit-company-id"] = tok.RealmID
			}
			return h
		}
	}
	// Proven service hosts authenticate with the secondary `apikey`
	// credential as an Intuit_APIKey Authorization header — NOT the
	// first-party banking key — and reject apikey/authtype/csrftoken headers
	// at CORS preflight (verified live 2026-09-16 on both hosts).
	// x-supergraph-version pins the v2 result shape the SPA reads on
	// commercecontrol; smallbusiness answers without it.
	serviceHosts := map[string]bool{
		"commercecontrol.api.intuit.com": true,
		"smallbusiness.api.intuit.com":   true,
	}
	if u, err := url.Parse(endpoint); err == nil && serviceHosts[u.Hostname()] {
		if tok.APIKey != "" {
			h["authorization"] = "Intuit_APIKey intuit_apikey=" + tok.APIKey + ",intuit_apikey_version=1.0"
		}
		if tok.RealmID != "" {
			h["intuit-company-id"] = tok.RealmID
		}
		if csrf := csrfFromToken(tok); csrf != "" {
			h["x-csrf-token"] = csrf
		}
		if u.Hostname() == "commercecontrol.api.intuit.com" {
			h["x-supergraph-version"] = "2"
		}
		if len(operations) > 0 {
			if op, lerr := Lookup(operations[0]); lerr == nil && op.Module != "" {
				h["intuit-plugin-id"] = op.Module
			}
		}
		return h
	}
	// warehouse-management-svc rejects apikey/authtype/csrftoken at CORS
	// preflight (proven TC2 2026-09-08: TypeError with them, HTTP reach
	// without); the v4 gateway and cost-group-svc accept the full set.
	warehouse := strings.Contains(endpoint, "warehouse-management-svc")
	if !warehouse && tok.APIKey != "" {
		h["apikey"] = tok.APIKey
	}
	if tok.Authorization != "" {
		h["authorization"] = tok.Authorization
	}
	if !warehouse && tok.TokenType != "" {
		h["authtype"] = tok.TokenType
	}
	if tok.RealmID != "" {
		h["intuit-company-id"] = tok.RealmID
	}
	if tok.IntuitAppID != "" {
		h["intuit_appid"] = tok.IntuitAppID
	}
	if !warehouse {
		if csrf := csrfFromToken(tok); csrf != "" {
			h["csrftoken"] = csrf
		}
	}

	return h
}

// csrfFromToken extracts the qbo.csrftoken value from saved cookies.
func csrfFromToken(tok *auth.TokenSet) string {
	for _, c := range tok.Cookies {
		if c.Name == "qbo.csrftoken" {
			return c.Value
		}
	}
	return ""
}
