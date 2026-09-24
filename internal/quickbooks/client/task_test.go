package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ADVANCED.TASKS_CREATE/EDIT/DELETE — taskManagement{Create,Update,Delete}Task
// on smallbusiness.api.intuit.com/graphql, reverse-engineered from the
// tasks-ui plugin bundle and live-proven on TC2 2026-09-19 (create →
// 1802244811, rename, delete success:true, list back to 0). The mutations
// need the tasks-ui namespace headers (intuit_target_domain/_usecase + the
// plugin apikey) — without them the service answers
// NAMESPACE_SHOULD_NOT_BE_EMPTY regardless of the GraphQL input.

// taskServer answers the identity assignee lookup (/v2/graphql) and the task
// mutation (/graphql) off one httptest.Server, discriminating on the path.
type taskServer struct {
	*httptest.Server
	mu        sync.Mutex
	calls     int
	lastPath  string
	lastBody  []byte
	lastHdrs  http.Header
	taskCalls int

	identityBody string
	taskBody     string
	taskStatus   int
}

func newTaskServer(t *testing.T) *taskServer {
	t.Helper()
	s := &taskServer{
		identityBody: `{"data":{"identityUsers":{"edges":[{"node":{"profile":{"id":"9341457769756876"}}}]}}}`,
		taskBody:     `{"data":{"taskManagementCreateTask":{"task":{"id":1802244811,"name":"zz","status":"Open","assignee":"9341457769756876","dueDate":"2026-09-30T00:00:00.000Z"}}}}`,
		taskStatus:   http.StatusOK,
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.calls++
		s.lastPath = r.URL.Path
		s.lastBody = b
		s.lastHdrs = r.Header.Clone()
		isTask := strings.HasSuffix(r.URL.Path, "/graphql") && !strings.HasSuffix(r.URL.Path, "/v2/graphql")
		if isTask {
			s.taskCalls++
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !isTask {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(s.identityBody))
			return
		}
		w.WriteHeader(s.taskStatus)
		_, _ = w.Write([]byte(s.taskBody))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *taskServer) snap() (calls, taskCalls int, path string, body []byte, hdrs http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.taskCalls, s.lastPath, s.lastBody, s.lastHdrs
}

func TestTaskCreatePostsRequiredInput(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayTaskMutate(context.Background(), "create", "", "zz task", "2026-09-30", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Item.ID != "1802244811" || res.Entity != "Task" {
		t.Fatalf("res = %+v", res)
	}
	calls, taskCalls, path, body, hdrs := srv.snap()
	if calls != 2 || taskCalls != 1 {
		t.Fatalf("expected identity lookup + task mutation, calls=%d taskCalls=%d", calls, taskCalls)
	}
	if path != "/graphql" {
		t.Fatalf("task mutation path = %s", path)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	doc, _ := m["query"].(string)
	for _, frag := range []string{`taskManagementCreateTask`, `name:"zz task"`, `status:"Open"`, `assignee:"9341457769756876"`, `dueDate:"2026-09-30T00:00:00Z"`} {
		if !strings.Contains(doc, frag) {
			t.Fatalf("doc missing %q: %s", frag, doc)
		}
	}
	if hdrs.Get("intuit_target_domain") != "QBO" || hdrs.Get("intuit_target_usecase") != "TASK_MANAGER" {
		t.Fatalf("namespace headers missing: %v", hdrs)
	}
	if !strings.Contains(hdrs.Get("Authorization"), "prdakyresBSLk02axlIe7sb1iNZAzFLfZDXm4waB") {
		t.Fatalf("tasks-ui apikey missing from Authorization: %v", hdrs.Get("Authorization"))
	}
}

func TestTaskCreateExplicitAssigneeSkipsIdentityLookup(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	interceptHTTP(t, srv.URL)
	_, err := ReplayTaskMutate(context.Background(), "create", "", "zz", "2026-09-30", "explicit-42")
	if err != nil {
		t.Fatal(err)
	}
	calls, taskCalls, _, body, _ := srv.snap()
	if calls != 1 || taskCalls != 1 {
		t.Fatalf("explicit assignee must skip the identity lookup, calls=%d", calls)
	}
	if !strings.Contains(string(body), `assignee:\"explicit-42\"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestTaskUpdatePostsIDAndName(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	srv.taskBody = `{"data":{"taskManagementUpdateTask":{"task":{"id":1802244811,"name":"renamed","status":"Open"}}}}`
	interceptHTTP(t, srv.URL)
	res, err := ReplayTaskMutate(context.Background(), "update", "1802244811", "renamed", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "1802244811" || res.Item.Type != "Task (Open)" {
		t.Fatalf("res = %+v", res)
	}
	_, _, _, body, _ := srv.snap()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	doc, _ := m["query"].(string)
	for _, frag := range []string{`taskManagementUpdateTask`, `id:1802244811`, `name:"renamed"`} {
		if !strings.Contains(doc, frag) {
			t.Fatalf("doc missing %q: %s", frag, doc)
		}
	}
}

func TestTaskDeletePostsID(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	srv.taskBody = `{"data":{"taskManagementDeleteTask":{"task":{"id":1802244811,"name":"zz"},"success":true}}}`
	interceptHTTP(t, srv.URL)
	res, err := ReplayTaskMutate(context.Background(), "delete", "1802244811", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Item.ID != "1802244811" {
		t.Fatalf("res = %+v", res)
	}
	_, _, _, body, _ := srv.snap()
	if !strings.Contains(string(body), `taskManagementDeleteTask`) || !strings.Contains(string(body), `id:1802244811`) {
		t.Fatalf("body = %s", body)
	}
}

func TestTaskMutateValidation(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	interceptHTTP(t, srv.URL)
	cases := []struct {
		op, id, name, due string
	}{
		{"create", "", "", "2026-09-30"},
		{"create", "", "zz", ""},
		{"create", "", "zz", "30/09/2026"},
		{"update", "", "zz", ""},
		{"update", "7", "", ""},
		{"delete", "", "", ""},
	}
	for _, c := range cases {
		if _, err := ReplayTaskMutate(context.Background(), c.op, c.id, c.name, c.due, ""); err == nil {
			t.Fatalf("%s(id=%q,name=%q,due=%q) expected error", c.op, c.id, c.name, c.due)
		}
	}
	calls, _, _, _, _ := srv.snap()
	if calls != 0 {
		t.Fatalf("validation must fail before any HTTP, calls=%d", calls)
	}
}

func TestTaskGQLErrorSurfaces(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	srv.taskBody = `{"errors":[{"message":"Domain and usecase values must be provided"}]}`
	interceptHTTP(t, srv.URL)
	_, err := ReplayTaskMutate(context.Background(), "delete", "7", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "Domain and usecase") {
		t.Fatalf("err = %v", err)
	}
}

func TestTaskDeleteRejectsFalseSuccessReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	srv.taskBody = `{"data":{"taskManagementDeleteTask":{"task":{"id":1802244811,"name":"zz"},"success":false}}}`
	interceptHTTP(t, srv.URL)
	_, err := ReplayTaskMutate(context.Background(), "delete", "1802244811", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}

func TestTaskAssigneeIDRejectsMultipleProfiles(t *testing.T) {
	saveUsableURIHost(t)
	srv := newTaskServer(t)
	srv.identityBody = `{"data":{"identityUsers":{"edges":[{"node":{"profile":{"id":"one"}}},{"node":{"profile":{"id":"two"}}}]}}}`
	interceptHTTP(t, srv.URL)
	if _, err := ReplayTaskMutate(context.Background(), "create", "", "zz", "2026-09-30", ""); err == nil || !strings.Contains(err.Error(), "pass --assignee explicitly") {
		t.Fatalf("err = %v", err)
	}
	if srv.taskCalls != 0 {
		t.Fatalf("task mutation must not fire after ambiguous identity lookup; calls=%d", srv.taskCalls)
	}
}
