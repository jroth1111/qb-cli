package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// CUSTOMERS.PROPOSAL_CREATE/EDIT/DELETE + PROPOSAL_SVC_CREATE —
// crm-proposal-svc.api.intuit.com /v1/proposals, live-proven on TC2
// 2026-09-19 (POST {contactId,contactType} → id:1002, PUT by refId UUID,
// DELETE by numeric id → isDeleted:true, list back to the one original).
// The id spaces are asymmetric: GET/PUT key by refId UUID, DELETE by the
// numeric id — refId on DELETE returns "Expected Integer".

// proposalServer records each request and answers by method + whether the
// trailing path segment looks like a numeric id vs a refId UUID.
type proposalServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    int
	lastMeth string
	lastPath string
	lastBody []byte

	getBody    string
	postBody   string
	putBody    string
	deleteBody string
	status     int
}

func newProposalServer(t *testing.T) *proposalServer {
	t.Helper()
	s := &proposalServer{
		getBody:    `{"id":1002,"refId":"a2675a4f-b098-43af-a12c-f6160c92f6b1","title":"Proposal 1002","status":"DRAFT","contactId":"1","contactType":"CUSTOMER","isDeleted":false}`,
		postBody:   `{"id":1002,"refId":"a2675a4f-b098-43af-a12c-f6160c92f6b1","title":"Proposal 1002","status":"DRAFT","contactId":"1","contactType":"CUSTOMER","isDeleted":false}`,
		putBody:    `{"id":1002,"refId":"a2675a4f-b098-43af-a12c-f6160c92f6b1","title":"Proposal 1002","status":"DRAFT","isDeleted":false}`,
		deleteBody: `{"id":1002,"refId":"a2675a4f-b098-43af-a12c-f6160c92f6b1","title":"Proposal 1002","isDeleted":true}`,
		status:     http.StatusOK,
	}
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
		if s.status != http.StatusOK {
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(s.postBody))
		case http.MethodPut:
			_, _ = w.Write([]byte(s.putBody))
		case http.MethodDelete:
			_, _ = w.Write([]byte(s.deleteBody))
		default:
			_, _ = w.Write([]byte(s.getBody))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *proposalServer) snap() (calls int, meth, path string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.lastMeth, s.lastPath, s.lastBody
}

func TestProposalCreatePostsContactBody(t *testing.T) {
	saveUsableURIHost(t)
	srv := newProposalServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayProposalCreate(context.Background(), "1", "", "My pitch")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Op != "create" || res.Item.ID != "1002" {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(res.Item.Type, "a2675a4f-b098-43af-a12c-f6160c92f6b1") {
		t.Fatalf("refId not surfaced: %q", res.Item.Type)
	}
	_, meth, path, body := srv.snap()
	if meth != http.MethodPost || path != "/v1/proposals" {
		t.Fatalf("request = %s %s", meth, path)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	if m["contactId"] != "1" || m["contactType"] != "CUSTOMER" || m["title"] != "My pitch" {
		t.Fatalf("body = %v", m)
	}
}

func TestProposalCreateRequiresCustomer(t *testing.T) {
	saveUsableURIHost(t)
	srv := newProposalServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayProposalCreate(context.Background(), "", "CUSTOMER", ""); err == nil {
		t.Fatal("expected missing-contact error")
	}
	if calls, _, _, _ := srv.snap(); calls != 0 {
		t.Fatalf("server should not be called, calls=%d", calls)
	}
}

func TestProposalUpdateFetchesThenPutsFullBody(t *testing.T) {
	saveUsableURIHost(t)
	srv := newProposalServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayProposalUpdate(context.Background(), "a2675a4f-b098-43af-a12c-f6160c92f6b1", "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "update" || res.Item.ID != "1002" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, path, body := srv.snap()
	if calls != 2 {
		t.Fatalf("expected GET + PUT, calls=%d", calls)
	}
	if meth != http.MethodPut || path != "/v1/proposals/a2675a4f-b098-43af-a12c-f6160c92f6b1" {
		t.Fatalf("request = %s %s", meth, path)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	// Full object must be resent — not a patch.
	if m["title"] != "Renamed" || m["contactId"] != "1" || m["status"] != "DRAFT" {
		t.Fatalf("PUT body = %v", m)
	}
}

func TestProposalDeleteUsesNumericID(t *testing.T) {
	saveUsableURIHost(t)
	srv := newProposalServer(t)
	interceptHTTP(t, srv.URL)
	res, err := ReplayProposalDelete(context.Background(), "1002")
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "delete" || res.Item.ID != "1002" || res.Item.Type != "Proposal (deleted)" {
		t.Fatalf("res = %+v", res)
	}
	calls, meth, path, _ := srv.snap()
	if calls != 1 || meth != http.MethodDelete || path != "/v1/proposals/1002" {
		t.Fatalf("request = %s %s (calls=%d)", meth, path, calls)
	}
}

func TestProposalDeleteRequiresID(t *testing.T) {
	saveUsableURIHost(t)
	srv := newProposalServer(t)
	interceptHTTP(t, srv.URL)
	if _, err := ReplayProposalDelete(context.Background(), "  "); err == nil {
		t.Fatal("expected missing-id error")
	}
	if calls, _, _, _ := srv.snap(); calls != 0 {
		t.Fatalf("server should not be called, calls=%d", calls)
	}
}

func TestProposalErrorPropagatesStatus(t *testing.T) {
	saveUsableURIHost(t)
	srv := newProposalServer(t)
	srv.status = http.StatusUnprocessableEntity
	interceptHTTP(t, srv.URL)
	_, err := ReplayProposalCreate(context.Background(), "1", "CUSTOMER", "")
	var re *ReplayError
	if err == nil || !errors.As(err, &re) || re.Status != http.StatusUnprocessableEntity {
		t.Fatalf("err = %v", err)
	}
}
