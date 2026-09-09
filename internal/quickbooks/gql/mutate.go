package gql

import (
	"fmt"
	"sort"
	"strings"
)

// LookupMutation returns the declared variable signature for a captured
// mutation (e.g. "$input: Commerce_ConsumeInventoryInput!") or an error
// naming the closest known operations when the name is unknown.
func LookupMutation(name string) (string, error) {
	sig, ok := mutationInputs[name]
	if !ok {
		return "", fmt.Errorf("unknown GraphQL mutation %q%s", name, suggestMutations(name))
	}
	return sig, nil
}

// ListMutations returns every captured mutation name sorted alphabetically.
func ListMutations() []string {
	names := make([]string, 0, len(mutationInputs))
	for name := range mutationInputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MutationVarTypes splits a signature into per-variable types in declaration
// order: "input" -> "Commerce_ConsumeInventoryInput!". Signatures come from
// webpack extraction and may separate declarations with runs of whitespace,
// so types are sliced between consecutive "$name:" markers rather than
// matched by a single pattern.
func MutationVarTypes(sig string) map[string]string {
	out := make(map[string]string)
	idx := varDeclNameRe.FindAllStringSubmatchIndex(sig, -1)
	for i, m := range idx {
		start := m[1] // just past "$name:"
		end := len(sig)
		if i+1 < len(idx) {
			end = idx[i+1][0] // start of the next "$name:"
		}
		typ := strings.TrimSpace(sig[start:end])
		if j := strings.Index(typ, "="); j >= 0 { // drop any default value
			typ = strings.TrimSpace(typ[:j])
		}
		out[sig[m[2]:m[3]]] = typ
	}
	return out
}

// RequiredMutationVars lists the variables a caller must supply for name:
// those declared non-null without a default value. Unknown names error.
func RequiredMutationVars(name string) ([]string, error) {
	if _, err := LookupMutation(name); err != nil {
		return nil, err
	}
	return mutationRequiredVars[name], nil // nil = nothing required
}

// ValidateMutationVars checks that every variable the signature declares
// non-null is present before the request is built, so invalid requests are
// refused locally instead of round-tripping a guaranteed GraphQL error.
// Extra variables are rejected too: the document does not declare them.
func ValidateMutationVars(name string, vars map[string]any) error {
	sig, err := LookupMutation(name)
	if err != nil {
		return err
	}
	types := MutationVarTypes(sig)
	for _, req := range mutationRequiredVars[name] {
		v, ok := vars[req]
		if !ok || v == nil {
			return fmt.Errorf("mutation %s requires -$%s %s", name, req, types[req])
		}
	}
	for v := range vars {
		if _, ok := types[v]; !ok {
			return fmt.Errorf("mutation %s does not declare $%s; declared variables: %s",
				name, v, sig)
		}
	}
	return nil
}

// suggestMutations renders up to three near-miss names ranked by their
// case-insensitive common prefix with the requested name (minimum five
// characters), or the full-list pointer when nothing resembles it.
func suggestMutations(name string) string {
	lower := strings.ToLower(name)
	type hit struct {
		name  string
		score int
	}
	var hits []hit
	for _, cand := range ListMutations() {
		n := commonPrefixLen(strings.ToLower(cand), lower)
		if n >= 5 {
			hits = append(hits, hit{cand, n})
		}
	}
	if len(hits) == 0 {
		return " (see `qb gql mutate` for all captured mutations)"
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].name < hits[j].name
	})
	if len(hits) > 3 {
		hits = hits[:3]
	}
	names := make([]string, len(hits))
	for i, h := range hits {
		names[i] = h.name
	}
	return " (did you mean: " + strings.Join(names, ", ") + "?)"
}

func commonPrefixLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
