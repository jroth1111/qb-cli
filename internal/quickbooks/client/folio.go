package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const folioURL = "https://universalreportinsights.api.intuit.com/v1/folio?locale=en-au&subType=CRB_GROUP"
const folioHost = "universalreportinsights.api.intuit.com"

func PlannedFolioURL() string { return folioURL }

func replayFolio(ctx context.Context, _, _ string, _ int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, folioURL, folioHost, nil)
	if err != nil {
		return nil, fmt.Errorf("folio GET /v1/folio: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading folio GET /v1/folio: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectFolio(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectFolio(body []byte) *QueryResult {
	note := "universalreportinsights GET /v1/folio"
	var arr []json.RawMessage
	if json.Unmarshal(body, &arr) == nil {
		return &QueryResult{
			Entity: "Folio",
			Counts: map[string]int{"items": 0, "totalCount": len(arr)},
			Items:  []QueryItem{},
			Note:   fmt.Sprintf("%s n=%d", note, len(arr)),
		}
	}
	return &QueryResult{Entity: "Folio", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
}
