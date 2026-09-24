package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const savedReportBase = "https://universalreportinsights.api.intuit.com/v1/reports/qbo/"

// PlannedSavedReportURL is the dry-run URL for saved-report create/update.
func PlannedSavedReportURL() string { return savedReportBase }

// REPORT_CREATE/EDIT/DELETE — saved custom reports (reportKindEnum
// CRB_REPORT) on universalreportinsights.api.intuit.com /v1/reports/qbo,
// captured from the report builder's "Save as new report" dialog and
// live-proven on TC2 2026-09-19:
//
//	POST   /v1/reports/qbo/?modelDimensionsAsLinkedRef=true {name,dataRequest} → {"id":"sbg:…"}
//	GET    /v1/reports/qbo/{id}                                               → full report
//	PUT    /v1/reports/qbo/{id}?modelDimensionsAsLinkedRef=true {name,dataRequest}
//	DELETE /v1/reports/qbo/{id}                                               → soft (active:false, "(deleted - …)" suffix)
//	PUT    /v1/reports/qbo/{id}/share?type=private|team                       → share scope
//
//	create --name X [--clone sbg:…] [--share private|team]
//	update --id sbg:… --name X
//	delete --id sbg:…
func ReplaySavedReportMutate(ctx context.Context, op, id, name, cloneID, share string) (*MutateResult, error) {
	op = strings.ToLower(strings.TrimSpace(op))
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	cloneID = strings.TrimSpace(cloneID)
	share = strings.ToLower(strings.TrimSpace(share))
	if share != "" && share != "private" && share != "team" {
		return nil, fmt.Errorf("share must be private or team; no report created")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	switch op {
	case "create":
		if name == "" {
			return nil, fmt.Errorf("report create requires --name")
		}
		var dataRequest json.RawMessage
		if cloneID != "" {
			dr, err := fetchReportDataRequest(ctx, ac, cloneID)
			if err != nil {
				return nil, err
			}
			dataRequest = dr
		} else {
			dataRequest = defaultDataRequest()
		}
		body, _ := json.Marshal(map[string]any{"name": name, "dataRequest": dataRequest})
		resp, err := ac.doURIHost(ctx, http.MethodPost,
			savedReportBase+"?modelDimensionsAsLinkedRef=true", "universalreportinsights.api.intuit.com", body)
		if err != nil {
			return nil, fmt.Errorf("saved report POST: %w", err)
		}
		raw, code, err := readAndClose(resp)
		if err != nil {
			return nil, err
		}
		if code != http.StatusOK {
			return nil, &ReplayError{Status: code, Message: errorMessage(raw)}
		}
		var out struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &out)
		if out.ID == "" {
			return nil, fmt.Errorf("saved report create receipt missing identity; inspect before retrying")
		}
		res := &MutateResult{Status: code, Op: "create", Entity: "Report",
			Item: QueryItem{Type: "SavedReport", ID: out.ID, Name: name},
			Note: "universalreportinsights POST /v1/reports/qbo/"}
		if share != "" {
			if err := shareReport(ctx, ac, out.ID, share); err != nil {
				return nil, fmt.Errorf("report %s created but sharing unverified; inspect before retrying: %w", out.ID, err)
			}
			res.Note += " + share?type=" + share
		}
		return res, nil
	case "update":
		if id == "" || name == "" {
			return nil, fmt.Errorf("report update requires --id and --name")
		}
		dr, err := fetchReportDataRequest(ctx, ac, id)
		if err != nil {
			return nil, err
		}
		body, _ := json.Marshal(map[string]any{"name": name, "dataRequest": dr})
		resp, err := ac.doURIHost(ctx, http.MethodPut,
			savedReportBase+url.PathEscape(id)+"?modelDimensionsAsLinkedRef=true",
			"universalreportinsights.api.intuit.com", body)
		if err != nil {
			return nil, fmt.Errorf("saved report PUT: %w", err)
		}
		raw, code, err := readAndClose(resp)
		if err != nil {
			return nil, err
		}
		if code != http.StatusOK {
			return nil, &ReplayError{Status: code, Message: errorMessage(raw)}
		}
		if err := validateSavedReportReceipt(raw, id, false); err != nil {
			return nil, err
		}
		return &MutateResult{Status: code, Op: "update", Entity: "Report",
			Item: QueryItem{Type: "SavedReport", ID: id, Name: name},
			Note: "universalreportinsights GET+PUT /v1/reports/qbo/{id}"}, nil
	case "delete":
		if id == "" {
			return nil, fmt.Errorf("report delete requires --id (sbg:…)")
		}
		resp, err := ac.doURIHost(ctx, http.MethodDelete,
			savedReportBase+url.PathEscape(id), "universalreportinsights.api.intuit.com", nil)
		if err != nil {
			return nil, fmt.Errorf("saved report DELETE: %w", err)
		}
		raw, code, err := readAndClose(resp)
		if err != nil {
			return nil, err
		}
		if code != http.StatusOK {
			return nil, &ReplayError{Status: code, Message: errorMessage(raw)}
		}
		if err := validateSavedReportReceipt(raw, id, true); err != nil {
			return nil, err
		}
		return &MutateResult{Status: code, Op: "delete", Entity: "Report",
			Item: QueryItem{Type: "SavedReport (deleted)", ID: id},
			Note: "universalreportinsights DELETE /v1/reports/qbo/{id}"}, nil
	}
	return nil, fmt.Errorf("unknown saved-report op %q", op)
}

func validateSavedReportReceipt(raw []byte, id string, deleted bool) error {
	type receipt struct {
		ID     string `json:"id"`
		Active *bool  `json:"active"`
	}
	var value receipt
	if json.Unmarshal(raw, &value) != nil {
		return fmt.Errorf("saved report receipt invalid; inspect %s before retrying", id)
	}
	if value.ID == "" {
		var envelope struct {
			Report receipt `json:"report"`
		}
		_ = json.Unmarshal(raw, &envelope)
		value = envelope.Report
	}
	if value.ID != id || (deleted && (value.Active == nil || *value.Active)) || (!deleted && value.Active != nil && !*value.Active) {
		return fmt.Errorf("saved report receipt has no matching identity or expected state; inspect %s before retrying", id)
	}
	return nil
}

// fetchReportDataRequest GETs a saved report and returns its dataRequest
// for clone/update resend.
func fetchReportDataRequest(ctx context.Context, ac *apiClient, id string) (json.RawMessage, error) {
	resp, err := ac.doURIHost(ctx, http.MethodGet,
		savedReportBase+url.PathEscape(id), "universalreportinsights.api.intuit.com", nil)
	if err != nil {
		return nil, fmt.Errorf("saved report GET %s: %w", id, err)
	}
	raw, code, err := readAndClose(resp)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, &ReplayError{Status: code, Message: errorMessage(raw)}
	}
	var wrap struct {
		Report struct {
			DataRequest json.RawMessage `json:"dataRequest"`
		} `json:"report"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil || len(wrap.Report.DataRequest) == 0 {
		return nil, fmt.Errorf("saved report %s: no dataRequest in response", id)
	}
	return wrap.Report.DataRequest, nil
}

// shareReport PUTs the share scope the builder dialog sends after save.
func shareReport(ctx context.Context, ac *apiClient, id, shareType string) error {
	resp, err := ac.doURIHost(ctx, http.MethodPut,
		savedReportBase+url.PathEscape(id)+"/share?type="+url.QueryEscape(shareType),
		"universalreportinsights.api.intuit.com", nil)
	if err != nil {
		return fmt.Errorf("saved report share PUT: %w", err)
	}
	raw, code, err := readAndClose(resp)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return &ReplayError{Status: code, Message: errorMessage(raw)}
	}
	return validateSavedReportReceipt(raw, id, false)
}

// readAndClose drains, closes, and returns the body + status — the
// drainAndClose+readBody pair used across the mutate clients.
func readAndClose(resp *http.Response) ([]byte, int, error) {
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	return raw, resp.StatusCode, err
}

// defaultDataRequest is the minimal valid spec a from-scratch custom report
// carries (live-proven 200 on TC2): a fiscal-YTD date filter plus one
// account-name projection on the transaction-details dataset.
func defaultDataRequest() json.RawMessage {
	return json.RawMessage(`{"filters":[{"on":{"datasetId":"TRANSACTION_DETAILS_V1","name":"DETAIL_TRANSACTION_DATE"},"operator":"DATE_MACRO","isPrimary":true,"value":{"macroName":"THIS_FISCAL_YEAR_TO_DATE"}}],"groups":[],"projections":[{"id":"p1","of":{"operation":null,"on":[{"name":"DETAIL_ACCOUNT_ID_REF","datasetId":"TRANSACTION_DETAILS_V1","type":"REF"}]},"as":"ACCOUNT","type":"RAW"}]}`)
}
