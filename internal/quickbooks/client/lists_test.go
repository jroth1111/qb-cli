package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProjectListsPrefsCapturedShape(t *testing.T) {
	body := []byte(`[{"$type":"/Result","data":[{"company":{"settings":{"entityVersion":null,"inventorySettings":{"trackQuantityOnHand":true}},"contacts":{"edges":[{"node":{"profiles":{"user":{"qboAppData":{"accountNumberVisible":false}}}}}]}}}]}]`)
	got := projectListsPrefs(body)
	if got.Entity != "ListsPrefs" {
		t.Fatalf("entity=%q, want ListsPrefs (not Account)", got.Entity)
	}
	if got.Entity == "Account" {
		t.Fatal("must not map to Account")
	}
	if got.Counts["contacts"] != 1 || got.Counts["settings"] != 1 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if len(got.Items) != 1 || got.Items[0].Type != "Prefs" {
		t.Fatalf("items=%+v", got.Items)
	}
	if got.Prefs == nil {
		t.Fatal("prefs payload missing")
	}
	if !strings.Contains(got.Note, "ListsPrefs") || !strings.Contains(got.Note, "contacts=1") {
		t.Fatalf("note=%q", got.Note)
	}
	if strings.Contains(got.Note, "Account") && strings.Contains(got.Note, "stand-in") {
		t.Fatalf("note leaked Account stand-in: %q", got.Note)
	}
}

func TestProjectListsPrefsUnparsed(t *testing.T) {
	got := projectListsPrefs([]byte(`not-json`))
	if got.Entity != "ListsPrefs" || len(got.Items) != 0 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Note, "unparsed") {
		t.Fatalf("note=%q", got.Note)
	}
}

func TestReplayQueryListsPrefsNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayQuery(t.Context(), "ListsPrefs", "", "", 2)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestPlannedQueryURLListsPrefs(t *testing.T) {
	got := PlannedQueryURL("ListsPrefs")
	if got != listsEntitiesURL {
		t.Fatalf("PlannedQueryURL(ListsPrefs)=%q, want v4/entities", got)
	}
	acc := PlannedQueryURL("Account")
	if !strings.Contains(acc, "/query?") || strings.Contains(acc, "v4/entities") {
		t.Fatalf("Account planned URL must stay v3: %s", acc)
	}
}

func TestListsPrefsPayloadMatchesCapture(t *testing.T) {
	q := listsPrefsGraphQuery("9341457731465363")
	if !strings.Contains(q, `settings (with: "ListsPrefs")`) {
		t.Fatal("captured ListsPrefs graphQuery missing")
	}
	if !strings.Contains(q, "groups='Prefs'") {
		t.Fatal("captured Prefs contact filter missing")
	}
	payload, err := json.Marshal([]map[string]any{{
		"$type": "/Query", "graphQuery": q, "variables": nil,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var arr []map[string]any
	if err := json.Unmarshal(payload, &arr); err != nil || len(arr) != 1 {
		t.Fatalf("%v %s", err, payload)
	}
	if arr[0]["$type"] != "/Query" {
		t.Fatalf("%v", arr[0]["$type"])
	}
}
