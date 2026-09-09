package client

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/enetx/surf"
)

// attachUploadTmpl is the v3 file-upload endpoint behind the webapp proxy
// (same ATS session as everything else). Proven live TC2: multipart with a
// single file_content_0 part returns AttachableResponse with the new id.
const attachUploadTmpl = "https://qbo.intuit.com/api/v3/company/{realm}/upload"

// AttachResult is the secret-free outcome of an attachable upload: the new
// Attachable id and file name. TempDownloadUri is NEVER surfaced — it
// embeds a live apikey query string.
type AttachResult struct {
	Status   int    `json:"status"`
	ID       string `json:"id"`
	FileName string `json:"file_name"`
	Bytes    int    `json:"bytes"`
}

// UploadAttachable posts a file as a v3 Attachable (verified live shape:
// part name=file_content_0, filename preserved). Content type sniffs from
// the filename extension, defaulting to application/octet-stream. Filenames
// with path separators are rejected so the server never sees a path.
func UploadAttachable(ctx context.Context, filename string, content []byte) (*AttachResult, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || strings.ContainsAny(filename, `/\`) {
		return nil, fmt.Errorf("upload requires a bare filename")
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("upload requires non-empty file content")
	}
	if len(content) > 100<<20 {
		return nil, fmt.Errorf("upload exceeds the 100MB QBO per-request cap")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	boundary := webkitBoundary()
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"file_content_0\"; filename=\"%s\"\r\n",
		strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(filename))
	fmt.Fprintf(&buf, "Content-Type: %s\r\n\r\n", sniffUploadType(filename))
	buf.Write(content)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)
	url := strings.ReplaceAll(attachUploadTmpl, "{realm}", ac.realm)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	ac.applyHeaders(req, "")
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("Accept", "application/json")
	resp, err := impersonatedDo(req)
	if err != nil {
		return nil, fmt.Errorf("attachable upload: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("attachable upload: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	id, name := parseAttachableResponse(raw)
	if id == "" {
		return nil, fmt.Errorf("attachable upload: unexpected response shape")
	}
	return &AttachResult{Status: resp.StatusCode, ID: id, FileName: name, Bytes: len(content)}, nil
}

// sniffUploadType maps common attachment extensions to MIME types.
func sniffUploadType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv"
	case strings.HasSuffix(lower, ".txt"):
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// InvoicePDFResult reports where a downloaded invoice PDF was written.
type InvoicePDFResult struct {
	Status      int    `json:"status"`
	Path        string `json:"path"`
	Bytes       int    `json:"bytes"`
	ContentType string `json:"content_type"`
}

// ReplayInvoicePDF downloads a sales form as PDF (GET
// /api/v3/company/{realm}/invoice/{id}/pdf, Accept: application/pdf —
// verified live TC2: 21KB PDF for a real invoice) and writes the bytes to
// outPath. The body is the PDF itself; at most 64MB is buffered.
func ReplayInvoicePDF(ctx context.Context, id, outPath string) (*InvoicePDFResult, error) {
	id = sanitizeToken(id)
	if id == "" {
		return nil, ErrMissingMutateID
	}
	if strings.TrimSpace(outPath) == "" {
		return nil, fmt.Errorf("invoice pdf requires --out path")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := "https://qbo.intuit.com/api/v3/company/" + ac.realm + "/invoice/" + id + "/pdf"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("invoice pdf request: %w", err)
	}
	ac.applyHeaders(req, "")
	req.Header.Set("Accept", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("invoice pdf: %w", err)
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
	if len(pdf) == 0 || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		return nil, fmt.Errorf("invoice pdf: response is not a PDF (%d bytes)", len(pdf))
	}
	if err := os.WriteFile(outPath, pdf, 0o644); err != nil {
		return nil, fmt.Errorf("writing pdf: %w", err)
	}
	return &InvoicePDFResult{
		Status:      resp.StatusCode,
		Path:        outPath,
		Bytes:       len(pdf),
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}

// attachXML is the XML envelope the upload endpoint serves to browser-like
// TLS fingerprints (Chrome impersonation); plain clients get JSON.
type attachXML struct {
	Response []struct {
		Attachable struct {
			ID       string `xml:"Id"`
			FileName string `xml:"FileName"`
		} `xml:"Attachable"`
	} `xml:"AttachableResponse"`
}

// parseAttachableResponse accepts both envelopes: JSON for plain clients,
// XML for impersonated (browser-like) clients. The server content-negotiates
// on TLS fingerprint, so both shapes are live on TC2.
func parseAttachableResponse(raw []byte) (id, name string) {
	var envelope struct {
		AttachableResponse []struct {
			Attachable struct {
				ID       string `json:"Id"`
				FileName string `json:"FileName"`
			} `json:"Attachable"`
		} `json:"AttachableResponse"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.AttachableResponse) > 0 {
		got := envelope.AttachableResponse[0].Attachable
		return got.ID, got.FileName
	}
	var x attachXML
	if err := xml.Unmarshal(raw, &x); err == nil && len(x.Response) > 0 {
		got := x.Response[0].Attachable
		return got.ID, got.FileName
	}
	return "", ""
}

// impersonatedDo issues req through the Chrome-impersonating client (house
// default for friction reduction). Defined once; upload and CSV staging
// share it.
func impersonatedDo(req *http.Request) (*http.Response, error) {
	builder := surf.NewClient().Builder().Impersonate().Chrome().Timeout(httpTimeout)
	sc, err := builder.Build().Result()
	if err != nil {
		return nil, err
	}
	std := sc.Std()
	std.Timeout = httpTimeout
	return std.Do(req)
}

// DownloadAttachable fetches an attachable's bytes to outPath. The v3
// download endpoint answers with a pre-signed financialdocument URL (it
// embeds a live apikey, so it is followed transiently and never logged or
// persisted); the second GET returns the file bytes. Verified live TC2.
func DownloadAttachable(ctx context.Context, id, outPath string) (*InvoicePDFResult, error) {
	id = sanitizeToken(id)
	if id == "" {
		return nil, fmt.Errorf("attachable download requires --id")
	}
	if strings.TrimSpace(outPath) == "" {
		return nil, fmt.Errorf("attachable download requires --out path")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := "https://qbo.intuit.com/api/v3/company/" + ac.realm + "/download/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("attachable download request: %w", err)
	}
	ac.applyHeaders(req, "")
	req.Header.Set("Accept", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("attachable download pointer: %w", err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	fileURL := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(fileURL, "https://") {
		return nil, fmt.Errorf("attachable download: unexpected pointer shape")
	}
	freq, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("attachable file request: %w", err)
	}
	fresp, err := http.DefaultClient.Do(freq)
	if err != nil {
		return nil, fmt.Errorf("attachable file fetch: %w", err)
	}
	defer drainAndClose(fresp)
	if fresp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attachable file fetch: status %d", fresp.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(fresp.Body, 100<<20))
	if err != nil {
		return nil, fmt.Errorf("reading attachable: %w", err)
	}
	if err := os.WriteFile(outPath, content, 0o644); err != nil {
		return nil, fmt.Errorf("writing attachable: %w", err)
	}
	return &InvoicePDFResult{
		Status:      fresp.StatusCode,
		Path:        outPath,
		Bytes:       len(content),
		ContentType: fresp.Header.Get("Content-Type"),
	}, nil
}
