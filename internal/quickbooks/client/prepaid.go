package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const prepaidGraphQLURL = "https://cbo.api.intuit.com/graphql"
const prepaidHost = "cbo.api.intuit.com"

const getSchedulesQuery = `query GetSchedules($first: PositiveInt, $after: String, $last: PositiveInt, $before: String, $filter: Cbo_ScheduleFilter) {
  cboSchedules(first: $first, after: $after, last: $last, before: $before, filter: $filter) {
    edges { node { id status amount homeAmount currency startDate endDate sourceTxnType } cursor }
    totalCount
    pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
  }
}`

func PlannedPrepaidURL() string { return prepaidGraphQLURL }

func replayGetSchedules(ctx context.Context, _, _ string, limit int) (*QueryResult, error) {
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
		"query": getSchedulesQuery,
		"variables": map[string]any{
			"first": limit,
			"filter": map[string]any{
				"createdAt": map[string]any{
					"onOrAfter":  "2026-07-01T00:00:00+10:00",
					"onOrBefore": "2027-06-30T23:59:59+10:00",
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("prepaid GetSchedules: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, prepaidGraphQLURL, prepaidHost, payload)
	if err != nil {
		return nil, fmt.Errorf("prepaid GetSchedules: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading prepaid GetSchedules: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectGetSchedules(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectGetSchedules(body []byte) *QueryResult {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Result struct {
				TotalCount int `json:"totalCount"`
			} `json:"cboSchedules"`
		} `json:"data"`
	}
	note := "cbo.api GetSchedules"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "PrepaidSchedule", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	total := wrap.Data.Result.TotalCount
	note = fmt.Sprintf("cbo.api GetSchedules totalCount=%d", total)
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "PrepaidSchedule",
		Counts: map[string]int{"items": 0, "totalCount": total},
		Items:  []QueryItem{},
		Note:   note,
	}
}
