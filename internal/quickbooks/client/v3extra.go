package client

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// ReplayCDC returns entities changed since the given timestamp (v3 change
// data capture: GET /cdc?entities=A,B&changedSince=...). Verified live TC2:
// Invoice changes stream back. changedSince accepts RFC3339 or yyyy-MM-dd.
func ReplayCDC(ctx context.Context, entities, since string, limit int) (map[string]any, error) {
	entities = strings.TrimSpace(entities)
	if entities == "" {
		return nil, fmt.Errorf("cdc requires --entities (comma-separated v3 names)")
	}
	if strings.TrimSpace(since) == "" {
		return nil, fmt.Errorf("cdc requires --since timestamp")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := "https://qbo.intuit.com/api/v3/company/" + ac.realm +
		"/cdc?minorversion=73&entities=" + url.QueryEscape(entities) +
		"&changedSince=" + url.QueryEscape(since)
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		return nil, fmt.Errorf("v3 cdc: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("v3 cdc: %w", err)
	}
	return out, nil
}

// sendableForms are sales entities supporting POST /{entity}/{id}/send
// (?sendTo=). Verified live TC2: creditmemo/11/send reaches the mailer
// (400 requesting an address proves endpoint+auth, not a missing route).
var sendableForms = map[string]bool{
	"Invoice": true, "Estimate": true, "CreditMemo": true, "SalesReceipt": true,
}

// SendSalesForm emails a sales form (optionally to sendTo). A 200 means the
// mail went out — treat --to as a loaded flag.
func SendSalesForm(ctx context.Context, entity, id, sendTo string) (map[string]any, error) {
	lower, ok := map[string]string{
		"Invoice": "invoice", "Estimate": "estimate",
		"CreditMemo": "creditmemo", "SalesReceipt": "salesreceipt",
	}[entity]
	if !ok || !sendableForms[entity] {
		return nil, fmt.Errorf("send unsupported for entity %q", entity)
	}
	id = sanitizeToken(id)
	if id == "" {
		return nil, ErrMissingMutateID
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	path := lower + "/" + id + "/send"
	if strings.TrimSpace(sendTo) != "" {
		path += "?sendTo=" + url.QueryEscape(strings.TrimSpace(sendTo))
	}
	u := "https://qbo.intuit.com/api/v3/company/" + ac.realm + "/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, fmt.Errorf("sales send request: %w", err)
	}
	ac.applyHeaders(req, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := impersonatedDo(req)
	if err != nil {
		return nil, fmt.Errorf("sales send: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("sales send: %w", err)
	}
	return out, nil
}

// pdfableForms are sales entities supporting GET /{entity}/{id}/pdf.
var pdfableForms = map[string]string{
	"Invoice": "invoice", "Estimate": "estimate",
	"CreditMemo": "creditmemo", "SalesReceipt": "salesreceipt",
}

// ReplaySalesFormPDF downloads any sendable sales form as PDF to outPath.
// ReplayInvoicePDF shares this implementation; all forms use the same path
// (verified live TC2: creditmemo/11/pdf returns %PDF bytes).
func ReplaySalesFormPDF(ctx context.Context, entity, id, outPath string) (*InvoicePDFResult, error) {
	lower, ok := pdfableForms[entity]
	if !ok {
		return nil, fmt.Errorf("pdf unsupported for entity %q", entity)
	}
	id = sanitizeToken(id)
	if id == "" {
		return nil, ErrMissingMutateID
	}
	if strings.TrimSpace(outPath) == "" {
		return nil, fmt.Errorf("form pdf requires --out path")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := "https://qbo.intuit.com/api/v3/company/" + ac.realm + "/" + lower + "/" + id + "/pdf"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("form pdf request: %w", err)
	}
	ac.applyHeaders(req, "")
	req.Header.Set("Accept", "application/pdf")
	resp, err := impersonatedDo(req)
	if err != nil {
		return nil, fmt.Errorf("form pdf: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	if resp.StatusCode != http.StatusOK {
		raw, err := readBody(resp)
		if err != nil {
			return nil, err
		}
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	n, err := writeDownload(resp.Body, outPath, maxPDFBytes, true)
	if err != nil {
		return nil, fmt.Errorf("form pdf: %w", err)
	}
	return &InvoicePDFResult{
		Status:      resp.StatusCode,
		Path:        outPath,
		Bytes:       n,
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

// VerifyWebhookSignature checks an Intuit webhook payload against the
// verifier token (HMAC-SHA256, base64) without dialing. Pure local proof;
// unit-tested with RFC vectors, no live call needed.
func VerifyWebhookSignature(payload []byte, signature, verifierToken string) error {
	if len(payload) == 0 {
		return fmt.Errorf("webhook verify requires a payload")
	}
	if strings.TrimSpace(signature) == "" || strings.TrimSpace(verifierToken) == "" {
		return fmt.Errorf("webhook verify requires --signature and --token")
	}
	mac := hmac.New(sha256.New, []byte(verifierToken))
	mac.Write(payload)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.TrimSpace(signature)), []byte(want)) {
		return fmt.Errorf("webhook signature mismatch")
	}
	return nil
}

// CreateTaxCode posts a TaxCode via the v3 taxservice (verified live TC2:
// the endpoint answers ValidationFault naming missing TaxRateDetails, which
// require a real TaxAgency — absent on this test company, so full success
// is conditional on agency presence; the path, shape, and validation are
// proven). ratesJSON is the TaxRateDetails array (@file allowed).
func CreateTaxCode(ctx context.Context, name, ratesJSON string) (map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("tax-code create requires --name")
	}
	trimmed := strings.TrimSpace(ratesJSON)
	if after, ok := strings.CutPrefix(trimmed, "@"); ok {
		raw, err := os.ReadFile(after)
		if err != nil {
			return nil, fmt.Errorf("reading --rates-json: %w", err)
		}
		trimmed = string(raw)
	}
	var rates []any
	if trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &rates); err != nil {
			return nil, fmt.Errorf("--rates-json must decode to a JSON array: %w", err)
		}
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"TaxCode": name, "TaxRateDetails": rates})
	if err != nil {
		return nil, fmt.Errorf("tax-code body: %w", err)
	}
	u := "https://qbo.intuit.com/api/v3/company/" + ac.realm + "/taxservice/taxcode"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tax-code request: %w", err)
	}
	ac.applyHeaders(req, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := impersonatedDo(req)
	if err != nil {
		return nil, fmt.Errorf("tax-code create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("tax-code create: %w", err)
	}
	return out, nil
}

// ReplayBatch posts native v3 BatchItemRequest entries (read and write ops,
// up to 25 per call) and returns the raw BatchItemResponse list. Items pass
// through verbatim: {"bId":"1","Query":"select ..."} for reads,
// {"bId":"2","operation":"create","Invoice":{...}} for writes. Verified
// live TC2. A 200 with per-item Fault entries is still success at the HTTP
// layer; callers inspect items.
func ReplayBatch(ctx context.Context, itemsJSON string) ([]any, error) {
	trimmed := strings.TrimSpace(itemsJSON)
	if after, ok := strings.CutPrefix(trimmed, "@"); ok {
		raw, err := os.ReadFile(after)
		if err != nil {
			return nil, fmt.Errorf("reading batch file: %w", err)
		}
		trimmed = string(raw)
	}
	var items []any
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, fmt.Errorf("batch items must decode to a JSON array: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("batch requires at least one item")
	}
	if len(items) > 25 {
		return nil, fmt.Errorf("batch caps at 25 items (got %d)", len(items))
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"BatchItemRequest": items})
	if err != nil {
		return nil, fmt.Errorf("batch body: %w", err)
	}
	u := "https://qbo.intuit.com/api/v3/company/" + ac.realm + "/batch?minorversion=73"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("batch request: %w", err)
	}
	ac.applyHeaders(req, "")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := impersonatedDo(req)
	if err != nil {
		return nil, fmt.Errorf("batch: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out struct {
		Items []any `json:"BatchItemResponse"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("batch: %w", err)
	}
	return out.Items, nil
}
