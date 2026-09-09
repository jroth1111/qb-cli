package llmpolish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildHelpPrompt(t *testing.T) {
	dir := setupFakeCLI(t)

	prompt := buildHelpPrompt(dir)

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "pet")
	assert.Contains(t, prompt, "Returns list of pets")
	assert.Contains(t, prompt, "developer-friendly")
}

func TestBuildHelpPromptEmptyDir(t *testing.T) {
	dir := t.TempDir()
	prompt := buildHelpPrompt(dir)
	assert.Empty(t, prompt)
}

func TestBuildExamplePrompt(t *testing.T) {
	dir := setupFakeCLI(t)

	prompt := buildExamplePrompt(dir)

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "pet")
	assert.Contains(t, prompt, "--status")
	assert.Contains(t, prompt, "examples")
}

func TestBuildREADMEPrompt(t *testing.T) {
	dir := t.TempDir()
	readme := "# my-cli\n\nA CLI for the My API.\n\n## Quick Start\n\n```\nmy-cli pet list\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644))

	prompt := buildREADMEPrompt(dir, "my-api")

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "my-api")
	assert.Contains(t, prompt, "Quick Start")
	assert.Contains(t, prompt, "pet list")
}

func TestBuildREADMEPromptMissingFile(t *testing.T) {
	dir := t.TempDir()
	prompt := buildREADMEPrompt(dir, "nope")
	assert.Empty(t, prompt)
}

func TestPolishSkipsWhenNoLLM(t *testing.T) {
	// Override PATH to ensure no LLM CLI is found
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", originalPath)

	result, err := Polish(PolishRequest{
		APIName:   "test",
		OutputDir: t.TempDir(),
	})

	require.NoError(t, err)
	assert.True(t, result.Skipped)
	assert.Contains(t, result.SkipReason, "no LLM CLI found")
}

func TestApplyHelpTexts(t *testing.T) {
	dir := setupFakeCLI(t)

	improvements := []HelpImprovement{
		{Command: "pet", Description: "List all pets, filtered by status"},
	}

	err := applyHelpTexts(dir, improvements)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "internal", "cli", "pet.go"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "List all pets, filtered by status")
	assert.NotContains(t, string(data), "Returns list of pets")
}

func TestApplyExamples(t *testing.T) {
	dir := setupFakeCLI(t)

	examples := []ExampleSet{
		{
			Command: "pet",
			Examples: []string{
				"# List available pets\npet-cli pet list --status available",
				"# Get a specific pet\npet-cli pet get --id 123",
			},
		},
	}

	err := applyExamples(dir, examples)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "internal", "cli", "pet.go"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "List available pets")
	assert.Contains(t, string(data), "pet-cli pet list --status available")
}

func TestApplyREADME(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("old"), 0o644))

	err := applyREADME(dir, "# New README\n\nBetter content.")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Better content")
	assert.NotContains(t, string(data), "old")
}

func TestApplyREADMEEmpty(t *testing.T) {
	dir := t.TempDir()
	err := applyREADME(dir, "  ")
	assert.Error(t, err)
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "bare array",
			input:  `[{"command":"pet","description":"List pets"}]`,
			expect: `[{"command":"pet","description":"List pets"}]`,
		},
		{
			name:   "wrapped in markdown",
			input:  "Here is the result:\n```json\n[{\"command\":\"pet\"}]\n```\n",
			expect: `[{"command":"pet"}]`,
		},
		{
			name:   "no json",
			input:  "No JSON here",
			expect: "[]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(extractJSON(tt.input))
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestExtractMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "plain text",
			input:  "# Hello\n\nWorld",
			expect: "# Hello\n\nWorld",
		},
		{
			name:   "fenced markdown",
			input:  "```markdown\n# Hello\n```",
			expect: "\n# Hello\n",
		},
		{
			name:   "fenced generic",
			input:  "```\n# Hello\n```",
			expect: "\n# Hello\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMarkdown(tt.input)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestCountExamples(t *testing.T) {
	examples := []ExampleSet{
		{Command: "a", Examples: []string{"ex1", "ex2"}},
		{Command: "b", Examples: []string{"ex3"}},
	}
	assert.Equal(t, 3, countExamples(examples))
}

func TestExtractCommands(t *testing.T) {
	dir := setupFakeCLI(t)
	commands := extractCommands(dir)

	require.Len(t, commands, 1)
	assert.Equal(t, "pet", commands[0].Name)
	assert.Equal(t, "Returns list of pets", commands[0].Description)
	assert.Contains(t, commands[0].Flags, "--status")
}

// setupFakeCLI creates a minimal generated CLI directory structure for testing.
func setupFakeCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cliDir := filepath.Join(dir, "internal", "cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))

	// Infrastructure files (should be skipped)
	for _, infra := range []string{"root.go", "helpers.go", "doctor.go", "auth.go"} {
		require.NoError(t, os.WriteFile(filepath.Join(cliDir, infra), []byte("package cli\n"), 0o644))
	}

	// Command file
	petGo := strings.Join([]string{
		"package cli",
		"",
		"import \"github.com/spf13/cobra\"",
		"",
		"func newPetCmd() *cobra.Command {",
		"\tvar status string",
		"\tcmd := &cobra.Command{",
		"\t\tUse:   \"pet\",",
		"\t\tShort: \"Returns list of pets\",",
		"\t\tExample: \"pet-cli pet list\",",
		"\t}",
		"\tcmd.Flags().StringVar(&status, \"status\", \"\", \"Filter by status\")",
		"\treturn cmd",
		"}",
	}, "\n")
	require.NoError(t, os.WriteFile(filepath.Join(cliDir, "pet.go"), []byte(petGo), 0o644))

	return dir
}
