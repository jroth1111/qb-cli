package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// TxnAttachResult reports the Attachable now linked to a posted QBO
// transaction. AttachableID is the v3 Attachable id (file upload or note).
type TxnAttachResult struct {
	Status       int    `json:"status"`
	AttachableID string `json:"attachable_id"`
	TxnType      string `json:"txn_type"`
	TxnID        string `json:"txn_id"`
	FileName     string `json:"file_name,omitempty"`
	Note         string `json:"note,omitempty"`
}

var (
	errAttachMissingID   = errors.New("attach requires --id (posted transaction id)")
	errAttachMissingType = errors.New("attach requires --txn-type (v3 entity type of the target record)")
	errAttachMissingBody = errors.New("attach requires --note and/or --file")
)

// attachableEntityURL is the v3 Attachable entity endpoint used to create
// note attachables and to sparse-link uploaded files to a transaction.
// Contract verified live TC2 2026-09-19: sparse {Id,SyncToken,AttachableRef}
// persists the link; note-only create returns a new Attachable id; unlink is
// a full update without AttachableRef; delete is POST ?operation=delete.
func attachableEntityURL(realm string) string {
	return "https://qbo.intuit.com/api/v3/company/" + realm + "/attachable?minorversion=73"
}

// attachableEntityRef builds the EntityRef entry AttachableRef carries.
func attachableEntityRef(txnType, txnID string) map[string]any {
	return map[string]any{
		"EntityRef": map[string]any{"type": txnType, "value": txnID},
	}
}

// postAttachable POSTs one Attachable body to the v3 entity endpoint and
// returns the decoded Attachable object. Non-200 surfaces as ReplayError.
func postAttachable(ctx context.Context, ac *apiClient, body map[string]any) (map[string]any, error) {
	var before map[string]any
	op := "create"
	if id := jsonNumberString(body["Id"]); id != "" {
		var err error
		before, err = fetchV3(ctx, ac, "attachable", id)
		if err != nil {
			return nil, err
		}
		op = "update"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding attachable request: %w", err)
	}
	submittingMutation(ctx)
	resp, err := ac.postJSON(ctx, attachableEntityURL(ac.realm), raw)
	if err != nil {
		return nil, fmt.Errorf("attachable post: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	out, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("attachable post: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(out)}
	}
	var env struct {
		Attachable map[string]any `json:"Attachable"`
	}
	if err := json.Unmarshal(out, &env); err != nil || env.Attachable == nil {
		return nil, fmt.Errorf("attachable post: unexpected response shape")
	}
	if err := verifyV3Readback(ctx, ac, "Attachable", op, jsonNumberString(env.Attachable["Id"]), before, body, ""); err != nil {
		return nil, err
	}
	confirmMutation(ctx)
	return env.Attachable, nil
}

// ReplayTxnAttach attaches a file and/or a note to a posted QBO transaction
// via the v3 Attachable entity — the same link the web app's transaction
// attachments ride (verified live TC2 2026-09-19 on Invoice 7):
//
//   - file: UploadAttachable (POST /upload) then a sparse
//     POST /attachable {Id,SyncToken:"0",AttachableRef:[{EntityRef}]}
//   - note (no file): POST /attachable {Note,AttachableRef:[{EntityRef}]}
//     creates a note-only Attachable already linked.
//
// --txn-type is the v3 entity type of --id (Invoice, Bill, Purchase,
// Transfer, ...). At least one of note/content is required.
func ReplayTxnAttach(ctx context.Context, txnType, txnID, note, filename string, content []byte) (*TxnAttachResult, error) {
	txnID = sanitizeToken(txnID)
	txnType = strings.TrimSpace(txnType)
	note = strings.TrimSpace(note)
	if txnID == "" {
		return nil, errAttachMissingID
	}
	if txnType == "" {
		return nil, errAttachMissingType
	}
	if note == "" && len(content) == 0 {
		return nil, errAttachMissingBody
	}
	res := &TxnAttachResult{TxnType: txnType, TxnID: txnID, Note: note}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if len(content) > 0 {
		up, err := UploadAttachable(ctx, filename, content)
		if err != nil {
			return nil, err
		}
		res.FileName = up.FileName
		link := map[string]any{
			"Id":            up.ID,
			"SyncToken":     "0",
			"sparse":        true,
			"AttachableRef": []any{attachableEntityRef(txnType, txnID)},
		}
		if note != "" {
			link["Note"] = note
		}
		if _, err := postAttachable(ctx, ac, link); err != nil {
			return nil, err
		}
		res.AttachableID = up.ID
		res.Status = http.StatusOK
		return res, nil
	}
	created, err := postAttachable(ctx, ac, map[string]any{
		"Note":          note,
		"AttachableRef": []any{attachableEntityRef(txnType, txnID)},
	})
	if err != nil {
		return nil, err
	}
	id, _ := created["Id"].(string)
	if id == "" {
		return nil, fmt.Errorf("attachable create: unexpected response shape")
	}
	res.AttachableID = id
	res.Status = http.StatusOK
	return res, nil
}

// PlanTxnAttach describes the attach call for dry-run without opening a
// socket. Validation matches ReplayTxnAttach.
func PlanTxnAttach(txnType, txnID, note, filename string) (*RequestPlan, error) {
	txnID = sanitizeToken(txnID)
	txnType = strings.TrimSpace(txnType)
	if txnID == "" {
		return nil, errAttachMissingID
	}
	if txnType == "" {
		return nil, errAttachMissingType
	}
	if strings.TrimSpace(note) == "" && strings.TrimSpace(filename) == "" {
		return nil, errAttachMissingBody
	}
	step := "POST /api/v3/company/{realm}/attachable {Note,AttachableRef}"
	if strings.TrimSpace(filename) != "" {
		step = "POST /api/v3/company/{realm}/upload then POST /attachable {Id,SyncToken,AttachableRef}"
	}
	body, _ := json.Marshal(map[string]any{
		"txn_type": txnType,
		"txn_id":   txnID,
		"note":     note,
		"file":     filename,
	})
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: http.MethodPost,
		URL:    attachableEntityURL(realmToken),
		Body:   json.RawMessage(body),
		Note:   "v3 Attachable link: " + step + "; not sent",
		op:     "txnAttach",
	}, nil
}
