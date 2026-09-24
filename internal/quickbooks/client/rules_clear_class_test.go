package client

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuleClearClassPreservesOtherActionsAndConditions(t *testing.T) {
	var old rawBankRule
	if err := json.Unmarshal([]byte(`{"id":116,"ruleName":"gsuite","conditionList":{"isAndRule":true,"ruleConditions":[{"ruleType":10,"value":"-1"},{"ruleType":6,"value":"Google Workspace"}]},"actionList":{"ruleActions":[{"actionType":0,"value":"90"},{"actionType":2,"value":"old-class"},{"actionType":8,"value":true}]}}`), &old); err != nil {
		t.Fatal(err)
	}
	body, err := buildRuleBody(116, 5, 91, map[string]string{"class-id": "none"}, &old)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body.ConditionList.RuleConditions, old.ConditionList.RuleConditions) {
		t.Fatal("conditions changed")
	}
	if len(body.ActionList.RuleActions) != 2 {
		t.Fatalf("unexpected actions: %#v", body.ActionList.RuleActions)
	}
	for _, action := range body.ActionList.RuleActions {
		if action.ActionType == ruleActionClass {
			t.Fatal("class action retained")
		}
	}
	if len(old.ActionList.RuleActions) != 3 {
		t.Fatal("input changed")
	}
}
