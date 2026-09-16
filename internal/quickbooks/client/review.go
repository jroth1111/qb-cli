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
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
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

// reviewItemKeys maps feedback-engagement response objects onto QueryItem.
var reviewItemKeys = itemKeys{
	id:     []string{"id", "Id", "responseId", "engagementId"},
	name:   []string{"name", "Name", "comment", "feedback", "title"},
	date:   []string{"submittedDate", "createdDate", "updatedDate", "date"},
	amount: []string{"rating", "score"},
	typeOf: []string{"status", "Status", "type"},
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
	items := projectJSONItems("Review", wrap.Responses, reviewItemKeys)
	return &QueryResult{
		Entity: "Review",
		Counts: map[string]int{"items": len(items), "totalCount": n},
		Items:  items,
		Note:   fmt.Sprintf("%s responses=%d", note, n),
	}
}
