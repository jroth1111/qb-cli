package gql

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestWalkRejectsNonAdvancingPages(t *testing.T) {
	for _, cursor := range []bool{false, true} {
		t.Run(fmt.Sprint(cursor), func(t *testing.T) {
			doc := "query Rows($offset: Int, $limit: Int) { rows { edges { node { id } } } }"
			if cursor {
				doc = "query Rows($after: String, $first: Int) { rows { edges { node { id } } pageInfo {hasNextPage endCursor} } }"
			}
			op := &Op{Name: "Rows", Kind: KindQuery, Document: doc}
			calls := 0
			res, err := walk(t.Context(), Request{Op: op}, Walker{MaxPages: 3, PageSize: 1}, func(context.Context, Request) (*Response, error) {
				calls++
				return &Response{Status: 200, Body: json.RawMessage(`{"data":{"rows":{"edges":[{"node":{"id":"1"}}],"pageInfo":{"hasNextPage":true,"endCursor":"same"}}}}`)}, nil
			})
			if err == nil || res == nil || !res.Truncated || calls != 2 {
				t.Fatalf("stalled walk: %+v, %v, calls=%d", res, err, calls)
			}
		})
	}
}

func TestWalkRejectsMissingContinuation(t *testing.T) {
	op := &Op{Name: "Rows", Kind: KindQuery, Document: "query Rows($after: String) { rows { edges { node { id } } pageInfo {hasNextPage endCursor} } }"}
	res, err := walk(t.Context(), Request{Op: op}, Walker{}, func(context.Context, Request) (*Response, error) {
		return &Response{Status: 200, Body: json.RawMessage(`{"data":{"rows":{"edges":[{"node":{"id":"1"}}],"pageInfo":{"hasNextPage":true}}}}`)}, nil
	})
	if err == nil || res == nil || !res.Truncated {
		t.Fatalf("missing cursor reported complete: %+v, %v", res, err)
	}
}

func TestWalkAdvancesByReturnedRows(t *testing.T) {
	op := &Op{Name: "Rows", Kind: KindQuery, Document: "query Rows($offset: Int, $limit: Int) { rows { edges { node { id } } } }"}
	calls := 0
	_, err := walk(t.Context(), Request{Op: op}, Walker{PageSize: 100}, func(_ context.Context, req Request) (*Response, error) {
		offset, _ := toInt(req.Variables["offset"])
		if offset != int64(calls) {
			t.Errorf("skipped capped rows: offset=%d want=%d", offset, calls)
		}
		calls++
		body := fmt.Sprintf(`{"data":{"rows":{"edges":[{"node":{"id":"%d"}}]}}}`, calls)
		if calls == 3 {
			body = `{"data":{"rows":{"edges":[]}}}`
		}
		return &Response{Status: 200, Body: json.RawMessage(body)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWalkRejectsHTTPFailure(t *testing.T) {
	op := &Op{Name: "Rows", Kind: KindQuery, Document: "query Rows { rows {edges {node {id}}} }"}
	res, err := walk(t.Context(), Request{Op: op}, Walker{}, func(context.Context, Request) (*Response, error) {
		return &Response{Status: 403, Body: json.RawMessage(`{"message":"denied"}`)}, nil
	})
	if err == nil || res == nil || !res.Truncated {
		t.Fatalf("HTTP failure reported complete: %+v, %v", res, err)
	}
}
