package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

type v3Spec struct {
	Entity string
	Report string
}

var v3ByID = map[string]v3Spec{
	"QBO.ACCOUNTING.BUDGET_READ":                  {Entity: "Budget"},
	"QBO.ACCOUNTING.BUDGET_SEARCH":                {Entity: "Budget"},
	"QBO.ACCOUNTING.CLASS_READ":                   {Entity: "Class"},
	"QBO.ACCOUNTING.CLASS_SEARCH":                 {Entity: "Class"},
	"QBO.ACCOUNTING.COA_READ":                     {Entity: "Account"},
	"QBO.ACCOUNTING.COA_SEARCH":                   {Entity: "Account"},
	"QBO.ACCOUNTING.DEPARTMENT_READ":              {Entity: "Department"},
	"QBO.ACCOUNTING.DEPARTMENT_SEARCH":            {Entity: "Department"},
	"QBO.ACCOUNTING.DEPOSIT_READ":                 {Entity: "Deposit"},
	"QBO.ACCOUNTING.JOURNAL_READ":                 {Entity: "JournalEntry"},
	"QBO.ACCOUNTING.LOCATION_READ":                {Entity: "Department"},
	"QBO.ACCOUNTING.RECURRING_READ":               {Entity: "RecurringTransaction"},
	"QBO.ACCOUNTING.RECURRING_SEARCH":             {Entity: "RecurringTransaction"},
	"QBO.ACCOUNTING.TRANSFER_READ":                {Entity: "Transfer"},
	"QBO.ACCOUNTING.PROJECT_READ":                 {Entity: "Customer"},
	"QBO.ACCOUNTING.PROJECT_SEARCH":               {Entity: "Customer"},
	"QBO.ACCOUNTING.PREPAID_READ":                 {Entity: "PrepaidSchedule"},
	"QBO.ACCOUNTING.REVENUE_RECOGNITION_READ":     {Entity: "RevenueRecognition"},
	"QBO.ADVANCED.CUSTOM_ROLES_READ":              {Entity: "Role"},
	"QBO.ADVANCED.WORKFLOWS_READ":                 {Entity: "Workflow"},
	"QBO.COMPANY.ATTACHABLE_READ":                 {Entity: "Attachable"},
	"QBO.COMPANY.ATTACHABLE_SEARCH":               {Entity: "Attachable"},
	"QBO.COMPANY.ATTACHMENT_READ":                 {Entity: "Attachable"},
	"QBO.COMPANY.BUSINESS_FEED_READ":              {Entity: "BusinessFeed"},
	"QBO.COMPANY.CURRENCY_READ":                   {Entity: "CompanyCurrency"},
	"QBO.COMPANY.EXCHANGE_RATE_READ":              {Entity: "ExchangeRate"},
	"QBO.COMPANY.EXCHANGE_RATE_SEARCH":            {Entity: "ExchangeRate"},
	"QBO.COMPANY.CUSTOM_FIELD_READ":               {Entity: "CustomField"},
	"QBO.COMPANY.FORM_STYLE_READ":                 {Entity: "FormStyle"},
	"QBO.COMPANY.LIST_READ":                       {Entity: "ListsPrefs"},
	"QBO.COMPANY.COMPANY_INFO_READ":               {Entity: "CompanyInfo"},
	"QBO.COMPANY.TERM_READ":                       {Entity: "Term"},
	"QBO.COMPANY.PAYMENT_METHOD_READ":             {Entity: "PaymentMethod"},
	"QBO.COMPANY.PREFERENCES_READ":                {Entity: "Preferences"},
	"QBO.COMPANY.ROLE_READ":                       {Entity: "Role"},
	"QBO.COMPANY.SETTINGS_READ":                   {Entity: "Preferences"},
	"QBO.CUSTOMERS.CUSTOMER_READ":                 {Entity: "Customer"},
	"QBO.CUSTOMERS.CUSTOMER_SEARCH":               {Entity: "Customer"},
	"QBO.CUSTOMERS.CUSTOMER_TYPE_READ":            {Entity: "CustomerType"},
	"QBO.CUSTOMERS.CUSTOMER_TYPE_SEARCH":          {Entity: "CustomerType"},
	"QBO.CUSTOMERS.OVERVIEW_READ":                 {Entity: "CustomersOverview"},
	"QBO.CUSTOMERS.CONTRACT_READ":                 {Entity: "Contract"},
	"QBO.CUSTOMERS.CONTRACT_SEARCH":               {Entity: "Contract"},
	"QBO.CUSTOMERS.PROPOSAL_READ":                 {Entity: "Proposal"},
	"QBO.CUSTOMERS.PROPOSAL_SEARCH":               {Entity: "Proposal"},
	"QBO.CUSTOMERS.REVIEW_READ":                   {Entity: "Review"},
	"QBO.CUSTOMERS.REVIEW_SEARCH":                 {Entity: "Review"},
	"QBO.EXPENSES.BILL_READ":                      {Entity: "Bill"},
	"QBO.EXPENSES.BILL_PAYMENT_READ":              {Entity: "BillPayment"},
	"QBO.EXPENSES.BILL_SEARCH":                    {Entity: "Bill"},
	"QBO.EXPENSES.CHEQUE_READ":                    {Entity: "Cheque"},
	"QBO.EXPENSES.CREDIT_CARD_PAYMENT_READ":       {Entity: "CreditCardPayment"},
	"QBO.EXPENSES.CREDIT_CARD_PAYMENT_SEARCH":     {Entity: "CreditCardPayment"},
	"QBO.EXPENSES.EXPENSE_READ":                   {Entity: "Purchase"},
	"QBO.EXPENSES.EXPENSE_SEARCH":                 {Entity: "Purchase"},
	"QBO.EXPENSES.RECEIPT_READ":                   {Entity: "Attachable"},
	"QBO.EXPENSES.RECEIPT_SEARCH":                 {Entity: "Attachable"},
	"QBO.EXPENSES.SUPPLIER_READ":                  {Entity: "Vendor"},
	"QBO.EXPENSES.SUPPLIER_SEARCH":                {Entity: "Vendor"},
	"QBO.EXPENSES.TIME_ACTIVITY_READ":             {Entity: "TimeActivity"},
	"QBO.EXPENSES.TIME_ACTIVITY_SEARCH":           {Entity: "TimeActivity"},
	"QBO.EXPENSES.VENDOR_CREDIT_READ":             {Entity: "VendorCredit"},
	"QBO.EXPENSES.PURCHASE_SEARCH":                {Entity: "Purchase"},
	"QBO.INVENTORY.ADJUST_READ":                   {Entity: "InventoryAdjustment"},
	"QBO.INVENTORY.ADJUST_SEARCH":                 {Entity: "InventoryAdjustment"},
	"QBO.INVENTORY.ITEM_READ":                     {Entity: "Item"},
	"QBO.INVENTORY.ITEM_SEARCH":                   {Entity: "Item"},
	"QBO.INVENTORY.ITEM_RECEIPT_READ":             {Entity: "ItemReceipt"},
	"QBO.INVENTORY.ITEM_RECEIPT_SEARCH":           {Entity: "ItemReceipt"},
	"QBO.INVENTORY.OVERVIEW_READ":                 {Entity: "InventoryOverview"},
	"QBO.INVENTORY.OVERVIEW_SEARCH":               {Entity: "InventoryOverviewSearch"},
	"QBO.INVENTORY.PURCHASE_ORDER_READ":           {Entity: "PurchaseOrder"},
	"QBO.INVENTORY.PURCHASE_ORDER_SEARCH":         {Entity: "PurchaseOrder"},
	"QBO.PAYROLL.EMPLOYEE_READ":                   {Entity: "Employee"},
	"QBO.PAYROLL.EMPLOYEE_SEARCH":                 {Entity: "Employee"},
	"QBO.PAYROLL.TIMESHEET_READ":                  {Entity: "TimeActivity"},
	"QBO.SALES.CREDIT_MEMO_READ":                  {Entity: "CreditMemo"},
	"QBO.SALES.ESTIMATE_READ":                     {Entity: "Estimate"},
	"QBO.SALES.ESTIMATE_SEARCH":                   {Entity: "Estimate"},
	"QBO.SALES.INVOICE_READ":                      {Entity: "Invoice"},
	"QBO.SALES.INVOICE_SEARCH":                    {Entity: "Invoice"},
	"QBO.SALES.PAYMENT_READ":                      {Entity: "Payment"},
	"QBO.SALES.RECEIPT_READ":                      {Entity: "SalesReceipt"},
	"QBO.SALES.REFUND_RECEIPT_READ":               {Entity: "RefundReceipt"},
	"QBO.SALES.SALES_ORDER_READ":                  {Entity: "SalesOrder"},
	"QBO.SALES.SALES_ORDER_SEARCH":                {Entity: "SalesOrder"},
	"QBO.SALES.HUB_READ":                          {Entity: "SalesHub"},
	"QBO.SALES.HUB_SEARCH":                        {Entity: "SalesHub"},
	"QBO.TAX.CODE_READ":                           {Entity: "TaxCode"},
	"QBO.TAX.CODE_SEARCH":                         {Entity: "TaxCode"},
	"QBO.TAX.TAX_AGENCY_READ":                     {Entity: "TaxAgency"},
	"QBO.TAX.TAX_RATE_READ":                       {Entity: "TaxRate"},
	"QBO.TAX.TPAR_READ":                           {Report: "TAXABLE_PAYMENTS"},
	"QBO.REPORTS.REPORT_READ":                     {Report: "ProfitAndLoss"},
	"QBO.REPORTS.CASH_FLOW_READ":                  {Report: "CashFlow"},
	"QBO.REPORTS.AGED_PAYABLES_READ":              {Report: "AgedPayables"},
	"QBO.REPORTS.AGED_RECEIVABLES_READ":           {Report: "AgedReceivables"},
	"QBO.REPORTS.GENERAL_LEDGER_READ":             {Report: "GeneralLedger"},
	"QBO.REPORTS.TRIAL_BALANCE_READ":              {Report: "TrialBalance"},
	"QBO.REPORTS.PROFIT_LOSS_READ":                {Report: "ProfitAndLoss"},
	"QBO.REPORTS.BALANCE_SHEET_READ":              {Report: "BalanceSheet"},
	"QBO.REPORTS.VENDOR_BALANCE_READ":             {Report: "VendorBalance"},
	"QBO.REPORTS.CUSTOMER_BALANCE_READ":           {Report: "CustomerBalance"},
	"QBO.REPORTS.CUSTOMER_SALES_READ":             {Report: "CustomerSales"},
	"QBO.REPORTS.VENDOR_EXPENSES_READ":            {Report: "VendorExpenses"},
	"QBO.REPORTS.PROFIT_LOSS_DETAIL_READ":         {Report: "ProfitAndLossDetail"},
	"QBO.REPORTS.INVENTORY_VALUATION_READ":        {Report: "InventoryValuationSummary"},
	"QBO.REPORTS.TRANSACTION_LIST_READ":           {Report: "TransactionList"},
	"QBO.REPORTS.JOURNAL_REPORT_READ":             {Report: "JournalReport"},
	"QBO.REPORTS.AGED_RECEIVABLE_DETAIL_READ":     {Report: "AgedReceivableDetail"},
	"QBO.REPORTS.AGED_PAYABLE_DETAIL_READ":        {Report: "AgedPayableDetail"},
	"QBO.REPORTS.CUSTOMER_INCOME_READ":            {Report: "CustomerIncome"},
	"QBO.REPORTS.CUSTOMER_BALANCE_DETAIL_READ":    {Report: "CustomerBalanceDetail"},
	"QBO.REPORTS.VENDOR_BALANCE_DETAIL_READ":      {Report: "VendorBalanceDetail"},
	"QBO.REPORTS.ITEM_SALES_READ":                 {Report: "ItemSales"},
	"QBO.REPORTS.DEPARTMENT_SALES_READ":           {Report: "DepartmentSales"},
	"QBO.REPORTS.CLASS_SALES_READ":                {Report: "ClassSales"},
	"QBO.REPORTS.INVENTORY_VALUATION_DETAIL_READ": {Report: "InventoryValuationDetail"},
	"QBO.REPORTS.ACCOUNT_LIST_READ":               {Report: "AccountList"},
	"QBO.REPORTS.CUSTOM_READ":                     {Entity: "Folio"},
	"QBO.REPORTS.MANAGEMENT_READ":                 {Entity: "ManagementFolio"},
}

func maybeV3Cmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	if e.ID == "QBO.ACCOUNTING.AUDIT_LOG_SEARCH" {
		return newAccountingAuditLogSearchCmd(flags)
	}
	if e.ID == "QBO.ACCOUNTING.INTEGRATION_TXN_READ" {
		return newAccountingIntegrationTxnReadCmd(flags)
	}

	if m := maybeV3MutateCmd(flags, e, command); m != nil {
		return m
	}
	if e.Mode == modeBlocked || e.Mode == modeExcluded {
		return nil
	}
	spec, ok := v3ByID[e.ID]
	if !ok {
		return nil
	}
	if spec.Report != "" {
		return newV3ReportCmd(flags, e, command, spec.Report)
	}
	return newV3QueryCmd(flags, e, command, spec.Entity)
}

func newV3QueryCmd(flags *rootFlags, e primitiveEntry, command, entity string) *cobra.Command {
	var id, query, active string
	var limit int
	use := verbOf(command)
	short := command + " (v3 query)"
	dryNote := "v3 query GET; not sent"
	if entity == "SalesOrder" {
		short = command + " (commercecontrol GetSalesOrders)"
		dryNote = "commercecontrol GetSalesOrders POST; not sent"
	}
	if entity == "RevenueRecognition" {
		short = command + " (sbseggraphqlorch accountingDeferredTransactionLineDetailsOp)"
		dryNote = "sbseggraphqlorch accountingDeferredTransactionLineDetailsOp POST; not sent"
	}
	if entity == "ListsPrefs" {
		short = command + " (v4/entities ListsPrefs)"
		dryNote = "v4/entities ListsPrefs POST; not sent"
	}
	if entity == "CustomersOverview" {
		short = command + " (v4 graphql invoiceSummary+getTransactions)"
		dryNote = "v4 graphql invoiceSummary+getTransactions POST; not sent"
	}
	if entity == "InventoryOverview" {
		short = command + " (commercecontrol GetAllListViewEntities)"
		dryNote = "commercecontrol GetAllListViewEntities POST; not sent"
	}
	if entity == "InventoryOverviewSearch" {
		short = command + " (commercecontrol SearchAllListViewEntities)"
		dryNote = "commercecontrol SearchAllListViewEntities POST; not sent"
	}
	if entity == "TPAR" {
		short = command + " (universalreportinsights TAXABLE_PAYMENTS/instances)"
		dryNote = "universalreportinsights TAXABLE_PAYMENTS/instances POST; not sent"
	}
	if entity == "ItemReceipt" {
		short = command + " (warehouse-management-svc GetItemReceipts)"
		dryNote = "warehouse-management-svc GetItemReceipts POST; not sent"
	}
	if entity == "Contract" {
		short = command + " (esign.platform modified-envelopes)"
		dryNote = "esign.platform GET /v1/modified-envelopes; not sent"
	}
	if entity == "CustomField" {
		short = command + " (customextensions CustomFieldsQueryCES)"
		dryNote = "customextensions CustomFieldsQueryCES POST; not sent"
	}
	if entity == "PrepaidSchedule" {
		short = command + " (cbo.api GetSchedules)"
		dryNote = "cbo.api GetSchedules POST; not sent"
	}
	if entity == "Workflow" {
		short = command + " (workflowautomation WASAllRules)"
		dryNote = "workflowautomation WASAllRules POST; not sent"
	}
	if entity == "BusinessFeed" {
		short = command + " (business-feed /v1/business-feed/items)"
		dryNote = "business-feed POST /v1/business-feed/items; not sent"
	}
	if entity == "Proposal" {
		short = command + " (crm-proposal-svc GET /v1/proposals)"
		dryNote = "crm-proposal-svc GET /v1/proposals; not sent"
	}
	if entity == "Review" {
		short = command + " (mccrmmessaging GET /v1/feedback-engagement)"
		dryNote = "mccrmmessaging GET /v1/feedback-engagement; not sent"
	}
	if entity == "Folio" {
		short = command + " (universalreportinsights GET /v1/folio)"
		dryNote = "universalreportinsights GET /v1/folio; not sent"
	}
	if entity == "SalesHub" {
		short = command + " (v4 graphql transactions)"
		dryNote = "v4 graphql transactions POST; not sent"
	}
	if entity == "Role" {
		short = command + " (roles.api accountRoles)"
		dryNote = "roles.api accountRoles POST; not sent"
	}
	if entity == "FormStyle" {
		short = command + " (txnsrendering /v2/customizations)"
		dryNote = "txnsrendering GET /v2/customizations; not sent"
	}
	if entity == "ManagementFolio" {
		short = command + " (universalreportinsights GET /v1/folio management)"
		dryNote = "universalreportinsights GET /v1/folio?locale=en-au; not sent"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if entity == "SalesOrder" && cmd.Flags().Changed("active") {
				return exitInput(fmt.Errorf("sales orders do not support --active; use --id or --query"))
			}
			if handled, err := dryRunGET(flags, cmd, command, e.ID, client.PlannedQueryURL(entity, id), dryNote); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayQuery(ctx, entity, id, query, limit, active)
			if err != nil {
				return feedErr(flags, err)
			}
			printQuery(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "v3 entity id")
	cmd.Flags().StringVar(&query, "query", "", "substring match on name or doc number")
	cmd.Flags().StringVar(&active, "active", "", "v3 Active filter: true, false, or all (include inactive)")
	cmd.Flags().IntVar(&limit, "limit", 20, "max rows")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

func newV3ReportCmd(flags *rootFlags, e primitiveEntry, command, def string) *cobra.Command {
	// Keep legacy flags parseable, but reject explicit options with no report
	// API contract instead of silently returning an unfiltered report.
	var report, dateRange, accountingMethod, columns, id, accountID string
	var limit int
	use := verbOf(command)
	short := command + " (v3 report)"
	dryNote := "v3 report GET; not sent"
	if def == "TAXABLE_PAYMENTS" {
		short = command + " (universalreportinsights TAXABLE_PAYMENTS/instances)"
		dryNote = "universalreportinsights TAXABLE_PAYMENTS/instances POST; not sent"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, flag := range []string{"id", "limit", "account-id"} {
				if cmd.Flags().Changed(flag) {
					return fmt.Errorf("--%s is not supported by this report command; omit it (reports are company-wide, not paginated entity lists)", flag)
				}
			}
			name := strings.TrimSpace(report)
			if name == "" {
				name = def
			}
			opts := client.ReportOptions{AccountingMethod: accountingMethod, SummarizeColumnBy: columns}
			params, err := opts.QueryParams(name)
			if err != nil {
				return err
			}
			start, end := splitDateRange(dateRange)
			plannedURL := client.PlannedReportURL(name)
			if name != "TAXABLE_PAYMENTS" {
				maps.Copy(params, client.ReportDateParams(name, start, end))
				if len(params) > 0 {
					plannedURL += "&" + params.Encode()
				}
			}
			if handled, err := dryRunGET(flags, cmd, command, e.ID, plannedURL, dryNote); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayReport(ctx, name, start, end, opts)
			if err != nil {
				return feedErr(flags, err)
			}
			printReport(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&report, "report", def, "v3 report name (ProfitAndLoss, BalanceSheet, CashFlow)")
	cmd.Flags().StringVar(&dateRange, "date-range", "", "start,end as YYYY-MM-DD,YYYY-MM-DD")
	cmd.Flags().StringVar(&accountingMethod, "accounting-method", "", "")
	cmd.Flags().StringVar(&columns, "columns", "", "")
	cmd.Flags().StringVar(&id, "id", "", "")
	cmd.Flags().IntVar(&limit, "limit", 20, "")
	if command == "feed rec get" {
		cmd.Flags().StringVar(&accountID, "account-id", client.DefaultAccountID, "")
	}
	for _, param := range reportParamDocs(e.ID, def) {
		if flag := cmd.Flags().Lookup(param.Name); flag != nil {
			flag.Usage = param.Help
		}
	}
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

func verbOf(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "read"
	}
	return parts[len(parts)-1]
}

func splitDateRange(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.IndexAny(s, ",/"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

func printQuery(stdout io.Writer, flags *rootFlags, res *client.QueryResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if res == nil || len(res.Items) == 0 {
		if res != nil && res.Note != "" {
			fmt.Fprintln(stdout, res.Note)
			return
		}
		fmt.Fprintln(stdout, "no rows")
		return
	}
	fmt.Fprintf(stdout, "%s %d row(s)\n", res.Entity, len(res.Items))
	for _, it := range res.Items {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%v\n", it.ID, it.Name, it.DocNumber, it.Amount)
	}
}

func printReport(stdout io.Writer, flags *rootFlags, res *client.ReportResult) {
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
	fmt.Fprintf(stdout, "report %s columns=%d\n", res.Report, len(res.Columns))
	for _, c := range res.Columns {
		fmt.Fprintf(stdout, "  %s\n", c)
	}
	if len(res.Rows) > 0 {
		var rows any
		if json.Unmarshal(res.Rows, &rows) == nil {
			body, _ := json.MarshalIndent(rows, "", "  ")
			fmt.Fprintln(stdout, string(body))
		}
	}
}
