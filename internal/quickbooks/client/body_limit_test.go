package client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNativeBodyLargerThanOneMiBIsComplete(t *testing.T) {
	want := `{"padding":"` + strings.Repeat("x", 2<<20) + `"}`
	raw, err := readBody(&http.Response{Body: io.NopCloser(strings.NewReader(want))})
	if err != nil || string(raw) != want {
		t.Fatalf("native body truncated: bytes=%d err=%v", len(raw), err)
	}
}

func TestNativeBodyBudgetRefusesPartialPayload(t *testing.T) {
	raw, err := readBoundedBody(io.LimitReader(zeroReader{}, maxResponseBytes+1))
	if err == nil || raw != nil {
		t.Fatal("oversized response exposed partial bytes")
	}
}

type zeroReader struct{}

func (zeroReader) Read(b []byte) (int, error) { clear(b); return len(b), nil }
