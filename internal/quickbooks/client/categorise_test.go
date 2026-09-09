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

func TestPlanCategoriseNoCreds(t *testing.T) {
	noCredsDir(t)
	start := time.Now()
	plan, err := PlanCategorise("209", []string{"3"}, TransactionDetail{
		CategoryRef: &RefValue{Value: "7"},
		ClassRef:    &RefValue{Value: "800398"},
	})
	assertFast(t, time.Since(start))
	if err != nil {
		t.Fatalf("PlanCategorise must not need credentials: %v", err)
	}
	assertPlanSafe(t, plan, "categoriseTransactions")
	// Body must contain the category and class refs.
	var probe map[string]any
	if err := json.Unmarshal(plan.Body, &probe); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	detail, ok := probe["transactionDetail"].(map[string]any)
	if !ok {
		t.Fatal("missing transactionDetail")
	}
	if detail["entityType"] != "Expense" {
		t.Fatalf("entityType = %v, want Expense", detail["entityType"])
	}
	catRef, ok := detail["categoryRef"].(map[string]any)
	if !ok || catRef["value"] != "7" {
		t.Fatalf("categoryRef = %+v, want value 7", detail["categoryRef"])
	}
	classRef, ok := detail["classRef"].(map[string]any)
	if !ok || classRef["value"] != "800398" {
		t.Fatalf("classRef = %+v, want value 800398", detail["classRef"])
	}
}

func TestPlanMatchEmptyIDs(t *testing.T) {
	_, err := PlanMatch("209", nil, []MatchTxn{{TxnID: "123", TxnType: "Bill"}})
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
	assertPlanSafe(t, plan, "matchTransactions")
	var probe map[string]any
	if err := json.Unmarshal(plan.Body, &probe); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	matches, ok := probe["matchTxns"].([]any)
	if !ok || len(matches) != 1 {
		t.Fatalf("matchTxns = %+v, want 1 entry", probe["matchTxns"])
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
	assertPlanSafe(t, plan, "batchAcceptTransactions")
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
	assertPlanSafe(t, plan, "splitTransactions")
	var probe map[string]any
	if err := json.Unmarshal(plan.Body, &probe); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	detail, ok := probe["transactionDetail"].(map[string]any)
	if !ok {
		t.Fatal("missing transactionDetail")
	}
	lines, ok := detail["lines"].([]any)
	if !ok || len(lines) != 2 {
		t.Fatalf("lines = %+v, want 2", detail["lines"])
	}
}
