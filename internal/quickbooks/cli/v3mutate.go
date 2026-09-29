package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	"QBO.ACCOUNTING.BUDGET_CREATE":       {Entity: "Budget", Op: "create"},
	"QBO.ACCOUNTING.BUDGET_EDIT":         {Entity: "Budget", Op: "update"},
	"QBO.ACCOUNTING.BUDGET_DELETE":       {Entity: "Budget", Op: "delete"},
	"QBO.ACCOUNTING.CLASS_CREATE":        {Entity: "Class", Op: "create"},
	"QBO.ACCOUNTING.CLASS_EDIT":          {Entity: "Class", Op: "update"},
	"QBO.ACCOUNTING.DEPARTMENT_CREATE":   {Entity: "Department", Op: "create"},
	"QBO.ACCOUNTING.DEPARTMENT_EDIT":     {Entity: "Department", Op: "update"},
	"QBO.ACCOUNTING.DEPARTMENT_DELETE":   {Entity: "Department", Op: "deactivate"},
	"QBO.ACCOUNTING.COA_CREATE":          {Entity: "Account", Op: "create"},
	"QBO.ACCOUNTING.COA_DEACTIVATE":      {Entity: "Account", Op: "deactivate"},
	"QBO.ACCOUNTING.DEPOSIT_CREATE":      {Entity: "Deposit", Op: "create"},
	"QBO.ACCOUNTING.DEPOSIT_DELETE":      {Entity: "Deposit", Op: "delete"},
	"QBO.ACCOUNTING.DEPOSIT_EDIT":        {Entity: "Deposit", Op: "update"},
	"QBO.ACCOUNTING.JOURNAL_CREATE":      {Entity: "JournalEntry", Op: "create"},
	"QBO.ACCOUNTING.OPENING_BALANCE_SET": {Entity: "JournalEntry", Op: "update"},

	"QBO.ACCOUNTING.JOURNAL_DELETE":   {Entity: "JournalEntry", Op: "delete"},
	"QBO.ACCOUNTING.JOURNAL_EDIT":     {Entity: "JournalEntry", Op: "update"},
	"QBO.ACCOUNTING.LOCATION_CREATE":  {Entity: "Department", Op: "create"},
	"QBO.ACCOUNTING.TRANSFER_CREATE":  {Entity: "Transfer", Op: "create"},
	"QBO.ACCOUNTING.RECURRING_CREATE": {Entity: "RecurringTransaction", Op: "create"},
	"QBO.ACCOUNTING.RECURRING_DELETE": {Entity: "RecurringTransaction", Op: "delete"},
	"QBO.ACCOUNTING.RECURRING_EDIT":   {Entity: "RecurringTransaction", Op: "update"},

	"QBO.ACCOUNTING.TRANSFER_DELETE":       {Entity: "Transfer", Op: "delete"},
	"QBO.ACCOUNTING.TRANSFER_EDIT":         {Entity: "Transfer", Op: "update"},
	"QBO.ACCOUNTING.TXN_VOID":              {Op: "void"},
	"QBO.COMPANY.ATTACHABLE_DELETE":        {Entity: "Attachable", Op: "delete"},
	"QBO.COMPANY.ATTACHABLE_EDIT":          {Entity: "Attachable", Op: "update"},
	"QBO.COMPANY.CURRENCY_CREATE":          {Entity: "CompanyCurrency", Op: "create"},
	"QBO.COMPANY.COMPANY_INFO_EDIT":        {Entity: "CompanyInfo", Op: "update"},
	"QBO.CUSTOMERS.CUSTOMER_CREATE":        {Entity: "Customer", Op: "create"},
	"QBO.CUSTOMERS.CUSTOMER_DELETE":        {Entity: "Customer", Op: "deactivate"},
	"QBO.CUSTOMERS.CUSTOMER_EDIT":          {Entity: "Customer", Op: "update"},
	"QBO.EXPENSES.BANK_FEE_CREATE":         {Entity: "Purchase", Op: "create"},
	"QBO.EXPENSES.BILL_CREATE":             {Entity: "Bill", Op: "create"},
	"QBO.EXPENSES.BILL_DELETE":             {Entity: "Bill", Op: "delete"},
	"QBO.EXPENSES.BILL_EDIT":               {Entity: "Bill", Op: "update"},
	"QBO.EXPENSES.BILL_PAY":                {Entity: "BillPayment", Op: "create"},
	"QBO.EXPENSES.BILL_PAYMENT_CREATE":     {Entity: "BillPayment", Op: "create"},
	"QBO.EXPENSES.BILL_PAYMENT_DELETE":     {Entity: "BillPayment", Op: "delete"},
	"QBO.EXPENSES.BILL_PAYMENT_EDIT":       {Entity: "BillPayment", Op: "update"},
	"QBO.EXPENSES.CHEQUE_CREATE":           {Entity: "Cheque", Op: "create"},
	"QBO.EXPENSES.CHEQUE_DELETE":           {Entity: "Cheque", Op: "delete"},
	"QBO.EXPENSES.CHEQUE_EDIT":             {Entity: "Cheque", Op: "update"},
	"QBO.EXPENSES.EXPENSE_CREATE":          {Entity: "Purchase", Op: "create"},
	"QBO.EXPENSES.GIFT_CERTIFICATE_CREATE": {Entity: "Purchase", Op: "create"},
	"QBO.EXPENSES.EXPENSE_DELETE":          {Entity: "Purchase", Op: "delete"},
	"QBO.EXPENSES.EXPENSE_EDIT":            {Entity: "Purchase", Op: "update"},
	"QBO.EXPENSES.TIME_ACTIVITY_CREATE":    {Entity: "TimeActivity", Op: "create"},
	"QBO.EXPENSES.TIME_ACTIVITY_EDIT":      {Entity: "TimeActivity", Op: "update"},
	"QBO.EXPENSES.TIME_ACTIVITY_DELETE":    {Entity: "TimeActivity", Op: "delete"},
	"QBO.EXPENSES.SUPPLIER_CREATE":         {Entity: "Vendor", Op: "create"},
	"QBO.EXPENSES.SUPPLIER_DELETE":         {Entity: "Vendor", Op: "deactivate"},
	"QBO.EXPENSES.SUPPLIER_EDIT":           {Entity: "Vendor", Op: "update"},
	"QBO.EXPENSES.VENDOR_CREDIT_CREATE":    {Entity: "VendorCredit", Op: "create"},
	"QBO.EXPENSES.VENDOR_CREDIT_DELETE":    {Entity: "VendorCredit", Op: "delete"},
	"QBO.EXPENSES.VENDOR_CREDIT_EDIT":      {Entity: "VendorCredit", Op: "update"},
	"QBO.INVENTORY.ITEM_CREATE":            {Entity: "Item", Op: "create"},
	"QBO.INVENTORY.ITEM_DELETE":            {Entity: "Item", Op: "deactivate"},
	"QBO.INVENTORY.ITEM_EDIT":              {Entity: "Item", Op: "update"},
	"QBO.INVENTORY.PURCHASE_ORDER_CREATE":  {Entity: "PurchaseOrder", Op: "create"},
	"QBO.INVENTORY.PURCHASE_ORDER_DELETE":  {Entity: "PurchaseOrder", Op: "delete"},
	"QBO.INVENTORY.PURCHASE_ORDER_EDIT":    {Entity: "PurchaseOrder", Op: "update"},
	"QBO.INVENTORY.ADJUST_CREATE":          {Entity: "InventoryAdjustment", Op: "create"},
	"QBO.PAYROLL.EMPLOYEE_TERMINATE":       {Entity: "Employee", Op: "deactivate"},
	"QBO.PAYROLL.EMPLOYEE_CREATE":          {Entity: "Employee", Op: "create"},
	"QBO.PAYROLL.EMPLOYEE_EDIT":            {Entity: "Employee", Op: "update"},
	"QBO.COMPANY.TERM_CREATE":              {Entity: "Term", Op: "create"},
	"QBO.COMPANY.TERM_EDIT":                {Entity: "Term", Op: "update"},
	"QBO.COMPANY.TERM_DELETE":              {Entity: "Term", Op: "deactivate"},
	"QBO.COMPANY.PAYMENT_METHOD_CREATE":    {Entity: "PaymentMethod", Op: "create"},
	"QBO.COMPANY.PAYMENT_METHOD_EDIT":      {Entity: "PaymentMethod", Op: "update"},
	"QBO.COMPANY.PAYMENT_METHOD_DELETE":    {Entity: "PaymentMethod", Op: "deactivate"},
	"QBO.PAYROLL.TIMESHEET_CREATE":         {Entity: "TimeActivity", Op: "create"},
	"QBO.PAYROLL.TIMESHEET_EDIT":           {Entity: "TimeActivity", Op: "update"},
	"QBO.SALES.CREDIT_MEMO_CREATE":         {Entity: "CreditMemo", Op: "create"},
	"QBO.SALES.CREDIT_MEMO_DELETE":         {Entity: "CreditMemo", Op: "delete"},
	"QBO.SALES.CREDIT_MEMO_EDIT":           {Entity: "CreditMemo", Op: "update"},
	"QBO.SALES.ESTIMATE_CREATE":            {Entity: "Estimate", Op: "create"},
	"QBO.SALES.ESTIMATE_DELETE":            {Entity: "Estimate", Op: "delete"},
	"QBO.SALES.ESTIMATE_EDIT":              {Entity: "Estimate", Op: "update"},
	"QBO.SALES.ESTIMATE_COPY":              {Entity: "Estimate", Op: "copy"},
	"QBO.SALES.INVOICE_CREATE":             {Entity: "Invoice", Op: "create"},
	"QBO.SALES.INVOICE_DELETE":             {Entity: "Invoice", Op: "delete"},
	"QBO.SALES.INVOICE_EDIT":               {Entity: "Invoice", Op: "update"},
	"QBO.SALES.INVOICE_COPY":               {Entity: "Invoice", Op: "copy"},
	"QBO.SALES.INVOICE_DISCOUNT":           {Entity: "Invoice", Op: "discount"},
	"QBO.SALES.INVOICE_SEND":               {Entity: "Invoice", Op: "send"},
	"QBO.SALES.INVOICE_WRITE_OFF":          {Entity: "Invoice", Op: "writeoff"},
	"QBO.SALES.PAYMENT_CREATE":             {Entity: "Payment", Op: "create"},
	"QBO.SALES.PAYMENT_DELETE":             {Entity: "Payment", Op: "delete"},
	"QBO.SALES.PAYMENT_EDIT":               {Entity: "Payment", Op: "update"},
	"QBO.SALES.RECEIPT_CREATE":             {Entity: "SalesReceipt", Op: "create"},
	"QBO.SALES.RECEIPT_DELETE":             {Entity: "SalesReceipt", Op: "delete"},
	"QBO.SALES.RECEIPT_EDIT":               {Entity: "SalesReceipt", Op: "update"},
	"QBO.SALES.REFUND_RECEIPT_CREATE":      {Entity: "RefundReceipt", Op: "create"},
	"QBO.SALES.REFUND_RECEIPT_DELETE":      {Entity: "RefundReceipt", Op: "delete"},
	"QBO.SALES.REFUND_RECEIPT_EDIT":        {Entity: "RefundReceipt", Op: "update"},
	"QBO.SALES.SALES_ORDER_CREATE":         {Entity: "SalesOrder", Op: "create"},
	"QBO.SALES.SALES_ORDER_DELETE":         {Entity: "SalesOrder", Op: "delete"},
	"QBO.SALES.SALES_ORDER_EDIT":           {Entity: "SalesOrder", Op: "update"},

	"QBO.TAX.TAX_AGENCY_CREATE": {Entity: "TaxAgency", Op: "create"},
}

func maybeV3MutateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	if e.Mode == modeBlocked || e.Mode == modeExcluded {
		return nil
	}
	spec, ok := v3MutateByID[e.ID]
	if !ok {
		return nil
	}
	if spec.Entity == "SalesOrder" {
		return newSalesOrderMutateCmd(flags, e, command, spec.Op)
	}
	return newV3MutateCmd(flags, e, command, spec)
}

func newV3MutateCmd(flags *rootFlags, e primitiveEntry, command string, spec v3MutateSpec) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Annotations: map[string]string{readbackAnnotation: "true"},
		Use:         use,
		Short:       command + " (v3 " + spec.Op + ")",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if err := client.CheckLineItemsJSON(fm); err != nil {
				return feedErr(flags, err)
			}
			entity := spec.Entity
			if entity == "" {
				var err error
				entity, err = client.VoidableEntity(firstFlagString(fm, "entity", "txn-type"))
				if err != nil {
					return exitInput(err)
				}
			}
			if fm["id"] == "" && fm["txn-id"] != "" {
				fm["id"] = fm["txn-id"]
			}
			// Contract gate: a flag the resolved entity:op never consumes is
			// rejected loudly (documented meta params carry their reason)
			// instead of silently dropping out of the request body.
			var flagNames []string
			cmd.LocalFlags().VisitAll(func(f *pflag.Flag) { flagNames = append(flagNames, f.Name) })
			if entity != "" {
				if verr := client.ValidateMutationFlags(entity, spec.Op, cmd.Flags().Changed,
					flagNames, mutationDeclaredAllowance(e.ID, paramsFor(e.ID)), mutationMetaSet(e.ID)); verr != nil {
					return exitInput(verr)
				}
			}
			// --payee on expense/cheque create names a v3 entity id that
			// becomes EntityRef; a free-text label would be silently dropped
			// (bank-fee create's label-style payee is the exception).
			if (e.ID == "QBO.EXPENSES.EXPENSE_CREATE" || e.ID == "QBO.EXPENSES.CHEQUE_CREATE") && spec.Op == "create" {
				if payee := strings.TrimSpace(fm["payee"]); payee != "" && !isAllDigits(payee) {
					return exitInput(fmt.Errorf("--payee %q is not a QBO entity id — pass the vendor/customer numeric id (bank-fee create accepts free-text payees)", payee))
				}
			}
			if spec.Op == "create" {
				for _, p := range paramsFor(e.ID) {
					if !p.Required || (p.Name != "line-items" && p.Name != "lines" && p.Name != "items") {
						continue
					}
					if strings.TrimSpace(fm[p.Name]) == "" {
						return exitInput(fmt.Errorf("create requires --%s (JSON array of v3 line objects); omitted lines would record a $1 default", p.Name))
					}
				}
			}
			if flags.dryRun {
				url := client.PlannedMutateURL(entity, spec.Op)
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
			res, err := client.ReplayMutate(ctx, entity, spec.Op, fm["id"], fm)
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
	if spec.Op == "send" || spec.Entity == "SalesOrder" || spec.Entity == "RecurringTransaction" || spec.Entity == "Budget" {
		cmd.Annotations[readbackAnnotation] = "false"
	}
	attachParamFlags(cmd, e.ID)
	// The shared surface is the contract's data (cli/mutation_contract.go);
	// adding a flag here means every v3 mutate command accepts it — set-but-
	// unconsumed flags are still rejected by ValidateMutationFlags at RunE.
	for _, sf := range sharedMutationFlags {
		ensureFlag(cmd, sf.name, sf.help)
	}
	// Contract-driven attachment: every flag the resolved entity:op consumes
	// must be reachable on the command — this is the reverse-drift guard made
	// physical (consumed ⇒ attached).
	if consumed, ok := client.MutationFlagsConsumed(spec.Entity, spec.Op); ok {
		for _, n := range consumed {
			ensureFlag(cmd, n, "consumed by "+spec.Entity+" "+spec.Op+" (mutation contract)")
		}
	}
	if spec.Entity == "" {
		ensureFlag(cmd, "entity", "v3 entity to void: invoice, payment, salesreceipt, bill")
	}
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// isAllDigits reports whether s is a nonempty run of ASCII digits — the
// shape of a QBO v3 entity id.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// firstFlagString returns the first non-empty flag value among names.
func firstFlagString(fm map[string]string, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(fm[n]); v != "" {
			return v
		}
	}
	return ""
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

// newProjectCmd wires the QBO Projects surface — Work_Project on the
// accountant-workflow service. v3 has no project entity (verified: "Operation
// project is not supported") and Customer Job=true is a sub-customer job, not
// a project (IsProject=false).
func newProjectCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (accountantworkflow Work_Project)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			switch use {
			case "create", "update":
				if flags.dryRun {
					return writePlan(cmd, flags, planEnvelope{
						Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
						URL: "https://accountantworkflow.api.intuit.com/v4/graphql", Flags: fm,
						Note: "Work_Project mutation; not sent",
					})
				}
				var res *client.MutateResult
				var err error
				if use == "create" {
					res, err = client.ReplayProjectCreate(ctx, fm)
				} else {
					res, err = client.ReplayProjectUpdate(ctx, fm)
				}
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
			default: // get, search
				if flags.dryRun {
					return writePlan(cmd, flags, planEnvelope{
						Command: command, ID: e.ID, Mode: modeRead, Method: "POST",
						URL: "https://accountantworkflow.api.intuit.com/v4/graphql", Flags: fm,
						Note: "getJobs_workflow company.projects; not sent",
					})
				}
				limit, _ := strconv.Atoi(firstFlagString(fm, "limit"))
				res, err := client.ReplayProjectList(ctx, firstFlagString(fm, "id"), firstFlagString(fm, "query"), limit)
				if err != nil {
					return feedErr(flags, err)
				}
				printQuery(cmd.OutOrStdout(), flags, res)
				return nil
			}
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "id", "Work_Project global id (get/update)")
	if use == "create" || use == "update" {
		for _, n := range dedicatedConsumed[e.ID] {
			ensureFlag(cmd, n, "consumed by "+command+" (mutation contract)")
		}
	}
	if use == "get" || use == "search" {
		ensureFlag(cmd, "query", "client-side substring filter")
		ensureFlag(cmd, "limit", "max results")
	}
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newReconcileCreateCmd wires reconcile session create to the proven
// updateIntegration_Reconciliation BEGIN mutation on qbonline-aws.
func newReconcileCreateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (qbonline-aws StartReconcile)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedReconcileMutateURL(), Flags: fm,
					Note: "updateIntegration_Reconciliation BEGIN; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayReconcileSession(ctx, "BEGIN",
				firstFlagString(fm, "id", "account-id", "account"),
				firstFlagString(fm, "ending-balance"), firstFlagString(fm, "ending-date"))
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
	ensureFlag(cmd, "id", "bank account id to reconcile (e.g. 45)")
	ensureFlag(cmd, "ending-balance", "statement ending balance, e.g. 0.00 (required)")
	ensureFlag(cmd, "ending-date", "statement ending date yyyy-MM-dd (required)")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newFixedAssetCmd wires fixed-asset create/update/delete to the captured
// assetservice.api.intuit.com GraphQL mutations (createAsset/updateAsset/
// deleteAsset) — proven end-to-end on TC2 (create → rename → delete).
func newFixedAssetCmd(flags *rootFlags, e primitiveEntry, command, op string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (assetservice " + op + "Asset)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedFixedAssetMutateURL(), Flags: fm,
					Note: "assetservice " + op + "Asset mutation; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayFixedAssetMutate(ctx, op, fm)
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
	ensureFlag(cmd, "id", "assetId (update/delete)")
	if op != "delete" {
		ensureFlag(cmd, "name", "asset display name")
		ensureFlag(cmd, "cost", "purchase cost (alias of --price)")
		ensureFlag(cmd, "price", "purchase price (alias of --cost)")
		ensureFlag(cmd, "purchase-date", "purchase date yyyy-MM-dd")
		ensureFlag(cmd, "asset-account", "COA account id for the asset (create)")
		ensureFlag(cmd, "dep-expense-account", "COA account id for depreciation expense (create)")
		ensureFlag(cmd, "life-years", "useful life in years (alias of --useful-life)")
		ensureFlag(cmd, "salvage", "salvage value")
		ensureFlag(cmd, "method", "depreciation method: STRAIGHT_LINE, DIMINISHING_VALUE_150")
		ensureFlag(cmd, "currency", "currency code (default AUD)")
		ensureFlag(cmd, "description", "asset description")
	}
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newSettingsUpdateCmd wires company settings update to the captured
// settingsfacade updateQbAppFoundationQbSettings mutation — proven live on TC2
// (accountNumbersEnabled toggle round-trip, entityVersion 5→7).
func newSettingsUpdateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	var patch string
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (settingsfacade UpdateSettings)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedSettingsUpdateURL(), Flags: fm,
					Note: "settingsfacade updateQbAppFoundationQbSettings; not sent",
				})
			}
			var p map[string]any
			if err := json.Unmarshal([]byte(patch), &p); err != nil {
				return exitInput(fmt.Errorf("--patch must decode to a JSON object: %v", err))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplaySettingsUpdate(ctx, p)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Note)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	cmd.Flags().StringVar(&patch, "patch", "", "JSON object of accountingCoreSettings leaf fields, e.g. {\"accountNumbersEnabled\":true} (required)")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newMileageCmd wires mileage reads to the trips service — company.trips on
// trips.api.intuit.com/v4/graphql. Trips are US-region; TC2 (AU) returns an
// empty edges list, which still proves the contract.
func newMileageCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (trips company.trips)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeRead, Method: "POST",
					URL: "https://trips.api.intuit.com/v4/graphql", Flags: fm,
					Note: "trips MileageListQuery; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			limit, _ := strconv.Atoi(firstFlagString(fm, "limit"))
			res, err := client.ReplayTripsList(ctx, firstFlagString(fm, "id"), firstFlagString(fm, "query"), limit)
			if err != nil {
				return feedErr(flags, err)
			}
			printQuery(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "id", "trip node id (get); omit to list")
	ensureFlag(cmd, "query", "client-side substring filter")
	ensureFlag(cmd, "limit", "max results")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newTripMutateCmd wires mileage create/update/delete to the trips
// updateTrips_Trip mutation on trips.api.intuit.com — captured live on TC2
// (Add-trip form) and proven end-to-end (create → update → soft delete).
func newTripMutateCmd(flags *rootFlags, e primitiveEntry, command, op string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (trips updateTrips_Trip)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedTripMutateURL(), Flags: fm,
					Note: "trips updateTrips_Trip " + op + "; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTripMutate(ctx, op, fm)
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
	ensureFlag(cmd, "id", "trip node id (update/delete)")
	if op != "delete" {
		for _, n := range dedicatedConsumed[e.ID] {
			ensureFlag(cmd, n, "consumed by "+command+" (mutation contract)")
		}
	}
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCurrencyRateCmd wires company currency update edit to the captured
// businessTransactionUpdateCurrencyExchangeRate mutation on sbseggraphqlorch —
// proven live on TC2 2026-09-17 (USD 1.405185→1.405190, restored to market).
func newCurrencyRateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (sbseggraphqlorch businessTransactionUpdateCurrencyExchangeRate)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedCurrencyRateURL(), Flags: fm,
					Note: "sbseggraphqlorch businessTransactionUpdateCurrencyExchangeRate; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCurrencyRateSet(ctx,
				firstFlagString(fm, "id", "currency"), firstFlagString(fm, "rate"), firstFlagString(fm, "date"))
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
	ensureFlag(cmd, "id", "3-letter currency code (e.g. USD)")
	ensureFlag(cmd, "rate", "exchange rate vs home currency")
	ensureFlag(cmd, "date", "rate effective date dd/MM/yyyy (default today)")
	attachDedicatedConsumed(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCurrencyDeleteCmd wires company currency delete to the captured
// businessTransactionDeletePreferredForeignCurrency mutation on
// sbseggraphqlorch — proven live on TC2 2026-09-17 (NZD added then removed).
func newCurrencyDeleteCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (sbseggraphqlorch businessTransactionDeletePreferredForeignCurrency)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedCurrencyDeleteURL(), Flags: fm,
					Note: "sbseggraphqlorch businessTransactionDeletePreferredForeignCurrency; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCurrencyDelete(ctx,
				firstFlagString(fm, "id", "currency"))
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
	ensureFlag(cmd, "id", "3-letter currency code (e.g. NZD)")
	attachDedicatedConsumed(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newReclassifyCmd wires advanced reclassify run to the captured
// qbo.intuit.com datarequest reclassifyTxn surface — proven live on TC2
// 2026-09-18 (Accounting and bookkeeping ↔ Office expenses, 3 lines).
func newReclassifyCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (qbo datarequest reclassifyTxn)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedReclassifyURL(), Flags: fm,
					Note: "qbo datarequest reclassifyTxn/reclassify; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayReclassify(ctx,
				firstFlagString(fm, "target"),
				firstFlagString(fm, "from-account", "source"),
				firstFlagString(fm, "to-account", "destination", "dest"),
				firstFlagString(fm, "from", "from-date"),
				firstFlagString(fm, "to", "to-date"),
				firstFlagString(fm, "basis"))
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
	ensureFlag(cmd, "from-account", "source account name or id (required)")
	ensureFlag(cmd, "to-account", "target account name or id (required)")
	ensureFlag(cmd, "from", "start date dd/MM/yyyy (default last month)")
	ensureFlag(cmd, "to", "end date dd/MM/yyyy (default today)")
	ensureFlag(cmd, "basis", "accrual | cash (default accrual)")
	attachDedicatedConsumed(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newBackupCreateCmd wires advanced backup create to the captured
// backuprestore POST /v1/backup — proven live on TC2 2026-09-18
// (incremental manual backup queued; row appears on the Backups tab).
func newBackupCreateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (backuprestore POST /v1/backup)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedBackupURL(), Flags: fm,
					Note: "backuprestore POST /v1/backup; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayBackupCreate(ctx, firstFlagString(fm, "type", "backup-type"))
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
	ensureFlag(cmd, "type", "incremental | full | complete (default incremental)")
	attachDedicatedConsumed(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCustomersImportCmd wires company customers import to the captured neo
// importdata/import contract — proven live on TC2 2026-09-18 (Customers
// wizard: upload → map → Done created customer id 20).
func newCustomersImportCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (neo importdata/import?entity=Customers)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedImportDataURL("Customers"), Flags: fm,
					Note: "neo importdata/import entityInfo+userMappings; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCustomersImport(ctx, firstFlagString(fm, "file", "path"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "path", "alias for --file")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newSuppliersImportCmd wires company suppliers import to the captured neo
// importdata/import?entity=Vendors contract — proven live on TC2 2026-09-18
// (Vendors wizard; identical field set to Customers, vendor custlist keys).
func newSuppliersImportCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (neo importdata/import?entity=Vendors)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedImportDataURL("Vendors"), Flags: fm,
					Note: "neo importdata/import entityInfo+userMappings; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplaySuppliersImport(ctx, firstFlagString(fm, "file", "path"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "path", "alias for --file")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newBillsImportCmd wires company bills import to the captured dataimport
// contract — v4 createTransactions_Transaction + lists/name/save for missing
// suppliers. Proven live on TC2 2026-09-18.
func newBillsImportCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 createTransactions_Transaction + lists/name/save)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedBillsImportURL(), Flags: fm,
					Note: "v4 createTransactions_Transaction per row; missing suppliers via neo lists/name/save; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayBillsImport(ctx, firstFlagString(fm, "file", "path"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "path", "alias for --file")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newInvoicesImportCmd wires sales invoice list create to the captured
// dataimport/invoices contract — v4 createTransactions_Transaction
// (SALE_INVOICE) + lists/name/save nameTypeId 0 for missing customers.
// Proven live on TC2 2026-09-18 (invoice 146, customer 37).
func newInvoicesImportCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 createTransactions_Transaction + lists/name/save)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedInvoicesImportURL(), Flags: fm,
					Note: "v4 createTransactions_Transaction per row; missing customers via neo lists/name/save; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayInvoicesImport(ctx, firstFlagString(fm, "file", "path"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "path", "alias for --file")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newReceiptExpenseCreateCmd wires expenses receipt-expense create to the
// captured receipts-UI contract — stagetransactions ACCEPTED_ADDED
// writeInSync converting a staged receipt into a PURCHASE. Proven live on
// TC2 2026-09-18 (Purchase 148).
func newReceiptExpenseCreateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (stagetransactions ACCEPTED_ADDED)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedReceiptExpenseURL(), Flags: fm,
					Note: "stagetransactions /stage/entities ACCEPTED_ADDED writeInSync; not sent",
				})
			}
			if m := firstFlagString(fm, "match"); strings.TrimSpace(m) != "" {
				return fmt.Errorf("--match %q: the captured contract only covers create; matching a receipt to an existing transaction is not wired (use the receipts UI)", m)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayReceiptExpenseCreate(ctx,
				firstFlagString(fm, "receipt-id", "id"),
				firstFlagString(fm, "payee"),
				firstFlagString(fm, "payment-account", "account"),
				firstFlagString(fm, "line-items", "lines"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "id", "alias for --receipt-id")
	ensureFlag(cmd, "account", "alias for --payment-account")
	ensureFlag(cmd, "lines", "alias for --line-items")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newChequeExportCmd wires expenses cheque export to the captured
// cheque-form Print contract — v4 transaction PDF download. Proven live on
// TC2 2026-09-18 (Cheque 159 preview iframe).
func newChequeExportCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 transaction .pdf)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "GET",
					URL: client.PlannedChequeExportURL(), Flags: fm,
					Note: "v4 transactions/<ns-id>.pdf; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayChequeExport(ctx,
				firstFlagString(fm, "id"),
				firstFlagString(fm, "out", "output", "file"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Note)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "out", "write the PDF to this path (default: report size only)")
	ensureFlag(cmd, "output", "alias for --out")
	ensureFlag(cmd, "file", "alias for --out")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newExpenseSplitCmd wires expenses expense update split to the v3
// sparse-update line-replacement contract — --splits lines must sum to the
// purchase total.
func newExpenseSplitCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v3 sparse line replacement)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/purchase?minorversion=73",
					Flags: fm,
					Note:  "v3 Purchase sparse update replacing Line[] with the split set; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayExpenseSplit(ctx, fm)
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
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newStatementCreateCmd wires sales statement create to the captured
// Create-statements dialog contract — neo statement/print with the
// type/customerIds/date-range body. Proven live on TC2 2026-09-18.
func newStatementCreateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (neo statement/print)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedStatementURL(), Flags: fm,
					Note: "neo statement/print type+customerIds+dates; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayStatementCreate(ctx,
				firstFlagString(fm, "customer"),
				firstFlagString(fm, "start-date", "start"),
				firstFlagString(fm, "end-date", "end"),
				firstFlagString(fm, "type", "format"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "start", "alias for --start-date")
	ensureFlag(cmd, "end", "alias for --end-date")
	ensureFlag(cmd, "format", "alias for --type")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newTimeInvoiceCmd wires sales time-invoice create to the captured
// browser invoice-form contract: billable TimeActivities become v4
// UpdateTransactions_Transaction_qbo invoices whose item lines consume the
// charge via links.sources{id,sourceLine,sourceTxn:ACTIVITY_TIME}. Each
// charge is preflighted against its v4 links.targets because v3
// BillableStatus lags behind consumption. Proven live on TC2 2026-09-19.
// --auto fails loudly (automation setting not on this build).
func newTimeInvoiceCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 links.sources invoice)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: "https://qbo.intuit.com/api/v4/graphql", Flags: fm,
					Note: "UpdateTransactions_Transaction_qbo invoice consuming charges via links.sources; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTimeInvoice(ctx,
				firstFlagString(fm, "customer"),
				firstFlagString(fm, "auto"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Name != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " %s", res.Item.Name)
			}
			if res.Item.Amount != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " %.2f", res.Item.Amount)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newInvoiceRemindCmd wires sales invoice run remind to the captured
// invoices-list "Send reminder" contract: neo purchsales/send with an
// EMAIL deliveryInfo (recipient resolved from the invoice's BillEmail or
// customer). Proven live on TC2 2026-09-19 (invoice 7). --automatic fails
// loudly — scheduled reminder workflows are a separate automation surface.
func newInvoiceRemindCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (preview by default; --send emails a reminder)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			send, _ := cmd.Flags().GetBool("send")
			if flags.dryRun || !send {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: "https://qbo.intuit.com/api/neo/v1/company/{realm}/purchsales/send", Flags: fm,
					Note: "EMAIL reminder preview; not sent; --send opts in to a real email",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayInvoiceReminder(ctx,
				firstFlagString(fm, "id"),
				firstFlagString(fm, "automatic"),
				firstFlagString(fm, "to"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Name != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " %s", res.Item.Name)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCreditCardCreditCmd wires sales credit-card-credit create to the
// captured v4 UpdateTransactions_Transaction_qbo contract (type PURCHASE,
// transactionType CREDIT_CARD_CREDIT, txnTypeId 11, accountLines). v3
// creditcardcredit is not supported on this build (verified live: 500
// "Operation creditcardcredit is not supported").
func newCreditCardCreditCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 transaction-form credit)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: "https://qbo.intuit.com/api/v4/graphql", Flags: fm,
					Note: "UpdateTransactions_Transaction_qbo PURCHASE/CREDIT_CARD_CREDIT with accountLines; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCreditCardCredit(ctx,
				firstFlagString(fm, "credit-card-account", "cc-account"),
				fm["account"],
				firstFlagString(fm, "date", "txn-date"),
				fm["category-account"],
				firstFlagString(fm, "line-items", "lines"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Amount != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " %.2f", res.Item.Amount)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newInvoiceProgressCmd wires sales invoice update progress to the captured
// v4 UpdateTransactions_Transaction_qbo contract: SALE_INVOICE with
// ESTIMATE_INVOICE_LINK detailType, progressTracking PERCENT, and
// links.sources consuming the estimate at line + transaction level. The
// company must have Progress Invoicing enabled (verified on TC2).
func newInvoiceProgressCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 progress invoice from estimate)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: "https://qbo.intuit.com/api/v4/graphql", Flags: fm,
					Note: "UpdateTransactions_Transaction_qbo SALE_INVOICE with ESTIMATE_INVOICE_LINK + links.sources; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayInvoiceProgress(ctx,
				fm["estimate"], fm["percent"], fm["id"])
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Amount != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " %.2f", res.Item.Amount)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newDelayedChargeCmd wires sales delayed-charge/delayed-credit create to the
// captured v4 UpdateTransactions_Transaction_qbo contracts — ACTIVITY_CHARGE
// (txnTypeId 25, /app/nonpostingcharge) and ACTIVITY_CREDIT (txnTypeId 13,
// /app/nonpostingcredit). v3 has neither entity on this build (verified live).
func newDelayedChargeCmd(flags *rootFlags, e primitiveEntry, command, kind string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 non-posting " + kind + ")",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: "https://qbo.intuit.com/api/v4/graphql", Flags: fm,
					Note: "UpdateTransactions_Transaction_qbo ACTIVITY_" + strings.ToUpper(kind) + " with itemLines; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDelayedCharge(ctx, kind,
				fm["customer"],
				firstFlagString(fm, kind+"-date", "date", "txn-date"),
				firstFlagString(fm, "line-items", "lines"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Amount != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " %.2f", res.Item.Amount)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newChequeBounceCmd wires expenses cheque update bounce to the v3
// JournalEntry reversal contract — the AU help-documented procedure (no form
// exposes a bounce action; v3 rejects A/R in expense lines, verified live).
func newChequeBounceCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v3 JournalEntry A/R reversal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/v3/company/<realm>/journalentry",
					Flags: fm,
					Note:  "JournalEntry Debit A/R(customer) / Credit deposit account = payment amount (+ optional --fee legs); not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayChequeBounce(ctx,
				firstFlagString(fm, "id", "payment", "payment-id"),
				firstFlagString(fm, "date", "txn-date"),
				fm["fee"])
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Amount != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " %.2f", res.Item.Amount)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCreditTransferCmd wires customers customer run (credit transfer) to the
// v3 JournalEntry workaround the AU doc prescribes — Dr A/R on the source
// customer, Cr A/R on the destination, both with customer entities (same
// mechanism as the cheque-bounce reversal; no transfer endpoint exists).
func newCreditTransferCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v3 JournalEntry credit transfer)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/v3/company/<realm>/journalentry",
					Flags: fm,
					Note:  "JournalEntry Debit A/R(--from) / Credit A/R(--to) × --amount; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCreditTransfer(ctx, fm["from"], fm["to"], fm["amount"])
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Amount != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " %.2f", res.Item.Amount)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCurrencyRevalueCmd wires company currency update revalue to the captured
// neo home-currency-adjustment contract (Settings > Currencies > Revalue
// currency): GET calculateHomeCurrencyAdjustment → POST
// doHomeCurrencyAdjustment. Captured on TC2 2026-09-19; creates a JE against
// the Exchange Gain or Loss account.
func newCurrencyRevalueCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (neo doHomeCurrencyAdjustment)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/neo/v1/company/<realm>/multicurrency/doHomeCurrencyAdjustment",
					Flags: fm,
					Note:  "GET calculateHomeCurrencyAdjustment then POST the entity list; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCurrencyRevalue(ctx, fm["date"], fm["rate"])
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s", res.Op, res.Entity, res.Item.ID)
			if res.Item.Amount != 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " %.2f", res.Item.Amount)
			}
			if res.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Note)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCoaImportCmd wires company coa import to the captured neo
// importdata/import?entity=coa contract — proven live on TC2 2026-09-18.
func newCoaImportCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (neo importdata/import?entity=coa)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedImportDataURL("coa"), Flags: fm,
					Note: "neo importdata/import entityInfo+userMappings; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCoaImport(ctx, firstFlagString(fm, "file", "path"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "path", "alias for --file")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newItemsImportCmd wires company items import to the captured
// qbbulkaction bulkActionOnProductList ASYNC contract — proven live on TC2
// 2026-09-18 (Products and Services import wizard → batch grid → Save).
func newItemsImportCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (qbbulkaction bulkActionOnProductList)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedItemsImportURL(), Flags: fm,
					Note: "bulk ASYNC product create + transactionBulkActionList poll; accounts/categories resolved via v3; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayItemsImport(ctx, firstFlagString(fm, "file", "path"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "path", "alias for --file")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newReceiptUploadCmd wires expenses receipt import to the captured
// financialdocument v2/documents multipart contract — proven live on TC2
// 2026-09-18 (/app/receipts "Upload from computer").
func newReceiptUploadCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (financialdocument v2/documents multipart)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedReceiptUploadURL(), Flags: fm,
					Note: "multipart document json + image; autoClassification OCR; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayReceiptUpload(ctx, firstFlagString(fm, "files", "file", "path"))
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "path", "alias for --file")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newMergeCmd wires customer/supplier merge to the captured v4
// updateContact_qbo mergedTo contract — proven live on TC2 2026-09-18
// (customer 32 folded into 31 via the "Merge contacts" confirmation).
// Merging is irreversible; the dup's record folds into keep.
func newMergeCmd(flags *rootFlags, e primitiveEntry, command, entity string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v4 updateContact mergedTo; irreversible)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			keep := firstFlagString(fm, "keep")
			merge := firstFlagString(fm, "merge")
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedMergeURL(), Flags: fm,
					Note: "v4 updateContact_qbo mergedTo — dup folds into keep, irreversible; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayMerge(ctx, entity, keep, merge)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.Name)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newFormStyleUpdateCmd wires company form-style update to the captured
// txnsrendering PUT /v2/customizations/{id} full-doc write — proven live on
// TC2 2026-09-18 (Standard template accent colour round-trip).
func newFormStyleUpdateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (txnsrendering PUT customizations)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			id := firstFlagString(fm, "id")
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "PUT",
					URL: client.PlannedFormStyleUpdateURL(id), Flags: fm,
					Note: "txnsrendering read-modify-write; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayFormStyleUpdate(ctx, id,
				firstFlagString(fm, "name"),
				firstFlagString(fm, "color", "colour"))
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
	ensureFlag(cmd, "name", "new customization (style) name")
	ensureFlag(cmd, "color", "accent colour #RRGGBB")
	ensureFlag(cmd, "colour", "alias for --color")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCustomFieldDeleteCmd wires custom-field delete ("Make inactive") to the
// captured updateCustomFieldDefinition mutation on customextensions — proven
// live on TC2 2026-09-17 (QBVerifyField → deleted:true).
func newCustomFieldDeleteCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (customextensions updateCustomFieldDefinition deleted=true)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "POST",
					URL: client.PlannedCustomFieldDeleteURL(), Flags: fm,
					Note: "customextensions updateCustomFieldDefinition deleted=true; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCustomFieldDelete(ctx, firstFlagString(fm, "id"))
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
	ensureFlag(cmd, "id", "custom-field definition id")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newCustomFieldMutateCmd wires company custom-field create/update to the
// create/updateCustomFieldDefinition mutations on customextensions.api.intuit.com.
func newCustomFieldMutateCmd(flags *rootFlags, e primitiveEntry, command, op string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (customextensions " + op + "CustomFieldDefinition)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command,
					ID:      e.ID,
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedCustomFieldMutateURL(),
					Flags:   fm,
					Note:    "customextensions CustomFieldDefinition mutation; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCustomFieldMutate(ctx, op, fm)
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
	ensureFlag(cmd, "id", "custom field definition id (update)")
	ensureFlag(cmd, "name", "custom field name")
	ensureFlag(cmd, "type", "field data type: text, dropdown, number, date")
	ensureFlag(cmd, "entities", "comma-separated entity types the field applies to")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newFormStyleDeleteCmd wires `sales invoice form delete` to the captured
// txnsrendering DELETE /v2/customizations/{id}?templateEditSequence&forceDelete
// — proven live on TC2 2026-09-19 (custom style removed via kebab → Delete).
func newFormStyleDeleteCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (txnsrendering DELETE customizations)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			id := firstFlagString(fm, "template-id", "id")
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command, ID: e.ID, Mode: modeWired, Method: "DELETE",
					URL: client.PlannedFormStyleUpdateURL(id) + "?templateEditSequence=<seq>&forceDelete=true", Flags: fm,
					Note: "txnsrendering GET seq then DELETE forceDelete; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayFormStyleDelete(ctx, id)
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
	applyCatalogHelp(cmd, e.ID)
	return cmd
}
