package generator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanForControlBytes_AllowsNormalText verifies that ordinary markdown
// content with no control bytes passes the scan unchanged.
func TestScanForControlBytes_AllowsNormalText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
	}{
		{"plain prose", "# Hello\n\nThis is a normal SKILL.md."},
		{"with tab", "| col1\tcol2 |\n"},
		{"with CRLF", "line1\r\nline2"},
		{"unicode", "—curly quotes — em dash — ✓"},
		{"backticks and pipes", "`hackernews-pp-cli stories top` | `--limit N` |"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, scanForControlBytes("SKILL.md", tc.content))
		})
	}
}

// TestScanForControlBytes_RejectsBackspace is the regression guard for
// the exact bug surfaced by hackernews retro #350: a recipe's regex
// `\bGo\b` parsed by JSON as backspace bytes leaked into the rendered
// SKILL.md. The scanner must reject this with a clear error pointing
// at the offset and explaining the likely cause.
func TestScanForControlBytes_RejectsBackspace(t *testing.T) {
	t.Parallel()

	content := "Recipe: `hackernews-pp-cli hiring '(remote).*\bGo\b'`"
	err := scanForControlBytes("SKILL.md", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SKILL.md")
	assert.Contains(t, err.Error(), "0x08")
	// The offset of the first \b in the string above (after "remote).*"):
	expectedOffset := strings.Index(content, "\b")
	require.GreaterOrEqual(t, expectedOffset, 0)
	assert.Contains(t, err.Error(), "JSON escape",
		"error should hint at the JSON-escape root cause")
	assert.Contains(t, err.Error(), "Double-escape backslashes",
		"error should give the actionable fix")
}

// TestScanForControlBytes_RejectsAllForbiddenBytes verifies every byte
// in 0x00-0x1F (except 0x09, 0x0A, 0x0D) triggers the rejection.
func TestScanForControlBytes_RejectsAllForbiddenBytes(t *testing.T) {
	t.Parallel()

	for b := 0; b <= 0x1F; b++ {
		if b == 0x09 || b == 0x0A || b == 0x0D {
			continue // explicitly allowed
		}
		content := "before" + string(rune(b)) + "after"
		err := scanForControlBytes("SKILL.md", content)
		assert.Error(t, err, "byte 0x%02X should be rejected", b)
	}
}

// TestScanForControlBytes_AllowsAllowedControlBytes verifies the
// explicit allowlist (tab, newline, CR) passes the scan.
func TestScanForControlBytes_AllowsAllowedControlBytes(t *testing.T) {
	t.Parallel()

	for _, b := range []byte{0x09, 0x0A, 0x0D} {
		content := "before" + string(rune(b)) + "after"
		require.NoError(t, scanForControlBytes("SKILL.md", content),
			"byte 0x%02X should be allowed", b)
	}
}

// TestScanForControlBytes_OnlyMarkdown verifies the scanner is wired
// only into README.md / SKILL.md via validateRenderedArtifact — other
// rendered files (Go source, JSON manifests) are not subject to this
// check because they have their own validation paths.
func TestScanForControlBytes_OnlyMarkdown(t *testing.T) {
	t.Parallel()

	contentWithBackspace := "before\bafter"
	// validateRenderedArtifact dispatches by filename — non-markdown
	// files should pass without scanning.
	require.NoError(t, validateRenderedArtifact("internal/cli/root.go", contentWithBackspace),
		"Go source files are out of scope")
	require.NoError(t, validateRenderedArtifact("tools-manifest.json", contentWithBackspace),
		"JSON manifests are out of scope")
	// Markdown files trigger the scan.
	require.Error(t, validateRenderedArtifact("README.md", contentWithBackspace))
	require.Error(t, validateRenderedArtifact("SKILL.md", contentWithBackspace))
}

// TestScanForControlBytes_OffsetIsAccurate verifies the error message
// names the byte offset of the first forbidden byte, not the last.
// Implementers fixing the input need to find the right field.
func TestScanForControlBytes_OffsetIsAccurate(t *testing.T) {
	t.Parallel()

	content := "0123456789\babcdef\bghi"
	err := scanForControlBytes("SKILL.md", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "offset 10",
		"should report the offset of the first \\b, not subsequent ones")
}
