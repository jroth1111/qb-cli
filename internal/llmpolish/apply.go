package llmpolish

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HelpImprovement represents an improved command description from the LLM.
type HelpImprovement struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// ExampleSet represents generated examples for a single command.
type ExampleSet struct {
	Command  string   `json:"command"`
	Examples []string `json:"examples"`
}

// applyHelpTexts reads each command .go file, finds Short: "...", and replaces
// it with the improved description from the LLM.
func applyHelpTexts(outputDir string, improvements []HelpImprovement) error {
	cliDir := filepath.Join(outputDir, "internal", "cli")

	// Build a map of command name -> improved description
	descMap := make(map[string]string)
	for _, imp := range improvements {
		descMap[imp.Command] = imp.Description
	}

	entries, err := os.ReadDir(cliDir)
	if err != nil {
		return fmt.Errorf("reading cli dir: %w", err)
	}

	shortRe := regexp.MustCompile(`(Short:\s*)"([^"]*)"`)
	useRe := regexp.MustCompile(`Use:\s*"([^"]*)"`)

	applied := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		filePath := filepath.Join(cliDir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		content := string(data)

		// Find the Use: value to match against the improvement map
		cmdName := strings.TrimSuffix(e.Name(), ".go")
		if m := useRe.FindStringSubmatch(content); len(m) > 1 {
			cmdName = m[1]
		}

		newDesc, ok := descMap[cmdName]
		if !ok {
			continue
		}

		// Replace the Short: value
		newContent := shortRe.ReplaceAllStringFunc(content, func(match string) string {
			return shortRe.ReplaceAllString(match, fmt.Sprintf(`${1}"%s"`, escapeGoString(newDesc)))
		})

		if newContent != content {
			if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", e.Name(), err)
			}
			applied++
		}
	}

	return nil
}

// applyExamples reads command files, finds Example: "...", and replaces with
// multi-line examples from the LLM.
func applyExamples(outputDir string, examples []ExampleSet) error {
	cliDir := filepath.Join(outputDir, "internal", "cli")

	// Build a map of command name -> examples
	exMap := make(map[string][]string)
	for _, ex := range examples {
		exMap[ex.Command] = ex.Examples
	}

	entries, err := os.ReadDir(cliDir)
	if err != nil {
		return fmt.Errorf("reading cli dir: %w", err)
	}

	exampleRe := regexp.MustCompile("(?s)(Example:\\s*)(`[^`]*`|\"[^\"]*\")")
	useRe := regexp.MustCompile(`Use:\s*"([^"]*)"`)

	applied := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		filePath := filepath.Join(cliDir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		content := string(data)

		cmdName := strings.TrimSuffix(e.Name(), ".go")
		if m := useRe.FindStringSubmatch(content); len(m) > 1 {
			cmdName = m[1]
		}

		exStrings, ok := exMap[cmdName]
		if !ok || len(exStrings) == 0 {
			continue
		}

		// Build the example string with backtick quoting for multi-line
		combined := strings.Join(exStrings, "\n\n")
		exampleLiteral := fmt.Sprintf("`%s`", combined)

		newContent := exampleRe.ReplaceAllStringFunc(content, func(match string) string {
			m := exampleRe.FindStringSubmatch(match)
			if len(m) < 2 {
				return match
			}
			return m[1] + exampleLiteral
		})

		if newContent != content {
			if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", e.Name(), err)
			}
			applied++
		}
	}

	return nil
}

// applyREADME writes the new README content to the output directory.
func applyREADME(outputDir, newContent string) error {
	readmePath := filepath.Join(outputDir, "README.md")
	trimmed := strings.TrimSpace(newContent)
	if trimmed == "" {
		return fmt.Errorf("empty README content")
	}
	return os.WriteFile(readmePath, []byte(trimmed+"\n"), 0o644)
}

// escapeGoString escapes characters that would break a Go string literal.
func escapeGoString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
