package cli

import (
	"bytes"
	"fmt"
	"regexp"
)

// httpErrorBodyPattern matches the raw-content 404 / 5xx body GitHub serves
// when a path is missing ("404: Not Found", "500: Internal Server Error").
// Catches stale spec_urls whose download silently writes the error body to
// disk and feeds it to the parser, where it surfaces several layers down as
// a misleading "validation: name is required" or similar error.
var httpErrorBodyPattern = regexp.MustCompile(`^[0-9]{3}: `)

// rejectIfNotSpec returns a descriptive error when data plainly looks like an
// HTTP error response or an HTML page rather than an OpenAPI / GraphQL / spec
// document. Heuristic by design: only catches signatures that no legitimate
// spec would produce. A real spec failing the parser is allowed through to
// produce the parser's own error message; this gate fires only when the
// content was never a spec in the first place.
func rejectIfNotSpec(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if httpErrorBodyPattern.Match(trimmed) {
		return fmt.Errorf("content looks like an HTTP error response, not a spec: %q", firstLineSnippet(trimmed))
	}
	lower := bytes.ToLower(trimmed)
	if bytes.HasPrefix(lower, []byte("<html")) || bytes.HasPrefix(lower, []byte("<!doctype html")) {
		return fmt.Errorf("content looks like an HTML page, not a spec")
	}
	return nil
}

// firstLineSnippet returns the first line of data, capped at 80 bytes, for
// inclusion in error messages. Avoids dumping a full HTML page back at the
// user when surfacing the rejection.
func firstLineSnippet(data []byte) string {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data = data[:i]
	}
	if len(data) > 80 {
		data = data[:80]
	}
	return string(data)
}
