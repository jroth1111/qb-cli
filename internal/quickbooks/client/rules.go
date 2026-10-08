package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
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
	ID            flexibleString `json:"id"`
	RuleID        flexibleString `json:"ruleId"`
	Name          string         `json:"name"`
	Title         string         `json:"title"`
	RuleName      string         `json:"ruleName"`
	AutoAdd       *bool          `json:"autoAdd"`
	Active        *bool          `json:"active"`
	EditSequence  flexibleString `json:"editSequence"`
	RuleOrder     flexibleString `json:"ruleOrder"`
	ConditionList struct {
		IsAndRule      bool               `json:"isAndRule"`
		RuleConditions []olbRuleCondition `json:"ruleConditions"`
	} `json:"conditionList"`
	ActionList struct {
		RuleActions []olbRuleAction `json:"ruleActions"`
	} `json:"actionList"`
}

// ReplayRules GETs lists/olbrules/getRules (captured 2026-08-17).
// Unknown shapes return status + byte count and an empty list — never the raw body.
func ReplayRules(ctx context.Context, limit int) (*RulesResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	url := strings.ReplaceAll(PlannedRulesURL(), realmToken, ac.realm)
	// limit <= 0 asks for the open-ended range (items=0-) — the server
	// decides how much it returns rather than a client-imposed window.
	xRange := fmt.Sprintf("items=0-%d", limit-1)
	if limit < 1 {
		xRange = "items=0-"
	}
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
	if limit > 0 && len(raw) > limit {
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
		auto := projectedRuleAutoAdd(r)
		out = append(out, BankRule{ID: id, Name: name, AutoAdd: auto})
	}

	return &RulesResult{
		Status: http.StatusOK,
		Counts: map[string]int{"rules": len(out)},
		Rules:  out,
	}
}

// Rule mutations ride the neo lists service: POST lists/olbrules/save is the
// upsert (id=-1 creates; id=<ruleId> edits) and POST lists/olbrules/batchDelete
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
	if v := firstFlag(flags, "bank-text-exclude"); v != "" {
		if !isAnd {
			return false, nil, fmt.Errorf("--bank-text-exclude requires --match all")
		}
		if firstFlag(flags, "bank-text") == "" || firstFlag(flags, "bank-text-op") == "does_not_contain" {
			return false, nil, fmt.Errorf("--bank-text-exclude requires a positive --bank-text condition")
		}
		conds = append(conds, olbRuleCondition{RuleType: ruleTypeBankNotContains, Value: v})
	}
	if v := firstFlag(flags, "bank-text-exclude-2"); v != "" {
		if !isAnd || firstFlag(flags, "bank-text") == "" || firstFlag(flags, "bank-text-exclude") == "" {
			return false, nil, fmt.Errorf("--bank-text-exclude-2 requires --match all, --bank-text and --bank-text-exclude")
		}
		conds = append(conds, olbRuleCondition{RuleType: ruleTypeBankNotContains, Value: v})
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
	// QBO represents no class by an absent action, not a class named "none".
	if !strings.EqualFold(strings.TrimSpace(firstFlag(flags, "class-id")), "none") {
		addStr(ruleActionClass, "class-id")
	}
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
// supplies editSequence/ruleOrder (and the name fallback) on edit. On edit,
// unspecified conditions/actions carry forward. Complete condition replacement
// requires an explicit flag; a partial update never removes omitted selectors.
func buildRuleBody(id, editSequence, ruleOrder int, flags map[string]string, existing *rawBankRule) (*olbRuleBody, error) {
	for _, key := range []string{"replace-conditions", "allow-broadened-auto-post"} {
		if v := firstFlag(flags, key); v != "" && v != "true" && v != "false" {
			return nil, fmt.Errorf("--%s must be true or false", key)
		}
	}
	fallbackName := ""
	if existing != nil {
		fallbackName = existing.RuleName
		if fallbackName == "" {
			fallbackName = existing.Name
		}
	}
	name := strings.TrimSpace(firstFlag(flags, "name", "rule-name"))
	if name == "" {
		name = fallbackName
	}
	if name == "" {
		return nil, fmt.Errorf("rule requires --name")
	}
	body := &olbRuleBody{ID: id, RuleName: name, RuleOrder: ruleOrder, EditSequence: editSequence}
	if existing != nil && hasConditionFlags(flags) && firstFlag(flags, "replace-conditions") != "true" {
		isAnd, conditions, err := patchRuleConditions(existing, flags)
		if err != nil {
			return nil, err
		}
		body.ConditionList.IsAndRule = isAnd
		body.ConditionList.RuleConditions = conditions
	} else if existing != nil && onlyNewBankTextExclusion(flags) && firstFlag(flags, "replace-conditions") != "true" {
		body.ConditionList.IsAndRule = existing.ConditionList.IsAndRule
		if !body.ConditionList.IsAndRule {
			return nil, fmt.Errorf("--bank-text-exclude requires an ALL rule")
		}
		positive := false
		for _, condition := range existing.ConditionList.RuleConditions {
			if condition.RuleType == ruleTypeBankContains || condition.RuleType == ruleTypeBankExact || condition.RuleType == ruleTypeDescContains || condition.RuleType == ruleTypeDescExact {
				positive = true
			}
		}
		if !positive {
			return nil, fmt.Errorf("--bank-text-exclude requires an existing positive text condition")
		}
		body.ConditionList.RuleConditions = append([]olbRuleCondition(nil), existing.ConditionList.RuleConditions...)
		exclusion := olbRuleCondition{RuleType: ruleTypeBankNotContains, Value: firstFlag(flags, "bank-text-exclude")}
		present := false
		for _, condition := range body.ConditionList.RuleConditions {
			if condition == exclusion {
				present = true
			}
		}
		if !present {
			body.ConditionList.RuleConditions = append(body.ConditionList.RuleConditions, exclusion)
		}
		if len(body.ConditionList.RuleConditions) > 5 {
			return nil, fmt.Errorf("a rule should have a maximum of 5 conditions")
		}
	} else if existing != nil && !hasConditionFlags(flags) && firstFlag(flags, "replace-conditions") != "true" {
		body.ConditionList.IsAndRule = existing.ConditionList.IsAndRule
		body.ConditionList.RuleConditions = existing.ConditionList.RuleConditions
	} else {
		isAnd, conds, err := ruleConditionSet(flags)
		if err != nil {
			return nil, err
		}
		if len(conds) < 1 || len(conds) > 5 {
			return nil, fmt.Errorf("a rule should have at least 1 condition and a maximum of 5 conditions")
		}
		body.ConditionList.IsAndRule = isAnd
		body.ConditionList.RuleConditions = conds
	}
	if existing != nil && !hasActionFlags(flags) {
		body.ActionList.RuleActions = existing.ActionList.RuleActions
	} else {
		acts, err := ruleActionSet(flags)
		if err != nil {
			return nil, err
		}
		body.ActionList.RuleActions = mergeRuleActions(existing, acts, flags)
	}
	if existing != nil {
		effective := *existing
		if firstFlag(flags, "auto-add") != "" {
			effective.AutoAdd = nil
		}
		effective.ActionList.RuleActions = body.ActionList.RuleActions
		auto := projectedRuleAutoAdd(effective)
		if auto != nil && *auto && !ruleConditionsNarrowed(existing, body) && firstFlag(flags, "allow-broadened-auto-post") != "true" {
			return nil, fmt.Errorf("criteria may broaden an auto-post rule; inspect complete criteria and matching population before --allow-broadened-auto-post true")
		}
	}
	return body, nil
}

// hasConditionFlags reports whether any matching-criteria flag was passed.
func hasConditionFlags(flags map[string]string) bool {
	for _, k := range []string{"match", "money", "direction", "bank-text", "bank-text-op",
		"bank-text-exclude", "bank-text-exclude-2", "description", "description-op", "amount", "amount-op", "account-ids"} {
		if firstFlag(flags, k) != "" {
			return true
		}
	}
	return false
}

func onlyNewBankTextExclusion(flags map[string]string) bool {
	if firstFlag(flags, "bank-text-exclude") == "" {
		return false
	}
	for _, k := range []string{"match", "money", "direction", "bank-text", "bank-text-op",
		"bank-text-exclude-2", "description", "description-op", "amount", "amount-op", "account-ids"} {
		if firstFlag(flags, k) != "" {
			return false
		}
	}
	return true
}

// hasActionFlags reports whether any action flag was passed.
func hasActionFlags(flags map[string]string) bool {
	for _, k := range []string{"category-id", "category", "payee-id", "payee", "memo",
		"class-id", "location-id", "customer-id", "auto-add", "exclude", "disabled"} {
		if firstFlag(flags, k) != "" {
			return true
		}
	}
	return false
}

// ReplayRuleSave creates or updates a bank rule via POST lists/olbrules/save.
// On edit (--id), the existing rule is fetched for its editSequence/ruleOrder.
func ReplayRuleSave(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	beforeRules, err := ruleSnapshot(ctx, ac)
	if err != nil {
		return nil, fmt.Errorf("rule preflight snapshot: %w", err)
	}
	base := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/"
	id := -1
	editSeq, order := 0, 0
	var existing *rawBankRule
	rawID := strings.TrimSpace(firstFlag(flags, "id"))
	if rawID != "" {
		n, err := strconv.Atoi(rawID)
		if err != nil {
			return nil, fmt.Errorf("--id must be an integer rule id")
		}
		id = n
		found, err := neoFindRule(ctx, ac, rawID)
		if err != nil {
			return nil, err
		}
		if found == nil {
			return nil, fmt.Errorf("rule %s not found", rawID)
		}
		editSeq = atoiOrZero(found.EditSequence.String())
		order = atoiOrZero(found.RuleOrder.String())
		existing = found
	}
	body, err := buildRuleBody(id, editSeq, order, flags, existing)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	submittingMutation(ctx)
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
	target := out.OlbRule.ID.String()
	if target == "" || rawID != "" && target != rawID {
		return nil, fmt.Errorf("%w: rule receipt identity", ErrMutationUnverified)
	}
	afterRules, err := ruleSnapshot(ctx, ac)
	if err != nil {
		return nil, fmt.Errorf("%w: rule readback: %v", ErrMutationUnverified, err)
	}
	rule, ok := afterRules[target]
	if !ok {
		return nil, fmt.Errorf("%w: saved rule absent", ErrMutationUnverified)
	}
	want := normalizedJSON(body).(map[string]any)
	for _, key := range []string{"id", "editSequence", "ruleOrder"} {
		delete(want, key)
	}
	if err := expectedFields(want, normalizedJSON(rule), "OlbRule"); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMutationUnverified, err)
	}
	wantCount := len(beforeRules)
	if rawID == "" {
		wantCount++
	}
	if len(afterRules) != wantCount {
		return nil, fmt.Errorf("%w: rule population changed", ErrMutationUnverified)
	}
	for key, old := range beforeRules {
		if key == target {
			continue
		}
		now, ok := afterRules[key]
		old.RuleOrder = now.RuleOrder
		old.EditSequence = now.EditSequence
		if !ok || !reflect.DeepEqual(old, now) {
			return nil, fmt.Errorf("%w: unrelated rule changed", ErrMutationUnverified)
		}
	}
	omit := ""
	if rawID == "" {
		omit = target
	}
	if !reflect.DeepEqual(orderedRuleIDs(beforeRules, omit), orderedRuleIDs(afterRules, omit)) {
		return nil, fmt.Errorf("%w: rule priority order changed", ErrMutationUnverified)
	}
	confirmMutation(ctx)
	return &MutateResult{
		Status:   resp.StatusCode,
		Op:       map[bool]string{true: "update", false: "create"}[id >= 0],
		Entity:   "OlbRule",
		Note:     "neo lists/olbrules/save",
		Verified: true,
		Item:     QueryItem{ID: out.OlbRule.ID.String(), Name: out.OlbRule.RuleName},
	}, nil
}

// ReplayRuleDelete uses the UI's batchDelete route with one space-joined ID.
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
	before, err := ruleSnapshot(ctx, ac)
	if err != nil {
		return nil, fmt.Errorf("rule delete before-state: %w", err)
	}
	if _, ok := before[rawID]; !ok {
		return nil, fmt.Errorf("rule %s not found; no delete sent", rawID)
	}
	base := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/"
	// The UI service calls batchDeleteByIds(ids), posting ids.join(" ") as
	// the body. {"id":...} to /delete is not the live mutation contract.
	raw := []byte(strconv.Itoa(n))
	submittingMutation(ctx)
	resp, err := ac.doURIHost(ctx, http.MethodPost, base+"lists/olbrules/batchDelete", "qbo.intuit.com", raw)
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
	var receipt map[string]json.RawMessage
	// batchDelete can return an empty/non-object body on HTTP 200. The exact
	// before/after rule-list comparison below is the authoritative receipt.
	if json.Unmarshal(sraw, &receipt) == nil && (receipt["Fault"] != nil || receipt["errorDetails"] != nil) {
		return nil, fmt.Errorf("rule delete response is not a clean receipt; inspect live state before retrying")
	}
	after, err := ruleSnapshot(ctx, ac)
	if err != nil {
		return nil, fmt.Errorf("rule delete readback failed; inspect live state before retrying: %w", err)
	}
	if _, remains := after[rawID]; remains || len(after) != len(before)-1 {
		return nil, fmt.Errorf("rule delete did not remove exactly the target; inspect live state before retrying")
	}
	for id := range after {
		if _, wasPresent := before[id]; !wasPresent {
			return nil, fmt.Errorf("rule list changed outside target; inspect live state before retrying")
		}
		old, now := before[id], after[id]
		old.RuleOrder = now.RuleOrder
		old.EditSequence = now.EditSequence
		if !reflect.DeepEqual(old, now) {
			return nil, fmt.Errorf("%w: unrelated rule changed", ErrMutationUnverified)
		}
	}
	if !reflect.DeepEqual(orderedRuleIDs(before, rawID), orderedRuleIDs(after, rawID)) {
		return nil, fmt.Errorf("%w: unrelated rule priority changed", ErrMutationUnverified)
	}
	confirmMutation(ctx)
	return &MutateResult{
		Status:   resp.StatusCode,
		Op:       "delete",
		Entity:   "OlbRule",
		Note:     "neo lists/olbrules/batchDelete; target removed in independent readback",
		Verified: true,
		Item:     QueryItem{ID: rawID},
	}, nil
}

func orderedRuleIDs(rows map[string]rawBankRule, omit string) []string {
	ids := []string{}
	for id := range rows {
		if id != omit {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := atoiOrZero(rows[ids[i]].RuleOrder.String()), atoiOrZero(rows[ids[j]].RuleOrder.String())
		if a == b {
			return ids[i] < ids[j]
		}
		return a < b
	})
	return ids
}

// ruleSnapshot refuses an unparseable or page-ceiling result so a successful
// HTTP status cannot make an incomplete list look like a successful delete.
func ruleSnapshot(ctx context.Context, ac *apiClient) (map[string]rawBankRule, error) {
	url := strings.ReplaceAll(PlannedRulesURL(), realmToken, ac.realm)
	resp, err := ac.get(ctx, url, "items=0-999")
	if err != nil {
		return nil, err
	}
	defer func() { _ = drainAndClose(resp) }()
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	rules, ok := extractRules(body)
	if !ok || len(rules) >= 1000 {
		return nil, fmt.Errorf("unparseable or truncated rule list")
	}
	out := make(map[string]rawBankRule, len(rules))
	for _, rule := range rules {
		id := rule.ID.String()
		if id == "" {
			id = rule.RuleID.String()
		}
		if id == "" {
			return nil, fmt.Errorf("rule without id")
		}
		if _, duplicate := out[id]; duplicate {
			return nil, fmt.Errorf("duplicate rule id %s", id)
		}
		out[id] = rule
	}
	return out, nil
}

// neoFindRule fetches the rules list (GET — the POST variant 500s) and
// returns the rule matching id, or nil. The GET response carries the full
// wire model including conditionList/actionList.
func neoFindRule(ctx context.Context, ac *apiClient, id string) (*rawBankRule, error) {
	url := strings.ReplaceAll(PlannedRulesURL(), realmToken, ac.realm)
	resp, err := ac.get(ctx, url, "items=0-99")
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
	rules, ok := extractRules(raw)
	if !ok {
		return nil, fmt.Errorf("getRules: unparseable response (%d bytes)", len(raw))
	}
	for i := range rules {
		if rules[i].ID.String() == id || rules[i].RuleID.String() == id {
			return &rules[i], nil
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
	return "https://qbo.intuit.com/api/neo/v1/company/{realm}/lists/olbrules/batchDelete"
}
