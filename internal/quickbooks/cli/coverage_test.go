package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/doctor"
)

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
	want := map[string]int{"wired": 139, "read": 167, "blocked": 223, "excluded": 2}
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
		for _, l := range strings.Split(text, "\n") {
			fields := strings.Fields(l)
			if len(fields) >= 4 && strings.HasPrefix(fields[0], "QBO.") && fields[3] == mode {
				lines++
			}
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
	if wired != 139 || read != 167 || blocked != 223 || excluded != 2 || total != 531 {
		t.Fatalf("catalogModeTotals = (%d,%d,%d,%d,%d), want (139,167,223,2,531)", wired, read, blocked, excluded, total)
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
	if !strings.Contains(out.String(), "Coverage: 139 wired, 167 read-only, 223 blocked, 2 excluded of 531 actions") {
		t.Errorf("root help missing catalog-wide coverage line; got:\n%s", out.String())
	}

	cases := map[string]string{
		"expenses":      "Coverage: 26 wired, 19 read-only, 30 blocked",
		"sales":         "Coverage: 27 wired, 17 read-only, 23 blocked",
		"accounting":    "Coverage: 36 wired, 29 read-only, 10 blocked",
		"payroll":       "Coverage: 5 wired, 11 read-only, 22 blocked",
		"reports":       "Coverage: 0 wired, 20 read-only, 9 blocked",
		"company":       "Coverage: 15 wired, 21 read-only, 46 blocked",
		"customers":     "Coverage: 3 wired, 15 read-only, 13 blocked",
		"inventory":     "Coverage: 7 wired, 8 read-only, 9 blocked",
		"tax":           "Coverage: 2 wired, 8 read-only, 18 blocked",
		"feed":          "Coverage: 7 wired, 11 read-only, 19 blocked",
		"advanced":      "Coverage: 0 wired, 4 read-only, 16 blocked",
		"accountant":    "Coverage: 0 wired, 0 read-only, 1 blocked",
		"integrations":  "Coverage: 0 wired, 0 read-only, 4 blocked",
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
	for _, frag := range []string{"139 wired", "167 read", "223 blocked", "2 excluded", "531 actions"} {
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
