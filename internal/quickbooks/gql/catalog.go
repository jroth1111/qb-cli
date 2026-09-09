// Package gql provides the qb GraphQL gateway: an embedded catalog of QBO
// operations captured from the production SPA bundles, a transport that
// executes them inside the authenticated browser session over CDP, and a
// cursor/offset pagination walker.
//
// Design constraint (proven in the capture corpus): offline HTTP replay of
// saved headers against smallbusiness.api.intuit.com/graphql and the other
// GraphQL hosts returns 401 — those endpoints are session-bound. Every
// request is therefore executed inside a qbo.intuit.com page via
// Runtime.evaluate(fetch(...)) on the OMP relay, so cookies, CSRF tokens and
// the SPA's own auth context all apply.
package gql

import (
	"fmt"
	"sort"
	"strings"
)

// Endpoint constants: per-operation GraphQL hosts, encoded as catalog
// metadata. Evidence for each mapping lives in the capture corpus
// (/tmp/qbo-cap): each host appears in exactly one webpack chunk that also
// contains the operations routed to it.
const (
	// EndpointDefault is the qbo webapp Apollo link.
	EndpointDefault = "https://qbo.intuit.com/api/v4/graphql"
	// EndpointWarehouse serves Commerce* inventory ops (order-management-ui
	// chunk 3275, inventory-addon-ui chunk 1156).
	EndpointWarehouse = "https://warehouse-management-svc.api.intuit.com/graphql"
	// EndpointCommerceControl serves order-management / P&S list mutations
	// (commercecontrol.api.intuit.com).
	EndpointCommerceControl = "https://commercecontrol.api.intuit.com/graphql"
	// EndpointSpendLists serves spend/tasks/app-rev ops observed on the
	// smallbusiness host (tasks-ui, app-revx-ui, integrations-apptransactions).
	EndpointSpendLists = "https://smallbusiness.api.intuit.com/graphql"
)

// Operation kinds.
const (
	KindQuery    = "query"
	KindMutation = "mutation"
)

// Op is one catalog entry.
type Op struct {
	Name string // operation name as declared in the document
	Kind string // query | mutation
	// Endpoint receives the POST. Per-op where the capture corpus proves a
	// dedicated host; EndpointDefault otherwise.
	Endpoint string
	// Module is the UI package the document was extracted from
	// (e.g. "spend-lists-xp"), useful for locating live traffic.
	Module string
	// VarTypes lists the declared variable type names in declaration order,
	// e.g. ["Int!", "String"].
	VarTypes []string
	// InputHint is the primary input type of a mutation when pass3 captured
	// it (e.g. "Banking_DisconnectOlbInput").
	InputHint string
	// BrokenTemplate marks documents whose webpack extraction left a JS
	// interpolation hole (.concat with a variable). They are listed but
	// refuse execution.
	BrokenTemplate bool
	Document       string // raw GraphQL document text
}

// ErrNotFound reports an unknown operation name.
type ErrNotFound struct{ Name string }

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("unknown GraphQL operation %q (see `qb gql ops`)", e.Name)
}

// catalog is built once from the generated table, sorted by name.
var catalog = buildCatalog()

func buildCatalog() map[string]*Op {
	m := make(map[string]*Op, len(catalogEntries))
	for i := range catalogEntries {
		op := catalogEntries[i]
		m[op.Name] = &op
	}
	return m
}

// Lookup returns the named operation or ErrNotFound.
func Lookup(name string) (*Op, error) {
	op, ok := catalog[name]
	if !ok {
		return nil, &ErrNotFound{Name: name}
	}
	return op, nil
}

// Ops returns every catalog operation sorted by name. When substr is
// non-empty only operations containing it (case-insensitive) are returned.
func Ops(substr string) []*Op {
	names := make([]string, 0, len(catalog))
	sub := strings.ToLower(substr)
	for name := range catalog {
		if sub == "" || strings.Contains(strings.ToLower(name), sub) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]*Op, len(names))
	for i, n := range names {
		out[i] = catalog[n]
	}
	return out
}

// Count returns the total number of catalog operations.
func Count() int { return len(catalog) }

// ValidateVars checks user variables against the operation's declared
// signature. It returns the first unknown variable name, if any. Declared
// names come from the document itself ($first, $after, ...), so validation
// matches what the server actually accepts.
func (op *Op) ValidateVars(vars map[string]any) error {
	if len(vars) == 0 {
		return nil
	}
	declared := op.DeclaredVars()
	declSet := make(map[string]bool, len(declared))
	for _, d := range declared {
		declSet[d] = true
	}
	for name := range vars {
		if !declSet[name] {
			return fmt.Errorf("operation %s does not declare $%s; declared variables: %s",
				op.Name, name, joinNames(declared))
		}
	}
	return nil
}

// DeclaredVars returns the variable names declared by the document, in
// declaration order (empty for none).
func (op *Op) DeclaredVars() []string {
	doc := op.Document
	open := strings.Index(doc, "(")
	closing := strings.Index(doc, ")")
	if open < 0 || closing < open || !strings.HasPrefix(strings.TrimSpace(doc), op.Kind) {
		return nil
	}
	sig := doc[open+1 : closing]
	var out []string
	for _, part := range strings.Split(sig, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "$") {
			if i := strings.IndexAny(part, " :"); i > 0 {
				out = append(out, part[1:i])
			}
		}
	}
	return out
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "$" + n
	}
	return strings.Join(q, ", ")
}

// WalkStyle classifies how an operation pages.
type WalkStyle string

// Pagination styles auto-detected from the document's variables.
const (
	WalkNone   WalkStyle = ""       // no pagination variables at all
	WalkCursor WalkStyle = "cursor" // $after / pageInfo.endCursor
	WalkOffset WalkStyle = "offset" // $offset incremented per page
	WalkSingle WalkStyle = "single" // limit-only: one fetch is the whole set
)

// DetectWalkStyle inspects the document for cursor ($after), offset
// ($offset/$skip) or limit-only pagination.
func (op *Op) DetectWalkStyle() WalkStyle {
	has := func(names ...string) bool {
		for _, v := range op.DeclaredVars() {
			for _, n := range names {
				if strings.EqualFold(v, n) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("after", "afterCursor", "cursor"):
		return WalkCursor
	case has("offset", "skip"):
		return WalkOffset
	default:
		return WalkSingle
	}
}
