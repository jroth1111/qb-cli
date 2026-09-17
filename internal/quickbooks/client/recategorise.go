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
	payee := strings.TrimSpace(firstFlag(flags, "payee-id", "payee"))
	lineID := strings.TrimSpace(firstFlag(flags, "line-id"))
	if catID == "" && payee == "" {
		return nil, fmt.Errorf("recategorise requires --category-id or --payee-id")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	existing, err := fetchV3(ctx, ac, "purchase", id)
	if err != nil {
		return nil, fmt.Errorf("purchase %s fetch: %w", id, err)
	}
	body, err := buildRecategoriseBody(existing, catID, payee, lineID)
	if err != nil {
		return nil, fmt.Errorf("purchase %s: %w", id, err)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/purchase?minorversion=73", ac.realm)
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
	return &MutateResult{Status: resp.StatusCode, Op: "recategorise", Entity: "Purchase", Item: projectMutateItem("Purchase", got)}, nil
}

// buildRecategoriseBody produces the sparse Purchase update. Line account
// refs are swapped in place; a missing/empty line list or a line-id that
// matches nothing is rejected before any update is sent.
func buildRecategoriseBody(existing map[string]any, catID, payee, lineID string) (map[string]any, error) {
	body := map[string]any{
		"Id":        existing["Id"],
		"SyncToken": existing["SyncToken"],
		"sparse":    true,
	}
	if catID == "" {
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
		detail["AccountRef"] = map[string]any{"value": catID}
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
