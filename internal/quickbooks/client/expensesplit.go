package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// EXPENSE_SPLIT replaces a Purchase's lines with N account-based splits via
// the v3 sparse-update contract — the same mutation the expenses UI's
// "split" produces (lines are rewritten; TotalAmt is recomputed server-side).
// Splits must sum to the original TotalAmt exactly.
//
//	--splits: JSON array [{"category-account":"Office expenses","amount":"10.00","gst-code":"GST"}]
//	category-account resolves by v3 name/id; gst-code resolves by TaxCode name/id
//	(optional — omitted keeps the line untaxed).
func ReplayExpenseSplit(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	id := strings.TrimSpace(firstFlag(flags, "id"))
	if id == "" {
		return nil, fmt.Errorf("expense split requires --id (v3 Purchase id)")
	}
	raw := strings.TrimSpace(firstFlag(flags, "splits"))
	if raw == "" {
		return nil, fmt.Errorf("expense split requires --splits '[{\"category-account\",\"amount\"}]'")
	}
	var spec []map[string]any
	if err := json.Unmarshal([]byte(raw), &spec); err != nil || len(spec) == 0 {
		return nil, fmt.Errorf("--splits must be a non-empty JSON array of {category-account, amount, gst-code?}")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	existing, err := fetchV3(ctx, ac, "purchase", id)
	if err != nil {
		return nil, fmt.Errorf("purchase %s fetch: %w", id, err)
	}
	origLines, ok := existing["Line"].([]any)
	if !ok || len(origLines) == 0 {
		return nil, fmt.Errorf("purchase %s has no lines to split", id)
	}
	origTotal := totalAmount(existing)

	var sum float64
	newLines := make([]any, 0, len(spec))
	for i, row := range spec {
		cat := kvStr(row, "category-account", "account", "account-id", "category")
		if cat == "" {
			return nil, fmt.Errorf("split %d: missing category-account", i+1)
		}
		amtStr := kvStr(row, "amount")
		amt, err := strconv.ParseFloat(strings.TrimSpace(amtStr), 64)
		if amtStr == "" || err != nil {
			return nil, fmt.Errorf("split %d: bad amount %q", i+1, amtStr)
		}
		acctID, err := reclassifyAccountID(ctx, cat)
		if err != nil {
			return nil, fmt.Errorf("split %d: %w", i+1, err)
		}
		detail := map[string]any{"AccountRef": map[string]any{"value": acctID}}
		if gst := kvStr(row, "gst-code", "tax-code", "taxcode"); gst != "" {
			taxID, err := resolveTaxCodeID(ctx, gst)
			if err != nil {
				return nil, fmt.Errorf("split %d: %w", i+1, err)
			}
			detail["TaxCodeRef"] = map[string]any{"value": taxID}
		}
		newLines = append(newLines, map[string]any{
			"Amount":                        amt,
			"DetailType":                    "AccountBasedExpenseLineDetail",
			"AccountBasedExpenseLineDetail": detail,
		})
		sum += amt
	}
	if origTotal != 0 && !nearEqual(sum, origTotal) {
		return nil, fmt.Errorf("splits sum %.2f does not equal purchase total %.2f", sum, origTotal)
	}
	body := map[string]any{
		"Id":        existing["Id"],
		"SyncToken": existing["SyncToken"],
		"sparse":    true,
		"Line":      newLines,
	}
	// v3 validates required Purchase fields even on sparse updates — carry
	// them through unchanged (same set as recategorise).
	for _, k := range []string{"PaymentType", "AccountRef", "EntityRef", "TxnDate", "CurrencyRef", "ExchangeRate"} {
		if v, ok := existing[k]; ok {
			body[k] = v
		}
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/purchase?minorversion=73", ac.realm)
	resp, err := ac.postJSON(ctx, u, rawBody)
	if err != nil {
		return nil, fmt.Errorf("v3 purchase split: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	got, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
	}
	res := &MutateResult{Status: resp.StatusCode, Op: "expense split", Entity: "Purchase", Item: projectMutateItem("Purchase", got)}
	res.Note = fmt.Sprintf("%d split lines", len(newLines))
	return res, nil
}

// totalAmount reads TotalAmt (or sums Line.Amount) from a fetched Purchase.
func totalAmount(existing map[string]any) float64 {
	if v, ok := existing["TotalAmt"].(float64); ok {
		return v
	}
	var sum float64
	if lines, ok := existing["Line"].([]any); ok {
		for _, l := range lines {
			if m, ok := l.(map[string]any); ok {
				if a, ok := m["Amount"].(float64); ok {
					sum += a
				}
			}
		}
	}
	return sum
}

// nearEqual compares money amounts with a half-cent tolerance.
func nearEqual(a, b float64) bool {
	d := a - b
	return d < 0.005 && d > -0.005
}

// kvStr returns the first non-empty string value among the given keys.
func kvStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return strings.TrimSpace(t)
				}
			case float64:
				return strconv.FormatFloat(t, 'f', -1, 64)
			}
		}
	}
	return ""
}

// resolveTaxCodeID resolves a GST/tax code name or numeric id to its v3
// TaxCode Id via the standard query service.
func resolveTaxCodeID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty tax code")
	}
	if _, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return ref, nil
	}
	res, err := ReplayQuery(ctx, "TaxCode", "", ref, 10)
	if err != nil {
		return "", err
	}
	for _, it := range res.Items {
		if strings.EqualFold(it.Name, ref) {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("no tax code named %q", ref)
}
