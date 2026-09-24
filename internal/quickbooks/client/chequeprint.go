package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// CHEQUE_REPRINT (expenses cheque export) rides the cheque form's Print
// contract, captured on TC2 2026-09-18 (Cheque 159 → Print preview iframe):
//
//	GET https://qbo.intuit.com/api/v4/companies/<realm>/transactions/
//	    <v4TxNS>:<v3Id>.pdf?intuit_apikey=<app-key>&intuit-company-id=<realm>
//
// The transaction id is the Relay-namespaced v4 node id. The namespace
// prefix is a company-level constant embedding the realm — decodes to
// "v4.1:9341457769756854:80271edd8a" on Test Company 2 (the same prefix the
// invoice nodes carry, e.g. …:146 for INV-9002). Plain --id values prepend
// the captured prefix when the realm matches; other realms must pass the
// full namespaced id.
const v4TxNSTestCompany2 = "djQuMTo5MzQxNDU3NzY5NzU2ODU0OjgwMjcxZWRkOGE"
const v4TxNSRealm = "9341457769756854"

// chequePDFAPIKey is the public webapp API key the cheque-print iframe embeds
// in its URL — not a user secret; it ships in the QBO page bundle.
const chequePDFAPIKey = "prdakyrest2lf1ZWr79ocuZKDnadWnwbW7zSvwqs"

// PlannedChequeExportURL reports the endpoint shape for dry-run plans.
func PlannedChequeExportURL() string {
	return "https://qbo.intuit.com/api/v4/companies/{realm}/transactions/{ns-id}.pdf"
}

// ReplayChequeExport downloads the printable PDF for an existing cheque.
// out empty → report status+bytes only; otherwise the PDF is written there.
func ReplayChequeExport(ctx context.Context, id, out string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("cheque export requires --id (v3 Purchase id or v4 namespaced id)")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	nsID := id
	if !strings.Contains(id, ":") {
		if ac.realm != v4TxNSRealm {
			return nil, fmt.Errorf("plain ids need the captured v4 namespace (only recorded for realm %s); pass the full djQu…:NN id", v4TxNSRealm)
		}
		nsID = v4TxNSTestCompany2 + ":" + id
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v4/companies/%s/transactions/%s.pdf?intuit_apikey=%s&intuit-company-id=%s",
		ac.realm, nsID, chequePDFAPIKey, ac.realm)
	resp, err := ac.doURIHost(ctx, http.MethodGet, u, "qbo.intuit.com", nil, map[string]string{
		"Accept":  "application/pdf,*/*",
		"Referer": "https://qbo.intuit.com/app/check",
	})
	if err != nil {
		return nil, fmt.Errorf("cheque export: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("cheque export: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	if len(raw) < 5 || string(raw[:4]) != "%PDF" {
		return nil, fmt.Errorf("cheque export: expected a PDF, got %d bytes: %.120s", len(raw), raw)
	}
	note := fmt.Sprintf("%d-byte PDF", len(raw))
	if out != "" {
		if err := os.WriteFile(out, raw, 0o600); err != nil {
			return nil, fmt.Errorf("cheque export: write %s: %w", out, err)
		}
		note = fmt.Sprintf("%d-byte PDF → %s", len(raw), out)
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "cheque export",
		Entity: "Purchase",
		Item:   QueryItem{Type: "Purchase", ID: id, Name: nsID},
		Note:   note,
	}, nil
}
