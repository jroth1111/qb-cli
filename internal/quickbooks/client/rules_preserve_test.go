package client

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuleAutoAddUpdatePreservesAccountsAndClasses(t *testing.T) {
	var existing rawBankRule
	if err := json.Unmarshal([]byte(`{"id":116,"ruleName":"example","conditionList":{"isAndRule":true,"ruleConditions":[{"ruleType":10,"value":"-1"},{"ruleType":1,"value":"Merchant"}]},"actionList":{"ruleActions":[{"actionType":0,"value":"90"},{"actionType":2,"value":"class-1"},{"actionType":8,"value":true}]}}`), &existing); err != nil {
		t.Fatal(err)
	}
	body, err := buildRuleBody(116, 2, 91, map[string]string{"auto-add": "false"}, &existing)
	if err != nil {
		t.Fatal(err)
	}
	want := []olbRuleAction{{ActionType: 0, Value: "90"}, {ActionType: 2, Value: "class-1"}}
	if !reflect.DeepEqual(body.ActionList.RuleActions, want) {
		t.Fatalf("lost accounting actions: %+v", body.ActionList.RuleActions)
	}
	if existing.ActionList.RuleActions[2].Value != true {
		t.Fatal("input rule mutated")
	}
	if !reflect.DeepEqual(body.ConditionList.RuleConditions, existing.ConditionList.RuleConditions) {
		t.Fatal("matching criteria changed")
	}
}
func TestRuleActiveIsNotAutoAdd(t *testing.T) {
	yes := true
	r := rawBankRule{Active: &yes}
	if projectedRuleAutoAdd(r) != nil {
		t.Fatal("active was treated as auto-add")
	}
	r.ActionList.RuleActions = []olbRuleAction{{ActionType: 0, Value: "90"}, {ActionType: 8, Value: true}}
	if got := projectedRuleAutoAdd(r); got == nil || !*got {
		t.Fatal("nested auto-add lost")
	}
}
