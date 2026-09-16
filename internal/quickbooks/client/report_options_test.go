package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

const reportContractBody = `{"Header":{"ReportName":"ProfitAndLoss","StartPeriod":"2026-07-01","EndPeriod":"2026-08-31","Currency":"AUD","ReportBasis":"Cash"},"Columns":{"Column":[{"ColTitle":""},{"ColTitle":"Total"}]},"Rows":{"Row":[]}}`

func TestReportOptionsReachRequest(t *testing.T) {
	saveUsable(t)
	for _, tc := range []struct {
		name, start, end string
		opts             []ReportOptions
		want             url.Values
	}{
		{"legacy defaults", "", "", nil, url.Values{"minorversion": {"73"}}},
		{"zero options", "", "", []ReportOptions{{}}, url.Values{"minorversion": {"73"}}},
		{"cash monthly", "2026-07-01", "2026-08-31", []ReportOptions{{AccountingMethod: "cash", SummarizeColumnBy: "monthly"}}, url.Values{
			"minorversion": {"73"}, "start_date": {"2026-07-01"}, "end_date": {"2026-08-31"}, "accounting_method": {"Cash"}, "summarize_column_by": {"Month"},
		}},
		{"accrual canonical", "", "", []ReportOptions{{AccountingMethod: "Accrual", SummarizeColumnBy: "Quarter"}}, url.Values{
			"minorversion": {"73"}, "accounting_method": {"Accrual"}, "summarize_column_by": {"Quarter"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan url.Values, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v3/company/12345/reports/ProfitAndLoss" {
					t.Errorf("wrong request: %s %s", r.Method, r.URL.Path)
				}
				requests <- r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, reportContractBody)
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			res, err := ReplayReport(t.Context(), "ProfitAndLoss", tc.start, tc.end, tc.opts...)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-requests:
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("query = %v; want %v", got, tc.want)
				}
			default:
				t.Fatal("no HTTP request reached the server")
			}
			if res.Status != 200 || res.Report != "ProfitAndLoss" || !reflect.DeepEqual(res.Columns, []string{"Total"}) || res.Counts["bytes"] != len(reportContractBody) || res.Counts["columns"] != 1 {
				t.Fatalf("changed report projection: %+v", res)
			}
			encoded, err := json.Marshal(res)
			if err != nil {
				t.Fatal(err)
			}
			var shape map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &shape); err != nil || len(shape) != 6 {
				t.Fatalf("changed default output shape: %s (%v)", encoded, err)
			}
			if res.Header["ReportBasis"] != "Cash" || len(res.Rows) == 0 {
				t.Fatal("report basis or native rows missing")
			}
		})
	}
}

func TestReportOptionsRejectUnsupportedBeforeCredentials(t *testing.T) {
	noCredsDir(t)
	for _, tc := range []struct {
		name string
		opts []ReportOptions
		want string
	}{
		{"ProfitAndLoss", []ReportOptions{{AccountingMethod: "invalid"}}, "accounting-method"},
		{"ProfitAndLoss", []ReportOptions{{SummarizeColumnBy: "previous-year,percent-change"}}, "columns"},
		{"ProfitAndLoss", []ReportOptions{{SummarizeColumnBy: "txn_date,amount"}}, "columns"},
		{"TAXABLE_PAYMENTS", []ReportOptions{{AccountingMethod: "Cash"}}, "TAXABLE_PAYMENTS"},
		{"TAXABLE_PAYMENTS", []ReportOptions{{SummarizeColumnBy: "Month"}}, "TAXABLE_PAYMENTS"},
		{"ProfitAndLoss", []ReportOptions{{}, {AccountingMethod: "Cash"}}, "at most one"},
	} {
		res, err := ReplayReport(t.Context(), tc.name, "", "", tc.opts...)
		if res != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("name=%s options=%+v: result=%+v error=%v", tc.name, tc.opts, res, err)
		}
	}
}

func TestReportRejectsFalseHTTP200(t *testing.T) {
	saveUsable(t)
	for name, body := range map[string]string{
		"html":                  `<html>session-marker</html>`,
		"xml":                   `<IntuitResponse><Report><Header><ReportName>ProfitAndLoss</ReportName></Header></Report></IntuitResponse>`,
		"malformed":             `{"Header":`,
		"null":                  `null`,
		"array":                 `[]`,
		"empty":                 `{}`,
		"query":                 `{"QueryResponse":{}}`,
		"fault":                 `{"Fault":{"Error":[{"Message":"session-marker"}]}}`,
		"lowercase fault":       `{"fault":{"type":"ValidationFault"}}`,
		"error":                 `{"error":"session-marker"}`,
		"errors with report":    strings.TrimSuffix(reportContractBody, "}") + `,"errors":[{"message":"session-marker"}]}`,
		"missing report name":   `{"Header":{},"Columns":{"Column":[]}}`,
		"wrong header type":     `{"Header":[],"Columns":{"Column":[]}}`,
		"wrong header field":    `{"Header":{"ReportName":"ProfitAndLoss","Currency":{}},"Columns":{"Column":[]}}`,
		"missing columns":       `{"Header":{"ReportName":"ProfitAndLoss"}}`,
		"empty columns wrapper": `{"Header":{"ReportName":"ProfitAndLoss"},"Columns":{}}`,
		"null columns":          `{"Header":{"ReportName":"ProfitAndLoss"},"Columns":{"Column":null}}`,
		"wrong columns type":    `{"Header":{"ReportName":"ProfitAndLoss"},"Columns":{"Column":{}}}`,
		"null column entry":     `{"Header":{"ReportName":"ProfitAndLoss"},"Columns":{"Column":[null]}}`,
		"wrong title type":      `{"Header":{"ReportName":"ProfitAndLoss"},"Columns":{"Column":[{"ColTitle":42}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			res, err := ReplayReport(t.Context(), "ProfitAndLoss", "", "")
			if err == nil || res != nil {
				t.Fatalf("false success: result=%+v error=%v", res, err)
			}
			if strings.Contains(err.Error(), "session-marker") {
				t.Fatal("error echoed response contents")
			}
		})
	}
}

func TestReportAllowsEmptyReport(t *testing.T) {
	saveUsable(t)
	body := `{"Header":{"ReportName":"ProfitAndLoss","Option":[{"Name":"NoReportData","Value":"true"}]},"Columns":{"Column":[]},"Rows":{}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	res, err := ReplayReport(t.Context(), "ProfitAndLoss", "", "")
	if err != nil || res == nil || res.Status != 200 || res.Counts["columns"] != 0 || len(res.Columns) != 0 {
		t.Fatalf("valid empty report rejected: result=%+v error=%v", res, err)
	}
}

func TestReportRetainsNativeRowsAndBasis(t *testing.T) {
	body := []byte(`{"Header":{"ReportName":"ProfitAndLoss","ReportBasis":"Cash"},"Columns":{"Column":[{"ColTitle":"Total"}]},"Rows":{"Row":[{"Summary":{"ColData":[{"value":"Net Income"},{"value":"12.34"}]}}]}}`)
	result, err := projectReport("ProfitAndLoss", body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Header["ReportBasis"] != "Cash" || !strings.Contains(string(result.Rows), "12.34") {
		t.Fatal("financial report values or basis were discarded")
	}
}

func TestBalanceReportsUseSnapshotDate(t *testing.T) {
	for _, name := range []string{"VendorBalance", "VendorBalanceDetail", "CustomerBalanceDetail", "AgedPayables", "AgedPayableDetail", "AgedReceivables", "AgedReceivableDetail"} {
		dates := ReportDateParams(name, "2026-01-01", "2026-09-12")
		if dates.Get("report_date") != "2026-09-12" || dates.Get("end_date") != "" {
			t.Fatalf("snapshot date lost: %s %v", name, dates)
		}
	}
}
