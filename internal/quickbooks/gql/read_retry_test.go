package gql

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestQueryRetriesLossWithoutReplayingMutation(t *testing.T) {
	for _, kind := range []string{"query", "mutation", "unknown"} {
		calls := 0
		req := Request{Op: &Op{Kind: kind, Document: "query Fixture { field }"}}
		_, _ = executeReadReliably(context.Background(), req, func(context.Context, Request) (*Response, error) {
			calls++
			if calls == 1 {
				return nil, io.ErrUnexpectedEOF
			}
			return &Response{Status: 200, Body: json.RawMessage(`{"data":{"field":1}}`)}, nil
		})
		want := 1
		if kind == "query" {
			want = 2
		}
		if calls != want {
			t.Fatalf("%s calls=%d", kind, calls)
		}
	}
}

func TestQueryRetryRejectsControlAndFalseKind(t *testing.T) {
	for _, doc := range []string{"query Fixture { field }", "mutation Fixture { field }"} {
		calls := 0
		_, _ = executeReadReliably(context.Background(), Request{Op: &Op{Kind: "query", Document: doc}}, func(context.Context, Request) (*Response, error) { calls++; return nil, auth.ErrEgoUserControl })
		if calls != 1 {
			t.Fatal("control or mutation document retried")
		}
	}
}

func TestQueryPacketLossDoesNotDoubleCountWalkedPage(t *testing.T) {
	op := &Op{Name: "Rows", Kind: "query", Document: "query Rows($offset: Int) { rows { edges {node {id}}} }"}
	calls := 0
	res, err := walk(t.Context(), Request{Op: op}, Walker{}, func(ctx context.Context, req Request) (*Response, error) {
		return executeReadReliably(ctx, req, func(_ context.Context, req Request) (*Response, error) {
			calls++
			offset, _ := toInt(req.Variables["offset"])
			if calls == 2 {
				return &Response{Status: 200, Body: json.RawMessage(`{"data":{"rows":{"edges":[`)}, nil
			}
			body := `{"data":{"rows":{"edges":[]}}}`
			if offset == 0 {
				body = `{"data":{"rows":{"edges":[{"node":{"id":"1"}}]}}}`
			}
			if offset == 1 {
				body = `{"data":{"rows":{"edges":[{"node":{"id":"2"}}]}}}`
			}
			return &Response{Status: 200, Body: json.RawMessage(body)}, nil
		})
	})
	if err != nil || res.Truncated || len(res.Nodes) != 2 || res.Pages != 3 || calls != 4 {
		t.Fatalf("replayed page counted twice: %+v calls=%d err=%v", res, calls, err)
	}
}
