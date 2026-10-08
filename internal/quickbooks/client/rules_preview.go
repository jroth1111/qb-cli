package client

import (
	"context"
	"fmt"
	"strconv"
)

// RulePreview is a hydrated read-only plan, not a native matching simulation.
type RulePreview struct {
	Before                     *rawBankRule `json:"before"`
	Planned                    *olbRuleBody `json:"planned"`
	MayBroaden                 bool         `json:"mayBroaden"`
	MatchingPopulationVerified bool         `json:"matchingPopulationVerified"`
}

func ReplayRulePreview(ctx context.Context, flags map[string]string) (*RulePreview, error) {
	id, err := strconv.Atoi(firstFlag(flags, "id"))
	if err != nil {
		return nil, fmt.Errorf("preview requires an integer --id")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	before, err := neoFindRule(ctx, ac, strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	if before == nil {
		return nil, fmt.Errorf("rule not found; no plan built")
	}
	copied := make(map[string]string, len(flags)+1)
	for k, v := range flags {
		copied[k] = v
	}
	// Preview may show a risky proposed body but never authorizes its submission.
	copied["allow-broadened-auto-post"] = "true"
	body, err := buildRuleBody(id, atoiOrZero(before.EditSequence.String()), atoiOrZero(before.RuleOrder.String()), copied, before)
	if err != nil {
		return nil, err
	}
	return &RulePreview{Before: before, Planned: body, MayBroaden: !ruleConditionsNarrowed(before, body)}, nil
}
