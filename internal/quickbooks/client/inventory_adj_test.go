package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- inventory_adj test seam ------------------------------------------------
//
// invPostV3 builds its URLs from https://qbo.intuit.com/api/v3/company/%s/...
// exactly like ReplayMutate, so the same DefaultTransport swap used by
// interceptHTTP routes production URLs to a local httptest server. The swap
// is saved/restored via t.Cleanup; nothing in this file touches production
// files or reaches the real network.

// invAdjServer captures one POST and replies with the given status/body.
type invAdjServer struct {
	t          *testing.T
	status     int
	respBody   string
	mu         chan struct{}
	lastPath   string
	lastMethod string
	lastBody   []byte
}

func newInvAdjServer(t *testing.T, status int, respBody string) *invAdjServer {
	return &invAdjServer{t: t, status: status, respBody: respBody, mu: make(chan struct{}, 1)}
}

func (s *invAdjServer) handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(w http.ResponseWriter, r *http.Request) {
		s.mu <- struct{}{}
		s.lastPath = r.URL.Path
		s.lastMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		s.lastBody = b
		<-s.mu
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.respBody))
	}
	for _, p := range []string{"/api/v3/company/12345/inventory-counts", "/api/v3/company/12345/buildassembly", "/api/v3/company/12345/inventory_starting_value"} {
		mux.HandleFunc(p, handle)
	}
	return mux
}

func (s *invAdjServer) start(t *testing.T) string {
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

func (s *invAdjServer) calls() (path, method string, body []byte) {
	return s.lastPath, s.lastMethod, s.lastBody
}

// countResponseShape is the minimal QueryResponse envelope projectMutateItem
// needs to surface an id.
const countResponseShape = `{"InventoryCount":{"Id":"177","SyncToken":"0","TxnDate":"2026-08-24"}}`

func TestReplayInventoryCountCreatePostsCountedQtyLine(t *testing.T) {
	saveUsable(t)
	fs := newInvAdjServer(t, http.StatusOK, countResponseShape)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayInventoryCountCreate(context.Background(), map[string]string{
		"item": "42", "counted-qty": "7",
	})
	if err != nil {
		t.Fatalf("ReplayInventoryCountCreate: %v", err)
	}
	if res.Status != http.StatusOK || res.Op != "create" || res.Entity != InventoryCountEntity {
		t.Fatalf("envelope = %+v", res)
	}
	if res.Item.ID != "177" {
		t.Fatalf("item id = %q, want 177", res.Item.ID)
	}
	path, method, body := fs.calls()
	if method != http.MethodPost {
		t.Fatalf("method = %s, want POST", method)
	}
	if !strings.HasSuffix(path, "/inventory-counts") {
		t.Fatalf("path = %q, want suffix /inventory-counts", path)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, body)
	}
	lines, _ := got["Line"].([]any)
	if len(lines) != 1 {
		t.Fatalf("lines = %v, want 1", got["Line"])
	}
	line := lines[0].(map[string]any)
	det := line["InventoryCountLineDetail"].(map[string]any)
	if det["ItemRef"].(map[string]any)["value"] != "42" {
		t.Fatalf("ItemRef = %v", det["ItemRef"])
	}
	if det["CountedQty"] != 7.0 {
		t.Fatalf("CountedQty = %v, want 7", det["CountedQty"])
	}
}

func TestReplayBuildAssemblyCreatePostsItemRefAndQty(t *testing.T) {
	saveUsable(t)
	fs := newInvAdjServer(t, http.StatusOK, `{"BuildAssembly":{"Id":"201","SyncToken":"0"}}`)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayBuildAssemblyCreate(context.Background(), map[string]string{
		"item": "88", "qty": "5",
	})
	if err != nil {
		t.Fatalf("ReplayBuildAssemblyCreate: %v", err)
	}
	if res.Entity != BuildAssemblyEntity || res.Item.ID != "201" {
		t.Fatalf("res = %+v", res)
	}
	_, _, body := fs.calls()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if got["ItemRef"].(map[string]any)["value"] != "88" {
		t.Fatalf("ItemRef = %v", got["ItemRef"])
	}
	if got["Qty"] != 5.0 {
		t.Fatalf("Qty = %v, want 5", got["Qty"])
	}
}

func TestReplayStartingValueCreatePostsQtyAndCost(t *testing.T) {
	saveUsable(t)
	fs := newInvAdjServer(t, http.StatusOK, `{"StartingValue":{"Id":"209","SyncToken":"0"}}`)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayStartingValueCreate(context.Background(), map[string]string{
		"item": "31", "qty": "12", "cost": "3.75", "asset-account": "81",
	})
	if err != nil {
		t.Fatalf("ReplayStartingValueCreate: %v", err)
	}
	if res.Entity != StartingValueEntity || res.Item.ID != "209" {
		t.Fatalf("res = %+v", res)
	}
	_, _, body := fs.calls()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if got["Qty"] != 12.0 || got["Cost"] != 3.75 {
		t.Fatalf("Qty/Cost = %v/%v, want 12/3.75", got["Qty"], got["Cost"])
	}
	if got["AssetAccountRef"].(map[string]any)["value"] != "81" {
		t.Fatalf("AssetAccountRef = %v", got["AssetAccountRef"])
	}
}

// --- non-200 surfaces as ReplayError ----------------------------------------

func TestReplayInventoryCreateSurfacesReplayError(t *testing.T) {
	saveUsable(t)
	fs := newInvAdjServer(t, http.StatusBadRequest, `{"Fault":{"Error":[{"Message":"bad shape"}]}}`)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayInventoryCountCreate(context.Background(), map[string]string{
		"item": "42", "counted-qty": "1",
	})
	var re *ReplayError
	if err == nil || res != nil || !errors.As(err, &re) || re.Status != http.StatusBadRequest {
		t.Fatalf("err=%v res=%+v, want ReplayError(400) and nil result", err, res)
	}
}

// --- validation negatives (no network possible: no server attached) ---------

func TestInventoryAdjBuildersRejectMissingInput(t *testing.T) {
	cases := []struct {
		name  string
		call  func() (*MutateResult, error)
		wantS string
	}{
		{"count missing item", func() (*MutateResult, error) {
			return ReplayInventoryCountCreate(context.Background(), map[string]string{"counted-qty": "3"})
		}, "--item"},
		{"count missing qty", func() (*MutateResult, error) {
			return ReplayInventoryCountCreate(context.Background(), map[string]string{"item": "1"})
		}, "--counted-qty"},
		{"assembly missing item", func() (*MutateResult, error) {
			return ReplayBuildAssemblyCreate(context.Background(), map[string]string{"qty": "2"})
		}, "--item"},
		{"assembly zero qty", func() (*MutateResult, error) {
			return ReplayBuildAssemblyCreate(context.Background(), map[string]string{"item": "2", "qty": "0"})
		}, "--qty"},
		{"starting value missing item", func() (*MutateResult, error) {
			return ReplayStartingValueCreate(context.Background(), map[string]string{"qty": "2"})
		}, "--item"},
		{"starting value all zeros", func() (*MutateResult, error) {
			return ReplayStartingValueCreate(context.Background(), map[string]string{"item": "3", "qty": "", "cost": ""})
		}, "--qty or --cost"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.call()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if res != nil {
				t.Fatalf("expected nil result on validation failure, got %+v", res)
			}
			if !strings.Contains(err.Error(), tc.wantS) {
				t.Fatalf("error %q does not mention %q", err, tc.wantS)
			}
		})
	}
}

// --- no-credentials fast-fail ------------------------------------------------

func TestInventoryAdjCreatesFailFastOnNoCredentials(t *testing.T) {
	cases := []struct {
		name string
		call func() (*MutateResult, error)
	}{
		{"count", func() (*MutateResult, error) {
			return ReplayInventoryCountCreate(context.Background(), map[string]string{"item": "1", "counted-qty": "1"})
		}},
		{"assembly", func() (*MutateResult, error) {
			return ReplayBuildAssemblyCreate(context.Background(), map[string]string{"item": "1", "qty": "1"})
		}},
		{"starting value", func() (*MutateResult, error) {
			return ReplayStartingValueCreate(context.Background(), map[string]string{"item": "1", "qty": "1"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			noCredsDir(t)
			start := time.Now()
			res, err := tc.call()
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("expected ErrNoCredentials, got nil")
			}
			if !errors.Is(err, ErrNoCredentials) {
				t.Fatalf("got %v, want ErrNoCredentials", err)
			}
			if res != nil {
				t.Fatalf("expected nil result without creds, got %+v", res)
			}
			assertFast(t, elapsed)
		})
	}
}

// --- planned URL never embeds a live realm -----------------------------------

func TestPlannedInventoryV3URLUsesRealmToken(t *testing.T) {
	u := PlannedInventoryV3URL(InventoryCountPath)
	if strings.Contains(u, "12345") {
		t.Fatalf("planned URL leaks realm: %s", u)
	}
	for _, part := range []string{"https://qbo.intuit.com/api/v3/company/{realm}/inventory-counts?minorversion=73"} {
		if u != part {
			t.Fatalf("planned URL = %q, want %q", u, part)
		}
	}
}
