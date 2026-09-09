package generator

import (
	"fmt"
	"regexp"
	"strings"
)

// AITextWarning records a single match of AI-characteristic text.
type AITextWarning struct {
	Pattern string
	Match   string
	Line    int
	Context string // surrounding text
}

// aiSlopPatterns are regex patterns that detect common AI-generated text patterns.
var aiSlopPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(comprehensive|robust|seamless|leverage|utilize|facilitate)\b`),
	regexp.MustCompile(`(?i)here's a .* that`),
	regexp.MustCompile(`(?i)not just .*, it's`),
	regexp.MustCompile(`(?i)whether you're .* or .*,`),
	regexp.MustCompile(`(?i)\b(streamline|empower|cutting-edge|game-changer)\b`),
	regexp.MustCompile(`(?i)\b(revolutionize|transform|elevate|harness)\b`),
	regexp.MustCompile(`(?i)in today's .* landscape`),
	regexp.MustCompile(`(?i)take .* to the next level`),
}

// CheckText scans text for AI-characteristic patterns and returns warnings.
// It never modifies text - only reports matches.
func CheckText(text string) []AITextWarning {
	var warnings []AITextWarning
	lines := strings.Split(text, "\n")

	for lineNum, line := range lines {
		for _, pat := range aiSlopPatterns {
			matches := pat.FindAllString(line, -1)
			for _, m := range matches {
				warnings = append(warnings, AITextWarning{
					Pattern: pat.String(),
					Match:   m,
					Line:    lineNum + 1,
					Context: truncateContext(line, 80),
				})
			}
		}
	}

	return warnings
}

// FormatWarnings returns a human-readable summary of text filter warnings.
func FormatWarnings(warnings []AITextWarning) string {
	if len(warnings) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "anti-AI text filter: %d warning(s)\n", len(warnings))
	for _, w := range warnings {
		fmt.Fprintf(&b, "  line %d: matched %q in: %s\n", w.Line, w.Match, w.Context)
	}
	return b.String()
}

func truncateContext(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
