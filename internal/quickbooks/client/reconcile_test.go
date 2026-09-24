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

// reconcileTestServer discriminates by request body: the finish flow reads
// the Integration_Reconciliation node first (query w/node), then POSTs
// updateIntegration_Reconciliation.
type reconcileTestServer struct {
	*httptest.Server
	mu      sync.Mutex
	bodies  [][]byte
	inProg  bool
	adjNode string
}

func newReconcileTestServer(t *testing.T, inProgress bool) *reconcileTestServer {
	t.Helper()
	s := &reconcileTestServer{inProg: inProgress, adjNode: "djQuMTo6MTIzNDU6NTFjZWQ4NTM2Yw:74"}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, b)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(b), "updateIntegration_Reconciliation") {
			var req struct {
				Variables struct {
					Input struct {
						Rec struct {
							ID string `json:"id"`
						} `json:"integrationReconciliation"`
					} `json:"input_0"`
				} `json:"variables"`
			}
			_ = json.Unmarshal(b, &req)
			_, _ = w.Write([]byte(`{"data":{"updateIntegration_Reconciliation":{"clientMutationId":"qb-cli","integrationReconciliation":{"id":"` + req.Variables.Input.Rec.ID + `","inProgress":false,"statementEndingDate":null,"statementEndingBalance":null}}}}`))
			return
		}
		node := `{"data":{"node":{"id":"djQuMTo6MTIzNDU6Y2VjNGZjZTgyZg:44","__typename":"Integration_Reconciliation","inProgress":` +
			map[bool]string{true: "true", false: "false"}[s.inProg] +
			`,"statementEndingDate":"2026-09-19","adjustingEntryDefaultAccount":{"id":"` + s.adjNode + `","name":"Reconciliation Discrepancies"}}}}`
		_, _ = w.Write([]byte(node))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *reconcileTestServer) mutationBody(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.bodies {
		if strings.Contains(string(b), "updateIntegration_Reconciliation") {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("decoding mutation body: %v", err)
			}
			return m
		}
	}
	t.Fatal("no updateIntegration_Reconciliation call recorded")
	return nil
}

func finishRec(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	vars, _ := m["variables"].(map[string]any)
	in, _ := vars["input_0"].(map[string]any)
	rec, _ := in["integrationReconciliation"].(map[string]any)
	if rec == nil {
		t.Fatalf("missing integrationReconciliation in %v", m)
	}
	return rec
}

func TestReconcileFinishFlow(t *testing.T) {
	saveUsableURIHost(t)
	srv := newReconcileTestServer(t, true)
	interceptHTTP(t, srv.URL)

	res, err := ReplayReconcileFinish(context.Background(), "44", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "finish" || res.Status != http.StatusOK {
		t.Fatalf("got status=%d op=%s", res.Status, res.Op)
	}
	rec := finishRec(t, srv.mutationBody(t))
	if rec["reconcileAction"] != "FINISH" {
		t.Errorf("reconcileAction = %v, want FINISH", rec["reconcileAction"])
	}
	// Defaults resolved from the node read: statement date + default
	// adjusting account node id.
	if rec["effectiveStatementEndingDate"] != "2026-09-19" {
		t.Errorf("effectiveStatementEndingDate = %v", rec["effectiveStatementEndingDate"])
	}
	if rec["adjustingEntryDate"] != "2026-09-19" {
		t.Errorf("adjustingEntryDate = %v", rec["adjustingEntryDate"])
	}
	acct, _ := rec["adjustingEntryAccount"].(map[string]any)
	if acct["id"] != "djQuMTo6MTIzNDU6NTFjZWQ4NTM2Yw:74" {
		t.Errorf("adjustingEntryAccount = %v", acct)
	}
	if rec["adjustingEntryExchangeRate"] != 1.0 {
		t.Errorf("adjustingEntryExchangeRate = %v", rec["adjustingEntryExchangeRate"])
	}
	if rec["id"] != "djQuMToxMjM0NTpjZWM0ZmNlODJm:44" {
		t.Errorf("reconciliation node id = %v", rec["id"])
	}
}

func TestReconcileFinishExplicitFields(t *testing.T) {
	saveUsableURIHost(t)
	srv := newReconcileTestServer(t, true)
	interceptHTTP(t, srv.URL)

	// Bare v3 account id converts to a Relay node id; explicit dates win.
	res, err := ReplayReconcileFinish(context.Background(), "44", "2026-09-20", "74", "1.5", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	rec := finishRec(t, srv.mutationBody(t))
	acct, _ := rec["adjustingEntryAccount"].(map[string]any)
	if !strings.HasSuffix(acct["id"].(string), ":74") {
		t.Errorf("adjustingEntryAccount id = %v, want ...:74 node", acct["id"])
	}
	if rec["adjustingEntryDate"] != "2026-09-20" || rec["effectiveStatementEndingDate"] != "2026-09-30" {
		t.Errorf("dates = %v / %v", rec["adjustingEntryDate"], rec["effectiveStatementEndingDate"])
	}
	if rec["adjustingEntryExchangeRate"] != 1.5 {
		t.Errorf("exchange rate = %v, want 1.5", rec["adjustingEntryExchangeRate"])
	}
	_ = res
}

func TestReconcileFinishNoSession(t *testing.T) {
	saveUsableURIHost(t)
	srv := newReconcileTestServer(t, false)
	interceptHTTP(t, srv.URL)

	_, err := ReplayReconcileFinish(context.Background(), "44", "", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "no open session") {
		t.Fatalf("want no-open-session error, got %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, b := range srv.bodies {
		if strings.Contains(string(b), "updateIntegration_Reconciliation") {
			t.Fatal("mutation must not fire without an open session")
		}
	}
}

func TestReconcileFinishValidation(t *testing.T) {
	saveUsableURIHost(t)
	srv := newReconcileTestServer(t, true)
	interceptHTTP(t, srv.URL)

	if _, err := ReplayReconcileFinish(context.Background(), "", "", "", "", ""); err == nil {
		t.Error("empty account id should error before network")
	}
	if _, err := ReplayReconcileFinish(context.Background(), "44", "", "", "abc", ""); err == nil {
		t.Error("bad exchange rate should error")
	}
	// The old session path must still refuse FINISH.
	if _, err := ReplayReconcileSession(context.Background(), "FINISH", "44", "", ""); err == nil {
		t.Error("ReplayReconcileSession should refuse FINISH")
	}
}

func TestPlanReconcileFinish(t *testing.T) {
	plan, err := PlanReconcileFinish("44", "2026-09-19", "74", "", "2026-09-19")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || plan.Dial {
		t.Error("plan must be dry-run, no dial")
	}
	if !strings.Contains(plan.URL, "qbonline-aws.api.intuit.com") {
		t.Errorf("plan URL = %s", plan.URL)
	}
	var m map[string]any
	if err := json.Unmarshal(plan.Body, &m); err != nil {
		t.Fatal(err)
	}
	rec := finishRec(t, m)
	if rec["reconcileAction"] != "FINISH" {
		t.Errorf("reconcileAction = %v", rec["reconcileAction"])
	}
	if rec["adjustingEntryExchangeRate"] != 1.0 {
		t.Errorf("default exchange rate = %v", rec["adjustingEntryExchangeRate"])
	}
	if _, err := PlanReconcileFinish("", "", "", "", ""); err == nil {
		t.Error("plan without account id should error")
	}
	// Without node defaults and no --adjust-account, the plan cannot shape
	// the entry — it must error rather than invent one.
	if _, err := PlanReconcileFinish("44", "", "", "", "2026-09-19"); err == nil {
		t.Error("plan without adjusting account should error")
	}
}

func TestAccountNodeID(t *testing.T) {
	got := accountNodeID("9341457769756854", "74")
	if !strings.HasSuffix(got, ":74") {
		t.Fatalf("accountNodeID = %q, want suffix :74", got)
	}
	// Same derivation the live node read reported for account 74 on TC2.
	want := "djQuMTo5MzQxNDU3NzY5NzU2ODU0OjUxY2VkODUzNmM:74"
	if got != want {
		t.Errorf("accountNodeID = %q, want %q", got, want)
	}
}

func TestReconcileBeginRejectsWrongActionState(t *testing.T) {
	saveUsableURIHost(t)
	srv := newReconcileTestServer(t, false)
	interceptHTTP(t, srv.URL)
	_, err := ReplayReconcileSession(context.Background(), "BEGIN", "45", "100.00", "2026-09-30")
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}

func TestReconcileCancelRejectsMissingStateOrWrongID(t *testing.T) {
	for name, receipt := range map[string]string{
		"missing-state": `{"data":{"updateIntegration_Reconciliation":{"integrationReconciliation":{"id":"any"}}}}`,
		"wrong-id":      `{"data":{"updateIntegration_Reconciliation":{"integrationReconciliation":{"id":"wrong","inProgress":false}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			saveUsableURIHost(t)
			srv := newDomServer(t, http.StatusOK, receipt)
			interceptHTTP(t, srv.URL)
			_, err := ReplayReconcileSession(context.Background(), "CANCEL", "45", "", "")
			if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
