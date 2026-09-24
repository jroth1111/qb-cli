package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql/routes"
)

const corpusDir = "/tmp/qbo-cap"

type indexEntry struct {
	File string `json:"file"`
	Size int    `json:"size"`
}

type pass3 struct {
	MutationInputs map[string]string `json:"mutation_inputs"`
	VariableTypes  []string          `json:"variable_types"`
}

// Endpoint constants share the runtime's routing source without depending
// on the generated catalog. Kind constants describe the captured documents.
const (
	EndpointDefault    = routes.EndpointDefault
	EndpointWarehouse  = routes.EndpointWarehouse
	EndpointCommerce   = routes.EndpointCommerceControl
	EndpointSpendLists = routes.EndpointSpendLists
	EndpointCES        = routes.EndpointCES

	KindQuery    = "query"
	KindMutation = "mutation"
)

// endpointFor uses verified routing when available and retains the
// generator's default endpoint for operations without routing evidence.
func endpointFor(operation string) string {
	if endpoint, found := routes.Lookup(operation); found {
		return endpoint
	}
	return EndpointDefault
}

var (
	// concatStrings matches the webpack ".concat(" string-literal joins left
	// when a UI template interpolated a selection set at runtime:
	// `".concat("frag1","frag2` — the fragments are joined verbatim.
	concatStrings = regexp.MustCompile(`"\s*\.concat\(\s*"(.*)","\s*(.*)`)
	// concatJSVar matches the unrecoverable variant where a JS expression
	// supplies the fragment (`".concat(e,"] } }`, `".concat(n?"baseAccounts {`).
	// Documents containing any leftover ".concat(" are marked BrokenTemplate.
	concatJSVar = regexp.MustCompile(`\.concat\(`)
	// templateVarHole matches webpack template-literal holes (`${N}`, `${_}`)
	// that survived extraction. Unless template_fragments.json supplies the
	// value, the document is marked BrokenTemplate (fail-safe: ego_execute
	// sends Document verbatim, so a hole would reach the server).
	templateVarHole = regexp.MustCompile(`\$\{[A-Za-z_]\w*\}`)
)

// templateFragments.json (checked in beside main.go) carries live-captured
// fragment values for template holes, keyed by operation then variable:
// {"CommerceReceiveInventory": {"N": "<selection set>"}}. Provenance lives
// under the file's "_provenance" key.
func cleanDoc(raw string) (doc string, broken bool) {
	doc = strings.ReplaceAll(raw, `\n`, "\n")
	doc = strings.ReplaceAll(doc, `\"`, `"`)
	doc = strings.ReplaceAll(doc, `\'`, "'")
	if m := concatStrings.FindStringSubmatch(doc); m != nil {
		doc = concatStrings.ReplaceAllString(doc, m[1]+"\n"+m[2])
	}
	broken = concatJSVar.MatchString(doc)
	return doc, broken
}

func loadTemplateFragments() map[string]map[string]string {
	_, src, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(src), "template_fragments.json"))
	if err != nil {
		return nil
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil
	}
	out := make(map[string]map[string]string, len(all))
	for name, msg := range all {
		// "_provenance" is a documentation string, not an op entry.
		if strings.HasPrefix(name, "_") {
			continue
		}
		var vars map[string]string
		if err := json.Unmarshal(msg, &vars); err != nil {
			continue
		}
		out[name] = vars
	}
	return out
}

// inlineTemplateFragments substitutes live-captured values for webpack
// template holes, then marks the doc broken if any hole remains. Values
// come from template_fragments.json; unknown holes fail safe to
// BrokenTemplate instead of reaching the server verbatim.
func inlineTemplateFragments(name, doc string, broken bool) (string, bool) {
	if frags := loadTemplateFragments(); frags != nil {
		for v, text := range frags[name] {
			doc = strings.ReplaceAll(doc, "${"+v+"}", text)
		}
	}
	if templateVarHole.MatchString(doc) {
		broken = true
	}
	return doc, broken
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gql/gen: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	idxPath := filepath.Join(corpusDir, "graphql", "index.json")
	pass3Path := filepath.Join(corpusDir, "gql_pass3.json")
	for _, p := range []string{idxPath, pass3Path} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("capture corpus missing (%s): %w — mount /tmp/qbo-cap or regenerate it first", p, err)
		}
	}

	var idx map[string]indexEntry
	if err := readJSON(idxPath, &idx); err != nil {
		return fmt.Errorf("parse %s: %w", idxPath, err)
	}
	var p3 pass3
	if err := readJSON(pass3Path, &p3); err != nil {
		return fmt.Errorf("parse %s: %w", pass3Path, err)
	}

	names := make([]string, 0, len(idx))
	for name := range idx {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder

	for _, name := range names {
		meta := idx[name]
		raw, err := os.ReadFile(filepath.Join(corpusDir, "graphql", name+".graphql"))
		if err != nil {
			return fmt.Errorf("read doc for %s: %w", name, err)
		}
		doc, broken := cleanDoc(string(raw))
		doc, broken = inlineTemplateFragments(name, doc, broken)
		kind := kindOf(doc)
		varTypes := varTypesOf(doc)
		endpoint := endpointFor(name)
		inputType := ""
		if t, ok := p3.MutationInputs[name]; ok && kind == KindMutation {
			inputType = summarizeInput(t)
		}
		moduleSrc := moduleName(meta.File)

		fmt.Fprintf(&b, "\t{\n")
		fmt.Fprintf(&b, "\t\tName: %q,\n", name)
		fmt.Fprintf(&b, "\t\tKind: %q,\n", kind)
		fmt.Fprintf(&b, "\t\tEndpoint: %q,\n", endpoint)
		fmt.Fprintf(&b, "\t\tModule: %q,\n", moduleSrc)
		fmt.Fprintf(&b, "\t\tVarTypes: []string{%s},\n", joinQuoted(varTypes))
		fmt.Fprintf(&b, "\t\tInputHint: %q,\n", inputType)
		fmt.Fprintf(&b, "\t\tBrokenTemplate: %v,\n", broken)
		fmt.Fprintf(&b, "\t\tDocument: %s,\n", backtick(doc, name))
		b.WriteString("\t},\n")
	}

	dest := destPath()
	src := emitFile(b.String())
	if err := os.WriteFile(dest, []byte(src), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	fmt.Fprintf(os.Stderr, "gql/gen: wrote %s (%d operations)\n", dest, len(names))
	return nil
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return json.NewDecoder(f).Decode(v)
}

func kindOf(doc string) string {
	if strings.HasPrefix(strings.TrimSpace(doc), "mutation") {
		return KindMutation
	}
	return KindQuery
}

// varTypesOf extracts declared variable type names from the operation
// signature: `query X($first: Int!, $after: String)` → ["Int!","String"].
var varRe = regexp.MustCompile(`\$\w+:\s*([^\s,)]+)`)

func varTypesOf(doc string) []string {
	head := doc
	if i := strings.Index(head, ")"); i >= 0 {
		head = head[:i]
	}
	var out []string
	for _, m := range varRe.FindAllStringSubmatch(head, -1) {
		out = append(out, m[1])
	}
	return out
}

// summarizeInput reduces a pass3 mutation_inputs blob like
// "$input: Banking_DisconnectOlbInput!" to its primary input type name.
var inputNameRe = regexp.MustCompile(`\$[\w]+:\s*([A-Za-z_][\w.]*)`)

func summarizeInput(decl string) string {
	m := inputNameRe.FindStringSubmatch(decl)
	if m == nil {
		return ""
	}
	return m[1]
}

// moduleName trims a webpack bundle filename to its readable package id.
func moduleName(file string) string {
	s := strings.TrimSuffix(file, ".js")
	parts := strings.SplitN(s, "__", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return s
}

func joinQuoted(items []string) string {
	if len(items) == 0 {
		return ""
	}
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = goStr(s)
	}
	return strings.Join(q, ", ")
}

// backtick renders doc as a raw Go string literal, falling back to a
// quoted literal when the document itself contains a backtick.
func backtick(doc, name string) string {
	if !strings.Contains(doc, "`") {
		return "`" + doc + "`"
	}
	return goStr(doc) + " /* quoted fallback: " + name + " */"
}

func goStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// destPath resolves the catalog output file: $GQL_GEN_OUT, else the
// sibling package directory one level up when run from this gen dir,
// else the current directory.
func destPath() string {
	if v := os.Getenv("GQL_GEN_OUT"); v != "" {
		return v
	}
	if wd, err := os.Getwd(); err == nil && filepath.Base(wd) == "gen" {
		return filepath.Join(wd, "..", "catalog_data.go")
	}
	return "catalog_data.go"
}

// emitFile wraps the catalog entries in a compilable Go source file that
// the gql package compiles in (catalog_data.go, package gql).
func emitFile(entries string) string {
	var b strings.Builder
	b.WriteString("// Code generated by gql/gen from the /tmp/qbo-cap capture corpus. DO NOT EDIT.\n\n")
	b.WriteString("package gql\n\n")
	b.WriteString("// catalogEntries is the generated operation table consumed by catalog.go.\n")
	b.WriteString("var catalogEntries = []Op{\n")
	b.WriteString(entries)
	b.WriteString("}\n")
	return b.String()
}
