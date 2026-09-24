package client

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
)

// ReplayExpenseRecategorise changes the category account on a posted expense
// (v3 Purchase) — the same account swap the banking UI performs when a user
// recategorises a transaction. It fetches the Purchase, rewrites
// AccountBasedExpenseLineDetail.AccountRef on the requested lines, and posts a
// sparse update. --payee-id optionally moves the payee (EntityRef). Pending
// bank-feed rows categorise via `feed txn update categorise`, not here.
func ReplayExpenseRecategorise(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	id := strings.TrimSpace(firstFlag(flags, "id"))
	if id == "" {
		return nil, fmt.Errorf("expense recategorise requires --id (v3 Purchase id)")
	}
	catID := strings.TrimSpace(firstFlag(flags, "category-id", "category"))
	classID := strings.TrimSpace(firstFlag(flags, "class-id"))
	payee := strings.TrimSpace(firstFlag(flags, "payee-id", "payee"))
	lineID := strings.TrimSpace(firstFlag(flags, "line-id"))
	if catID == "" && classID == "" && payee == "" {
		return nil, fmt.Errorf("recategorise requires --category-id, --class-id, or --payee-id")
	}
	if classID != "" && lineID == "" {
		return nil, fmt.Errorf("--class-id requires --line-id to avoid changing other expense lines")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	existing, err := fetchV3(ctx, ac, "purchase", id)
	if err != nil {
		return nil, fmt.Errorf("purchase %s fetch: %w", id, err)
	}
	if existing["Id"] != id {
		return nil, fmt.Errorf("purchase readback did not match the requested id")
	}
	if expected := strings.TrimSpace(flags["expected-sync-token"]); expected != "" && fmt.Sprint(existing["SyncToken"]) != expected {
		return nil, fmt.Errorf("purchase sync token changed; inspect live state before retrying")
	}
	if err := checkExpectedExpenseLine(existing, lineID, flags); err != nil {
		return nil, err
	}
	body, err := buildRecategoriseBodyWithClass(existing, catID, classID, payee, lineID)
	if err != nil {
		return nil, fmt.Errorf("purchase %s: %w", id, err)
	}
	if flags["repair-credit-card-type"] == "true" {
		ref, _ := existing["AccountRef"].(map[string]any)
		accountID, _ := ref["value"].(string)
		if accountID == "" {
			return nil, fmt.Errorf("payment-type repair requires an existing funding account")
		}
		account, err := fetchV3(ctx, ac, "account", accountID)
		if err != nil {
			return nil, fmt.Errorf("funding account verification: %w", err)
		}
		if err := repairCreditCardType(existing, body, account); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/purchase?minorversion=73", ac.realm)
	evidence, err := startMutationEvidence("recategorise", existing, raw)
	if err != nil {
		return nil, err
	}
	submittingMutation(ctx)
	resp, err := ac.postJSON(ctx, u, raw)
	if err != nil {
		return nil, fmt.Errorf("v3 purchase recategorise: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	got, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
	}
	item := projectMutateItem("Purchase", got)
	if item.ID != id {
		return nil, fmt.Errorf("recategorise returned no matching Purchase receipt; inspect live state before retrying")
	}
	if err := verifyV3Readback(ctx, ac, "Purchase", "update", id, existing, body, evidence); err != nil {
		return nil, err
	}
	confirmMutation(ctx)
	return &MutateResult{Status: resp.StatusCode, Op: "recategorise", Entity: "Purchase", Item: item, Verified: true, Evidence: evidence}, nil
}

// Changing type can be irreversible through the same API; callers must opt in.
// A credit-card funding account is checked independently of the stored type.
func repairCreditCardType(existing, body, account map[string]any) error {
	ref, _ := existing["AccountRef"].(map[string]any)
	fundingID, _ := ref["value"].(string)
	if fundingID == "" || existing["PaymentType"] != "Check" || account["AccountType"] != "Credit Card" || fundingID != account["Id"] {
		return fmt.Errorf("payment-type repair requires a Check funded by the verified Credit Card account")
	}
	body["PaymentType"] = "CreditCard"
	return nil
}

// buildRecategoriseBody produces the sparse Purchase update. Line account
// refs are swapped in place; a missing/empty line list or a line-id that
// matches nothing is rejected before any update is sent.
func buildRecategoriseBody(existing map[string]any, catID, payee, lineID string) (map[string]any, error) {
	return buildRecategoriseBodyWithClass(existing, catID, "", payee, lineID)
}

func buildRecategoriseBodyWithClass(existing map[string]any, catID, classID, payee, lineID string) (map[string]any, error) {
	body := map[string]any{
		"Id":        existing["Id"],
		"SyncToken": existing["SyncToken"],
		"sparse":    true,
	}
	// v3 validates required Purchase fields even on sparse updates — carry
	// them through unchanged.
	// Carry an existing memo explicitly: the live service can re-mask bank
	// identifiers in an omitted PrivateNote while processing line changes.
	for _, k := range []string{"PaymentType", "AccountRef", "EntityRef", "TxnDate", "CurrencyRef", "ExchangeRate", "PrivateNote"} {
		if v, ok := existing[k]; ok {
			body[k] = v
		}
	}
	if catID == "" && classID == "" {
		return body, applyPayee(body, existing, payee)
	}
	lines, ok := existing["Line"].([]any)
	if !ok || len(lines) == 0 {
		return nil, fmt.Errorf("expense has no lines to recategorise")
	}
	updated := make([]any, 0, len(lines))
	touched := 0
	for _, value := range lines {
		line, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid expense line")
		}
		if lineID != "" && fmt.Sprint(line["Id"]) != lineID {
			updated = append(updated, line)
			continue
		}
		detail, ok := line["AccountBasedExpenseLineDetail"].(map[string]any)
		if !ok {
			if lineID != "" {
				updated = append(updated, line)
				continue
			}
			return nil, fmt.Errorf("expense line is not account-based (item lines keep their own accounts)")
		}
		detail = shallowCopyMap(detail)
		if catID != "" {
			detail["AccountRef"] = map[string]any{"value": catID}
		}
		if classID != "" {
			if strings.EqualFold(classID, "none") {
				delete(detail, "ClassRef")
			} else {
				detail["ClassRef"] = map[string]any{"value": classID}
			}
		}
		line = shallowCopyMap(line)
		line["AccountBasedExpenseLineDetail"] = detail
		updated = append(updated, line)
		touched++
	}
	if touched == 0 {
		return nil, fmt.Errorf("--line-id %s matches no account-based line", lineID)
	}
	body["Line"] = updated
	return body, applyPayee(body, existing, payee)
}

func checkExpectedExpenseLine(existing map[string]any, lineID string, flags map[string]string) error {
	account, hasAccount := flags["expected-category-id"]
	class, hasClass := flags["expected-class-id"]
	if !hasAccount && !hasClass {
		return nil
	}
	if lineID == "" {
		return fmt.Errorf("expected line values require --line-id")
	}
	lines, _ := existing["Line"].([]any)
	for _, value := range lines {
		line, _ := value.(map[string]any)
		if fmt.Sprint(line["Id"]) != lineID {
			continue
		}
		detail, ok := line["AccountBasedExpenseLineDetail"].(map[string]any)
		if !ok {
			return fmt.Errorf("line %s is not account-based", lineID)
		}
		if hasAccount && expenseRefValue(detail["AccountRef"]) != account {
			return fmt.Errorf("line %s category changed; inspect live state before retrying", lineID)
		}
		if hasClass {
			if class == "none" {
				class = ""
			}
			if expenseRefValue(detail["ClassRef"]) != class {
				return fmt.Errorf("line %s class changed; inspect live state before retrying", lineID)
			}
		}
		return nil
	}
	return fmt.Errorf("line %s not found", lineID)
}

func expenseRefValue(value any) string {
	ref, _ := value.(map[string]any)
	if ref == nil || ref["value"] == nil {
		return ""
	}
	return fmt.Sprint(ref["value"])
}

func applyPayee(body, existing map[string]any, payee string) error {
	if payee == "" {
		return nil
	}
	ref, ok := existing["EntityRef"].(map[string]any)
	if !ok {
		return fmt.Errorf("expense has no payee (EntityRef) to move")
	}
	entityType, _ := ref["type"].(string)
	body["EntityRef"] = map[string]any{"value": payee, "type": entityType}
	return nil
}

func shallowCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}
