package client

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestReadRetryDoesNotExposePartialJSONOrReplayWrites(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		req, _ := http.NewRequest(method, "https://example.test", nil)
		calls := 0
		resp, err := readReliably(req, func(*http.Request) (*http.Response, error) {
			calls++
			body := `{"items":[`
			if calls > 1 {
				body = `{"items":[{"id":"1"}]}`
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		want := 1
		if method == "GET" {
			want = 2
		}
		if calls != want {
			t.Fatalf("%s calls=%d", method, calls)
		}
	}
}

func TestReadRetryRefusesControlAndHasBoundedNetworkBudget(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.test", nil)
	calls := 0
	_, _ = readReliably(req, func(*http.Request) (*http.Response, error) { calls++; return nil, auth.ErrEgoUserControl })
	if calls != 1 {
		t.Fatal("user control retried")
	}
	calls = 0
	_, err := readReliably(req, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
	if err == nil || calls != 3 {
		t.Fatalf("unbounded retry %d: %v", calls, err)
	}
}

func TestReadRetryNativeLiteralExceptionIsRouteSpecific(t *testing.T) {
	for _, path := range []string{"/getTxnData", "/query"} {
		req, _ := http.NewRequest("GET", "https://example.test"+path, nil)
		calls := 0
		resp, err := readReliably(req, func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"date":new Date(2026,1,1)}`))}, nil
		})
		if path == "/getTxnData" {
			if err != nil || calls != 1 {
				t.Fatalf("native literal calls=%d err=%v", calls, err)
			}
			_ = resp.Body.Close()
		} else if err == nil || calls != 3 {
			t.Fatalf("invalid JSON escaped strict query validation: calls=%d err=%v", calls, err)
		}
	}
}

func TestReadRetryBuffersArraysBeforePagerCanCommit(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.test/query", nil)
	calls := 0
	resp, err := readReliably(req, func(*http.Request) (*http.Response, error) {
		calls++
		body := `[{"id":"1"},`
		if calls > 1 {
			body = `[{"id":"1"}]`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(raw) != `[{"id":"1"}]` {
		t.Fatalf("exposed partial page: %s", raw)
	}
}
