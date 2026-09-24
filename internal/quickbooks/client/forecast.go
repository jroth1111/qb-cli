package client

// REPORTS.FORECAST_CREATE / FORECAST_EDIT — Advanced forecast CRUD on
// planningforecasting.api.intuit.com/graphql, reverse-engineered from the
// forecasting-ui bundle and live-proven on TC2 2026-09-19 (create
// 3432826404261164992 → rename v0→v1 → delete status:true; list back to 0).
//
// createBusinessForecast validates incrementally: name → forecastMode →
// forecastType → intervalType → dates, then fails PNF002 INTERNAL until
// secondaryListType and forecastSetupData are present. The wire enum values
// differ from the UI labels: forecastMode AUTO_SMART|SMART_INTEL|
// MANUAL_FROM_SMART|MANUAL, forecastType PROFIT_AND_LOSS|THREE_WAY|BALANCE_SHEET,
// setupDetails.measure AVG|RAW|SMART (UI names AVG_ACTUALS|RAW_ACTUALS|
// AI_FORECAST map to them). updateBusinessForecast requires the complete
// input including forecastId+version — no sparse patch. deleteBusinessForecasts
// takes forecastIds:[ID!]! and returns per-id {status,description}.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const forecastGQLURL = "https://planningforecasting.api.intuit.com/graphql"
const forecastGQLHost = "planningforecasting.api.intuit.com"

// PlannedForecastURL is the dry-run URL for forecast mutations.
func PlannedForecastURL() string { return forecastGQLURL }

var forecastModes = map[string]bool{
	"AUTO_SMART": true, "SMART_INTEL": true, "MANUAL_FROM_SMART": true, "MANUAL": true,
}
var forecastTypes = map[string]bool{
	"PROFIT_AND_LOSS": true, "THREE_WAY": true, "BALANCE_SHEET": true,
}
var forecastIntervals = map[string]bool{
	"MONTHLY": true, "QUARTERLY": true, "ANNUALLY": true,
}
var forecastMeasures = map[string]string{
	"raw": "RAW", "avg": "AVG", "smart": "SMART",
}

// forecastNode is the BusinessForecast projection the mutations return and
// the read query lists.
type forecastNode struct {
	ForecastID   string `json:"forecastId"`
	Version      int    `json:"version"`
	Name         string `json:"name"`
	ForecastMode string `json:"forecastMode"`
	ForecastType string `json:"forecastType"`
	IntervalType string `json:"intervalType"`
	StartDate    string `json:"startDate"`
	EndDate      string `json:"endDate"`
}

// forecastGQL posts one forecasting document and returns the data envelope.
func forecastGQL(ctx context.Context, ac *apiClient, body []byte) (json.RawMessage, error) {
	resp, err := ac.doURIHost(ctx, http.MethodPost, forecastGQLURL, forecastGQLHost, body)
	if err != nil {
		return nil, fmt.Errorf("forecast mutation: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("malformed forecast response %.200s", raw)
	}
	if len(env.Errors) > 0 {
		return nil, fmt.Errorf("%s", env.Errors[0].Message)
	}
	return env.Data, nil
}

// forecastFetch loads one forecast's full input fields for an update resend.
func forecastFetch(ctx context.Context, ac *apiClient, id string) (map[string]any, error) {
	doc := `query ($first: Int) {
	  getAllBusinessForecasts(first: $first) {
	    edges { node {
	      forecastId version name forecastMode forecastType intervalType
	      secondaryListType startDate endDate
	      viewSettings { secondaryId dimensionDefinitionId }
	      forecastSetupData { base setupDetails { period measure budgetId } }
	    } }
	  }
	}`
	body, _ := json.Marshal(map[string]any{
		"query":     doc,
		"variables": map[string]any{"first": 100},
	})
	data, err := forecastGQL(ctx, ac, body)
	if err != nil {
		return nil, err
	}
	var out struct {
		GetAllBusinessForecasts struct {
			Edges []struct {
				Node map[string]any `json:"node"`
			} `json:"edges"`
		} `json:"getAllBusinessForecasts"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("malformed forecast list %.200s", data)
	}
	for _, e := range out.GetAllBusinessForecasts.Edges {
		if fmt.Sprint(e.Node["forecastId"]) == id {
			return e.Node, nil
		}
	}
	return nil, fmt.Errorf("forecast %s not found", id)
}

// forecastInput assembles the BusinessForecastInput object the mutations
// require — every field, no sparse form.
func forecastInput(node map[string]any) map[string]any {
	in := map[string]any{
		"forecastId":        node["forecastId"],
		"version":           node["version"],
		"name":              node["name"],
		"forecastMode":      node["forecastMode"],
		"forecastType":      node["forecastType"],
		"intervalType":      node["intervalType"],
		"startDate":         node["startDate"],
		"endDate":           node["endDate"],
		"secondaryListType": node["secondaryListType"],
		"viewSettings":      node["viewSettings"],
		"forecastSetupData": node["forecastSetupData"],
	}
	if in["viewSettings"] == nil {
		in["viewSettings"] = map[string]any{}
	}
	if in["secondaryListType"] == nil {
		in["secondaryListType"] = "NOT_SUBDIVIDED"
	}
	return in
}

// ReplayForecastMutate creates, updates, or deletes an Advanced forecast.
//
//	create --name X --start 2026-10-01 --end 2027-09-30
//	  [--mode MANUAL] [--type PROFIT_AND_LOSS] [--interval MONTHLY] [--measure raw]
//	update --id 3432826404261164992 --name X   (fetches + resends full input)
//	delete --id 3432826404261164992
func ReplayForecastMutate(ctx context.Context, op, id, name, mode, ftype, interval, start, end, measure string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	switch op {
	case "create":
		if name == "" {
			return nil, fmt.Errorf("forecast create requires --name")
		}
		if mode == "" {
			mode = "MANUAL"
		}
		if ftype == "" {
			ftype = "PROFIT_AND_LOSS"
		}
		if interval == "" {
			interval = "MONTHLY"
		}
		mode = strings.ToUpper(mode)
		ftype = strings.ToUpper(ftype)
		interval = strings.ToUpper(interval)
		if !forecastModes[mode] {
			return nil, fmt.Errorf("invalid --mode %q — AUTO_SMART, SMART_INTEL, MANUAL_FROM_SMART or MANUAL", mode)
		}
		if !forecastTypes[ftype] {
			return nil, fmt.Errorf("invalid --type %q — PROFIT_AND_LOSS, THREE_WAY or BALANCE_SHEET", ftype)
		}
		if !forecastIntervals[interval] {
			return nil, fmt.Errorf("invalid --interval %q — MONTHLY, QUARTERLY or ANNUALLY", interval)
		}
		if measure != "" {
			if _, ok := forecastMeasures[strings.ToLower(strings.TrimSpace(measure))]; !ok {
				return nil, fmt.Errorf("invalid --measure %q — raw, avg or smart", measure)
			}
		}
		if _, err := time.Parse("2006-01-02", start); err != nil {
			return nil, fmt.Errorf("forecast create requires --start yyyy-mm-dd (%v)", err)
		}
		if _, err := time.Parse("2006-01-02", end); err != nil {
			return nil, fmt.Errorf("forecast create requires --end yyyy-mm-dd (%v)", err)
		}
	case "update":
		if id == "" {
			return nil, fmt.Errorf("forecast update requires --id")
		}
		if name == "" {
			return nil, fmt.Errorf("forecast update requires --name (the only field proven mutable)")
		}
	case "delete":
		if id == "" {
			return nil, fmt.Errorf("forecast delete requires --id")
		}
	default:
		return nil, fmt.Errorf("unknown forecast op %q", op)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	switch op {
	case "create":
		mk := forecastMeasures[strings.ToLower(strings.TrimSpace(measure))]
		if mk == "" {
			mk = "RAW"
		}
		vars := map[string]any{
			"businessForecastInput": map[string]any{
				"name":              name,
				"forecastMode":      strings.ToUpper(mode),
				"forecastType":      strings.ToUpper(ftype),
				"intervalType":      strings.ToUpper(interval),
				"startDate":         start,
				"endDate":           end,
				"secondaryListType": "NOT_SUBDIVIDED",
				"viewSettings":      map[string]any{},
				"forecastSetupData": map[string]any{
					"base":         "ACTUAL",
					"setupDetails": map[string]any{"measure": mk},
				},
			},
		}
		doc := `mutation createBusinessForecast($businessForecastInput: BusinessForecastInput!) {
		  createBusinessForecast(forecastInput: $businessForecastInput) {
		    forecastId version name forecastMode forecastType intervalType startDate endDate
		  }
		}`
		body, _ := json.Marshal(map[string]any{"query": doc, "variables": vars})
		data, err := forecastGQL(ctx, ac, body)
		if err != nil {
			return nil, err
		}
		var out struct {
			Create *forecastNode `json:"createBusinessForecast"`
		}
		_ = json.Unmarshal(data, &out)
		return forecastResult(out.Create, "create"), nil
	case "update":
		node, err := forecastFetch(ctx, ac, id)
		if err != nil {
			return nil, err
		}
		in := forecastInput(node)
		in["name"] = name
		doc := `mutation updateBusinessForecast($forecastInput: BusinessForecastInput!) {
		  updateBusinessForecast(forecastInput: $forecastInput) {
		    forecastId version name forecastMode forecastType intervalType startDate endDate
		  }
		}`
		body, _ := json.Marshal(map[string]any{
			"query":     doc,
			"variables": map[string]any{"forecastInput": in},
		})
		data, err := forecastGQL(ctx, ac, body)
		if err != nil {
			return nil, err
		}
		var out struct {
			Update *forecastNode `json:"updateBusinessForecast"`
		}
		_ = json.Unmarshal(data, &out)
		return forecastResult(out.Update, "update"), nil
	case "delete":
		doc := `mutation deleteBusinessForecastsOperation($forecastIds: [ID!]!) {
		  deleteBusinessForecasts(forecastIds: $forecastIds) {
		    forecastId status description
		  }
		}`
		body, _ := json.Marshal(map[string]any{
			"query":     doc,
			"variables": map[string]any{"forecastIds": []string{id}},
		})
		data, err := forecastGQL(ctx, ac, body)
		if err != nil {
			return nil, err
		}
		var out struct {
			Delete []struct {
				ForecastID  string `json:"forecastId"`
				Status      bool   `json:"status"`
				Description string `json:"description"`
			} `json:"deleteBusinessForecasts"`
		}
		_ = json.Unmarshal(data, &out)
		res := &MutateResult{Status: http.StatusOK, Op: "delete", Entity: "BusinessForecast",
			Item: QueryItem{Type: "BusinessForecast", ID: id}}
		if len(out.Delete) > 0 {
			res.Item.ID = out.Delete[0].ForecastID
			res.Item.Name = out.Delete[0].Description
			if !out.Delete[0].Status {
				return res, fmt.Errorf("delete reported status=false: %s", out.Delete[0].Description)
			}
		}
		res.Note = "planningforecasting delete"
		return res, nil
	}
	return nil, fmt.Errorf("unknown forecast op %q", op)
}

func forecastResult(n *forecastNode, op string) *MutateResult {
	item := QueryItem{Type: "BusinessForecast"}
	if n != nil {
		item.ID = n.ForecastID
		item.Name = n.Name
		if n.ForecastMode != "" {
			item.Type = "BusinessForecast (" + n.ForecastMode + ")"
		}
	}
	return &MutateResult{Status: http.StatusOK, Op: op, Entity: "BusinessForecast",
		Item: item, Note: "planningforecasting " + op}
}
