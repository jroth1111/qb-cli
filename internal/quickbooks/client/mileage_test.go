package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The mileage suite exercises the trips service surface: company.trips on
// trips.api.intuit.com/v4/graphql, filterBy "deleted=false".

func tripsServer(t *testing.T, body string) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, body)
	interceptHTTP(t, srv.URL)
	return srv
}

func TestTripsListSendsFilterBy(t *testing.T) {
	srv := tripsServer(t, `{"data":{"company":{"trips":{"edges":[
		{"node":{"id":"gid:9","notes":"airport run","deleted":false,"entityVersion":1,"distance":{"value":12.4,"unit":"mi"},"vehicle":{"id":"v1","description":"sedan"}}}
	]}}}}`)
	res, err := ReplayTripsList(context.Background(), "", "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Counts["items"] != 1 || res.Items[0].ID != "gid:9" {
		t.Fatalf("items = %+v", res.Items)
	}
	sent := srv.sentMap()
	vars := sent["variables"].(map[string]any)
	if vars["filter"] != "deleted=false" {
		t.Fatalf("filter = %v", vars["filter"])
	}
}

func TestTripsListIDFilter(t *testing.T) {
	tripsServer(t, `{"data":{"company":{"trips":{"edges":[
		{"node":{"id":"g:1","notes":"a"}},{"node":{"id":"g:2","notes":"b"}}
	]}}}}`)
	res, err := ReplayTripsList(context.Background(), "2", "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Counts["items"] != 1 || res.Items[0].ID != "g:2" {
		t.Fatalf("items = %+v", res.Items)
	}
}

func TestTripsListQueryFilter(t *testing.T) {
	tripsServer(t, `{"data":{"company":{"trips":{"edges":[
		{"node":{"id":"g:1","notes":"airport"}},{"node":{"id":"g:2","notes":"office"}}
	]}}}}`)
	res, err := ReplayTripsList(context.Background(), "", "airport", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Counts["items"] != 1 || res.Items[0].ID != "g:1" {
		t.Fatalf("items = %+v", res.Items)
	}
}

func TestTripsListRejectsGraphQLError(t *testing.T) {
	tripsServer(t, `{"errors":[{"message":"bad filter"}],"data":{"company":{"trips":null}}}`)
	res, err := ReplayTripsList(context.Background(), "", "", 10)
	if err == nil || !strings.Contains(err.Error(), "bad filter") || res != nil {
		t.Fatalf("GraphQL failure must not report successful emptiness: result=%+v err=%v", res, err)
	}
}

// --- mutations (updateTrips_Trip) --------------------------------------------

// tripsMutServer answers by request content: the vehicles lookup, the trip
// list (update merge read), and the updateTrips_Trip mutation all ride the
// same /v4/graphql path, so dispatch keys off the query text.
func tripsMutServer(t *testing.T) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.calls++
		s.lastMeth = r.Method
		s.lastPath = r.URL.Path
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(string(b), "company { vehicles"):
			_, _ = w.Write([]byte(`{"data":{"company":{"vehicles":{"edges":[
				{"node":{"id":"veh:1","primary":true,"description":"My vehicle","active":true,"deleted":false}}
			]}}}}`))
		case strings.Contains(string(b), "trips(filterBy"):
			_, _ = w.Write([]byte(`{"data":{"company":{"trips":{"edges":[
				{"node":{"id":"trip:9","description":"old purpose","deleted":false,"reviewState":"BUSINESS",
					"distance":{"value":"5","unit":"KILOMETER"},"autoTracked":false,
					"startDateTime":{"dateTime":"2026-09-18T09:00:00+10:00"},
					"startAddress":{"addressComponents":[{"name":"address_line_1","value":"Sydney"}]},
					"vehicle":{"id":"veh:1"}}}
			]}}}}`))
		default:
			deleted := strings.Contains(string(b), `"deleted":true`)
			_, _ = w.Write([]byte(`{"data":{"updateTrips_Trip":{"tripsTrip":{
				"id":"trip:9","description":"qb test","deleted":` + map[bool]string{true: "true", false: "false"}[deleted] + `,"reviewState":"BUSINESS"}}}}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestTripCreateSendsTripsTrip(t *testing.T) {
	srv := tripsMutServer(t)
	res, err := ReplayTripMutate(context.Background(), "create", map[string]string{
		"vehicle": "my vehicle", "date": "19/09/2026", "distance-km": "7",
		"purpose": "client visit", "trip-type": "business",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Item.ID != "trip:9" {
		t.Fatalf("result = %+v", res.Item)
	}
	vars := srv.sentMap()["variables"].(map[string]any)
	trip := vars["tripsTrip"].(map[string]any)
	if _, hasID := trip["id"]; hasID {
		t.Fatalf("create must omit id: %v", trip)
	}
	if trip["reviewState"] != "BUSINESS" || trip["description"] != "client visit" {
		t.Fatalf("trip = %v", trip)
	}
	if trip["vehicle"].(map[string]any)["id"] != "veh:1" {
		t.Fatalf("vehicle = %v", trip["vehicle"])
	}
	d := trip["distance"].(map[string]any)
	if d["unit"] != "KILOMETRE" || d["value"] != "7" {
		t.Fatalf("distance = %v", d)
	}
	if trip["startDateTime"].(map[string]any)["dateTime"] == "" {
		t.Fatalf("startDateTime = %v", trip["startDateTime"])
	}
}

func TestTripUpdateMergesExistingNode(t *testing.T) {
	srv := tripsMutServer(t)
	_, err := ReplayTripMutate(context.Background(), "update", map[string]string{
		"id": "trip:9", "distance-km": "12", "trip-type": "personal",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	vars := srv.sentMap()["variables"].(map[string]any)
	trip := vars["tripsTrip"].(map[string]any)
	if trip["id"] != "trip:9" {
		t.Fatalf("trip = %v", trip)
	}
	if trip["reviewState"] != "PERSONAL" {
		t.Fatalf("reviewState = %v", trip["reviewState"])
	}
	if trip["distance"].(map[string]any)["value"] != "12" {
		t.Fatalf("distance = %v", trip["distance"])
	}
	// merged from the live node, not cleared
	if trip["description"] != "old purpose" || trip["vehicle"].(map[string]any)["id"] != "veh:1" {
		t.Fatalf("merge lost fields: %v", trip)
	}
}

func TestTripDeleteSendsDeletedTrue(t *testing.T) {
	srv := tripsMutServer(t)
	res, err := ReplayTripMutate(context.Background(), "delete", map[string]string{"id": "trip:9"})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Item.ID != "trip:9" {
		t.Fatalf("result = %+v", res.Item)
	}
	trip := srv.sentMap()["variables"].(map[string]any)["tripsTrip"].(map[string]any)
	if trip["id"] != "trip:9" || trip["deleted"] != true {
		t.Fatalf("tripsTrip = %v", trip)
	}
}
