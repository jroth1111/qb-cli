package policy

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// catSignalFile mirrors categorisation-signals.yaml — repeated posted
// mappings distilled from the decision model and two-year audit. Curated
// static content; not an automation rulebook.
type catSignalFile struct {
	Version             int    `yaml:"version"`
	SourceWindow        string `yaml:"source_window"`
	Signals             []catSignal
	Conflicts           []catConflict
	AmbiguousClassNames []string `yaml:"ambiguous_class_names"`
	UnitTokenPattern    string   `yaml:"unit_token_pattern"`
}

type catSignal struct {
	Match         []string `yaml:"match"`
	Account       string   `yaml:"account"`
	Evidence      string   `yaml:"evidence"`
	Strong        bool     `yaml:"strong"`
	Discriminator string   `yaml:"discriminator"`
	Note          string   `yaml:"note"`
	ClassHint     string   `yaml:"class_hint"`
}

type catConflict struct {
	Match        []string `yaml:"match"`
	Reason       string   `yaml:"reason"`
	Alternatives []string `yaml:"alternatives"`
}

// AccountCandidate is one suggested account with its posted-pattern evidence.
type AccountCandidate struct {
	Account       string `json:"account"`
	Evidence      string `json:"evidence"`
	Matched       string `json:"matched"`    // the pattern that fired
	Confidence    string `json:"confidence"` // "strong" | "weak"
	Discriminator string `json:"discriminator,omitempty"`
	Note          string `json:"note,omitempty"`
}

// ConflictHit records a text signal the audit says must never auto-classify.
type ConflictHit struct {
	Matched      string   `json:"matched"`
	Reason       string   `json:"reason"`
	Alternatives []string `json:"alternatives"`
}

// ClassCandidate is a suggested class with the evidence that produced it.
type ClassCandidate struct {
	Class string `json:"class"`
	ID    string `json:"id,omitempty"`
	Basis string `json:"basis"`
}

// CategoriseResult is the suggestion envelope for one transaction text.
type CategoriseResult struct {
	Input             string             `json:"input"`
	AccountCandidates []AccountCandidate `json:"account_candidates,omitempty"`
	Conflicts         []ConflictHit      `json:"conflicts,omitempty"`
	ClassCandidates   []ClassCandidate   `json:"class_candidates,omitempty"`
	Notes             []string           `json:"notes,omitempty"`
	Verdict           string             `json:"verdict"` // suggested | conflict | unresolved
}

func loadSignals(company string) (*catSignalFile, error) {
	company, err := resolveCompanyForCOA(company)
	if err != nil {
		return nil, err
	}
	raw, err := companyFS.ReadFile(path.Join("companies", company, "categorisation-signals.yaml"))
	if err != nil {
		return nil, err
	}
	var sf catSignalFile
	if err := yaml.Unmarshal(raw, &sf); err != nil {
		return nil, fmt.Errorf("categorisation-signals.yaml: %w", err)
	}
	return &sf, nil
}

// Categorise suggests account/class treatment for a transaction text
// (party + memo + description, joined) from the embedded posted-pattern
// evidence. It never asserts a posting — weak signals and known conflicts
// return unresolved/conflict verdicts with their required discriminators.
func Categorise(company, text string) (*CategoriseResult, error) {
	sf, err := loadSignals(company)
	if err != nil {
		return nil, err
	}
	res := &CategoriseResult{Input: text}
	lower := strings.ToLower(text)

	matched := func(patterns []string) string {
		for _, p := range patterns {
			if re, err := regexp.Compile("(?i)" + p); err == nil && re.MatchString(text) {
				return p
			}
		}
		return ""
	}

	var strong *catSignal
	var strongHit string
	for i := range sf.Signals {
		s := &sf.Signals[i]
		hit := matched(s.Match)
		if hit == "" {
			continue
		}
		conf := "weak"
		if s.Strong {
			conf = "strong"
		}
		res.AccountCandidates = append(res.AccountCandidates, AccountCandidate{
			Account: s.Account, Evidence: s.Evidence, Matched: hit,
			Confidence: conf, Discriminator: s.Discriminator, Note: s.Note,
		})
		if s.Strong && strong == nil {
			strong, strongHit = s, hit
		}
	}
	for i := range sf.Conflicts {
		c := &sf.Conflicts[i]
		if hit := matched(c.Match); hit != "" {
			res.Conflicts = append(res.Conflicts, ConflictHit{
				Matched: hit, Reason: c.Reason, Alternatives: c.Alternatives,
			})
		}
	}

	// Class evidence: explicit unit tokens, signal class hints, ambiguous
	// building-name detection.
	if strong != nil && strong.ClassHint != "" {
		res.ClassCandidates = append(res.ClassCandidates, classCandidate(company, strong.ClassHint, fmt.Sprintf("signal %q established person/central class", strongHit)))
	}
	classes, _ := Classes(company)
	// Index distinctive class-name words: a word matching exactly one class
	// is safe class evidence ("toorak" → "10 Toorak Melody"); a word shared
	// by several classes ("mainpoint") is not.
	classWordStop := map[string]bool{
		"the": true, "and": true, "pty": true, "ltd": true, "unit": true,
		"property": true, "apartment": true, "deleted": true, "trust": true,
	}
	wordClasses := map[string][]QBClass{}
	for _, c := range classes {
		for _, w := range strings.FieldsFunc(strings.ToLower(c.Name), func(r rune) bool {
			return !(r >= 'a' && r <= 'z')
		}) {
			if len(w) < 4 || classWordStop[w] {
				continue
			}
			wordClasses[w] = append(wordClasses[w], c)
		}
	}
	if tok, err := regexp.Compile("(?i)" + sf.UnitTokenPattern); err == nil {
		seen := map[string]bool{}
		for _, m := range tok.FindAllStringSubmatch(text, -1) {
			for _, c := range classes {
				name := c.Name
				if (strings.HasPrefix(name, m[1]+" ") || name == m[1]) && !seen[name] {
					seen[name] = true
					res.ClassCandidates = append(res.ClassCandidates, ClassCandidate{
						Class: name, ID: c.ID, Basis: fmt.Sprintf("unit token %q in text", m[1]),
					})
				}
			}
		}
	}
	for w, matches := range wordClasses {
		if len(matches) != 1 || !strings.Contains(lower, w) {
			continue
		}
		dup := false
		for _, c := range res.ClassCandidates {
			if c.ID == matches[0].ID {
				dup = true
			}
		}
		if !dup {
			res.ClassCandidates = append(res.ClassCandidates, ClassCandidate{
				Class: matches[0].Name, ID: matches[0].ID, Basis: fmt.Sprintf("distinctive word %q resolves to a single class", w),
			})
		}
	}
	for _, amb := range sf.AmbiguousClassNames {
		if strings.Contains(lower, amb) {
			res.Notes = append(res.Notes, fmt.Sprintf("building name %q is ambiguous class evidence — a unit number is required", amb))
		}
	}

	switch {
	case strong != nil:
		res.Verdict = "suggested"
		if len(res.Conflicts) > 0 {
			res.Notes = append(res.Notes, "a known-conflict signal also matched; the strong posted pattern won — verify the discriminator if the match is borderline")
		}
	case len(res.Conflicts) > 0:
		res.Verdict = "conflict"
	case len(res.AccountCandidates) > 0:
		res.Verdict = "unresolved"
		res.Notes = append(res.Notes, "only weak (<98%) signals matched; supply the listed discriminator before using the suggestion")
	default:
		res.Verdict = "unresolved"
		res.Notes = append(res.Notes, "no posted mapping matched; classify from property/person evidence or leave unresolved")
	}
	return res, nil
}

// classCandidate resolves a documented class name against the embedded
// inventory so suggestions carry the live id when known.
func classCandidate(company, name, basis string) ClassCandidate {
	classes, _ := Classes(company)
	for _, c := range classes {
		if strings.EqualFold(c.Name, name) {
			return ClassCandidate{Class: c.Name, ID: c.ID, Basis: basis}
		}
	}
	return ClassCandidate{Class: name, Basis: basis + " (class not in inventory — verify)"}
}
