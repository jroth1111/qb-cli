package client

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRuleBankTextExclusionCreate(t *testing.T) {
	body, err := buildRuleBody(-1, 0, 0, map[string]string{
		"name": "Binge fee-safe", "money": "out", "match": "all",
		"bank-text": "BINGE", "bank-text-exclude": "FEE", "account-ids": "204",
		"category-id": "37", "class-id": "3700000000001146662", "auto-add": "false",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []olbRuleCondition{
		{RuleType: ruleTypeMoneyFlow, Value: "-1"},
		{RuleType: ruleTypeBankContains, Value: "BINGE"},
		{RuleType: ruleTypeBankNotContains, Value: "FEE"},
		{RuleType: ruleTypeInAccount, Value: "204"},
	}
	if !body.ConditionList.IsAndRule || !reflect.DeepEqual(body.ConditionList.RuleConditions, want) {
		t.Fatalf("conditions = %+v", body.ConditionList)
	}
	if len(body.ActionList.RuleActions) != 2 || body.ActionList.RuleActions[0].ActionType != ruleActionCategory || body.ActionList.RuleActions[1].ActionType != ruleActionClass {
		t.Fatalf("actions = %+v", body.ActionList.RuleActions)
	}
}

func TestRuleTwoBankTextExclusionsForImportedFeeRows(t *testing.T) {
	body, err := buildRuleBody(-1, 0, 0, map[string]string{
		"name": "AppSumo future only", "bank-text": "APPSUMO", "bank-text-exclude": "FEE",
		"bank-text-exclude-2": "[Coles CS", "account-ids": "204,93", "category-id": "90",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []olbRuleCondition{
		{RuleType: ruleTypeMoneyFlow, Value: "-1"},
		{RuleType: ruleTypeBankContains, Value: "APPSUMO"},
		{RuleType: ruleTypeBankNotContains, Value: "FEE"},
		{RuleType: ruleTypeBankNotContains, Value: "[Coles CS"},
		{RuleType: ruleTypeInAccount, Value: "204,93"},
	}
	if !reflect.DeepEqual(body.ConditionList.RuleConditions, want) {
		t.Fatalf("conditions = %+v", body.ConditionList.RuleConditions)
	}
	if _, err := buildRuleBody(-1, 0, 0, map[string]string{
		"name": "broad", "bank-text": "APPSUMO", "bank-text-exclude-2": "[Coles CS",
	}, nil); err == nil {
		t.Fatal("second exclusion should require first exclusion")
	}
}

func TestRuleBankTextExclusionUpdatePreservesScope(t *testing.T) {
	var existing rawBankRule
	if err := json.Unmarshal([]byte(`{"ruleName":"Binge","conditionList":{"isAndRule":true,"ruleConditions":[{"ruleType":10,"value":"-1"},{"ruleType":6,"value":"BINGE"},{"ruleType":11,"value":"204"}]},"actionList":{"ruleActions":[{"actionType":0,"value":"37"},{"actionType":2,"value":"Maggie"}]}}`), &existing); err != nil {
		t.Fatal(err)
	}
	body, err := buildRuleBody(9, 2, 3, map[string]string{"bank-text-exclude": "FEE"}, &existing)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]olbRuleCondition(nil), existing.ConditionList.RuleConditions...), olbRuleCondition{RuleType: ruleTypeBankNotContains, Value: "FEE"})
	if !reflect.DeepEqual(body.ConditionList.RuleConditions, want) || !reflect.DeepEqual(body.ActionList.RuleActions, existing.ActionList.RuleActions) {
		t.Fatalf("update lost existing scope/actions: %+v", body)
	}
	if len(existing.ConditionList.RuleConditions) != 3 {
		t.Fatal("input rule mutated")
	}
}

func TestRuleBankTextExclusionRejectsBroadMatching(t *testing.T) {
	for _, flags := range []map[string]string{
		{"name": "x", "bank-text-exclude": "FEE"},
		{"name": "x", "bank-text": "BINGE", "bank-text-exclude": "FEE", "match": "any"},
		{"name": "x", "bank-text": "BINGE", "bank-text-op": "does_not_contain", "bank-text-exclude": "FEE"},
	} {
		if _, err := buildRuleBody(-1, 0, 0, flags, nil); err == nil || !strings.Contains(err.Error(), "bank-text-exclude") {
			t.Fatalf("expected safe exclusion rejection for %+v; got %v", flags, err)
		}
	}
}
