package client

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const managedMemoPrefix = "[[QBO-METADATA:"
const managedMemoClosePrefix = "[[/QBO-METADATA:"
const managedMemoMaxRunes = 4000

var managedMemoKey = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
var memoBrackets = regexp.MustCompile(`\[[^\[\]\n]+\]`)
var legacyMemoAnnotation = regexp.MustCompile(`(?i)^\[(?:Card\s*:|Fee\s*:|Fee link\s*:|Parent\s*:|Parent group|Parent QBO|International fee QBO|Inferred (?:parent|separate fee)|Provisional (?:allocation|fee)|Most-probable separate fee)`)

type managedMemoSection struct {
	key, text string
}

func managedMemoSections(note string) ([]managedMemoSection, error) {
	var sections []managedMemoSection
	seen := map[string]bool{}
	for len(note) > 0 {
		start := strings.Index(note, managedMemoPrefix)
		closing := strings.Index(note, managedMemoClosePrefix)
		if closing >= 0 && (start < 0 || closing < start) {
			return nil, fmt.Errorf("managed memo has an unmatched closing marker")
		}
		if start < 0 {
			break
		}
		rest := note[start+len(managedMemoPrefix):]
		endKey := strings.Index(rest, "]]")
		if endKey < 0 || !managedMemoKey.MatchString(rest[:endKey]) {
			return nil, fmt.Errorf("managed memo has an invalid opening marker")
		}
		key := rest[:endKey]
		if seen[key] {
			return nil, fmt.Errorf("managed memo repeats section %q", key)
		}
		seen[key] = true
		bodyStart := start + len(managedMemoPrefix) + endKey + 2
		endMarker := managedMemoClosePrefix + key + "]]"
		end := strings.Index(note[bodyStart:], endMarker)
		if end < 0 || strings.Contains(note[bodyStart:bodyStart+end], managedMemoPrefix) || strings.Contains(note[bodyStart:bodyStart+end], managedMemoClosePrefix) {
			return nil, fmt.Errorf("managed memo section %q is unclosed or nested", key)
		}
		stop := bodyStart + end + len(endMarker)
		sections = append(sections, managedMemoSection{key: key, text: note[start:stop]})
		note = note[stop:]
	}
	return sections, nil
}

// Invoice text replacements must not silently discard separately managed metadata.
// A supplied section with the same key is an intentional metadata replacement.
func preserveManagedMemo(existing, proposed string) (string, error) {
	old, err := managedMemoSections(existing)
	if err != nil {
		return "", err
	}
	newSections, err := managedMemoSections(proposed)
	if err != nil {
		return "", err
	}
	keys := map[string]bool{}
	for _, section := range newSections {
		keys[section.key] = true
	}
	merged := proposed
	for _, section := range old {
		if !keys[section.key] {
			merged = strings.TrimRight(merged, " \t\r\n") + "\n\n" + section.text
		}
	}
	// Legacy free-form annotation suffixes cannot safely be reconstructed by
	// guessing which qualification belongs to which reference. Require retention
	// or a deliberately supplied managed replacement instead of deleting them.
	if len(old) == 0 && len(newSections) == 0 {
		fragments := memoBrackets.FindAllString(existing, -1)
		protected := false
		for _, fragment := range fragments {
			protected = protected || legacyMemoAnnotation.MatchString(fragment)
		}
		if protected {
			for _, fragment := range fragments {
				if !strings.Contains(proposed, fragment) {
					return "", fmt.Errorf("memo replacement would remove existing annotations; retain their bracketed references and qualifications or supply a validated managed metadata section")
				}
			}
		}
	}
	if (len(old) > 0 || len(newSections) > 0) && utf8.RuneCountInString(merged) > managedMemoMaxRunes {
		return "", fmt.Errorf("managed memo exceeds %d characters; use an annotation attachment rather than truncate metadata", managedMemoMaxRunes)
	}
	return merged, nil
}
