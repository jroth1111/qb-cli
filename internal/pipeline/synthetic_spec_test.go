package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSyntheticSpec_DogfoodSkipsPathCheck verifies that specs declared with
// `kind: synthetic` bypass the strict path-validity check in dogfood. This is
// the fix for combo/cross-site CLIs (e.g., Recipe GOAT) where the spec
// describes a primary API but the CLI intentionally adds hand-built commands
// that would otherwise trigger "not in spec" false failures.
// Regression guard for #203.
func TestSyntheticSpec_DogfoodSkipsPathCheck(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0o755))

	// A CLI command whose path intentionally does NOT appear in the spec.
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "recipes_search.go"), `package cli
func recipesSearch() {
	path := "/unrelated/endpoint"
	_ = path
}
`)
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "root.go"), `package cli
func main() {}
`)

	// Synthetic spec: primary endpoint is /widgets but the CLI surface is broader.
	specPath := filepath.Join(dir, "spec.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte(`name: recipe-goat
version: "0.1.0"
kind: synthetic
base_url: "https://api.example.com"
auth:
  type: api_key
  header: Authorization
  format: "Bearer {token}"
  env_vars: [RECIPE_GOAT_TOKEN]
config:
  format: toml
  path: "~/.config/recipe-goat-pp-cli/config.toml"
resources:
  widgets:
    description: "Widgets"
    endpoints:
      list:
        method: GET
        path: "/widgets"
        description: "List widgets"
`), 0o644))

	report, err := RunDogfood(dir, specPath)
	require.NoError(t, err)

	require.True(t, report.PathCheck.Skipped,
		"synthetic spec should mark path check as skipped")
	require.Contains(t, report.PathCheck.Detail, "synthetic",
		"skip reason should name the cause")
	require.Zero(t, report.PathCheck.Tested,
		"skipped check should not report Tested > 0")
	require.Empty(t, report.PathCheck.Invalid)
}

// TestRESTSpec_DogfoodStillChecksPaths originally guarded against over-broad
// application of the synthetic-spec path-validity skip. After hackernews retro
// #350 finding F8, internal-yaml specs (synthetic OR plain REST) skip the
// path-validity check by default — the OpenAPI-derived path matcher produces
// "0/0 valid (FAIL)" against perfectly valid internal-yaml CLIs while the
// scorecard's parallel check correctly records the dimension as unscored. This
// test now asserts the broader skip behavior; OpenAPI specs (a separate
// fixture flow) still run the check.
func TestRESTSpec_DogfoodStillChecksPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0o755))

	writeTestFile(t, filepath.Join(dir, "internal", "cli", "widgets_list.go"), `package cli
func widgetsList() {
	path := "/widgets"
	_ = path
}
`)
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "root.go"), `package cli
func main() {}
`)

	specPath := filepath.Join(dir, "spec.yaml")
	// No kind field — defaults to rest behavior.
	require.NoError(t, os.WriteFile(specPath, []byte(`name: regular-api
version: "0.1.0"
base_url: "https://api.example.com"
auth:
  type: api_key
  header: Authorization
  format: "Bearer {token}"
  env_vars: [REGULAR_API_TOKEN]
config:
  format: toml
  path: "~/.config/regular-api-pp-cli/config.toml"
resources:
  widgets:
    description: "Widgets"
    endpoints:
      list:
        method: GET
        path: "/widgets"
        description: "List widgets"
`), 0o644))

	report, err := RunDogfood(dir, specPath)
	require.NoError(t, err)

	// Internal-yaml specs (synthetic or plain REST) skip the OpenAPI-shaped
	// path check. Confirms the broader skip from retro #350 finding F8.
	require.True(t, report.PathCheck.Skipped,
		"internal-yaml rest spec should skip path-validity")
	require.Contains(t, report.PathCheck.Detail, "internal-yaml",
		"detail should explain the skip reason")
}

// TestSyntheticSpec_ScorecardMarksPathValidityUnscored verifies scorecard
// excludes PathValidity from the tier-2 denominator for synthetic specs
// rather than awarding a free 10-point cushion. Regression guard for #203.
func TestSyntheticSpec_ScorecardMarksPathValidityUnscored(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "cli"), 0o755))
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "recipes_search.go"), `package cli
func recipesSearch() {
	path := "/unrelated/endpoint"
	_ = path
}
`)
	writeTestFile(t, filepath.Join(dir, "internal", "cli", "root.go"), `package cli
func main() {}
`)

	specPath := filepath.Join(dir, "spec.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte(`name: recipe-goat
version: "0.1.0"
kind: synthetic
base_url: "https://api.example.com"
auth:
  type: api_key
  header: Authorization
  format: "Bearer {token}"
  env_vars: [RECIPE_GOAT_TOKEN]
config:
  format: toml
  path: "~/.config/recipe-goat-pp-cli/config.toml"
resources:
  widgets:
    description: "Widgets"
    endpoints:
      list:
        method: GET
        path: "/widgets"
        description: "List widgets"
`), 0o644))

	sc, err := RunScorecard(dir, t.TempDir(), specPath, nil)
	require.NoError(t, err)
	require.True(t, sc.IsDimensionUnscored("path_validity"),
		"synthetic spec should mark path_validity as unscored so the tier-2 denominator excludes it")
	require.Zero(t, sc.Steinberger.PathValidity,
		"unscored dimension should not carry a score")
}
