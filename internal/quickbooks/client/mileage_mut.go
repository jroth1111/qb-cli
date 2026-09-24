package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Trip mutations ride the same trips GraphQL surface as the reads:
// trips.api.intuit.com/v4/graphql. Captured live on Test Company 2
// (2026-09-17): the Add-trip form posts updateTrips_Trip with a Trips_TripInput
// that omits id for create; update sends id plus changed fields; delete is a
// soft delete (id + deleted:true). All three replayed server-side via
// doURIHost — create/delete verified end-to-end on a real trip.

const tripsUpdateDoc = `mutation updateTrip($tripsTrip: Trips_TripInput!, $clientMutationId: String!) {
    updateTrips_Trip(input: {clientMutationId: $clientMutationId, tripsTrip: $tripsTrip}) {
        tripsTrip {
            id description deleted distance { value unit } reviewState autoTracked
            startDateTime { dateTime } endDateTime { dateTime } vehicle { id }
        }
    }
}`

const tripsVehiclesDoc = `query {
    company { vehicles { edges { node { id primary description active deleted } } } }
}`

// ReplayTripMutate runs updateTrips_Trip for create / update / delete.
func ReplayTripMutate(ctx context.Context, op string, flags map[string]string) (*MutateResult, error) {
	op = strings.ToLower(strings.TrimSpace(op))
	if op != "create" && op != "update" && op != "delete" {
		return nil, fmt.Errorf("unsupported mileage op %q", op)
	}
	id := strings.TrimSpace(flags["id"])
	if op != "create" && id == "" {
		return nil, fmt.Errorf("mileage %s requires --id <trip node id> (see expenses mileage search)", op)
	}
	var trip map[string]any
	switch op {
	case "create":
		m, err := tripInput(ctx, flags, nil)
		if err != nil {
			return nil, err
		}
		trip = m
	case "update":
		existing, err := fetchTripNode(ctx, id)
		if err != nil {
			return nil, err
		}
		m, err := tripInput(ctx, flags, existing)
		if err != nil {
			return nil, err
		}
		m["id"] = id
		trip = m
	case "delete":
		trip = map[string]any{"id": id, "deleted": true}
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "updateTrip",
		"variables": map[string]any{
			"clientMutationId": "qb-cli",
			"tripsTrip":        trip,
		},
		"query": tripsUpdateDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, tripsGraphQLURL, tripsHost, payload)
	if err != nil {
		return nil, fmt.Errorf("trips updateTrip %s: %w", op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading trips updateTrip: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			Update struct {
				Trip *struct {
					ID          string `json:"id"`
					Description string `json:"description"`
					Deleted     *bool  `json:"deleted"`
					ReviewState string `json:"reviewState"`
				} `json:"tripsTrip"`
			} `json:"updateTrips_Trip"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("trips updateTrip %s: malformed response: %w", op, err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("trips updateTrip %s: %s", op, wrap.Errors[0].Message)
	}
	tripReceipt := wrap.Data.Update.Trip
	if tripReceipt == nil || strings.TrimSpace(tripReceipt.ID) == "" || tripReceipt.Deleted == nil {
		return nil, fmt.Errorf("trips updateTrip %s outcome unknown: missing tripsTrip receipt; inspect before retrying", op)
	}
	if op != "create" && tripReceipt.ID != id {
		return nil, fmt.Errorf("trips updateTrip %s outcome unknown: receipt id %q does not match requested id %q; inspect before retrying", op, tripReceipt.ID, id)
	}
	if (op == "delete") != *tripReceipt.Deleted {
		return nil, fmt.Errorf("trips updateTrip %s outcome unknown: receipt deleted=%v; inspect before retrying", op, *tripReceipt.Deleted)
	}
	item := QueryItem{Type: "Trip", ID: tripReceipt.ID, Name: tripReceipt.Description}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     op,
		Entity: "Trip",
		Item:   item,
		Note:   "trips updateTrips_Trip",
	}, nil
}

// tripInput assembles the Trips_TripInput shape. On update, unset flags keep
// the live node's values so a rename doesn't clear addresses or distance.
func tripInput(ctx context.Context, flags map[string]string, existing map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if existing != nil {
		for _, k := range []string{
			"reviewState", "description", "distance", "startDateTime", "endDateTime",
			"startAddress", "endAddress", "vehicle", "autoTracked", "notes",
		} {
			if v, ok := existing[k]; ok {
				out[k] = v
			}
		}
	}
	review := strings.ToUpper(strings.TrimSpace(flags["trip-type"]))
	if review != "" {
		switch review {
		case "BUSINESS", "PERSONAL":
			out["reviewState"] = review
		default:
			return nil, fmt.Errorf("--trip-type must be business or personal")
		}
	} else if existing == nil {
		out["reviewState"] = "BUSINESS"
	}
	if s := strings.TrimSpace(flags["purpose"]); s != "" {
		out["description"] = s
	} else if s := strings.TrimSpace(flags["description"]); s != "" {
		out["description"] = s
	}
	date := strings.TrimSpace(flags["date"])
	if date == "" && existing == nil {
		return nil, fmt.Errorf("mileage create requires --date dd/MM/yyyy")
	}
	if date != "" {
		t, err := time.ParseInLocation("02/01/2006", date, time.Local)
		if err != nil {
			if t2, err2 := time.ParseInLocation("2006-01-02", date, time.Local); err2 == nil {
				t = t2
			} else {
				return nil, fmt.Errorf("--date must be dd/MM/yyyy: %v", err)
			}
		}
		out["startDateTime"] = map[string]any{"dateTime": t.Format("2006-01-02T15:04:05-07:00")}
	}
	dist := strings.TrimSpace(flags["distance-km"])
	if dist == "" {
		dist = strings.TrimSpace(flags["distance"])
	}
	if dist != "" {
		f, err := strconv.ParseFloat(dist, 64)
		if err != nil || f <= 0 {
			return nil, fmt.Errorf("--distance-km must be a positive number")
		}
		out["distance"] = map[string]any{"unit": "KILOMETRE", "value": strconv.FormatFloat(f, 'f', -1, 64)}
	} else if existing == nil {
		return nil, fmt.Errorf("mileage create requires --distance-km")
	}
	if s := strings.TrimSpace(flags["start"]); s != "" {
		out["startAddress"] = map[string]any{
			"addressComponents": []any{map[string]any{"name": "address_line_1", "value": s}},
			"geoLocation":       nil,
		}
	}
	if s := strings.TrimSpace(flags["end"]); s != "" {
		out["endAddress"] = map[string]any{
			"addressComponents": []any{map[string]any{"name": "address_line_1", "value": s}},
			"geoLocation":       nil,
		}
	}
	if existing == nil {
		out["autoTracked"] = false
	}
	if v := strings.TrimSpace(flags["vehicle"]); v != "" {
		vid, err := resolveTripVehicle(ctx, v)
		if err != nil {
			return nil, err
		}
		out["vehicle"] = map[string]any{"id": vid}
	} else if existing == nil {
		return nil, fmt.Errorf("mileage create requires --vehicle (vehicle node id or description; see the vehicles list)")
	}
	return out, nil
}

// resolveTripVehicle matches a flag value to a live vehicle node id — accepts
// the node id directly or a case-insensitive description substring.
func resolveTripVehicle(ctx context.Context, want string) (string, error) {
	vehicles, err := fetchTripVehicles(ctx)
	if err != nil {
		return "", err
	}
	for _, v := range vehicles {
		if v.id == want {
			return v.id, nil
		}
	}
	lower := strings.ToLower(want)
	for _, v := range vehicles {
		if strings.Contains(strings.ToLower(v.description), lower) {
			return v.id, nil
		}
	}
	return "", fmt.Errorf("no active vehicle matching %q (see company vehicles)", want)
}

type tripVehicle struct{ id, description string }

func fetchTripVehicles(ctx context.Context) ([]tripVehicle, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": "MileageVehicles",
		"variables":     map[string]any{},
		"query":         tripsVehiclesDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, tripsGraphQLURL, tripsHost, payload)
	if err != nil {
		return nil, fmt.Errorf("trips vehicles: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading trips vehicles: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data struct {
			Company struct {
				Vehicles struct {
					Edges []struct {
						Node struct {
							ID          string `json:"id"`
							Primary     bool   `json:"primary"`
							Description string `json:"description"`
							Active      bool   `json:"active"`
							Deleted     bool   `json:"deleted"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"vehicles"`
			} `json:"company"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("trips vehicles: malformed response")
	}
	if len(doc.Errors) > 0 && doc.Errors[0].Message != "" {
		return nil, fmt.Errorf("trips vehicles: %s", doc.Errors[0].Message)
	}
	var out []tripVehicle
	for _, e := range doc.Data.Company.Vehicles.Edges {
		n := e.Node
		if n.Deleted || !n.Active || n.ID == "" {
			continue
		}
		out = append(out, tripVehicle{id: n.ID, description: n.Description})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no active vehicles on this company")
	}
	return out, nil
}

// fetchTripNode returns the live trips node for update merges.
func fetchTripNode(ctx context.Context, id string) (map[string]any, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": "MileageListQuery",
		"variables":     map[string]any{"filter": "deleted=false"},
		"query":         tripsNodeDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, tripsGraphQLURL, tripsHost, payload)
	if err != nil {
		return nil, fmt.Errorf("trips MileageListQuery: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading trips MileageListQuery: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data struct {
			Company struct {
				Trips struct {
					Edges []struct {
						Node map[string]any `json:"node"`
					} `json:"edges"`
				} `json:"trips"`
			} `json:"company"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("trips MileageListQuery: malformed response")
	}
	for _, e := range doc.Data.Company.Trips.Edges {
		if s, _ := e.Node["id"].(string); s == id {
			return e.Node, nil
		}
	}
	return nil, fmt.Errorf("trip %s not found", id)
}

// tripsNodeDoc mirrors the UI's list query minus vehicle detail — enough for
// update merges (addresses, times, distance, reviewState, vehicle id).
const tripsNodeDoc = `query MileageListQuery($filter: String!) {
    company { trips(filterBy: $filter, limit: 50000) { edges { node {
        id description distance { value unit } reviewState autoTracked deleted
        startDateTime { dateTime } endDateTime { dateTime }
        startAddress { addressComponents { name value } geoLocation { latitude longitude } }
        endAddress { addressComponents { name value } geoLocation { latitude longitude } }
        vehicle { id }
    } } } }
}`

// PlannedTripMutateURL reports the mutation host for dry-run plans.
func PlannedTripMutateURL() string { return tripsGraphQLURL }
