package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const proposalURL = "https://crm-proposal-svc.api.intuit.com/v1/proposals?page=0&size=25&sortBy=UPDATED_DATE&sortDir=DESC"
const proposalHost = "crm-proposal-svc.api.intuit.com"

func PlannedProposalURL() string { return proposalURL }

func replayProposals(ctx context.Context, _, _ string, _ int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, proposalURL, proposalHost, nil)
	if err != nil {
		return nil, fmt.Errorf("proposal GET /v1/proposals: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading proposal GET /v1/proposals: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectProposals(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectProposals(body []byte) *QueryResult {
	var wrap struct {
		Proposals     []json.RawMessage `json:"proposals"`
		TotalElements int               `json:"totalElements"`
	}
	note := "crm-proposal-svc GET /v1/proposals"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "Proposal", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	n := wrap.TotalElements
	if n == 0 {
		n = len(wrap.Proposals)
	}
	return &QueryResult{
		Entity: "Proposal",
		Counts: map[string]int{"items": 0, "totalCount": n, "totalElements": wrap.TotalElements},
		Items:  []QueryItem{},
		Note:   fmt.Sprintf("%s totalElements=%d", note, wrap.TotalElements),
	}
}
