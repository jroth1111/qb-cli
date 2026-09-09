package client

import "testing"

func TestBuildCreateClassRequiresName(t *testing.T) {
	_, err := buildCreateBody("Class", map[string]string{})
	if err == nil {
		t.Fatal("expected Class create to require --name")
	}
	body, err := buildCreateBody("Class", map[string]string{"name": "QB-CLI-TEST-CLASS"})
	if err != nil {
		t.Fatal(err)
	}
	if body["Name"] != "QB-CLI-TEST-CLASS" {
		t.Fatalf("Name=%v", body["Name"])
	}
}

func TestBuildCreateAccountDefaultsExpense(t *testing.T) {
	_, err := buildCreateBody("Account", map[string]string{})
	if err == nil {
		t.Fatal("expected Account create to require --name")
	}
	body, err := buildCreateBody("Account", map[string]string{"name": "QB-CLI-TEST-COA"})
	if err != nil {
		t.Fatal(err)
	}
	if body["Name"] != "QB-CLI-TEST-COA" {
		t.Fatalf("Name=%v", body["Name"])
	}
	if body["AccountType"] != "Expense" {
		t.Fatalf("AccountType=%v want Expense default", body["AccountType"])
	}
	fa, err := buildCreateBody("Account", map[string]string{
		"name": "QB-CLI-TEST-FA", "type": "Fixed Asset", "subtype": "FurnitureAndFixtures",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fa["AccountType"] != "Fixed Asset" {
		t.Fatalf("AccountType=%v", fa["AccountType"])
	}
	if fa["AccountSubType"] != "FurnitureAndFixtures" {
		t.Fatalf("AccountSubType=%v", fa["AccountSubType"])
	}
}

func TestBuildCreateDepositRequiresAccount(t *testing.T) {
	_, err := buildCreateBody("Deposit", map[string]string{"amount": "1"})
	if err == nil {
		t.Fatal("expected Deposit create to require --account")
	}
	body, err := buildCreateBody("Deposit", map[string]string{
		"account": "48", "from-account": "1", "amount": "0.17",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["DepositToAccountRef"].(map[string]any)["value"] != "48" {
		t.Fatalf("DepositToAccountRef=%v", body["DepositToAccountRef"])
	}
	lines := body["Line"].([]map[string]any)
	det := lines[0]["DepositLineDetail"].(map[string]any)
	if det["AccountRef"].(map[string]any)["value"] != "1" {
		t.Fatalf("source AccountRef=%v", det["AccountRef"])
	}
	if lines[0]["Amount"] != 0.17 {
		t.Fatalf("Amount=%v", lines[0]["Amount"])
	}
}

func TestBuildUpdateBudgetSetsName(t *testing.T) {
	existing := map[string]any{
		"Id": "1000000001", "SyncToken": "0",
		"Name": "OLD", "StartDate": "2026-07-01", "EndDate": "2027-06-30",
		"BudgetType": "ProfitAndLoss",
	}
	body, err := buildUpdateBody("Budget", map[string]string{"name": "QB-CLI-TEST-BUDGET-ED"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if body["Id"] != "1000000001" || body["SyncToken"] != "0" {
		t.Fatalf("identity=%v", body)
	}
	if body["Name"] != "QB-CLI-TEST-BUDGET-ED" {
		t.Fatalf("Name=%v", body["Name"])
	}
	if body["sparse"] != true {
		t.Fatalf("sparse=%v", body["sparse"])
	}
}

func TestLeftover17V3Paths(t *testing.T) {
	if _, ok := v3Path["Budget"]; !ok {
		t.Fatal("Budget missing from v3Path")
	}
	if _, ok := v3Path["Class"]; !ok {
		t.Fatal("Class missing from v3Path")
	}
	if _, ok := v3Path["Account"]; !ok {
		t.Fatal("Account missing from v3Path")
	}
	if _, ok := v3Path["Deposit"]; !ok {
		t.Fatal("Deposit missing from v3Path")
	}
}
