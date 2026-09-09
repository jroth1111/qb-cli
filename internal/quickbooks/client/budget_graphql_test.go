package client

import "testing"

func TestBudgetGraphQLInputMonthlyPnL(t *testing.T) {
	in := budgetGraphQLInput(map[string]string{
		"name":        "QB-CLI-TEST-BUDGET-EMPIRICAL",
		"fiscal-year": "2026",
		"type":        "pnl",
		"interval":    "monthly",
	})
	if in["budgetName"] != "QB-CLI-TEST-BUDGET-EMPIRICAL" {
		t.Fatalf("name=%v", in["budgetName"])
	}
	if in["budgetType"] != "PROFIT_AND_LOSS" {
		t.Fatalf("type=%v", in["budgetType"])
	}
	if in["startDate"] != "2026-07-01" || in["endDate"] != "2027-06-30" {
		t.Fatalf("dates %v %v", in["startDate"], in["endDate"])
	}
	if in["intervalType"] != "MONTHLY" {
		t.Fatalf("interval=%v", in["intervalType"])
	}
	if in["secondaryListType"] != "NOT_SUBDIVIDED" {
		t.Fatalf("sub=%v", in["secondaryListType"])
	}
}
