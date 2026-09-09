// Package client wraps the QBO frontend banking APIs with an in-process Go
// HTTP client. The transport ladder (see http.go) tries stdlib net/http
// first and retries a 403 once through a surf Chrome-impersonating client.
// No Python helper is involved.
//
// Entry points (ReplayAccounts, ReplayFeed, ReplayLookup, ReplayRegister,
// and the backward-compatible ReplayPending) all guard on the presence of
// saved credentials before any network activity, so a missing-credentials
// failure is deterministic and never reaches the network. RequireSession
// performs the same guard without any network activity for mutation stubs.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// Account is the secret-free projection of a QBO banking account.
type Account struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Balance float64 `json:"balance"`
}

// AccountsResult is the JSON envelope returned by ReplayAccounts.
type AccountsResult struct {
	Status   int            `json:"status"`
	Counts   map[string]int `json:"counts"`
	Accounts []Account      `json:"accounts"`
}

// Transaction is the secret-free projection of a pending review txn.
type Transaction struct {
	ID          string  `json:"id"`
	OLBTxnID    string  `json:"olbTxnId,omitempty"`
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	Description string  `json:"description"`
	AcceptType  string  `json:"acceptType"`
}

// PendingResult is the JSON envelope returned by ReplayPending.
type PendingResult struct {
	Status       int            `json:"status"`
	Counts       map[string]int `json:"counts"`
	Transactions []Transaction  `json:"transactions"`

	// Expected is the server-authoritative numTxnToReview from
	// getInitialData for the requested account; -1 when unknown.
	// Complete reports whether len(Transactions) == Expected.
	Expected  int    `json:"expected,omitempty"`
	Complete  bool   `json:"complete"`
	AccountID string `json:"accountId,omitempty"`
}

// DefaultAccountID is the account used when `qb feed pending` omits
// --account-id. Override per call; list accounts to find yours.
const DefaultAccountID = "204"

// ErrNoCredentials is returned when no credentials file is present. It
// wraps auth.ErrNoCredentials so callers can branch on either the client
// sentinel or the underlying auth cause via errors.Is.
var ErrNoCredentials = &credentialError{inner: auth.ErrNoCredentials}

// credentialError lets ErrNoCredentials carry a client-package identity
// while still unwrapping to auth.ErrNoCredentials.
type credentialError struct{ inner error }

func (e *credentialError) Error() string {
	if e == nil || e.inner == nil {
		return ""
	}
	return e.inner.Error()
}

func (e *credentialError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.inner
}

// ReplayAccounts calls getInitialData and projects the connected banking
// accounts (id/name/type/balance). It fails fast with a wrapped
// ErrNoCredentials when no credentials are saved, before any network
// activity. No secrets are returned.
func ReplayAccounts(ctx context.Context) (*AccountsResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	atsURL := ac.baseURL() + "/getInitialData"
	res, err := decodeInitialData(ctx, ac, atsURL)
	if err == nil {
		return res, nil
	}
	neoURL := fmt.Sprintf("https://qbo.intuit.com/api/neo/v1/company/%s/olb/ng/getInitialData", ac.realm)
	res, neoErr := decodeInitialData(ctx, ac, neoURL)
	if neoErr != nil {
		return nil, fmt.Errorf("getInitialData: %v; neo: %w", err, neoErr)
	}
	return res, nil
}

func decodeInitialData(ctx context.Context, ac *apiClient, url string) (*AccountsResult, error) {
	resp, err := ac.get(ctx, url, "")
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading getInitialData: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	var raw initialData
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decoding getInitialData: %w", err)
	}
	return projectAccounts(&raw), nil
}

// ReplayFeed calls getTransactions for the given account and projects
// review transactions for the requested review state. accountID defaults
// to DefaultAccountID when empty. An empty reviewState defaults to
// "PENDING" (after uppercasing and trimming). limit maps to the x-range
// header (items=0-{limit-1}). It fails fast with a wrapped ErrNoCredentials
// when no credentials are saved, before any network activity. No secrets
// are returned.
func ReplayFeed(ctx context.Context, accountID, reviewState string, limit int) (*PendingResult, error) {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	if limit < 1 {
		limit = 1
	}
	res, err := replayFeedOnce(ctx, accountID, reviewState, limit)
	if err == nil || !allowInactiveFallback(accountID) || !isInactiveAccount(err) {
		return res, err
	}
	if live, ok := otherLiveAccount(ctx, accountID); ok {
		res, err = replayFeedOnce(ctx, live, reviewState, limit)
		if err == nil || !isInactiveAccount(err) {
			return res, err
		}
	}
	return &PendingResult{
		Status:       http.StatusOK,
		Counts:       map[string]int{"transactions": 0},
		Transactions: []Transaction{},
	}, nil
}

func replayFeedOnce(ctx context.Context, accountID, reviewState string, limit int) (*PendingResult, error) {
	state := strings.ToUpper(strings.TrimSpace(reviewState))
	if state == "" {
		state = "PENDING"
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/getTransactions?sort=-txnDate&reviewState=%s&ignoreMatching=false&accountId=%s",
		ac.baseURL(), state, accountID)
	xRange := fmt.Sprintf("items=0-%d", limit-1)

	resp, err := ac.get(ctx, url, xRange)
	if err != nil {
		return nil, fmt.Errorf("getTransactions: %w", err)
	}
	defer drainAndClose(resp)

	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading getTransactions: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}

	var raw pendingData
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decoding getTransactions: %w", err)
	}
	return projectPending(&raw), nil
}

// ReplayPending is the original entry point for pending review
// transactions. It is preserved for backward compatibility and delegates
// to ReplayFeed with a default review state of "PENDING".
func ReplayPending(ctx context.Context, accountID, reviewState string, limit int) (*PendingResult, error) {
	return ReplayFeed(ctx, accountID, reviewState, limit)
}

// feedPage is one server response from the getTransactions walk.
type feedPage struct {
	Items []rawTxn `json:"items"`
}

// ReplayFeedComplete walks getTransactions with startIndex/chunkSize +
// x-range until the full population for the requested review state has
// been retrieved, then verifies completeness against the
// server-authoritative numTxnToReview from getInitialData.
//
// Server contract:
//   - A single unpaged request is capped at 999 items regardless of
//     query params.
//   - Paging works: startIndex + chunkSize + x-range items=start-end
//     return distinct pages.
//   - Date parameters (fromDate/toDate/startDate/endDate/txnDateFilter/
//     dateFilter) are ignored server-side. They are not sent; date
//     slicing must be done client-side by filtering Transaction.Date.
//
// When expectedCount > 0 and unique ids fetched < expectedCount after the
// hard page ceiling, ErrIncomplete is returned rather than a silently
// partial result. accountID defaults to DefaultAccountID; an empty
// reviewState defaults to PENDING. maxPages bounds runaway loops.
func ReplayFeedComplete(ctx context.Context, accountID, reviewState string, maxPages int) (*PendingResult, error) {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	state := strings.ToUpper(strings.TrimSpace(reviewState))
	if state == "" {
		state = "PENDING"
	}
	if maxPages < 1 {
		maxPages = 40 // 300/page x 40 = 12000 rows ceiling
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}

	expected := ac.expectedPending(ctx, accountID)

	const pageSize = 300
	seen := make(map[string]struct{})
	out := make([]Transaction, 0)
	for pageNum := 0; pageNum < maxPages; pageNum++ {
		start := pageNum * pageSize
		page, err := fetchFeedPage(ctx, ac, accountID, state, start, pageSize)
		if err != nil {
			return nil, err
		}
		for _, t := range page.Items {
			if _, dup := seen[t.ID]; !dup {
				seen[t.ID] = struct{}{}
				out = append(out, projectTxn(t))
			}
		}
		// Stop early once we have the server-reported count — avoids an
		// extra page when the oracle is satisfied mid-page.
		if expected > 0 && len(out) >= expected {
			break
		}
		if len(page.Items) < pageSize {
			break // short page: end of population
		}
	}

	res := &PendingResult{
		Status:       http.StatusOK,
		Counts:       map[string]int{"transactions": len(out)},
		Transactions: out,
		Expected:     expected,
		AccountID:    accountID,
	}

	var complete bool
	switch {
	case expected == 0 && len(out) == 0:
		complete = true
	case expected > 0 && len(out) == expected:
		complete = true
	default:
		complete = false
	}
	res.Complete = complete

	if !complete {
		if expected > 0 {
			return res, fmt.Errorf("%w: fetched %d unique ids, server reports %d pending",
				ErrIncomplete, len(out), expected)
		} else if expected < 0 {
			return res, fmt.Errorf("%w: fetched %d unique ids, server count unavailable",
				ErrIncomplete, len(out))
		}
		return res, fmt.Errorf("%w: fetched %d rows", ErrIncomplete, len(out))
	}
	return res, nil
}

// ErrIncomplete wraps a complete-but-mismatched walk result so callers can
// distinguish "verified complete" from "exhausted pages without reaching
// the server count".
var ErrIncomplete = errors.New("feed enumeration incomplete")

// expectedPending reads numTxnToReview for accountID from getInitialData.
// Returns -1 when the account is absent or the count is absent.
func (c *apiClient) expectedPending(ctx context.Context, accountID string) int {
	resp, err := c.get(ctx, c.baseURL()+"/getInitialData", "")
	if err != nil {
		return -1
	}
	defer drainAndClose(resp)
	body, err := readBody(resp)
	if err != nil || resp.StatusCode != http.StatusOK {
		return -1
	}
	var raw initialData
	if json.Unmarshal(body, &raw) != nil {
		return -1
	}
	for _, a := range raw.Accounts {
		if rawID(a.QboAccountID) == accountID && a.NumTxnToReview != nil {
			return *a.NumTxnToReview
		}
	}
	return -1
}

// fetchFeedPage requests one startIndex/chunkSize page of the feed.
func fetchFeedPage(ctx context.Context, ac *apiClient, accountID, state string, start, size int) (*feedPage, error) {
	url := fmt.Sprintf("%s/getTransactions?sort=-txnDate&reviewState=%s&ignoreMatching=false&accountId=%s&startIndex=%d&chunkSize=%d",
		ac.baseURL(), state, accountID, start, size)
	xRange := fmt.Sprintf("items=%d-%d", start, start+size-1)
	resp, err := ac.get(ctx, url, xRange)
	if err != nil {
		return nil, fmt.Errorf("getTransactions page @%d: %w", start, err)
	}
	defer drainAndClose(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading getTransactions page @%d: %w", start, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	var page feedPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("decoding getTransactions page @%d: %w", start, err)
	}
	return &page, nil
}

// projectTxn projects one raw transaction (shared with projectPending).
func projectTxn(t rawTxn) Transaction {
	desc := t.Description
	if t.OrigDescription != "" && (desc == "" || len(t.OrigDescription) > len(desc)) {
		desc = t.OrigDescription
	}
	return Transaction{
		ID:          t.ID,
		OLBTxnID:    t.OlbTxnID,
		Date:        t.OlbTxnDate,
		Amount:      orZero(t.Amount),
		Description: desc,
		AcceptType:  t.AcceptType,
	}
}

// ErrEmptyQuery is returned by ReplayLookup when the query is empty or
// whitespace-only. The lookup is a client-side filter over already-fetched
// transactions, so an empty query would match everything and is rejected
// before any network activity.
var ErrEmptyQuery = errors.New("replay lookup: query must not be empty")

// ReplayLookup fetches transactions across all review states (PENDING,
// ACCEPTED, EXCLUDED) via ReplayFeed and filters to those whose id,
// olbTxnId, or description contains query (case-insensitive). Hyphens
// and underscores fold to spaces so "QB-CLI-TEST" matches
// "Qb Cli Test Do Not" (the title-case truncation QBO stores on import).
// An empty query returns ErrEmptyQuery before any network activity.
func ReplayLookup(ctx context.Context, accountID, query string, limit int) (*PendingResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, ErrEmptyQuery
	}

	states := []string{"PENDING", "ACCEPTED", "EXCLUDED"}
	combined := make([]Transaction, 0)
	for _, state := range states {
		res, err := ReplayFeed(ctx, accountID, state, limit)
		if err != nil {
			return nil, err
		}
		for _, t := range res.Transactions {
			if txnMatches(t, query) {
				combined = append(combined, t)
			}
		}
	}

	return &PendingResult{
		Status:       http.StatusOK,
		Counts:       map[string]int{"transactions": len(combined)},
		Transactions: combined,
	}, nil
}

// txnMatches reports whether a projected transaction matches query.
// Needle and haystacks are lowercased and have '-' / '_' folded to spaces.
func txnMatches(t Transaction, query string) bool {
	needle := normalizeLookup(query)
	if needle == "" {
		return false
	}
	for _, field := range []string{t.ID, t.OLBTxnID, t.Description} {
		if strings.Contains(normalizeLookup(field), needle) {
			return true
		}
	}
	return false
}

func normalizeLookup(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.Join(strings.Fields(s), " ")
}

// ReplayError is returned when the API responds with a non-200 status.
type ReplayError struct {
	Status  int
	Message string
}

func (e *ReplayError) Error() string {
	if e == nil {
		return ""
	}
	if e.Status != 0 {
		return fmt.Sprintf("replay failed (status %d): %s", e.Status, e.Message)
	}
	return fmt.Sprintf("replay failed: %s", e.Message)
}

// isInactiveAccount reports a 400 whose message says the requested
// account/customer/item is inactive. This remint's DefaultAccountID (204)
// is gone; the host says "inactive field" / "made inactive". We do not
// send an Active/inactive query clause on register or feed.
func isInactiveAccount(err error) bool {
	var re *ReplayError
	if !errors.As(err, &re) || re == nil {
		return false
	}
	if re.Status != http.StatusBadRequest {
		return false
	}
	return strings.Contains(strings.ToLower(re.Message), "inactive")
}

// allowInactiveFallback is true only for the omitted/--account-id default
// (204). An explicit other id is left as a 400 so we do not invent a row.
func allowInactiveFallback(accountID string) bool {
	return accountID == "" || accountID == DefaultAccountID
}

// otherLiveAccount returns the first getInitialData account that is not
// used. Missing credentials or an empty list → false (caller returns
// honest empty 200).
func otherLiveAccount(ctx context.Context, used string) (string, bool) {
	res, err := ReplayAccounts(ctx)
	if err != nil || res == nil {
		return "", false
	}
	for _, a := range res.Accounts {
		if a.ID != "" && a.ID != used {
			return a.ID, true
		}
	}
	return "", false
}

// --- raw API envelopes (secret-free fields only) -------------------------

// initialData is the subset of getInitialData we project from.
type initialData struct {
	Accounts []rawAccount `json:"accounts"`
}

type rawAccount struct {
	QboAccountID       json.RawMessage `json:"qboAccountId"`
	OlbAccountNickname string          `json:"olbAccountNickname"`
	QboAccountFullName string          `json:"qboAccountFullName"`
	FiName             string          `json:"fiName"`
	QboAccountType     string          `json:"qboAccountType"`
	QboBalance         *float64        `json:"qboBalance"`
	// NumTxnToReview is the server-authoritative count of pending
	// review transactions for this account; the completeness oracle
	// for feed enumeration.
	NumTxnToReview *int `json:"numTxnToReview"`
}

// pendingData is the subset of getTransactions we project from.
type pendingData struct {
	Items                  []rawTxn `json:"items"`
	TotalTransactionsCount *int     `json:"totalTransactionsCount"`
}

type rawTxn struct {
	ID              string   `json:"id"`
	OlbTxnID        string   `json:"olbTxnId"`
	OlbTxnDate      string   `json:"olbTxnDate"`
	Amount          *float64 `json:"amount"`
	Description     string   `json:"description"`
	OrigDescription string   `json:"origDescription"`
	AcceptType      string   `json:"acceptType"`
}

// --- projection (secret-free output) -------------------------------------

func projectAccounts(data *initialData) *AccountsResult {
	out := make([]Account, 0, len(data.Accounts))
	for _, a := range data.Accounts {
		name := a.OlbAccountNickname
		if name == "" {
			name = a.QboAccountFullName
		}
		if name == "" {
			name = a.FiName
		}
		out = append(out, Account{
			ID:      rawID(a.QboAccountID),
			Name:    name,
			Type:    a.QboAccountType,
			Balance: orZero(a.QboBalance),
		})
	}
	return &AccountsResult{
		Status:   http.StatusOK,
		Counts:   map[string]int{"accounts": len(out)},
		Accounts: out,
	}
}

func projectPending(data *pendingData) *PendingResult {
	out := make([]Transaction, 0, len(data.Items))
	for _, t := range data.Items {
		out = append(out, projectTxn(t))
	}
	counts := map[string]int{"transactions": len(out)}
	if data.TotalTransactionsCount != nil {
		counts["total"] = *data.TotalTransactionsCount
	}
	return &PendingResult{
		Status:       http.StatusOK,
		Counts:       counts,
		Transactions: out,
	}
}

// --- helpers --------------------------------------------------------------

// readBody reads the full response body, capped at 1 MiB so a runaway
// response cannot exhaust memory.
func readBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// errorMessage extracts a human-readable message from a JSON error body
// without leaking secrets. Falls back to a generic non-200 message.
func errorMessage(body []byte) string {
	var probe struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Fault   struct {
			Type  string `json:"type"`
			Error []struct {
				Message string `json:"Message"`
				Detail  string `json:"Detail"`
				Code    string `json:"code"`
			} `json:"Error"`
		} `json:"Fault"`
	}
	if json.Unmarshal(body, &probe) == nil {
		if probe.Message != "" {
			return probe.Message
		}
		if probe.Error != "" {
			return probe.Error
		}
		if len(probe.Fault.Error) > 0 {
			e := probe.Fault.Error[0]
			msg := e.Message
			if e.Detail != "" {
				if msg != "" {
					msg = msg + ": " + e.Detail
				} else {
					msg = e.Detail
				}
			}
			if e.Code != "" && msg != "" {
				return e.Code + ": " + msg
			}
			if msg != "" {
				return msg
			}
			if e.Code != "" {
				return e.Code
			}
		}
		if probe.Fault.Type != "" {
			return probe.Fault.Type
		}
	}

	return "non-200 response"
}

func rawID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return formatFloatID(&f)
	}
	return strings.Trim(string(raw), `"`)
}

func formatFloatID(v *float64) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", *v)
	if i := strings.Index(s, "."); i >= 0 {
		frac := s[i+1:]
		if frac == "0" || frac == "" {
			return s[:i]
		}
	}
	return s
}

// orZero returns *v dereferenced, or 0 when v is nil.
func orZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
