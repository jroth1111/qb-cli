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

// ruleServer answers getRules/save/delete by path suffix, recording bodies.
func ruleServer(t *testing.T, getRulesBody string) (*domServer, *map[string][]map[string]any) {
	t.Helper()
	calls := map[string][]map[string]any{}
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getRules"):
			calls["getRules"] = append(calls["getRules"], m)
			_, _ = w.Write([]byte(getRulesBody))
		case strings.HasSuffix(r.URL.Path, "/save"):
			calls["save"] = append(calls["save"], m)
			_, _ = w.Write([]byte(`{"olbRule":{"id":9,"ruleName":"x","validRule":true}}`))
		case strings.HasSuffix(r.URL.Path, "/delete"):
			calls["delete"] = append(calls["delete"], m)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(s.Close)
	return s, &calls
}

func TestRuleSaveBuildsWireBody(t *testing.T) {
	saveUsableURIHost(t)
	srv, calls := ruleServer(t, `{"rules":[]}`)
	interceptHTTP(t, srv.URL)

	res, err := ReplayRuleSave(context.Background(), map[string]string{
		"name":        "CR-Test-Rule",
		"bank-text":   "CRTESTDESC",
		"category-id": "7",
		"money":       "out",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "create" || res.Item.ID != "9" {
		t.Fatalf("result = %+v", res)
	}
	saved := (*calls)["save"]
	if len(saved) != 1 {
		t.Fatalf("save calls = %d", len(saved))
	}
	body := saved[0]
	if body["id"] != float64(-1) || body["ruleName"] != "CR-Test-Rule" {
		t.Fatalf("body = %#v", body)
	}
	cl, _ := body["conditionList"].(map[string]any)
	if cl["isAndRule"] != true {
		t.Fatalf("isAndRule = %#v", cl)
	}
	conds, _ := cl["ruleConditions"].([]any)
	if len(conds) != 2 {
		t.Fatalf("ruleConditions = %#v", conds)
	}
	c0, _ := conds[0].(map[string]any)
	c1, _ := conds[1].(map[string]any)
	if c0["ruleType"] != float64(10) || c0["value"] != "-1" {
		t.Fatalf("money-flow condition = %#v", c0)
	}
	if c1["ruleType"] != float64(6) || c1["value"] != "CRTESTDESC" {
		t.Fatalf("bank-text condition = %#v", c1)
	}
	al, _ := body["actionList"].(map[string]any)
	acts, _ := al["ruleActions"].([]any)
	a0, _ := acts[0].(map[string]any)
	if a0["actionType"] != float64(0) || a0["value"] != "7" {
		t.Fatalf("category action = %#v", a0)
	}
}

func TestRuleEditFetchesEditSequence(t *testing.T) {
	saveUsableURIHost(t)
	getRules := `{"rules":[{"id":4,"ruleName":"Old Name","editSequence":3,"ruleOrder":2}]}`
	srv, calls := ruleServer(t, getRules)
	interceptHTTP(t, srv.URL)

	_, err := ReplayRuleSave(context.Background(), map[string]string{
		"id":          "4",
		"bank-text":   "NEW",
		"category-id": "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len((*calls)["getRules"]) != 1 {
		t.Fatal("edit did not fetch existing rule")
	}
	body := (*calls)["save"][0]
	if body["id"] != float64(4) || body["editSequence"] != float64(3) || body["ruleOrder"] != float64(2) {
		t.Fatalf("edit body = %#v", body)
	}
	if body["ruleName"] != "Old Name" {
		t.Fatalf("name fallback lost: %#v", body["ruleName"])
	}
}

func TestRuleEditMissingRuleErrors(t *testing.T) {
	saveUsableURIHost(t)
	srv, calls := ruleServer(t, `{"rules":[]}`)
	interceptHTTP(t, srv.URL)

	_, err := ReplayRuleSave(context.Background(), map[string]string{"id": "99", "bank-text": "x"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing rule accepted: %v", err)
	}
	if len((*calls)["save"]) != 0 {
		t.Fatal("save posted for a missing rule")
	}
}

func TestRuleDeletePostsID(t *testing.T) {
	saveUsableURIHost(t)
	deleted := false
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getRules") {
			if deleted {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_, _ = w.Write([]byte(`[{"id":4,"ruleName":"pilot"}]`))
			}
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/batchDelete") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var raw []byte
		var err error
		if raw, err = io.ReadAll(r.Body); err != nil {
			t.Fatal(err)
		}
		body = string(raw)
		deleted = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)

	res, err := ReplayRuleDelete(context.Background(), map[string]string{"id": "4"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "delete" {
		t.Fatalf("result = %+v", res)
	}
	if body != "4" {
		t.Fatalf("delete body = %q, want space-joined ID", body)
	}
	if !deleted {
		t.Fatal("mock target was not deleted")
	}
}

func TestRuleDeleteRejectsHTTP200WithoutRemoval(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getRules") {
			_, _ = w.Write([]byte(`[{"id":4,"ruleName":"still-here"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	if result, err := ReplayRuleDelete(context.Background(), map[string]string{"id": "4"}); err == nil || result != nil || !strings.Contains(err.Error(), "inspect live state") {
		t.Fatalf("false success: result=%v err=%v", result, err)
	}
}

func TestRuleDeleteAcceptsEmpty200OnlyAfterReadback(t *testing.T) {
	saveUsableURIHost(t)
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/getRules") {
			if deleted {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_, _ = w.Write([]byte(`[{"id":4,"ruleName":"pilot"}]`))
			}
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/batchDelete") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		deleted = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	result, err := ReplayRuleDelete(context.Background(), map[string]string{"id": "4"})
	if err != nil || result == nil || !deleted {
		t.Fatalf("result=%v err=%v deleted=%v", result, err, deleted)
	}
}

func TestRuleValidation(t *testing.T) {
	saveUsableURIHost(t)
	srv, calls := ruleServer(t, `{}`)
	interceptHTTP(t, srv.URL)

	if _, err := ReplayRuleSave(context.Background(), map[string]string{"bank-text": "x"}); err == nil {
		t.Fatal("missing --name accepted")
	}
	if _, err := ReplayRuleSave(context.Background(), map[string]string{"name": "x", "match": "bogus"}); err == nil {
		t.Fatal("bad --match accepted")
	}
	if _, err := ReplayRuleSave(context.Background(), map[string]string{"name": "x", "money": "sideways"}); err == nil {
		t.Fatal("bad --money accepted")
	}
	if _, err := ReplayRuleDelete(context.Background(), map[string]string{}); err == nil {
		t.Fatal("delete without --id accepted")
	}
	if _, err := ReplayRuleDelete(context.Background(), map[string]string{"id": "abc"}); err == nil {
		t.Fatal("non-integer --id accepted")
	}
	if len((*calls)["save"])+len((*calls)["delete"]) != 0 {
		t.Fatal("validation failures still posted")
	}
}
