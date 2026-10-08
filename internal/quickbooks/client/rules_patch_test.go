package client

import "testing"

func patchFixture() *rawBankRule {
	r := &rawBankRule{RuleName: "Software"}
	r.ConditionList.IsAndRule = true
	r.ConditionList.RuleConditions = []olbRuleCondition{
		{RuleType: ruleTypeMoneyFlow, Value: "-1"},
		{RuleType: ruleTypeBankContains, Value: "SoftwareVendor"},
		{RuleType: ruleTypeInAccount, Value: "10"},
	}
	r.ActionList.RuleActions = []olbRuleAction{{ActionType: ruleActionAutoAdd, Value: true}}
	return r
}

func TestPartialRulePreservesUnspecifiedSelectors(t *testing.T) {
	cases := []map[string]string{
		{"money": "in"}, {"match": "any"}, {"amount": "12"},
		{"bank-text-op": "is_exactly"}, {"account-ids": "20"},
		{"description": "Subscription"}, {"bank-text-exclude": "FEE"},
	}
	for _, flags := range cases {
		t.Run(firstNonemptyKey(flags), func(t *testing.T) {
			original := patchFixture()
			flags["auto-add"] = "false"
			body, err := buildRuleBody(1, 1, 1, flags, original)
			if err != nil {
				t.Fatal(err)
			}
			for _, old := range original.ConditionList.RuleConditions {
				replaced := old.RuleType == ruleTypeMoneyFlow && flags["money"] != "" ||
					old.RuleType == ruleTypeInAccount && flags["account-ids"] != "" ||
					old.RuleType == ruleTypeBankContains && flags["bank-text-op"] != ""
				if replaced {
					continue
				}
				found := false
				for _, c := range body.ConditionList.RuleConditions {
					if c == old {
						found = true
					}
				}
				if !found {
					t.Fatalf("lost unspecified selector %#v", old)
				}
			}
		})
	}
}
func firstNonemptyKey(flags map[string]string) string {
	for k := range flags {
		return k
	}
	return ""
}

func TestRuleBroadeningRequiresAcknowledgment(t *testing.T) {
	for _, flags := range []map[string]string{
		{"money": "in"}, {"match": "any"}, {"account-ids": "10,20"},
		{"replace-conditions": "true", "money": "out"},
	} {
		if _, err := buildRuleBody(1, 1, 1, flags, patchFixture()); err == nil {
			t.Fatalf("accepted risky flags %v", flags)
		}
		flags["allow-broadened-auto-post"] = "true"
		if _, err := buildRuleBody(1, 1, 1, flags, patchFixture()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRulePatchFailsClosedOnAmbiguousOperators(t *testing.T) {
	r := patchFixture()
	r.ConditionList.RuleConditions = append(r.ConditionList.RuleConditions, olbRuleCondition{RuleType: ruleTypeBankContains, Value: "SecondSelector"})
	if _, err := buildRuleBody(1, 1, 1, map[string]string{"bank-text-op": "is_exactly"}, r); err == nil {
		t.Fatal("ambiguous selectors accepted")
	}
}

func TestRootAutoAddCannotBypassBroadeningGuard(t *testing.T) {
	r := patchFixture()
	enabled := true
	r.AutoAdd = &enabled
	r.ActionList.RuleActions = nil
	if _, err := buildRuleBody(1, 1, 1, map[string]string{"match": "any"}, r); err == nil {
		t.Fatal("root autoAdd bypassed guard")
	}
}
