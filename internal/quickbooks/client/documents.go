// documents.go replays the QBO document print/email service endpoints seen
// in the captured frontend bundles and traffic (/tmp/qbo-cap):
//
//	emailingestion.api.intuit.com  GET/PUT/POST /v2/destinationEmail,
//	                               GET /v2/destinationEmails,
//	                               POST /v2/destinationEmail/checkAvailability
//	                                   (live-captured 200 {"email":"…@qbodocs.com"})
//	txnsrendering.api.intuit.com   POST /v2/print, GET|POST /v3/documents/pdf
//	                                   (order-management-ui resolves its
//	                                   render-env map prod → txnsrendering)
//	financialdocumentpci.api.intuit.com
//	                               POST /v2/documents, POST /v2/documents/list,
//	                               PUT /v2/documents/{id}
//	                                   (banking-reconcile-ui prod map)
//
// Transport follows the v3 mutation pattern in mutate.go: the shared stdlib
// ladder in http.go (captured ATS headers + cookie jar, remint on 401), so
// tests can route every host through the http.DefaultTransport swap. The
// dedicated hosts expect the browser's Intuit_APIKey app key; when the ATS
// credential is rejected the API status is surfaced verbatim via ReplayError.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Dedicated document-service hosts (see file comment for evidence).
const (
	docEmailHost  = "https://emailingestion.api.intuit.com"
	docRenderHost = "https://txnsrendering.api.intuit.com"
	docFDPHost    = "https://financialdocumentpci.api.intuit.com"
)

var (
	// ErrMissingTxnID is returned when a documents command has no --txn-id.
	ErrMissingTxnID = errors.New("documents requires --txn-id")
	// ErrMissingEmail is returned when an email destination is required but
	// no address was supplied.
	ErrMissingEmail = errors.New("documents requires --email")
	// ErrMissingDocumentIDs is returned when documents list has no --ids.
	ErrMissingDocumentIDs = errors.New("documents list requires --ids")
)

// DestinationEmailResult is the secret-free envelope for the
// emailingestion.api.intuit.com /v2/destinationEmail* endpoints.
type DestinationEmailResult struct {
	Status    int            `json:"status"`
	Op        string         `json:"op"`
	Email     string         `json:"email,omitempty"`
	Available *bool          `json:"available,omitempty"`
	History   []QueryItem    `json:"history,omitempty"`
	Counts    map[string]int `json:"counts,omitempty"`
	Note      string         `json:"note,omitempty"`
}

// DocumentsResult is the secret-free envelope for POST /v2/documents/list.
type DocumentsResult struct {
	Status int            `json:"status"`
	Entity string         `json:"entity"`
	Counts map[string]int `json:"counts"`
	Items  []QueryItem    `json:"items"`
	Note   string         `json:"note,omitempty"`
}

// PrintResult describes a completed /v2/print call without dumping the PDF.
type PrintResult struct {
	Status      int    `json:"status"`
	TxnID       string `json:"txnId,omitempty"`
	Bytes       int    `json:"bytes"`
	ContentType string `json:"contentType,omitempty"`
	Note        string `json:"note,omitempty"`
}

// PDFResult reports where a downloaded document PDF was written.
type PDFResult struct {
	Status int    `json:"status"`
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Note   string `json:"note,omitempty"`
}

// --- planned URLs (--dry-run; never embed a live realm) ---------------------

// PlannedDocumentSendURL is the dry-run URL for POST /v2/destinationEmail.
func PlannedDocumentSendURL() string {
	return docEmailHost + "/v2/destinationEmail"
}

// PlannedDocumentCheckURL is the dry-run URL for the availability check.
func PlannedDocumentCheckURL() string {
	return docEmailHost + "/v2/destinationEmail/checkAvailability"
}

// PlannedDocumentEmailsURL is the dry-run URL for GET /v2/destinationEmails.
func PlannedDocumentEmailsURL() string {
	return docEmailHost + "/v2/destinationEmails"
}

// PlannedDocumentPrintURL is the dry-run URL for POST /v2/print.
func PlannedDocumentPrintURL() string {
	return docRenderHost + "/v2/print"
}

// PlannedDocumentPDFURL is the dry-run URL for GET /v3/documents/pdf.
func PlannedDocumentPDFURL(txnID string) string {
	return docRenderHost + "/v3/documents/pdf?txnId=" + url.QueryEscape(sanitizeToken(txnID)) + "&documentType=invoice"
}

// PlannedDocumentListURL is the dry-run URL for POST /v2/documents/list.
func PlannedDocumentListURL() string {
	return docFDPHost + "/v2/documents/list"
}

// PlannedDocumentUpdateURL is the dry-run URL for PUT /v2/documents/{id}.
func PlannedDocumentUpdateURL(id string) string {
	return docFDPHost + "/v2/documents/" + sanitizeToken(id)
}

// --- destination email ------------------------------------------------------

// ReplayDestinationEmailGet fetches the company's unique inbound email
// destination (GET /v2/destinationEmail). Captured live response shape:
// {"email":"company@qbodocs.com"}.
func ReplayDestinationEmailGet(ctx context.Context) (*DestinationEmailResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := docEmailHost + "/v2/destinationEmail"
	resp, err := ac.doStdlibJSON(ctx, http.MethodGet, u, nil, "")
	if err != nil {
		return nil, fmt.Errorf("destinationEmail get: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var got struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(raw, &got)
	return &DestinationEmailResult{
		Status: resp.StatusCode,
		Op:     "get",
		Email:  got.Email,
		Counts: map[string]int{"items": boolToInt(got.Email != "")},
		Note:   "emailingestion GET /v2/destinationEmail",
	}, nil
}

// ReplayDestinationEmailSend registers/sends to an email destination
// (POST /v2/destinationEmail). Captured body carries destinationEmailAddress;
// txnId rides along as a correlation field for transaction delivery.
func ReplayDestinationEmailSend(ctx context.Context, txnID, email string) (*DestinationEmailResult, error) {
	txnID = strings.TrimSpace(txnID)
	if txnID == "" {
		return nil, ErrMissingTxnID
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, ErrMissingEmail
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"destinationEmailAddress": email,
		"txnId":                   sanitizeToken(txnID),
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, docEmailHost+"/v2/destinationEmail", body, nil)
	if err != nil {
		return nil, fmt.Errorf("destinationEmail send: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var got struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(raw, &got)
	out := &DestinationEmailResult{
		Status: resp.StatusCode,
		Op:     "send",
		Email:  firstNonEmpty(got.Email, email),
		Counts: map[string]int{"items": 1},
		Note:   "emailingestion POST /v2/destinationEmail",
	}
	return out, nil
}

// ReplayDestinationEmailCheck asks whether a custom destination address is
// available (POST /v2/destinationEmail/checkAvailability). The UI treats a
// 400 as "invalid/unavailable email".
func ReplayDestinationEmailCheck(ctx context.Context, email string) (*DestinationEmailResult, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, ErrMissingEmail
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]string{"destinationEmailAddress": email})
	if err != nil {
		return nil, err
	}
	u := docEmailHost + "/v2/destinationEmail/checkAvailability"
	resp, err := ac.postJSONExtra(ctx, u, body, nil)
	if err != nil {
		return nil, fmt.Errorf("destinationEmail checkAvailability: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	res := &DestinationEmailResult{Status: resp.StatusCode, Op: "check", Note: "emailingestion POST /v2/destinationEmail/checkAvailability"}
	switch resp.StatusCode {
	case http.StatusOK:
		ok := true
		res.Available = &ok
	case http.StatusBadRequest:
		ok := false
		res.Available = &ok
	default:
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res.Email = email
	return res, nil
}

// ReplayDestinationEmailsList fetches the destination-email history
// (GET /v2/destinationEmails). Captured response wraps {"destinationEmails":
// [...]}; the UI rejects payloads without that key.
func ReplayDestinationEmailsList(ctx context.Context) (*DestinationEmailResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := docEmailHost + "/v2/destinationEmails"
	resp, err := ac.doStdlibJSON(ctx, http.MethodGet, u, nil, "")
	if err != nil {
		return nil, fmt.Errorf("destinationEmails list: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var got struct {
		DestinationEmails []struct {
			ID        any    `json:"id"`
			Email     string `json:"email"`
			CreatedAt string `json:"createdAt"`
		} `json:"destinationEmails"`
	}
	items := []QueryItem{}
	if err := json.Unmarshal(raw, &got); err == nil {
		for _, d := range got.DestinationEmails {
			items = append(items, QueryItem{ID: fmt.Sprint(d.ID), Name: d.Email, Date: d.CreatedAt, Type: "destinationEmail"})
		}
	}
	return &DestinationEmailResult{
		Status:  resp.StatusCode,
		Op:      "list",
		History: items,
		Counts:  map[string]int{"items": len(items)},
		Note:    fmt.Sprintf("emailingestion GET /v2/destinationEmails n=%d", len(items)),
	}, nil
}

// --- print / pdf ------------------------------------------------------------

// ReplayDocumentPrint renders a transaction form (POST /v2/print) and reports
// the returned payload size without emitting it. The captured payload carries
// v4TxnData/companySettings/locale/shouldUploadToDocService/intuitTid/region/
// txnType/templateId; the CLI sends the minimal subset keyed by txn id.
func ReplayDocumentPrint(ctx context.Context, txnID string) (*PrintResult, error) {
	txnID = strings.TrimSpace(txnID)
	if txnID == "" {
		return nil, ErrMissingTxnID
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"txnType":                  "Invoice",
		"txnId":                    sanitizeToken(txnID),
		"locale":                   "en-us",
		"shouldUploadToDocService": false,
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, docRenderHost+"/v2/print", body, map[string]string{"Accept": "application/pdf"})
	if err != nil {
		return nil, fmt.Errorf("document print: %w", err)
	}
	defer drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		raw, err := readBody(resp)
		if err != nil {
			return nil, err
		}
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	n, err := countBody(resp)
	if err != nil {
		return nil, err
	}
	return &PrintResult{
		Status:      resp.StatusCode,
		TxnID:       sanitizeToken(txnID),
		Bytes:       n,
		ContentType: resp.Header.Get("Content-Type"),
		Note:        "txnsrendering POST /v2/print",
	}, nil
}

// ReplayDocumentPDF downloads a rendered document (GET /v3/documents/pdf) and
// writes the bytes to outPath. The response body is the PDF itself.
func ReplayDocumentPDF(ctx context.Context, txnID, outPath string) (*PDFResult, error) {
	txnID = strings.TrimSpace(txnID)
	if txnID == "" {
		return nil, ErrMissingTxnID
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := docRenderHost + "/v3/documents/pdf?txnId=" + sanitizeToken(txnID) + "&documentType=invoice"
	resp, err := ac.doStdlib(ctx, http.MethodGet, u, nil, "")
	if err != nil {
		return nil, fmt.Errorf("document pdf: %w", err)
	}
	defer drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		raw, err := readBody(resp)
		if err != nil {
			return nil, err
		}
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	pdf, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("reading pdf: %w", err)
	}
	if err := os.WriteFile(outPath, pdf, 0o644); err != nil {
		return nil, err
	}
	return &PDFResult{
		Status: resp.StatusCode,
		Path:   outPath,
		Bytes:  len(pdf),
		Note:   "txnsrendering GET /v3/documents/pdf",
	}, nil
}

// --- documents list / update ------------------------------------------------

// ReplayDocumentsList POSTs /v2/documents/list with the exact captured body
// shape: {"documentIds":[…],"includeEntityData":true,"contextFilters":
// {"includeLocatorInfo":true}} (banking-reconcile-ui chunks in batches ≤200;
// the CLI passes ids straight through).
func ReplayDocumentsList(ctx context.Context, entity string, ids []string) (*DocumentsResult, error) {
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			clean = append(clean, sanitizeToken(id))
		}
	}
	entity = strings.TrimSpace(entity)
	if entity == "" {
		entity = "document"
	}
	if len(clean) == 0 {
		return nil, ErrMissingDocumentIDs
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"documentIds":       clean,
		"includeEntityData": true,
		"contextFilters":    map[string]bool{"includeLocatorInfo": true},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, docFDPHost+"/v2/documents/list", body, nil)
	if err != nil {
		return nil, fmt.Errorf("documents list: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return projectDocuments(raw, entity, resp.StatusCode), nil
}

func projectDocuments(body []byte, entity string, status int) *DocumentsResult {
	note := "financialdocumentpci POST /v2/documents/list"
	var got struct {
		Documents []struct {
			SystemAttributes struct {
				ID         string `json:"id"`
				CreateDate string `json:"createDate"`
				UpdateDate string `json:"updateDate"`
			} `json:"systemAttributes"`
			CommonAttributes struct {
				DocumentType string `json:"documentType"`
				DocumentName string `json:"documentName"`
			} `json:"commonAttributes"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return &DocumentsResult{Status: status, Entity: entity, Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	items := make([]QueryItem, 0, len(got.Documents))
	for _, d := range got.Documents {
		items = append(items, QueryItem{
			ID:   d.SystemAttributes.ID,
			Name: d.CommonAttributes.DocumentName,
			Date: d.SystemAttributes.CreateDate,
			Type: d.CommonAttributes.DocumentType,
		})
	}
	note = fmt.Sprintf("%s n=%d items=%d", note, len(got.Documents), len(items))
	return &DocumentsResult{
		Status: status,
		Entity: entity,
		Counts: map[string]int{"items": len(items)},
		Items:  items,
		Note:   note,
	}
}

// ReplayDocumentUpdate PUTs a caller-supplied JSON document body to
// /v2/documents/{id} (banking-reconcile PUTs the whole document object back).
// data must be a JSON object.
func ReplayDocumentUpdate(ctx context.Context, id, data string) (*MutateResult, error) {
	id = sanitizeToken(id)
	if id == "" {
		return nil, ErrMissingMutateID
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(data), &probe); err != nil || probe == nil {
		return nil, fmt.Errorf("--data must be a JSON object: %w", err)
	}
	resp, err := ac.putDocument(ctx, docFDPHost+"/v2/documents/"+id, []byte(data))
	if err != nil {
		return nil, fmt.Errorf("document update: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	item := projectDocumentItem(raw, id)
	return &MutateResult{Status: resp.StatusCode, Op: "update", Entity: "Document", Item: item}, nil
}

func projectDocumentItem(body []byte, fallbackID string) QueryItem {
	var got struct {
		SystemAttributes struct {
			ID         string `json:"id"`
			CreateDate string `json:"createDate"`
			UpdateDate string `json:"updateDate"`
		} `json:"systemAttributes"`
		CommonAttributes struct {
			DocumentType string `json:"documentType"`
			DocumentName string `json:"documentName"`
		} `json:"commonAttributes"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return QueryItem{ID: fallbackID}
	}
	id := got.SystemAttributes.ID
	if id == "" {
		id = fallbackID
	}
	return QueryItem{ID: id, Name: got.CommonAttributes.DocumentName, Date: got.SystemAttributes.UpdateDate, Type: got.CommonAttributes.DocumentType}
}

// --- small helpers ----------------------------------------------------------

// putDocument issues a PUT with the shared captured-header ladder. The
// transport helpers in http.go cover GET and POST; PUT appears only here.
func (c *apiClient) putDocument(ctx context.Context, rawURL string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.applyHeaders(req, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	cli := &http.Client{Timeout: httpTimeout}
	return cli.Do(req)
}

// countBody drains the response counting bytes without keeping them.
func countBody(resp *http.Response) (int, error) {
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	return int(n), err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
