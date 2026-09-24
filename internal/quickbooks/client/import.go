package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CSVImportAccountID is the Jacob Reimbursement CSV-import feed (account 209).
// Live OAuth bank feeds are refused by ReplayImportCSV.
const CSVImportAccountID = "209"

var (
	// ErrEmptyImportAccount is returned when ReplayImportCSV is called with no account id.
	ErrEmptyImportAccount = errors.New("import requires account-id")
	// ErrEmptyImportDescription is returned when the import description is empty.
	ErrEmptyImportDescription = errors.New("import requires a description")
	// ErrEmptyImportAmount is returned when the import amount is missing or not numeric.
	ErrEmptyImportAmount = errors.New("import requires a numeric amount")
	// ErrImportBlockedAccount is returned when the target is a live OAuth bank feed.
	ErrImportBlockedAccount = errors.New("import refuses live bank accounts 204 and 93; use CSV account 209")
)

// ImportResult is the secret-free envelope returned by ReplayImportCSV.
type ImportResult struct {
	Status      int    `json:"status"`
	AccountID   string `json:"accountId"`
	Imported    int    `json:"imported"`
	Date        string `json:"date"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
}

type csvImportRequest struct {
	Data []struct {
		Date          string `json:"date"`
		Description   string `json:"description"`
		Amount        string `json:"amount"`
		MarkForImport bool   `json:"markForImport"`
	} `json:"data"`
	DateFormat     string            `json:"dateFormat"`
	AccountID      string            `json:"accountId"`
	ColumnMappings map[string]string `json:"columnMappings"`
}

// ReplayImportCSV POSTs /api/neo/v1/company/{realm}/olb/processCsvFile
// as captured 2026-08-17 from /app/newfileupload?accountId=209.
// Bank rules may automatically post imported rows. The import result is not
// proof of Pending state; callers must verify feed and ledger effects.
// Validation (account/description/amount, live-account refuse) runs
// before newAPIClient so bad input never opens a socket.
func ReplayImportCSV(ctx context.Context, accountID, date, description, amount string) (*ImportResult, error) {
	plan, meta, err := PlanImportCSV(accountID, date, description, amount)
	if err != nil {
		return nil, err
	}
	status, err := postPlanned(ctx, plan)
	if err != nil {
		return nil, err
	}
	meta.Status = status
	return &meta, nil
}

// UploadResult is the secret-free envelope for a staged CSV file.
type UploadResult struct {
	Status   int    `json:"status"`
	Endpoint string `json:"endpoint"`
	Bytes    int    `json:"bytes"`
	Summary  string `json:"summary"`
}

// UploadCSVFile stages a bank-statement file via multipart POST to
// /api/neo/v1/company/{realm}/olb/uploadCsvFile — the exact call the
// /app/newfileupload wizard makes on file select (captured TC2 2026-09-08:
// single part name=csvFileName, filename preserved, text/csv). The JSON
// processCsvFile replay alone 500s without a staged file, so file import
// must stage first, then process.
func UploadCSVFile(ctx context.Context, filename string, content []byte) (*UploadResult, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, fmt.Errorf("upload requires a filename")
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("upload requires non-empty file content")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	// The endpoint validates the multipart boundary shape (TC2 2026-09-08:
	// Go hex boundaries 400 "Invalid file format" while a
	// ----WebKitFormBoundary<16 alnum> token succeeds). Frame manually.
	boundary := webkitBoundary()
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"csvFileName\"; filename=\"%s\"\r\n",
		strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(filename))
	fmt.Fprintf(&buf, "Content-Type: text/csv\r\n\r\n")
	buf.Write(content)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)
	contentType := "multipart/form-data; boundary=" + boundary
	url := strings.ReplaceAll(neoCompanyTmpl+"/olb/uploadCsvFile", realmToken, ac.realm)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("upload request: %w", err)
	}
	ac.applyHeaders(req, "")
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	// The bank-connections UI identifies itself; without it the endpoint 500s.
	req.Header.Set("intuit-plugin-id", "integrations-bankconnections-ui")
	resp, err := impersonatedDo(req)
	if err != nil {
		return nil, fmt.Errorf("uploadCsvFile: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("uploadCsvFile: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	summary := summarizeUploadResponse(raw)
	return &UploadResult{Status: resp.StatusCode, Endpoint: "uploadCsvFile", Bytes: len(content), Summary: summary}, nil
}

// webkitBoundary mints a Chrome-style multipart boundary: the upload endpoint
// rejects other shapes with "Invalid file format".
func webkitBoundary() string {
	const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var sb strings.Builder
	sb.WriteString("----WebKitFormBoundary")
	seed := time.Now().UnixNano()
	x := uint64(seed)
	for range 16 {
		// xorshift64 for non-crypto boundary randomness.
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		sb.WriteByte(alpha[int(x%uint64(len(alpha)))])
	}
	return sb.String()
}

// summarizeUploadResponse extracts a secret-free one-line description of
// the staged file (ids, counts) without logging the raw payload.
func summarizeUploadResponse(raw []byte) string {
	var probe struct {
		FileID   any `json:"fileId"`
		FileName any `json:"fileName"`
		Count    any `json:"count"`
		Total    any `json:"total"`
		Status   any `json:"status"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return fmt.Sprintf("%d response bytes", len(raw))
	}
	return fmt.Sprintf("fileId=%v fileName=%v count=%v total=%v status=%v",
		probe.FileID, probe.FileName, probe.Count, probe.Total, probe.Status)
}

func isLiveBankAccount(accountID string) bool {
	switch accountID {
	case "204", "93":
		return true
	default:
		return false
	}
}

func defaultImportDate() string {
	loc, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		loc = time.Local
	}
	return time.Now().In(loc).Format("02/01/2006")
}
