package cli

// mutation_contract_test.go enforces the mutation flag contract against the
// REAL resolved command tree — not the catalog maps alone. Single source of
// truth:
//
//   - client.MutationFlagsConsumed: flag sets each entity:op builder path
//     consumes (v3 generic path)
//   - dedicatedConsumed: audited consumed sets for commands that resolve to
//     dedicated implementations (bulk journal, dimensions, sales-order
//     GraphQL, send/pdf, v4 transaction forms, merges, etc.)
//   - dedicatedScopeExcluded: wired mutations that belong to separate
//     subsystems (feed, relay GraphQL, report/proposal/task/workflow/cost
//     builders) — pinned so a new dedicated mutation must be classified
//     deliberately
//   - mutationMetaParams: catalog params documented as intentionally
//     unconsumed (ValidateMutationFlags rejects them at run time)
//
// Three invariants per wired mutation entry:
//
//	forward:  declared params ⊆ consumed ∪ meta
//	reverse:  consumed ⊆ attached flags (alias-aware via mutationAliasGroups)
//	tight:    attached ⊆ consumed ∪ declared ∪ meta ∪ control ∪ shared
//
// Coverage: every wired non-R0 catalog entry resolves to a real command and
// is classified into exactly one contract bucket — drift fails the build.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// mutationKind classifies a wired mutation catalog entry by which contract
// bucket governs it.
type mutationKind int

const (
	mutationV3         mutationKind = iota // generic newV3MutateCmd → client contract
	mutationDedicated                      // dedicatedConsumed[id]
	mutationOutOfScope                     // dedicatedScopeExcluded[id]
)

// resolveMutationCommand finds the real command for a catalog entry via the
// root tree — the same path a user invocation takes.
func resolveMutationCommand(root *cobra.Command, e primitiveEntry) *cobra.Command {
	cmd, _, err := root.Find(strings.Fields(e.Command))
	if err != nil || cmd == nil || cmd == root {
		return nil
	}
	return cmd
}

// classifyMutation decides which contract bucket an entry falls into, using
// the resolved command (not just the catalog spec).
func classifyMutation(e primitiveEntry, cmd *cobra.Command) (mutationKind, v3MutateSpec) {
	if _, ok := dedicatedScopeExcluded[e.ID]; ok {
		return mutationOutOfScope, v3MutateSpec{}
	}
	if _, ok := dedicatedConsumed[e.ID]; ok {
		return mutationDedicated, v3MutateSpec{}
	}
	spec, ok := v3MutateByID[e.ID]
	if ok && cmd != nil {
		// newV3MutateCmd always stamps the readback annotation ("true" for
		// live-cleared entities, "false" for gated ones like Budget and
		// Recurring) — its presence identifies the generic path.
		if _, stamped := cmd.Annotations[readbackAnnotation]; stamped {
			return mutationV3, spec
		}
	}
	return -1, spec
}

func flagSet(names []string) map[string]bool {
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

// mutationEntries enumerates every wired mutation in scope: catalog rows
// plus the inline primitiveEntry registrations the catalog never sees.
func mutationEntries() []primitiveEntry {
	var out []primitiveEntry
	for _, e := range catalogPrimitives {
		if e.Risk == "R0" || e.Mode != modeWired || e.Command == "" {
			continue
		}
		out = append(out, e)
	}
	for command, id := range mutationInlineCommands {
		out = append(out, primitiveEntry{ID: id, Risk: "R2", Mode: modeWired, Command: command})
	}
	return out
}

func sharedFlagSet() map[string]bool {
	out := map[string]bool{}
	for _, sf := range sharedMutationFlags {
		out[sf.name] = true
	}
	return out
}

// TestMutationContractCoverage: every wired mutation catalog entry resolves
// to a real command and is classified into exactly one contract bucket.
func TestMutationContractCoverage(t *testing.T) {
	root := NewRootCommand()
	for _, e := range mutationEntries() {
		cmd := resolveMutationCommand(root, e)
		if cmd == nil {
			t.Errorf("%s (%q): wired mutation does not resolve to a real command", e.ID, e.Command)
			continue
		}
		kind, spec := classifyMutation(e, cmd)
		switch kind {
		case mutationV3:
			if _, ok := client.MutationFlagsConsumed(spec.Entity, spec.Op); !ok {
				t.Errorf("%s: v3 spec %s:%s has no client contract entry", e.ID, spec.Entity, spec.Op)
			}
		case mutationDedicated, mutationOutOfScope:
			// classified
		default:
			t.Errorf("%s (%q): wired mutation resolves to a command with no contract coverage — audit it into dedicatedConsumed, dedicatedScopeExcluded, or v3MutateByID", e.ID, e.Command)
		}
	}
}

// TestMutationContractForward: every catalog-declared param on a wired
// mutation command is either consumed by its path or explicitly meta-listed
// with a documented reason. A declared param that lands nowhere is the
// documented-but-dropped drift class.
func TestMutationContractForward(t *testing.T) {
	root := NewRootCommand()
	for _, e := range mutationEntries() {
		cmd := resolveMutationCommand(root, e)
		if cmd == nil {
			continue // coverage test reports this
		}
		kind, spec := classifyMutation(e, cmd)
		var consumed map[string]bool
		switch kind {
		case mutationV3:
			names, _ := client.MutationFlagsConsumed(spec.Entity, spec.Op)
			consumed = flagSet(names)
		case mutationDedicated:
			consumed = flagSet(dedicatedConsumed[e.ID])
		default:
			continue // out-of-scope entries carry their own plumbing
		}
		meta := mutationMetaParams[e.ID]
		for _, p := range paramsFor(e.ID) {
			if consumed[p.Name] || client.MutationIsControl(p.Name) {
				continue
			}
			if _, ok := meta[p.Name]; ok {
				continue
			}
			t.Errorf("%s (%s): declared param --%s is not consumed and not meta-listed", e.ID, e.Command, p.Name)
		}
	}
}

// TestMutationContractReverse: every flag a mutation path consumes is
// reachable on the real command — directly attached, catalog-declared, or
// satisfied through an alias group. This is the original bug class:
// a builder reading a flag cobra never exposes.
func TestMutationContractReverse(t *testing.T) {
	root := NewRootCommand()
	for _, e := range mutationEntries() {
		cmd := resolveMutationCommand(root, e)
		if cmd == nil {
			continue
		}
		kind, spec := classifyMutation(e, cmd)
		var consumed map[string]bool
		switch kind {
		case mutationV3:
			names, _ := client.MutationFlagsConsumed(spec.Entity, spec.Op)
			consumed = flagSet(names)
		case mutationDedicated:
			consumed = flagSet(dedicatedConsumed[e.ID])
		default:
			continue
		}
		attached := mutationAttachedNames(cmd)
		reachable := func(name string) bool { return attached[name] }
		for name := range consumed {
			if name == "cheque" {
				continue // injected by RunE for Cheque specs; never user-facing
			}
			if (name == "entity" || name == "txn-type") && spec.Entity != "" {
				continue // selector flags are real only on the generic void command
			}
			if mutationAliasSatisfied(name, reachable) {
				continue
			}
			t.Errorf("%s (%s): consumed flag --%s is not attached or reachable via an alias", e.ID, e.Command, name)
		}
	}
}

// TestMutationContractTight: every flag attached to a wired mutation command
// is accounted for — consumed, catalog-declared, meta-listed, control, or
// part of the shared v3 surface. An attached flag outside every bucket is
// either dead plumbing or an undocumented param.
func TestMutationContractTight(t *testing.T) {
	root := NewRootCommand()
	shared := sharedFlagSet()
	for _, e := range mutationEntries() {
		cmd := resolveMutationCommand(root, e)
		if cmd == nil {
			continue
		}
		kind, spec := classifyMutation(e, cmd)
		var consumed map[string]bool
		switch kind {
		case mutationV3:
			names, _ := client.MutationFlagsConsumed(spec.Entity, spec.Op)
			consumed = flagSet(names)
		case mutationDedicated:
			consumed = flagSet(dedicatedConsumed[e.ID])
		default:
			continue
		}
		meta := mutationMetaParams[e.ID]
		declared := map[string]bool{}
		for _, p := range paramsFor(e.ID) {
			declared[p.Name] = true
		}
		for name := range mutationAttachedNames(cmd) {
			if name == "help" {
				continue
			}
			if consumed[name] || declared[name] || client.MutationIsControl(name) {
				continue
			}
			if _, ok := meta[name]; ok {
				continue
			}
			if kind == mutationV3 && shared[name] {
				continue // shared surface is validated at RunE by ValidateMutationFlags
			}
			t.Errorf("%s (%s): attached flag --%s is neither consumed, declared, meta, nor control", e.ID, e.Command, name)
		}
	}
}

// TestMutationContractConsistency pins internal invariants: dedicatedConsumed
// and dedicatedScopeExcluded are disjoint, every mutationMetaParams key is a
// real catalog entry, and wildcard contract entries only carry real ops.
func TestMutationContractConsistency(t *testing.T) {
	inlineIDs := map[string]bool{}
	for _, id := range mutationInlineCommands {
		inlineIDs[id] = true
	}
	for id := range dedicatedConsumed {
		if _, ok := dedicatedScopeExcluded[id]; ok {
			t.Errorf("%s: in both dedicatedConsumed and dedicatedScopeExcluded", id)
		}
		if _, ok := catalogByID(id); !ok && !inlineIDs[id] {
			t.Errorf("%s: dedicatedConsumed entry has no catalog row or inline registration", id)
		}
	}
	for command, id := range mutationInlineCommands {
		if _, ok := dedicatedConsumed[id]; !ok {
			t.Errorf("%s (%q): inline mutation has no dedicatedConsumed entry", id, command)
		}
	}
	for id, meta := range mutationMetaParams {
		if _, ok := catalogByID(id); !ok {
			t.Errorf("%s: mutationMetaParams entry has no catalog row", id)
		}
		for name := range meta {
			found := false
			for _, p := range paramsFor(id) {
				if p.Name == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: meta param --%s is not declared on the command", id, name)
			}
		}
	}
}

// sortedNames is a test helper for readable failure output.
func sortedNames(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// TestMutationBuilderDriftGuard is the source-level drift guard: it scans the
// client package's mutation builders for every flag-name literal they read —
// firstFlag(flags, ...), numericFlag(flags, ...), flags["x"], fm["x"] — and
// asserts each name is covered by the contract union (client
// MutationConsumedUnion ∪ dedicatedConsumed) or a per-file allowance for
// excluded-subsystem plumbing (feed rules, gql passthroughs). A builder read
// that lands in none of those fails the build, so a new flag read cannot
// silently escape the contract.
func TestMutationBuilderDriftGuard(t *testing.T) {
	union := client.MutationConsumedUnion()
	for _, names := range dedicatedConsumed {
		for _, n := range names {
			union[n] = true
		}
	}
	// Files owned by dedicatedScopeExcluded subsystems read their own flag
	// surfaces; pin the exact names so growth still needs an audit.
	excludedSubsystemFlags := map[string][]string{
		"rules.go": {"account-ids", "amount", "amount-op", "auto-add",
			"bank-text", "bank-text-exclude", "bank-text-exclude-2",
			"bank-text-op", "class-id", "description", "description-op",
			"direction", "disabled", "exclude", "id", "match", "money",
			"name", "rule-name"},
		"rules_preserve.go": {"class-id"},
		"bankdisconnect.go": {"account-id", "id"},
	}
	srcDir := filepath.Join("..", "client")
	files, err := filepath.Glob(filepath.Join(srcDir, "*.go"))
	if err != nil {
		t.Fatalf("scan client sources: %v", err)
	}
	readRe := regexp.MustCompile(`(?:firstFlag|numericFlag|firstFlagString)\((?:flags|fm)\s*,([^)]*)\)|(?:flags|fm)\[\s*"([^"]+)"\s*\]`)
	litRe := regexp.MustCompile(`"([^"]+)"`)
	for _, path := range files {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", base, err)
		}
		for _, m := range readRe.FindAllSubmatch(data, -1) {
			var names []string
			if len(m[2]) > 0 {
				names = []string{string(m[2])}
			} else {
				for _, l := range litRe.FindAllSubmatch(m[1], -1) {
					names = append(names, string(l[1]))
				}
			}
			for _, n := range names {
				if union[n] || client.MutationIsControl(n) {
					continue
				}
				if allowed := flagSet(excludedSubsystemFlags[base]); allowed[n] {
					continue
				}
				t.Errorf("%s reads flag --%s not covered by the mutation contract (add to a contract entry or classify the file)", base, n)
			}
		}
	}
}
