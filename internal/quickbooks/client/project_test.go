package client

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// The project suite exercises the accountantworkflow Work_Project surface:
// create/update ride catalogued mutations over doURIHost, and get/search run
// the getJobs_workflow company.projects edges query.

func projectServer(t *testing.T, body string) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, body)
	interceptHTTP(t, srv.URL)
	return srv
}

func TestProjectCreateSendsWorkProjectInput(t *testing.T) {
	srv := projectServer(t, `{"data":{"createWork_Project":{"clientMutationId":"x","workProjectEdge":{"node":{"id":"gid:1","name":"P"}}}}}`)
	res, err := ReplayProjectCreate(context.Background(), map[string]string{
		"name": "P", "customer": "7", "due-date": "2026-12-31", "description": "d",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Item.ID != "gid:1" || res.Item.Name != "P" {
		t.Fatalf("item = %+v", res.Item)
	}
	sent := srv.sentMap()
	vars := sent["variables"].(map[string]any)
	input := vars["input"].(map[string]any)
	wp := input["workProject"].(map[string]any)
	if wp["name"] != "P" || wp["dueDate"] != "2026-12-31" || wp["description"] != "d" {
		t.Fatalf("workProject = %v", wp)
	}
	if client := wp["client"].(map[string]any); client["id"] != "7" {
		t.Fatalf("client = %v", client)
	}
	if q, _ := sent["query"].(string); strings.Contains(q, `\t`) {
		t.Fatal("document still carries literal \\t artifacts")
	}
}

func TestProjectCreateRequiresNameAndNumericCustomer(t *testing.T) {
	projectServer(t, `{}`)
	if _, err := ReplayProjectCreate(context.Background(), map[string]string{"customer": "1"}); err == nil {
		t.Fatal("missing --name accepted")
	}
	if _, err := ReplayProjectCreate(context.Background(), map[string]string{"name": "P"}); err == nil {
		t.Fatal("missing --customer accepted")
	}
	if _, err := ReplayProjectCreate(context.Background(), map[string]string{"name": "P", "customer": "abc"}); err == nil {
		t.Fatal("non-numeric --customer accepted")
	}
}

func TestProjectUpdateSendsOnlyTouchedFields(t *testing.T) {
	srv := projectServer(t, `{"data":{"updateWork_Project":{"clientMutationId":"x","workProject":{"id":"gid:9","name":"P2"}}}}`)
	res, err := ReplayProjectUpdate(context.Background(), map[string]string{
		"id": "gid:9", "name": "P2", "entity-version": "3",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if res.Item.ID != "gid:9" {
		t.Fatalf("item = %+v", res.Item)
	}
	input := srv.sentMap()["variables"].(map[string]any)["input"].(map[string]any)
	wp := input["workProject"].(map[string]any)
	if wp["id"] != "gid:9" || wp["name"] != "P2" || wp["entityVersion"] != "3" {
		t.Fatalf("workProject = %v", wp)
	}
	if _, ok := wp["dueDate"]; ok {
		t.Fatalf("untouched field sent: %v", wp)
	}
}

func TestProjectUpdateRejectsNoop(t *testing.T) {
	projectServer(t, `{}`)
	if _, err := ReplayProjectUpdate(context.Background(), map[string]string{"id": "gid:9"}); err == nil {
		t.Fatal("update without fields accepted")
	}
	if _, err := ReplayProjectUpdate(context.Background(), map[string]string{"id": "gid:9", "entity-version": "1"}); err == nil {
		t.Fatal("entityVersion alone must not count as a change")
	}
}

func TestProjectUpdateGraphQLErrorSurfaces(t *testing.T) {
	projectServer(t, `{"errors":[{"message":"STALE_DATA_ERROR"}],"data":null}`)
	_, err := ReplayProjectUpdate(context.Background(), map[string]string{"id": "gid:9", "name": "x", "entity-version": "0"})
	if err == nil || !strings.Contains(err.Error(), "STALE_DATA_ERROR") {
		t.Fatalf("err = %v", err)
	}
}

func TestProjectUpdateRejectsMismatchedReceipt(t *testing.T) {
	projectServer(t, `{"data":{"updateWork_Project":{"workProject":{"id":"gid:other","name":"P"}}}}`)
	_, err := ReplayProjectUpdate(context.Background(), map[string]string{"id": "gid:9", "name": "P"})
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}

func TestProjectListProjectsEdgesAndFiltersByID(t *testing.T) {
	projectServer(t, `{"data":{"company":{"projects":{"edges":[
		{"node":{"id":"g:100","name":"Alpha","status":"Inprogress"}},
		{"node":{"id":"g:200","name":"Beta","status":"Inprogress"}}
	]}}}}`)
	res, err := ReplayProjectList(context.Background(), "", "", 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Items) != 2 || res.Items[0].ID != "g:100" {
		t.Fatalf("items = %+v", res.Items)
	}
	res, err = ReplayProjectList(context.Background(), "200", "", 20)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "g:200" {
		t.Fatalf("id-suffix filter items = %+v", res.Items)
	}
	res, err = ReplayProjectList(context.Background(), "", "beta", 20)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "Beta" {
		t.Fatalf("query filter items = %+v", res.Items)
	}
}
