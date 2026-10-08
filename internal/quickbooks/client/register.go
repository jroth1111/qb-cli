// register.go replays the QBO register (ledger) transactions API.
//
// The register endpoint lives on a different API root than the ATS banking
// APIs used by ReplayAccounts/ReplayFeed:
//
//	GET https://qbo.intuit.com/api/neo/v1/company/{realm}/register/transactions/?accountId=ID
//
// It is still authenticated with the same captured session (Intuit_APIKey
// + cookie jar + CSRF), so it goes through newAPIClient().get. The response
// shape is not fully captured; ReplayRegister projects the known fields
// (id/txnId, date/txnDate, amount, description/memo/payee) and, when the
// shape is unrecognised, fails rather than reporting a false empty register.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// registerBaseURL is the neo API root for register transactions, scoped to
// a realm. The trailing slash is part of the path.
const registerBaseURL = "https://qbo.intuit.com/api/neo/v1/company/%s/register/transactions/"

// RegisterTxn is the secret-free projection of a QBO register transaction.
type RegisterTxn struct {
	ID            string  `json:"id"`
	Date          string  `json:"date"`
	Description   string  `json:"description"`
	Amount        float64 `json:"amount"`
	AccountID     string  `json:"account_id,omitempty"`
	Account       string  `json:"account,omitempty"`
	LineAccountID string  `json:"line_account_id,omitempty"`
	ClassID       string  `json:"class_id,omitempty"`
	Class         string  `json:"class,omitempty"`
	TxnTypeID     string  `json:"txn_type_id,omitempty"`
	TxnType       string  `json:"txn_type,omitempty"`
	Sequence      string  `json:"sequence,omitempty"`
	Memo          string  `json:"memo,omitempty"`
	// Raw server state, not a historical reconciliation certification.
	ClearState string `json:"clear_state,omitempty"`
}

// RegisterResult is the JSON envelope returned by ReplayRegister.
type RegisterResult struct {
	Status       int            `json:"status"`
	Counts       map[string]int `json:"counts"`
	Transactions []RegisterTxn  `json:"transactions"`
}

// ReplayRegister calls the register transactions endpoint for the given
// account and projects ledger transactions. accountID defaults to
// DefaultAccountID when empty. limit maps to the x-range header
// (items=0-{limit-1}). It fails fast with a wrapped ErrNoCredentials when
// no credentials are saved, before any network activity.
//
// The response shape is not fully known. An unrecognisable or truncated body
// is an error, never an empty-register result. The raw body is not included
// in output or errors.
func ReplayRegister(ctx context.Context, accountID string, limit int) (*RegisterResult, error) {
	return ReplayRegisterPage(ctx, accountID, 0, limit)
}

// ReplayRegisterPage preserves the register's explicit X-Range pagination.
// Positive limits return one bounded page; zero walks until an empty page.
func ReplayRegisterPage(ctx context.Context, accountID string, offset, limit int) (*RegisterResult, error) {
	if offset < 0 || (limit > 0 && offset > int(^uint(0)>>1)-limit) {
		return nil, fmt.Errorf("invalid register offset/limit")
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	res, err := replayRegisterOnce(ctx, accountID, offset, limit)
	if err == nil || !allowInactiveFallback(accountID) || !isInactiveAccount(err) {
		return res, err
	}
	if live, ok := otherLiveAccount(ctx, accountID); ok {
		res, err = replayRegisterOnce(ctx, live, offset, limit)
		if err == nil || !isInactiveAccount(err) {
			return res, err
		}
	}
	return &RegisterResult{
		Status:       http.StatusOK,
		Counts:       map[string]int{"transactions": 0},
		Transactions: []RegisterTxn{},
	}, nil
}

func replayRegisterOnce(ctx context.Context, accountID string, offset, limit int) (*RegisterResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		// Open-ended X-Range requests are rejected by the live register.
		// Reuse the empty-terminal-page walk and its repeated-page guard.
		rows, err := reconciliationRegisterRows(ctx, ac, accountID)
		if err != nil {
			return nil, fmt.Errorf("register transactions: %w", err)
		}
		if offset >= len(rows) {
			rows = []map[string]any{}
		} else {
			rows = rows[offset:]
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			return nil, err
		}
		txns, ok := extractRegisterTxns(raw)
		if !ok {
			return nil, fmt.Errorf("register response is not a transaction list")
		}
		return projectRegister(txns), nil
	}
	url := fmt.Sprintf(registerBaseURL+"?accountId=%s", ac.realm, accountID)
	xRange := fmt.Sprintf("items=%d-%d", offset, offset+limit-1)

	resp, err := ac.get(ctx, url, xRange)
	if err != nil {
		return nil, fmt.Errorf("register transactions: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading register transactions: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}

	txns, ok := extractRegisterTxns(body)
	if !ok {
		return nil, fmt.Errorf("register response is not a transaction list (%d bytes); use a smaller --limit and explicit --offset pagination", len(body))
	}
	return projectRegister(txns), nil
}

// --- raw register envelope (secret-free fields only) ---------------------

// rawRegisterTxn captures the union of field names the register API may use
// for each transaction. The projection picks the first non-empty value in
// each alias group.
type rawRegisterTxn struct {
	ID            flexibleString `json:"id"`
	TxnID         flexibleString `json:"txnId"`
	Date          string         `json:"date"`
	TxnDate       string         `json:"txnDate"`
	Amount        *float64       `json:"amount"`
	Description   string         `json:"description"`
	Memo          string         `json:"memo"`
	Payee         string         `json:"payee"`
	AccountID     flexibleString `json:"accountId"`
	Account       string         `json:"accountName"`
	LineAccountID flexibleString `json:"lineAccountId"`
	ClassID       flexibleString `json:"klassId"`
	Class         string         `json:"klass"`
	TxnTypeID     flexibleString `json:"txnTypeId"`
	TxnType       string         `json:"txnTypeString"`
	Sequence      flexibleString `json:"sequence"`
	ClearState    flexibleString `json:"clearState"`
}

// extractRegisterTxns tries several plausible response envelopes and
// returns the transaction slice when one matches. The second return is
// false when the body shape is unrecognised (not a bare array and not an
// object with an items/transactions field carrying a JSON array).
func extractRegisterTxns(body []byte) ([]rawRegisterTxn, bool) {
	// Bare JSON array.
	var arr []rawRegisterTxn
	if json.Unmarshal(body, &arr) == nil {
		return arr, true
	}

	// Object with "items" (matching the banking getTransactions envelope).
	var withItems struct {
		Items []rawRegisterTxn `json:"items"`
	}
	if json.Unmarshal(body, &withItems) == nil && withItems.Items != nil {
		return withItems.Items, true
	}

	// Object with "transactions".
	var withTxns struct {
		Transactions []rawRegisterTxn `json:"transactions"`
	}
	if json.Unmarshal(body, &withTxns) == nil && withTxns.Transactions != nil {
		return withTxns.Transactions, true
	}

	return nil, false
}

// --- projection (secret-free output) --------------------------------------

func projectRegister(txns []rawRegisterTxn) *RegisterResult {
	out := make([]RegisterTxn, 0, len(txns))
	for _, t := range txns {
		id := t.ID.String()
		if id == "" {
			id = t.TxnID.String()
		}
		date := t.Date
		if date == "" {
			date = t.TxnDate
		}
		desc := t.Description
		if desc == "" {
			desc = t.Memo
		}
		if desc == "" {
			desc = t.Payee
		}
		out = append(out, RegisterTxn{
			ID:            id,
			Date:          date,
			Description:   desc,
			Amount:        orZero(t.Amount),
			AccountID:     t.AccountID.String(),
			Account:       t.Account,
			LineAccountID: t.LineAccountID.String(),
			ClassID:       t.ClassID.String(),
			Class:         t.Class,
			TxnTypeID:     t.TxnTypeID.String(),
			TxnType:       t.TxnType,
			Sequence:      t.Sequence.String(),
			Memo:          t.Memo,
			ClearState:    t.ClearState.String(),
		})
	}

	return &RegisterResult{
		Status:       http.StatusOK,
		Counts:       map[string]int{"transactions": len(out)},
		Transactions: out,
	}
}
