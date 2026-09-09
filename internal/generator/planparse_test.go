package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           string
		wantCLIName     string
		wantCmdCount    int
		wantCmdNames    []string
		wantDescription string
	}{
		{
			name: "minimal plan with backtick commands",
			input: `# Screen Tool

## Commands
- ` + "`record`" + ` - Record screen capture
- ` + "`screenshot`" + ` - Take a screenshot
- ` + "`gif`" + ` - Convert recording to GIF
`,
			wantCLIName:  "screen-tool",
			wantCmdCount: 3,
			wantCmdNames: []string{"record", "screenshot", "gif"},
		},
		{
			name: "plan with subcommands",
			input: `# MyApp CLI

## Commands
- ` + "`auth login`" + ` - Log in to your account
- ` + "`auth logout`" + ` - Log out of your account
- ` + "`auth status`" + ` - Show auth status
- ` + "`deploy`" + ` - Deploy application
`,
			wantCLIName:  "myapp",
			wantCmdCount: 4,
			wantCmdNames: []string{"auth login", "auth logout", "auth status", "deploy"},
		},
		{
			name: "plan with implementation units",
			input: `# Video Editor

### WU-1: Trim command
- **Goal:** Trim video clips to specified duration

### WU-2: Merge command
- **Goal:** Merge multiple video clips
`,
			wantCLIName:  "video-editor",
			wantCmdCount: 2,
			wantCmdNames: []string{"trim-command", "merge-command"},
		},
		{
			name: "plan with architecture section",
			input: `# Deploy Tool

## Architecture
- init - Initialize a new project
- push - Push current state to remote
- status - Show deployment status
`,
			wantCLIName:  "deploy-tool",
			wantCmdCount: 3,
			wantCmdNames: []string{"init", "push", "status"},
		},
		{
			name: "plan title cleaned of suffixes",
			input: `# Acme CLI Implementation Plan

## CLI Commands
- ` + "`sync`" + ` - Sync data
`,
			wantCLIName:  "acme",
			wantCmdCount: 1,
		},
		{
			name: "deduplicates commands",
			input: `# DupTool

## Commands
- ` + "`run`" + ` - Run the thing
- ` + "`run`" + ` - Run the thing again
`,
			wantCLIName:  "duptool",
			wantCmdCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ps := ParsePlan(tt.input)

			assert.Equal(t, tt.wantCLIName, ps.CLIName)
			assert.Len(t, ps.Commands, tt.wantCmdCount)

			if tt.wantCmdNames != nil {
				var gotNames []string
				for _, cmd := range ps.Commands {
					gotNames = append(gotNames, cmd.Name)
				}
				assert.Equal(t, tt.wantCmdNames, gotNames)
			}
		})
	}
}

func TestPlanCommand_ParentAndLeaf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cmd        PlanCommand
		wantParent string
		wantLeaf   string
	}{
		{
			name:       "top-level command",
			cmd:        PlanCommand{Name: "deploy"},
			wantParent: "",
			wantLeaf:   "deploy",
		},
		{
			name:       "subcommand",
			cmd:        PlanCommand{Name: "auth login"},
			wantParent: "auth",
			wantLeaf:   "login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantParent, tt.cmd.Parent())
			assert.Equal(t, tt.wantLeaf, tt.cmd.Leaf())
		})
	}
}

func TestCleanCLIName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Acme CLI", "acme"},
		{"My Cool Tool", "my-cool-tool"},
		{"Something Implementation Plan", "something"},
		{"Video Editor Plan", "video-editor"},
		{"simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cleanCLIName(tt.input))
		})
	}
}
