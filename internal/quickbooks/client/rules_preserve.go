package client

import (
	"strconv"
	"strings"
)

func projectedRuleAutoAdd(r rawBankRule) *bool {
	if r.AutoAdd != nil {
		return r.AutoAdd
	}
	if len(r.ActionList.RuleActions) == 0 {
		return nil
	}
	value := false
	for _, a := range r.ActionList.RuleActions {
		if a.ActionType != ruleActionAutoAdd {
			continue
		}
		switch v := a.Value.(type) {
		case bool:
			value = v
		case string:
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				return nil
			}
			value = parsed
		default:
			return nil
		}
	}
	return &value
}

// Updating one action must retain the rule's other accounting instructions.
func mergeRuleActions(existing *rawBankRule, updates []olbRuleAction, flags map[string]string) []olbRuleAction {
	if existing == nil {
		return updates
	}
	disabled := map[int]bool{}
	if strings.EqualFold(strings.TrimSpace(firstFlag(flags, "class-id")), "none") {
		disabled[ruleActionClass] = true
	}
	for name, kind := range map[string]int{"auto-add": ruleActionAutoAdd, "exclude": ruleActionExclude, "disabled": ruleActionRuleOff} {
		switch firstFlag(flags, name) {
		case "false", "0", "no":
			disabled[kind] = true
		}
	}
	out := make([]olbRuleAction, 0, len(existing.ActionList.RuleActions))
	for _, action := range existing.ActionList.RuleActions {
		if !disabled[action.ActionType] {
			out = append(out, action)
		}
	}
	for _, update := range updates {
		found := false
		for i := range out {
			if out[i].ActionType == update.ActionType {
				out[i] = update
				found = true
				break
			}
		}
		if !found {
			out = append(out, update)
		}
	}
	return out
}
