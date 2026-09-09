package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const reviewURL = "https://mccrmmessaging.api.intuit.com/v1/feedback-engagement"
const reviewHost = "mccrmmessaging.api.intuit.com"

func PlannedReviewURL() string { return reviewURL }

func replayFeedbackEngagement(ctx context.Context, _, _ string, _ int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, reviewURL, reviewHost, nil)
	if err != nil {
		return nil, fmt.Errorf("review feedback-engagement: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading review feedback-engagement: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectFeedbackEngagement(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectFeedbackEngagement(body []byte) *QueryResult {
	var wrap struct {
		Responses []json.RawMessage `json:"responses"`
	}
	note := "mccrmmessaging GET /v1/feedback-engagement"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "Review", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	n := len(wrap.Responses)
	return &QueryResult{
		Entity: "Review",
		Counts: map[string]int{"items": 0, "totalCount": n},
		Items:  []QueryItem{},
		Note:   fmt.Sprintf("%s responses=%d", note, n),
	}
}
