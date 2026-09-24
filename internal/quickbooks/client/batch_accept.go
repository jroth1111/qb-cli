package client

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// The current banking SPA no longer calls the discrete
// categoriseTransactions / splitTransactions / matchTransactions endpoints
// the CLI was built against. Observed live contract (Space 1, Test Company 2,
// 2026-09-15 captures in .sol/evidence/fusion-remaining-safe-21):
//
//   - Post / categorise / split all ride
//     POST /olb/ng/batchAcceptTransactions?acceptOnly=true with
//     {nextTxnInfo:{accountId,nextTransactionIndex:-1,reviewState:PENDING,
//     sort:-txnDate}, txnList:{olbTxns:[<full feed-row objects>]}}.
//   - Match rides POST /olb/ng/acceptTransactions (no query, no
//     nextTxnInfo) with a reduced olbTxns entry shape.
//
// Mutation builders therefore fetch the live feed row first and echo the
// whole object back with only the observed edits applied, so unknown JSON
// fields are retained instead of sending a lossy projection.

// batchAcceptQuery is the accept-only marker the SPA sends on every
// batchAcceptTransactions mutation call.
const batchAcceptQuery = "?acceptOnly=true"

const feedMutationPageSize = 300

// --- feed row fetch ------------------------------------------------------

// feedRowsForIDs pages getTransactions (PENDING review state) for accountID
// and returns the raw item objects matching wanted ids, in request order.
// An id matches a row's olbTxnId first, then its :ofx display id. Missing
// rows fail with ErrFeedRowsNotFound before any mutation is sent.
func feedRowsForIDs(ctx context.Context, ac *apiClient, accountID string, ids []string) ([]map[string]any, error) {
	want := make(map[string]int, len(ids))
	for i, id := range ids {
		want[id] = i
	}
	found := make([]map[string]any, len(ids))
	for page := 0; page < 10 && len(want) > 0; page++ {
		start := page * feedMutationPageSize
		url := fmt.Sprintf("%s/getTransactions?sort=-txnDate&reviewState=PENDING&ignoreMatching=false&accountId=%s&startIndex=%d&chunkSize=%d",
			ac.neoFeedURL(), accountID, start, feedMutationPageSize)
		resp, err := ac.get(ctx, url, fmt.Sprintf("items=%d-%d", start, start+feedMutationPageSize-1))
		if err != nil {
			return nil, fmt.Errorf("getTransactions: %w", err)
		}
		body, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return nil, fmt.Errorf("reading getTransactions: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
		}
		var env struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, fmt.Errorf("decoding getTransactions: %w", err)
		}
		for _, row := range env.Items {
			for id, idx := range want {
				if feedRowMatchesID(row, id) {
					found[idx] = row
					delete(want, id)
					break
				}
			}
		}
		if len(env.Items) < feedMutationPageSize {
			break
		}
	}
	var missing []string
	for i, row := range found {
		if row == nil {
			missing = append(missing, ids[i])
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("%w: %s (account %s)", ErrFeedRowsNotFound, strings.Join(missing, ", "), accountID)
	}
	return found, nil
}

// feedRowMatchesID reports whether a raw getTransactions item is the row a
// caller asked for. olbTxnId is the documented id; the :ofx display id is
// accepted as a fallback so either form resolves the same live row.
func feedRowMatchesID(row map[string]any, id string) bool {
	if id == "" {
		return false
	}
	if s, _ := row["olbTxnId"].(string); s == id {
		return true
	}
	d, _ := row["id"].(string)
	return d == id || d == id+":ofx"
}

// --- register row fetch (match candidate source) -------------------------

// registerRowsForAccount pages register/transactions for accountID and
// returns the raw rows. The register is the same OLB surface the match UI
// draws candidates from: each row carries txnId, txnTypeId, sequence,
// editSequence and the payment/deposit amount the acceptTransactions
// selectedMatches contract needs.
func registerRowsForAccount(ctx context.Context, ac *apiClient, accountID string) ([]map[string]any, error) {
	var out []map[string]any
	seen := make(map[string]bool)
	for page := range 10 {
		start := page * feedMutationPageSize
		url := fmt.Sprintf(registerBaseURL+"?accountId=%s", ac.realm, accountID)
		resp, err := ac.get(ctx, url, fmt.Sprintf("items=%d-%d", start, start+feedMutationPageSize-1))
		if err != nil {
			return nil, fmt.Errorf("register transactions: %w", err)
		}
		body, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return nil, fmt.Errorf("reading register transactions: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
		}
		items, err := registerRowItems(body)
		if err != nil {
			return nil, fmt.Errorf("decoding register transactions: %w", err)
		}
		fresh := 0
		for _, row := range items {
			key := jsonNumberString(row["txnId"])
			if key == "" {
				key = fmt.Sprintf("%p", row)
			}
			if !seen[key] {
				seen[key] = true
				fresh++
				out = append(out, row)
			}
		}
		if len(items) < feedMutationPageSize || fresh == 0 {
			break
		}
	}
	return out, nil
}

// registerRowItems decodes the register response into raw row objects. The
// live response is a bare JSON array; the items/transactions envelopes are
// accepted for the same shapes extractRegisterTxns tolerates.
func registerRowItems(body []byte) ([]map[string]any, error) {
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}
	for _, key := range []string{"items", "transactions"} {
		var env map[string]json.RawMessage
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, err
		}
		raw, ok := env[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(raw, &arr); err == nil {
			return arr, nil
		}
	}
	return nil, fmt.Errorf("unrecognised register response shape")
}

// --- shared row helpers ----------------------------------------------------

func mapStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func mapNum(m map[string]any, key string) (float64, bool) {
	f, ok := m[key].(float64)
	return f, ok
}

// jsonNumberString renders a JSON numeric (or numeric string) field without
// trailing-zero noise: 54 -> "54", 0 -> "0".
func jsonNumberString(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case string:
		return n
	case json.Number:
		return n.String()
	default:
		return ""
	}
}

func setIfAbsent(m map[string]any, key string, v any) {
	if _, ok := m[key]; !ok {
		m[key] = v
	}
}

// addAsQboTxnMap returns the row's addAsQboTxn object, creating it when the
// feed row predates the field (unseen rows keep their other fields).
func addAsQboTxnMap(row map[string]any) map[string]any {
	if add, ok := row["addAsQboTxn"].(map[string]any); ok {
		return add
	}
	add := map[string]any{}
	row["addAsQboTxn"] = add
	return add
}

func addDetailsList(add map[string]any) []map[string]any {
	raw, _ := add["details"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// --- accept (Post) row shaping ---------------------------------------------

// acceptRowForPost applies the deterministic transforms the SPA applies to a
// live feed row before POSTing it to batchAcceptTransactions: acceptType is
// forced to ADD, openBalance is stringified, the bookkeeping fields
// (attachmentIds, docs, recommendationId) are added when absent, and each
// addAsQboTxn detail gains the observed taxApplicableOn marker. trackingInfo
// is populated from grounded row values only — the effective category id
// being posted, whether the user changed it, and whether it differs from the
// engine's originalCategoryId.
func acceptRowForPost(row map[string]any, effectiveCategory string, userChangedCat bool) {
	row["acceptType"] = "ADD"
	if n, ok := mapNum(row, "openBalance"); ok {
		row["openBalance"] = strconv.FormatFloat(n, 'f', -1, 64)
	}
	setIfAbsent(row, "attachmentIds", []any{})
	setIfAbsent(row, "docs", []any{})
	setIfAbsent(row, "recommendationId", "")
	add := addAsQboTxnMap(row)
	setIfAbsent(add, "attachments", []any{})
	for _, d := range addDetailsList(add) {
		setIfAbsent(d, "taxApplicableOn", "SALES")
	}
	row["trackingInfo"] = map[string]any{
		"initialCategoryId":      effectiveCategory,
		"weChangedCatForUser":    false,
		"userChangedCat":         userChangedCat,
		"userOverrodeChangedCat": false,
		"userChangedName":        false,
		"isCatChanged":           effectiveCategory != "" && effectiveCategory != mapStr(row, "originalCategoryId"),
	}
}

// rowEffectiveCategory returns the category id currently set on the row's
// addAsQboTxn detail (the value a plain Post would submit).
func rowEffectiveCategory(row map[string]any) string {
	details := addDetailsList(addAsQboTxnMap(row))
	if len(details) == 0 {
		return ""
	}
	return mapStr(details[0], "categoryId")
}

// buildAcceptBody wraps mutated feed rows in the observed
// batchAcceptTransactions envelope.
func buildAcceptBody(accountID string, rows []any) ([]byte, error) {
	body := map[string]any{
		"nextTxnInfo": map[string]any{
			"accountId":            accountID,
			"nextTransactionIndex": -1,
			"reviewState":          "PENDING",
			"sort":                 "-txnDate",
		},
		"txnList": map[string]any{"olbTxns": rows},
	}
	return json.Marshal(body)
}

// replayAccept runs the shared fetch-build-post pipeline for the
// batchAcceptTransactions family: validate via the plan, load the session,
// hydrate full feed rows, shape each row with mutate, then POST.
func replayAccept(ctx context.Context, accountID string, olbTxnIDs []string, plan *RequestPlan, mutate func(row map[string]any) error) (int, error) {
	acct := accountID
	if acct == "" {
		acct = DefaultAccountID
	}
	ac, err := newAPIClient()
	if err != nil {
		return 0, err
	}
	rows, err := feedRowsForIDs(ctx, ac, acct, normalizeExcludeIDs(olbTxnIDs))
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if err := mutate(row); err != nil {
			return 0, err
		}
	}
	olbTxns := make([]any, 0, len(rows))
	for _, row := range rows {
		olbTxns = append(olbTxns, row)
	}
	body, err := buildAcceptBody(acct, olbTxns)
	if err != nil {
		return 0, fmt.Errorf("encoding batch accept request: %w", err)
	}
	return postResolved(ctx, ac, plan, body)
}

// ReplayBatchAccept POSTs
// /api/neo/v1/company/{realm}/olb/ng/batchAcceptTransactions?acceptOnly=true.
// It accepts (posts) pending feed rows in bulk using each row's current
// category state — the same shape the SPA's Post button sends. Empty
// olbTxnIds are rejected before any network call. Callers must only pass
// olbTxnIds (or the :ofx display id for the same row).
func ReplayBatchAccept(ctx context.Context, accountID string, olbTxnIDs []string) (int, error) {
	plan, err := PlanBatchAccept(accountID, olbTxnIDs)
	if err != nil {
		return 0, err
	}
	return replayAccept(ctx, accountID, olbTxnIDs, plan, func(row map[string]any) error {
		if rowEffectiveCategory(row) == "" {
			return fmt.Errorf("%w: olbTxnId %s", ErrMissingAcceptCategory, mapStr(row, "olbTxnId"))
		}
		acceptRowForPost(row, rowEffectiveCategory(row), false)
		addAsQboTxnMap(row)["txnDate"] = mapStr(row, "olbTxnDate")
		return nil
	})
}

// --- match (acceptTransactions) shaping ------------------------------------

// findRegisterRow locates a register transaction by txnId (numeric or
// string form both match the caller's id).
func findRegisterRow(regRows []map[string]any, id string) map[string]any {
	for _, row := range regRows {
		if jsonNumberString(row["txnId"]) == id {
			return row
		}
	}
	return nil
}

// registerPaymentAmount returns the amount the register row applies: the
// payment field for money-out records, deposit for money-in, else the
// absolute amount.
func registerPaymentAmount(reg map[string]any) (float64, bool) {
	if v, ok := mapNum(reg, "payment"); ok {
		return v, true
	}
	if v, ok := mapNum(reg, "deposit"); ok {
		return v, true
	}
	if v, ok := mapNum(reg, "amount"); ok {
		return math.Abs(v), true
	}
	return 0, false
}
