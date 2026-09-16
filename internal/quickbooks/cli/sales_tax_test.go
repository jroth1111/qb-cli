package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

// salestx CLI surface tests: structure (closed verb sets), dry-run plans
// (secret-free, correct URL/op), and no-credentials fast-fail per entity
// group. RunE is only exercised through --dry-run or the credentials guard,
// so nothing dials.
//
// Kill denominator: 8. Rejects a domain that (1) drops verbs, (2) leaks the
// live realm into plans, (3) sends dry-run requests, (4) lets writes run
// without creds, (5) maps entities to the wrong v3 path, (6) skips plan
// validation for gst file / bas lodge, (7) claims tpar generate is wired,
// (8) dials the captured-invalid DelayedCharge v3 query context.

func TestSalestxSurface(t *testing.T) {
	root := NewRootCommand()
	st := findSub(root, "salestx")
	if st == nil {
		t.Fatal("root missing salestx")
	}
	want := map[string][]string{
		"invoice":       {"list", "get", "create", "update", "delete", "void", "send", "pdf"},
		"estimate":      {"list", "get", "create", "update", "delete", "send"},
		"payment":       {"list", "get", "create", "update", "delete"},
		"creditmemo":    {"list", "get", "create", "update", "delete"},
		"delayedcharge": {"list", "create"},
		"salesreceipt":  {"list", "get", "create", "update", "delete"},
		"taxcode":       {"list"},
		"taxrate":       {"list", "get"},
		"gst":           {"list", "file"},
		"bas":           {"list", "lodge"},
		"tpar":          {"list", "generate"},
	}
	for ent, verbs := range want {
		e := findSub(st, ent)
		if e == nil {
			t.Errorf("salestx missing %q", ent)
			continue
		}
		got := map[string]bool{}
		for _, c := range e.Commands() {
			got[strings.Fields(c.Use)[0]] = true
		}
		for _, v := range verbs {
			if !got[v] {
				t.Errorf("salestx %s missing %q", ent, v)
			}
		}
		if len(got) != len(verbs) {
			t.Errorf("salestx %s verb set = %v, want exactly %v", ent, got, verbs)
		}
	}
}

func TestSalestxDryRunPlansAreSecretFreeAndCorrect(t *testing.T) {
	cases := []struct {
		args     []string
		wantURL  []string
		noHave   []string // substrings that must NOT appear
		wantFail error
	}{
		{args: []string{"salestx", "invoice", "create", "--customer", "5", "--amount", "9"},
			wantURL: []string{"https://qbo.intuit.com/api/v3/company/{realm}/invoice?minorversion=73"}},
		{args: []string{"salestx", "invoice", "delete", "--id", "188"},
			wantURL: []string{"/invoice?minorversion=73&operation=delete"}},
		{args: []string{"salestx", "invoice", "void", "--id", "188"},
			wantURL: []string{"operation=void"}},
		{args: []string{"salestx", "invoice", "send", "--id", "188", "--to", "a@b.co"},
			wantURL: []string{"/invoice/{id}/send"}},
		{args: []string{"salestx", "invoice", "pdf", "--id", "188", "--output", "/tmp/x.pdf"},
			wantURL: []string{"/v3/documents/pdf?txnId=188&documentType=invoice"}},
		{args: []string{"salestx", "estimate", "update", "--id", "3", "--customer", "5"},
			wantURL: []string{"/estimate?minorversion=73"}},
		{args: []string{"salestx", "salesreceipt", "delete", "--id", "9"},
			wantURL: []string{"/salesreceipt?minorversion=73&operation=delete"}},
		{args: []string{"salestx", "invoice", "list"},
			wantURL: []string{"/query?minorversion=73&query=", "select+%2A+from+Invoice"}},
		{args: []string{"salestx", "gst", "file", "--amount", "10", "--account", "91"},
			wantFail: client.ErrEmptyTaxPaymentID},
		{args: []string{"salestx", "bas", "lodge"},
			wantFail: client.ErrEmptyTaxReturnID},
		{args: []string{"salestx", "bas", "list"},
			wantFail: client.ErrMissingTaxAgencyID},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			stdout, elapsed, err := runQB(t, append(tc.args, "--dry-run", "--json")...)
			if elapsed > 2*1e9 {
				t.Fatalf("dry-run took %s — likely dialed", elapsed)
			}
			if tc.wantFail != nil {
				if err == nil || !errors.Is(err, tc.wantFail) {
					t.Fatalf("err = %v, want %v", err, tc.wantFail)
				}
				return
			}
			if err != nil {
				t.Fatalf("execute: %v; stdout=%q", err, stdout)
			}
			env := decodePlan(t, stdout)
			if env.Dial || !env.DryRun {
				t.Fatalf("envelope = %+v", env)
			}
			for _, want := range tc.wantURL {
				if !strings.Contains(env.URL, want) {
					t.Fatalf("url %q missing %q", env.URL, want)
				}
			}
			for _, bad := range tc.noHave {
				if strings.Contains(env.URL, bad) {
					t.Fatalf("url %q must not contain %q", env.URL, bad)
				}
			}
			if strings.Contains(stdout, "12345") {
				t.Fatalf("plan leaked live realm:\n%s", stdout)
			}
		})
	}
}

func TestSalestxTaxReadersDryRunCarryCapturedOps(t *testing.T) {
	cases := []struct {
		args    []string
		wantOp  string
		wantURL string
	}{
		{[]string{"salestx", "taxcode", "list"}, "GetTaxCodes_qbo", client.PlannedSalesTaxGatewayURL()},
		{[]string{"salestx", "taxrate", "list"}, "TaxRates__indirect_tax_ui_qbo", client.PlannedSalesTaxGatewayURL()},
		{[]string{"salestx", "gst", "list"}, "IndirectTaxConfigQuery_qbo", client.PlannedGSTConfigURL()},
		{[]string{"salestx", "gst", "file", "--amount", "50", "--account", "91", "--agency", "ag"}, "CreateTaxPayment", client.PlannedGSTFileURL()},
		{[]string{"salestx", "bas", "list", "--agency", "ag"}, "TaxReturns__indirect_tax_ui_qbo", client.PlannedGSTConfigURL()},
		{[]string{"salestx", "bas", "lodge", "--id", "ret"}, "EfileTaxReturn", client.PlannedGSTFileURL()},
		{[]string{"salestx", "tpar", "list"}, "", "TAXABLE_PAYMENTS/instances"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			stdout, _, err := runQB(t, append(tc.args, "--dry-run", "--json")...)
			if err != nil {
				t.Fatalf("execute: %v; stdout=%q", err, stdout)
			}
			env := decodePlan(t, stdout)
			if env.Method != "POST" || env.Dial {
				t.Fatalf("envelope = %+v", env)
			}
			if !strings.Contains(env.URL, tc.wantURL) {
				t.Fatalf("url %q missing %q", env.URL, tc.wantURL)
			}
			var body struct {
				OperationName string `json:"operationName"`
			}
			if env.Body != nil && json.Unmarshal(env.Body, &body) == nil && body.OperationName != "" {
				if tc.wantOp != "" && body.OperationName != tc.wantOp {
					t.Fatalf("operationName = %q, want %q", body.OperationName, tc.wantOp)
				}
			} else if tc.wantOp != "" && !strings.Contains(string(env.Body), tc.wantOp) {
				t.Fatalf("body %s missing op %q", env.Body, tc.wantOp)
			}
			if strings.Contains(stdout, "intuit_apikey") || strings.Contains(stdout, "Authorization") {
				t.Fatalf("secrets in stdout:\n%s", stdout)
			}
		})
	}
}

// No credentials: every write must exit as an auth failure before surfacing
// anything else. Grouped per entity family as the assignment requires.
func TestSalestxWritesFailFastWithoutCreds(t *testing.T) {
	groups := [][]string{
		{"salestx", "invoice", "create", "--customer", "1"},
		{"salestx", "estimate", "delete", "--id", "1"},
		{"salestx", "payment", "create", "--customer", "1"},
		{"salestx", "creditmemo", "update", "--id", "1", "--amount", "2"},
		{"salestx", "delayedcharge", "create", "--customer", "1"},
		{"salestx", "salesreceipt", "delete", "--id", "1"},
		{"salestx", "gst", "file", "--amount", "1", "--account", "91", "--agency", "ag"},
		{"salestx", "bas", "lodge", "--id", "ret"},
	}
	for _, args := range groups {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := NewRootCommand()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append(args, "--home", t.TempDir()))
			err := root.Execute()
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != ExitAuthError {
				t.Fatalf("err = %v, want ExitAuthError", err)
			}
		})
	}
}

func TestSalestxDelayedChargeListUnsupportedContract(t *testing.T) {
	stdout, elapsed, err := runQB(t, "salestx", "delayedcharge", "list", "--dry-run", "--json")
	if elapsed > 2*1e9 {
		t.Fatalf("dry-run took %s — likely dialed", elapsed)
	}
	if err != nil {
		t.Fatalf("dry-run: %v; stdout=%q", err, stdout)
	}
	env := decodePlan(t, stdout)
	if env.Dial || !env.DryRun || env.Method != "" || env.URL != "" {
		t.Fatalf("plan must carry no sendable request: %+v", env)
	}
	if !strings.Contains(env.Note, "unsupported") {
		t.Fatalf("note = %q, want unsupported-contract reason", env.Note)
	}

	for _, extra := range [][]string{{"--limit", "5"}, {"--id", "41"}, {"--query", "DC-9"}} {
		args := append(append([]string{"salestx", "delayedcharge", "list"}, extra...), "--json")
		_, elapsed, err := runQB(t, args...)
		if elapsed > 2*1e9 {
			t.Fatalf("%v took %s — likely dialed", args, elapsed)
		}
		if !errors.Is(err, client.ErrUnsupportedQueryContext) {
			t.Fatalf("%v: err = %v, want ErrUnsupportedQueryContext", args, err)
		}
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != ExitInputError {
			t.Fatalf("%v: err = %v, want ExitInputError", args, err)
		}
	}
}

func TestSalestxTPARGenerateStaysNotWired(t *testing.T) {
	// Dry-run plans honestly ("no captured API"), live run fails with the
	// documented sentinel after the session check.
	stdout, _, err := runQB(t, "salestx", "tpar", "generate", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("dry-run: %v; stdout=%q", err, stdout)
	}
	env := decodePlan(t, stdout)
	if env.Dial {
		t.Fatalf("envelope = %+v", env)
	}

	_, _, err = runQB(t, "salestx", "tpar", "generate", "--home", t.TempDir())
	if err == nil {
		t.Fatal("tpar generate without creds must fail (auth first)")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != ExitAuthError {
		t.Fatalf("err = %v, want ExitAuthError", err)
	}
}
