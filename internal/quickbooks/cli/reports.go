package cli

import (
	"context"

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
	reportEnt.AddCommand(newStubCmd(flags, "reports report", "get", "report read (not wired)"))
	cmd.AddCommand(reportEnt)
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
	transaction_listEnt := &cobra.Command{Use: "transaction-list", Short: "transaction-list"}
	transaction_listEnt.AddCommand(newStubCmd(flags, "reports transaction-list", "get", "transaction-list read"))
	cmd.AddCommand(transaction_listEnt)
	trial_balanceEnt := &cobra.Command{Use: "trial-balance", Short: "trial-balance"}
	trial_balanceEnt.AddCommand(newStubCmd(flags, "reports trial-balance", "get", "trial-balance read"))
	cmd.AddCommand(trial_balanceEnt)
	return cmd
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
// referencing a report token (PANDL, BAL_SHEET, or an sbg: saved-report id).
func newCustomCreateCmd(flags *rootFlags) *cobra.Command {
	var name, report, title, dateMacro string
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
			res, err := client.ReplayFolioMutate(ctx, "custom", "create", "", name, report, title, dateMacro, "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "report group name (required)")
	cmd.Flags().StringVar(&report, "report", "PANDL", "report token for the page (PANDL, BAL_SHEET, or an sbg: saved-report id)")
	cmd.Flags().StringVar(&title, "title", "", "page title (defaults to the report's display name)")
	cmd.Flags().StringVar(&dateMacro, "date-macro", "thisyeartodate", "report date macro (e.g. thisyeartodate, thismonth, lastmonth)")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.REPORTS.CUSTOM_CREATE")
	return cmd
}

// newManagementCreateCmd implements `reports management create`: POST a FOLIO
// folio — cover page plus report pages — the object Management reports lists.
func newManagementCreateCmd(flags *rootFlags) *cobra.Command {
	var name, report, dateMacro, preparedBy string
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
			res, err := client.ReplayFolioMutate(ctx, "management", "create", "", name, report, "", dateMacro, preparedBy)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "management report name (required)")
	cmd.Flags().StringVar(&report, "report", "PANDL", "report token for the page (PANDL, BAL_SHEET, or an sbg: saved-report id)")
	cmd.Flags().StringVar(&dateMacro, "date-macro", "thisyear", "report date macro (e.g. thisyear, thisyeartodate)")
	cmd.Flags().StringVar(&preparedBy, "prepared-by", "", "cover-page 'prepared by' (defaults to the account email)")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.REPORTS.MANAGEMENT_CREATE")
	return cmd
}

// newManagementUpdateCmd implements `reports management update`: PUT the full
// folio object with a new name (the service 500s on partial bodies).
func newManagementUpdateCmd(flags *rootFlags) *cobra.Command {
	var id, name string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Rename a management report (universalreportinsights PUT folio)",
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
			res, err := client.ReplayFolioMutate(ctx, "management", "update", id, name, "", "", "", "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "folio id (required, e.g. sbg:...)")
	cmd.Flags().StringVar(&name, "name", "", "new report name (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("name")
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
