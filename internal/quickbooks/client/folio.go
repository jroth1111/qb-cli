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
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
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

// folioItemKeys maps universalreportinsights folio objects onto QueryItem.
var folioItemKeys = itemKeys{
	id:     []string{"id", "Id", "folioId"},
	name:   []string{"name", "Name", "title", "reportName"},
	date:   []string{"updatedDate", "createdDate", "generatedDate", "date"},
	typeOf: []string{"status", "Status", "subType", "type"},
}

func projectFolio(body []byte) *QueryResult {
	note := "universalreportinsights GET /v1/folio"
	var arr []json.RawMessage
	if json.Unmarshal(body, &arr) == nil {
		items := projectJSONItems("Folio", arr, folioItemKeys)
		return &QueryResult{
			Entity: "Folio",
			Counts: map[string]int{"items": len(items), "totalCount": len(arr)},
			Items:  items,
			Note:   fmt.Sprintf("%s n=%d", note, len(arr)),
		}
	}
	return &QueryResult{Entity: "Folio", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
}
