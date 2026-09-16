//go:build live

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"testing"
	"time"
)

// inventory_live_test.go exercises the three inventory_adj.go v3 write
// surfaces against Test Company 2 with LIVE credentials:
//
//	ReplayInventoryCountCreate  -> POST /inventory-counts
//	ReplayBuildAssemblyCreate   -> POST /buildassembly
//	ReplayStartingValueCreate   -> POST /inventory_starting_value
//
// Run:
//
//	QB_HOME=/path/to/home (logged in as that company) \
//	  go test -tags live -run TestLiveInventory -v ./internal/quickbooks/client/
//
// Each test performs the full CRUD cycle: create via the production mutation,
// read back the created entity via GET, delete, then verify deletion via
// GET returning 404.
//
// LIVE FINDING (2026-08-24, realm 12345): all three route-derived
// v3 paths are rejected server-side —
//
//	POST {realm}/inventory-counts          -> 400 {"500": "Unsupported Operation: Operation inventory-counts is not supported."}
//	POST {realm}/buildassembly             -> same, "Operation buildassembly is not supported"
//	POST {realm}/inventory_starting_value  -> same, "Operation inventory_starting_value is not supported"
//
// i.e. these SPA routes (/app/inventory-counts, /app/buildassembly,
// /app/inventory_starting_value) are NOT v3 company-API operations. The
// capture corpus agrees: only read-side GraphQL exists (GetInventoryCounts);
// no captured write shape for any of the three. Fixture item id '42' is
// INACTIVE in the test company (GET -> code 610), so when the item gate opens the tests
// surface the Unsupported Operation failure from the create call below —
// which is the honest live result until real wire shapes are captured.

const invTestParentAccount = "44" // CR-Test-Account

func invLiveGate(t *testing.T) *apiClient {
	t.Helper()
	if os.Getenv("QB_HOME") == "" {
		t.Skip("live test: set QB_HOME=<test-home> to run")
	}
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if expected := os.Getenv("QB_EXPECTED_REALM"); expected == "" || ac.realm != expected {
		t.Fatal("live inventory writes require QB_EXPECTED_REALM matching the verified test session")
	}
	return ac
}

func liveJSONShape(raw []byte) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return fmt.Sprintf("non-object response (%d bytes)", len(raw))
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return fmt.Sprintf("object keys=%v", keys)
}

func invLiveCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func invV3Get(t *testing.T, ac *apiClient, path, id string) ([]byte, int) {
	t.Helper()
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/%s/%s?minorversion=73", ac.realm, path, id)
	resp, err := ac.getJSON(invLiveCtx(t), u, "")
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	body, err := readBody(resp)
	drainAndClose(resp)
	if err != nil {
		t.Fatalf("read GET %s response: %v", path, err)
	}
	return body, resp.StatusCode
}

func invV3Delete(t *testing.T, ac *apiClient, path, id string) ([]byte, int) {
	t.Helper()
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/%s/%s?minorversion=73", ac.realm, path, id)
	resp, err := ac.doStdlibJSON(invLiveCtx(t), http.MethodDelete, u, nil, "")
	if err != nil {
		t.Fatalf("DELETE %s: %v", u, err)
	}
	body, err := readBody(resp)
	drainAndClose(resp)
	if err != nil {
		t.Fatalf("read DELETE %s response: %v", path, err)
	}
	return body, resp.StatusCode
}

func invV3Wrap(body []byte, entity string) map[string]json.RawMessage {
	var wrap map[string]json.RawMessage
	_ = json.Unmarshal(body, &wrap)
	var obj map[string]json.RawMessage
	if wrap[entity] != nil {
		_ = json.Unmarshal(wrap[entity], &obj)
	}
	return obj
}

func invV3ID(obj map[string]json.RawMessage) string {
	return jsonString(obj, "Id")
}

func requireItemFixture(t *testing.T, ac *apiClient) string {
	t.Helper()
	body, status := invV3Get(t, ac, "item", "42")
	obj := invV3Wrap(body, "Item")
	id := invV3ID(obj)
	if id == "" || status != http.StatusOK {
		t.Skip("item not available")
	}
	return id
}

func invV3Account44(t *testing.T, ac *apiClient) string {
	t.Helper()
	body, status := invV3Get(t, ac, "account", invTestParentAccount)
	obj := invV3Wrap(body, "Account")
	name := jsonString(obj, "Name")
	if status != http.StatusOK || (name == "" && invV3ID(obj) == "") {
		t.Fatalf("parent account 44 not found: status=%d", status)
	}
	t.Logf("parent account 44 = %q", name)
	return invTestParentAccount
}

func assertDeleted(t *testing.T, ac *apiClient, path, id string) {
	t.Helper()
	_, status := invV3Get(t, ac, path, id)
	if status != http.StatusNotFound {
		t.Errorf("deletion not confirmed for %s/%s: expected 404, got status=%d", path, id, status)
	}
}

func TestLiveInventoryCountLifecycle(t *testing.T) {
	ac := invLiveGate(t)
	parent := invV3Account44(t, ac)
	item := requireItemFixture(t, ac)

	res, err := ReplayInventoryCountCreate(t.Context(), map[string]string{
		"item":        item,
		"counted-qty": "5",
		"memo":        "QB-CLI live lifecycle test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Status != http.StatusOK || res.Item.ID == "" {
		t.Fatalf("create returned no Id: %+v", res.Item)
	}
	id := res.Item.ID
	t.Logf("created InventoryCount id=%s", id)
	body, status := invV3Get(t, ac, InventoryCountPath, id)
	got := invV3Wrap(body, InventoryCountEntity)
	if status != http.StatusOK {
		t.Fatalf("read back %s/%s: status=%d", InventoryCountPath, id, status)
	}
	back := invV3ID(got)
	if back == "" {
		t.Fatalf("read-back missing Id in %v", liveJSONShape(body))
	}
	if back != id {
		t.Errorf("read-back Id %q != created Id %q", back, id)
	}
	t.Logf("read back InventoryCount id=%s sync_token=%s",
		back, jsonString(got, "SyncToken"))

	delBody, delStatus := invV3Delete(t, ac, InventoryCountPath, id)
	if delStatus != http.StatusOK {
		t.Fatalf("delete failed: status=%d msg=%s", delStatus, errorMessage(delBody))
	}
	assertDeleted(t, ac, InventoryCountPath, id)
	_ = parent
}

func TestLiveBuildAssemblyLifecycle(t *testing.T) {
	ac := invLiveGate(t)
	parent := invV3Account44(t, ac)
	item := requireItemFixture(t, ac)

	res, err := ReplayBuildAssemblyCreate(t.Context(), map[string]string{
		"item": item,
		"qty":  "1",
		"memo": "QB-CLI live lifecycle test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Status != http.StatusOK || res.Item.ID == "" {
		t.Fatalf("create returned no Id: %+v", res.Item)
	}
	id := res.Item.ID
	t.Logf("created BuildAssembly id=%s", id)

	body, status := invV3Get(t, ac, BuildAssemblyPath, id)
	got := invV3Wrap(body, BuildAssemblyEntity)
	if status != http.StatusOK {
		t.Fatalf("read back %s/%s: status=%d", BuildAssemblyPath, id, status)
	}
	if invV3ID(got) == "" {
		t.Fatalf("read-back missing Id in %v", liveJSONShape(body))
	}

	delBody, delStatus := invV3Delete(t, ac, BuildAssemblyPath, id)
	if delStatus != http.StatusOK {
		t.Fatalf("delete failed: status=%d msg=%s", delStatus, errorMessage(delBody))
	}
	assertDeleted(t, ac, BuildAssemblyPath, id)
	_ = parent
}

func TestLiveStartingValueLifecycle(t *testing.T) {
	ac := invLiveGate(t)
	parent := invV3Account44(t, ac)
	item := requireItemFixture(t, ac)

	res, err := ReplayStartingValueCreate(t.Context(), map[string]string{
		"item":          item,
		"qty":           "2",
		"cost":          "1.00",
		"asset-account": parent,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Status != http.StatusOK || res.Item.ID == "" {
		t.Fatalf("create returned no Id: %+v", res.Item)
	}
	id := res.Item.ID
	t.Logf("created StartingValue id=%s", id)

	body, status := invV3Get(t, ac, InventoryStartingValuePath, id)
	got := invV3Wrap(body, StartingValueEntity)
	if status != http.StatusOK {
		t.Fatalf("read back %s/%s: status=%d", InventoryStartingValuePath, id, status)
	}
	if invV3ID(got) == "" {
		t.Fatalf("read-back missing Id in %v", liveJSONShape(body))
	}

	delBody, delStatus := invV3Delete(t, ac, InventoryStartingValuePath, id)
	if delStatus != http.StatusOK {
		t.Fatalf("delete failed: status=%d msg=%s", delStatus, errorMessage(delBody))
	}
	assertDeleted(t, ac, InventoryStartingValuePath, id)
}
