package client

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"
)

// verifyAcceptReadback proves both the feed transition and the accounting
// details from independent queries. The POST's own echoed fields are not proof.
func verifyAcceptReadback(ctx context.Context, ac *apiClient, request, response []byte) error {
	var req struct {
		Next struct {
			Account string `json:"accountId"`
		} `json:"nextTxnInfo"`
		List struct {
			Rows []map[string]any `json:"olbTxns"`
		} `json:"txnList"`
	}
	var receipt struct {
		Rows []struct {
			ID    string `json:"olbTxnId"`
			Added []struct {
				ID flexibleString `json:"qboTxnId"`
			} `json:"addedQboTxns"`
		} `json:"acceptedTxns"`
	}
	if json.Unmarshal(request, &req) != nil || json.Unmarshal(response, &receipt) != nil {
		return ErrMutationUnverified
	}
	ids := []string{}
	old := map[string]map[string]any{}
	expectedIDs := map[string]map[string]bool{}
	for _, r := range req.List.Rows {
		id := mapStr(r, "olbTxnId")
		ids = append(ids, id)
		old[id] = r
	}
	for _, r := range receipt.Rows {
		expectedIDs[r.ID] = map[string]bool{}
		for _, ref := range r.Added {
			expectedIDs[r.ID][ref.ID.String()] = true
		}
	}
	after, err := stateRows(ctx, ac, req.Next.Account, "ACCEPTED", ids, true)
	if err != nil {
		return fmt.Errorf("%w: accepted feed read: %v", ErrMutationUnverified, err)
	}
	seenBooks := map[string]bool{}
	for _, row := range after {
		id := mapStr(row, "olbTxnId")
		before := old[id]
		if before == nil {
			return fmt.Errorf("%w: feed identity changed", ErrMutationUnverified)
		}
		for _, key := range []string{"amount", "olbTxnDate", "origDescription", "qboAccountId"} {
			if !reflect.DeepEqual(before[key], row[key]) {
				return fmt.Errorf("%w: feed %s changed %s", ErrMutationUnverified, id, key)
			}
		}
		refs, _ := row["matchedQboTxns"].([]any)
		if len(refs) != 1 || len(expectedIDs[id]) != 1 {
			return fmt.Errorf("%w: unsupported accounting linkage for %s", ErrMutationUnverified, id)
		}
		ref, _ := refs[0].(map[string]any)
		recordID := jsonNumberString(ref["qboTxnId"])
		entity := mapStr(ref, "txnFdmName")
		if !expectedIDs[id][recordID] {
			return fmt.Errorf("%w: feed linked a different entity", ErrMutationUnverified)
		}
		bookKey := entity + "/" + recordID
		if seenBooks[bookKey] {
			return fmt.Errorf("%w: distinct ADD rows share a ledger record", ErrMutationUnverified)
		}
		seenBooks[bookKey] = true
		record, err := readMutationEntity(ctx, ac, entity, recordID)
		if err != nil || record == nil {
			return fmt.Errorf("%w: linked accounting record unavailable", ErrMutationUnverified)
		}
		fundingKey, incoming := "AccountRef", false
		switch entity {
		case "Purchase":
			if credit, present := record["Credit"]; present && credit != nil {
				var valid bool
				incoming, valid = credit.(bool)
				if !valid {
					return fmt.Errorf("%w: unrecognized accounting direction", ErrMutationUnverified)
				}
			}
		case "CreditCardCredit":
			incoming = true
		case "Deposit":
			fundingKey = "DepositToAccountRef"
			incoming = true
		default:
			return fmt.Errorf("%w: unsupported posting entity %s", ErrMutationUnverified, entity)
		}
		funding, _ := record[fundingKey].(map[string]any)
		if jsonNumberString(funding["value"]) != req.Next.Account {
			return fmt.Errorf("%w: accounting funding account differs", ErrMutationUnverified)
		}
		flow, ok := before["amount"].(float64)
		if !ok || flow > 0 && !incoming || flow < 0 && incoming {
			return fmt.Errorf("%w: accounting direction differs", ErrMutationUnverified)
		}
		add, _ := before["addAsQboTxn"].(map[string]any)
		date := normalizeDate(mapStr(add, "txnDate"))
		if stamp, err := time.Parse(time.RFC3339Nano, date); err == nil {
			date = stamp.Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", date); err != nil || record["TxnDate"] != date {
			return fmt.Errorf("%w: accounting date differs or is unrecognized", ErrMutationUnverified)
		}
		details := sliceObjects(add["details"])
		lines := sliceObjects(record["Line"])
		actual := []map[string]any{}
		for _, line := range lines {
			for _, key := range []string{"AccountBasedExpenseLineDetail", "DepositLineDetail", "JournalEntryLineDetail"} {
				if d, ok := line[key].(map[string]any); ok {
					copy := shallowCopyMap(d)
					copy["Amount"] = line["Amount"]
					actual = append(actual, copy)
				}
			}
		}
		if len(actual) != len(details) || len(details) == 0 {
			return fmt.Errorf("%w: accounting line count differs", ErrMutationUnverified)
		}
		for i, want := range details {
			account, _ := actual[i]["AccountRef"].(map[string]any)
			klass, _ := actual[i]["ClassRef"].(map[string]any)
			if jsonNumberString(account["value"]) != jsonNumberString(want["categoryId"]) || jsonNumberString(klass["value"]) != jsonNumberString(want["klassId"]) {
				return fmt.Errorf("%w: accounting category/Class differs", ErrMutationUnverified)
			}
			if value, present := want["amount"]; present && value != nil && value != "" {
				amount, ok := readbackAmount(value)
				got, valid := actual[i]["Amount"].(float64)
				if !ok || !valid || math.Abs(got-math.Abs(amount)) > 0.000001 {
					return fmt.Errorf("%w: split amount differs", ErrMutationUnverified)
				}
			}
		}
		amount, ok := before["amount"].(float64)
		total, valid := record["TotalAmt"].(float64)
		if !ok || !valid || total < 0 || math.Abs(math.Abs(amount)-total) > 0.000001 {
			return fmt.Errorf("%w: accounting total differs", ErrMutationUnverified)
		}
		if memo, ok := add["txnMemo"].(string); ok {
			actual, valid := record["PrivateNote"].(string)
			if record["PrivateNote"] != nil && !valid || actual != memo {
				return fmt.Errorf("%w: accounting memo differs", ErrMutationUnverified)
			}
		}
	}
	pending, err := stateRows(ctx, ac, req.Next.Account, "PENDING", ids, false)
	if err != nil || len(pending) != 0 {
		return fmt.Errorf("%w: pending absence not proved", ErrMutationUnverified)
	}
	confirmMutation(ctx)
	return nil
}

func readbackAmount(value any) (float64, bool) {
	text := jsonNumberString(value)
	n, err := strconv.ParseFloat(text, 64)
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}
