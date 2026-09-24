package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ReplayFormDelete deletes a transaction through the v4 form-delete mutation —
// deleteSavedTransaction__transactions_experiences_qbo, which is
// updateTransactions_Transaction with transactionsTransaction{id, type,
// deleted:true, header:{closeBooksPassword:"*"}}. Captured on TC2 from the
// expense form's More → Delete flow (Purchase 184).
//
// The mutation echoes deleted:true even for nonexistent node ids, so the node
// is read first: a missing node fails fast, and the type field from the read
// (validated against wantType) fills the delete payload — v4 type names do not
// always match v3 entity names (v3 Bill is PURCHASE_BILL in v4).
func ReplayFormDelete(ctx context.Context, txnID, wantType string) (*MutateResult, error) {
	txnID = strings.TrimSpace(txnID)
	if txnID == "" {
		return nil, fmt.Errorf("form delete requires --id")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	node, err := fetchTxnNode(ctx, ac, txnID)
	if err != nil {
		return nil, err
	}
	txnType := str(node, "type")
	if txnType == "" {
		return nil, fmt.Errorf("transaction %s: v4 node has no type", txnID)
	}
	if wantType != "" && txnType != wantType {
		return nil, fmt.Errorf("transaction %s is %s, not %s", txnID, txnType, wantType)
	}
	payload, err := json.Marshal(map[string]any{
		"query": `mutation deleteSavedTransaction__transactions_experiences_qbo($input_0: UpdateTransactions_TransactionInput!) {
  updateTransactions_Transaction(input: $input_0) {
    clientMutationId
    transactionsTransaction {
      id
      type
      deleted
      traits { payment { paymentType } }
      __typename
    }
  }
}`,
		"variables": map[string]any{"input_0": map[string]any{
			"clientMutationId": "0",
			"transactionsTransaction": map[string]any{
				"id":      txnNodeID(ac, txnID),
				"type":    txnType,
				"deleted": true,
				"header":  map[string]any{"closeBooksPassword": "*"},
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSONExtra(ctx, invoicesImportGraphQLURL, payload, map[string]string{
		"Referer":      "https://qbo.intuit.com/app/expense",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("form delete: %w", err)
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
					ID      string `json:"id"`
					Type    string `json:"type"`
					Deleted *bool  `json:"deleted"`
				} `json:"transactionsTransaction"`
			} `json:"updateTransactions_Transaction"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parsing form-delete response: %w", err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("form delete: %s", out.Errors[0].Message)
	}
	txn := out.Data.Mut.Txn
	if txn.Deleted == nil || !*txn.Deleted {
		return nil, fmt.Errorf("form delete: server did not confirm deletion: %.300s", raw)
	}
	id := txn.ID
	if i := strings.LastIndex(id, ":"); i >= 0 {
		id = id[i+1:]
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "delete",
		Entity: entityLabelForTxnType(txnType),
		Item:   QueryItem{Type: entityLabelForTxnType(txnType), ID: id},
	}, nil
}

// entityLabelForTxnType maps the v4 transaction type to the entity label used
// in MutateResult/printed output.
func entityLabelForTxnType(t string) string {
	switch t {
	case "PURCHASE":
		return "Purchase"
	case "PURCHASE_BILL":
		return "Bill"
	case "SALE_INVOICE":
		return "Invoice"
	default:
		return t
	}
}
