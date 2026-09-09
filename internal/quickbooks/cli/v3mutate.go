package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type v3MutateSpec struct {
	Entity string
	Op     string
}

var v3MutateByID = map[string]v3MutateSpec{
	"QBO.ACCOUNTING.BUDGET_CREATE":      {Entity: "Budget", Op: "create"},
	"QBO.ACCOUNTING.BUDGET_EDIT":        {Entity: "Budget", Op: "update"},
	"QBO.ACCOUNTING.BUDGET_DELETE":      {Entity: "Budget", Op: "delete"},
	"QBO.ACCOUNTING.CLASS_CREATE":       {Entity: "Class", Op: "create"},
	"QBO.ACCOUNTING.CLASS_EDIT":         {Entity: "Class", Op: "update"},
	"QBO.ACCOUNTING.DEPARTMENT_CREATE":  {Entity: "Department", Op: "create"},
	"QBO.ACCOUNTING.DEPARTMENT_EDIT":    {Entity: "Department", Op: "update"},
	"QBO.ACCOUNTING.DEPARTMENT_DELETE":  {Entity: "Department", Op: "deactivate"},
	"QBO.ACCOUNTING.COA_CREATE":         {Entity: "Account", Op: "create"},
	"QBO.ACCOUNTING.COA_DEACTIVATE":     {Entity: "Account", Op: "deactivate"},
	"QBO.ACCOUNTING.FIXED_ASSET_CREATE": {Entity: "Account", Op: "create"},
	"QBO.ACCOUNTING.FIXED_ASSET_DELETE": {Entity: "Account", Op: "deactivate"},
	"QBO.ACCOUNTING.FIXED_ASSET_EDIT":   {Entity: "Account", Op: "update"},

	"QBO.ACCOUNTING.DEPOSIT_CREATE":      {Entity: "Deposit", Op: "create"},
	"QBO.ACCOUNTING.DEPOSIT_DELETE":      {Entity: "Deposit", Op: "delete"},
	"QBO.ACCOUNTING.DEPOSIT_EDIT":        {Entity: "Deposit", Op: "update"},
	"QBO.ACCOUNTING.JOURNAL_CREATE":      {Entity: "JournalEntry", Op: "create"},
	"QBO.ACCOUNTING.OPENING_BALANCE_SET": {Entity: "JournalEntry", Op: "update"},

	"QBO.ACCOUNTING.JOURNAL_DELETE":   {Entity: "JournalEntry", Op: "delete"},
	"QBO.ACCOUNTING.JOURNAL_EDIT":     {Entity: "JournalEntry", Op: "update"},
	"QBO.ACCOUNTING.LOCATION_CREATE":  {Entity: "Department", Op: "create"},
	"QBO.ACCOUNTING.PROJECT_CREATE":   {Entity: "Customer", Op: "create"},
	"QBO.ACCOUNTING.PROJECT_EDIT":     {Entity: "Customer", Op: "update"},
	"QBO.ACCOUNTING.TRANSFER_CREATE":  {Entity: "Transfer", Op: "create"},
	"QBO.ACCOUNTING.RECURRING_CREATE": {Entity: "RecurringTransaction", Op: "create"},
	"QBO.ACCOUNTING.RECURRING_DELETE": {Entity: "RecurringTransaction", Op: "delete"},
	"QBO.ACCOUNTING.RECURRING_EDIT":   {Entity: "RecurringTransaction", Op: "update"},

	"QBO.ACCOUNTING.TRANSFER_DELETE":      {Entity: "Transfer", Op: "delete"},
	"QBO.ACCOUNTING.TRANSFER_EDIT":        {Entity: "Transfer", Op: "update"},
	"QBO.ACCOUNTING.TXN_VOID":             {Entity: "Invoice", Op: "void"},
	"QBO.COMPANY.ATTACHABLE_DELETE":       {Entity: "Attachable", Op: "delete"},
	"QBO.COMPANY.ATTACHABLE_EDIT":         {Entity: "Attachable", Op: "update"},
	"QBO.COMPANY.CURRENCY_CREATE":         {Entity: "CompanyCurrency", Op: "create"},
	"QBO.COMPANY.CURRENCY_DELETE":         {Entity: "CompanyCurrency", Op: "delete"},
	"QBO.COMPANY.COMPANY_INFO_EDIT":       {Entity: "CompanyInfo", Op: "update"},
	"QBO.COMPANY.TAG_CREATE":              {Entity: "Class", Op: "create"},
	"QBO.COMPANY.TAG_DELETE":              {Entity: "Class", Op: "deactivate"},
	"QBO.COMPANY.TAG_EDIT":                {Entity: "Class", Op: "update"},
	"QBO.CUSTOMERS.CUSTOMER_CREATE":       {Entity: "Customer", Op: "create"},
	"QBO.CUSTOMERS.CUSTOMER_DELETE":       {Entity: "Customer", Op: "deactivate"},
	"QBO.CUSTOMERS.CUSTOMER_EDIT":         {Entity: "Customer", Op: "update"},
	"QBO.EXPENSES.BANK_FEE_CREATE":        {Entity: "Purchase", Op: "create"},
	"QBO.EXPENSES.BILL_CREATE":            {Entity: "Bill", Op: "create"},
	"QBO.EXPENSES.BILL_DELETE":            {Entity: "Bill", Op: "delete"},
	"QBO.EXPENSES.BILL_EDIT":              {Entity: "Bill", Op: "update"},
	"QBO.EXPENSES.BILL_PAY":               {Entity: "BillPayment", Op: "create"},
	"QBO.EXPENSES.BILL_PAYMENT_CREATE":    {Entity: "BillPayment", Op: "create"},
	"QBO.EXPENSES.BILL_PAYMENT_DELETE":    {Entity: "BillPayment", Op: "delete"},
	"QBO.EXPENSES.BILL_PAYMENT_EDIT":      {Entity: "BillPayment", Op: "update"},
	"QBO.EXPENSES.CHEQUE_CREATE":          {Entity: "Purchase", Op: "create"},
	"QBO.EXPENSES.CHEQUE_DELETE":          {Entity: "Purchase", Op: "delete"},
	"QBO.EXPENSES.CHEQUE_EDIT":            {Entity: "Purchase", Op: "update"},
	"QBO.EXPENSES.EXPENSE_CREATE":         {Entity: "Purchase", Op: "create"},
	"QBO.EXPENSES.EXPENSE_DELETE":         {Entity: "Purchase", Op: "delete"},
	"QBO.EXPENSES.EXPENSE_EDIT":           {Entity: "Purchase", Op: "update"},
	"QBO.EXPENSES.MILEAGE_CREATE":         {Entity: "TimeActivity", Op: "create"},
	"QBO.EXPENSES.MILEAGE_DELETE":         {Entity: "TimeActivity", Op: "delete"},
	"QBO.EXPENSES.MILEAGE_EDIT":           {Entity: "TimeActivity", Op: "update"},
	"QBO.EXPENSES.TIME_ACTIVITY_CREATE":   {Entity: "TimeActivity", Op: "create"},
	"QBO.EXPENSES.TIME_ACTIVITY_EDIT":     {Entity: "TimeActivity", Op: "update"},
	"QBO.EXPENSES.TIME_ACTIVITY_DELETE":   {Entity: "TimeActivity", Op: "delete"},
	"QBO.EXPENSES.SUPPLIER_CREATE":        {Entity: "Vendor", Op: "create"},
	"QBO.EXPENSES.SUPPLIER_DELETE":        {Entity: "Vendor", Op: "deactivate"},
	"QBO.EXPENSES.SUPPLIER_EDIT":          {Entity: "Vendor", Op: "update"},
	"QBO.EXPENSES.VENDOR_CREDIT_CREATE":   {Entity: "VendorCredit", Op: "create"},
	"QBO.EXPENSES.VENDOR_CREDIT_DELETE":   {Entity: "VendorCredit", Op: "delete"},
	"QBO.EXPENSES.VENDOR_CREDIT_EDIT":     {Entity: "VendorCredit", Op: "update"},
	"QBO.INVENTORY.ITEM_CREATE":           {Entity: "Item", Op: "create"},
	"QBO.INVENTORY.ITEM_DELETE":           {Entity: "Item", Op: "deactivate"},
	"QBO.INVENTORY.ITEM_EDIT":             {Entity: "Item", Op: "update"},
	"QBO.INVENTORY.PURCHASE_ORDER_CREATE": {Entity: "PurchaseOrder", Op: "create"},
	"QBO.INVENTORY.PURCHASE_ORDER_DELETE": {Entity: "PurchaseOrder", Op: "delete"},
	"QBO.INVENTORY.PURCHASE_ORDER_EDIT":   {Entity: "PurchaseOrder", Op: "update"},
	"QBO.INVENTORY.ADJUST_CREATE":         {Entity: "InventoryAdjustment", Op: "create"},
	"QBO.INVENTORY.ITEM_RECEIPT_CREATE":   {Entity: "ItemReceipt", Op: "create"},
	"QBO.INVENTORY.ITEM_RECEIPT_EDIT":     {Entity: "ItemReceipt", Op: "update"},
	"QBO.PAYROLL.EMPLOYEE_TERMINATE":      {Entity: "Employee", Op: "deactivate"},
	"QBO.PAYROLL.EMPLOYEE_CREATE":         {Entity: "Employee", Op: "create"},
	"QBO.PAYROLL.EMPLOYEE_EDIT":           {Entity: "Employee", Op: "update"},
	"QBO.COMPANY.TERM_CREATE":             {Entity: "Term", Op: "create"},
	"QBO.COMPANY.TERM_EDIT":               {Entity: "Term", Op: "update"},
	"QBO.COMPANY.TERM_DELETE":             {Entity: "Term", Op: "deactivate"},
	"QBO.COMPANY.PAYMENT_METHOD_CREATE":   {Entity: "PaymentMethod", Op: "create"},
	"QBO.COMPANY.PAYMENT_METHOD_EDIT":     {Entity: "PaymentMethod", Op: "update"},
	"QBO.COMPANY.PAYMENT_METHOD_DELETE":   {Entity: "PaymentMethod", Op: "deactivate"},
	"QBO.PAYROLL.TIMESHEET_CREATE":        {Entity: "TimeActivity", Op: "create"},
	"QBO.PAYROLL.TIMESHEET_EDIT":          {Entity: "TimeActivity", Op: "update"},
	"QBO.SALES.CREDIT_CARD_CREDIT_CREATE": {Entity: "Purchase", Op: "create"},
	"QBO.SALES.CREDIT_MEMO_CREATE":        {Entity: "CreditMemo", Op: "create"},
	"QBO.SALES.CREDIT_MEMO_DELETE":        {Entity: "CreditMemo", Op: "delete"},
	"QBO.SALES.CREDIT_MEMO_EDIT":          {Entity: "CreditMemo", Op: "update"},
	"QBO.SALES.ESTIMATE_CREATE":           {Entity: "Estimate", Op: "create"},
	"QBO.SALES.ESTIMATE_DELETE":           {Entity: "Estimate", Op: "delete"},
	"QBO.SALES.ESTIMATE_EDIT":             {Entity: "Estimate", Op: "update"},
	"QBO.SALES.ESTIMATE_COPY":             {Entity: "Estimate", Op: "copy"},
	"QBO.SALES.INVOICE_CREATE":            {Entity: "Invoice", Op: "create"},
	"QBO.SALES.INVOICE_DELETE":            {Entity: "Invoice", Op: "delete"},
	"QBO.SALES.INVOICE_EDIT":              {Entity: "Invoice", Op: "update"},
	"QBO.SALES.INVOICE_COPY":              {Entity: "Invoice", Op: "copy"},
	"QBO.SALES.DELAYED_CHARGE_CREATE":     {Entity: "DelayedCharge", Op: "create"},
	"QBO.SALES.DELAYED_CREDIT_CREATE":     {Entity: "DelayedCredit", Op: "create"},
	"QBO.SALES.INVOICE_DISCOUNT":          {Entity: "Invoice", Op: "discount"},
	"QBO.SALES.INVOICE_SEND":              {Entity: "Invoice", Op: "send"},
	"QBO.SALES.INVOICE_WRITE_OFF":         {Entity: "Invoice", Op: "writeoff"},
	"QBO.SALES.TIME_INVOICE_CREATE":       {Entity: "Invoice", Op: "create"},
	"QBO.COMPANY.SETTINGS_EDIT":           {Entity: "Preferences", Op: "update"},
	"QBO.SALES.PAYMENT_CREATE":            {Entity: "Payment", Op: "create"},
	"QBO.SALES.PAYMENT_DELETE":            {Entity: "Payment", Op: "delete"},
	"QBO.SALES.PAYMENT_EDIT":              {Entity: "Payment", Op: "update"},
	"QBO.SALES.RECEIPT_CREATE":            {Entity: "SalesReceipt", Op: "create"},
	"QBO.SALES.RECEIPT_DELETE":            {Entity: "SalesReceipt", Op: "delete"},
	"QBO.SALES.RECEIPT_EDIT":              {Entity: "SalesReceipt", Op: "update"},
	"QBO.SALES.REFUND_RECEIPT_CREATE":     {Entity: "RefundReceipt", Op: "create"},
	"QBO.SALES.REFUND_RECEIPT_DELETE":     {Entity: "RefundReceipt", Op: "delete"},
	"QBO.SALES.REFUND_RECEIPT_EDIT":       {Entity: "RefundReceipt", Op: "update"},
	"QBO.SALES.SALES_ORDER_CREATE":        {Entity: "Invoice", Op: "create"},
	"QBO.SALES.SALES_ORDER_DELETE":        {Entity: "Invoice", Op: "delete"},
	"QBO.SALES.SALES_ORDER_EDIT":          {Entity: "Invoice", Op: "update"},
	"QBO.TAX.TAX_AGENCY_CREATE":           {Entity: "TaxAgency", Op: "create"},
}

func maybeV3MutateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	if e.Mode == modeBlocked || e.Mode == modeExcluded {
		return nil
	}
	spec, ok := v3MutateByID[e.ID]
	if !ok {
		return nil
	}
	return newV3MutateCmd(flags, e, command, spec)
}

func newV3MutateCmd(flags *rootFlags, e primitiveEntry, command string, spec v3MutateSpec) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v3 " + spec.Op + ")",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if err := client.CheckLineItemsJSON(fm); err != nil {
				return feedErr(flags, err)
			}
			if spec.Op == "create" {
				for _, p := range paramsFor(e.ID) {
					if !p.Required || (p.Name != "line-items" && p.Name != "lines") {
						continue
					}
					if strings.TrimSpace(fm[p.Name]) == "" {
						return exitInput(fmt.Errorf("create requires --%s (JSON array of v3 line objects); omitted lines would record a $1 default", p.Name))
					}
				}
			}
			if flags.dryRun {
				url := client.PlannedMutateURL(spec.Entity, spec.Op)
				note := "v3 POST; not sent"
				if spec.Entity == "Budget" && spec.Op == "create" {
					url = client.PlannedBudgetCreateURL()
					note = "budgeting-ui graphql businessPlanningCreateBudget; not sent"
				}
				return writePlan(cmd, flags, planEnvelope{
					Command: command,
					ID:      e.ID,
					Mode:    modeWired,
					Method:  "POST",
					URL:     url,
					Flags:   fm,
					Note:    note,
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayMutate(ctx, spec.Entity, spec.Op, fm["id"], fm)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.ID)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "id", "target QBO id")
	ensureFlag(cmd, "amount", "AUD amount")
	ensureFlag(cmd, "item", "item id for sales lines")
	ensureFlag(cmd, "account", "account id")
	ensureFlag(cmd, "start-date", "budget start date dd/MM/yyyy (alternative to --fiscal-year)")
	ensureFlag(cmd, "from-account", "debit/from account id")
	ensureFlag(cmd, "to-account", "credit/to account id")
	ensureFlag(cmd, "name", "display or account name")
	ensureFlag(cmd, "type", "account or item type")
	ensureFlag(cmd, "customer", "customer id")
	ensureFlag(cmd, "supplier", "supplier/vendor id")
	ensureFlag(cmd, "bill-id", "bill id to pay (BillPayment)")
	ensureFlag(cmd, "category-account", "expense/category account id (distinct from payment)")
	ensureFlag(cmd, "payment-type", "Cash, Check, or CreditCard")
	ensureFlag(cmd, "pay-type", "Check or CreditCard (BillPayment)")
	ensureFlag(cmd, "email", "email address to send to")
	ensureFlag(cmd, "to", "override recipient email (invoice send)")
	ensureFlag(cmd, "employee", "employee id for timesheets and mileage")
	ensureFlag(cmd, "hours", "hours for TimeActivity")
	ensureFlag(cmd, "qty", "quantity difference for inventory adjust")
	ensureFlag(cmd, "subtype", "account subtype (e.g. SuppliesMaterialsCogs)")
	ensureFlag(cmd, "income-account", "income account id for items")
	ensureFlag(cmd, "expense-account", "COGS/expense account id for inventory items")
	ensureFlag(cmd, "asset-account", "inventory asset account id")
	ensureFlag(cmd, "discount", "invoice discount amount")
	ensureFlag(cmd, "discount-percent", "invoice discount percent")
	ensureFlag(cmd, "invoice-id", "invoice id to link (credit memo / write-off / payment)")
	ensureFlag(cmd, "time-activity", "TimeActivity id to bill on an invoice")
	ensureFlag(cmd, "address", "billing street address")
	ensureFlag(cmd, "charge-date", "delayed charge date")
	ensureFlag(cmd, "credit-date", "delayed credit date")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

func ensureFlag(cmd *cobra.Command, name, help string) {
	if cmd.Flags().Lookup(name) == nil {
		cmd.Flags().String(name, "", help)
	}
}

func collectFlags(cmd *cobra.Command) map[string]string {
	out := map[string]string{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f == nil || f.Name == "help" {
			return
		}
		out[f.Name] = f.Value.String()
	})
	return out
}
