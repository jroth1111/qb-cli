package cli

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// These plans check parameter/default metadata only. BAS/IAS/GST amendments,
// forecast and performance still have unresolved semantic routes in v3ByID;
// a ProfitAndLoss plan is not evidence that those feature reports work.
func TestReportContractMetadataMatchesDefaults(t *testing.T) {
	for id, spec := range v3ByID {
		if spec.Report == "" {
			continue
		}
		t.Run(id, func(t *testing.T) {
			e, ok := catalogByID(id)
			if !ok {
				t.Fatal("report missing from catalog")
			}
			cmd := newV3ReportCmd(&rootFlags{dryRun: true, asJSON: true}, e, e.Command, spec.Report)
			params := paramsFor(id)
			for _, p := range params {
				f := cmd.Flags().Lookup(p.Name)
				if f == nil || f.DefValue != p.Default || f.Usage != p.Help || p.Required {
					t.Fatalf("metadata differs from optional/defaulted report flag: %+v flag=%+v", p, f)
				}
				if strings.Contains(p.Help, "accepted, not forwarded") {
					t.Fatalf("stale ignored-option help: %s", p.Help)
				}
			}
			if params[0].Name != "report" || params[0].Default != spec.Report {
				t.Fatalf("missing report default: %+v", params)
			}
			// Defaults must execute without supplying UI catalog 'required' flags.
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{})
			if err := cmd.ExecuteContext(t.Context()); err != nil {
				t.Fatalf("default report plan: %v", err)
			}
			var plan planEnvelope
			if err := json.Unmarshal(out.Bytes(), &plan); err != nil || !plan.DryRun || plan.Dial {
				t.Fatalf("invalid default plan: %s (%v)", out.String(), err)
			}
			if spec.Report != "TAXABLE_PAYMENTS" && !strings.HasSuffix(plan.URL, "/reports/"+spec.Report+"?minorversion=73") {
				t.Fatalf("default report request changed: %s", plan.URL)
			}
		})
	}
}

func TestReportContractPlanForwardsOptions(t *testing.T) {
	e, _ := catalogByID("QBO.REPORTS.REPORT_READ")
	cmd := newV3ReportCmd(&rootFlags{dryRun: true, asJSON: true}, e, e.Command, "ProfitAndLoss")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--report", "BalanceSheet", "--date-range", "2026-07-01,2026-08-31", "--accounting-method", "cash", "--columns", "monthly"})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	var plan planEnvelope
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(plan.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{"minorversion": {"73"}, "start_date": {"2026-07-01"}, "end_date": {"2026-08-31"}, "accounting_method": {"Cash"}, "summarize_column_by": {"Month"}}
	if !reflect.DeepEqual(u.Query(), want) || !strings.HasSuffix(u.Path, "/reports/BalanceSheet") {
		t.Fatalf("wrong report plan: %s", plan.URL)
	}
}

func TestReportContractRejectsIgnoredFlags(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	for _, dry := range []bool{false, true} {
		for _, tc := range []struct {
			id   string
			args []string
			want string
		}{
			{"QBO.REPORTS.REPORT_READ", []string{"--limit", "1"}, "--limit"},
			{"QBO.REPORTS.REPORT_READ", []string{"--limit", "20"}, "--limit"},
			{"QBO.REPORTS.REPORT_READ", []string{"--id", "42"}, "--id"},
			{"QBO.FEED.REC_REPORT", []string{"--account-id", "204"}, "--account-id"},
			{"QBO.REPORTS.PERFORMANCE_READ", []string{"--accounting-method", "invalid"}, "--accounting-method"},
			{"QBO.REPORTS.REPORT_READ", []string{"--columns", "previous-year,percent-change"}, "--columns"},
			{"QBO.TAX.TPAR_READ", []string{"--accounting-method", "cash"}, "TAXABLE_PAYMENTS"},
			{"QBO.REPORTS.REPORT_READ", []string{"--report", "TAXABLE_PAYMENTS", "--columns", "Month"}, "TAXABLE_PAYMENTS"},
		} {
			e, _ := catalogByID(tc.id)
			cmd := newV3ReportCmd(&rootFlags{dryRun: dry, asJSON: true}, e, e.Command, v3ByID[tc.id].Report)
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)
			err := cmd.ExecuteContext(t.Context())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("dry=%v %s %v: %v", dry, tc.id, tc.args, err)
			}
			if out.Len() != 0 {
				t.Fatalf("unsupported options printed a success/plan: %s", out.String())
			}
		}
	}
}
