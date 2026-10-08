package client

import (
	"context"
	"testing"
)

func TestHydratedRulePreviewNeverSaves(t *testing.T) {
	saveUsableURIHost(t)
	srv, calls := ruleServer(t, `{"rules":[{"id":7,"ruleName":"Software","editSequence":3,"ruleOrder":2,"conditionList":{"isAndRule":true,"ruleConditions":[{"ruleType":10,"value":"-1"},{"ruleType":6,"value":"SoftwareVendor"}]},"actionList":{"ruleActions":[{"actionType":8,"value":true}]}}]}`)
	interceptHTTP(t, srv.URL)
	plan, err := ReplayRulePreview(context.Background(), map[string]string{"id": "7", "match": "any"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.MayBroaden || plan.MatchingPopulationVerified {
		t.Fatal("preview overstated safety")
	}
	if len((*calls)["save"]) != 0 {
		t.Fatal("preview submitted a save")
	}
	if plan.Planned.EditSequence != 3 || len(plan.Planned.ConditionList.RuleConditions) != 2 {
		t.Fatal("plan not hydrated")
	}
}
