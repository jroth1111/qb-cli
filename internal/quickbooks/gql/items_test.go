package gql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestItemsWalkPreservesVariantsAndStopsAtNativeTotal(t *testing.T) {
	req := ItemsRequest()
	if err := req.Op.ValidateVars(req.Variables); err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := walk(context.Background(), req, Walker{PageSize: 1, MaxPages: 10, NodesPath: []string{"data", "result", "entities"}}, func(_ context.Context, r Request) (*Response, error) {
		offset, _ := toInt(r.Variables["offset"])
		if offset != int64(calls) {
			t.Fatalf("offset=%d at request %d", offset, calls)
		}
		calls++
		return &Response{Status: 200, Body: json.RawMessage(fmt.Sprintf(`{"data":{"result":{"entities":[{"productsAndServicesListEntity":{"id":"%d","entityName":"fixture"},"variants":[{"qboItemId":"8"}]}],"totalCount":2}}}`, calls))}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(result.Nodes) != 2 || result.HasNext || result.Truncated {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
	if !strings.Contains(string(result.Nodes[0]), `"qboItemId":"8"`) {
		t.Fatal("lost variant")
	}
}

func TestItemsWalkRejectsMissingOrMalformedNativeCollection(t *testing.T) {
	for _, body := range []string{`{"data":{}}`, `{"data":{"result":{"entities":null}}}`, `{"data":{"result":{"entities":{}}}}`} {
		_, err := walk(context.Background(), ItemsRequest(), Walker{NodesPath: []string{"data", "result", "entities"}}, func(context.Context, Request) (*Response, error) {
			return &Response{Status: 200, Body: json.RawMessage(body)}, nil
		})
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestEmptyWalkEncodesJSONArray(t *testing.T) {
	result, err := walk(context.Background(), ItemsRequest(), Walker{NodesPath: []string{"data", "result", "entities"}}, func(context.Context, Request) (*Response, error) {
		return &Response{Status: 200, Body: json.RawMessage(`{"data":{"result":{"entities":[],"totalCount":0}}}`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result.Nodes)
	if string(b) != "[]" {
		t.Fatalf("empty output=%s", b)
	}
}
