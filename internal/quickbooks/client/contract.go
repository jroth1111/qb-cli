package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const contractEnvelopesURL = "https://esign.platform.intuit.com/v1/modified-envelopes?appname=Intuit.crm.marketing.crmdocbuilderui&provider=INTUITSIGN&excludeStatus=deleted"
const contractHost = "esign.platform.intuit.com"

func PlannedContractURL() string { return contractEnvelopesURL }

func replayModifiedEnvelopes(ctx context.Context, _, _ string, _ int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, contractEnvelopesURL, contractHost, nil)
	if err != nil {
		return nil, fmt.Errorf("contract modified-envelopes: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading contract modified-envelopes: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectModifiedEnvelopes(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectModifiedEnvelopes(body []byte) *QueryResult {
	note := "esign.platform GET /v1/modified-envelopes"
	var arr []json.RawMessage
	if json.Unmarshal(body, &arr) == nil {
		return &QueryResult{
			Entity: "Contract",
			Counts: map[string]int{"items": 0, "totalCount": len(arr)},
			Items:  []QueryItem{},
			Note:   fmt.Sprintf("%s n=%d", note, len(arr)),
		}
	}
	return &QueryResult{Entity: "Contract", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
}
