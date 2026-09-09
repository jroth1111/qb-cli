package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// inventory_adj.go wires the remaining products-and-services inventory write
// surfaces onto the v3 company API.
//
// Endpoint evidence (/tmp/qbo-cap/js_extraction.json, captured 2026-08 SPA
// bundles): the QBO frontend navigates to four dedicated pages —
//
//	/app/inventory-counts         physical inventory counts
//	/app/inventory_quantity_adj   quantity adjustments
//	/app/inventory_starting_value starting value adjustments
//	/app/buildassembly            build assembly
//
// Per the ticket, each page is served by a v3 company-API path derived from
// the route segment. Only InventoryAdjustment (mutate.go) has a captured
// wire shape; the three bodies below are therefore kept minimal and their
// field layout is flagged unverified — do not extend them without a capture.
// Exported so the CLI wiring (cli/inventory.go) can reference the same
// entity names and v3 paths without duplicating string literals.
const (
	InventoryCountEntity       = "InventoryCount"
	BuildAssemblyEntity        = "BuildAssembly"
	StartingValueEntity        = "StartingValue"
	InventoryCountPath         = "inventory-counts"
	BuildAssemblyPath          = "buildassembly"
	InventoryStartingValuePath = "inventory_starting_value"
)

func invV3URL(realm, path string) string {
	return fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/%s?minorversion=73", realm, path)
}

// PlannedInventoryV3URL is the secret-free dry-run URL: it never embeds a
// live company id, matching PlannedMutateURL's {realm} convention.
func PlannedInventoryV3URL(path string) string {
	return invV3URL(realmToken, path)
}

// buildInventoryCountBody validates flags and builds a physical inventory
// count create. Requires --item and --counted-qty (the corrected physical
// count). UNVERIFIED wire shape: TxnDate/PrivateNote follow the common v3
// envelope; the Line detail mirrors ItemAdjustmentLineDetail.
func buildInventoryCountBody(flags map[string]string) (map[string]any, error) {
	item := firstFlag(flags, "item")
	if item == "" {
		return nil, fmt.Errorf("inventory count create requires --item")
	}
	qty := parseAmount(firstFlag(flags, "counted-qty", "new-qty", "qty"))
	if qty == 0 {
		return nil, fmt.Errorf("inventory count create requires --counted-qty")
	}
	out := map[string]any{
		"TxnDate": time.Now().Format("2006-01-02"),
		"Line": []map[string]any{{
			"DetailType": "InventoryCountLineDetail",
			"InventoryCountLineDetail": map[string]any{
				"ItemRef":    map[string]any{"value": item},
				"CountedQty": qty,
			},
		}},
	}
	if d := firstFlag(flags, "date"); d != "" {
		out["TxnDate"] = normalizeDate(d)
	}
	if memo := firstFlag(flags, "memo"); memo != "" {
		out["PrivateNote"] = memo
	}
	return out, nil
}

// buildBuildAssemblyBody validates flags and builds a build assembly create.
// Requires --item (the assembly to build) and --qty (units to build).
// UNVERIFIED wire shape.
func buildBuildAssemblyBody(flags map[string]string) (map[string]any, error) {
	item := firstFlag(flags, "item")
	if item == "" {
		return nil, fmt.Errorf("buildassembly create requires --item")
	}
	qty := parseAmount(firstFlag(flags, "qty"))
	if qty <= 0 {
		return nil, fmt.Errorf("buildassembly create requires --qty greater than zero")
	}
	out := map[string]any{
		"ItemRef": map[string]any{"value": item},
		"Qty":     qty,
	}
	if d := firstFlag(flags, "date"); d != "" {
		out["TxnDate"] = normalizeDate(d)
	}
	if memo := firstFlag(flags, "memo"); memo != "" {
		out["PrivateNote"] = memo
	}
	return out, nil
}

// buildStartingValueBody validates flags and builds an inventory starting
// value adjustment. Requires --item, --qty, and --cost. UNVERIFIED wire
// shape.
func buildStartingValueBody(flags map[string]string) (map[string]any, error) {
	item := firstFlag(flags, "item")
	if item == "" {
		return nil, fmt.Errorf("startingvalue create requires --item")
	}
	qty := parseAmount(firstFlag(flags, "qty"))
	cost := parseAmount(firstFlag(flags, "cost", "initial-cost"))
	if qty == 0 && cost == 0 {
		return nil, fmt.Errorf("startingvalue create requires --qty or --cost")
	}
	out := map[string]any{"ItemRef": map[string]any{"value": item}}
	if qty != 0 {
		out["Qty"] = qty
	}
	if cost != 0 {
		out["Cost"] = cost
	}
	if acct := firstFlag(flags, "asset-account", "account"); acct != "" {
		out["AssetAccountRef"] = map[string]any{"value": acct}
	}
	if d := firstFlag(flags, "date"); d != "" {
		out["TxnDate"] = normalizeDate(d)
	}
	return out, nil
}

// invPostV3 marshals body, POSTs it to the realm-scoped v3 path, applies the
// production-realm guard, and projects the response into a MutateResult. It
// is the inventory_adj analogue of ReplayMutate's shared tail.
func invPostV3(ctx context.Context, entity, path string, body map[string]any, op string) (*MutateResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := ac.postJSON(ctx, invV3URL(ac.realm, path), raw)
	if err != nil {
		return nil, fmt.Errorf("v3 %s %s: %w", op, entity, err)
	}
	defer drainAndClose(resp)
	got, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
	}
	item := projectMutateItem(entity, got)
	item.Type = entity
	return &MutateResult{Status: resp.StatusCode, Op: op, Entity: entity, Item: item}, nil
}

// ReplayInventoryCountCreate POSTs a physical inventory count to
// /api/v3/company/{realm}/inventory-counts. Route-derived path; see the
// file-header evidence note.
func ReplayInventoryCountCreate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	flags = normalizeFlags(flags)
	body, err := buildInventoryCountBody(flags)
	if err != nil {
		return nil, err
	}
	return invPostV3(ctx, InventoryCountEntity, InventoryCountPath, body, "create")
}

// ReplayBuildAssemblyCreate POSTs a build assembly to
// /api/v3/company/{realm}/buildassembly. Route-derived path.
func ReplayBuildAssemblyCreate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	flags = normalizeFlags(flags)
	body, err := buildBuildAssemblyBody(flags)
	if err != nil {
		return nil, err
	}
	return invPostV3(ctx, BuildAssemblyEntity, BuildAssemblyPath, body, "create")
}

// ReplayStartingValueCreate POSTs an inventory starting value adjustment to
// /api/v3/company/{realm}/inventory_starting_value. Route-derived path.
func ReplayStartingValueCreate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	flags = normalizeFlags(flags)
	body, err := buildStartingValueBody(flags)
	if err != nil {
		return nil, err
	}
	return invPostV3(ctx, StartingValueEntity, InventoryStartingValuePath, body, "create")
}

// normalizeFlags copies a nilable flag map so the builders below can mutate
// defaults without touching a caller's map.
func normalizeFlags(flags map[string]string) map[string]string {
	out := make(map[string]string, len(flags))
	for k, v := range flags {
		k = strings.TrimSpace(k)
		out[k] = strings.TrimSpace(v)
	}
	return out
}
