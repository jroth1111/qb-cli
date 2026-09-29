// Package policy embeds company accounting policy as curated static
// reference data for the qb CLI.
//
// Provenance: the embedded documents under companies/ are verbatim copies of
// the Mega Style Apartments company profile from the quickbooks-ui agent
// skill (~/.agents/skills/quickbooks-ui/companies/mega-style-apartments).
// They record evidence-reviewed accounting treatment — fee rates, paired
// journal patterns, chart-of-accounts mappings, and classification guardrails.
// They are reference data, not permission to post: every mutation remains
// governed by the CLI's own readback gates.
//
// The CLI itself stays company-agnostic (CONTRACT.md: no company gate). This
// package serves the knowledge keyed by company profile name; nothing here
// restricts which company a command operates on.
package policy

import (
	"embed"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed companies
var companyFS embed.FS

// DocInfo describes one embedded reference document.
type DocInfo struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Company string `json:"company"`
	Bytes   int    `json:"bytes"`
	Title   string `json:"title"`
}

// Rule is one entry in a company policy registry (policies.yaml).
type Rule struct {
	RuleID        string   `json:"rule_id" yaml:"rule_id"`
	Scope         string   `json:"scope" yaml:"scope"`
	Property      []string `json:"property" yaml:"property"`
	Rate          *float64 `json:"rate,omitempty" yaml:"rate"`
	EffectiveFrom *string  `json:"effective_from" yaml:"effective_from"`
	EffectiveTo   *string  `json:"effective_to" yaml:"effective_to"`
	Authority     string   `json:"authority" yaml:"authority"`
	EvidenceRef   string   `json:"evidence_ref" yaml:"evidence_ref"`
	Dependency    string   `json:"dependency,omitempty" yaml:"dependency"`
	Note          string   `json:"note" yaml:"note"`
	AppliesTo     string   `json:"applies_to" yaml:"applies_to"`
	Supersedes    *string  `json:"supersedes" yaml:"supersedes"`
}

type policyFile struct {
	Rules []Rule `yaml:"rules"`
}

// Companies lists embedded company profile names.
func Companies() ([]string, error) {
	entries, err := companyFS.ReadDir("companies")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// CompanyExists reports whether a company profile is embedded.
func CompanyExists(company string) bool {
	companies, err := Companies()
	if err != nil {
		return false
	}
	for _, c := range companies {
		if c == company {
			return true
		}
	}
	return false
}

// docTitle returns the first markdown heading line, or the basename.
func docTitle(content string, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
		if line != "" && !strings.HasPrefix(line, "#") {
			// yaml or prose: stop at first non-heading line
			return fallback
		}
	}
	return fallback
}

// Docs lists the reference documents embedded for a company.
func Docs(company string) ([]DocInfo, error) {
	if !CompanyExists(company) {
		return nil, fmt.Errorf("unknown company profile %q", company)
	}
	base := path.Join("companies", company)
	entries, err := companyFS.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var out []DocInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		content, _ := companyFS.ReadFile(path.Join(base, e.Name()))
		id := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		out = append(out, DocInfo{
			ID:      id,
			Path:    path.Join(company, e.Name()),
			Company: company,
			Bytes:   int(info.Size()),
			Title:   docTitle(string(content), e.Name()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Doc returns one embedded document by id (e.g. "policies",
// "landlord-statement-journal-patterns") or file name.
func Doc(company, id string) (string, error) {
	docs, err := Docs(company)
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		if d.ID == id || path.Base(d.Path) == id {
			content, err := companyFS.ReadFile(path.Join("companies", d.Path))
			if err != nil {
				return "", err
			}
			return string(content), nil
		}
	}
	var known []string
	for _, d := range docs {
		known = append(known, d.ID)
	}
	return "", fmt.Errorf("unknown doc %q for %s (have: %s)", id, company, strings.Join(known, ", "))
}

// Rules parses the company's policies.yaml into the rule registry.
func Rules(company string) ([]Rule, error) {
	content, err := companyFS.ReadFile(path.Join("companies", company, "policies.yaml"))
	if err != nil {
		return nil, fmt.Errorf("company %q has no policies.yaml: %w", company, err)
	}
	var pf policyFile
	if err := yaml.Unmarshal(content, &pf); err != nil {
		return nil, fmt.Errorf("parse policies.yaml for %s: %w", company, err)
	}
	return pf.Rules, nil
}

// RuleByID returns one policy rule by id (e.g. "MSA.LANDLORD_MANAGEMENT_FEE").
func RuleByID(company, ruleID string) (*Rule, error) {
	rules, err := Rules(company)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].RuleID == ruleID {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("unknown rule %q for %s", ruleID, company)
}

// JournalTemplate is a distilled paired-journal pattern from
// landlord-statement-journal-patterns.md. Amounts are never filled here —
// this is the account/class skeleton an entry must satisfy.
type JournalTemplate struct {
	Kind          string   `json:"kind"`
	Description   string   `json:"description"`
	DebitAccount  string   `json:"debit_account"`
	CreditAccount string   `json:"credit_account"`
	ClassRequired bool     `json:"class_required"`
	AppliesTo     []string `json:"applies_to"`
	Note          string   `json:"note"`
	EvidenceRef   string   `json:"evidence_ref"`
}

// journalTemplates encodes the paired-journal patterns documented for the
// landlord-management properties. Keep in sync with
// landlord-statement-journal-patterns.md; rates come from policies.yaml.
var journalTemplates = []JournalTemplate{
	{
		Kind:          "management-fee",
		Description:   "Month-end management fee: 20.9% of the property's Client Property Revenue",
		DebitAccount:  "Client Property Expenses:Short-Term Business Operating Expenses:Management Fee",
		CreditAccount: "Management Income",
		ClassRequired: true,
		AppliesTo:     []string{"10 Toorak Melody", "14 Agatha"},
		Note:          "Rate rule MSA.LANDLORD_MANAGEMENT_FEE. Same property class, amount, date-period logic and description on both sides; independently recompute revenue x 20.9% before proposing.",
		EvidenceRef:   "landlord-statement-journal-patterns.md + policies.yaml MSA.LANDLORD_MANAGEMENT_FEE",
	},
	{
		Kind:          "cleaning",
		Description:   "Cleaning fee pair (usually unit 14)",
		DebitAccount:  "Client Property Expenses:Short-Term Business Operating Expenses:Commercial Cleaning Services",
		CreditAccount: "Cleaning Amenities Income",
		ClassRequired: true,
		AppliesTo:     []string{"10 Toorak Melody", "14 Agatha"},
		Note:          "Amount source is the prior month's approved cleaning fee (rule MSA.CLEANING_FEE_CARRY_FORWARD); do not copy the other property's fee. If carry-forward and activity pricing conflict, present both and ask.",
		EvidenceRef:   "landlord-statement-journal-patterns.md + policies.yaml MSA.CLEANING_FEE_CARRY_FORWARD",
	},
	{
		Kind:          "owner-distribution",
		Description:   "Owner distribution draw (separate from statement equation)",
		DebitAccount:  "Client Property Expenses:Owner Distribution",
		CreditAccount: "*",
		ClassRequired: false,
		AppliesTo:     []string{"10 Toorak Melody", "14 Agatha"},
		Note:          "Recorded separately from the owner-statement equation; never contractor pay and never another expense journal. Reconcile statement net to distributions separately.",
		EvidenceRef:   "landlord-statement-journal-patterns.md",
	},
	{
		Kind:          "cost-recharge",
		Description:   "Landlord cost recharge billed with margin",
		DebitAccount:  "Client Property Expenses:*",
		CreditAccount: "Management Income",
		ClassRequired: true,
		AppliesTo:     []string{"10 Toorak Melody", "14 Agatha"},
		Note:          "Locate the underlying supplier cost before asserting a margin (rule MSA.COST_MARGIN_EVIDENCE). A billed recharge is not proof of its margin; unresolved cost evidence means unresolved margin.",
		EvidenceRef:   "landlord-statement-journal-patterns.md + policies.yaml MSA.COST_MARGIN_EVIDENCE",
	},
}

// JournalTemplates returns the distilled paired-journal patterns.
func JournalTemplates() []JournalTemplate {
	out := make([]JournalTemplate, len(journalTemplates))
	copy(out, journalTemplates)
	return out
}

// JournalTemplate returns one template by kind.
func JournalTemplateFor(kind string) (*JournalTemplate, error) {
	for i := range journalTemplates {
		if journalTemplates[i].Kind == kind {
			return &journalTemplates[i], nil
		}
	}
	var kinds []string
	for _, t := range journalTemplates {
		kinds = append(kinds, t.Kind)
	}
	return nil, fmt.Errorf("unknown journal template %q (have: %s)", kind, strings.Join(kinds, ", "))
}

// FeeCalculation is the deterministic result of the management-fee rule.
type FeeCalculation struct {
	RuleID        string   `json:"rule_id"`
	Revenue       float64  `json:"revenue"`
	Rate          float64  `json:"rate"`
	Fee           float64  `json:"fee"`
	RawFee        float64  `json:"raw_fee"`
	Rounding      string   `json:"rounding"`
	DebitAccount  string   `json:"debit_account"`
	CreditAccount string   `json:"credit_account"`
	Class         string   `json:"class,omitempty"`
	ClassWarning  string   `json:"class_warning,omitempty"`
	Properties    []string `json:"applies_to_properties"`
	Note          string   `json:"note"`
}

// landlordClasses lists the classes the fee rule is evidenced for.
var landlordClasses = map[string]bool{
	"10 Toorak Melody": true,
	"14 Agatha":        true,
}

// ManagementFee applies MSA.LANDLORD_MANAGEMENT_FEE: fee = revenue x rate,
// rounded half-away-from-zero to cents. Class is advisory: an unknown class
// produces a warning field, never a hard stop (the CLI has no company gate).
func ManagementFee(company string, revenue float64, class string) (*FeeCalculation, error) {
	rule, err := RuleByID(company, "MSA.LANDLORD_MANAGEMENT_FEE")
	if err != nil {
		return nil, err
	}
	if rule.Rate == nil {
		return nil, fmt.Errorf("rule %s carries no rate", rule.RuleID)
	}
	if revenue < 0 {
		return nil, fmt.Errorf("revenue must be non-negative, got %v", revenue)
	}
	raw := revenue * *rule.Rate
	fee := math.Round(raw*100) / 100
	tpl, _ := JournalTemplateFor("management-fee")
	calc := &FeeCalculation{
		RuleID:        rule.RuleID,
		Revenue:       revenue,
		Rate:          *rule.Rate,
		Fee:           fee,
		RawFee:        raw,
		Rounding:      "half-up to cents",
		DebitAccount:  tpl.DebitAccount,
		CreditAccount: tpl.CreditAccount,
		Class:         class,
		Properties:    rule.Property,
		Note:          "Recompute independently before posting: verify the revenue base (Client Property Revenue for the period, same class) and compare against the prior approved journal. Historical rounding evidence takes precedence over this computed default.",
	}
	if class != "" && !landlordClasses[class] {
		calc.ClassWarning = fmt.Sprintf("class %q is not one of the landlord-management properties this rule is evidenced for (%s)", class, strings.Join(rule.Property, ", "))
	}
	return calc, nil
}
