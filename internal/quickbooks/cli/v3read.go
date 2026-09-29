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
	"QBO.ACCOUNTING.BUDGET_READ":         {Entity: "Budget"},
	"QBO.ACCOUNTING.BUDGET_SEARCH":       {Entity: "Budget"},
	"QBO.ACCOUNTING.BUDGET_PLANNING_GET": {Entity: "BudgetPlanning"},
	"QBO.ACCOUNTING.CLASS_READ":          {Entity: "Class"},
	"QBO.ACCOUNTING.CLASS_SEARCH":        {Entity: "Class"},
	"QBO.ACCOUNTING.COA_READ":            {Entity: "Account"},
	"QBO.ACCOUNTING.COA_SEARCH":          {Entity: "Account"},
	"QBO.ACCOUNTING.DEPARTMENT_READ":     {Entity: "Department"},
	"QBO.ACCOUNTING.DEPARTMENT_SEARCH":   {Entity: "Department"},
	"QBO.ACCOUNTING.DEPOSIT_READ":        {Entity: "Deposit"},
	"QBO.ACCOUNTING.FIXED_ASSET_READ":    {Entity: "FixedAsset"},
	"QBO.ACCOUNTING.FIXED_ASSET_SEARCH":  {Entity: "FixedAsset"},
	"QBO.ACCOUNTING.JOURNAL_READ":        {Entity: "JournalEntry"},
	"QBO.ACCOUNTING.LOCATION_READ":       {Entity: "Department"},
	"QBO.ACCOUNTING.RECURRING_READ":      {Entity: "RecurringTransaction"},
	"QBO.ACCOUNTING.RECURRING_SEARCH":    {Entity: "RecurringTransaction"},
	"QBO.ACCOUNTING.TRANSFER_READ":       {Entity: "Transfer"},

	"QBO.ACCOUNTING.PREPAID_READ":                 {Entity: "PrepaidSchedule"},
	"QBO.ACCOUNTING.REVENUE_RECOGNITION_READ":     {Entity: "RevenueRecognition"},
	"QBO.ACCOUNTING.RECONCILE_READ":               {Entity: "Reconciliation"},
	"QBO.ACCOUNTING.TAXCONFIG_MIRROR_GET":         {Entity: "TaxConfigGroup"},
	"QBO.ADVANCED.CUSTOM_ROLES_READ":              {Entity: "Role"},
	"QBO.ADVANCED.WORKFLOWS_READ":                 {Entity: "Workflow"},
	"QBO.INTEGRATIONS.WORKFLOW_AUTOMATION_GET":    {Entity: "Workflow"},
	"QBO.COMPANY.ATTACHABLE_READ":                 {Entity: "Attachable"},
	"QBO.COMPANY.ATTACHABLE_SEARCH":               {Entity: "Attachable"},
	"QBO.COMPANY.ATTACHMENT_READ":                 {Entity: "Attachable"},
	"QBO.COMPANY.BUSINESS_FEED_READ":              {Entity: "BusinessFeed"},
	"QBO.COMPANY.CURRENCY_READ":                   {Entity: "CompanyCurrency"},
	"QBO.COMPANY.EXCHANGE_RATE_READ":              {Entity: "ExchangeRate"},
	"QBO.COMPANY.EXCHANGE_RATE_SEARCH":            {Entity: "ExchangeRate"},
	"QBO.COMPANY.CUSTOM_FIELD_READ":               {Entity: "CustomField"},
	"QBO.COMPANY.DIMENSIONS_GRAPHQL_GET":          {Entity: "DimensionDefinition"},
	"QBO.ADVANCED.TASKS_READ":                     {Entity: "Task"},
	"QBO.INTEGRATIONS.IDX_GET":                    {Entity: "IDX"},
	"QBO.SALES.COMMERCE_CONTROL_GET":              {Entity: "IMSPref"},
	"QBO.COMPANY.FORM_STYLE_READ":                 {Entity: "FormStyle"},
	"QBO.COMPANY.LIST_READ":                       {Entity: "ListsPrefs"},
	"QBO.COMPANY.COMPANY_INFO_READ":               {Entity: "CompanyInfo"},
	"QBO.COMPANY.TERM_READ":                       {Entity: "Term"},
	"QBO.COMPANY.PAYMENT_METHOD_READ":             {Entity: "PaymentMethod"},
	"QBO.COMPANY.PREFERENCES_READ":                {Entity: "Preferences"},
	"QBO.COMPANY.ROLE_READ":                       {Entity: "Role"},
	"QBO.COMPANY.MARKETING_READ":                  {Entity: "MarketingOffer"},
	"QBO.COMPANY.TAG_READ":                        {Entity: "Tag"},
	"QBO.COMPANY.SETTINGS_READ":                   {Entity: "Preferences"},
	"QBO.COMPANY.USER_READ":                       {Entity: "IdentityUser"},
	"QBO.COMPANY.IDENTITY_GET":                    {Entity: "IdentityAccount"},
	"QBO.COMPANY.SETTINGS_FACADE_GET":             {Entity: "SettingsFacade"},
	"QBO.COMPANY.CUSTOM_EXTENSIONS_LIST":          {Entity: "CustomExtension"},
	"QBO.COMPANY.EXPERIMENT_ASSIGNMENT_GET":       {Entity: "ExperimentAssignment"},
	"QBO.TAX.EXPERIMENT_ASSIGNMENT_GET":           {Entity: "ExperimentAssignment"},
	"QBO.EXPENSES.PAYMENT_ACCOUNT_GET":            {Entity: "PaymentAccount"},
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
	"QBO.EXPENSES.SUPPLIER_READ":                  {Entity: "Vendor"},
	"QBO.EXPENSES.SUPPLIER_SEARCH":                {Entity: "Vendor"},
	"QBO.EXPENSES.TIME_ACTIVITY_READ":             {Entity: "TimeActivity"},
	"QBO.EXPENSES.TIME_ACTIVITY_SEARCH":           {Entity: "TimeActivity"},
	"QBO.EXPENSES.VENDOR_CREDIT_READ":             {Entity: "VendorCredit"},
	"QBO.EXPENSES.PURCHASE_SEARCH":                {Entity: "Purchase"},
	"QBO.EXPENSES.OVERVIEW_READ":                  {Entity: "ExpensesOverview"},
	"QBO.EXPENSES.BILL_PAY_ONBOARDING_GET":        {Entity: "BillPayOnboarding"},
	"QBO.EXPENSES.STAGETXN_GET":                   {Entity: "StageTransaction"},
	"QBO.EXPENSES.B2B_BILLPAY_GET":                {Entity: "B2BBillpay"},
	"QBO.EXPENSES.B2B_NETWORK_GET":                {Entity: "B2BNetwork"},
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
	"QBO.INVENTORY.WAREHOUSE_SERVICE_GET":         {Entity: "InventoryLocation"},
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
	"QBO.SALES.OVERVIEW_READ":                     {Entity: "SalesOverview"},
	"QBO.SALES.SALESTXN_RISK_GET":                 {Entity: "SalesTxnRisk"},
	"QBO.SALES.QBONLINE_GRAPHQL_GET":              {Entity: "QBOnlineGraphql"},
	"QBO.TAX.CODE_READ":                           {Entity: "TaxCode"},
	"QBO.TAX.CODE_SEARCH":                         {Entity: "TaxCode"},
	"QBO.TAX.TAX_AGENCY_READ":                     {Entity: "TaxAgency"},
	"QBO.TAX.TAX_RATE_READ":                       {Entity: "TaxRate"},
	"QBO.TAX.BAS_READ":                            {Entity: "TaxReturn"},
	"QBO.TAX.IAS_READ":                            {Entity: "TaxReturn"},
	"QBO.TAX.INDIRECT_REPORTS_GET":                {Entity: "TaxLiability"},
	"QBO.TAX.NEXUS_GET":                           {Entity: "TaxJurisdiction"},
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
	"QBO.REPORTS.FORECAST_READ":                   {Entity: "BusinessForecast"},
	"QBO.REPORTS.PERFORMANCE_READ":                {Entity: "PerformanceMetric"},
}

func maybeV3Cmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	if e.ID == "QBO.ACCOUNTING.AUDIT_LOG_SEARCH" {
		return newAccountingAuditLogSearchCmd(flags)
	}
	if e.ID == "QBO.ACCOUNTING.INTEGRATION_TXN_READ" {
		return newAccountingIntegrationTxnReadCmd(flags)
	}
	if e.ID == "QBO.COMPANY.CUSTOM_FIELD_CREATE" {
		return newCustomFieldMutateCmd(flags, e, command, "create")
	}
	if e.ID == "QBO.COMPANY.CUSTOM_FIELD_EDIT" {
		return newCustomFieldMutateCmd(flags, e, command, "update")
	}
	if e.ID == "QBO.COMPANY.CUSTOM_FIELD_DELETE" {
		return newCustomFieldDeleteCmd(flags, e, command)
	}
	switch e.ID {
	case "QBO.ACCOUNTING.PROJECT_CREATE", "QBO.ACCOUNTING.PROJECT_EDIT",
		"QBO.ACCOUNTING.PROJECT_READ", "QBO.ACCOUNTING.PROJECT_SEARCH":
		return newProjectCmd(flags, e, command)
	case "QBO.EXPENSES.MILEAGE_READ", "QBO.EXPENSES.MILEAGE_SEARCH":
		return newMileageCmd(flags, e, command)
	case "QBO.EXPENSES.MILEAGE_CREATE":
		return newTripMutateCmd(flags, e, command, "create")
	case "QBO.EXPENSES.MILEAGE_EDIT":
		return newTripMutateCmd(flags, e, command, "update")
	case "QBO.EXPENSES.MILEAGE_DELETE":
		return newTripMutateCmd(flags, e, command, "delete")
	case "QBO.ACCOUNTING.RECONCILE_CREATE":
		return newReconcileCreateCmd(flags, e, command)
	case "QBO.ACCOUNTING.FIXED_ASSET_CREATE":
		return newFixedAssetCmd(flags, e, command, "create")
	case "QBO.ACCOUNTING.FIXED_ASSET_EDIT":
		return newFixedAssetCmd(flags, e, command, "update")
	case "QBO.ACCOUNTING.FIXED_ASSET_DELETE":
		return newFixedAssetCmd(flags, e, command, "delete")
	case "QBO.COMPANY.SETTINGS_EDIT":
		return newSettingsUpdateCmd(flags, e, command)
	case "QBO.COMPANY.CURRENCY_EDIT":
		return newCurrencyRateCmd(flags, e, command)
	case "QBO.COMPANY.CURRENCY_DELETE":
		return newCurrencyDeleteCmd(flags, e, command)
	case "QBO.ADVANCED.RECLASSIFY_RUN":
		return newReclassifyCmd(flags, e, command)
	case "QBO.COMPANY.FORM_STYLE_EDIT":
		return newFormStyleUpdateCmd(flags, e, command)
	case "QBO.SALES.INVOICE_FORM_DELETE":
		return newFormStyleDeleteCmd(flags, e, command)
	case "QBO.ADVANCED.BACKUP_CREATE":
		return newBackupCreateCmd(flags, e, command)
	case "QBO.COMPANY.CUSTOMERS_IMPORT":
		return newCustomersImportCmd(flags, e, command)
	case "QBO.COMPANY.SUPPLIERS_IMPORT":
		return newSuppliersImportCmd(flags, e, command)
	case "QBO.COMPANY.BILLS_IMPORT", "QBO.EXPENSES.BILL_IMPORT":
		return newBillsImportCmd(flags, e, command)
	case "QBO.SALES.INVOICE_LIST_CREATE":
		return newInvoicesImportCmd(flags, e, command)
	case "QBO.COMPANY.COA_IMPORT":
		return newCoaImportCmd(flags, e, command)
	case "QBO.COMPANY.ITEMS_IMPORT":
		return newItemsImportCmd(flags, e, command)
	case "QBO.EXPENSES.RECEIPT_UPLOAD":
		return newReceiptUploadCmd(flags, e, command)
	case "QBO.EXPENSES.RECEIPT_EXPENSE_CREATE":
		return newReceiptExpenseCreateCmd(flags, e, command)
	case "QBO.SALES.STATEMENT_CREATE":
		return newStatementCreateCmd(flags, e, command)
	case "QBO.EXPENSES.EXPENSE_SPLIT":
		return newExpenseSplitCmd(flags, e, command)
	case "QBO.EXPENSES.CHEQUE_REPRINT":
		return newChequeExportCmd(flags, e, command)
	case "QBO.SALES.TIME_INVOICE_CREATE":
		return newTimeInvoiceCmd(flags, e, command)
	case "QBO.SALES.INVOICE_REMIND":
		return newInvoiceRemindCmd(flags, e, command)
	case "QBO.SALES.CREDIT_CARD_CREDIT_CREATE":
		return newCreditCardCreditCmd(flags, e, command)
	case "QBO.SALES.INVOICE_PROGRESS":
		return newInvoiceProgressCmd(flags, e, command)
	case "QBO.SALES.DELAYED_CHARGE_CREATE":
		return newDelayedChargeCmd(flags, e, command, "charge")
	case "QBO.SALES.DELAYED_CREDIT_CREATE":
		return newDelayedChargeCmd(flags, e, command, "credit")
	case "QBO.CUSTOMERS.CUSTOMER_MERGE":
		return newMergeCmd(flags, e, command, "Customer")
	case "QBO.EXPENSES.SUPPLIER_MERGE":
		return newMergeCmd(flags, e, command, "Vendor")
	case "QBO.EXPENSES.CHEQUE_BOUNCE":
		return newChequeBounceCmd(flags, e, command)
	case "QBO.CUSTOMERS.CUSTOMER_CREDIT_TRANSFER":
		return newCreditTransferCmd(flags, e, command)
	case "QBO.COMPANY.CURRENCY_REVALUE":
		return newCurrencyRevalueCmd(flags, e, command)
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
	if entity == "Tag" {
		short = command + " (tags.api GET tags)"
		dryNote = "tags.api GET tags; not sent"
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
	if entity == "MarketingOffer" {
		short = command + " (personalization.api ipd placement)"
		dryNote = "personalization.api POST /v1/experience/ipd/placement/<id>; not sent"
	}
	if entity == "FormStyle" {
		short = command + " (txnsrendering /v2/customizations)"
		dryNote = "txnsrendering GET /v2/customizations; not sent"
	}
	if entity == "ManagementFolio" {
		short = command + " (universalreportinsights GET /v1/folio management)"
		dryNote = "universalreportinsights GET /v1/folio?locale=en-au; not sent"
	}
	if entity == "IDX" {
		short = command + " (v4 graphql Connections)"
		dryNote = "v4 graphql Connections POST; not sent"
	}
	if entity == "IMSPref" {
		short = command + " (commercecontrol GetIMSPref)"
		dryNote = "commercecontrol GetIMSPref POST; not sent"
	}
	if entity == "DimensionDefinition" {
		short = command + " (v4 graphql GetCustomDimensionDefinitions)"
		dryNote = "v4 graphql GetCustomDimensionDefinitions POST; not sent"
	}
	if entity == "Task" {
		short = command + " (v4 graphql TaskManagementTasks)"
		dryNote = "v4 graphql TaskManagementTasks POST; not sent"
	}
	if entity == "TaxReturn" {
		short = command + " (v4 graphql indirect-tax taxReturns)"
		dryNote = "v4 graphql node__indirect_tax_ui_qbo POST; not sent"
	}
	if entity == "BusinessForecast" {
		short = command + " (planningforecasting getAllBusinessForecasts)"
		dryNote = "planningforecasting getAllBusinessForecasts POST; not sent"
	}
	if entity == "PerformanceMetric" {
		short = command + " (universalreportinsights /v1/metrics)"
		dryNote = "universalreportinsights POST /v1/metrics/<Metric>; not sent"
	}
	if entity == "FixedAsset" {
		short = command + " (assetservice financeAssets)"
		dryNote = "assetservice financeAssets POST; not sent"
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
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows (0 = all)")
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
	cmd.Flags().IntVar(&limit, "limit", 0, "(0 = all)")
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
