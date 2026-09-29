package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// QBO mileage rides the trips service: REST /v1/trips/tripsSummary plus the
// trips GraphQL surface at /v4/graphql (company.trips filterBy). Live-proven
// on Test Company 2 (2026-09-17): tripsSummary 200 (all-zero fleet), trips
// query returns edges=[] with filter "deleted=false". The qbo v4
// MileageListQuery op is a different, broken surface — do not use it.

const tripsHost = "trips.api.intuit.com"
const tripsGraphQLURL = "https://trips.api.intuit.com/v4/graphql"

const tripsListDoc = `query MileageListQuery($filter: String!) {
    company { trips (filterBy: $filter) { edges { node {
        id distance { value unit } startDateTime { dateTime } endDateTime { dateTime } autoTracked
        notes entityVersion deleted
        vehicle { id description }
    } } } }
}`

// ReplayTripsList queries company.trips on the trips host. --id filters on the
// node id (or its numeric suffix); --query substring-filters node JSON.
func ReplayTripsList(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": "MileageListQuery",
		"variables":     map[string]any{"filter": "deleted=false"},
		"query":         tripsListDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, tripsGraphQLURL, tripsHost, payload)
	if err != nil {
		return nil, fmt.Errorf("trips MileageListQuery: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("trips MileageListQuery: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res, err := projectEdgeNodes("Trip", "trips.api.intuit.com MileageListQuery", raw,
		[]string{"data", "company", "trips", "edges"}, query, limit, resp.StatusCode)
	if err != nil {
		return nil, err
	}
	if id = strings.TrimSpace(id); id != "" {
		var keep []QueryItem
		for _, it := range res.Items {
			if it.ID == id || strings.HasSuffix(it.ID, ":"+id) {
				keep = append(keep, it)
			}
		}
		res.Items = keep
		res.Counts["items"] = len(keep)
	}
	return res, nil
}
