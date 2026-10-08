package client

import (
	"net/http"
	"testing"
)

func TestDropPseudoHeadersPreservesAPIHeaders(t *testing.T) {
	headers := http.Header{
		":authority": {""}, ":method": {""}, ":path": {""}, ":scheme": {""},
		"Authorization": {"synthetic"}, "Content-Type": {"application/json"},
	}
	dropPseudoHeaders(headers)
	for _, name := range []string{":authority", ":method", ":path", ":scheme"} {
		if _, exists := headers[name]; exists {
			t.Fatalf("pseudo-header %s remains", name)
		}
	}
	if headers.Get("Authorization") != "synthetic" || headers.Get("Content-Type") != "application/json" {
		t.Fatal("API headers changed")
	}
}
