package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

func newReportsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reports",
		Short: "Reports and forecasts",
		Long:  "Reports and forecasts. qb reports <entity> <verb>. Stubs return not-wired.",
	}
	account_listEnt := &cobra.Command{Use: "account-list", Short: "account-list"}
	account_listEnt.AddCommand(newStubCmd(flags, "reports account-list", "get", "account-list read"))
	cmd.AddCommand(account_listEnt)
	account_list_detailEnt := &cobra.Command{Use: "account-list-detail", Short: "account-list-detail"}
	account_list_detailEnt.AddCommand(newStubCmd(flags, "reports account-list-detail", "get", "account-list-detail read"))
	cmd.AddCommand(account_list_detailEnt)
	aged_payablesEnt := &cobra.Command{Use: "aged-payables", Short: "aged-payables"}
	aged_payablesEnt.AddCommand(newStubCmd(flags, "reports aged-payables", "get", "aged-payables read"))
	cmd.AddCommand(aged_payablesEnt)
	aged_payable_detailEnt := &cobra.Command{Use: "aged-payable-detail", Short: "aged-payable-detail"}
	aged_payable_detailEnt.AddCommand(newStubCmd(flags, "reports aged-payable-detail", "get", "aged-payable-detail read"))
	cmd.AddCommand(aged_payable_detailEnt)
	aged_receivablesEnt := &cobra.Command{Use: "aged-receivables", Short: "aged-receivables"}
	aged_receivablesEnt.AddCommand(newStubCmd(flags, "reports aged-receivables", "get", "aged-receivables read"))
	cmd.AddCommand(aged_receivablesEnt)
	aged_receivable_detailEnt := &cobra.Command{Use: "aged-receivable-detail", Short: "aged-receivable-detail"}
	aged_receivable_detailEnt.AddCommand(newStubCmd(flags, "reports aged-receivable-detail", "get", "aged-receivable-detail read"))
	cmd.AddCommand(aged_receivable_detailEnt)
	cash_flowEnt := &cobra.Command{Use: "cash-flow", Short: "cash-flow"}
	cash_flowEnt.AddCommand(newStubCmd(flags, "reports cash-flow", "get", "cash-flow read (not wired)"))
	cmd.AddCommand(cash_flowEnt)
	class_salesEnt := &cobra.Command{Use: "class-sales", Short: "class-sales"}
	class_salesEnt.AddCommand(newStubCmd(flags, "reports class-sales", "get", "class-sales read"))
	cmd.AddCommand(class_salesEnt)
	customEnt := &cobra.Command{Use: "custom", Short: "custom"}
	customEnt.AddCommand(newCustomCreateCmd(flags))
	customEnt.AddCommand(newStubCmd(flags, "reports custom", "get", "custom read (universalreportinsights /v1/folio CRB_GROUP)"))
	cmd.AddCommand(customEnt)
	department_salesEnt := &cobra.Command{Use: "department-sales", Short: "department-sales"}
	department_salesEnt.AddCommand(newStubCmd(flags, "reports department-sales", "get", "department-sales read"))
	cmd.AddCommand(department_salesEnt)
	forecastEnt := &cobra.Command{Use: "forecast", Short: "forecast"}
	forecastEnt.AddCommand(newForecastCreateCmd(flags))
	forecastEnt.AddCommand(newForecastUpdateCmd(flags))
	forecastEnt.AddCommand(newForecastDeleteCmd(flags))
	forecastEnt.AddCommand(newStubCmd(flags, "reports forecast", "get", "forecast read (planningforecasting getAllBusinessForecasts)"))
	cmd.AddCommand(forecastEnt)
	general_ledgerEnt := &cobra.Command{Use: "general-ledger", Short: "general-ledger"}
	general_ledgerEnt.AddCommand(newStubCmd(flags, "reports general-ledger", "get", "general-ledger read"))
	cmd.AddCommand(general_ledgerEnt)
	managementEnt := &cobra.Command{Use: "management", Short: "management"}
	managementEnt.AddCommand(newManagementCreateCmd(flags))
	managementEnt.AddCommand(newManagementUpdateCmd(flags))
	managementEnt.AddCommand(newStubCmd(flags, "reports management", "get", "management read (universalreportinsights /v1/folio)"))
	managementEnt.AddCommand(newManagementAuditCmd(flags))
	managementEnt.AddCommand(newManagementAttributionCmd(flags))
	cmd.AddCommand(managementEnt)
	performanceEnt := &cobra.Command{Use: "performance", Short: "performance"}
	performanceEnt.AddCommand(newPerformanceCreateCmd(flags))
	performanceEnt.AddCommand(newPerformanceDeleteCmd(flags))
	performanceEnt.AddCommand(newPerformanceUpdateCmd(flags))
	performanceEnt.AddCommand(newStubCmd(flags, "reports performance", "get", "performance read (universalreportinsights /v1/metrics)"))
	cmd.AddCommand(performanceEnt)
	reportEnt := &cobra.Command{Use: "report", Short: "report"}
	reportEnt.AddCommand(newSavedReportCmd(flags, "create"))
	reportEnt.AddCommand(newSavedReportCmd(flags, "update"))
	reportEnt.AddCommand(newSavedReportCmd(flags, "delete"))
	reportEnt.AddCommand(newStubCmd(flags, "reports report", "get", "report read (v3 report)"))
	cmd.AddCommand(reportEnt)
	savedEnt := &cobra.Command{Use: "saved", Short: "saved custom reports (CRB_REPORT definitions)"}
	savedEnt.AddCommand(newStubCmd(flags, "reports saved", "list", "saved report list (universalreportinsights /v1/reports/qbo)"))
	savedEnt.AddCommand(newStubCmd(flags, "reports saved", "get", "saved report read — definition incl. dataRequest filters"))
	cmd.AddCommand(savedEnt)

	memorizedEnt := &cobra.Command{Use: "memorized", Short: "memorized custom reports (v4/entities reportDefinitions)"}
	memorizedEnt.AddCommand(newStubCmd(flags, "reports memorized", "list", "memorized report list (v4/entities reportDefinitions MEMORIZED)"))
	memorizedEnt.AddCommand(newStubCmd(flags, "reports memorized", "get", "memorized report read — full definition; --id accepts composite id or mem_rpt_id"))
	memorizedEnt.AddCommand(newMemorizedRunCmd(flags))
	cmd.AddCommand(memorizedEnt)
	balance_sheetEnt := &cobra.Command{Use: "balance-sheet", Short: "balance-sheet"}
	balance_sheetEnt.AddCommand(newStubCmd(flags, "reports balance-sheet", "get", "balance-sheet read"))
	cmd.AddCommand(balance_sheetEnt)
	customer_balanceEnt := &cobra.Command{Use: "customer-balance", Short: "customer-balance"}
	customer_balanceEnt.AddCommand(newStubCmd(flags, "reports customer-balance", "get", "customer-balance read"))
	cmd.AddCommand(customer_balanceEnt)
	customer_balance_detailEnt := &cobra.Command{Use: "customer-balance-detail", Short: "customer-balance-detail"}
	customer_balance_detailEnt.AddCommand(newStubCmd(flags, "reports customer-balance-detail", "get", "customer-balance-detail read"))
	cmd.AddCommand(customer_balance_detailEnt)
	customer_incomeEnt := &cobra.Command{Use: "customer-income", Short: "customer-income"}
	customer_incomeEnt.AddCommand(newStubCmd(flags, "reports customer-income", "get", "customer-income read"))
	cmd.AddCommand(customer_incomeEnt)
	customer_salesEnt := &cobra.Command{Use: "customer-sales", Short: "customer-sales"}
	customer_salesEnt.AddCommand(newStubCmd(flags, "reports customer-sales", "get", "customer-sales read"))
	cmd.AddCommand(customer_salesEnt)
	profit_lossEnt := &cobra.Command{Use: "profit-loss", Short: "profit-loss"}
	profit_lossEnt.AddCommand(newStubCmd(flags, "reports profit-loss", "get", "profit-loss read"))
	cmd.AddCommand(profit_lossEnt)
	vendor_balanceEnt := &cobra.Command{Use: "vendor-balance", Short: "vendor-balance"}
	vendor_balanceEnt.AddCommand(newStubCmd(flags, "reports vendor-balance", "get", "vendor-balance read"))
	cmd.AddCommand(vendor_balanceEnt)
	vendor_balance_detailEnt := &cobra.Command{Use: "vendor-balance-detail", Short: "vendor-balance-detail"}
	vendor_balance_detailEnt.AddCommand(newStubCmd(flags, "reports vendor-balance-detail", "get", "vendor-balance-detail read"))
	cmd.AddCommand(vendor_balance_detailEnt)
	vendor_expensesEnt := &cobra.Command{Use: "vendor-expenses", Short: "vendor-expenses"}
	vendor_expensesEnt.AddCommand(newStubCmd(flags, "reports vendor-expenses", "get", "vendor-expenses read"))
	cmd.AddCommand(vendor_expensesEnt)
	inventory_valuationEnt := &cobra.Command{Use: "inventory-valuation", Short: "inventory-valuation"}
	inventory_valuationEnt.AddCommand(newStubCmd(flags, "reports inventory-valuation", "get", "inventory-valuation read"))
	cmd.AddCommand(inventory_valuationEnt)
	inventory_valuation_detailEnt := &cobra.Command{Use: "inventory-valuation-detail", Short: "inventory-valuation-detail"}
	inventory_valuation_detailEnt.AddCommand(newStubCmd(flags, "reports inventory-valuation-detail", "get", "inventory-valuation-detail read"))
	cmd.AddCommand(inventory_valuation_detailEnt)
	item_salesEnt := &cobra.Command{Use: "item-sales", Short: "item-sales"}
	item_salesEnt.AddCommand(newStubCmd(flags, "reports item-sales", "get", "item-sales read"))
	cmd.AddCommand(item_salesEnt)
	journal_reportEnt := &cobra.Command{Use: "journal-report", Short: "journal-report"}
	journal_reportEnt.AddCommand(newStubCmd(flags, "reports journal-report", "get", "journal-report read"))
	cmd.AddCommand(journal_reportEnt)
	profit_loss_detailEnt := &cobra.Command{Use: "profit-loss-detail", Short: "profit-loss-detail"}
	profit_loss_detailEnt.AddCommand(newStubCmd(flags, "reports profit-loss-detail", "get", "profit-loss-detail read"))
	cmd.AddCommand(profit_loss_detailEnt)
	tax_summaryEnt := &cobra.Command{Use: "tax-summary", Short: "tax-summary"}
	tax_summaryEnt.AddCommand(newStubCmd(flags, "reports tax-summary", "get", "tax-summary read"))
	cmd.AddCommand(tax_summaryEnt)
	transaction_listEnt := &cobra.Command{Use: "transaction-list", Short: "transaction-list"}
	transaction_listEnt.AddCommand(newStubCmd(flags, "reports transaction-list", "get", "transaction-list read"))
	cmd.AddCommand(transaction_listEnt)
	transaction_list_by_customerEnt := &cobra.Command{Use: "transaction-list-by-customer", Short: "transaction-list-by-customer"}
	transaction_list_by_customerEnt.AddCommand(newStubCmd(flags, "reports transaction-list-by-customer", "get", "transaction-list-by-customer read"))
	cmd.AddCommand(transaction_list_by_customerEnt)
	transaction_list_by_vendorEnt := &cobra.Command{Use: "transaction-list-by-vendor", Short: "transaction-list-by-vendor"}
	transaction_list_by_vendorEnt.AddCommand(newStubCmd(flags, "reports transaction-list-by-vendor", "get", "transaction-list-by-vendor read"))
	cmd.AddCommand(transaction_list_by_vendorEnt)
	transaction_list_with_splitsEnt := &cobra.Command{Use: "transaction-list-with-splits", Short: "transaction-list-with-splits"}
	transaction_list_with_splitsEnt.AddCommand(newStubCmd(flags, "reports transaction-list-with-splits", "get", "transaction-list-with-splits read"))
	cmd.AddCommand(transaction_list_with_splitsEnt)
	trial_balanceEnt := &cobra.Command{Use: "trial-balance", Short: "trial-balance"}
	trial_balanceEnt.AddCommand(newStubCmd(flags, "reports trial-balance", "get", "trial-balance read"))
	cmd.AddCommand(trial_balanceEnt)
	trial_balance_frEnt := &cobra.Command{Use: "trial-balance-fr", Short: "trial-balance-fr"}
	trial_balance_frEnt.AddCommand(newStubCmd(flags, "reports trial-balance-fr", "get", "trial-balance-fr read"))
	cmd.AddCommand(trial_balance_frEnt)
	return cmd
}

// newMemorizedRunCmd implements `reports memorized run`: execute a saved
// custom report server-side through v4/entities createReports_Report — the
// same call the classic reportv2 grid makes — and print the grid (sections,
// transaction lines, account/section totals, report total).
func newMemorizedRunCmd(flags *rootFlags) *cobra.Command {
	var id, dateRange, basis string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute a memorized custom report (v4/entities createReports_Report)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("--id is required (composite id or mem_rpt_id, e.g. 42)")
			}
			start, end := splitDateRange(dateRange)
			if (start == "") != (end == "") {
				return fmt.Errorf("--date-range needs start,end as YYYY-MM-DD,YYYY-MM-DD")
			}
			if b := strings.ToLower(strings.TrimSpace(basis)); b != "" && b != "cash" && b != "accrual" {
				return fmt.Errorf("--basis must be cash or accrual")
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports memorized run", ID: "QBO.REPORTS.MEMORIZED_RUN",
					Mode: modeRead, Method: "POST",
					URL:   client.PlannedMemorizedRunURL(),
					Flags: localFlagMap(cmd), Note: "v4/entities createReports_Report POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayMemorizedRun(ctx, id, client.MemorizedRunOptions{LowDate: start, HighDate: end, Basis: basis})
			if err != nil {
				return feedErr(flags, err)
			}
			// Overrides replay the saved filter set through the v3 report
			// engine (v3-shaped rows); the default saved period returns the
			// classic grid.
			if start != "" || basis != "" {
				printV3ReportRows(cmd.OutOrStdout(), flags, res)
			} else {
				printMemorizedRun(cmd.OutOrStdout(), flags, res)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "memorized report id — composite id or mem_rpt_id (e.g. 42)")
	cmd.Flags().StringVar(&dateRange, "date-range", "", "start,end as YYYY-MM-DD,YYYY-MM-DD (defaults to the saved report's period)")
	cmd.Flags().StringVar(&basis, "basis", "", "cash or accrual (defaults to the saved report's basis)")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.REPORTS.MEMORIZED_RUN")
	return cmd
}

// printMemorizedRun renders the executed grid: one line per row, cells joined,
// indented by the row's indent attribute; summary rows are printed as-is
// (the API marks them type=summary + isBold). --json emits the raw envelope.
func printMemorizedRun(stdout io.Writer, flags *rootFlags, res *client.ReportResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if res == nil {
		fmt.Fprintln(stdout, "no report")
		return
	}
	name := res.Report
	if name == "" {
		name = fmt.Sprint(res.Header["memRptId"])
	}
	fmt.Fprintf(stdout, "%s\n", name)
	var rows []struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Cells    []struct {
			Value        string `json:"value"`
			DisplayValue string `json:"displayValue"`
			Attributes   []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"attributes"`
		} `json:"cells"`
	}
	if json.Unmarshal(res.Rows, &rows) != nil {
		return
	}
	for _, row := range rows {
		var indent int
		var cells []string
		for _, c := range row.Cells {
			v := c.Value
			if c.DisplayValue != "" {
				v = c.DisplayValue
			}
			for _, a := range c.Attributes {
				if a.Name == "indent" {
					if value, err := strconv.Atoi(a.Value); err == nil {
						indent = max(0, min(value, 64))
					}
				}
			}
			if v != "" {
				cells = append(cells, v)
			}
		}
		fmt.Fprintf(stdout, "%s%s\n", strings.Repeat("  ", indent), strings.Join(cells, "\t"))
	}
}

// printV3ReportRows walks a v3 report's nested Row tree (Header/Section/Data/
// Summary) and prints each row's ColData tab-joined, indented by depth — a
// readable facsimile of the classic grouped grid.
func printV3ReportRows(stdout io.Writer, flags *rootFlags, res *client.ReportResult) {
	if flags.asJSON {
		printReport(stdout, flags, res)
		return
	}
	if res == nil {
		fmt.Fprintln(stdout, "no report")
		return
	}
	if res.Report != "" {
		fmt.Fprintln(stdout, res.Report)
	}
	var tree struct {
		Row []v3Row `json:"Row"`
	}
	if json.Unmarshal(res.Rows, &tree) != nil {
		return
	}
	for _, r := range tree.Row {
		printV3Row(stdout, r, 0)
	}
}

type v3Row struct {
	Type   string `json:"type"`
	Header *struct {
		ColData []v3Col `json:"ColData"`
	} `json:"Header"`
	ColData []v3Col `json:"ColData"`
	Rows    *struct {
		Row []v3Row `json:"Row"`
	} `json:"Rows"`
	Summary *struct {
		ColData []v3Col `json:"ColData"`
	} `json:"Summary"`
}

type v3Col struct {
	Value string `json:"value"`
}

func v3ColLine(cols []v3Col) string {
	var cells []string
	for _, c := range cols {
		cells = append(cells, c.Value)
	}
	return strings.Join(cells, "\t")
}

func printV3Row(w io.Writer, r v3Row, depth int) {
	pad := strings.Repeat("  ", depth)
	if r.Header != nil {
		fmt.Fprintln(w, pad+v3ColLine(r.Header.ColData))
	}
	if len(r.ColData) > 0 {
		fmt.Fprintln(w, pad+v3ColLine(r.ColData))
	}
	if r.Rows != nil {
		for _, sub := range r.Rows.Row {
			printV3Row(w, sub, depth+1)
		}
	}
	if r.Summary != nil {
		fmt.Fprintln(w, pad+v3ColLine(r.Summary.ColData))
	}
}

// newPerformanceCreateCmd implements `reports performance create`: POST one
// METRIC panel onto the company's Performance dashboard
// (dashboardframework.api.intuit.com/v1/dashboards/{id}/panels).
func newPerformanceCreateCmd(flags *rootFlags) *cobra.Command {
	var name, metric, view string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Add a Performance centre chart (dashboardframework POST panels)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports performance create", ID: "QBO.REPORTS.PERFORMANCE_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedDashboardsURL(),
					Flags: localFlagMap(cmd), Note: "dashboard lookup + POST panel; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayPerformanceMutate(ctx, "create", "", name, metric, view)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "chart name (required)")
	cmd.Flags().StringVar(&metric, "metric", "", "chart definition id (required, e.g. Revenue, Expenses, NetProfit)")
	cmd.Flags().StringVar(&view, "view", "", "visualization (defaults to the definition's defaultView)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("metric")
	applyCatalogHelp(cmd, "QBO.REPORTS.PERFORMANCE_CREATE")
	return cmd
}

// newPerformanceUpdateCmd implements `reports performance update`: PUT the
// full panel object with a new name.
func newPerformanceUpdateCmd(flags *rootFlags) *cobra.Command {
	var id, name string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Rename a Performance centre chart (dashboardframework PUT panel)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports performance update", ID: "QBO.REPORTS.PERFORMANCE_EDIT",
					Mode: modeWired, Method: "PUT",
					URL:   client.PlannedDashboardsURL(),
					Flags: localFlagMap(cmd), Note: "dashboard lookup + PUT panel; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayPerformanceMutate(ctx, "update", id, name, "", "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "panel id (required, e.g. sbg:...)")
	cmd.Flags().StringVar(&name, "name", "", "new chart name (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.REPORTS.PERFORMANCE_EDIT")
	return cmd
}

// newPerformanceDeleteCmd implements `reports performance delete`: DELETE the
// panel (a soft-deactivate — the service returns the panel with active:false).
func newPerformanceDeleteCmd(flags *rootFlags) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Remove a Performance centre chart (dashboardframework DELETE panel)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports performance delete", ID: "QBO.REPORTS.PERFORMANCE_DELETE",
					Mode: modeWired, Method: "DELETE",
					URL:   client.PlannedDashboardsURL(),
					Flags: localFlagMap(cmd), Note: "dashboard lookup + DELETE panel; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayPerformanceMutate(ctx, "delete", id, "", "", "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "panel id (required, e.g. sbg:...)")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.REPORTS.PERFORMANCE_DELETE")
	return cmd
}

// newCustomCreateCmd implements `reports custom create`: POST a CRB_GROUP
// folio — the object the Custom reports page lists — with one report page
// referencing a report token (PANDL, BAL_SHEET, an sbg: saved-report id, or
// a numeric mem_rpt_id like 42).
func newCustomCreateCmd(flags *rootFlags) *cobra.Command {
	var name, title, dateMacro, start, end string
	var reports []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a custom report group (universalreportinsights POST folio)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports custom create", ID: "QBO.REPORTS.CUSTOM_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedFolioMutateURL(),
					Flags: localFlagMap(cmd), Note: "POST CRB_GROUP folio; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayFolioMutate(ctx, "custom", "create", "", name, reports, title, dateMacro, start, end, "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "report group name (required)")
	cmd.Flags().StringSliceVar(&reports, "report", []string{"PANDL"}, "report token for a page — repeat to add pages in order (PANDL, BAL_SHEET, an sbg: saved-report id, or a mem_rpt_id); TOKEN:macro overrides that page's period (e.g. 70:all)")
	cmd.Flags().StringVar(&title, "title", "", "page title (defaults to the report's display name)")
	cmd.Flags().StringVar(&dateMacro, "date-macro", "thisyeartodate", "report date macro (e.g. thisyeartodate, thismonth, lastmonth)")
	cmd.Flags().StringVar(&start, "start-date", "", "pin a custom period start (YYYY-MM-DD); implies date-macro=custom")
	cmd.Flags().StringVar(&end, "end-date", "", "pin a custom period end (YYYY-MM-DD); implies date-macro=custom")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.REPORTS.CUSTOM_CREATE")
	return cmd
}

// newManagementCreateCmd implements `reports management create`: POST a FOLIO
// folio — cover page plus report pages — the object Management reports lists.
func newManagementCreateCmd(flags *rootFlags) *cobra.Command {
	var name, dateMacro, start, end, preparedBy string
	var reports []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a management report (universalreportinsights POST folio)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports management create", ID: "QBO.REPORTS.MANAGEMENT_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedFolioMutateURL(),
					Flags: localFlagMap(cmd), Note: "POST FOLIO folio; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayFolioMutate(ctx, "management", "create", "", name, reports, "", dateMacro, start, end, preparedBy)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "management report name (required)")
	cmd.Flags().StringSliceVar(&reports, "report", []string{"PANDL"}, "report token for a page — repeat to add pages in order (mem_rpt_ids like 42,41,59,70; sbg: ids; standard tokens); TOKEN:macro overrides that page's period (e.g. 70:all)")
	cmd.Flags().StringVar(&dateMacro, "date-macro", "thisyear", "report date macro (e.g. thisyear, thisyeartodate)")
	cmd.Flags().StringVar(&start, "start-date", "", "pin a custom period start (YYYY-MM-DD); implies date-macro=custom")
	cmd.Flags().StringVar(&end, "end-date", "", "pin a custom period end (YYYY-MM-DD); implies date-macro=custom")
	cmd.Flags().StringVar(&preparedBy, "prepared-by", "", "cover-page 'prepared by' (defaults to the account email)")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.REPORTS.MANAGEMENT_CREATE")
	return cmd
}

// newManagementUpdateCmd implements `reports management update`: PUT the full
// folio object (the service 500s on partial bodies) to rename it and/or pin a
// new reporting period — the monthly statement workflow.
func newManagementUpdateCmd(flags *rootFlags) *cobra.Command {
	var id, name, start, end string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a management report — rename and/or repin the period (universalreportinsights PUT folio)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports management update", ID: "QBO.REPORTS.MANAGEMENT_EDIT",
					Mode: modeWired, Method: "PUT",
					URL:   client.PlannedFolioMutateURL(),
					Flags: localFlagMap(cmd), Note: "GET folio + PUT full object; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayFolioMutate(ctx, "management", "update", id, name, nil, "", "", start, end, "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "folio id (required, e.g. sbg:...)")
	cmd.Flags().StringVar(&name, "name", "", "new report name")
	cmd.Flags().StringVar(&start, "start-date", "", "repin period start (YYYY-MM-DD); sets date-macro=custom on period-bound report pages")
	cmd.Flags().StringVar(&end, "end-date", "", "repin period end (YYYY-MM-DD); pages with reportDateMacro=all are untouched")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.REPORTS.MANAGEMENT_EDIT")
	return cmd
}

// newForecastCreateCmd implements `reports forecast create`:
// createBusinessForecast on planningforecasting.api.intuit.com. The service
// validates incrementally and requires the full input — name, mode, type,
// interval, dates, secondaryListType and forecastSetupData.
func newForecastCreateCmd(flags *rootFlags) *cobra.Command {
	var name, mode, ftype, interval, start, end, measure string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an Advanced forecast (createBusinessForecast)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports forecast create", ID: "QBO.REPORTS.FORECAST_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedForecastURL(),
					Flags: localFlagMap(cmd), Note: "POST graphql; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayForecastMutate(ctx, "create", "", name, mode, ftype, interval, start, end, measure)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "forecast name (required)")
	cmd.Flags().StringVar(&start, "start", "", "forecast start date yyyy-mm-dd (required)")
	cmd.Flags().StringVar(&end, "end", "", "forecast end date yyyy-mm-dd (required)")
	cmd.Flags().StringVar(&mode, "mode", "MANUAL", "AUTO_SMART, SMART_INTEL, MANUAL_FROM_SMART or MANUAL")
	cmd.Flags().StringVar(&ftype, "type", "PROFIT_AND_LOSS", "PROFIT_AND_LOSS, THREE_WAY or BALANCE_SHEET")
	cmd.Flags().StringVar(&interval, "interval", "MONTHLY", "MONTHLY, QUARTERLY or ANNUALLY")
	cmd.Flags().StringVar(&measure, "measure", "raw", "forecast baseline measure: raw, avg or smart")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	applyCatalogHelp(cmd, "QBO.REPORTS.FORECAST_CREATE")
	return cmd
}

// newForecastUpdateCmd implements `reports forecast update`: the service has
// no sparse patch — the CLI fetches the forecast's current input, applies
// the rename, and resends the complete object with its version.
func newForecastUpdateCmd(flags *rootFlags) *cobra.Command {
	var id, name string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Rename a forecast (updateBusinessForecast, full-object resend)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports forecast update", ID: "QBO.REPORTS.FORECAST_EDIT",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedForecastURL(),
					Flags: localFlagMap(cmd), Note: "GET forecast + POST full input; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayForecastMutate(ctx, "update", id, name, "", "", "", "", "", "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "forecast id (required)")
	cmd.Flags().StringVar(&name, "name", "", "new forecast name (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.REPORTS.FORECAST_EDIT")
	return cmd
}

// newForecastDeleteCmd implements `reports forecast delete`:
// deleteBusinessForecasts(forecastIds:[ID!]!) — hard delete, returns
// per-id status+description.
func newForecastDeleteCmd(flags *rootFlags) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a forecast (deleteBusinessForecasts)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports forecast delete", ID: "QBO.REPORTS.FORECAST_DELETE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedForecastURL(),
					Flags: localFlagMap(cmd), Note: "POST graphql; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayForecastMutate(ctx, "delete", id, "", "", "", "", "", "", "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "forecast id (required)")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.REPORTS.FORECAST_DELETE")
	return cmd
}

// newSavedReportCmd implements `reports report create|update|delete`:
// saved custom reports (CRB_REPORT) on universalreportinsights
// /v1/reports/qbo — the "Save as new report" contract captured from the
// report builder and live-proven on TC2 2026-09-19.
func newSavedReportCmd(flags *rootFlags, op string) *cobra.Command {
	var id, name, clone, share string
	verb := op
	if op == "update" {
		verb = "update"
	}
	catID := map[string]string{
		"create": "QBO.REPORTS.REPORT_CREATE",
		"update": "QBO.REPORTS.REPORT_EDIT",
		"delete": "QBO.REPORTS.REPORT_DELETE",
	}[op]
	method := map[string]string{"create": "POST", "update": "PUT", "delete": "DELETE"}[op]
	cmd := &cobra.Command{
		Use:   verb,
		Short: op + " a saved custom report (universalreportinsights /v1/reports/qbo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports report " + verb, ID: catID,
					Mode: modeWired, Method: method,
					URL:   client.PlannedSavedReportURL(),
					Flags: localFlagMap(cmd), Note: "saved report " + op + "; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplaySavedReportMutate(ctx, op, id, name, clone, share)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	if op == "create" {
		cmd.Flags().StringVar(&name, "name", "", "name to save the custom report under (required)")
		cmd.Flags().StringVar(&clone, "clone", "", "clone the dataRequest of an existing saved report (sbg:…)")
		cmd.Flags().StringVar(&share, "share", "", "share scope after save: private or team")
		_ = cmd.MarkFlagRequired("name")
	} else {
		cmd.Flags().StringVar(&id, "id", "", "saved report id sbg:… (required)")
		_ = cmd.MarkFlagRequired("id")
		if op == "update" {
			cmd.Flags().StringVar(&name, "name", "", "new report name (required)")
			_ = cmd.MarkFlagRequired("name")
		}
	}
	applyCatalogHelp(cmd, catID)
	return cmd
}

// newManagementAuditCmd implements `reports management audit`: execute every
// REPORT page of a management folio for each month in --from..--to and
// reconcile the constituents — per-unit detail nets vs the combined summary,
// plus the all-time distributions ledger sliced per period.
func newManagementAuditCmd(flags *rootFlags) *cobra.Command {
	var id, from, to string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit a management report's constituents per period (unit detail vs combined summary vs distributions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("--id is required (folio id, e.g. sbg:…)")
			}
			if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
				return fmt.Errorf("--from and --to are required (YYYY-MM)")
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports management audit", ID: "QBO.REPORTS.MANAGEMENT_AUDIT",
					Mode: modeRead, Method: "GET+POST(read)",
					URL:   "https://universalreportinsights.api.intuit.com/v1/folio/" + id + " + per-page report executions",
					Flags: localFlagMap(cmd), Note: "folio GET + resolved-filter v3 report reads + v4/entities saved-report resolution; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			audit, err := client.AuditFolio(ctx, id, from, to)
			if err != nil {
				return feedErr(flags, err)
			}
			printFolioAudit(cmd.OutOrStdout(), flags, audit)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "folio id (required, e.g. sbg:…)")
	cmd.Flags().StringVar(&from, "from", "", "first statement month YYYY-MM (required)")
	cmd.Flags().StringVar(&to, "to", "", "last statement month YYYY-MM (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	applyCatalogHelp(cmd, "QBO.REPORTS.MANAGEMENT_AUDIT")
	return cmd
}

// newManagementAttributionCmd implements `reports management attribution`:
// scan deposit and invoice lines in a statement window for cancellation,
// break-lease, claim, or guest-fee semantics posting to the owner revenue
// account — money that belongs to Management Income, not the owner.
// Detection-only; reclassification stays a human decision.
func newManagementAttributionCmd(flags *rootFlags) *cobra.Command {
	var from, to, account string
	cmd := &cobra.Command{
		Use:   "attribution",
		Short: "Find revenue lines carrying MSA semantics (cancellation/claim/guest fee) posted to the owner revenue account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
				return fmt.Errorf("--from and --to are required (YYYY-MM)")
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "reports management attribution",
					Mode:    modeRead, Method: "GET(read)",
					URL:   "v3 query deposit+invoice TxnDate in window",
					Flags: localFlagMap(cmd), Note: "two paged v3 entity reads + local line scan; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			audit, err := client.AuditRevenueAttribution(ctx, from, to, account)
			if err != nil {
				return feedErr(flags, err)
			}
			printAttributionAudit(cmd.OutOrStdout(), flags, audit)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "first statement month YYYY-MM (required)")
	cmd.Flags().StringVar(&to, "to", "", "last statement month YYYY-MM (required)")
	cmd.Flags().StringVar(&account, "account", "", "owner revenue account id (default 47, Client Property Revenue)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// printAttributionAudit renders one row per flagged line — severity first so
// msa-semantics rows sort ahead of review rows, then date, entity, class.
func printAttributionAudit(stdout io.Writer, flags *rootFlags, audit *client.AttributionAudit) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(audit)
		return
	}
	fmt.Fprintf(stdout, "window=%s→%s\taccount=%s\tscanned=%d\tfindings=%d\n",
		audit.From, audit.To, audit.RevenueAccount, audit.Scanned, len(audit.Findings))
	if len(audit.Findings) == 0 {
		return
	}
	sort.Slice(audit.Findings, func(i, j int) bool {
		a, b := audit.Findings[i], audit.Findings[j]
		if a.Severity != b.Severity {
			return a.Severity == "msa"
		}
		if a.TxnDate != b.TxnDate {
			return a.TxnDate < b.TxnDate
		}
		return a.EntityID < b.EntityID
	})
	fmt.Fprintln(stdout, "severity\tdirection\tdate\tentity\tid\tclass\tamount\tdescription")
	for _, f := range audit.Findings {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			f.Severity, f.Direction, f.TxnDate, f.Entity, f.EntityID, f.Class, money(f.Amount), f.Description)
	}
}

// printFolioAudit renders the reconciliation matrix: one row per period, one
// column per unit page, the combined total, and the tie-out delta — then the
// distributions ledger and cumulative position, then structural warnings.
func printFolioAudit(stdout io.Writer, flags *rootFlags, audit *client.FolioAudit) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(audit)
		return
	}
	fmt.Fprintf(stdout, "%s\t%s\n", audit.FolioID, audit.FolioName)
	fmt.Fprintf(stdout, "window=%s→%s\n", audit.From, audit.To)
	for _, p := range audit.Pages {
		fmt.Fprintf(stdout, "  page p%d\t%s\tkind=%s\ttoken=%s\tclasses=%s\n",
			p.Seq, p.Title, p.Kind, p.Token, strings.Join(p.Classes, ","))
	}
	// stable column order: unit titles sorted
	var unitTitles []string
	for _, p := range audit.Pages {
		if p.Kind == "unit" {
			unitTitles = append(unitTitles, p.Title)
		}
	}
	sort.Strings(unitTitles)
	fmt.Fprintf(stdout, "\nperiod\t%s\tcombined\tdelta\tstatus\n", strings.Join(unitTitles, "\t"))
	for _, pr := range audit.Periods {
		cells := []string{pr.Label}
		for _, t := range unitTitles {
			cells = append(cells, money(pr.Units[t]))
		}
		status := "ok"
		if !pr.OK {
			status = "MISMATCH"
		}
		cells = append(cells, money(pr.CombinedSum), money(pr.Delta), status)
		fmt.Fprintln(stdout, strings.Join(cells, "\t"))
	}
	if len(audit.Distributions) > 0 {
		fmt.Fprintln(stdout, "\ndistributions:")
		for _, d := range audit.Distributions {
			cls := d.Class
			if cls == "" {
				cls = "(unscoped)"
			}
			var months []string
			for m, v := range d.ByMonth {
				if v != 0 {
					months = append(months, m+"="+money(v))
				}
			}
			sort.Strings(months)
			fmt.Fprintf(stdout, "  %s\tclass=%s\tin-window=%s\tall-time=%s\t%s\n",
				d.Page, cls, money(d.InRange), money(d.AllTime), strings.Join(months, " "))
		}
	}
	if len(audit.Position) > 0 {
		fmt.Fprintln(stdout, "\nposition (earned − distributed, window):")
		var keys []string
		for k := range audit.Position {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(stdout, "  %s\t%s\n", k, money(audit.Position[k]))
		}
	}
	if len(audit.Warnings) > 0 {
		fmt.Fprintln(stdout, "\nwarnings:")
		for _, w := range audit.Warnings {
			fmt.Fprintf(stdout, "  ! %s\n", w)
		}
	}
}

func money(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
