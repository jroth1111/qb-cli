package client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAuditResponseOverOrdinaryBodyLimit(t *testing.T) {
	raw := `{"logs":[{"what":{"qboAuditId":"a1"}}],"snapshot":"` + strings.Repeat("x", (1<<20)+100) + `"}`
	b, err := readAuditBody(&http.Response{Body: io.NopCloser(strings.NewReader(raw))})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := extractAuditEvents(b)
	if !ok || len(rows) != 1 {
		t.Fatal("large audit response was truncated")
	}
}
func TestAuditResponseLimitFailsExplicitly(t *testing.T) {
	_, err := readAuditBody(&http.Response{Body: io.NopCloser(strings.NewReader(strings.Repeat("x", (16<<20)+1)))})
	if err == nil {
		t.Fatal("oversized audit body was silently accepted")
	}
}
