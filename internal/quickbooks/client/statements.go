package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// STATEMENT_CREATE rides the Create statements dialog contract, captured on
// TC2 2026-09-18 (/app/statements → Save):
//
//	POST https://qbo.intuit.com/api/neo/v1/company/<realm>/statement/print
//	{"type":"1|2|3","customerIds":[<v3 customer id>],
//	 "startDate":"YYYY-MM-DD","endDate":"YYYY-MM-DD","statementDate":"YYYY-MM-DD"}
//
// Type enum (verified live): 1 = Balance Forward, 2 = Open Item (the UI
// auto-ranges start to statementDate-365d — honour it the same way),
// 3 = Transaction Statement. The dialog also fires statement/generate for the
// preview widget; print is the durable write.
const statementsReferer = "https://qbo.intuit.com/app/statements"

// PlannedStatementURL reports the endpoint for dry-run plans.
func PlannedStatementURL() string {
	return "https://qbo.intuit.com/api/neo/v1/company/{realm}/statement/print"
}

// statementTypeCode maps the CLI's --type aliases to the captured enum.
func statementTypeCode(s string) (string, error) {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", "-")) {
	case "1", "balance-forward", "balance forward", "balanceforward":
		return "1", nil
	case "2", "open-item", "open item", "openitem", "customer-statement", "customer statement":
		return "2", nil
	case "3", "transaction", "transaction-statement", "transaction statement", "transactionstatement":
		return "3", nil
	}
	return "", fmt.Errorf("--type %q: expected balance-forward | open-item | transaction (the three QBO statement formats)", s)
}

// ReplayStatementCreate creates a customer statement via the captured
// statement/print neo endpoint.
//
//	customer:  display name or v3 id (sub-customers should be statemented via
//	           the parent per QBO semantics).
//	startDate/endDate: statement period — accepts the wizard's D/M/YYYY and
//	           ISO forms via billDateISO. Ignored for open-item (the UI forces
//	           a trailing-365-day window there).
//	stype:     balance-forward | open-item | transaction (see statementTypeCode).
func ReplayStatementCreate(ctx context.Context, customer, startDate, endDate, stype string) (*MutateResult, error) {
	customer = strings.TrimSpace(customer)
	if customer == "" {
		return nil, fmt.Errorf("statement create requires --customer")
	}
	code, err := statementTypeCode(stype)
	if err != nil {
		return nil, err
	}
	end, err := billDateISO(endDate)
	if err != nil {
		return nil, fmt.Errorf("--end-date: %w", err)
	}
	start, err := billDateISO(startDate)
	if err != nil {
		return nil, fmt.Errorf("--start-date: %w", err)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	custID, err := resolveNameID(ctx, "Customer", customer)
	if err != nil {
		return nil, fmt.Errorf("customer: %w", err)
	}
	cid, err := strconv.ParseInt(custID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("customer id %q is not numeric", custID)
	}
	if code == "2" {
		// Open Item ignores the pickers and statements the trailing 365 days
		// ending on the statement date (captured: 2025-09-18 → 2026-09-18).
		t, perr := time.Parse("2006-01-02", end)
		if perr == nil {
			start = t.AddDate(0, 0, -365).Format("2006-01-02")
		}
	}
	body, err := json.Marshal(map[string]any{
		"type":          code,
		"customerIds":   []int64{cid},
		"startDate":     start,
		"endDate":       end,
		"statementDate": end,
	})
	if err != nil {
		return nil, err
	}
	u := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/statement/print"
	overrides := map[string]string{
		"Referer":      statementsReferer,
		"Content-Type": "application/json",
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, u, "qbo.intuit.com", body, overrides)
	if err != nil {
		return nil, fmt.Errorf("statement create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("statement create: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var receipt struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return nil, fmt.Errorf("statement create outcome unknown; inspect before retrying: unreadable response: %w", err)
	}
	if !strings.EqualFold(receipt.Status, "ok") {
		return nil, fmt.Errorf("statement create outcome unknown; inspect before retrying: expected status ok, got %q", receipt.Status)
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "statement create",
		Entity: "Statement",
		Item:   QueryItem{Type: "Statement", ID: custID, Name: customer},
		Note:   fmt.Sprintf("neo statement/print type=%s %s→%s", code, start, end),
	}, nil
}
