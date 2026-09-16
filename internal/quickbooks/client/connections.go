package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Captured 2026-09-15 from the authenticated apptransactions SPA. The
// first-party QBO v4 gateway accepts the saved ATS key; the v4.api host
// rejects the same replay with 403 unless a separate per-host capture exists.
const connectionsGraphQLURL = "https://qbo.intuit.com/api/v4/graphql"
const connectionsReferer = "https://qbo.intuit.com/app/apptransactions"

const connectionsQuery = `query connections($filterBy: String!) {
  company {
    connections(filterBy: $filterBy) {
      edges {
        node {
          id
          meta {
            created
            updated
            __typename
          }
          provider {
            name
            providerAppId
            settings {
              edges {
                node {
                  id
                  name
                  label
                  settingsGroupName
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          status
          connectionAccount {
            edges {
              node {
                id
                account {
                  id
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          settings {
            edges {
              node {
                id
                providerSettings {
                  id
                  name
                  settingsGroupName
                  __typename
                }
                value
                parentSetting {
                  id
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          bankTrait {
            lastCorrelationId
            fiCredSetId
            fdpProviderId
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}
`

const connectionsFilterBy = "status='CONNECTED' AND provider.providerAppId IN ('QBCOMMERCE_EBAY','QBCOMMERCE_AMAZON','QBCOMMERCE_SHOPIFY','QBCOMMERCE_ETSY','5dba2520-ada9-466e-8954-bffc3dc0520a','b7t2yes3zg','b7uapdq6u6','d4c181d6-58ae-4dcd-9f9c-f425a43e123f','PAYPAL_IDX','SQUARE_IDX','STRIPE_IDX','SQUARESPACE_IDX','WIX_IDX','SHOPIFY_IDX','ETSY_IDX','EBAY_IDX','AMAZON_IDX','WOOCOMMERCE_IDX','BIGCOMMERCE_IDX')"

func PlannedConnectionsURL() string { return connectionsGraphQLURL }

// ReplayConnections POSTs the captured apptransactions connections GraphQL
// via wrap ATS (same leftover_ats_probe extras). Read-only. Empty edges is
// an honest empty list, not a remap onto Purchase.
func ReplayConnections(ctx context.Context, id string, limit int) (*QueryResult, error) {
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
	body, err := json.Marshal(map[string]any{
		"operationName": "connections",
		"variables":     map[string]any{"filterBy": connectionsFilterBy},
		"query":         connectionsQuery,
	})
	if err != nil {
		return nil, err
	}
	extra := map[string]string{
		"Referer":      connectionsReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, connectionsGraphQLURL, body, extra)
	if err != nil {
		return nil, fmt.Errorf("connections graphql: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading connections: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	items, note, err := projectConnections(raw)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id != "" {
		filtered := items[:0]
		for _, it := range items {
			if it.ID == id {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	if len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		items = []QueryItem{}
	}
	return &QueryResult{
		Status: resp.StatusCode,
		Entity: "connections",
		Counts: map[string]int{"items": len(items)},
		Items:  items,
		Note:   note,
	}, nil
}

func projectConnections(raw []byte) ([]QueryItem, string, error) {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data *struct {
			Company *struct {
				Connections *struct {
					Edges []struct {
						Node *struct {
							ID       string `json:"id"`
							Status   string `json:"status"`
							Provider *struct {
								Name          string `json:"name"`
								ProviderAppID string `json:"providerAppId"`
							} `json:"provider"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"connections"`
			} `json:"company"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, "", fmt.Errorf("connections: %w", err)
	}
	if wrap.Data == nil || wrap.Data.Company == nil || wrap.Data.Company.Connections == nil {
		if len(wrap.Errors) > 0 {
			return nil, "", fmt.Errorf("connections graphql: %s", wrap.Errors[0].Message)
		}
		return []QueryItem{}, "200 empty connections (no data)", nil
	}
	edges := wrap.Data.Company.Connections.Edges
	items := make([]QueryItem, 0, len(edges))
	for _, e := range edges {
		if e.Node == nil {
			continue
		}
		it := QueryItem{ID: e.Node.ID, Type: e.Node.Status}
		if e.Node.Provider != nil {
			it.Name = e.Node.Provider.Name
			if it.Name == "" {
				it.Name = e.Node.Provider.ProviderAppID
			}
		}
		items = append(items, it)
	}
	note := ""
	if len(items) == 0 {
		note = "200 empty connections.edges=[]"
	}
	return items, note, nil
}
