package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Partial receipt of a purchase order rides the proven warehouse
// CommerceReceiveInventory mutation: a movement against the PO's lines with
// quantity < ordered leaves the remaining quantity open server-side.
// Proven live on Test Company 2 (2026-09-16) for full receipts; the partial
// path sends the same input contract with reduced quantities.
const receiveInventoryMutation = `mutation CommerceReceiveInventory($input: Commerce_ReceiveInventoryInput!) {
  commerceReceiveInventory(input: $input) {
    __typename
    ... on Commerce_InventoryMovement {
      id
      movementType
      version
      sourceTransaction { txnId txnType version }
      lines { inventoryMovementLineId sourceTransactionLineId lineOrder itemId quantity }
    }
    ... on Commerce_InventoryOperationError { message }
  }
}`

// poPartialItem is one --items entry: which PO line and how much is being
// received now. line-id matches the v3 PO line Id; item-id matches the
// line's ItemRef. Exactly one of the two must identify a PO line.
type poPartialItem struct {
	LineID string  `json:"line-id"`
	ItemID string  `json:"item-id"`
	Qty    float64 `json:"qty"`
}

// ReplayPurchaseOrderPartial loads PO --id via v3, resolves --items
// (JSON array of {line-id|item-id, qty}) against its item lines, and posts
// CommerceReceiveInventory for the partial quantities.
func ReplayPurchaseOrderPartial(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	poID := strings.TrimSpace(flags["id"])
	if poID == "" {
		poID = strings.TrimSpace(flags["purchase-order"])
	}
	if poID == "" {
		return nil, fmt.Errorf("purchase-order partial requires --id (the PO to receive against)")
	}
	raw := strings.TrimSpace(flags["items"])
	if raw == "" {
		return nil, fmt.Errorf("purchase-order partial requires --items (JSON array of {line-id|item-id, qty})")
	}
	var spec []poPartialItem
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, fmt.Errorf("--items must decode to a JSON array: %w", err)
	}
	if len(spec) == 0 {
		return nil, fmt.Errorf("--items must contain at least one line to receive")
	}
	for i, want := range spec {
		if want.Qty <= 0 {
			return nil, fmt.Errorf("--items[%d]: qty must be positive", i)
		}
		if want.LineID == "" && want.ItemID == "" {
			return nil, fmt.Errorf("--items[%d]: one of line-id or item-id is required", i)
		}
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	po, err := fetchV3(ctx, ac, "purchaseorder", poID)
	if err != nil {
		return nil, fmt.Errorf("purchase-order %s: %w", poID, err)
	}
	version, err := syncTokenInt(po["SyncToken"])
	if err != nil {
		return nil, fmt.Errorf("purchase-order %s: %w", poID, err)
	}
	lines, err := poPartialLines(po, spec)
	if err != nil {
		return nil, fmt.Errorf("purchase-order %s: %w", poID, err)
	}
	src := map[string]any{"txnId": poID, "txnType": "PURCHASE_ORDER", "version": version}
	if d, ok := po["TxnDate"].(string); ok && d != "" {
		src["txnDate"] = d
	}
	if v, ok := po["VendorRef"].(map[string]any); ok {
		src["vendorId"] = fmt.Sprint(v["value"])
	}
	input := map[string]any{"sourceTransaction": src, "lines": lines}
	payload, err := json.Marshal(map[string]any{
		"operationName": "CommerceReceiveInventory",
		"variables":     map[string]any{"input": input},
		"query":         receiveInventoryMutation,
	})
	if err != nil {
		return nil, err
	}
	return receiveInventoryMutate(ctx, payload)
}

// syncTokenInt converts a v3 SyncToken (string or number) to the Int the
// warehouse sourceTransaction.version declares.
func syncTokenInt(v any) (int, error) {
	switch t := v.(type) {
	case float64:
		return int(t), nil
	case string:
		var n int
		if _, err := fmt.Sscanf(t, "%d", &n); err != nil {
			return 0, fmt.Errorf("unparseable SyncToken %q", t)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("missing SyncToken")
	}
}

// poPartialLines maps requested items onto the PO's item lines. Quantity is
// sent as requested — the warehouse service enforces the open balance.
func poPartialLines(po map[string]any, spec []poPartialItem) ([]any, error) {
	rawLines, _ := po["Line"].([]any)
	out := make([]any, 0, len(spec))
	for i, want := range spec {
		matched := false
		for order, rl := range rawLines {
			lm, _ := rl.(map[string]any)
			if lm["DetailType"] != "ItemBasedExpenseLineDetail" {
				continue
			}
			detail, _ := lm["ItemBasedExpenseLineDetail"].(map[string]any)
			itemRef, _ := detail["ItemRef"].(map[string]any)
			lineID := fmt.Sprint(lm["Id"])
			itemID := fmt.Sprint(itemRef["value"])
			if (want.LineID != "" && want.LineID != lineID) ||
				(want.ItemID != "" && want.ItemID != itemID) ||
				(want.LineID == "" && want.ItemID == "") {
				continue
			}
			out = append(out, map[string]any{
				"itemId":                  itemID,
				"quantity":                want.Qty,
				"sourceTransactionLineId": lineID,
				"lineOrder":               order + 1,
			})
			matched = true
			break
		}
		if !matched {
			return nil, fmt.Errorf("--items[%d] matches no item line on the PO", i)
		}
	}
	return out, nil
}

// receiveInventoryMutate posts CommerceReceiveInventory and projects the
// returned movement into a MutateResult.
func receiveInventoryMutate(ctx context.Context, payload []byte) (*MutateResult, error) {
	raw, note, err := postWarehouseOp(ctx, payload, "CommerceReceiveInventory")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data struct {
			Result json.RawMessage `json:"commerceReceiveInventory"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("parsing CommerceReceiveInventory: %w", err)
	}
	var mv struct {
		ID           string `json:"id"`
		MovementType string `json:"movementType"`
		Version      int    `json:"version"`
		Message      string `json:"message"`
		Typename     string `json:"__typename"`
	}
	if err := json.Unmarshal(wrap.Data.Result, &mv); err != nil {
		return nil, fmt.Errorf("parsing CommerceReceiveInventory payload: %w", err)
	}
	if strings.HasSuffix(mv.Typename, "Error") && mv.Message != "" {
		return nil, fmt.Errorf("CommerceReceiveInventory: %s", mv.Message)
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "partial",
		Entity: "InventoryMovement",
		Note:   note,
		Item:   QueryItem{ID: mv.ID, Type: mv.MovementType},
	}, nil
}
