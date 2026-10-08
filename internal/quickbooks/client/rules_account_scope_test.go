package client

import "testing"

func TestAccountScopeUpdatePreservesMerchantConditions(t *testing.T) {
	existing := &rawBankRule{RuleName: "Workspace"}
	existing.ConditionList.IsAndRule = true
	existing.ConditionList.RuleConditions = []olbRuleCondition{
		{RuleType: ruleTypeMoneyFlow, Value: "-1"},
		{RuleType: ruleTypeBankContains, Value: "Workspace"},
		{RuleType: ruleTypeBankContains, Value: "Google"},
		{RuleType: ruleTypeInAccount, Value: "204,93"},
	}
	body, err := buildRuleBody(217, 1, 1, map[string]string{"account-ids": "204,93,209"}, existing)
	if err != nil {
		t.Fatal(err)
	}
	conditions := body.ConditionList.RuleConditions
	if !body.ConditionList.IsAndRule || len(conditions) != 4 {
		t.Fatalf("criteria changed: %#v", body.ConditionList)
	}
	for i := range 3 {
		if conditions[i] != existing.ConditionList.RuleConditions[i] {
			t.Fatalf("merchant or direction condition changed: %#v", conditions)
		}
	}
	if conditions[3].RuleType != ruleTypeInAccount || conditions[3].Value != "204,93,209" {
		t.Fatalf("scope not updated: %#v", conditions)
	}
}
