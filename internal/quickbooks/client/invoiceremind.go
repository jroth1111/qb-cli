package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var ErrReminderOutcomeUnknown = errors.New("reminder outcome unverified; inspect current state before resending")

// ReplayInvoiceReminder sends a one-off payment-reminder email for an
// invoice — the CLI equivalent of the invoices list's "Send reminder"
// action. The send rides the same neo endpoint the dialog uses:
//
//	POST https://qbo.intuit.com/api/neo/v1/company/<realm>/purchsales/send
//	{"txnTypeId":4,"txnId":"<id>","deliveryInfo":{deliveryType:"EMAIL",
//	  deliveryAddress,deliveryEmailSubject,deliveryEmailBody,...}}
//
// Contract captured live on TC2 2026-09-19 (invoice 7, reminder sent and
// dialog closed 200). Recipient defaults to the invoice's BillEmail, then
// the customer's PrimaryEmailAddr — matching the dialog's emailInfo
// prefill. --automatic (scheduled reminder workflows) is a separate
// automation surface that was never captured; passing it fails loudly.
func ReplayInvoiceReminder(ctx context.Context, id, automatic, to string) (*MutateResult, error) {
	if os.Getenv("PRINTING_PRESS_VERIFY") == "1" || os.Getenv("PRINTING_PRESS_DOGFOOD") == "1" {
		return nil, fmt.Errorf("refusing real reminder email under a verification harness")
	}
	if strings.TrimSpace(automatic) == "true" || strings.TrimSpace(automatic) == "1" {
		return nil, fmt.Errorf("--automatic schedules reminder workflows (a separate automation surface not captured on this build); omit it to send a one-off reminder now")
	}
	id = strings.TrimSpace(id)
	if !mutationEntityID.MatchString(id) {
		return nil, fmt.Errorf("invoice remind requires --id as a numeric invoice identity")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	rows, err := queryV3Rows(ctx, ac, "Invoice", fmt.Sprintf("select * from Invoice where Id = '%s'", id))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("invoice %s not found", id)
	}
	inv := rows[0]
	if len(rows) != 1 || jsonNumberString(inv["Id"]) != id {
		return nil, fmt.Errorf("invoice reminder before-state identity mismatch; no email sent")
	}
	custName, custID := refNameValue(inv, "CustomerRef")
	docNum, _ := inv["DocNumber"].(string)
	email := strings.TrimSpace(to)
	if email == "" {
		email = billEmail(inv)
	}
	if email == "" && custID != "" {
		email = customerEmail(ctx, ac, custID)
	}
	if email == "" {
		return nil, fmt.Errorf("invoice %s: no recipient email (customer %s has none); pass --to", id, custName)
	}
	company := companyName(ctx, ac)
	subject := fmt.Sprintf("Reminder: Your payment to %s is due", company)
	body := fmt.Sprintf("Dear %s,\r\n\r\nWe're sending a reminder to let you know that invoice %s has not been paid. "+
		"If you already paid this invoice or have any questions, let us know!\r\n\r\nHave a great day!\r\n%s",
		custName, docNum, company)
	payload, err := json.Marshal(map[string]any{
		"txnTypeId": 4,
		"txnId":     id,
		"deliveryInfo": map[string]any{
			"deliveryType":         "EMAIL",
			"deliveryAddress":      email,
			"deliveryAddressCc":    "",
			"deliveryAddressBcc":   "",
			"deliveryEmailSubject": subject,
			"deliveryEmailBody":    body,
		},
	})
	if err != nil {
		return nil, err
	}
	u := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/purchsales/send"
	evidence, err := startMutationEvidence("invoice-reminder", map[string]any{"realm_id": ac.realm, "invoice": inv}, payload)
	if err != nil {
		return nil, fmt.Errorf("cannot preserve reminder before-state: %w", err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, u, "qbo.intuit.com", payload, map[string]string{
		"Referer":      "https://qbo.intuit.com/app/invoices",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("%w (evidence %s): %v", ErrReminderOutcomeUnknown, evidence, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("%w (evidence %s): %v", ErrReminderOutcomeUnknown, evidence, err)
	}
	if err := os.WriteFile(filepath.Join(evidence, "response.json"), raw, 0600); err != nil {
		return nil, fmt.Errorf("%w: cannot persist response (evidence %s)", ErrReminderOutcomeUnknown, evidence)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	// The captured success body is an empty acknowledgement, not a delivery
	// receipt. Preserve it and never invite an automatic duplicate send.
	return nil, fmt.Errorf("%w: HTTP %d acknowledged the request but did not prove delivery (evidence %s)", ErrReminderOutcomeUnknown, resp.StatusCode, evidence)
}

// refNameValue pulls {value,name} out of a v3 *Ref object.
func refNameValue(row map[string]any, key string) (name, id string) {
	if ref, ok := row[key].(map[string]any); ok {
		name, _ = ref["name"].(string)
		id = fmt.Sprint(ref["value"])
	}
	return name, id
}

// billEmail returns the invoice's BillEmail address when present.
func billEmail(inv map[string]any) string {
	if be, ok := inv["BillEmail"].(map[string]any); ok {
		if s, ok := be["Address"].(string); ok {
			return s
		}
	}
	return ""
}

// customerEmail resolves a customer's PrimaryEmailAddr via v3.
func customerEmail(ctx context.Context, ac *apiClient, custID string) string {
	rows, err := queryV3Rows(ctx, ac, "Customer", fmt.Sprintf("select * from Customer where Id = '%s'", custID))
	if err != nil || len(rows) == 0 {
		return ""
	}
	if em, ok := rows[0]["PrimaryEmailAddr"].(map[string]any); ok {
		s, _ := em["Address"].(string)
		return s
	}
	return ""
}

// companyName resolves the company display name for the reminder template.
func companyName(ctx context.Context, ac *apiClient) string {
	rows, err := queryV3Rows(ctx, ac, "CompanyInfo", "select * from CompanyInfo")
	if err != nil || len(rows) == 0 {
		return "your supplier"
	}
	if s, ok := rows[0]["CompanyName"].(string); ok && s != "" {
		return s
	}
	return "your supplier"
}
