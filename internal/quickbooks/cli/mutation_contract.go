package cli

import (
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// mutation_contract.go carries the cli-layer half of the mutation flag
// contract (the consumed sets live in client.MutationFlagsConsumed):
//
//   - sharedMutationFlags: the common flag surface every v3 mutate command
//     attaches via ensureFlag — a superset of what any single op consumes, so
//     a mistyped flag is accepted by cobra then rejected loudly by
//     client.ValidateMutationFlags instead of being silently dropped
//   - dedicatedConsumed: per-catalog-ID consumed sets for commands that
//     resolve to dedicated implementations instead of the v3 builder
//   - mutationMetaParams: catalog-declared params that are intentionally not
//     consumed, each with a documented reason — ValidateMutationFlags rejects
//     them at run time with that reason
//   - mutationAliasGroups: equivalent flag names — attaching any member of a
//     group satisfies the contract for all members
//
// mutation_contract_test.go cross-checks on the real resolved command tree:
// declared ⊆ consumed ∪ meta, consumed ⊆ attached (alias-aware), and
// attached ⊆ declared ∪ consumed ∪ shared ∪ meta.

// sharedMutationFlags are attached to every v3 mutate command. They are a
// superset of what any single op consumes; setting one the op does not
// consume is rejected by ValidateMutationFlags at run time.
var sharedMutationFlags = []struct{ name, help string }{
	{"id", "target QBO id"},
	{"amount", "AUD amount"},
	{"item", "item id for sales lines"},
	{"account", "account id"},
	{"start-date", "budget start date dd/MM/yyyy (alternative to --fiscal-year)"},
	{"from-account", "debit/from account id"},
	{"to-account", "credit/to account id"},
	{"name", "display or account name"},
	{"type", "account or item type"},
	{"customer", "customer id"},
	{"supplier", "supplier/vendor id"},
	{"bill-id", "bill id to pay (BillPayment)"},
	{"category-account", "expense/category account id (distinct from payment)"},
	{"payment-type", "Cash, Check, or CreditCard"},
	{"pay-type", "Check or CreditCard (BillPayment)"},
	{"email", "email address to send to"},
	{"to", "override recipient email (invoice send)"},
	{"employee", "employee id for timesheets and mileage"},
	{"hours", "hours for TimeActivity"},
	{"qty", "quantity difference for inventory adjust"},
	{"subtype", "account subtype (e.g. SuppliesMaterialsCogs)"},
	{"income-account", "income account id for items"},
	{"expense-account", "COGS/expense account id for inventory items"},
	{"asset-account", "inventory asset account id"},
	{"discount", "invoice discount amount"},
	{"discount-percent", "invoice discount percent"},
	{"invoice-id", "invoice id to link (credit memo / write-off / payment)"},
	{"time-activity", "TimeActivity id to bill on an invoice"},
	{"address", "billing street address"},
	{"charge-date", "delayed charge date"},
	{"credit-date", "delayed credit date"},
}

// dedicatedConsumed records the consumed flag set for catalog commands that
// resolve to a dedicated implementation instead of the generic v3 builder —
// keyed by catalog ID because the entity:op contract does not apply there.
// Each entry is the audited set of flag names that command reads (directly,
// or via the fm map it passes to its client replay function). For commands
// that bind flags without attachParamFlags this doubles as the locals list.
var dedicatedConsumed = map[string][]string{
	// Bulk journal create/delete run v3 /batch, registered directly under
	// `accounting journal` (not via stubs).
	"QBO.ACCOUNTING.JOURNAL_CREATE":         {"items-json"},
	"QBO.ACCOUNTING.JOURNAL_DELETE":         {"ids"},
	"QBO.ACCOUNTING.OPENING_BALANCE_CREATE": {"date", "lines"},
	// Class/Department update+delete are the dimension commands (ReplayMutate
	// underneath, but with a deliberately narrow bound-flag surface).
	"QBO.ACCOUNTING.CLASS_EDIT":      {"id", "name", "active"},
	"QBO.ACCOUNTING.CLASS_DELETE":    {"id"},
	"QBO.ACCOUNTING.LOCATION_EDIT":   {"id", "name", "active"},
	"QBO.ACCOUNTING.LOCATION_DELETE": {"id"},
	// Deferred/prepaid schedules pass --data through to the captured
	// UpdateDeferredRecognitionScheduleOp; --draft previews, --yes executes.
	"QBO.ACCOUNTING.DEFERRED_RECOGNITION_CREATE": {"data", "yes", "draft"},
	"QBO.ACCOUNTING.PREPAID_CREATE":              {"data", "yes", "draft"},
	// Projects use the work-svc GraphQL mutation surface.
	"QBO.ACCOUNTING.PROJECT_CREATE": {"customer", "customer-id", "client-id", "name", "description", "memo", "notes", "due-date"},
	"QBO.ACCOUNTING.PROJECT_EDIT":   {"id", "name", "description", "memo", "notes", "due-date", "status", "entity-version"},
	// Reconcile session start (qbonline-aws StartReconcile).
	"QBO.ACCOUNTING.RECONCILE_CREATE": {"id", "account-id", "account", "ending-balance", "ending-date"},
	// Fixed assets use the fixed-asset-svc GraphQL surface.
	"QBO.ACCOUNTING.FIXED_ASSET_CREATE": {"name", "description", "asset-account", "dep-expense-account", "currency", "life-years", "useful-life", "method", "price", "cost", "purchase-date", "salvage"},
	"QBO.ACCOUNTING.FIXED_ASSET_EDIT":   {"id", "name", "description", "asset-account", "dep-expense-account", "currency", "life-years", "useful-life", "method", "price", "cost", "purchase-date", "salvage"},
	"QBO.ACCOUNTING.FIXED_ASSET_DELETE": {"id"},
	// Neo/import and miscellaneous single-purpose commands.
	"QBO.ADVANCED.BACKUP_CREATE":    {"type", "backup-type"},
	"QBO.ADVANCED.BATCH_RUN":        {"file"},
	"QBO.ADVANCED.RECLASSIFY_RUN":   {"target", "from-account", "source", "to-account", "destination", "dest", "from", "from-date", "to", "to-date", "basis"},
	"QBO.COMPANY.ATTACHABLE_CREATE": {"file"},
	"QBO.COMPANY.BILLS_IMPORT":      {"file", "path"},
	"QBO.COMPANY.COA_IMPORT":        {"file", "path"},
	"QBO.COMPANY.CUSTOMERS_IMPORT":  {"file", "path"},
	"QBO.COMPANY.ITEMS_IMPORT":      {"file", "path"},
	"QBO.COMPANY.SUPPLIERS_IMPORT":  {"file", "path"},
	"QBO.EXPENSES.BILL_IMPORT":      {"file", "path"},
	"QBO.SALES.INVOICE_LIST_CREATE": {"file", "path"},
	// Company currency rates, custom fields, form styles, settings patch.
	"QBO.COMPANY.CURRENCY_EDIT":        {"id", "currency", "rate", "date"},
	"QBO.COMPANY.CURRENCY_DELETE":      {"id", "currency"},
	"QBO.COMPANY.CURRENCY_REVALUE":     {"date", "rate"},
	"QBO.COMPANY.CUSTOM_FIELD_CREATE":  {"entities", "id", "name", "type"},
	"QBO.COMPANY.CUSTOM_FIELD_EDIT":    {"entities", "id", "name", "type"},
	"QBO.COMPANY.CUSTOM_FIELD_DELETE":  {"id"},
	"QBO.COMPANY.FORM_STYLE_EDIT":      {"id", "name", "color", "colour"},
	"QBO.SALES.INVOICE_FORM_DELETE":    {"template-id", "id"},
	"QBO.EXPENSES.BILL_FORM_DELETE":    {"id"},
	"QBO.EXPENSES.EXPENSE_FORM_DELETE": {"id"},
	"QBO.COMPANY.SETTINGS_EDIT":        {"patch"},
	// Customer/supplier merges take --keep/--merge entity ids.
	"QBO.CUSTOMERS.CUSTOMER_MERGE":           {"keep", "merge"},
	"QBO.EXPENSES.SUPPLIER_MERGE":            {"keep", "merge"},
	"QBO.CUSTOMERS.CUSTOMER_CREDIT_TRANSFER": {"from", "to", "amount"},
	// Expense-side dedicated paths.
	"QBO.EXPENSES.CHEQUE_BOUNCE":          {"id", "payment", "payment-id", "date", "txn-date", "fee"},
	"QBO.EXPENSES.CHEQUE_REPRINT":         {"id", "out", "output", "file"},
	"QBO.EXPENSES.EXPENSE_RECATEGORISE":   {"id", "line-id", "category", "category-id", "class-id", "payee", "payee-id", "expected-category-id", "expected-class-id", "expected-sync-token", "repair-credit-card-type"},
	"QBO.EXPENSES.EXPENSE_SPLIT":          {"id", "splits"},
	"QBO.EXPENSES.MILEAGE_CREATE":         {"trip-type", "purpose", "description", "date", "distance-km", "distance", "start", "end", "vehicle"},
	"QBO.EXPENSES.MILEAGE_EDIT":           {"id", "trip-type", "purpose", "description", "date", "distance-km", "distance", "start", "end", "vehicle"},
	"QBO.EXPENSES.MILEAGE_DELETE":         {"id"},
	"QBO.EXPENSES.RECEIPT_UPLOAD":         {"files", "file", "path"},
	"QBO.EXPENSES.RECEIPT_EXPENSE_CREATE": {"receipt-id", "id", "payee", "payment-account", "account", "line-items", "lines", "match"},
	// Warehouse-management item receipts, PO partial-receive, and the shared
	// inventory v3-mutation command (count/build-assembly/starting-value
	// creates and adjust delete — registered from inline primitiveEntry
	// literals in inventory.go, not the catalog).
	"QBO.INVENTORY.ITEM_RECEIPT_CREATE":    {"vendor-id", "item-id", "qty", "rate", "date", "ref-no", "memo", "currency"},
	"QBO.INVENTORY.ITEM_RECEIPT_EDIT":      {"id", "qty", "rate", "date", "ref-no", "memo"},
	"QBO.INVENTORY.PURCHASE_ORDER_PARTIAL": {"id", "items", "purchase-order"},
	"QBO.INVENTORY.ADJUST_DELETE":          {"id"},
	"QBO.INVENTORY.COUNT_CREATE":           {"item", "counted-qty", "new-qty", "qty", "date", "memo"},
	"QBO.INVENTORY.BUILD_ASSEMBLY_CREATE":  {"item", "qty", "date", "memo"},
	"QBO.INVENTORY.STARTING_VALUE_CREATE":  {"item", "qty", "cost", "initial-cost", "asset-account", "account", "date"},
	// Sales orders use the commercecontrol GraphQL service; the strict
	// allowlist lives in client.salesOrderFlagFields.
	"QBO.SALES.SALES_ORDER_CREATE": {
		"customer", "customer-id", "date", "txn-date", "order-date", "sales-order-date",
		"due-date", "issued-at", "term", "terms", "memo", "message", "customer-note",
		"order-number", "order-no", "doc-number", "currency", "currency-iso",
		"print-template-id", "billing-address", "shipping-address",
		"line-items", "lines", "ignore-duplicate-order-num",
	},
	"QBO.SALES.SALES_ORDER_EDIT": {
		"id", "customer", "customer-id", "date", "txn-date", "order-date", "sales-order-date",
		"due-date", "issued-at", "term", "terms", "memo", "message", "customer-note",
		"order-number", "order-no", "doc-number", "currency", "currency-iso",
		"print-template-id", "billing-address", "shipping-address",
		"line-items", "lines", "ignore-duplicate-order-num",
	},
	"QBO.SALES.SALES_ORDER_DELETE": {"id"},
	// `sales <doc> run send` commands are dedicated SendSalesForm calls.
	"QBO.SALES.INVOICE_SEND":     {"id", "to"},
	"QBO.SALES.ESTIMATE_SEND":    {"id", "to"},
	"QBO.SALES.CREDIT_MEMO_SEND": {"id", "to"},
	"QBO.SALES.RECEIPT_SEND":     {"id", "to"},
	"QBO.SALES.CREDIT_MEMO_PDF":  {"id", "out"},
	"QBO.SALES.ESTIMATE_PDF":     {"id", "out"},
	"QBO.SALES.RECEIPT_PDF":      {"id", "out"},
	"QBO.SALES.INVOICE_PDF":      {"id", "out"},
	"QBO.SALES.INVOICE_PROGRESS": {"estimate", "percent", "id"},
	"QBO.SALES.INVOICE_REMIND":   {"id", "send", "automatic", "to"},
	// Time-invoice create drafts an invoice from uninvoiced activities.
	"QBO.SALES.TIME_INVOICE_CREATE": {"customer", "auto"},
	// v4 GraphQL transaction-form paths (no v3 entity on this build).
	"QBO.SALES.CREDIT_CARD_CREDIT_CREATE": {"credit-card-account", "cc-account", "account", "date", "txn-date", "category-account", "line-items", "lines"},
	"QBO.SALES.DELAYED_CHARGE_CREATE":     {"customer", "charge-date", "date", "txn-date", "line-items", "lines"},
	"QBO.SALES.DELAYED_CREDIT_CREATE":     {"customer", "credit-date", "date", "txn-date", "line-items", "lines"},
	"QBO.SALES.STATEMENT_CREATE":          {"customer", "start-date", "start", "end-date", "end", "type", "format"},
	// taxservice taxcode create.
	"QBO.TAX.CODE_CREATE": {"name", "rates-json"},
}

// dedicatedScopeExcluded pins the wired mutation catalog entries that resolve
// to subsystem-local commands outside this contract's scope — bank-feed ops,
// relay GraphQL mutations, report/forecast builders, proposals, tasks,
// workflows, and cost groups. Each binds and reads its own flag surface; the
// coverage test requires every wired mutation to be classified here or in a
// contract table, so a new dedicated command fails until someone audits it.
var dedicatedScopeExcluded = map[string]string{
	"QBO.FEED.REC_AUTO_ADJUST":             "bank-feed rec auto-adjust — feed plumbing",
	"QBO.FEED.RULE_CREATE":                 "bank-feed rule create — feed plumbing",
	"QBO.FEED.RULE_DELETE":                 "bank-feed rule delete — feed plumbing",
	"QBO.FEED.RULE_EDIT":                   "bank-feed rule update — feed plumbing",
	"QBO.FEED.TXN_ATTACH":                  "bank-feed txn attach — feed plumbing",
	"QBO.FEED.TXN_BATCH_ACCEPT":            "bank-feed batch accept — feed plumbing",
	"QBO.FEED.TXN_CATEGORISE":              "bank-feed categorise — feed plumbing",
	"QBO.FEED.TXN_EXCLUDE":                 "bank-feed exclude — feed plumbing",
	"QBO.FEED.TXN_IMPORT":                  "bank-feed import — feed plumbing",
	"QBO.FEED.TXN_MATCH":                   "bank-feed match — feed plumbing",
	"QBO.FEED.TXN_SPLIT":                   "bank-feed split — feed plumbing",
	"QBO.FEED.TXN_TRANSFER":                "bank-feed transfer — feed plumbing",
	"QBO.FEED.TXN_UNDO_EXCLUDED":           "bank-feed undo-excluded — feed plumbing",
	"QBO.FEED.TXN_UNPOST":                  "bank-feed unpost — feed plumbing",
	"QBO.GQL.MUTATE_ACTIVATE_PRODUCTS":     "relay-tab GraphQL mutation — variables passthrough",
	"QBO.GQL.MUTATE_ASSIGN_DIMENSIONS":     "relay-tab GraphQL mutation — variables passthrough",
	"QBO.GQL.MUTATE_BANK_DISCONNECT":       "relay-tab GraphQL mutation — variables passthrough",
	"QBO.GQL.MUTATE_BATCH_UPDATE_PRODUCTS": "relay-tab GraphQL mutation — variables passthrough",
	"QBO.GQL.MUTATE_CANCEL_RECONCILE":      "relay-tab GraphQL mutation — variables passthrough",
	"QBO.GQL.MUTATE_CONSUME_INVENTORY":     "relay-tab GraphQL mutation — variables passthrough",
	"QBO.GQL.MUTATE_CREATE_ACCOUNT":        "relay-tab GraphQL mutation — variables passthrough",
	"QBO.GQL.MUTATE_RECEIVE_INVENTORY":     "relay-tab GraphQL mutation — variables passthrough",
	"QBO.REPORTS.CUSTOM_CREATE":            "report-builder saved report — reports plumbing",
	"QBO.REPORTS.FORECAST_CREATE":          "forecast builder — reports plumbing",
	"QBO.REPORTS.FORECAST_DELETE":          "forecast builder — reports plumbing",
	"QBO.REPORTS.FORECAST_EDIT":            "forecast builder — reports plumbing",
	"QBO.REPORTS.MANAGEMENT_CREATE":        "management report builder — reports plumbing",
	"QBO.REPORTS.MANAGEMENT_EDIT":          "management report builder — reports plumbing",
	"QBO.REPORTS.PERFORMANCE_CREATE":       "performance report builder — reports plumbing",
	"QBO.REPORTS.PERFORMANCE_DELETE":       "performance report builder — reports plumbing",
	"QBO.REPORTS.PERFORMANCE_EDIT":         "performance report builder — reports plumbing",
	"QBO.REPORTS.REPORT_CREATE":            "report builder — reports plumbing",
	"QBO.REPORTS.REPORT_DELETE":            "report builder — reports plumbing",
	"QBO.REPORTS.REPORT_EDIT":              "report builder — reports plumbing",
	"QBO.CUSTOMERS.PROPOSAL_CREATE":        "proposal builder — proposals plumbing",
	"QBO.CUSTOMERS.PROPOSAL_DELETE":        "proposal builder — proposals plumbing",
	"QBO.CUSTOMERS.PROPOSAL_EDIT":          "proposal builder — proposals plumbing",
	"QBO.CUSTOMERS.PROPOSAL_SVC_CREATE":    "proposal-svc builder — proposals plumbing",
	"QBO.ADVANCED.TASKS_CREATE":            "tasks builder — advanced plumbing",
	"QBO.ADVANCED.TASKS_DELETE":            "tasks builder — advanced plumbing",
	"QBO.ADVANCED.TASKS_EDIT":              "tasks builder — advanced plumbing",
	"QBO.ADVANCED.WORKFLOWS_CREATE":        "workflow builder — advanced plumbing",
	"QBO.ADVANCED.WORKFLOWS_EDIT":          "workflow builder — advanced plumbing",
	"QBO.COSTGROUPS.GROUP_CREATE":          "cost-group builder — customobjects plumbing",
	"QBO.COSTGROUPS.GROUP_DELETE":          "cost-group builder — customobjects plumbing",
	"QBO.COSTGROUPS.GROUP_UPDATE":          "cost-group builder — customobjects plumbing",
}

// mutationInlineCommands are wired mutations registered from inline
// primitiveEntry literals (inventory.go) rather than catalogPrimitives — the
// coverage test iterates these alongside the catalog so inline additions are
// audited too. Map: command string → catalog-style ID.
var mutationInlineCommands = map[string]string{
	"inventory adjust delete":                 "QBO.INVENTORY.ADJUST_DELETE",
	"inventory count create":                  "QBO.INVENTORY.COUNT_CREATE",
	"inventory buildassembly create":          "QBO.INVENTORY.BUILD_ASSEMBLY_CREATE",
	"inventory startingvalue create":          "QBO.INVENTORY.STARTING_VALUE_CREATE",
	"inventory item-receipt create":           "QBO.INVENTORY.ITEM_RECEIPT_CREATE",
	"inventory item-receipt update":           "QBO.INVENTORY.ITEM_RECEIPT_EDIT",
	"inventory purchase-order update partial": "QBO.INVENTORY.PURCHASE_ORDER_PARTIAL",
}

// mutationMetaParams are catalog-declared params that no builder consumes —
// each is an intentional documented gap, not a silent bug. When the flag is
// set, client.ValidateMutationFlags rejects it with the reason below.
var mutationMetaParams = map[string]map[string]string{
	// An opening balance is a journal entry, not a Customer field — the
	// param stays declared for discoverability and is denied at run time.
	"QBO.CUSTOMERS.CUSTOMER_CREATE": {
		"opening-balance": "opening balances post as journal entries (accounting opening-balance create); not a Customer field",
	},
	// Sparse update is implicit; the catalog's field-selector is a no-op.
	"QBO.CUSTOMERS.CUSTOMER_EDIT": {
		"fields": "field selection is not implemented — update is always sparse on the flags you set",
	},
	// TimeActivity has no work-type field in v3.
	"QBO.PAYROLL.TIMESHEET_CREATE": {
		"work-type": "v3 TimeActivity has no work-type field",
	},
	// v3 Employee has no termination-reason field (--date wires
	// TerminationDate).
	"QBO.PAYROLL.EMPLOYEE_TERMINATE": {
		"reason": "v3 Employee has no termination-reason field; --date sets TerminationDate",
	},
	// Cheque printing is a separate post-create op, not a create field.
	"QBO.EXPENSES.CHEQUE_CREATE": {
		"print": "cheque printing is a separate post-create operation (not a v3 Purchase field)",
	},
	// PO email send is a follow-up op, not part of the create body.
	"QBO.INVENTORY.PURCHASE_ORDER_CREATE": {
		"send": "purchase orders are emailed by a follow-up send op, not during create",
	},
	// PO status transitions are not mapped to a sparse field.
	"QBO.INVENTORY.PURCHASE_ORDER_EDIT": {
		"status": "purchase-order status transitions are not a sparse v3 field",
	},
	// Remittance advice is a separate send op after the payment posts.
	"QBO.EXPENSES.BILL_PAY": {
		"remittance": "remittance advice is sent by a follow-up op after the payment posts",
	},
	// Item CategoryRef/TaxCodeRef are not wired in the item builder.
	"QBO.INVENTORY.ITEM_CREATE": {
		"category": "item CategoryRef is not wired in the create builder",
		"tax":      "item TaxCodeRef is not wired in the create builder",
	},
	// Item Type cannot change on a sparse update (it is copied forward).
	"QBO.INVENTORY.ITEM_EDIT": {
		"type": "item Type cannot be changed on an existing item (v3 rejects it)",
	},
	// Invoices do not deposit into an account — payments do that.
	"QBO.SALES.INVOICE_CREATE": {
		"deposit-to": "invoices have no deposit account; use sales payment create --deposit-to",
	},
	// The write-off posts against the credit memo's line item; --account is
	// declared but has no wire target today.
	"QBO.SALES.INVOICE_WRITE_OFF": {
		"account": "write-off expense account is not wired — the credit memo carries the invoice's own item",
	},
	// Recorded on the dry-run plan envelope only — the captured schedule
	// mutation takes --data verbatim and has no source-id field.
	"QBO.ACCOUNTING.DEFERRED_RECOGNITION_CREATE": {
		"source-id": "recorded in the plan envelope only; the schedule body comes from --data verbatim",
	},
	"QBO.ACCOUNTING.PREPAID_CREATE": {
		"source-id": "recorded in the plan envelope only; the schedule body comes from --data verbatim",
	},
	// The receipt upload path posts files to financialdocument v2/documents;
	// the declared --source enum is UI provenance, not a wire field.
	"QBO.EXPENSES.RECEIPT_UPLOAD": {
		"source": "upload source is UI provenance only — the documents endpoint does not read it",
	},
	// Batch invoice creation is a QuickBooks Online Advanced surface the
	// invoice importer does not implement.
	"QBO.SALES.INVOICE_LIST_CREATE": {
		"batch": "batch create is QBO Advanced-only; the importer posts --file rows individually",
	},
	// The trips GraphQL mutation has no employee field — trips are recorded
	// for the signed-in driver.
	"QBO.EXPENSES.MILEAGE_CREATE": {
		"employee": "the trips mutation has no employee field — trips belong to the signed-in user",
	},
	// Backup runs immediately on invocation; no confirmation gate is wired.
	"QBO.ADVANCED.BACKUP_CREATE": {
		"confirm": "backup create runs immediately — there is no confirmation gate on this path",
	},
}

// mutationAliasGroups are flag-name equivalence classes: when the contract
// says a flag is consumed, attaching ANY member of its group on the command
// satisfies it (e.g. --txn-date attached means --bill-date needn't be). Each
// group mirrors a firstFlag-style alias read in the builders.
var mutationAliasGroups = [][]string{
	{"date", "txn-date", "invoice-date", "bill-date", "purchase-date",
		"quote-date", "payment-date", "refund-date", "charge-date", "credit-date",
		"order-date", "sales-order-date"},
	{"name", "title"},
	{"supplier", "payee", "payee-id", "vendor"},
	{"category", "category-id"},
	{"customer", "customer-id", "client-id"},
	{"item", "item-id"},
	{"line-items", "lines", "items", "items-json"},
	{"address", "line1"},
	{"postcode", "postal"},
	{"memo", "notes", "note", "message", "customer-note"},
	{"discount", "discount-percent", "percent"},
	{"qty", "quantity", "new-qty", "initial-qty"},
	{"due-days", "due_days", "duedays"},
	{"subtype", "sub-type"},
	{"agency", "agency-id"},
	{"time-activity", "time-activity-id"},
	{"template-type", "recur-type", "recurring-type"},
	{"interval", "interval-type"},
	{"days-in-advance", "days-before"},
	{"doc-number", "number", "order-number", "order-no"},
	{"bill-id", "bill-ids"},
	{"deposit-to", "refund-from", "account"},
	{"payment-account", "bank-account", "account", "deposit-to", "refund-from"},
	{"expense-account", "cogs-account"},
	{"asset-account", "inventory-asset-account"},
	{"category-account", "account"},
	{"credit-card-account", "cc-account"},
	{"payment-type", "pay-type", "type"},
	{"supplier-credit-ids", "credit-ids"},
	{"email", "to", "send-to"},
	{"term", "terms"},
	{"currency", "currency-iso"},
	{"entity", "txn-type"},
	{"id", "txn-id", "employee"},
	{"id", "receipt-id"},
	{"id", "payment", "payment-id"},
	{"id", "account-id", "account"},
	{"id", "template-id"},
	{"life-years", "useful-life"},
	{"distance", "distance-km"},
	{"file", "path"},
	{"out", "output", "file"},
	{"start", "start-date"},
	{"end", "end-date"},
	{"color", "colour"},
	{"purpose", "description"},
	{"start-date", "fiscal-year"},
}

// mutationAliasSatisfied reports whether a consumed flag name has an attached
// representative on the command: either the name itself or any alias-group
// member.
func mutationAliasSatisfied(name string, attached func(string) bool) bool {
	if attached(name) {
		return true
	}
	for _, group := range mutationAliasGroups {
		inGroup := slices.Contains(group, name)
		if !inGroup {
			continue
		}
		if slices.ContainsFunc(group, attached) {
			return true
		}
	}
	return false
}

// mutationMetaSet returns the documented-dead params for a catalog ID as the
// deny map ValidateMutationFlags expects.
func mutationMetaSet(id string) map[string]string {
	return mutationMetaParams[id]
}

// mutationDeclaredAllowance returns the declared params minus the meta set —
// command-local plumbing a catalog entry may legitimately consume outside
// ReplayMutate (kept permissive; the contract test pins declared ⊆ consumed ∪
// meta so this cannot hide drift).
func mutationDeclaredAllowance(id string, declared []paramDoc) map[string]bool {
	out := map[string]bool{}
	meta := mutationMetaParams[id]
	for _, p := range declared {
		if _, dead := meta[p.Name]; !dead {
			out[p.Name] = true
		}
	}
	return out
}

// mutationAttachedNames lists the local flags a resolved command carries.
func mutationAttachedNames(cmd *cobra.Command) map[string]bool {
	out := map[string]bool{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) { out[f.Name] = true })
	return out
}

// attachDedicatedConsumed binds every flag in the command's dedicatedConsumed
// contract entry — the dedicated half of "consumed ⇒ attached". Dedicated
// constructors call this after attachParamFlags instead of hand-maintaining a
// parallel ensureFlag list (which is how flags like --source/--currency on
// reclassify/currency drifted out of reach).
func attachDedicatedConsumed(cmd *cobra.Command, id string) {
	for _, n := range dedicatedConsumed[id] {
		ensureFlag(cmd, n, "consumed by "+id+" (mutation contract)")
	}
}
