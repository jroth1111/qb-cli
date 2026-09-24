package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Form-style update rides txnsrendering GET+PUT on /v2/customizations/{id} —
// a full-document read-modify-write where the GET's numeric
// templateEditsequence is echoed back as the PUT's string
// templateEditSequence plus a static clientMutationId.

const formStyleDoc = `{
	"id": "1125188",
	"customizationName": "Standard",
	"formType": "ALL",
	"customizationStatus": "ACTIVE",
	"defaultCustomization": true,
	"templateEditsequence": 7,
	"templateCustomization": {
		"design": {
			"templateName": "airy",
			"stylePrefs": {
				"generalStyles": {
					"foregroundColor": "#4F90BB",
					"backgroundColor": "#d6eee4"
				}
			}
		}
	}
}`

func formStyleServer(t *testing.T) *domServer {
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
		if !strings.Contains(r.URL.Path, "/v2/customizations/1125188") {
			_, _ = w.Write([]byte(`{"error":[{"message":"unmatched"}]}`))
			return
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(formStyleDoc))
		case http.MethodPut:
			_, _ = w.Write([]byte(`{"id":"1125188","ok":true}`))
		default:
			_, _ = w.Write([]byte(`{"error":[{"message":"method"}]}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestFormStyleUpdateRoundTripsDoc(t *testing.T) {
	srv := formStyleServer(t)
	res, err := ReplayFormStyleUpdate(context.Background(), "1125188", "", "#33AA77")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if res.Op != "form-style update" || res.Item.ID != "1125188" {
		t.Fatalf("result = %+v", res)
	}
	calls, meth, _, body := srv.snap()
	if calls != 2 || meth != http.MethodPut {
		t.Fatalf("calls=%d meth=%s", calls, meth)
	}
	var put map[string]any
	if err := json.Unmarshal(body, &put); err != nil {
		t.Fatalf("put body: %v", err)
	}
	if put["templateEditSequence"] != "7" {
		t.Fatalf("templateEditSequence = %v, want \"7\" (string echo of GET's 7)", put["templateEditSequence"])
	}
	if _, ok := put["templateEditsequence"]; ok {
		t.Fatal("raw GET key templateEditsequence must be dropped from PUT")
	}
	if put["clientMutationId"] != "0" {
		t.Fatalf("clientMutationId = %v", put["clientMutationId"])
	}
	styles := put["templateCustomization"].(map[string]any)["design"].(map[string]any)["stylePrefs"].(map[string]any)["generalStyles"].(map[string]any)
	if styles["foregroundColor"] != "#33AA77" {
		t.Fatalf("foregroundColor = %v", styles["foregroundColor"])
	}
	if styles["backgroundColor"] != "#d6eee4" {
		t.Fatalf("untouched field mutated: %v", styles["backgroundColor"])
	}
}

func TestFormStyleUpdateName(t *testing.T) {
	srv := formStyleServer(t)
	res, err := ReplayFormStyleUpdate(context.Background(), "1125188", "Standard Renamed", "")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if res.Item.Name != "Standard Renamed" {
		t.Fatalf("result = %+v", res)
	}
	var put map[string]any
	_, _, _, body := srv.snap()
	if err := json.Unmarshal(body, &put); err != nil {
		t.Fatalf("put body: %v", err)
	}
	if put["customizationName"] != "Standard Renamed" {
		t.Fatalf("customizationName = %v", put["customizationName"])
	}
}

func TestFormStyleUpdateNoopWhenUnchanged(t *testing.T) {
	srv := formStyleServer(t)
	res, err := ReplayFormStyleUpdate(context.Background(), "1125188", "Standard", "#4F90BB")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(res.Note, "already at requested") {
		t.Fatalf("result = %+v", res)
	}
	calls, meth, _, _ := srv.snap()
	if calls != 1 || meth != http.MethodGet {
		t.Fatalf("noop must not PUT: calls=%d meth=%s", calls, meth)
	}
}

func TestFormStyleUpdateValidation(t *testing.T) {
	for _, tc := range []struct{ id, name, color, want string }{
		{"", "x", "", "requires --id"},
		{"1", "", "", "needs a change"},
		{"1", "", "33AA77", "want #RRGGBB"},
		{"1", "", "#ZZZZZZ", "want #RRGGBB"},
		{"1", "", "#FFF", "want #RRGGBB"},
	} {
		if _, err := ReplayFormStyleUpdate(context.Background(), tc.id, tc.name, tc.color); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%+v: err = %v, want %q", tc, err, tc.want)
		}
	}
}

func TestFormStyleUpdateMissingStylePrefs(t *testing.T) {
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"9","customizationName":"X","templateCustomization":{"design":{}}}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	if _, err := ReplayFormStyleUpdate(context.Background(), "9", "", "#123456"); err == nil ||
		!strings.Contains(err.Error(), "no design.stylePrefs.generalStyles") {
		t.Fatalf("err = %v", err)
	}
}
