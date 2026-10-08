package client

import (
	"fmt"
	"reflect"
)

// patchRuleConditions never infers deletion from an omitted flag. Ambiguous
// repeated selectors require an explicit complete replacement.
func patchRuleConditions(existing *rawBankRule, flags map[string]string) (bool, []olbRuleCondition, error) {
	isAnd := existing.ConditionList.IsAndRule
	if m := firstFlag(flags, "match"); m != "" {
		if m != "all" && m != "any" {
			return false, nil, fmt.Errorf("--match must be all or any")
		}
		isAnd = m == "all"
	}
	result := append([]olbRuleCondition(nil), existing.ConditionList.RuleConditions...)
	groups := []struct {
		value, op string
		types     []int
	}{
		{"money", "", []int{ruleTypeMoneyFlow}},
		{"account-ids", "", []int{ruleTypeInAccount}},
		{"bank-text", "bank-text-op", []int{ruleTypeBankContains, ruleTypeBankExact}},
		{"description", "description-op", []int{ruleTypeDescContains, ruleTypeDescExact}},
		{"amount", "amount-op", []int{ruleTypeAmountEquals, ruleTypeAmountGreater, ruleTypeAmountLess, ruleTypeAmountNotEqual}},
	}
	for _, g := range groups {
		value := firstFlag(flags, g.value)
		if g.value == "money" {
			value = firstFlag(flags, "money", "direction")
		}
		op := firstFlag(flags, g.op)
		if value == "" && op == "" {
			continue
		}
		indexes := []int{}
		for i, c := range result {
			for _, kind := range g.types {
				if c.RuleType == kind {
					indexes = append(indexes, i)
				}
			}
		}
		if len(indexes) > 1 {
			return false, nil, fmt.Errorf("ambiguous %s selectors: use --replace-conditions true with complete criteria", g.value)
		}
		if value == "" {
			if len(indexes) != 1 {
				return false, nil, fmt.Errorf("--%s requires an existing unique --%s value", g.op, g.value)
			}
			value = result[indexes[0]].Value
		}
		patch := map[string]string{g.value: value}
		if op != "" {
			patch[g.op] = op
		} else if len(indexes) == 1 {
			switch result[indexes[0]].RuleType {
			case ruleTypeBankExact, ruleTypeDescExact:
				patch[g.op] = "is_exactly"
			case ruleTypeAmountGreater:
				patch[g.op] = "is_greater_than"
			case ruleTypeAmountLess:
				patch[g.op] = "is_less_than"
			case ruleTypeAmountNotEqual:
				patch[g.op] = "does_not_equal"
			}
		}
		_, generated, err := ruleConditionSet(patch)
		if err != nil {
			return false, nil, err
		}
		var replacement olbRuleCondition
		found := false
		for _, c := range generated {
			if g.value == "money" || c.RuleType != ruleTypeMoneyFlow {
				replacement = c
				found = true
			}
		}
		if !found {
			return false, nil, fmt.Errorf("missing replacement for %s", g.value)
		}
		if len(indexes) == 1 {
			result[indexes[0]] = replacement
		} else {
			result = append(result, replacement)
		}
	}
	for _, key := range []string{"bank-text-exclude", "bank-text-exclude-2"} {
		if value := firstFlag(flags, key); value != "" {
			if key == "bank-text-exclude-2" {
				hasFirst := false
				for _, c := range result {
					if c.RuleType == ruleTypeBankNotContains {
						hasFirst = true
					}
				}
				if !hasFirst {
					return false, nil, fmt.Errorf("--bank-text-exclude-2 requires a first exclusion")
				}
			}
			positive := false
			for _, c := range result {
				if c.RuleType == ruleTypeBankContains || c.RuleType == ruleTypeBankExact {
					positive = true
				}
			}
			if !isAnd || !positive {
				return false, nil, fmt.Errorf("--%s requires an ALL rule with positive bank text", key)
			}
			added := olbRuleCondition{RuleType: ruleTypeBankNotContains, Value: value}
			present := false
			for _, c := range result {
				if c == added {
					present = true
				}
			}
			if !present {
				result = append(result, added)
			}
		}
	}
	if len(result) < 1 || len(result) > 5 {
		return false, nil, fmt.Errorf("a rule requires 1 to 5 conditions")
	}
	return isAnd, result, nil
}

// Without a native matching-population proof, only exact preservation or an
// additional ALL constraint is accepted as demonstrably non-broadening.
func ruleConditionsNarrowed(existing *rawBankRule, body *olbRuleBody) bool {
	if existing.ConditionList.IsAndRule != body.ConditionList.IsAndRule {
		return false
	}
	if !body.ConditionList.IsAndRule {
		return reflect.DeepEqual(existing.ConditionList.RuleConditions, body.ConditionList.RuleConditions)
	}
	for _, old := range existing.ConditionList.RuleConditions {
		found := false
		for _, current := range body.ConditionList.RuleConditions {
			if current == old {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
