package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTripDeleteRejectsActiveReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"updateTrips_Trip":{"tripsTrip":{"id":"trip-1","deleted":false}}}}`)
	interceptHTTP(t, srv.URL)
	_, err := ReplayTripMutate(context.Background(), "delete", map[string]string{"id": "trip-1"})
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}

func TestTripDeleteRejectsMissingDeletedState(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"updateTrips_Trip":{"tripsTrip":{"id":"trip-1"}}}}`)
	interceptHTTP(t, srv.URL)
	_, err := ReplayTripMutate(context.Background(), "delete", map[string]string{"id": "trip-1"})
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}

func TestTripCreateRejectsDeletedReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "company { vehicles") {
			_, _ = w.Write([]byte(`{"data":{"company":{"vehicles":{"edges":[{"node":{"id":"veh:1","primary":true,"description":"Test vehicle","active":true,"deleted":false}}]}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"updateTrips_Trip":{"tripsTrip":{"id":"trip-1","deleted":true}}}}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	_, err := ReplayTripMutate(context.Background(), "create", map[string]string{
		"vehicle": "veh:1", "date": "2026-09-19", "distance-km": "7",
	})
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}
