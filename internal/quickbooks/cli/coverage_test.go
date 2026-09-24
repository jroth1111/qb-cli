package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/doctor"
)

func TestREADMECountsMatchCatalog(t *testing.T) {
	readme, err := os.ReadFile("../../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	roots := regexp.MustCompile("`qb ([a-z]+)`")
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(readme), "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		columns := strings.Split(line, "|")
		if len(columns) != 6 {
			continue
		}
		matches := roots.FindAllStringSubmatch(columns[4], -1)
		if len(matches) == 0 {
			continue
		}
		wired, read := 0, 0
		for _, match := range matches {
			domain := match[1]
			if seen[domain] {
				t.Errorf("duplicate README domain %s", domain)
			}
			seen[domain] = true
			for _, row := range catalogPrimitives {
				if row.Domain != domain {
					continue
				}
				switch row.Mode {
				case modeWired:
					wired++
				case modeRead:
					read++
				}
			}
		}
		if strings.TrimSpace(columns[2]) != fmt.Sprint(wired) || strings.TrimSpace(columns[3]) != fmt.Sprint(read) {
			t.Errorf("README coverage drift: %s; catalog has %d wired, %d read", line, wired, read)
		}
	}
	for _, row := range catalogPrimitives {
		if !seen[row.Domain] {
			t.Fatalf("README missing catalog domain %s", row.Domain)
		}
	}
}

// runRootArgs executes qb against args, capturing stdout+stderr.
func runCoverageRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	root.SetArgs(args)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.ExecuteContext(ctx)
	return out.String(), err
}

// TestActionsModeFlagFiltersRows asserts `qb actions --mode <m>` emits exactly
// the rows whose mode is m, in both text and JSON renderings, and rejects an
// unknown mode instead of silently listing everything. Rejects: a flag that
// parses but never filters (all rows shown), an off-by-one filter, and a
// filter that breaks the JSON contract.
func TestActionsModeFlagFiltersRows(t *testing.T) {
	want := map[string]int{"wired": 207, "read": 184, "blocked": 155, "excluded": 2}
	for mode, n := range want {
		out, err := runCoverageRoot(t, "actions", "--json", "--mode", mode)
		if err != nil {
			t.Fatalf("actions --json --mode %s failed: %v", mode, err)
		}
		got := strings.Count(out, fmt.Sprintf("%q", mode))
		if got != n {
			t.Errorf("mode %s: %d JSON rows labelled %q, want %d", mode, got, mode, n)
		}
		text, err := runCoverageRoot(t, "actions", "--mode", mode)
		if err != nil {
			t.Fatalf("actions --mode %s failed: %v", mode, err)
		}
		lines := 0
		for l := range strings.SplitSeq(text, "\n") {
			fields := strings.Fields(l)
			if len(fields) >= 4 && strings.HasPrefix(fields[0], "QBO.") && fields[3] == mode {
				lines++
			}
		}
		if lines != n {
			t.Errorf("mode %s: %d text rows, want %d", mode, lines, n)
		}
	}

	// Negative: an unlisted mode is a usage error, not an empty success.
	out, err := runCoverageRoot(t, "actions", "--mode", "wiredish")
	if err == nil {
		t.Fatalf("--mode wiredish accepted; output=%q", out)
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitInputError {
		t.Errorf("--mode wiredish error = %v, want ExitError code %d", err, ExitInputError)
	}
}

// TestCoverageCountsMatchCatalog pins the aggregate counts every surface
// (help lines, doctor, COVERAGE.md) derives from, and the per-domain string
// format. Rejects a counter that drifts from catalogPrimitives or a format
// change that breaks agents parsing "Coverage: N wired, M read-only, K blocked".
func TestCoverageCountsMatchCatalog(t *testing.T) {
	wired, read, blocked, excluded, total := catalogModeTotals()
	if wired != 207 || read != 184 || blocked != 155 || excluded != 2 || total != 548 {
		t.Fatalf("catalogModeTotals = (%d,%d,%d,%d,%d), want (207,184,155,2,548)", wired, read, blocked, excluded, total)
	}
	if wired+read+blocked+excluded != total {
		t.Errorf("modes sum %d != total %d", wired+read+blocked+excluded, total)
	}
	// Domain counts recomputed straight from the catalog must equal what
	// coverageFor renders, for every domain present in the catalog.
	domains := map[string]bool{}
	for _, r := range catalogPrimitives {
		domains[r.Domain] = true
	}
	for d := range domains {
		var w, r, b int
		for _, e := range catalogPrimitives {
			if e.Domain != d {
				continue
			}
			switch e.Mode {
			case modeWired:
				w++
			case modeRead:
				r++
			case modeBlocked:
				b++
			}
		}
		want := fmt.Sprintf("Coverage: %d wired, %d read-only, %d blocked", w, r, b)
		if got := coverageFor(d); got != want {
			t.Errorf("coverageFor(%q) = %q, want %q", d, got, want)
		}
	}
	if s := coverageFor("nosuchdomain"); s != "" {
		t.Errorf("coverageFor(unknown domain) = %q, want empty", s)
	}
}

// TestDomainHelpShowsCoverageLine asserts each domain group's help ends its
// Long section with a coverage line derived from the catalog, the root help
// carries the catalog-wide line, and non-domain commands stay stock. Rejects:
// injection onto leaves, a missing domain line, and wrong counts on screen.
func TestDomainHelpShowsCoverageLine(t *testing.T) {
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Help(); err != nil {
		t.Fatalf("root help: %v", err)
	}
	if !strings.Contains(out.String(), "Coverage: 207 wired, 184 read-only, 155 blocked, 2 excluded of 548 actions") {
		t.Errorf("root help missing catalog-wide coverage line; got:\n%s", out.String())
	}

	cases := map[string]string{
		"expenses":      "Coverage: 37 wired, 22 read-only, 16 blocked",
		"sales":         "Coverage: 38 wired, 21 read-only, 8 blocked",
		"accounting":    "Coverage: 37 wired, 30 read-only, 8 blocked",
		"payroll":       "Coverage: 5 wired, 3 read-only, 30 blocked",
		"reports":       "Coverage: 12 wired, 30 read-only, 0 blocked",
		"company":       "Coverage: 24 wired, 28 read-only, 31 blocked",
		"customers":     "Coverage: 9 wired, 11 read-only, 13 blocked",
		"inventory":     "Coverage: 10 wired, 9 read-only, 5 blocked",
		"tax":           "Coverage: 2 wired, 10 read-only, 16 blocked",
		"feed":          "Coverage: 14 wired, 11 read-only, 12 blocked",
		"advanced":      "Coverage: 8 wired, 3 read-only, 10 blocked",
		"accountant":    "Coverage: 0 wired, 0 read-only, 1 blocked",
		"integrations":  "Coverage: 0 wired, 2 read-only, 2 blocked",
		"gql":           "Coverage: 8 wired, 3 read-only, 0 blocked",
		"costgroups":    "Coverage: 3 wired, 1 read-only, 0 blocked",
		"customobjects": "Coverage: 0 wired, 0 read-only, 3 blocked",
	}
	for name, want := range cases {
		var buf bytes.Buffer
		root.SetOut(&buf)
		sub, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if err := sub.Help(); err != nil {
			t.Fatalf("%s help: %v", name, err)
		}
		if !strings.Contains(buf.String(), want) {
			t.Errorf("%s help missing %q; got:\n%s", name, want, buf.String())
		}
	}

	var buf bytes.Buffer
	root.SetOut(&buf)
	login, _, err := root.Find([]string{"login"})
	if err != nil {
		t.Fatalf("find login: %v", err)
	}
	if err := login.Help(); err != nil {
		t.Fatalf("login help: %v", err)
	}
	if strings.Contains(buf.String(), "Coverage:") {
		t.Errorf("login help should not carry a coverage line; got:\n%s", buf.String())
	}
}

// TestDoctorCoverageCheckIsInformational asserts the appended coverage check
// always reports ok and never flips OverallOK, even for a wholly failing
// session result. Rejects: a check that can fail doctor's exit gate, a detail
// line missing any mode count, and mutation of earlier checks' verdicts.
func TestDoctorCoverageCheckIsInformational(t *testing.T) {
	res := doctor.DoctorResult{
		OverallOK: false,
		Checks: []doctor.Check{
			{Name: "credentials-file", OK: false, Detail: "missing"},
		},
	}
	got := appendCoverageCheck(res)
	if len(got.Checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(got.Checks))
	}
	last := got.Checks[len(got.Checks)-1]
	if last.Name != "coverage" || !last.OK {
		t.Errorf("last check = %+v, want named \"coverage\" and ok", last)
	}
	for _, frag := range []string{"207 wired", "184 read", "155 blocked", "2 excluded", "548 actions"} {
		if !strings.Contains(last.Detail, frag) {
			t.Errorf("detail %q missing %q", last.Detail, frag)
		}
	}
	if got.OverallOK {
		t.Error("appendCoverageCheck flipped OverallOK to true")
	}
	if got.Checks[0].OK {
		t.Error("appendCoverageCheck altered an earlier check verdict")
	}
}
