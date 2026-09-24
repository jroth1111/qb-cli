package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// v4AcctNSTestCompany2 is the v4 Relay namespace for Account nodes on Test
// Company 2 — decodes to "v4.1:9341457769756854:51ced8536c". Captured from
// the credit-card-credit form's header.account.id (differs from the
// transaction namespace 80271edd8a).
const v4AcctNSTestCompany2 = "djQuMTo5MzQxNDU3NzY5NzU2ODU0OjUxY2VkODUzNmM"

// acctNodeID renders the Relay node id for an Account. Falls back to the
// realm-encoded namespace for other companies.
func acctNodeID(ac *apiClient, acctID string) string {
	if ac.realm == v4TxNSRealm {
		return v4AcctNSTestCompany2 + ":" + acctID
	}
	return base64.RawURLEncoding.EncodeToString([]byte("v4.1:"+ac.realm+":51ced8536c")) + ":" + acctID
}

// ReplayCreditCardCredit creates a credit card credit through the v4
// transaction-form mutation — the only supported path on this build (v3
// creditcardcredit returns 500 "Operation creditcardcredit is not
// supported", verified live). Contract captured on TC2 2026-09-19:
// UpdateTransactions_Transaction_qbo with type PURCHASE,
// header.transactionType CREDIT_CARD_CREDIT, qboAppData.txnTypeId "11",
// header.account = namespaced Account node, and accountLines[] carrying
// bare account ids + ACCOUNT_client_N line ids. payment.paymentType is
// CREDIT_CARD_CREDIT.
func ReplayCreditCardCredit(ctx context.Context, ccAccount, accountAlias, date, categoryAccount, lineItems string) (*MutateResult, error) {
	ccAccount = strings.TrimSpace(ccAccount)
	accountAlias = strings.TrimSpace(accountAlias)
	if ccAccount == "" {
		ccAccount = accountAlias
	}
	if ccAccount == "" {
		return nil, fmt.Errorf("credit-card-credit create requires --credit-card-account")
	}
	date = strings.TrimSpace(date)
	if date == "" {
		return nil, fmt.Errorf("credit-card-credit create requires --date")
	}
	iso, err := billDateISO(date)
	if err != nil {
		return nil, fmt.Errorf("--date: %w", err)
	}
	lines, err := parseCCCLines(lineItems, strings.TrimSpace(categoryAccount))
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	ccID, err := reclassifyAccountID(ctx, ccAccount)
	if err != nil {
		return nil, fmt.Errorf("credit-card account: %w", err)
	}
	if accountAlias != "" && accountAlias != ccAccount {
		aliasID, aerr := reclassifyAccountID(ctx, accountAlias)
		if aerr != nil {
			return nil, fmt.Errorf("--account: %w", aerr)
		}
		if aliasID != ccID {
			return nil, fmt.Errorf("--account resolves to account %s but --credit-card-account resolves to %s: the cash side of a credit card credit is the card account itself", aliasID, ccID)
		}
	}
	acctLines := make([]any, 0, len(lines))
	total := 0.0
	for i, l := range lines {
		acctID, err := reclassifyAccountID(ctx, l.Account)
		if err != nil {
			return nil, fmt.Errorf("line %d account: %w", i+1, err)
		}
		total += l.Amount
		acctLines = append(acctLines, map[string]any{
			"description": l.Description,
			"amount":      fmt.Sprintf("%.2f", l.Amount),
			"projectId":   "",
			"id":          fmt.Sprintf("ACCOUNT_client_%d", i),
			"account":     map[string]any{"id": acctID},
			"class":       map[string]any{},
			"department":  map[string]any{},
			"traits": map[string]any{
				"deposit":             map[string]any{},
				"journal":             map[string]any{},
				"billable":            map[string]any{},
				"purchaseOrder":       map[string]any{},
				"progressivelyBilled": map[string]any{},
			},
		})
	}
	today := time.Now().Format("2006-01-02")
	amtS := fmt.Sprintf("%.2f", total)
	txn := map[string]any{
		"id":            txnDraftID(ac),
		"entityVersion": "0",
		"stageEntity":   nil,
		"meta":          map[string]any{"created": today + "T00:00:00.000+10:00", "updated": today + "T00:00:00.000+10:00"},
		"qboAppData": map[string]any{
			"txnTypeId": "11", "purchSaleLocationEnabled": false,
			"hasCustomPurchaseRefNum": false, "eligibleToReclaimTaxWithFRS": false,
			"tparExpenseEnabled": false, "hasPurchaseTax": false,
		},
		"header": map[string]any{
			"projectId":       "",
			"convenienceFee":  map[string]any{"ach": map[string]any{}},
			"transactionType": "CREDIT_CARD_CREDIT",
			"txnDate":         iso,
			"amount":          amtS,
			"currencyInfo": map[string]string{
				"symbol": "A$", "homeAmount": amtS, "code": "AUD",
				"exchangeRate": "1.00", "name": "Australian Dollar", "currency": "AUD",
			},
			"is_ict_txn":            false,
			"regionExtensions":      map[string]any{"defaultTxnSubType": map[string]any{}},
			"voided":                false,
			"account":               map[string]any{"id": acctNodeID(ac, ccID)},
			"tags":                  []any{},
			"skipInventoryTracking": false,
		},
		"type":         "PURCHASE",
		"attachments":  []any{},
		"customFields": []any{},
		"lines":        map[string]any{"itemLines": []any{}, "accountLines": acctLines},
		"traits": map[string]any{
			"costEstimate": map[string]any{},
			"esignature":   map[string]any{},
			"delivery":     map[string]any{"correspondenceAddress": map[string]any{}, "emailDeliveryInfo": map[string]any{}},
			"shipping":     map[string]any{"shipAddress": map[string]any{}, "shipFromAddress": map[string]any{}},
			"discount":     map[string]any{},
			"prePayment":   map[string]any{},
			"balance": map[string]any{
				"onlinePaymentInfo": map[string]any{"enableCCPayment": false, "enableBankPayment": false, "enableFinancing": false},
				"balance":           "0",
				"dueDate":           iso,
			},
			"invoiceDeposit": nil,
			"payment": map[string]any{
				"paymentDetailsMessage": nil,
				"paymentMethod":         map[string]any{},
				"paymentType":           "CREDIT_CARD_CREDIT",
				"processedPayment":      map[string]any{},
				"creditCardInfo":        map[string]any{},
				"checkInfo":             map[string]any{},
				"qboAppData":            map[string]any{},
			},
			"expirationTrait": map[string]any{},
			"transfer":        map[string]any{"fromAccount": map[string]any{}, "toAccount": map[string]any{}},
			"acceptStatus":    map[string]any{},
			"tax":             map[string]any{},
			"bankDeposit":     map[string]any{"cashBackAccount": map[string]any{}},
			"journal":         map[string]any{},
			"redactionTrait":  map[string]any{"lineRedactionSummary": map[string]any{}},
		},
	}
	payload, err := json.Marshal(map[string]any{
		"query": `mutation UpdateTransactions_Transaction_qbo($input_0: UpdateTransactions_TransactionInput!) {
  updateTransactions_Transaction(input: $input_0) {
    clientMutationId
    transactionsTransaction { id entityVersion }
  }
}`,
		"variables": map[string]any{"input_0": map[string]any{
			"clientMutationId":        "0",
			"transactionsTransaction": txn,
		}},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, invoicesImportGraphQLURL, payload, map[string]string{
		"Referer":      "https://qbo.intuit.com/app/creditcardcredit",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("credit-card-credit create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out struct {
		Data struct {
			Mut struct {
				Txn struct {
					ID string `json:"id"`
				} `json:"transactionsTransaction"`
			} `json:"updateTransactions_Transaction"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parsing credit-card-credit response: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("credit-card-credit create: %s", out.Errors[0].Message)
	}
	txnID := out.Data.Mut.Txn.ID
	if txnID == "" {
		return nil, fmt.Errorf("credit-card-credit create: no node id in response: %.300s", raw)
	}
	if i := strings.LastIndex(txnID, ":"); i >= 0 {
		txnID = txnID[i+1:]
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "create",
		Entity: "CreditCardCredit",
		Item:   QueryItem{Type: "CreditCardCredit", ID: txnID, Amount: total},
		Note:   fmt.Sprintf("%d account line(s)", len(lines)),
	}, nil
}

type cccLine struct {
	Account     string
	Amount      float64
	Description string
}

// parseCCCLines decodes --line-items JSON: [{account|category-account|account-id,
// amount, description?}]. Account may be a name or id; rows that omit it fall
// back to the --category-account default.
func parseCCCLines(raw, fallbackAccount string) ([]cccLine, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("credit-card-credit create requires --line-items (JSON array); omitted lines would record a $1 default")
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(raw), &rows); err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("--line-items must decode to a non-empty JSON array")
	}
	out := make([]cccLine, 0, len(rows))
	for i, row := range rows {
		acct := kvStr(row, "account", "category-account", "account-id", "category")
		if acct == "" {
			acct = fallbackAccount
		}
		if acct == "" {
			return nil, fmt.Errorf("line %d: missing account (set per-row or via --category-account)", i+1)
		}
		amtStr := kvStr(row, "amount")
		amt, err := strconv.ParseFloat(strings.TrimSpace(amtStr), 64)
		if amtStr == "" || err != nil {
			if f, ok := row["amount"].(float64); ok {
				amt = f
			} else {
				return nil, fmt.Errorf("line %d: bad amount %q", i+1, amtStr)
			}
		}
		out = append(out, cccLine{Account: acct, Amount: amt, Description: kvStr(row, "description", "desc", "memo")})
	}
	return out, nil
}
