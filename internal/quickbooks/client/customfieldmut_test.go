package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The custom-field delete suite exercises the CES surface: the node fetch
// (CustomFieldsQueryCES) and the soft-delete mutation
// (updateCustomFieldDefinition, payload.deleted=true) both ride
// customextensions.api.intuit.com/graphql — dispatch keys off the query text.

func cesMutServer(t *testing.T) *domServer {
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
		case strings.Contains(string(b), "CustomFieldsQueryCES"):
			_, _ = w.Write([]byte(`{"data":{"customFieldDefinitions":{"edges":[
				{"node":{"id":"cf:1","name":"udcf_1","deleted":false,"colorCode":null,
					"schema":{"type":"string","title":"Field One","format":null,
						"allowedOperations":[],"allowedValues":[],
						"uiValidations":null,"metadataProperties":[],"__typename":"CustomFieldDefinition_Schema"},
					"associatedEntityTypes":[{"type":"/network/Contact","deleted":false,
						"allowedOperations":[],"entityConditions":[{"subtype":"CUSTOMER",
						"deleted":false,"allowedOperations":[],"__typename":"C"}],
						"__typename":"CustomFieldDefinition_AssociatedEntityType"}],
					"__typename":"CustomFieldDefinition"}}
			]}}}`))
		case strings.Contains(string(b), "createCustomFieldDefinition"):
			_, _ = w.Write([]byte(`{"data":{"createCustomFieldDefinition":{
				"id":"cf:1","name":"udcf_1","deleted":false}}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"updateCustomFieldDefinition":{
				"id":"cf:1","name":"udcf_1","deleted":true}}}`))
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestCustomFieldDeleteSendsDeletedTrue(t *testing.T) {
	srv := cesMutServer(t)
	res, err := ReplayCustomFieldDelete(context.Background(), "cf:1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Item.ID != "cf:1" || res.Item.Name != "udcf_1" {
		t.Fatalf("result = %+v", res.Item)
	}
	vars := srv.sentMap()["variables"].(map[string]any)
	in := vars["input_0"].(map[string]any)
	payload := in["payload"].(map[string]any)
	if payload["id"] != "cf:1" || payload["name"] != "udcf_1" || payload["deleted"] != true {
		t.Fatalf("payload = %v", payload)
	}
	// __typename must be stripped from nested read nodes
	schema := payload["schema"].(map[string]any)
	if _, has := schema["__typename"]; has {
		t.Fatalf("schema leaks __typename: %v", schema)
	}
	aet := payload["associatedEntityTypes"].([]any)[0].(map[string]any)
	if _, has := aet["__typename"]; has {
		t.Fatalf("associatedEntityTypes leaks __typename: %v", aet)
	}
	ec := aet["entityConditions"].([]any)[0].(map[string]any)
	if _, has := ec["__typename"]; has {
		t.Fatalf("entityConditions leaks __typename: %v", ec)
	}
}

func TestCustomFieldCreateSendsCESPayload(t *testing.T) {
	srv := cesMutServer(t)
	res, err := ReplayCustomFieldMutate(context.Background(), "create", map[string]string{
		"name": "Lifecycle", "type": "text",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Item.ID != "cf:1" || res.Op != "create" {
		t.Fatalf("result = %+v", res)
	}
	vars := srv.sentMap()["variables"].(map[string]any)
	payload := vars["input_0"].(map[string]any)["payload"].(map[string]any)
	schema := payload["schema"].(map[string]any)
	if schema["title"] != "Lifecycle" || schema["type"] != "string" {
		t.Fatalf("schema = %v", schema)
	}
	if payload["deleted"] != false {
		t.Fatalf("deleted = %v", payload["deleted"])
	}
	assoc := payload["associatedEntityTypes"].([]any)
	if len(assoc) == 0 {
		t.Fatal("create must send at least one active association")
	}
	first := assoc[0].(map[string]any)
	if first["type"] != "/network/Contact" {
		t.Fatalf("assoc = %v", first)
	}
	ec := first["entityConditions"].([]any)[0].(map[string]any)
	if ec["subtype"] != "CUSTOMER" || ec["deleted"] != false {
		t.Fatalf("entityCondition = %v", ec)
	}
}

func TestCustomFieldUpdateMergesNode(t *testing.T) {
	srv := cesMutServer(t)
	_, err := ReplayCustomFieldMutate(context.Background(), "update", map[string]string{
		"id": "cf:1", "name": "Renamed",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	vars := srv.sentMap()["variables"].(map[string]any)
	payload := vars["input_0"].(map[string]any)["payload"].(map[string]any)
	if payload["id"] != "cf:1" || payload["name"] != "udcf_1" {
		t.Fatalf("payload = %v", payload)
	}
	schema := payload["schema"].(map[string]any)
	if schema["title"] != "Renamed" || schema["type"] != "string" {
		t.Fatalf("schema = %v", schema)
	}
	// untouched facets merge from the live node
	if payload["deleted"] != false {
		t.Fatalf("deleted = %v", payload["deleted"])
	}
	assoc := payload["associatedEntityTypes"].([]any)
	if len(assoc) != 1 || assoc[0].(map[string]any)["type"] != "/network/Contact" {
		t.Fatalf("assoc = %v", assoc)
	}
}

func TestCustomFieldCreateRequiresName(t *testing.T) {
	if _, err := ReplayCustomFieldMutate(context.Background(), "create", map[string]string{}); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCustomFieldDeleteRequiresID(t *testing.T) {
	if _, err := ReplayCustomFieldDelete(context.Background(), "  "); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestCustomFieldDeleteNotFound(t *testing.T) {
	cesMutServer(t)
	if _, err := ReplayCustomFieldDelete(context.Background(), "cf:missing"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestCustomFieldDeleteSurfacesGraphQLError(t *testing.T) {
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
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(b), "CustomFieldsQueryCES") {
			_, _ = w.Write([]byte(`{"data":{"customFieldDefinitions":{"edges":[
				{"node":{"id":"cf:1","name":"x","schema":{},"associatedEntityTypes":[]}}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"errors":[{"message":"payload rejected"}]}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	if _, err := ReplayCustomFieldDelete(context.Background(), "cf:1"); err == nil ||
		!strings.Contains(err.Error(), "payload rejected") {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomFieldCreateRejectsMissingReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"createCustomFieldDefinition":null}}`)
	interceptHTTP(t, srv.URL)
	_, err := ReplayCustomFieldMutate(context.Background(), "create", map[string]string{"name": "Lifecycle"})
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}
