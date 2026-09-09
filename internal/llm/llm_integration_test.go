package llm

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunWithRealLLM tests the LLM runner with a real Claude CLI call.
// This test costs ~$0.01 in tokens. Run with: LLM_TEST=1 go test ./internal/llm/ -run TestRunWithRealLLM -v
func TestRunWithRealLLM(t *testing.T) {
	if os.Getenv("LLM_TEST") == "" {
		t.Skip("Set LLM_TEST=1 to test with real LLM (costs tokens)")
	}
	if !Available() {
		t.Skip("No LLM CLI available (install claude or codex)")
	}

	response, err := Run("Respond with exactly the word 'hello' and nothing else. No punctuation, no explanation.")
	require.NoError(t, err, "LLM Run should not error")
	assert.Contains(t, strings.ToLower(response), "hello", "Response should contain 'hello'")
	t.Logf("LLM response: %q", response)
}

// TestDocSpecPromptWithRealLLM tests that the doc-to-spec prompt produces valid YAML.
// Run with: LLM_TEST=1 go test ./internal/llm/ -run TestDocSpecPromptWithRealLLM -v -timeout 2m
func TestDocSpecPromptWithRealLLM(t *testing.T) {
	if os.Getenv("LLM_TEST") == "" {
		t.Skip("Set LLM_TEST=1 to test with real LLM (costs ~$0.30)")
	}
	if !Available() {
		t.Skip("No LLM CLI available")
	}

	prompt := `Generate a YAML API spec for a simple pet store API with these endpoints:
- GET /pets (list all pets)
- GET /pets/{id} (get one pet)
- POST /pets (create a pet, body: name string, status string)
- DELETE /pets/{id} (delete a pet)

Output ONLY valid YAML in this format (no markdown fences):
name: petstore
description: "Simple pet store"
base_url: "https://api.example.com"
auth:
  type: api_key
  header: "X-API-Key"
  env_vars:
    - PETSTORE_API_KEY
resources:
  pets:
    description: "Pet management"
    endpoints:
      list:
        method: GET
        path: /pets
        description: "List all pets"
      get:
        method: GET
        path: /pets/{id}
        description: "Get a pet by ID"
        params:
          - name: id
            type: string
            positional: true`

	response, err := Run(prompt)
	require.NoError(t, err)

	// Verify it looks like YAML
	assert.Contains(t, response, "name:", "Response should contain YAML name field")
	assert.Contains(t, response, "resources:", "Response should contain resources")
	assert.Contains(t, response, "endpoints:", "Response should contain endpoints")
	t.Logf("LLM response length: %d chars", len(response))
	t.Logf("First 500 chars: %s", response[:min(500, len(response))])
}
