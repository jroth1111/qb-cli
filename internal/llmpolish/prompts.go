package llmpolish

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// commandInfo holds extracted metadata about a generated CLI command.
type commandInfo struct {
	Name        string
	Description string
	Method      string
	Path        string
	Flags       []string
}

// buildHelpPrompt reads command files from the generated CLI and builds a
// prompt asking the LLM to rewrite their Short descriptions.
func buildHelpPrompt(outputDir string) string {
	commands := extractCommands(outputDir)
	if len(commands) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Rewrite these CLI command descriptions to be developer-friendly.\n")
	b.WriteString("Rules: under 80 chars, starts with a verb, no API jargon.\n\n")
	b.WriteString("Current:\n")
	for _, cmd := range commands {
		fmt.Fprintf(&b, "- %s: %q\n", cmd.Name, cmd.Description)
	}
	b.WriteString("\nReturn ONLY a JSON array, no other text:\n")
	b.WriteString(`[{"command": "example", "description": "Improved description here"}]`)
	b.WriteString("\n")

	return b.String()
}

// buildExamplePrompt reads commands and builds a prompt asking the LLM to
// generate realistic examples for each command.
func buildExamplePrompt(outputDir string) string {
	commands := extractCommands(outputDir)
	if len(commands) == 0 {
		return ""
	}

	cliName := filepath.Base(outputDir)

	var b strings.Builder
	fmt.Fprintf(&b, "Write 2-3 realistic examples for each command in %s.\n", cliName)
	b.WriteString("Each example should show a real developer workflow with a comment.\n\n")
	b.WriteString("Commands:\n")
	for _, cmd := range commands {
		flags := "none"
		if len(cmd.Flags) > 0 {
			flags = strings.Join(cmd.Flags, ", ")
		}
		method := cmd.Method
		if method == "" {
			method = "GET"
		}
		path := cmd.Path
		if path == "" {
			path = "/" + strings.ReplaceAll(cmd.Name, " ", "/")
		}
		fmt.Fprintf(&b, "- %s (%s %s, flags: %s)\n", cmd.Name, method, path, flags)
	}
	b.WriteString("\nReturn ONLY a JSON array, no other text:\n")
	fmt.Fprintf(&b, `[{"command": "example list", "examples": ["# List all examples\n%s example list --limit 10"]}]`, cliName)
	b.WriteString("\n")

	return b.String()
}

// buildREADMEPrompt reads the current README and builds a prompt asking the
// LLM to rewrite it to sell the tool to developers.
func buildREADMEPrompt(outputDir, apiName string) string {
	readmePath := filepath.Join(outputDir, "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return ""
	}
	if len(content) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Rewrite this CLI README to sell the tool to developers.\n\n")
	fmt.Fprintf(&b, "API: %s\n", apiName)
	fmt.Fprintf(&b, "Current README:\n%s\n\n", string(content))
	b.WriteString("Write a README with:\n")
	b.WriteString("1. One-line hook that makes developers want to install it\n")
	b.WriteString("2. Why this exists (what gap it fills)\n")
	b.WriteString("3. Quick start (3 commands max)\n")
	b.WriteString("4. Full command list from the current README\n\n")
	b.WriteString("Keep existing section structure but improve the copy.\n")
	b.WriteString("Return ONLY the markdown content, no code fences.\n")

	return b.String()
}

// extractCommands reads .go files from the generated CLI's internal/cli/
// directory and extracts command metadata.
func extractCommands(outputDir string) []commandInfo {
	cliDir := filepath.Join(outputDir, "internal", "cli")
	entries, err := os.ReadDir(cliDir)
	if err != nil {
		return nil
	}

	infraFiles := map[string]bool{
		"helpers.go": true,
		"root.go":    true,
		"doctor.go":  true,
		"auth.go":    true,
	}

	shortRe := regexp.MustCompile(`Short:\s*"([^"]*)"`)
	useRe := regexp.MustCompile(`Use:\s*"([^"]*)"`)
	flagRe := regexp.MustCompile(`Flags\(\)\.\w+Var\w*\([^,]+,\s*"([^"]+)"`)
	methodRe := regexp.MustCompile(`(?:Method|method):\s*"([^"]*)"`)
	pathRe := regexp.MustCompile(`(?:Path|path|URL|url):\s*"([^"]*)"`)

	var commands []commandInfo
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if infraFiles[name] {
			continue
		}

		data, err := os.ReadFile(filepath.Join(cliDir, name))
		if err != nil {
			continue
		}
		content := string(data)

		cmd := commandInfo{
			Name: strings.TrimSuffix(name, ".go"),
		}

		if m := shortRe.FindStringSubmatch(content); len(m) > 1 {
			cmd.Description = m[1]
		}
		if m := useRe.FindStringSubmatch(content); len(m) > 1 {
			cmd.Name = m[1]
		}
		if m := methodRe.FindStringSubmatch(content); len(m) > 1 {
			cmd.Method = m[1]
		}
		if m := pathRe.FindStringSubmatch(content); len(m) > 1 {
			cmd.Path = m[1]
		}

		flags := flagRe.FindAllStringSubmatch(content, -1)
		for _, f := range flags {
			if len(f) > 1 {
				cmd.Flags = append(cmd.Flags, "--"+f[1])
			}
		}

		commands = append(commands, cmd)
	}
	return commands
}
