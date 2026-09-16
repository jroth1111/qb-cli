package client

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestPlanCategoriseEmptyIDs(t *testing.T) {
	start := time.Now()
	_, err := PlanCategorise("209", nil, TransactionDetail{CategoryRef: &RefValue{Value: "7"}})
	assertFast(t, time.Since(start))
	if !errors.Is(err, ErrEmptyCategoriseIDs) {
		t.Fatalf("got %v, want ErrEmptyCategoriseIDs", err)
	}
}

func TestPlanCategoriseEmptyCategory(t *testing.T) {
	_, err := PlanCategorise("209", []string{"3"}, TransactionDetail{})
	if !errors.Is(err, ErrEmptyCategory) {
		t.Fatalf("got %v, want ErrEmptyCategory", err)
	}
}

// TestPlanCategoriseRejectsClass asserts a --class value is refused at plan
// time: the captured batchAcceptTransactions detail carries categoryId only,
// so the class field name cannot be guessed onto the wire.
func TestPlanCategoriseRejectsClass(t *testing.T) {
	_, err := PlanCategorise("209", []string{"3"}, TransactionDetail{
		CategoryRef: &RefValue{Value: "7"},
		ClassRef:    &RefValue{Value: "800398"},
	})
	if !errors.Is(err, ErrUnsupportedClass) {
		t.Fatalf("got %v, want ErrUnsupportedClass", err)
	}
	// An explicitly empty class ref remains tolerated (same as omitting it).
	if _, err := PlanCategorise("209", []string{"3"}, TransactionDetail{
		CategoryRef: &RefValue{Value: "7"},
		ClassRef:    &RefValue{Value: ""},
	}); err != nil {
		t.Fatalf("empty class ref should be ignored, got %v", err)
	}
}

func TestPlanCategoriseNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	plan, err := PlanCategorise("209", []string{"3"}, TransactionDetail{
		CategoryRef: &RefValue{Value: "7"},
	})
	assertFast(t, time.Since(start))
	if err != nil {
		t.Fatalf("PlanCategorise must not need credentials: %v", err)
	}
	assertPlanSafe(t, plan, "batchAcceptTransactions?acceptOnly=true")
	var body struct {
		NextTxnInfo struct {
			AccountID string `json:"accountId"`
			Index     int    `json:"nextTransactionIndex"`
			Review    string `json:"reviewState"`
			Sort      string `json:"sort"`
		} `json:"nextTxnInfo"`
		TxnList struct {
			OlbTxns []struct {
				OlbTxnID   string `json:"olbTxnId"`
				AcceptType string `json:"acceptType"`
				Add        struct {
					Details []struct {
						CategoryID string `json:"categoryId"`
					} `json:"details"`
				} `json:"addAsQboTxn"`
			} `json:"olbTxns"`
		} `json:"txnList"`
	}
	if err := json.Unmarshal(plan.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body.NextTxnInfo.AccountID != "209" || body.NextTxnInfo.Index != -1 ||
		body.NextTxnInfo.Review != "PENDING" || body.NextTxnInfo.Sort != "-txnDate" {
		t.Fatalf("nextTxnInfo = %+v", body.NextTxnInfo)
	}
	if len(body.TxnList.OlbTxns) != 1 {
		t.Fatalf("olbTxns = %+v, want 1 entry", body.TxnList.OlbTxns)
	}
	row := body.TxnList.OlbTxns[0]
	if row.OlbTxnID != "3" || row.AcceptType != "ADD" {
		t.Fatalf("olbTxns[0] = %+v, want stub olbTxnId=3 acceptType=ADD", row)
	}
	if len(row.Add.Details) != 1 || row.Add.Details[0].CategoryID != "7" {
		t.Fatalf("addAsQboTxn.details = %+v, want [0].categoryId=7", row.Add.Details)
	}
}

func TestPlanMatchEmptyIDs(t *testing.T) {
	_, err := PlanMatch("209", nil, []MatchTxn{{TxnID: "123"}})
	if !errors.Is(err, ErrEmptyMatchIDs) {
		t.Fatalf("got %v, want ErrEmptyMatchIDs", err)
	}
}

func TestPlanMatchEmptyMatchTxns(t *testing.T) {
	_, err := PlanMatch("209", []string{"3"}, nil)
	if !errors.Is(err, ErrEmptyMatchTxns) {
		t.Fatalf("got %v, want ErrEmptyMatchTxns", err)
	}
}

func TestPlanMatchNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	plan, err := PlanMatch("209", []string{"3"}, []MatchTxn{{TxnID: "123", TxnType: "Bill"}})
	assertFast(t, time.Since(start))
	if err != nil {
		t.Fatalf("PlanMatch must not need credentials: %v", err)
	}
	assertPlanSafe(t, plan, "acceptTransactions")
	var body struct {
		OlbTxns []struct {
			AcceptType string `json:"acceptType"`
			Selected   struct {
				Matched []struct {
					QboTxnID string `json:"qboTxnId"`
				} `json:"matchedTxns"`
			} `json:"selectedMatches"`
		} `json:"olbTxns"`
	}
	if err := json.Unmarshal(plan.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if len(body.OlbTxns) != 1 || body.OlbTxns[0].AcceptType != "MATCH" {
		t.Fatalf("olbTxns = %+v, want 1 MATCH entry", body.OlbTxns)
	}
	matched := body.OlbTxns[0].Selected.Matched
	if len(matched) != 1 || matched[0].QboTxnID != "123" {
		t.Fatalf("matchedTxns = %+v, want qboTxnId 123", matched)
	}
}

func TestPlanBatchAcceptEmptyIDs(t *testing.T) {
	_, err := PlanBatchAccept("209", nil)
	if !errors.Is(err, ErrEmptyBatchAcceptIDs) {
		t.Fatalf("got %v, want ErrEmptyBatchAcceptIDs", err)
	}
}

func TestPlanBatchAcceptNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	plan, err := PlanBatchAccept("209", []string{"3", "4"})
	assertFast(t, time.Since(start))
	if err != nil {
		t.Fatalf("PlanBatchAccept must not need credentials: %v", err)
	}
	assertPlanSafe(t, plan, "batchAcceptTransactions?acceptOnly=true")
	var body struct {
		TxnList struct {
			OlbTxns []struct {
				OlbTxnID   string `json:"olbTxnId"`
				AcceptType string `json:"acceptType"`
			} `json:"olbTxns"`
		} `json:"txnList"`
	}
	if err := json.Unmarshal(plan.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if len(body.TxnList.OlbTxns) != 2 {
		t.Fatalf("olbTxns = %+v, want 2 stubs", body.TxnList.OlbTxns)
	}
	for i, want := range []string{"3", "4"} {
		if body.TxnList.OlbTxns[i].OlbTxnID != want || body.TxnList.OlbTxns[i].AcceptType != "ADD" {
			t.Fatalf("olbTxns[%d] = %+v, want %s/ADD", i, body.TxnList.OlbTxns[i], want)
		}
	}
}

func TestPlanSplitEmptyIDs(t *testing.T) {
	_, err := PlanSplit("209", nil, "Expense", []SplitLine{
		{CategoryRef: RefValue{Value: "7"}, Amount: -6},
		{CategoryRef: RefValue{Value: "8"}, Amount: -4},
	})
	if !errors.Is(err, ErrEmptySplitIDs) {
		t.Fatalf("got %v, want ErrEmptySplitIDs", err)
	}
}

func TestPlanSplitTooFewLines(t *testing.T) {
	_, err := PlanSplit("209", []string{"3"}, "Expense", []SplitLine{
		{CategoryRef: RefValue{Value: "7"}, Amount: -10},
	})
	if !errors.Is(err, ErrEmptySplitLines) {
		t.Fatalf("got %v, want ErrEmptySplitLines", err)
	}
}

func TestPlanSplitNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	plan, err := PlanSplit("209", []string{"3"}, "Expense", []SplitLine{
		{CategoryRef: RefValue{Value: "7"}, Amount: -6},
		{CategoryRef: RefValue{Value: "8"}, Amount: -4},
	})
	assertFast(t, time.Since(start))
	if err != nil {
		t.Fatalf("PlanSplit must not need credentials: %v", err)
	}
	assertPlanSafe(t, plan, "batchAcceptTransactions?acceptOnly=true")
	var body struct {
		TxnList struct {
			OlbTxns []struct {
				Add struct {
					Details []struct {
						CategoryID string `json:"categoryId"`
						Amount     string `json:"amount"`
					} `json:"details"`
				} `json:"addAsQboTxn"`
			} `json:"olbTxns"`
		} `json:"txnList"`
	}
	if err := json.Unmarshal(plan.Body, &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if len(body.TxnList.OlbTxns) != 1 {
		t.Fatalf("olbTxns = %+v, want 1 stub", body.TxnList.OlbTxns)
	}
	details := body.TxnList.OlbTxns[0].Add.Details
	if len(details) != 2 {
		t.Fatalf("details = %+v, want 2 lines", details)
	}
	if details[0].CategoryID != "7" || details[0].Amount != "6" ||
		details[1].CategoryID != "8" || details[1].Amount != "4" {
		t.Fatalf("details = %+v, want magnitudes 7=6 / 8=4", details)
	}
}
