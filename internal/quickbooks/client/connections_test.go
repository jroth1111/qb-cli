package client

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProjectConnectionsEmpty(t *testing.T) {
	raw := []byte(`{"data":{"company":{"__typename":"Company","connections":{"__typename":"Integration_ConnectionConnection","edges":[]}}}}`)
	items, note, err := projectConnections(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items=%+v, want empty", items)
	}
	if !strings.Contains(note, "edges=[]") {
		t.Fatalf("note=%q, want edges=[]", note)
	}
}

func TestProjectConnectionsOneEdge(t *testing.T) {
	raw := []byte(`{"data":{"company":{"connections":{"edges":[{"node":{"id":"c1","status":"CONNECTED","provider":{"name":"Shopify","providerAppId":"SHOPIFY_IDX"}}}]}}}}`)
	items, note, err := projectConnections(raw)
	if err != nil {
		t.Fatal(err)
	}
	if note != "" {
		t.Fatalf("note=%q, want empty when rows exist", note)
	}
	if len(items) != 1 || items[0].ID != "c1" || items[0].Name != "Shopify" || items[0].Type != "CONNECTED" {
		t.Fatalf("%+v", items)
	}
}

func TestProjectConnectionsGraphQLError(t *testing.T) {
	raw := []byte(`{"errors":[{"message":"nope"}]}`)
	_, _, err := projectConnections(raw)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("got %v", err)
	}
}

func TestPlannedConnectionsURL(t *testing.T) {
	if PlannedConnectionsURL() != "https://qbo.intuit.com/api/v4/graphql" {
		t.Fatalf("%s", PlannedConnectionsURL())
	}
}

func TestReplayConnectionsNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayConnections(t.Context(), "", 2)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v", err)
	}
}
