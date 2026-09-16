package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	ID       flexibleString `json:"id"`
	RuleID   flexibleString `json:"ruleId"`
	Name     string         `json:"name"`
	Title    string         `json:"title"`
	RuleName string         `json:"ruleName"`
	AutoAdd  *bool          `json:"autoAdd"`
	Active   *bool          `json:"active"`
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
