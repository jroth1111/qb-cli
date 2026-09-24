package client

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Fixed-asset mutations ride assetservice.api.intuit.com/v1/graphql — captured
// live on Test Company 2 (2026-09-17): createAsset, updateAsset, deleteAsset
// all posted through the Fixed Assets UI and replayed server-side. The model
// envelope is Finance_AssetCreateInput / Finance_AssetUpdateInput; delete takes
// the assetId as a bare String variable.

const financeCreateAssetDoc = `mutation createAsset($model: Finance_AssetCreateInput!) {
  financeCreateAsset(assetInput: $model) {
    asset {
      assetId name description assetNumber associatedIds purchaseDate disposalDate status lifecycleStatus createdAt
      purchaseValue { currency value }
      netBookValue { currency value }
      salvageValue { currency value }
      depreciationInfos { method assetId depreciationType accumulatedDepreciation { currency value } initialPriorDepreciation { currency value } depreciationStartDate lifeYear depreciationRate }
      __typename
    }
    __typename
  }
}`

const financeUpdateAssetDoc = `mutation updateAsset($model: Finance_AssetUpdateInput!, $clientTimezone: String) {
  financeUpdateAsset(asset: $model, clientTimezone: $clientTimezone) {
    asset {
      assetId name description assetNumber associatedIds purchaseDate disposalDate status lifecycleStatus
      purchaseValue { currency value }
      netBookValue { currency value }
      salvageValue { currency value }
      saleValue { currency value }
      disposalFeeValue { currency value }
      postDisposalProfit { currency value }
      depreciationInfos { method assetId depreciationType accumulatedDepreciation { currency value } initialPriorDepreciation { currency value } depreciationStartDate lifeYear depreciationRate }
      __typename
    }
    __typename
  }
}`

const financeDeleteAssetDoc = `mutation deleteAsset($model: String!) {
  financeDeleteAsset(assetId: $model) {
    assetId
    __typename
  }
}`

// assetModel assembles the Finance_AssetCreateInput/UpdateInput shape the UI
// posts. currency defaults to AUD (the TC2 company currency) — callers can
// override with --currency. The associatedIds map links the COA accounts the
// asset posts against.
func assetModel(flags map[string]string, existing map[string]any) (map[string]any, error) {
	name := strings.TrimSpace(flags["name"])
	if ex, _ := existing["name"].(string); name == "" && ex != "" {
		name = ex
	}
	if name == "" {
		return nil, fmt.Errorf("fixed-asset create/update requires --name")
	}
	currency := strings.ToUpper(strings.TrimSpace(flags["currency"]))
	if currency == "" {
		currency = "AUD"
	}
	money := func(flag string, fallback float64) map[string]any {
		v := fallback
		if s := strings.TrimSpace(flags[flag]); s != "" {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				v = f
			}
		}
		return map[string]any{"currency": currency, "value": v}
	}
	purchaseDate := strings.TrimSpace(flags["purchase-date"])
	if purchaseDate == "" {
		if s, _ := existing["purchaseDate"].(string); s != "" {
			purchaseDate = s
		} else {
			purchaseDate = time.Now().Format("2006-01-02")
		}
	}
	price := 0.0
	costStr := strings.TrimSpace(flags["cost"])
	if costStr == "" {
		costStr = strings.TrimSpace(flags["price"])
	}
	if costStr != "" {
		f, err := strconv.ParseFloat(costStr, 64)
		if err != nil {
			return nil, fmt.Errorf("--cost must be a number: %v", err)
		}
		price = f
	} else if m, _ := existing["purchaseValue"].(map[string]any); m != nil {
		price, _ = m["value"].(float64)
	} else {
		return nil, fmt.Errorf("fixed-asset create requires --cost")
	}
	life := 0
	lifeStr := strings.TrimSpace(flags["useful-life"])
	if lifeStr == "" {
		lifeStr = strings.TrimSpace(flags["life-years"])
	}
	if lifeStr != "" {
		n, err := strconv.Atoi(lifeStr)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("--useful-life must be a positive integer")
		}
		life = n
	}
	method := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(flags["method"]), "-", "_"))
	if method == "" {
		method = "STRAIGHT_LINE"
	}
	assoc := map[string]any{"offering_id": "qbo"}
	if m, _ := existing["associatedIds"].(map[string]any); m != nil {
		maps.Copy(assoc, m)
	}
	if v := strings.TrimSpace(flags["asset-account"]); v != "" {
		assoc["coa_asset"] = v
		assoc["coa_asset_depreciation"] = v
	}
	if v := strings.TrimSpace(flags["dep-expense-account"]); v != "" {
		assoc["coa_depreciation_expense"] = v
	}
	if assoc["coa_asset"] == nil || assoc["coa_depreciation_expense"] == nil {
		return nil, fmt.Errorf("fixed-asset create requires --asset-account and --dep-expense-account (COA ids; see accounting coa search)")
	}
	dep := map[string]any{
		"automaticDepreciation":    true,
		"method":                   method,
		"schedulePeriod":           "MONTHLY",
		"accumulatedDepreciation":  map[string]any{"currency": currency, "value": 0},
		"depreciationStartDate":    purchaseDate,
		"initialPriorDepreciation": map[string]any{"currency": currency, "value": 0},
		"depreciationType":         "BOOK",
	}
	if life > 0 {
		dep["lifeYear"] = life
	} else if l, _ := existingLifeYear(existing); l > 0 {
		dep["lifeYear"] = l
	} else {
		return nil, fmt.Errorf("fixed-asset create requires --useful-life")
	}
	model := map[string]any{
		"assetId":           "",
		"name":              name,
		"description":       strings.TrimSpace(flags["description"]),
		"associatedIds":     assoc,
		"purchaseDate":      purchaseDate,
		"purchaseValue":     map[string]any{"currency": currency, "value": price},
		"netBookValue":      map[string]any{"currency": currency, "value": price},
		"salvageValue":      money("salvage", 0),
		"depreciationInfos": []any{dep},
		"customExtensions":  map[string]any{"dimensions": []any{}},
	}
	return model, nil
}

func existingLifeYear(existing map[string]any) (float64, bool) {
	infos, _ := existing["depreciationInfos"].([]any)
	for _, d := range infos {
		if m, ok := d.(map[string]any); ok {
			if l, ok := m["lifeYear"].(float64); ok {
				return l, true
			}
		}
	}
	return 0, false
}

// ReplayFixedAssetMutate runs createAsset / updateAsset / deleteAsset on the
// asset service. update reads the live financeAssets row first so the model
// keeps every server-side field the UI sends (status, associatedIds, …).
func ReplayFixedAssetMutate(ctx context.Context, op string, flags map[string]string) (*MutateResult, error) {
	op = strings.ToLower(strings.TrimSpace(op))
	id := strings.TrimSpace(flags["id"])
	if op != "create" && op != "update" && op != "delete" {
		return nil, fmt.Errorf("unsupported fixed-asset op %q", op)
	}
	if op != "create" && id == "" {
		return nil, fmt.Errorf("fixed-asset %s requires --id <assetId> (see accounting fixed-asset search)", op)
	}
	var doc, opName, field string
	vars := map[string]any{}
	switch op {
	case "create":
		doc, opName, field = financeCreateAssetDoc, "createAsset", "financeCreateAsset"
		model, err := assetModel(flags, nil)
		if err != nil {
			return nil, err
		}
		vars["model"] = model
	case "update":
		doc, opName, field = financeUpdateAssetDoc, "updateAsset", "financeUpdateAsset"
		existing, err := fetchFinanceAsset(ctx, id)
		if err != nil {
			return nil, err
		}
		model, err := assetModel(flags, existing)
		if err != nil {
			return nil, err
		}
		model["assetId"] = id
		if s, _ := existing["status"].(string); s != "" {
			model["status"] = s
		}
		vars["model"] = model
		vars["clientTimezone"] = time.Local.String()
	case "delete":
		doc, opName, field = financeDeleteAssetDoc, "deleteAsset", "financeDeleteAsset"
		vars["model"] = id
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": opName,
		"variables":     vars,
		"query":         doc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, assetServiceGraphQLURL, assetServiceHost, payload)
	if err != nil {
		return nil, fmt.Errorf("assetservice %s: %w", opName, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading assetservice %s: %w", opName, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data   map[string]json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("assetservice %s: malformed response: %w", opName, err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("assetservice %s: %s", opName, wrap.Errors[0].Message)
	}
	d, ok := wrap.Data[field]
	if !ok || len(d) == 0 || string(d) == "null" {
		return nil, fmt.Errorf("assetservice %s outcome unknown: missing %s receipt; inspect before retrying", opName, field)
	}
	item := QueryItem{Type: "FixedAsset"}
	{
		var out struct {
			Asset struct {
				AssetID string `json:"assetId"`
				Name    string `json:"name"`
				Status  string `json:"status"`
			} `json:"asset"`
			AssetID string `json:"assetId"`
		}
		if err := json.Unmarshal(d, &out); err != nil {
			return nil, fmt.Errorf("assetservice %s: malformed receipt: %w", opName, err)
		}
		if out.Asset.AssetID != "" {
			item.ID, item.Name = out.Asset.AssetID, out.Asset.Name
		} else {
			item.ID = out.AssetID
		}
	}
	if item.ID == "" {
		return nil, fmt.Errorf("assetservice %s outcome unknown: receipt has no asset id; inspect before retrying", opName)
	}
	if op != "create" && item.ID != id {
		return nil, fmt.Errorf("assetservice %s outcome unknown: receipt asset id %q does not match requested id %q; inspect before retrying", opName, item.ID, id)
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     op,
		Entity: "FixedAsset",
		Item:   item,
		Note:   "assetservice " + opName,
	}, nil
}

// fetchFinanceAsset returns the live financeAssets node for assetId (update
// needs the full server-side model).
func fetchFinanceAsset(ctx context.Context, assetID string) (map[string]any, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": "financeAssets",
		"variables": map[string]any{
			"first":              50,
			"offset":             0,
			"limit":              50,
			"financeAssetFilter": map[string]any{"associatedIDFilter": []string{}},
		},
		"query": financeAssetsDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, assetServiceGraphQLURL, assetServiceHost, payload)
	if err != nil {
		return nil, fmt.Errorf("assetservice financeAssets: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading assetservice financeAssets: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data struct {
			FinanceAssets struct {
				Edges []struct {
					Node map[string]any `json:"node"`
				} `json:"edges"`
			} `json:"financeAssets"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("assetservice financeAssets: malformed response")
	}
	if len(doc.Errors) > 0 && doc.Errors[0].Message != "" {
		return nil, fmt.Errorf("assetservice financeAssets: %s", doc.Errors[0].Message)
	}
	for _, e := range doc.Data.FinanceAssets.Edges {
		if s, _ := e.Node["assetId"].(string); s == assetID {
			return e.Node, nil
		}
	}
	return nil, fmt.Errorf("fixed asset %s not found", assetID)
}

// PlannedFixedAssetMutateURL reports the mutation host for dry-run plans.
func PlannedFixedAssetMutateURL() string { return assetServiceGraphQLURL }
