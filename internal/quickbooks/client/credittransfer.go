package client

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CUSTOMER_CREDIT_TRANSFER — "customers customer run". QBO has no
// transfer-credit endpoint; the AU doc the catalog note cites (L4vT5bz8K)
// prescribes the journal-entry workaround. The same JournalEntryLineDetail
// entity mechanism proven by CHEQUE_BOUNCE carries it: a JE that debits A/R
// on the source customer (consuming their credit balance) and credits A/R on
// the destination customer (giving them the credit). Entity Type must be
// title-case "Customer" — "CUSTOMER" is a live 2010 "unsupported property".
//
//	POST /api/v3/company/{realm}/journalentry
//	Line1: Debit  A/R × amount, Entity {EntityRef: from-customer, Type: Customer}
//	Line2: Credit A/R × amount, Entity {EntityRef: to-customer,   Type: Customer}
//
// --from/--to accept customer display names or v3 ids; TxnDate is today (the
// catalog declares no --date param).
func ReplayCreditTransfer(ctx context.Context, from, to, amountS string) (*MutateResult, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" {
		return nil, fmt.Errorf("credit transfer requires --from (customer holding the credit)")
	}
	if to == "" {
		return nil, fmt.Errorf("credit transfer requires --to (customer receiving the credit)")
	}
	if strings.EqualFold(from, to) {
		return nil, fmt.Errorf("--from and --to are the same customer %q — nothing to transfer", from)
	}
	amt, err := strconv.ParseFloat(strings.TrimSpace(amountS), 64)
	if err != nil || amt <= 0 {
		return nil, fmt.Errorf("--amount must be a positive amount, got %q", amountS)
	}
	fromID, err := resolveNameID(ctx, "Customer", from)
	if err != nil {
		return nil, fmt.Errorf("--from: %w", err)
	}
	toID, err := resolveNameID(ctx, "Customer", to)
	if err != nil {
		return nil, fmt.Errorf("--to: %w", err)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	arID, err := bounceAccountByType(ctx, ac, "Accounts Receivable")
	if err != nil {
		return nil, fmt.Errorf("resolving A/R account: %w", err)
	}
	desc := fmt.Sprintf("Credit transfer %s → %s", from, to)
	leg := func(posting, custID string) map[string]any {
		return map[string]any{
			"DetailType":  "JournalEntryLineDetail",
			"Amount":      amt,
			"Description": desc,
			"JournalEntryLineDetail": map[string]any{
				"PostingType": posting,
				"AccountRef":  map[string]any{"value": arID},
				"Entity": map[string]any{
					"EntityRef": map[string]any{"value": custID},
					"Type":      "Customer",
				},
			},
		}
	}
	jeID, err := createJournalEntry(ctx, ac, time.Now().Format("2006-01-02"),
		[]map[string]any{leg("Debit", fromID), leg("Credit", toID)})
	if err != nil {
		return nil, fmt.Errorf("credit transfer: %w", err)
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "transfer",
		Entity: "JournalEntry",
		Item:   QueryItem{Type: "JournalEntry", ID: jeID, Amount: amt},
		Note:   fmt.Sprintf("credit moved %s→%s", from, to),
	}, nil
}
