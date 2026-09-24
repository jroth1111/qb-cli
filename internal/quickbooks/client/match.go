package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// ReplayMatch POSTs /api/neo/v1/company/{realm}/olb/ng/acceptTransactions.
// It matches pending feed rows to existing QuickBooks records: each feed row
// collapses to the observed entry shape (acceptType MATCH, id, olbTxnDate,
// qboAccountId) and each --match-id is resolved against the feed account's
// advancedMatchDetails so txnTypeId, qboTxnSeqId, txnSyncToken and paymentAmount all
// come from a live read. Empty olbTxnIds or an empty match list are
// rejected before any network call; a match id absent from the register
// fails with ErrMatchTargetNotFound before the POST is sent.
func ReplayMatch(ctx context.Context, accountID string, olbTxnIDs []string, matchTxns []MatchTxn) (int, error) {
	_, err := PlanMatch(accountID, olbTxnIDs, matchTxns)
	if err != nil {
		return 0, err
	}
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
	regRows, err := registerRowsForAccount(ctx, ac, acct)
	if err != nil {
		return 0, err
	}
	for _, target := range cleanMatchTxns(matchTxns) {
		if findRegisterRow(regRows, target.TxnID) == nil {
			return 0, fmt.Errorf("%w: %s missing from before-state register", ErrMatchTargetNotFound, target.TxnID)
		}
	}
	body, err := buildLiveMatchBody(ctx, ac, acct, rows, cleanMatchTxns(matchTxns))
	if err != nil {
		return 0, err
	}
	evidenceDir, err := startMatchEvidence(acct, rows, regRows, cleanMatchTxns(matchTxns), body)
	if err != nil {
		return 0, fmt.Errorf("cannot preserve match before-state: %w", err)
	}
	// Preserve the browser banking Accept header; forcing application/json
	// changes content negotiation on this endpoint.
	resp, err := ac.post(ctx, ac.neoFeedURL()+"/acceptTransactions", body)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrMatchVerification, err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%w: response read failed", ErrMatchVerification)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "response.json"), raw, 0600); err != nil {
		return resp.StatusCode, fmt.Errorf("%w: cannot preserve response in %s", ErrMatchVerification, evidenceDir)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	if err = verifyMatchResponse(raw, acct, rows, cleanMatchTxns(matchTxns)); err != nil {
		return resp.StatusCode, fmt.Errorf("%w (evidence: %s)", err, evidenceDir)
	}
	if err = verifyMatchReadback(ctx, ac, acct, rows, regRows, cleanMatchTxns(matchTxns)); err != nil {
		return resp.StatusCode, fmt.Errorf("%w (evidence: %s)", err, evidenceDir)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "verified.json"), []byte(`{"verified":true,"accepted_rows_checked":true,"register_fields_checked":true}`), 0600); err != nil {
		return resp.StatusCode, fmt.Errorf("%w: cannot persist verification", ErrMatchVerification)
	}
	return resp.StatusCode, nil
}
