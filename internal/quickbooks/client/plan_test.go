package client

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// These tests reject 5 wrong builders:
// 1. empty exclude ids accepted
// 2. import to 204/93 accepted
// 3. plan URL embeds a live realm
// 4. plan requires credentials (would block offline dry-run)
// 5. plan.Dial == true
func TestPlanExcludeEmptyIDs(t *testing.T) {
	start := time.Now()
	_, err := PlanExclude("209", nil)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrEmptyExcludeIDs) {
		t.Fatalf("got %v, want ErrEmptyExcludeIDs", err)
	}
}

func TestPlanExcludeNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	plan, err := PlanExclude("209", []string{"olb-1"})
	assertFast(t, time.Since(start))
	if err != nil {
		t.Fatalf("PlanExclude must not need credentials: %v", err)
	}
	assertPlanSafe(t, plan, "excludeTransactions")
}

func TestPlanUndoEmptyIDs(t *testing.T) {
	_, err := PlanUndo("209", nil)
	if !errors.Is(err, ErrEmptyUndoIDs) {
		t.Fatalf("got %v, want ErrEmptyUndoIDs", err)
	}
}

func TestFeedMutationPlansMatchLiveUIExcludeUndoContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		plan   func() (*RequestPlan, error)
		review string
	}{
		{"exclude", func() (*RequestPlan, error) { return PlanExclude("44", []string{"2"}) }, "PENDING"},
		{"undo", func() (*RequestPlan, error) { return PlanUndo("44", []string{"3"}) }, "EXCLUDED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := tc.plan()
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				NextTxnInfo struct {
					AccountID string `json:"accountId"`
					Index     int    `json:"nextTransactionIndex"`
					Review    string `json:"reviewState"`
				} `json:"nextTxnInfo"`
				TxnIDList struct {
					Pairs []any `json:"txnIdPairs"`
				} `json:"txnIdList"`
			}
			if err := json.Unmarshal(plan.Body, &body); err != nil {
				t.Fatal(err)
			}
			if body.NextTxnInfo.AccountID != "44" || body.NextTxnInfo.Index != -1 || body.NextTxnInfo.Review != tc.review {
				t.Fatalf("nextTxnInfo=%+v", body.NextTxnInfo)
			}
			if body.TxnIDList.Pairs == nil || len(body.TxnIDList.Pairs) != 0 {
				t.Fatalf("txnIdPairs=%v, want an explicit empty array", body.TxnIDList.Pairs)
			}
		})
	}
}

func TestPlanImportBlockedLiveAccounts(t *testing.T) {
	for _, id := range []string{"204", "93"} {
		_, _, err := PlanImportCSV(id, "17/08/2026", "QB-CLI-TEST", "0.01")
		if !errors.Is(err, ErrImportBlockedAccount) {
			t.Fatalf("account %s: got %v, want ErrImportBlockedAccount", id, err)
		}
	}
}

// TestUploadCSVFileRejectsEmptyInput proves the multipart builder fails
// input-class before any session load or dial: empty filename and empty
// content must never open a socket.
func TestUploadCSVFileRejectsEmptyInput(t *testing.T) {
	ctx := context.Background()
	if _, err := UploadCSVFile(ctx, "", []byte("a")); err == nil {
		t.Fatal("empty filename accepted")
	}
	if _, err := UploadCSVFile(ctx, "a.csv", nil); err == nil {
		t.Fatal("empty content accepted")
	}
}

func TestPlanImportCSVNoCreds(t *testing.T) {
	noCredsDir(t)
	plan, meta, err := PlanImportCSV("209", "17/08/2026", "QB-CLI-TEST", "0.01")
	if err != nil {
		t.Fatalf("PlanImportCSV must not need credentials: %v", err)
	}
	if meta.AccountID != "209" || meta.Description != "QB-CLI-TEST" {
		t.Fatalf("meta = %+v", meta)
	}
	assertPlanSafe(t, plan, "processCsvFile")
}

func TestPlannedURLsUseRealmToken(t *testing.T) {
	for _, u := range []string{
		PlannedAccountsURL(),
		PlannedFeedURL("204", "PENDING"),
		PlannedRegisterURL("204"),
		PlannedRulesURL(),
	} {
		if !strings.Contains(u, "{realm}") {
			t.Errorf("url %q missing {realm}", u)
		}
		if strings.Contains(u, "99999") {
			t.Errorf("url %q leaked live realm", u)
		}
	}
	if PlannedAuditURL() != auditLogsURL {
		t.Fatalf("audit url %q", PlannedAuditURL())
	}
}

func TestExtractRulesUnknownShape(t *testing.T) {
	if _, ok := extractRules([]byte(`{"nope":true}`)); ok {
		t.Fatal("unknown object must not extract as rules")
	}
}

func TestExtractRulesItems(t *testing.T) {
	raw, ok := extractRules([]byte(`{"rules":[{"id":"r1","name":"Telstra"}]}`))
	if !ok || len(raw) != 1 || raw[0].Name != "Telstra" {
		t.Fatalf("got ok=%v raw=%+v", ok, raw)
	}
}

func TestExtractRulesNumericID(t *testing.T) {
	raw, ok := extractRules([]byte(`[{"id":12,"ruleName":"Telstra"}]`))
	if !ok || len(raw) != 1 || raw[0].ID.String() != "12" || raw[0].RuleName != "Telstra" {
		t.Fatalf("got ok=%v raw=%+v", ok, raw)
	}
	got := projectRules(raw)
	if len(got.Rules) != 1 || got.Rules[0].ID != "12" || got.Rules[0].Name != "Telstra" {
		t.Fatalf("projected %+v", got.Rules)
	}
}

func TestExtractRegisterNumericTxnID(t *testing.T) {
	raw, ok := extractRegisterTxns([]byte(`[{"txnId":99,"date":"2026-08-17","amount":1.5,"memo":"x"}]`))
	if !ok || len(raw) != 1 || raw[0].TxnID.String() != "99" {
		t.Fatalf("got ok=%v raw=%+v", ok, raw)
	}
	got := projectRegister(raw)
	if len(got.Transactions) != 1 || got.Transactions[0].ID != "99" || got.Transactions[0].Amount != 1.5 {
		t.Fatalf("projected %+v", got.Transactions)
	}
}

func TestReplayRulesNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayRules(t.Context(), 20)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func TestReplayAuditLogNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	_, err := ReplayAuditLog(t.Context(), 20)
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("got %v, want ErrNoCredentials", err)
	}
}

func assertPlanSafe(t *testing.T, plan *RequestPlan, wantPath string) {
	t.Helper()
	if plan == nil {
		t.Fatal("nil plan")
	}
	if !plan.DryRun || plan.Dial {
		t.Fatalf("plan dry_run=%v dial=%v", plan.DryRun, plan.Dial)
	}
	if plan.Method != "POST" {
		t.Fatalf("method %q", plan.Method)
	}
	if !strings.Contains(plan.URL, "{realm}") || !strings.Contains(plan.URL, wantPath) {
		t.Fatalf("url %q want {realm} and %s", plan.URL, wantPath)
	}
	if strings.Contains(string(plan.Body), "intuit_apikey") || strings.Contains(plan.URL, "99999") {
		t.Fatalf("plan leaked secret: url=%s body=%s", plan.URL, plan.Body)
	}
	var probe any
	if err := json.Unmarshal(plan.Body, &probe); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
}
