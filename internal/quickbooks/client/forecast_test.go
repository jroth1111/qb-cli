package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// REPORTS.FORECAST_CREATE/EDIT/DELETE — planningforecasting
// createBusinessForecast / updateBusinessForecast / deleteBusinessForecasts,
// reverse-engineered from the forecasting-ui bundle and live-proven on TC2
// 2026-09-19 (create 3432826404261164992 → rename v0→v1 → delete
// status:true; list back to 0). Wire enum values differ from UI labels
// (measure RAW not RAW_ACTUALS); update requires the complete input — the
// client fetches the forecast and resends it.

// fcServer answers every /graphql call from canned bodies in order — create
// takes 1 call, update takes 2 (list-fetch then mutate), delete takes 1.
type fcServer struct {
	*httptest.Server
	mu      sync.Mutex
	bodies  [][]byte
	respond func(n int, body []byte) (int, string)
}

func newFCServer(t *testing.T, respond func(n int, body []byte) (int, string)) *fcServer {
	t.Helper()
	s := &fcServer{respond: respond}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, b)
		n := len(s.bodies)
		s.mu.Unlock()
		status, out := s.respond(n, b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *fcServer) calls() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte{}, s.bodies...)
}

func TestForecastCreatePostsFullInput(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFCServer(t, func(n int, b []byte) (int, string) {
		return 200, `{"data":{"createBusinessForecast":{"forecastId":"fc-1","version":0,"name":"zz","forecastMode":"MANUAL","forecastType":"PROFIT_AND_LOSS","intervalType":"MONTHLY","startDate":"2026-10-01","endDate":"2027-09-30"}}}`
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayForecastMutate(context.Background(), "create", "", "zz", "", "", "", "2026-10-01", "2027-09-30", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "fc-1" || res.Item.Type != "BusinessForecast (MANUAL)" {
		t.Fatalf("res = %+v", res)
	}
	calls := srv.calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	var m map[string]any
	if err := json.Unmarshal(calls[0], &m); err != nil {
		t.Fatal("no JSON body")
	}
	in := m["variables"].(map[string]any)["businessForecastInput"].(map[string]any)
	for k, want := range map[string]any{
		"name": "zz", "forecastMode": "MANUAL", "forecastType": "PROFIT_AND_LOSS",
		"intervalType": "MONTHLY", "startDate": "2026-10-01", "endDate": "2027-09-30",
		"secondaryListType": "NOT_SUBDIVIDED",
	} {
		if in[k] != want {
			t.Fatalf("input[%s] = %v want %v (input=%v)", k, in[k], want, in)
		}
	}
	sd := in["forecastSetupData"].(map[string]any)
	if sd["base"] != "ACTUAL" || sd["setupDetails"].(map[string]any)["measure"] != "RAW" {
		t.Fatalf("setupData = %v", sd)
	}
}

func TestForecastUpdateFetchesThenResends(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFCServer(t, func(n int, b []byte) (int, string) {
		if n == 1 {
			return 200, `{"data":{"getAllBusinessForecasts":{"edges":[{"node":{
				"forecastId":"fc-9","version":3,"name":"old","forecastMode":"SMART_INTEL",
				"forecastType":"THREE_WAY","intervalType":"QUARTERLY",
				"secondaryListType":"NOT_SUBDIVIDED","startDate":"2026-01-01","endDate":"2026-12-31",
				"viewSettings":{"secondaryId":null,"dimensionDefinitionId":null},
				"forecastSetupData":{"base":"ACTUAL","setupDetails":{"period":null,"measure":"RAW","budgetId":null}}
			}}]}}}`
		}
		return 200, `{"data":{"updateBusinessForecast":{"forecastId":"fc-9","version":4,"name":"new name","forecastMode":"SMART_INTEL"}}}`
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayForecastMutate(context.Background(), "update", "fc-9", "new name", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "fc-9" || res.Item.Type != "BusinessForecast (SMART_INTEL)" {
		t.Fatalf("res = %+v", res)
	}
	calls := srv.calls()
	if len(calls) != 2 {
		t.Fatalf("expected fetch+update, calls=%d", len(calls))
	}
	var m map[string]any
	_ = json.Unmarshal(calls[1], &m)
	in := m["variables"].(map[string]any)["forecastInput"].(map[string]any)
	if in["name"] != "new name" || in["version"].(float64) != 3 || in["forecastMode"] != "SMART_INTEL" || in["startDate"] != "2026-01-01" {
		t.Fatalf("resent input = %v", in)
	}
}

func TestForecastDeletePostsIDList(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFCServer(t, func(n int, b []byte) (int, string) {
		return 200, `{"data":{"deleteBusinessForecasts":[{"forecastId":"fc-9","status":true,"description":"Forecast Deleted Successfully"}]}}`
	})
	interceptHTTP(t, srv.URL)
	res, err := ReplayForecastMutate(context.Background(), "delete", "fc-9", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "fc-9" {
		t.Fatalf("res = %+v", res)
	}
	var m map[string]any
	_ = json.Unmarshal(srv.calls()[0], &m)
	ids := m["variables"].(map[string]any)["forecastIds"].([]any)
	if len(ids) != 1 || ids[0] != "fc-9" {
		t.Fatalf("forecastIds = %v", ids)
	}
}

func TestForecastMutateValidation(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFCServer(t, func(n int, b []byte) (int, string) { return 200, `{}` })
	interceptHTTP(t, srv.URL)
	cases := []struct {
		op, id, name, start, end string
	}{
		{"create", "", "", "2026-10-01", "2027-09-30"},
		{"create", "", "zz", "", "2027-09-30"},
		{"create", "", "zz", "2026-10-01", ""},
		{"create", "", "zz", "10/01/2026", "2027-09-30"},
		{"update", "", "zz", "", ""},
		{"update", "fc-9", "", "", ""},
		{"delete", "", "", "", ""},
	}
	for _, c := range cases {
		if _, err := ReplayForecastMutate(context.Background(), c.op, c.id, c.name, "", "", "", c.start, c.end, ""); err == nil {
			t.Fatalf("%s(id=%q,name=%q,start=%q) expected error", c.op, c.id, c.name, c.start)
		}
	}
	if len(srv.calls()) != 0 {
		t.Fatalf("validation must fail before HTTP")
	}
}

func TestForecastCreateBadEnumsRejected(t *testing.T) {
	saveUsableURIHost(t)
	srv := newFCServer(t, func(n int, b []byte) (int, string) { return 200, `{}` })
	interceptHTTP(t, srv.URL)
	if _, err := ReplayForecastMutate(context.Background(), "create", "", "zz", "CASH", "", "", "2026-10-01", "2027-09-30", ""); err == nil {
		t.Fatal("bad mode accepted")
	}
	if _, err := ReplayForecastMutate(context.Background(), "create", "", "zz", "", "CASH_FLOW", "", "2026-10-01", "2027-09-30", ""); err == nil {
		t.Fatal("bad type accepted")
	}
	if _, err := ReplayForecastMutate(context.Background(), "create", "", "zz", "", "", "WEEKLY", "2026-10-01", "2027-09-30", ""); err == nil {
		t.Fatal("bad interval accepted")
	}
	if _, err := ReplayForecastMutate(context.Background(), "create", "", "zz", "", "", "", "2026-10-01", "2027-09-30", "RAW_ACTUALS"); err == nil {
		t.Fatal("UI-side measure name accepted — wire expects raw")
	}
}
