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

// CHEQUE_BOUNCE — no form exposes a "bounce" action (expense/payment More
// menus checked on TC2 2026-09-19: Copy/Void/Delete/Journal/Audit only) and v3
// empirically rejects A/R in expense lines (400 6430 "Invalid account type
// used"). The AU help docs the catalog note cites (L4O144llZ, L4rxNrG0Y,
// L87982gDC) describe a manual procedure: take the money back out of the bank
// into A/R against the customer, plus record the bank's fee. The journal-entry
// variant is the only one v3 accepts — JournalEntryLineDetail carries A/R with
// a customer entity (Type must be title case "Customer"; "CUSTOMER" is a 2010
// "unsupported property" — verified live on TC2, JE 195).
//
//	POST /api/v3/company/{realm}/journalentry
//	Line1: Debit A/R × payment.TotalAmt, Entity {EntityRef: customer, Type: Customer}
//	Line2: Credit payment.DepositToAccountRef × TotalAmt
//	(--fee) Line3: Debit bank-charges account × fee
//	(--fee) Line4: Credit same deposit account × fee
//
// --id is the v3 Payment id whose cheque bounced. Reversal restores the
// receivable without touching the payment record — the doc's JE variant
// settles it via re-invoice, which is a separate action.
func ReplayChequeBounce(ctx context.Context, id, date, feeS string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("cheque bounce requires --id (v3 Payment id)")
	}
	iso := ""
	if s := strings.TrimSpace(date); s != "" {
		var err error
		iso, err = billDateISO(s)
		if err != nil {
			return nil, fmt.Errorf("--date: %w", err)
		}
	}
	if iso == "" {
		iso = time.Now().Format("2006-01-02")
	}
	fee := 0.0
	if s := strings.TrimSpace(feeS); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("--fee must be a non-negative amount, got %q", feeS)
		}
		fee = v
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	pay, err := bouncePayment(ctx, ac, id)
	if err != nil {
		return nil, err
	}
	amt := bounceFloat(pay["TotalAmt"])
	if amt <= 0 && fee <= 0 {
		return nil, fmt.Errorf("payment %s has TotalAmt %v and no --fee — nothing to reverse", id, pay["TotalAmt"])
	}
	arID, err := bounceAccountByType(ctx, ac, "Accounts Receivable")
	if err != nil {
		return nil, fmt.Errorf("resolving A/R account: %w", err)
	}
	bankID := refValue(pay, "DepositToAccountRef")
	if bankID == "" {
		if bankID, err = bounceAccountByName(ctx, ac, "Undeposited funds"); err != nil {
			return nil, fmt.Errorf("payment %s has no DepositToAccountRef and no 'Undeposited funds' account exists", id)
		}
	}
	custID := refValue(pay, "CustomerRef")
	if custID == "" {
		return nil, fmt.Errorf("payment %s has no CustomerRef — bounce only applies to customer payments", id)
	}
	custName := refName(pay, "CustomerRef")
	desc := fmt.Sprintf("Bounced cheque — payment %s", id)
	lines := []map[string]any{}
	add := func(posting, acct string, amount float64, desc string, entity map[string]any) {
		detail := map[string]any{
			"PostingType": posting,
			"AccountRef":  map[string]any{"value": acct},
		}
		if entity != nil {
			detail["Entity"] = entity
		}
		lines = append(lines, map[string]any{
			"DetailType":             "JournalEntryLineDetail",
			"Amount":                 amount,
			"Description":            desc,
			"JournalEntryLineDetail": detail,
		})
	}
	custEntity := map[string]any{
		"EntityRef": map[string]any{"value": custID},
		"Type":      "Customer",
	}
	if amt > 0 {
		add("Debit", arID, amt, desc, custEntity)
		add("Credit", bankID, amt, desc, nil)
	}
	if fee > 0 {
		chargesID, err := bounceAccountByName(ctx, ac, "Bank charges and fees", "Bank charges", "Bank fees", "Bank Charges")
		if err != nil {
			return nil, fmt.Errorf("--fee %v needs a bank-charges expense account (none found by name; e.g. 'Bank charges and fees')", fee)
		}
		add("Debit", chargesID, fee, desc+" — bank fee", custEntity)
		add("Credit", bankID, fee, desc+" — bank fee", nil)
	}
	jeID, err := createJournalEntry(ctx, ac, iso, lines)
	if err != nil {
		return nil, fmt.Errorf("cheque bounce: %w", err)
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "bounce",
		Entity: "JournalEntry",
		Item:   QueryItem{Type: "JournalEntry", ID: jeID, Name: custName, Amount: amt + fee},
		Note:   fmt.Sprintf("payment %s reversed to A/R", id),
	}, nil
}

// createJournalEntry POSTs a v3 JournalEntry ({TxnDate, Line}) and returns the
// created Id. Shared by the cheque-bounce reversal and the credit transfer.
func createJournalEntry(ctx context.Context, ac *apiClient, txnDate string, lines []map[string]any) (string, error) {
	body, err := json.Marshal(map[string]any{
		"TxnDate": txnDate,
		"Line":    lines,
	})
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/journalentry?minorversion=73", ac.realm)
	resp, err := ac.postJSON(ctx, u, body)
	if err != nil {
		return "", err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	got, err := readBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
	}
	var wrap map[string]json.RawMessage
	if json.Unmarshal(got, &wrap) != nil || wrap["JournalEntry"] == nil {
		return "", fmt.Errorf("malformed response %.200s", got)
	}
	var je map[string]any
	if err := json.Unmarshal(wrap["JournalEntry"], &je); err != nil {
		return "", err
	}
	jeID, _ := je["Id"].(string)
	if jeID == "" {
		return "", fmt.Errorf("no JournalEntry id in response %.200s", got)
	}
	return jeID, nil
}

// bouncePayment fetches the Payment record being bounced.
func bouncePayment(ctx context.Context, ac *apiClient, id string) (map[string]any, error) {
	rows, err := queryV3Rows(ctx, ac, "Payment", fmt.Sprintf("select * from Payment where Id='%s'", strings.ReplaceAll(id, "'", "")))
	if err != nil {
		return nil, fmt.Errorf("fetching payment %s: %w", id, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no payment with id %s (cheque bounce takes a v3 Payment id)", id)
	}
	return rows[0], nil
}

// bounceAccountByType resolves the first account of a v3 AccountType.
func bounceAccountByType(ctx context.Context, ac *apiClient, acctType string) (string, error) {
	rows, err := queryV3Rows(ctx, ac, "Account",
		fmt.Sprintf("select Id from Account where AccountType='%s'", strings.ReplaceAll(acctType, "'", "")))
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no account of type %q", acctType)
	}
	id, _ := rows[0]["Id"].(string)
	if id == "" {
		return "", fmt.Errorf("account of type %q returned no Id", acctType)
	}
	return id, nil
}

// bounceAccountByName resolves an account by exact name match, trying each
// candidate in order.
func bounceAccountByName(ctx context.Context, ac *apiClient, names ...string) (string, error) {
	rows, err := queryV3Rows(ctx, ac, "Account", "select Id,Name from Account")
	if err != nil {
		return "", err
	}
	for _, want := range names {
		for _, row := range rows {
			name, _ := row["Name"].(string)
			if strings.EqualFold(name, want) {
				if id, _ := row["Id"].(string); id != "" {
					return id, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no account named any of %v", names)
}

func refValue(obj map[string]any, key string) string {
	if r, ok := obj[key].(map[string]any); ok {
		if v, _ := r["value"].(string); v != "" {
			return v
		}
	}
	return ""
}

func refName(obj map[string]any, key string) string {
	if r, ok := obj[key].(map[string]any); ok {
		if v, _ := r["name"].(string); v != "" {
			return v
		}
	}
	return ""
}

func bounceFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
