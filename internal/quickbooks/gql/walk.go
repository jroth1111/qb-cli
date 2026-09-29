package gql

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"
)

// WalkResult is the merged output of a pagination walk.
type WalkResult struct {
	Op        string            `json:"op"`
	Style     WalkStyle         `json:"style"`
	Pages     int               `json:"pages"`
	Nodes     []json.RawMessage `json:"nodes"`
	HasNext   bool              `json:"hasNextPage"`
	EndCursor string            `json:"endCursor,omitempty"`
	Truncated bool              `json:"truncated"` // stopped at MaxPages with more remaining
	// Errors collects GraphQL-level error messages from every page fetch
	// (transport failures abort the walk instead). Non-empty means the
	// merged node set is whatever the server managed to return — never a
	// complete result.
	Errors []string `json:"errors,omitempty"`
}

// Walker executes paged operations via the relay transport.
type Walker struct {
	MaxPages int // hard ceiling on pages fetched (0 = unlimited)
	PageSize any // value bound to $first/$limit per page when non-nil
	// InputVar names the object variable that page size and offset mirror
	// into when an operation pages through a nested input (Items'
	// getItemsInput.limit/offset) instead of top-level variables.
	InputVar string
	// NodesPath selects a native array for APIs that do not use edges/node.
	NodesPath []string
	// Window, when non-nil, is injected into the $filter variable before
	// the walk: filter[Field][AfterKey] / [BeforeKey] carry the ISO forms
	// of From/To. Zero bounds drop their key; a fully-zero window still
	// guarantees the filter object non-null schemas require. Filter keys
	// the caller supplied survive on both levels.
	Window *DateWindow
}

// Walk fetches successive pages of op and merges edges[].node arrays.
//
//   - cursor: binds $after from pageInfo.endCursor until hasNextPage=false;
//     an Int/Long-typed $after is an offset position and walks as offset
//   - offset: increments $offset/$skip by the page size
//   - single: one fetch — or, when InputVar names a nested paging input
//     (Items: getItemsInput), successive offsets until totalCount drains
func Walk(ctx context.Context, req Request, w Walker) (*WalkResult, error) {
	return walk(ctx, req, w, Execute)
}

func walk(ctx context.Context, req Request, w Walker, execute func(context.Context, Request) (*Response, error)) (*WalkResult, error) {
	style := req.Op.DetectWalkStyle()
	// A numerically-typed $after (TaskManagementTasks declares Int!) is a
	// page position, not an opaque Relay cursor: no endCursor ever comes
	// back, so such operations must continue by incrementing instead.
	offsetVar := ""
	if style == WalkCursor && intDeclaredAfter(req.Op) {
		style = WalkOffset
		offsetVar = "after"
	}
	res := &WalkResult{Op: req.Op.Name, Style: style, Nodes: []json.RawMessage{}}
	vars := map[string]any{}
	maps.Copy(vars, req.Variables)
	if offsetVar != "" {
		if _, present := vars[offsetVar]; !present {
			vars[offsetVar] = int64(0)
		}
	}
	if w.Window != nil {
		injectDateWindow(vars, w.Window)
	}
	if w.PageSize != nil {
		for _, d := range req.Op.DeclaredVars() {
			switch d {
			case "first", "limit":
				vars[d] = w.PageSize
			}
		}
	}
	if w.InputVar != "" {
		inp := nestedInput(vars, w.InputVar)
		if w.PageSize != nil {
			inp["limit"] = w.PageSize
		}
		inp["offset"] = 0
	}

	seenPages := map[[32]byte]bool{}
	seenCursors := map[string]bool{}
	for page := 0; ; page++ {
		if w.MaxPages > 0 && page >= w.MaxPages {
			res.Truncated = res.HasNext
			break
		}
		resp, err := execute(ctx, Request{Op: req.Op, Variables: vars, Endpoint: req.Endpoint})
		if err != nil {
			res.Truncated = true
			return res, err
		}
		if resp == nil || resp.Status != http.StatusOK {
			res.Truncated = true
			return res, fmt.Errorf("gql: walk %s page %d: unsuccessful HTTP response", req.Op.Name, page+1)
		}
		nodes, hasNext, endCursor, err := extractNodes(resp.Body)
		if len(w.NodesPath) > 0 && len(resp.Errors) == 0 {
			nodes, err = extractNodesPath(resp.Body, w.NodesPath)
		}
		if err != nil {
			res.Truncated = true
			return res, fmt.Errorf("gql: walk %s page %d: %w", req.Op.Name, page+1, err)
		}
		if len(nodes) > 0 {
			b, _ := json.Marshal(nodes)
			sum := sha256.Sum256(b)
			if seenPages[sum] {
				res.Truncated = true
				return res, fmt.Errorf("gql: walk %s: repeated page; result is incomplete", req.Op.Name)
			}
			seenPages[sum] = true
		}
		res.Nodes = append(res.Nodes, nodes...)
		res.Pages++
		for _, ge := range resp.Errors {
			res.Errors = append(res.Errors, ge.Message)
		}
		if len(res.Errors) > 0 {
			res.Truncated = true
			return res, nil
		}
		switch style {
		case WalkCursor:
			res.HasNext = hasNext
			res.EndCursor = endCursor
			if !hasNext {
				return res, nil
			}
			if endCursor == "" || seenCursors[endCursor] {
				res.Truncated = true
				return res, fmt.Errorf("gql: walk %s: missing or repeated continuation cursor", req.Op.Name)
			}
			seenCursors[endCursor] = true
			setVarAny(vars, endCursor, "after", "afterCursor", "cursor")
		case WalkOffset:
			if total, ok := extractTotalCount(resp.Body); len(w.NodesPath) > 0 && ok && int64(len(res.Nodes)) >= total {
				res.HasNext = false
				return res, nil
			}
			res.HasNext = len(nodes) > 0
			if len(nodes) == 0 {
				return res, nil
			}
			// Servers may cap the requested size. Advance by observed rows so
			// the next request cannot skip the unreturned part of a window.
			inc := int64(len(nodes))
			offsetNames := []string{"offset", "skip"}
			if offsetVar != "" {
				offsetNames = []string{offsetVar}
			}
			cur := int64(0)
			for _, n := range offsetNames {
				if v, ok := toInt(vars[n]); ok {
					cur = v
					break
				}
			}
			setVarAny(vars, cur+inc, offsetNames...)
			if w.InputVar != "" {
				inp := nestedInput(vars, w.InputVar)
				c2, _ := toInt(inp["offset"])
				inp["offset"] = c2 + inc
			}
		case WalkSingle:
			res.HasNext = false
			// Limit-only documents are one fetch — unless the op pages
			// through a nested input whose connection selects totalCount
			// (Items): then keep fetching until the total is drained.
			if w.InputVar == "" || len(nodes) == 0 {
				return res, nil
			}
			total, ok := extractTotalCount(resp.Body)
			if !ok || int64(len(res.Nodes)) >= total {
				return res, nil
			}
			res.HasNext = true
			inc := int64(len(nodes))
			inp := nestedInput(vars, w.InputVar)
			cur, _ := toInt(inp["offset"])
			inp["offset"] = cur + inc
		default:
			return res, nil
		}
	}
	return res, nil
}

func extractNodesPath(body json.RawMessage, path []string) ([]json.RawMessage, error) {
	raw := body
	for _, key := range path {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, fmt.Errorf("expected object at %s", key)
		}
		value, ok := object[key]
		if !ok {
			return nil, fmt.Errorf("missing node path field %s", key)
		}
		raw = value
	}
	var nodes []json.RawMessage
	if len(raw) == 0 || raw[0] != '[' {
		return nil, fmt.Errorf("node path must contain an array")
	}
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// setVarAny binds val to the first of names already present in vars, else
// the first non-empty name.
func setVarAny(vars map[string]any, val any, names ...string) {
	for _, n := range names {
		if n == "" {
			continue
		}
		if _, ok := vars[n]; ok {
			vars[n] = val
			return
		}
	}
	for _, n := range names {
		if n != "" {
			vars[n] = val
			return
		}
	}
}

// extractNodes pulls edges[].node arrays out of every connection found in a
// GraphQL envelope, plus the first pageInfo for cursor continuation.
func extractNodes(body json.RawMessage) ([]json.RawMessage, bool, string, error) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	raw := body
	if json.Unmarshal(body, &env) == nil && len(env.Data) > 0 {
		raw = env.Data
	}
	var out []json.RawMessage
	hasNext, endCursor := false, ""
	var scan func(v any) bool
	scan = func(v any) bool {
		m, ok := v.(map[string]any)
		if !ok {
			return true
		}
		if pi, ok := m["pageInfo"].(map[string]any); ok {
			if b, ok := pi["hasNextPage"].(bool); ok {
				hasNext = b
			}
			if s, ok := pi["endCursor"].(string); ok && s != "" {
				endCursor = s
			}
		}
		if edges, ok := m["edges"].([]any); ok {
			for _, e := range edges {
				em, ok := e.(map[string]any)
				if !ok {
					continue
				}
				if node, ok := em["node"]; ok {
					b, err := json.Marshal(node)
					if err == nil {
						out = append(out, b)
					}
				}
			}
			return false // do not descend past an already-harvested connection
		}
		for _, child := range m {
			scan(child)
		}
		return true
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, false, "", err
	}
	scan(data)
	return out, hasNext, endCursor, nil
}

func toInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

// intDeclaredAfter reports whether op declares $after with an Int/Long
// scalar — an integer page position rather than an opaque string cursor.
func intDeclaredAfter(op *Op) bool {
	for i, n := range op.DeclaredVars() {
		if i >= len(op.VarTypes) {
			break
		}
		if n == "after" {
			t := op.VarTypes[i]
			return strings.HasPrefix(t, "Int") || strings.HasPrefix(t, "Long")
		}
	}
	return false
}

// nestedInput returns vars[name] as a map, creating an empty object there
// when absent or mistyped so pagination keys have somewhere to bind.
func nestedInput(vars map[string]any, name string) map[string]any {
	m, _ := vars[name].(map[string]any)
	if m == nil {
		m = map[string]any{}
		vars[name] = m
	}
	return m
}

// extractTotalCount reports the first totalCount found in a GraphQL body —
// the exhaustion signal for connections selecting totals instead of
// pageInfo.hasNextPage.
func extractTotalCount(body json.RawMessage) (int64, bool) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	raw := body
	if json.Unmarshal(body, &env) == nil && len(env.Data) > 0 {
		raw = env.Data
	}
	var data any
	if json.Unmarshal(raw, &data) != nil {
		return 0, false
	}
	total := int64(-1)
	var scan func(v any)
	scan = func(v any) {
		if total >= 0 {
			return
		}
		m, ok := v.(map[string]any)
		if !ok {
			if arr, ok := v.([]any); ok {
				for _, c := range arr {
					scan(c)
				}
			}
			return
		}
		if n, ok := m["totalCount"].(float64); ok {
			total = int64(n)
			return
		}
		for _, c := range m {
			scan(c)
		}
	}
	scan(data)
	if total < 0 {
		return 0, false
	}
	return total, true
}

// cliDateLayouts lists the accepted --from/--to forms: dd/MM/yyyy first,
// ISO yyyy-MM-dd as the shorthand the captured webapp requests use.
var cliDateLayouts = []string{"02/01/2006", "2006-01-02"}

// wireDateLayout is the ISO date form QBO's GraphQL filters expect.
const wireDateLayout = "2006-01-02"

// DateWindowSpec templates where a query nests its date comparisons inside
// $filter: Commerce_BillFilter.transactionDate{createdOnOrAfter,...},
// TaskManagement_TaskFilter.dueDate{onOrAfter,...}.
type DateWindowSpec struct {
	Field     string
	AfterKey  string
	BeforeKey string
}

// Window presets for the first-class walk commands.
var (
	// BillDateWindow ranges Commerce_Bill transaction dates.
	BillDateWindow = DateWindowSpec{Field: "transactionDate", AfterKey: "createdOnOrAfter", BeforeKey: "createdOnOrBefore"}
	// TaskDueDateWindow ranges task-management due dates.
	TaskDueDateWindow = DateWindowSpec{Field: "dueDate", AfterKey: "onOrAfter", BeforeKey: "onOrBefore"}
)

// DateWindow is a parsed spec plus concrete From/To instants (zero =
// unbounded on that side).
type DateWindow struct {
	DateWindowSpec
	From time.Time
	To   time.Time
}

// ParseWindow parses dd/MM/yyyy (or ISO yyyy-MM-dd) from/to bounds — each
// may be empty — under the spec's filter path. --from after --to is
// rejected.
func (s DateWindowSpec) ParseWindow(from, to string) (*DateWindow, error) {
	win := &DateWindow{DateWindowSpec: s}
	parse := func(val, flag string) (time.Time, error) {
		for _, layout := range cliDateLayouts {
			if t, err := time.Parse(layout, val); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("--%s %q: want dd/MM/yyyy (or yyyy-MM-dd)", flag, val)
	}
	if from != "" {
		t, err := parse(from, "from")
		if err != nil {
			return nil, err
		}
		win.From = t
	}
	if to != "" {
		t, err := parse(to, "to")
		if err != nil {
			return nil, err
		}
		win.To = t
	}
	if !win.From.IsZero() && !win.To.IsZero() && win.From.After(win.To) {
		return nil, fmt.Errorf("--from %s is after --to %s", from, to)
	}
	return win, nil
}

// injectDateWindow merges win into vars' filter variable without disturbing
// sibling keys: filter[Field] gains AfterKey/BeforeKey entries carrying the
// ISO date strings. A bounds-less window still guarantees the filter object
// exists (several captured schemas declare $filter non-null).
func injectDateWindow(vars map[string]any, win *DateWindow) {
	src, _ := vars["filter"].(map[string]any)
	filter := make(map[string]any, len(src)+1)
	maps.Copy(filter, src)
	vars["filter"] = filter
	if win.From.IsZero() && win.To.IsZero() {
		return
	}
	nestedSrc, _ := filter[win.Field].(map[string]any)
	nested := make(map[string]any, len(nestedSrc)+2)
	maps.Copy(nested, nestedSrc)
	if !win.From.IsZero() {
		nested[win.AfterKey] = win.From.Format(wireDateLayout)
	}
	if !win.To.IsZero() {
		nested[win.BeforeKey] = win.To.Format(wireDateLayout)
	}
	filter[win.Field] = nested
}
