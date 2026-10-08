package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const MaxMatchMappings = 25

// MatchMapping assigns one downloaded occurrence to one existing ledger entry.
// Expected values are optional fail-closed preconditions, not fields to edit.
type MatchMapping struct {
	OlbTxnID           string   `json:"olb_txn_id"`
	MatchID            string   `json:"match_id"`
	ExpectedAmount     *float64 `json:"expected_amount,omitempty"`
	ExpectedDate       string   `json:"expected_date,omitempty"`
	ExpectedLedgerDate string   `json:"expected_ledger_date,omitempty"`
	ExpectedCategoryID string   `json:"expected_category_id,omitempty"`
	ExpectedClassID    string   `json:"expected_class_id,omitempty"`
}

type MappedMatchResult struct {
	Status   int    `json:"status"`
	Matched  int    `json:"matched"`
	Verified bool   `json:"verified"`
	Evidence string `json:"evidence"`
}

type mappedMatchSnapshot struct {
	RealmID   string           `json:"realm_id"`
	AccountID string           `json:"account_id"`
	Mappings  []MatchMapping   `json:"mappings"`
	Rows      []map[string]any `json:"feed_rows"`
	Registers []map[string]any `json:"register_rows"`
}

func LoadMatchMappings(path string) ([]MatchMapping, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	var mappings []MatchMapping
	if err := d.Decode(&mappings); err != nil {
		return nil, fmt.Errorf("invalid match mapping file: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("match mapping file must contain one JSON array")
	}
	return mappings, validateMatchMappings(mappings)
}

func validateMatchMappings(mappings []MatchMapping) error {
	if len(mappings) == 0 || len(mappings) > MaxMatchMappings {
		return fmt.Errorf("match mappings require 1 to %d rows", MaxMatchMappings)
	}
	feeds, books := map[string]bool{}, map[string]bool{}
	for _, m := range mappings {
		if !mutationEntityID.MatchString(m.OlbTxnID) || !mutationEntityID.MatchString(m.MatchID) || feeds[m.OlbTxnID] || books[m.MatchID] {
			return fmt.Errorf("match mappings require unique numeric string feed and ledger IDs")
		}
		feeds[m.OlbTxnID], books[m.MatchID] = true, true
		if m.ExpectedAmount != nil && (math.IsNaN(*m.ExpectedAmount) || math.IsInf(*m.ExpectedAmount, 0)) {
			return fmt.Errorf("expected amount must be finite")
		}
		if m.ExpectedDate != "" {
			if _, err := time.Parse(time.DateOnly, m.ExpectedDate); err != nil {
				return fmt.Errorf("expected date must be YYYY-MM-DD")
			}
		}
		if m.ExpectedLedgerDate != "" {
			if m.ExpectedDate == "" {
				return fmt.Errorf("expected ledger date requires an explicit expected feed date")
			}
			if _, err := time.Parse(time.DateOnly, m.ExpectedLedgerDate); err != nil {
				return fmt.Errorf("expected ledger date must be YYYY-MM-DD")
			}
		}
	}
	return nil
}

func PlanMappedMatch(account string, mappings []MatchMapping) (*RequestPlan, error) {
	if err := validateMatchMappings(mappings); err != nil {
		return nil, err
	}
	if account == "" {
		account = DefaultAccountID
	}
	entries := make([]any, 0, len(mappings))
	for _, m := range mappings {
		entries = append(entries, map[string]any{"acceptType": "MATCH", "id": m.OlbTxnID + ":ofx", "qboAccountId": account, "selectedMatches": map[string]any{"matchedTxns": []any{map[string]any{"qboTxnId": m.MatchID}}, "addAdjQboTxn": nil, "addAsQboTxn": nil}})
	}
	body, err := json.Marshal(map[string]any{"olbTxns": entries})
	if err != nil {
		return nil, err
	}
	return newPOSTPlan(acceptPath, body, "acceptTransactions", "one-to-one mappings; live eligibility/version fields hydrate before submission; not sent"), nil
}

func mappedMatchTargets(rows []map[string]any, mappings []MatchMapping) (map[string][]MatchTxn, error) {
	if err := validateMatchMappings(mappings); err != nil {
		return nil, err
	}
	if len(rows) != len(mappings) {
		return nil, fmt.Errorf("match feed population differs from mapping count")
	}
	out := map[string][]MatchTxn{}
	for i, m := range mappings {
		id := mapStr(rows[i], "id")
		if id == "" || out[id] != nil || mapStr(rows[i], "olbTxnId") != m.OlbTxnID {
			return nil, fmt.Errorf("match feed identity differs from mapping")
		}
		out[id] = []MatchTxn{{TxnID: m.MatchID}}
	}
	return out, nil
}

func uniqueMatchRegister(rows []map[string]any, id string) (map[string]any, error) {
	var found map[string]any
	for _, row := range rows {
		if jsonNumberString(row["txnId"]) == id {
			if found != nil {
				return nil, fmt.Errorf("ambiguous ledger entry %s", id)
			}
			found = row
		}
	}
	if found == nil {
		return nil, fmt.Errorf("%w: %s", ErrMatchTargetNotFound, id)
	}
	return found, nil
}

func checkMappedMatch(m MatchMapping, feed, book map[string]any, account string) error {
	flow, ok := mapNum(feed, "amount")
	amount, valid := mapNum(book, "amount")
	if !ok || !valid || flow == 0 || flow != amount || jsonNumberString(book["lineAccountId"]) != account {
		return fmt.Errorf("mapping %s has different ledger amount/direction/funding", m.OlbTxnID)
	}
	if jsonNumberString(book["txnTypeId"]) == "" || jsonNumberString(book["sequence"]) == "" {
		return fmt.Errorf("mapping %s lacks ledger type/sequence", m.OlbTxnID)
	}
	if state := jsonNumberString(book["clearState"]); state != "1" && state != "2" {
		return fmt.Errorf("%w: mapping %s has unknown or reconciled register state", ErrReconciliationUnsafe, m.OlbTxnID)
	}
	date, err := time.Parse(time.RFC3339, mapStr(feed, "olbTxnDate"))
	if err != nil || m.ExpectedDate != "" && date.Format(time.DateOnly) != m.ExpectedDate || m.ExpectedAmount != nil && flow != *m.ExpectedAmount {
		return fmt.Errorf("mapping %s failed feed date/amount precondition", m.OlbTxnID)
	}
	if m.ExpectedDate != "" {
		// A settlement can arrive after the original ledger posting. Pin both
		// dates explicitly rather than dropping the feed-date precondition.
		expectedBookDate := m.ExpectedDate
		if m.ExpectedLedgerDate != "" {
			expectedBookDate = m.ExpectedLedgerDate
		}
		bookDate, err := time.Parse(time.RFC3339, mapStr(book, "date"))
		if err != nil || bookDate.Format(time.DateOnly) != expectedBookDate {
			return fmt.Errorf("mapping %s failed ledger date precondition", m.OlbTxnID)
		}
	}
	klass := jsonNumberString(book["klassId"])
	if klass == "" {
		klass = "none"
	}
	if m.ExpectedCategoryID != "" && jsonNumberString(book["accountId"]) != m.ExpectedCategoryID || m.ExpectedClassID != "" && klass != m.ExpectedClassID {
		return fmt.Errorf("mapping %s failed ledger category/Class precondition", m.OlbTxnID)
	}
	return nil
}

// ReplayMappedMatch submits once. Shared censuses do not relax per-row identity,
// eligibility, version, accounting preservation, or pending-absence checks.
func ReplayMappedMatch(ctx context.Context, account string, mappings []MatchMapping) (*MappedMatchResult, error) {
	if _, err := PlanMappedMatch(account, mappings); err != nil {
		return nil, err
	}
	if account == "" {
		account = DefaultAccountID
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(mappings))
	for i, m := range mappings {
		ids[i] = m.OlbTxnID
	}
	rows, err := stateRows(ctx, ac, account, "PENDING", ids, true)
	if err != nil {
		return nil, err
	}
	targets, err := mappedMatchTargets(rows, mappings)
	if err != nil {
		return nil, err
	}
	registers, err := registerRowsForAccount(ctx, ac, account)
	if err != nil {
		return nil, err
	}
	if err := mappedTargetsUnclaimed(ctx, ac, account, mappings); err != nil {
		return nil, err
	}
	selected := make([]map[string]any, 0, len(mappings))
	entries := []json.RawMessage{}
	for i, m := range mappings {
		book, err := uniqueMatchRegister(registers, m.MatchID)
		if err != nil {
			return nil, err
		}
		if err = checkMappedMatch(m, rows[i], book, account); err != nil {
			return nil, err
		}
		selected = append(selected, book)
		body, err := buildLiveMatchBodyWithLedgerDate(ctx, ac, account, rows[i:i+1], targets[mapStr(rows[i], "id")], m.ExpectedLedgerDate)
		if err != nil {
			return nil, err
		}
		var one struct {
			Rows []json.RawMessage `json:"olbTxns"`
		}
		if err := json.Unmarshal(body, &one); err != nil || len(one.Rows) != 1 {
			return nil, fmt.Errorf("match builder returned an unexpected row")
		}
		var entry map[string]any
		if err := json.Unmarshal(one.Rows[0], &entry); err != nil {
			return nil, err
		}
		selection, _ := entry["selectedMatches"].(map[string]any)
		links := sliceObjects(selection["matchedTxns"])
		if len(links) != 1 || jsonNumberString(links[0]["qboTxnId"]) != m.MatchID || jsonNumberString(links[0]["txnTypeId"]) != jsonNumberString(book["txnTypeId"]) || jsonNumberString(links[0]["qboTxnSeqId"]) != jsonNumberString(book["sequence"]) {
			return nil, fmt.Errorf("mapping %s eligibility identity/type/sequence changed", m.OlbTxnID)
		}
		payment, err := strconv.ParseFloat(jsonNumberString(links[0]["paymentAmount"]), 64)
		flow, _ := mapNum(rows[i], "amount")
		if err != nil || payment != math.Abs(flow) {
			return nil, fmt.Errorf("mapping %s eligibility amount differs from complete feed amount", m.OlbTxnID)
		}
		entries = append(entries, one.Rows[0])
	}
	body, err := json.Marshal(map[string]any{"olbTxns": entries})
	if err != nil {
		return nil, err
	}
	snapshot := mappedMatchSnapshot{RealmID: ac.realm, AccountID: account, Mappings: mappings, Rows: rows, Registers: selected}
	dir, err := startMutationEvidence("match-mapped", snapshot, body)
	if err != nil {
		return nil, err
	}
	result := &MappedMatchResult{Matched: len(rows), Evidence: dir}
	submittingMutation(ctx)
	resp, err := ac.post(ctx, ac.neoFeedURL()+"/acceptTransactions", body)
	if err != nil {
		return result, fmt.Errorf("%w: %v (evidence: %s)", ErrMatchVerification, err, dir)
	}
	result.Status = resp.StatusCode
	raw, readErr := readBody(resp)
	_ = drainAndClose(resp)
	if readErr != nil {
		return result, fmt.Errorf("%w: %v (evidence: %s)", ErrMatchVerification, readErr, dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "response.json"), raw, 0600); err != nil {
		return result, fmt.Errorf("%w: %v", ErrMatchVerification, err)
	}
	if result.Status != http.StatusOK {
		return result, fmt.Errorf("%w: HTTP %d: %s (evidence: %s)", ErrMatchVerification, result.Status, errorMessage(raw), dir)
	}
	if err := verifyMatchResponseMapped(raw, account, rows, targets); err != nil {
		return result, fmt.Errorf("%w (evidence: %s)", err, dir)
	}
	if err := verifyMatchReadbackMapped(ctx, ac, account, rows, selected, targets); err != nil {
		return result, fmt.Errorf("%w (evidence: %s)", err, dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "verified.json"), []byte(`{"verified":true,"one_to_one_links_checked":true,"register_fields_checked":true}`), 0600); err != nil {
		return result, fmt.Errorf("%w: %v", ErrMatchVerification, err)
	}
	result.Verified = true
	return result, nil
}

func mappedTargetsUnclaimed(ctx context.Context, ac *apiClient, account string, mappings []MatchMapping) error {
	wanted := map[string]bool{}
	for _, m := range mappings {
		wanted[m.MatchID] = true
	}
	seen := map[string]bool{}
	start := 0
	size := ac.censusSize
	for range 100 {
		url := fmt.Sprintf("%s/getTransactions?sort=-txnDate&reviewState=ACCEPTED&ignoreMatching=false&accountId=%s&startIndex=%d&chunkSize=%d", ac.neoFeedURL(), account, start, size)
		resp, err := ac.get(ctx, url, fmt.Sprintf("items=%d-%d", start, start+size-1))
		if err != nil {
			return err
		}
		raw, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("accepted allocation read HTTP %d", resp.StatusCode)
		}
		var env map[string]json.RawMessage
		if json.Unmarshal(raw, &env) != nil || len(env["items"]) == 0 || string(env["items"]) == "null" {
			return fmt.Errorf("accepted allocation read missing items")
		}
		var rows []map[string]any
		if err := json.Unmarshal(env["items"], &rows); err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		fresh := 0
		for _, row := range rows {
			id := mapStr(row, "id")
			if id == "" || jsonNumberString(row["qboAccountId"]) != account {
				return fmt.Errorf("accepted allocation identity/account missing")
			}
			if !seen[id] {
				seen[id] = true
				fresh++
			}
			for _, link := range sliceObjects(row["matchedQboTxns"]) {
				if wanted[jsonNumberString(link["qboTxnId"])] {
					return fmt.Errorf("ledger target already allocated to accepted feed row %s", id)
				}
			}
		}
		if fresh == 0 {
			return fmt.Errorf("accepted allocation pagination repeated")
		}
		start += len(rows)
	}
	return fmt.Errorf("accepted allocation census incomplete")
}

// VerifyMappedMatchEvidence performs reads only, including after a lost response.
func VerifyMappedMatchEvidence(ctx context.Context, dir string) (*MappedMatchResult, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "before.json"))
	if err != nil {
		return nil, err
	}
	var snapshot mappedMatchSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	targets, err := mappedMatchTargets(snapshot.Rows, snapshot.Mappings)
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if snapshot.RealmID != ac.realm || !mutationEntityID.MatchString(snapshot.AccountID) {
		return nil, fmt.Errorf("match evidence belongs to a different or invalid company/account")
	}
	for _, m := range snapshot.Mappings {
		if _, err := uniqueMatchRegister(snapshot.Registers, m.MatchID); err != nil {
			return nil, err
		}
	}
	if err := verifyMatchReadbackMapped(ctx, ac, snapshot.AccountID, snapshot.Rows, snapshot.Registers, targets); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "recovered-verified.json"), []byte(`{"verified":true,"recovery_reads_only":true}`), 0600); err != nil {
		return nil, err
	}
	return &MappedMatchResult{Status: 200, Matched: len(snapshot.Rows), Verified: true, Evidence: dir}, nil
}
