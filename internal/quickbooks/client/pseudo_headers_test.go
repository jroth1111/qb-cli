package client

import (
	"net/http"
	"testing"
)

func TestDropPseudoHeadersPreservesAPIHeaders(t *testing.T) {
	headers := http.Header{":authority": {""}, ":method": {""}, ":path": {""}, ":scheme": {""}, "Authorization": {"synthetic"}, "Content-Type": {"application/json"}}
	dropPseudoHeaders(headers)
	for name := range headers {
		if len(name) > 0 && name[0] == ':' {
			t.Fatalf("pseudo-header remains: %s", name)
		}
	}
	if headers.Get("Authorization") != "synthetic" || headers.Get("Content-Type") != "application/json" {
		t.Fatal("API headers changed")
	}
}
