package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProjectGetAllListViewEntitiesEmpty(t *testing.T) {
	body := []byte(`{"data":{"result":{"entities":[],"__typename":"ProductAndServicesListEntityWithVariantsPagedResult"}}}`)
	got := projectGetAllListViewEntities(body)
	if got.Entity != "InventoryOverview" {
		t.Fatalf("entity=%q, want InventoryOverview (not Item)", got.Entity)
	}
	if got.Entity == "Item" {
		t.Fatal("must not map to Item")
	}
	if len(got.Items) != 0 {
		t.Fatalf("items=%+v", got.Items)
	}
	if strings.Contains(got.Note, "Item") && !strings.Contains(got.Note, "ListView") {
		t.Fatalf("note leaked Item stand-in: %q", got.Note)
	}
}

func TestProjectGetAllListViewEntitiesDoesNotInventItemIDs(t *testing.T) {
	body := []byte(`{"data":{"result":{"entities":[{"productsAndServicesListEntity":{"id":"2","default":true},"__typename":"ProductAndServicesListEntityWithVariants"},{"productsAndServicesListEntity":{"id":"6","default":false},"__typename":"ProductAndServicesListEntityWithVariants"}],"__typename":"ProductAndServicesListEntityWithVariantsPagedResult"}}}`)
	got := projectGetAllListViewEntities(body)
	if got.Entity == "Item" {
		t.Fatal("must not map to Item")
	}
	if got.Counts["entities"] != 2 || got.Counts["items"] != 2 {
		t.Fatalf("counts=%v", got.Counts)
	}
	if got.Items[0].ID != "2" || got.Items[1].ID != "6" {
		t.Fatalf("items=%+v", got.Items)
	}
	if !strings.Contains(got.Note, "GetAllListViewEntities") {
		t.Fatalf("note=%q", got.Note)
	}
}

func TestProjectSearchAllListViewEntitiesEmpty(t *testing.T) {
	body := []byte(`{"data":{"searchAllListViewEntities":{"entities":[],"totalCount":0,"__typename":"ProductAndServicesListEntityPagedResult"}}}`)
	got := projectSearchAllListViewEntities(body)
	if got.Entity != "InventoryOverviewSearch" {
		t.Fatalf("entity=%q, want InventoryOverviewSearch (not Item)", got.Entity)
	}
	if got.Entity == "Item" {
		t.Fatal("must not map to Item")
	}
	if len(got.Items) != 0 || got.Counts["totalCount"] != 0 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Note, "SearchAllListViewEntities") || !strings.Contains(got.Note, "totalCount=0") {
		t.Fatalf("note=%q", got.Note)
	}
}

func TestProjectSearchAllListViewEntitiesRows(t *testing.T) {
	body := []byte(`{"data":{"searchAllListViewEntities":{"entities":[{"id":"9","entityName":"QB-CLI","qtyOnHand":3,"__typename":"ProductAndServicesListEntity"}],"totalCount":1}}}`)
	got := projectSearchAllListViewEntities(body)
	if got.Entity == "Item" {
		t.Fatal("must not map to Item")
	}
	if len(got.Items) != 1 || got.Items[0].ID != "9" || got.Items[0].Name != "QB-CLI" || got.Items[0].Qty != 3 {
		t.Fatalf("items=%+v", got.Items)
	}
}

func TestReplayQueryInventoryOverviewNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayQuery(t.Context(), "GetAllListViewEntities", "", "", 2)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
	_, err = ReplayQuery(t.Context(), "SearchAllListViewEntities", "", "QB-CLI", 2)
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("search got %v", err)
	}
}

func TestPlannedQueryURLInventoryOverview(t *testing.T) {
	got := PlannedQueryURL("GetAllListViewEntities")
	if got != inventoryOverviewGraphQLURL {
		t.Fatalf("%q", got)
	}
	got = PlannedQueryURL("SearchAllListViewEntities")
	if got != inventoryOverviewGraphQLURL {
		t.Fatalf("search %q", got)
	}
	item := PlannedQueryURL("Item")
	if !strings.Contains(item, "/query?") || strings.Contains(item, "commercecontrol") {
		t.Fatalf("Item planned URL must stay v3: %s", item)
	}
}

func TestInventoryOverviewPayloadMatchesCapture(t *testing.T) {
	if !strings.Contains(getAllListViewEntitiesQuery, "getAllListViewEntities") {
		t.Fatal("captured GetAllListViewEntities query missing")
	}
	if !strings.Contains(searchAllListViewEntitiesQuery, "searchAllListViewEntities") {
		t.Fatal("captured SearchAllListViewEntities query missing")
	}
	if strings.Contains(searchAllListViewEntitiesQuery, "GetIMSPref") {
		t.Fatal("must not wire GetIMSPref")
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "SearchAllListViewEntities",
		"variables": map[string]any{
			"filter":  map[string]any{"entityType": map[string]any{"value": []string{"PRODUCT"}, "comparator": "IN"}},
			"limit":   2,
			"keyword": "QB-CLI",
		},
		"query": searchAllListViewEntitiesQuery,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	if m["operationName"] != "SearchAllListViewEntities" {
		t.Fatalf("%v", m["operationName"])
	}
	vars := m["variables"].(map[string]any)
	if vars["keyword"] != "QB-CLI" {
		t.Fatalf("keyword=%v", vars["keyword"])
	}
	raw := string(payload)
	if strings.Contains(raw, "LOW_ON_STOCK") {
		t.Fatal("must not hard-require LOW_ON_STOCK as the only search")
	}
}
