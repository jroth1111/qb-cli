package cli

import (
	"strings"
	"testing"
)

// TestCatalogParamTypesMatchCobraFlags guards the third catalog direction:
// every advertised param type must match the cobra flag value type, so an
// agent passing --limit 20 never hits a bool parser. Audit-clean on adoption.
func TestCatalogParamTypesMatchCobraFlags(t *testing.T) {
	want := map[string][]string{
		"string":  {"string"},
		"int":     {"int", "int8", "int16", "int32", "int64", "uint", "count"},
		"bool":    {"bool"},
		"strings": {"stringSlice", "stringArray"},
	}
	root := NewRootCommand()
	for _, e := range catalogPrimitives {
		if e.Mode != modeWired && e.Mode != modeRead {
			continue
		}
		if e.Command == "" {
			continue
		}
		target, _, err := root.Find(strings.Fields(e.Command))
		if err != nil || target == nil {
			continue
		}
		for _, p := range paramsFor(e.ID) {
			f := target.Flags().Lookup(p.Name)
			if f == nil {
				f = target.InheritedFlags().Lookup(p.Name)
			}
			if f == nil {
				f = target.PersistentFlags().Lookup(p.Name)
			}
			if f == nil {
				continue
			}
			ok := false
			for _, v := range want[p.Type] {
				if f.Value.Type() == v {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s param --%s catalog type %q vs cobra %q", e.ID, p.Name, p.Type, f.Value.Type())
			}
		}
	}
}
