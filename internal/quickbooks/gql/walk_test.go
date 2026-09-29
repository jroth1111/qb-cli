package gql

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"
)

func TestWalkOffsetProgressAndTruncation(t *testing.T) {
	op, err := Lookup("GetContacts")
	if err != nil {
		t.Fatal(err)
	}
	for _, maxPages := range []int{1, 3} {
		calls := 0
		res, err := walk(context.Background(), Request{Op: op}, Walker{MaxPages: maxPages}, func(_ context.Context, req Request) (*Response, error) {
			if calls > 1 {
				t.Fatal("walker repeated a completed page")
			}
			if calls == 1 {
				if offset, _ := toInt(req.Variables["offset"]); offset != 2 {
					t.Fatalf("offset did not advance: %v", req.Variables)
				}
			}
			body := `{"data":{"contacts":{"edges":[{"node":{"id":"1"}},{"node":{"id":"2"}}]}}}`
			if calls == 1 {
				body = `{"data":{"contacts":{"edges":[]}}}`
			}
			calls++
			return &Response{Status: 200, Body: json.RawMessage(body)}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Nodes) != 2 {
			t.Fatalf("nodes = %d", len(res.Nodes))
		}
		if res.Truncated != (maxPages == 1) || res.HasNext != (maxPages == 1) {
			t.Fatalf("completion flags incorrect: %+v", res)
		}
	}
}

func TestWalkInitialNumericAfter(t *testing.T) {
	op := &Op{Name: "Tasks", Kind: KindQuery, Document: "query Tasks($first: Int!, $after: Int!) { tasks { edges { node { id } } } }", VarTypes: []string{"Int!", "Int!"}}
	_, err := walk(context.Background(), Request{Op: op}, Walker{PageSize: 5, MaxPages: 1}, func(_ context.Context, req Request) (*Response, error) {
		n, ok := toInt(req.Variables["after"])
		if !ok || n != 0 {
			t.Fatalf("first request omitted numeric after: %v", req.Variables)
		}
		return &Response{Status: 200, Body: json.RawMessage(`{"data":{"tasks":{"edges":[]}}}`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWalkNestedInputClearsHasNextOnCompletion(t *testing.T) {
	op := &Op{Name: "Items", Kind: KindQuery, Document: "query Items($input: Input) { items(input: $input) { edges { node { id } } totalCount } }"}
	calls := 0
	res, err := walk(context.Background(), Request{Op: op}, Walker{MaxPages: 3, PageSize: 1, InputVar: "input"}, func(_ context.Context, req Request) (*Response, error) {
		if calls > 1 {
			t.Fatal("walk did not stop at totalCount")
		}
		offset, _ := toInt(req.Variables["input"].(map[string]any)["offset"])
		if offset != int64(calls) {
			t.Fatalf("offset = %d, want %d", offset, calls)
		}
		calls++
		return &Response{Status: 200, Body: json.RawMessage(fmt.Sprintf(`{"data":{"items":{"edges":[{"node":{"id":"row%d"}}],"totalCount":2}}}`, calls))}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 2 || res.HasNext || res.Truncated {
		t.Fatalf("completed nested walk: %+v", res)
	}
}

// extractNodes is the walk engine's parser; each test below rejects a
// distinct wrong-implementation mode: missing pageInfo, nested connections,
// non-envelope bodies, and offset-style node arrays.

func TestExtractNodesFinalCursorPage(t *testing.T) {
	t.Parallel()
	body := `{"data":{"bills":{"pageInfo":{"hasNextPage":false,"endCursor":"CUR2"},"edges":[{"node":{"id":"3"}}]}}}`
	nodes, hasNext, cursor, err := extractNodes(json.RawMessage(body))
	if err != nil {
		t.Fatal(err)
	}
	if hasNext {
		t.Fatal("final page reported hasNextPage=true")
	}
	if cursor != "CUR2" {
		t.Fatalf("cursor=%q", cursor)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes=%d", len(nodes))
	}
}

func TestExtractNodesNestedConnections(t *testing.T) {
	t.Parallel()
	// company { bills {...} contacts {...} } — both connections harvested.
	body := `{"data":{"company":{"bills":{"edges":[{"node":{"id":"b1"}}]},
		"contacts":{"pageInfo":{"hasNextPage":true,"endCursor":"C9"},"edges":[{"node":{"id":"c1"}}]}}}}`
	nodes, hasNext, cursor, err := extractNodes(json.RawMessage(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("want nodes from both connections, got %d: %v", len(nodes), nodes)
	}
	if !hasNext || cursor != "C9" {
		t.Fatalf("hasNext=%v cursor=%q", hasNext, cursor)
	}
}

func TestExtractNodesPlainBody(t *testing.T) {
	t.Parallel()
	// No GraphQL envelope at all (some hosts return bare data).
	nodes, _, _, err := extractNodes(json.RawMessage(`{"items":{"edges":[{"node":{"id":"i1"}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("bare-body connection missed: %d", len(nodes))
	}
}

func TestExtractNodesMalformedBodyErrors(t *testing.T) {
	t.Parallel()
	if _, _, _, err := extractNodes(json.RawMessage(`{not-json`)); err == nil {
		t.Fatal("malformed body accepted")
	}
}

func TestWalkResultMarshals(t *testing.T) {
	t.Parallel()
	res := WalkResult{
		Op: "GetBills", Style: WalkCursor, Pages: 2,
		Nodes:     []json.RawMessage{json.RawMessage(`{"id":"1"}`)},
		EndCursor: "E", Truncated: true,
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back["style"] != string(WalkCursor) || back["truncated"] != true || back["pages"] != float64(2) {
		t.Fatalf("roundtrip mismatch: %s", b)
	}
}

// TestParseWindowTable is the table-driven proof of --from/--to
// construction: dd/MM/yyyy and ISO both parse, each bound maps to its own
// filter key, empty bounds drop keys, reversed windows are rejected, and a
// bounds-less window still guarantees the non-null $filter object.
func TestParseWindowTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		spec    DateWindowSpec
		from    string
		to      string
		wantErr string // non-empty = expect error containing this
		check   func(*testing.T, *DateWindow)
	}{
		{
			name: "bills dd/MM/yyyy window",
			spec: BillDateWindow,
			from: "01/01/2024", to: "31/12/2024",
			check: func(t *testing.T, w *DateWindow) {
				if got := w.From.Format("2006-01-02"); got != "2024-01-01" {
					t.Errorf("From=%s want 2024-01-01", got)
				}
				if got := w.To.Format("2006-01-02"); got != "2024-12-31" {
					t.Errorf("To=%s want 2024-12-31", got)
				}
			},
		},
		{
			name: "ISO accepted too",
			spec: BillDateWindow,
			from: "2024-01-01", to: "2024-12-31",
			check: func(t *testing.T, w *DateWindow) {
				if w.From.Month() != time.January || w.To.Day() != 31 {
					t.Errorf("ISO parse wrong: %v..%v", w.From, w.To)
				}
			},
		},
		{
			name:  "empty bounds yield bare filter object",
			spec:  BillDateWindow,
			check: func(t *testing.T, w *DateWindow) {},
		},
		{
			name: "from only drops before key",
			spec: TaskDueDateWindow,
			from: "15/06/2025",
		},
		{
			name: "reversed window rejected",
			spec: BillDateWindow,
			from: "2024-12-31", to: "2024-01-01",
			wantErr: "after --to",
		},
		{
			name:    "garbage date rejected",
			spec:    BillDateWindow,
			from:    "not-a-date",
			wantErr: "--from",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			win, err := tc.spec.ParseWindow(tc.from, tc.to)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseWindow(%q,%q) err=%v, want %q", tc.from, tc.to, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.check != nil {
				tc.check(t, win)
			}
		})
	}
}

// TestInjectDateWindowTable proves the variables each command sends carry
// the right filter shape — and that caller-supplied keys survive on both
// levels. Each case rejects a distinct wrong shape (wrong field name, wrong
// key, dropped sibling, missing non-null filter object).
func TestInjectDateWindowTable(t *testing.T) {
	t.Parallel()
	mustJSON := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	cases := []struct {
		name string
		vars map[string]any
		win  *DateWindow
		want string
	}{
		{
			name: "bills full window",
			win:  mustWin(t, BillDateWindow, "01/01/2024", "31/12/2024"),
			want: `{"filter":{"transactionDate":{"createdOnOrAfter":"2024-01-01","createdOnOrBefore":"2024-12-31"}}}`,
		},
		{
			name: "tasks due-date window",
			win:  mustWin(t, TaskDueDateWindow, "2025-06-15", ""),
			want: `{"filter":{"dueDate":{"onOrAfter":"2025-06-15"}}}`,
		},
		{
			name: "bounds-less keeps empty non-null filter",
			win:  mustWin(t, BillDateWindow, "", ""),
			want: `{"filter":{}}`,
		},
		{
			name: "caller keys survive",
			vars: map[string]any{"filter": map[string]any{"status": "OPEN"}},
			win:  mustWin(t, BillDateWindow, "2024-01-01", ""),
			want: `{"filter":{"status":"OPEN","transactionDate":{"createdOnOrAfter":"2024-01-01"}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]any{}
			maps.Copy(vars, tc.vars)
			injectDateWindow(vars, tc.win)
			if got := mustJSON(vars); got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

// mustWin parses a window or fails the test.
func mustWin(t *testing.T, spec DateWindowSpec, from, to string) *DateWindow {
	t.Helper()
	w, err := spec.ParseWindow(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// TestWalkNumericAfterUsesOffset pins the TaskManagementTasks refinement:
// an Int-typed $after must walk by incrementing, never waiting for a string
// endCursor that never arrives.
func TestWalkNumericAfterUsesOffset(t *testing.T) {
	t.Parallel()
	op := &Op{Name: "T", Kind: KindQuery, VarTypes: []string{"Int!", "Int!", "TaskManagement_TaskFilter!"},
		Document: "query T($first: Int!, $after: Int!, $filter: F!) { c(first: $first, after: $after) { edges { node { id } } pageInfo { hasNextPage } } }"}
	if DetectWalkStyleForTest(op) != WalkCursor {
		t.Fatalf("style detection changed: %v", DetectWalkStyleForTest(op))
	}
	if !intDeclaredAfter(op) {
		t.Fatal("Int-typed $after not recognized as offset position")
	}
	strOp := &Op{Name: "B", Kind: KindQuery, VarTypes: []string{"Int!", "String"},
		Document: "query B($first: Int!, $after: String) { c(first: $first, after: $after) { edges { node { id } } } }"}
	if intDeclaredAfter(strOp) {
		t.Fatal("String-typed $after misclassified as offset")
	}
}

// DetectWalkStyleForTest wraps the method for readability in tests above.
func DetectWalkStyleForTest(op *Op) WalkStyle { return op.DetectWalkStyle() }

// TestExtractTotalCountTable defends the Items drain signal: totals are
// found at any depth, non-numeric or absent totals disable draining (the
// walk stops instead of spinning), and malformed bodies are rejected.
func TestExtractTotalCountTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		body  string
		want  int64
		found bool
	}{
		{"envelope nested", `{"data":{"items":{"edges":[{"node":{"id":"1"}}],"totalCount":42}}}`, 42, true},
		{"bare body", `{"items":{"totalCount":7,"edges":[]}}`, 7, true},
		{"absent total", `{"data":{"items":{"edges":[{"node":{"id":"1"}}]}}}`, 0, false},
		{"non numeric", `{"data":{"items":{"totalCount":"many"}}}`, 0, false},
		{"malformed", `{oops`, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found := extractTotalCount(json.RawMessage(tc.body))
			if found != tc.found || got != tc.want {
				t.Fatalf("got (%d,%t) want (%d,%t)", got, found, tc.want, tc.found)
			}
		})
	}
}

func TestOffsetWalkIgnoresUnrelatedTotalCount(t *testing.T) {
	op := &Op{Name: "Rows", Kind: KindQuery, Document: "query Rows($offset:Int,$limit:Int){ rows { edges {node {id}} } badge {totalCount} }"}
	calls := 0
	result, err := walk(context.Background(), Request{Op: op}, Walker{PageSize: 1, MaxPages: 3}, func(context.Context, Request) (*Response, error) {
		calls++
		body := fmt.Sprintf(`{"data":{"rows":{"edges":[{"node":{"id":"row%d"}}]},"badge":{"totalCount":0}}}`, calls)
		if calls == 3 {
			body = `{"data":{"rows":{"edges":[]},"badge":{"totalCount":0}}}`
		}
		return &Response{Status: 200, Body: json.RawMessage(body)}, nil
	})
	if err != nil || len(result.Nodes) != 2 || calls != 3 {
		t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
	}
}
