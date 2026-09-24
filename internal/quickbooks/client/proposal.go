package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
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

// proposalItemKeys maps crm-proposal-svc proposal objects onto QueryItem.
var proposalItemKeys = itemKeys{
	id:        []string{"proposalId", "id", "Id"},
	name:      []string{"name", "Name", "title", "subject", "proposalName"},
	date:      []string{"updatedDate", "lastModifiedDate", "modifiedDate", "createdDate", "sentDate", "date"},
	docNumber: []string{"docNumber", "DocNumber", "proposalNumber"},
	amount:    []string{"totalAmount", "amount", "Amount", "total"},
	typeOf:    []string{"status", "Status"},
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
	items := projectJSONItems("Proposal", wrap.Proposals, proposalItemKeys)
	return &QueryResult{
		Entity: "Proposal",
		Counts: map[string]int{"items": len(items), "totalCount": n, "totalElements": wrap.TotalElements},
		Items:  items,
		Note:   fmt.Sprintf("%s totalElements=%d", note, wrap.TotalElements),
	}
}

// PROPOSAL_CREATE — POST /v1/proposals on crm-proposal-svc, live-proven on
// TC2 2026-09-19 (POST {contactId,contactType} → 200 id:1002; PUT
// /v1/proposals/{refId} → 200; DELETE /v1/proposals/{numericId} → 200
// isDeleted:true — asymmetric id spaces: GET/PUT key by refId UUID, DELETE
// by numeric id). The service auto-names proposals "Proposal {id}" — a
// caller-supplied title is accepted but not displayed.
//
//	delete --id <numeric proposal id>
//	(create also proven: --customer <contactId> [--contact-type CUSTOMER])
func ReplayProposalDelete(ctx context.Context, id string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("proposal delete requires --id (numeric proposal id)")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodDelete,
		"https://crm-proposal-svc.api.intuit.com/v1/proposals/"+id, proposalHost, nil)
	if err != nil {
		return nil, fmt.Errorf("proposal DELETE: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := &MutateResult{Status: resp.StatusCode, Op: "delete", Entity: "Proposal",
		Item: QueryItem{Type: "Proposal", ID: id}}
	var out struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		IsDeleted bool   `json:"isDeleted"`
	}
	if json.Unmarshal(raw, &out) != nil || out.ID <= 0 || fmt.Sprint(out.ID) != id || !out.IsDeleted {
		return nil, fmt.Errorf("proposal deletion unverified; inspect proposal %s before retrying", id)
	}
	if out.ID != 0 {
		res.Item.ID = fmt.Sprintf("%d", out.ID)
		res.Item.Name = out.Title
		if out.IsDeleted {
			res.Item.Type = "Proposal (deleted)"
		}
	}
	res.Note = "crm-proposal-svc DELETE /v1/proposals/{id}"
	return res, nil
}

// ReplayProposalUpdate fetches a proposal by its refId UUID, applies the
// provided field overrides, and PUTs the full object back — the service
// requires a complete body, not a patch. Live-proven 2026-09-19.
func ReplayProposalUpdate(ctx context.Context, refID, title string) (*MutateResult, error) {
	refID = strings.TrimSpace(refID)
	title = strings.TrimSpace(title)
	if refID == "" {
		return nil, fmt.Errorf("proposal update requires --id (refId UUID)")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	base := "https://crm-proposal-svc.api.intuit.com/v1/proposals/" + refID
	getResp, err := ac.doURIHost(ctx, http.MethodGet, base, proposalHost, nil)
	if err != nil {
		return nil, fmt.Errorf("proposal GET for update: %w", err)
	}
	cur, err := readBody(getResp)
	_ = drainAndClose(getResp)
	if err != nil {
		return nil, err
	}
	if getResp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: getResp.StatusCode, Message: errorMessage(cur)}
	}
	var full map[string]any
	if err := json.Unmarshal(cur, &full); err != nil {
		return nil, fmt.Errorf("decoding proposal %s: %w", refID, err)
	}
	if full == nil || mapStr(full, "refId") != refID || jsonNumberString(full["id"]) == "" {
		return nil, fmt.Errorf("proposal before-state identity mismatch; no update sent")
	}
	if title != "" {
		full["title"] = title
	}
	body, _ := json.Marshal(full)
	resp, err := ac.doURIHost(ctx, http.MethodPut, base, proposalHost, body)
	if err != nil {
		return nil, fmt.Errorf("proposal PUT: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := &MutateResult{Status: resp.StatusCode, Op: "update", Entity: "Proposal",
		Item: QueryItem{Type: "Proposal", ID: refID}}
	var out struct {
		ID    int64  `json:"id"`
		RefID string `json:"refId"`
		Title string `json:"title"`
	}
	if json.Unmarshal(raw, &out) != nil || out.ID <= 0 || out.RefID != refID || fmt.Sprint(out.ID) != jsonNumberString(full["id"]) {
		return nil, fmt.Errorf("proposal update receipt unverified; inspect %s before retrying", refID)
	}
	if out.ID != 0 {
		res.Item.ID = fmt.Sprintf("%d", out.ID)
		res.Item.Name = out.Title
	}
	res.Note = "crm-proposal-svc GET+PUT /v1/proposals/{refId}"
	return res, nil
}

// ReplayProposalCreate posts a new proposal draft for a customer/lead
// contact. The service assigns id+refId and the display title
// "Proposal {id}"; --title is forwarded but the service display-name wins.
func ReplayProposalCreate(ctx context.Context, contactID, contactType, title string) (*MutateResult, error) {
	contactID = strings.TrimSpace(contactID)
	contactType = strings.ToUpper(strings.TrimSpace(contactType))
	title = strings.TrimSpace(title)
	if contactID == "" {
		return nil, fmt.Errorf("proposal create requires --customer (contactId, e.g. 1)")
	}
	if contactType == "" {
		contactType = "CUSTOMER"
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	bodyMap := map[string]any{"contactId": contactID, "contactType": contactType}
	if title != "" {
		bodyMap["title"] = title
	}
	body, _ := json.Marshal(bodyMap)
	resp, err := ac.doURIHost(ctx, http.MethodPost,
		"https://crm-proposal-svc.api.intuit.com/v1/proposals", proposalHost, body)
	if err != nil {
		return nil, fmt.Errorf("proposal POST: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := &MutateResult{Status: resp.StatusCode, Op: "create", Entity: "Proposal",
		Item: QueryItem{Type: "Proposal"}}
	var out struct {
		ID    int64  `json:"id"`
		RefID string `json:"refId"`
		Title string `json:"title"`
	}
	if json.Unmarshal(raw, &out) != nil || out.ID <= 0 || out.RefID == "" {
		return nil, fmt.Errorf("proposal create receipt missing identity; inspect before retrying")
	}
	if out.ID != 0 {
		res.Item.ID = fmt.Sprintf("%d", out.ID)
		res.Item.Name = out.Title
		if out.RefID != "" {
			res.Item.Type = "Proposal (refId " + out.RefID + ")"
		}
	}
	res.Note = "crm-proposal-svc POST /v1/proposals"
	return res, nil
}
