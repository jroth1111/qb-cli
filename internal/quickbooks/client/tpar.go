package client

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
)

//go:embed tpar_instances_body.json
var tparInstancesBodyJSON []byte

const tparInstancesURL = "https://universalreportinsights.api.intuit.com/v1/reports/qbo/sbg:8c5dfdf2-5543-41ec-a00f-a3288e3f643d::TAXABLE_PAYMENTS/instances?locale=en-au&includeFirstPage=true"
const tparHost = "universalreportinsights.api.intuit.com"

func PlannedTPARURL() string { return tparInstancesURL }

func applyTPARDateRange(payload map[string]any, start, end string) {
	if start == "" || end == "" {
		return
	}
	filters, ok := payload["filters"].([]any)
	if !ok || len(filters) == 0 {
		return
	}
	f0, ok := filters[0].(map[string]any)
	if !ok {
		return
	}
	val, ok := f0["value"].(map[string]any)
	if !ok {
		return
	}
	val["macroName"] = "CUSTOM"
	val["startDate"] = start
	val["endDate"] = end
}

func replayTPARInstances(ctx context.Context, start, end string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(tparInstancesBodyJSON, &payload); err != nil {
		return nil, fmt.Errorf("tpar instances body: %w", err)
	}
	applyTPARDateRange(payload, start, end)
	if pag, ok := payload["paginationOptions"].(map[string]any); ok {
		pag["pageSize"] = limit
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("tpar instances body: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, tparInstancesURL, tparHost, body)
	if err != nil {
		return nil, fmt.Errorf("tpar instances: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading tpar instances: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectTPARInstances(raw)
	res.Status = resp.StatusCode
	return res, nil
}

func projectTPARInstances(body []byte) *QueryResult {
	var wrap struct {
		Status   string `json:"status"`
		Metadata struct {
			TotalRows int `json:"totalRows"`
		} `json:"metadata"`
		Data struct {
			Rows json.RawMessage `json:"rows"`
		} `json:"data"`
	}
	note := "universalreportinsights TAXABLE_PAYMENTS/instances"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "TPAR", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	total := wrap.Metadata.TotalRows
	note = fmt.Sprintf("universalreportinsights TAXABLE_PAYMENTS/instances totalRows=%d status=%s", total, wrap.Status)
	return &QueryResult{
		Entity: "TPAR",
		Counts: map[string]int{"items": 0, "totalRows": total},
		Items:  []QueryItem{},
		Note:   note,
	}
}

func replayTPARReport(ctx context.Context, start, end string) (*ReportResult, error) {
	q, err := replayTPARInstances(ctx, start, end, 20)
	if err != nil {
		return nil, err
	}
	header := map[string]any{
		"ReportName": "TAXABLE_PAYMENTS",
		"source":     "universalreportinsights TAXABLE_PAYMENTS/instances",
	}
	if start != "" {
		header["StartPeriod"] = start
	}
	if end != "" {
		header["EndPeriod"] = end
	}
	return &ReportResult{
		Status:  q.Status,
		Report:  "TAXABLE_PAYMENTS",
		Counts:  q.Counts,
		Header:  header,
		Columns: []string{},
		Note:    q.Note,
	}, nil
}
