package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// BankRule is the secret-free projection of a bank rule.
type BankRule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	AutoAdd *bool  `json:"autoAdd,omitempty"`
}

// RulesResult is the JSON envelope returned by ReplayRules.
type RulesResult struct {
	Status int            `json:"status"`
	Counts map[string]int `json:"counts"`
	Rules  []BankRule     `json:"rules"`
}

type rawBankRule struct {
	ID           flexibleString `json:"id"`
	RuleID       flexibleString `json:"ruleId"`
	Name         string         `json:"name"`
	Title        string         `json:"title"`
	RuleName     string         `json:"ruleName"`
	AutoAdd      *bool          `json:"autoAdd"`
	Active       *bool          `json:"active"`
	EditSequence flexibleString `json:"editSequence"`
	RuleOrder    flexibleString `json:"ruleOrder"`
}

// ReplayRules GETs lists/olbrules/getRules (captured 2026-08-17).
// Unknown shapes return status + byte count and an empty list — never the raw body.
func ReplayRules(ctx context.Context, limit int) (*RulesResult, error) {
	if limit < 1 {
		limit = 20
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	url := strings.ReplaceAll(PlannedRulesURL(), realmToken, ac.realm)
	xRange := fmt.Sprintf("items=0-%d", limit-1)
	resp, err := ac.get(ctx, url, xRange)
	if err != nil {
		return nil, fmt.Errorf("getRules: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading getRules: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	raw, ok := extractRules(body)
	if !ok {
		return &RulesResult{
			Status: resp.StatusCode,
			Counts: map[string]int{"bytes": len(body)},
			Rules:  []BankRule{},
		}, nil
	}
	if len(raw) > limit {
		raw = raw[:limit]
	}
	return projectRules(raw), nil
}

func extractRules(body []byte) ([]rawBankRule, bool) {
	var arr []rawBankRule
	if json.Unmarshal(body, &arr) == nil && arr != nil {
		return arr, true
	}
	var wrap struct {
		Rules []rawBankRule `json:"rules"`
		Items []rawBankRule `json:"items"`
	}
	if json.Unmarshal(body, &wrap) != nil {
		return nil, false
	}
	if wrap.Rules != nil {
		return wrap.Rules, true
	}
	if wrap.Items != nil {
		return wrap.Items, true
	}
	return nil, false
}

func projectRules(in []rawBankRule) *RulesResult {
	out := make([]BankRule, 0, len(in))
	for _, r := range in {
		id := r.ID.String()
		if id == "" {
			id = r.RuleID.String()
		}
		name := r.Name
		if name == "" {
			name = r.Title
		}
		if name == "" {
			name = r.RuleName
		}
		auto := r.AutoAdd
		if auto == nil {
			auto = r.Active
		}
		out = append(out, BankRule{ID: id, Name: name, AutoAdd: auto})
	}

	return &RulesResult{
		Status: http.StatusOK,
		Counts: map[string]int{"rules": len(out)},
		Rules:  out,
	}
}

// Rule mutations ride the neo lists service: POST lists/olbrules/save is the
// upsert (id=-1 creates; id=<ruleId> edits) and POST lists/olbrules/delete
// removes by {id}. Contract captured from the olbrules widget
// (integrations_datain_ui): the drawer view-model serializes to
// {id, ruleName, ruleOrder, editSequence, conditionList:{isAndRule,
// ruleConditions:[{ruleType,value}]}, actionList:{ruleActions:
// [{actionType,value}]}}. Live-proven on Test Company 2 (2026-09-16):
// ruleType 10=MONEY_FLOW ("1" in / "-1" out), 6=ANY_TEXT_MATCH_CONTAINS,
// 11=IN_ACCOUNT; actionType 0=CATEGORY. The TRANSACTION_TYPE action (7)
// validates only in the drawer's full action set and is not exposed.

const (
	ruleTypeDescExact       = 0
	ruleTypeDescContains    = 1
	ruleTypeAmountEquals    = 2
	ruleTypeAmountGreater   = 3
	ruleTypeAmountLess      = 4
	ruleTypeBankContains    = 6
	ruleTypeAmountNotEqual  = 7
	ruleTypeBankNotContains = 8
	ruleTypeDescNotContains = 9
	ruleTypeMoneyFlow       = 10
	ruleTypeInAccount       = 11
	ruleTypeBankExact       = 13
)

const (
	ruleActionCategory = 0
	ruleActionMemo     = 1
	ruleActionClass    = 2
	ruleActionLocation = 3
	ruleActionPayee    = 5
	ruleActionAutoAdd  = 8
	ruleActionRuleOff  = 10
	ruleActionExclude  = 12
	ruleActionCustomer = 14
)

type olbRuleCondition struct {
	RuleType int    `json:"ruleType"`
	Value    string `json:"value"`
}

type olbRuleAction struct {
	ActionType int `json:"actionType"`
	Value      any `json:"value"`
}

type olbRuleBody struct {
	ID            int    `json:"id"`
	RuleName      string `json:"ruleName"`
	RuleOrder     int    `json:"ruleOrder"`
	EditSequence  int    `json:"editSequence"`
	ConditionList struct {
		IsAndRule      bool               `json:"isAndRule"`
		RuleConditions []olbRuleCondition `json:"ruleConditions"`
	} `json:"conditionList"`
	ActionList struct {
		RuleActions []olbRuleAction `json:"ruleActions"`
	} `json:"actionList"`
}

// ruleConditionSet builds the wire conditionList from flags. Money-flow comes
// first (the serializer emits it unconditionally); row conditions follow in
// flag order; IN_ACCOUNT is appended only when a subset of accounts is chosen.
func ruleConditionSet(flags map[string]string) (bool, []olbRuleCondition, error) {
	isAnd := firstFlag(flags, "match") != "any"
	if m := firstFlag(flags, "match"); m != "" && m != "all" && m != "any" {
		return false, nil, fmt.Errorf("--match must be all or any")
	}
	conds := []olbRuleCondition{}
	money := firstFlag(flags, "money", "direction")
	switch money {
	case "", "out":
		conds = append(conds, olbRuleCondition{RuleType: ruleTypeMoneyFlow, Value: "-1"})
	case "in":
		conds = append(conds, olbRuleCondition{RuleType: ruleTypeMoneyFlow, Value: "1"})
	default:
		return false, nil, fmt.Errorf("--money must be in or out")
	}
	textRuleTypes := func(op string, contains, exact, notContains int) (int, error) {
		switch op {
		case "", "contains":
			return contains, nil
		case "is_exactly", "exact":
			return exact, nil
		case "does_not_contain":
			return notContains, nil
		}
		return 0, fmt.Errorf("unsupported condition operator %q", op)
	}
	if v := firstFlag(flags, "bank-text"); v != "" {
		rt, err := textRuleTypes(firstFlag(flags, "bank-text-op"), ruleTypeBankContains, ruleTypeBankExact, ruleTypeBankNotContains)
		if err != nil {
			return false, nil, err
		}
		conds = append(conds, olbRuleCondition{RuleType: rt, Value: v})
	}
	if v := firstFlag(flags, "description"); v != "" {
		rt, err := textRuleTypes(firstFlag(flags, "description-op"), ruleTypeDescContains, ruleTypeDescExact, ruleTypeDescNotContains)
		if err != nil {
			return false, nil, err
		}
		conds = append(conds, olbRuleCondition{RuleType: rt, Value: v})
	}
	if v := firstFlag(flags, "amount"); v != "" {
		var rt int
		switch op := firstFlag(flags, "amount-op"); op {
		case "", "equals":
			rt = ruleTypeAmountEquals
		case "does_not_equal", "not_equal":
			rt = ruleTypeAmountNotEqual
		case "is_greater_than", "greater_than":
			rt = ruleTypeAmountGreater
		case "is_less_than", "less_than":
			rt = ruleTypeAmountLess
		default:
			return false, nil, fmt.Errorf("unsupported --amount-op %q", op)
		}
		conds = append(conds, olbRuleCondition{RuleType: rt, Value: v})
	}
	if v := firstFlag(flags, "account-ids"); v != "" {
		conds = append(conds, olbRuleCondition{RuleType: ruleTypeInAccount, Value: v})
	}
	return isAnd, conds, nil
}

// ruleActionSet builds the wire actionList from flags.
func ruleActionSet(flags map[string]string) ([]olbRuleAction, error) {
	acts := []olbRuleAction{}
	addStr := func(actionType int, keys ...string) {
		if v := firstFlag(flags, keys...); v != "" {
			acts = append(acts, olbRuleAction{ActionType: actionType, Value: v})
		}
	}
	addStr(ruleActionCategory, "category-id", "category")
	addStr(ruleActionPayee, "payee-id", "payee")
	addStr(ruleActionMemo, "memo")
	addStr(ruleActionClass, "class-id")
	addStr(ruleActionLocation, "location-id")
	addStr(ruleActionCustomer, "customer-id")
	truthy := func(v string) bool { return v == "true" || v == "1" || v == "yes" }
	if truthy(firstFlag(flags, "auto-add")) {
		acts = append(acts, olbRuleAction{ActionType: ruleActionAutoAdd, Value: true})
	}
	if truthy(firstFlag(flags, "exclude")) {
		acts = append(acts, olbRuleAction{ActionType: ruleActionExclude, Value: true})
	}
	if truthy(firstFlag(flags, "disabled")) {
		acts = append(acts, olbRuleAction{ActionType: ruleActionRuleOff, Value: true})
	}
	return acts, nil
}

// buildRuleBody assembles the save body. id=-1 creates; a fetched rule
// supplies editSequence/ruleOrder (and the name fallback) on edit.
func buildRuleBody(id, editSequence, ruleOrder int, flags map[string]string, fallbackName string) (*olbRuleBody, error) {
	name := strings.TrimSpace(firstFlag(flags, "name", "rule-name"))
	if name == "" {
		name = fallbackName
	}
	if name == "" {
		return nil, fmt.Errorf("rule requires --name")
	}
	isAnd, conds, err := ruleConditionSet(flags)
	if err != nil {
		return nil, err
	}
	if len(conds) < 1 || len(conds) > 5 {
		return nil, fmt.Errorf("a rule should have at least 1 condition and a maximum of 5 conditions")
	}
	acts, err := ruleActionSet(flags)
	if err != nil {
		return nil, err
	}
	body := &olbRuleBody{ID: id, RuleName: name, RuleOrder: ruleOrder, EditSequence: editSequence}
	body.ConditionList.IsAndRule = isAnd
	body.ConditionList.RuleConditions = conds
	body.ActionList.RuleActions = acts
	return body, nil
}

// ReplayRuleSave creates or updates a bank rule via POST lists/olbrules/save.
// On edit (--id), the existing rule is fetched for its editSequence/ruleOrder.
func ReplayRuleSave(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	base := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/"
	id := -1
	editSeq, order := 0, 0
	existingName := ""
	rawID := strings.TrimSpace(firstFlag(flags, "id"))
	if rawID != "" {
		n, err := strconv.Atoi(rawID)
		if err != nil {
			return nil, fmt.Errorf("--id must be an integer rule id")
		}
		id = n
		found, err := neoFindRule(ctx, ac, base, rawID)
		if err != nil {
			return nil, err
		}
		if found == nil {
			return nil, fmt.Errorf("rule %s not found", rawID)
		}
		editSeq = atoiOrZero(found.EditSequence.String())
		order = atoiOrZero(found.RuleOrder.String())
		existingName = found.RuleName
		if existingName == "" {
			existingName = found.Name
		}
	}
	body, err := buildRuleBody(id, editSeq, order, flags, existingName)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, base+"lists/olbrules/save", "qbo.intuit.com", raw)
	if err != nil {
		return nil, fmt.Errorf("olbrules save: %w", err)
	}
	sraw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("olbrules save: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(sraw)}
	}
	var out struct {
		OlbRule struct {
			ID       flexibleString `json:"id"`
			RuleName string         `json:"ruleName"`
		} `json:"olbRule"`
	}
	_ = json.Unmarshal(sraw, &out)
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     map[bool]string{true: "update", false: "create"}[id >= 0],
		Entity: "OlbRule",
		Note:   "neo lists/olbrules/save",
		Item:   QueryItem{ID: out.OlbRule.ID.String(), Name: out.OlbRule.RuleName},
	}, nil
}

// ReplayRuleDelete removes a bank rule via POST lists/olbrules/delete {id}.
func ReplayRuleDelete(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	rawID := strings.TrimSpace(firstFlag(flags, "id"))
	if rawID == "" {
		return nil, fmt.Errorf("rule delete requires --id")
	}
	n, err := strconv.Atoi(rawID)
	if err != nil {
		return nil, fmt.Errorf("--id must be an integer rule id")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	base := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/"
	raw, err := json.Marshal(map[string]int{"id": n})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, base+"lists/olbrules/delete", "qbo.intuit.com", raw)
	if err != nil {
		return nil, fmt.Errorf("olbrules delete: %w", err)
	}
	sraw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("olbrules delete: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(sraw)}
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "delete",
		Entity: "OlbRule",
		Note:   "neo lists/olbrules/delete",
		Item:   QueryItem{ID: rawID},
	}, nil
}

// neoFindRule fetches getRules and returns the rule matching id, or nil.
func neoFindRule(ctx context.Context, ac *apiClient, base, id string) (*rawBankRule, error) {
	resp, err := ac.doURIHost(ctx, http.MethodPost, base+"lists/olbrules/getRules", "qbo.intuit.com", []byte("{}"))
	if err != nil {
		return nil, fmt.Errorf("getRules: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("getRules: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Rules []rawBankRule `json:"rules"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("getRules parse: %w", err)
	}
	for i := range wrap.Rules {
		if wrap.Rules[i].ID.String() == id {
			return &wrap.Rules[i], nil
		}
	}
	return nil, nil
}

func atoiOrZero(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// PlannedRuleSaveURL is the dry-run target for rule create/update.
func PlannedRuleSaveURL() string {
	return "https://qbo.intuit.com/api/neo/v1/company/{realm}/lists/olbrules/save"
}

// PlannedRuleDeleteURL is the dry-run target for rule delete.
func PlannedRuleDeleteURL() string {
	return "https://qbo.intuit.com/api/neo/v1/company/{realm}/lists/olbrules/delete"
}
