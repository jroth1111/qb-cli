package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// RECEIPT_UPLOAD rides the financialdocument platform service — captured
// live on TC2 2026-09-18 from /app/receipts "Upload from computer":
//
//	POST https://financialdocument.platform.intuit.com/v2/documents?autoClassification=true
//	multipart/form-data:
//	  document = <json blob>   (metadata — see receiptDocumentMeta)
//	  image    = <file bytes>  (the receipt file itself)
//
// Headers carry Intuit_APIKey + x-csrf-token +
// intuit_appid/offeringid=Intuit.smallbusiness.receiptsclient +
// intuit_country/locale. autoClassification=true queues OCR extraction;
// the receipt lands in the For review list.
const receiptDocHost = "financialdocument.platform.intuit.com"
const receiptDocURL = "https://financialdocument.platform.intuit.com/v2/documents?autoClassification=true"

// PlannedReceiptUploadURL reports the upload URL for dry-run plans.
func PlannedReceiptUploadURL() string { return receiptDocURL }

// receiptDocumentMeta builds the captured `document` JSON field — the
// wizard sends this verbatim with the file name filled in.
func receiptDocumentMeta(fileName string) ([]byte, error) {
	meta := map[string]any{
		"semanticData": nil,
		"commonAttributes": map[string]any{
			"offeringAttributes": []any{
				map[string]any{
					"offeringId": "Intuit.smallbusiness.receiptsclient",
					"nameValues": []any{map[string]any{"name": "subsource", "value": "FILE_UPLOAD"}},
				},
				map[string]any{
					"externalAssociations": []any{},
					"offeringId":           "Intuit.cto.ocr",
					"nameValues": []any{
						map[string]any{"name": "paidExtraction", "value": "true"},
						map[string]any{"name": "country", "value": "AU"},
					},
				},
			},
			"documentType": "receipt",
			"annotation":   nil,
			"name":         fileName,
			"is7216":       false,
		},
	}
	return json.Marshal(meta)
}

// ReplayReceiptUpload uploads one or more receipt files (pdf/png/jpg)
// through the captured financialdocument contract — one documents POST
// per file, like the wizard's multi-select upload.
func ReplayReceiptUpload(ctx context.Context, paths string) (*MutateResult, error) {
	paths = strings.TrimSpace(paths)
	if paths == "" {
		return nil, fmt.Errorf("receipt import requires --files <pdf|png|jpg>[,<file>...]")
	}
	var results []string
	var createdIDs []string
	var last *MutateResult
	for p := range strings.SplitSeq(paths, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		res, err := replayOneReceipt(ctx, p)
		if err != nil {
			if len(createdIDs) > 0 {
				return nil, fmt.Errorf("receipt import: %w; %d receipt(s) uploaded (ids: %s); inspect before retrying", err, len(createdIDs), strings.Join(createdIDs, ","))
			}
			return nil, err
		}
		last = res
		results = append(results, res.Item.Name)
		createdIDs = append(createdIDs, res.Item.ID)
	}
	if last == nil {
		return nil, fmt.Errorf("receipt import: no files in --files")
	}
	if len(results) == 1 {
		return last, nil
	}
	return &MutateResult{
		Status: last.Status,
		Op:     "receipt import",
		Entity: "Receipt",
		Item:   QueryItem{Type: "Receipt", ID: strings.Join(createdIDs, ","), Name: fmt.Sprintf("%d files", len(results))},
		Note:   last.Note + " — " + strings.Join(results, ", "),
	}, nil
}

func replayOneReceipt(ctx context.Context, path string) (*MutateResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("receipt import: %w", err)
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("receipt import: %s is empty", path)
	}
	name := filepath.Base(path)
	mime := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		mime = "application/pdf"
	case ".png":
		mime = "image/png"
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".heic":
		mime = "image/heic"
	case ".tif", ".tiff":
		mime = "image/tiff"
	}
	meta, err := receiptDocumentMeta(name)
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if ac.realm == "" {
		return nil, fmt.Errorf("receipt import: no realm id in credentials")
	}

	var buf bytes.Buffer
	boundary := webkitBoundary()
	esc := strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"document\"; filename=\"blob\"\r\n")
	fmt.Fprintf(&buf, "Content-Type: application/json\r\n\r\n")
	buf.Write(meta)
	fmt.Fprintf(&buf, "\r\n--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"image\"; filename=\"%s\"\r\n", esc.Replace(name))
	fmt.Fprintf(&buf, "Content-Type: %s\r\n\r\n", mime)
	buf.Write(content)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)

	overrides := map[string]string{
		"Content-Type":      "multipart/form-data; boundary=" + boundary,
		"Referer":           "https://qbo.intuit.com/app/receipts",
		"intuit-company-id": ac.realm,
		"intuit_realmid":    ac.realm,
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, receiptDocURL, receiptDocHost, buf.Bytes(), overrides)
	if err != nil {
		return nil, fmt.Errorf("receipt import: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading receipt import: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	// The response carries the created document descriptor.
	var receipt struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return nil, fmt.Errorf("receipt import outcome unknown; inspect before retrying: unreadable response: %w", err)
	}
	if receipt.ID == "" || !strings.EqualFold(receipt.Status, "ACCEPTED") {
		return nil, fmt.Errorf("receipt import outcome unknown; inspect before retrying: expected accepted document receipt")
	}
	note := "financialdocument v2/documents autoClassification=true"
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "receipt import",
		Entity: "Receipt",
		Item:   QueryItem{Type: "Receipt", ID: receipt.ID, Name: name},
		Note:   note,
	}, nil
}
