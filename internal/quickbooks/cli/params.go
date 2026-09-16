package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// paramDoc is the agent-facing flag contract for one CLI parameter.
type paramDoc struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // string | strings | int | bool
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
	Help     string `json:"help"`
}

func globalParamDocs() []paramDoc {
	return []paramDoc{
		{Name: "json", Type: "bool", Help: "machine-readable JSON on stdout; never prints secrets"},
		{Name: "home", Type: "string", Help: "qb config home (default $QB_HOME or ~/.config/qb)"},
		{Name: "timeout", Type: "string", Default: "10m", Help: "overall command timeout"},
		{Name: "relay-url", Type: "string", Help: "Chrome DevTools relay URL (default $QB_RELAY_URL or http://127.0.0.1:9224)"},
		{Name: "dry-run", Type: "bool", Help: "plan only; make no changes"},
		{Name: "yes", Type: "bool", Help: "assume yes to prompts"},
		{Name: "no-input", Type: "bool", Help: "never prompt"},
		{Name: "quiet", Type: "bool", Help: "suppress non-essential output"},
		{Name: "audit-dir", Type: "string", Help: "audit output directory"},
	}
}

func p(name, typ, help string, req bool, def string) paramDoc {
	return paramDoc{Name: name, Type: typ, Required: req, Default: def, Help: help}
}

// paramsFor returns the intended flags for a primitive. Wired commands honour
// these; blocked stubs register them so --help is complete, then still return
// ErrMutationNotWired. Stub flags come from AU learn-support extracts when present.
func paramsFor(id string) []paramDoc {
	if ps, ok := salesOrderParamDocs(id); ok {
		return ps
	}
	if spec, ok := v3ByID[id]; ok && spec.Report != "" {
		// UI catalog help has required flags and dimensions that the report
		// reader does not implement. Keep the executable contract authoritative.
		return reportParamDocs(id, spec.Report)
	}
	var out []paramDoc
	if w, ok := wiredParams(id); ok {
		out = mergeParamHelp(w, firstParams(id))
	} else if v, ok := v3UniformParams(id); ok {
		out = mergeParamHelp(v, firstParams(id))
	} else if au := firstParams(id); len(au) > 0 {
		// The catalog owns these slices; help normalization must not mutate
		// shared entries while other command trees are being constructed.
		out = slices.Clone(au)
	} else {
		out = inferredParams(id)
	}
	return canonicalTimeHelp(canonicalFileHelp(canonicalLineItemsHelp(out, id), id), id)
}

func canonicalTimeHelp(ps []paramDoc, id string) []paramDoc {
	if id != "QBO.EXPENSES.TIME_ACTIVITY_CREATE" && id != "QBO.EXPENSES.TIME_ACTIVITY_EDIT" && id != "QBO.PAYROLL.TIMESHEET_CREATE" && id != "QBO.PAYROLL.TIMESHEET_EDIT" {
		return ps
	}
	for _, field := range []paramDoc{
		p("hours", "string", "non-negative whole hours; explicit zero is preserved", false, ""),
		p("minutes", "string", "separate native minutes, 0 through 59; explicit zero is preserved", false, ""),
		p("rate", "string", "finite non-negative hourly rate; defaults to zero on create, preserved when omitted on update", false, ""),
		p("billable", "string", "true or false for the native TimeActivity billing status", false, ""),
	} {
		found := false
		for i := range ps {
			if ps[i].Name == field.Name {
				ps[i].Help = field.Help
				found = true
				break
			}
		}
		if !found {
			ps = append(ps, field)
		}
	}
	return ps
}

func reportParamDocs(id, report string) []paramDoc {
	ps := []paramDoc{
		p("report", "string", "report API name (omit to use this command's default)", false, report),
		p("date-range", "string", "optional period start,end as YYYY-MM-DD,YYYY-MM-DD; omit for the API default", false, ""),
		p("accounting-method", "string", "Cash or Accrual basis, sent as accounting_method (omit for the API default)", false, ""),
		p("columns", "string", "column grouping sent as summarize_column_by (Total, Month, Quarter, Year; monthly is an alias for Month); comparison/detail-field lists are unsupported", false, ""),
		p("id", "string", "unsupported for reports; explicit use is rejected (use --report to select a report)", false, ""),
		p("limit", "int", "unsupported for reports; explicit use is rejected (reports are not paginated entity lists)", false, "20"),
	}
	if report == "TAXABLE_PAYMENTS" {
		ps[2].Help = "unsupported by TAXABLE_PAYMENTS; explicit non-empty use is rejected"
		ps[3].Help = "unsupported by TAXABLE_PAYMENTS; explicit non-empty use is rejected"
	}
	if id == "QBO.FEED.REC_REPORT" {
		ps = append(ps, p("account-id", "string", "unsupported for this company-wide report; explicit use is rejected", false, "204"))
	}
	return ps
}

// canonicalLineItemsHelp replaces any line-items/lines help with the JSON
// truth: only a JSON array of v3 line objects is honored (CSV rows are
// rejected by client.CheckLineItemsJSON). The generated AU texts promising
// CSV rows are wrong and must not reach agents. Exempted: feed split lines,
// which are genuinely comma-separated categoryId=amount pairs parsed by
// parseSplitLines — overwriting them would lie in the other direction.
func canonicalLineItemsHelp(ps []paramDoc, id string) []paramDoc {
	for i, prm := range ps {
		if prm.Name == "line-items" || (prm.Name == "lines" && id != "QBO.FEED.TXN_SPLIT") {
			ps[i].Help = "JSON array of v3 line objects (CSV rows are rejected, never coerced); set TaxCodeRef per line for GST; verify with --dry-run before posting"
		}
	}
	return ps
}

// canonicalFileHelp scopes bank-feed import help to the upload truth: only CSV
// bank-statement files are staged via uploadCsvFile (text/csv). The generated
// AU texts promising XLSX/PDF describe the UI wizard, which the CLI does not
// drive, and must not reach agents.
func canonicalFileHelp(ps []paramDoc, id string) []paramDoc {
	if id != "QBO.FEED.TXN_IMPORT" {
		return ps
	}
	for i, prm := range ps {
		if prm.Name == "file" {
			ps[i].Help = "CSV bank-statement file staged via uploadCsvFile then processed row by row (other formats are UI-only)"
		}
	}
	return ps
}

// v3UniformParams is the honest flag contract for generic v3-query reads:
// every such command honours exactly id/query/active/limit via
// client.ReplayQuery, so UI filter dimensions beyond these (status,
// supplier, sku, ...) must not appear in the runnable contract. Search
// verbs require --query; gets accept all four optionally.
func v3UniformParams(id string) ([]paramDoc, bool) {
	spec, ok := v3ByID[id]
	if !ok || spec.Entity == "" {
		return nil, false
	}
	e, ok := catalogByID(id)
	if !ok {
		return nil, false
	}
	verb := ""
	if f := strings.Fields(e.Command); len(f) > 0 {
		verb = f[len(f)-1]
	}
	queryReq := verb == "search"
	return []paramDoc{
		p("query", "string", "substring match on name or doc number (case-insensitive)", queryReq, ""),
		p("id", "string", "v3 entity id to fetch a single record (omit to list)", false, ""),
		p("active", "string", "v3 Active filter: true, false, or all (include inactive)", false, ""),
		p("limit", "int", "max rows to return from the v3 query endpoint", false, "20"),
	}, true
}

func firstParams(id string) []paramDoc {
	if au := auParamAll[id]; len(au) > 0 {
		return au
	}
	if au := auParamOverlay[id]; len(au) > 0 {
		return au
	}
	return auParamGaps[id]
}

func mergeParamHelp(base, extra []paramDoc) []paramDoc {
	if len(extra) == 0 {
		return base
	}
	by := make(map[string]paramDoc, len(extra))
	for _, e := range extra {
		by[e.Name] = e
	}
	out := make([]paramDoc, len(base))
	for i, p := range base {
		if e, ok := by[p.Name]; ok && e.Help != "" {
			p.Help = e.Help
		}
		out[i] = p
	}
	return out
}

func wiredParams(id string) ([]paramDoc, bool) {
	switch id {
	case "QBO.FEED.ACCOUNT_LIST":
		return nil, true
	case "QBO.FEED.TXN_PENDING", "QBO.FEED.TXN_POSTED", "QBO.FEED.TXN_EXCLUDED":
		return []paramDoc{
			p("account-id", "string", "banking account id (see feed account list)", false, "204"),
			p("limit", "int", "max rows to fetch (X-Range page)", false, "20"),
		}, true
	case "QBO.FEED.TXN_POPULATION":
		// Population runs a full completeness-checked walk; --limit is
		// intentionally not a flag (cobra omits it), so the catalog
		// must not advertise it.
		return []paramDoc{
			p("account-id", "string", "connected account to check (see feed account list)", false, "204"),
		}, true
	case "QBO.FEED.TXN_VERIFY":
		return []paramDoc{
			p("account-id", "string", "banking account id to completeness-check against the server count", false, "204"),
		}, true
	case "QBO.FEED.TXN_POPULATION_ALL":
		return []paramDoc{
			p("max-pages-per-account", "int", "page ceiling per account at 300 rows per page", false, "40"),
		}, true
	case "QBO.ACCOUNTING.PREPAID_READ":
		// Schedule fetch keyed solely by --source-id; the cobra command
		// defines no --limit/--id.
		return []paramDoc{
			p("source-id", "string", "source transaction id carrying the schedule (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.BUDGET_LIST":
		return []paramDoc{
			p("limit", "int", "max budgets to fetch from the budgeting backend", false, "20"),
		}, true
	case "QBO.ACCOUNTING.CLASS_DELETE":
		return []paramDoc{
			p("id", "string", "target class id for deactivation (required)", true, ""),
			p("active", "string", "set deactivation state explicitly, false deactivates", false, ""),
		}, true
	case "QBO.ACCOUNTING.LOCATION_DELETE":
		return []paramDoc{
			p("id", "string", "target department id for deactivation (required)", true, ""),
			p("active", "string", "set deactivation state explicitly, false deactivates", false, ""),
		}, true
	case "QBO.ACCOUNTING.LOCATION_EDIT":
		return []paramDoc{
			p("id", "string", "target department id for rename (required)", true, ""),
			p("name", "string", "new display name for the department record", false, ""),
			p("active", "string", "set deactivation state explicitly, false deactivates", false, ""),
		}, true
	case "QBO.ACCOUNTING.OPENING_BALANCE_CREATE":
		return []paramDoc{
			p("date", "string", "opening balance date as dd/MM/yyyy or yyyy-MM-dd (required)", true, ""),
			p("lines", "string", "JSON array of account/debit-credit lines from the trial balance (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.PREPAID_CREATE":
		return []paramDoc{
			p("data", "string", "mutation input JSON object for the prepaid schedule (required)", true, ""),
			p("draft", "bool", "preview via GET_SCHEDULE_PREVIEW instead of writing", false, ""),
			p("source-id", "string", "optional source txn id recorded in the plan", false, ""),
		}, true
	case "QBO.ACCOUNTING.DEFERRED_RECOGNITION_GET":
		return []paramDoc{
			p("source-id", "string", "source transaction id carrying the schedule (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.DEFERRED_RECOGNITION_CREATE":
		return []paramDoc{
			p("data", "string", "mutation input JSON object for the schedule (required)", true, ""),
			p("draft", "bool", "preview via GET_SCHEDULE_PREVIEW instead of writing", false, ""),
			p("source-id", "string", "optional source txn id recorded in the plan", false, ""),
		}, true
	case "QBO.EXPENSES.CHEQUE_CREATE":
		// Purchase cash side: --bank-account/--account are aliases (unified
		// in the builder). --category-account stays required: omitting it
		// would silently post lines to the hardcoded default account 7.
		return []paramDoc{
			p("payee", "string", "customer, supplier, or employee the cheque is paid to (required)", true, ""),
			p("bank-account", "string", "bank account the cheque draws from, AU spelling cheque (required)", true, ""),
			p("account", "string", "cash-side alias, usually the same bank account", false, ""),
			p("category-account", "string", "expense/category account id, must differ from the cash side (required)", true, ""),
			p("line-items", "string", "JSON array of v3 line objects (required)", true, ""),
			p("amount", "string", "AUD amount of the cheque (required)", true, ""),
			p("print", "string", "whether to queue for printing on QuickBooks-formatted cheque stock", false, ""),
		}, true
	case "QBO.EXPENSES.EXPENSE_CREATE":
		return []paramDoc{
			p("payee", "string", "supplier, customer, or employee paid (required)", true, ""),
			p("payment-account", "string", "bank or credit card account the expense was paid from (required)", true, ""),
			p("account", "string", "cash-side alias for the payment account id", false, ""),
			p("line-items", "string", "JSON array of v3 line objects (required)", true, ""),
			p("amount", "string", "AUD amount of the expense (required)", true, ""),
			p("billable", "string", "mark a line billable to a customer/project", false, ""),
			p("project", "string", "assign line items to a project", false, ""),
		}, true
	case "QBO.EXPENSES.BILL_PAYMENT_DELETE":
		return []paramDoc{
			p("id", "string", "target bill-payment id to delete, unapplying it from the bill (required)", true, ""),
		}, true
	case "QBO.SALES.CREDIT_CARD_CREDIT_CREATE":
		// Purchase-backed like cheque/expense: the builder demands the
		// cash side and a distinct category account at runtime.
		return []paramDoc{
			p("credit-card-account", "string", "credit card account the credit is recorded against (required)", true, ""),
			p("date", "string", "date of the credit as dd/MM/yyyy (required)", true, ""),
			p("account", "string", "cash-side account id, usually the same card account (required)", true, ""),
			p("category-account", "string", "expense/category account id, must differ from the cash side (required)", true, ""),
			p("line-items", "string", "JSON array of v3 line objects (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.JOURNAL_CREATE":
		// Bulk-create path: one --items-json array, not per-entry flags.
		return []paramDoc{
			p("items-json", "string", "JSON array of {date, amount, from-account, to-account, memo} (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.JOURNAL_DELETE":
		// Bulk-delete path: entry ids via --ids, not a single --id.
		return []paramDoc{
			p("ids", "strings", "journal entry ids to bulk-delete via v3 batch (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.AUDIT_LOG_SEARCH":
		// The search command filters by event-type/user/query only;
		// the backend has no date-range dimension.
		return []paramDoc{
			p("query", "string", "substring match on type/name/user", true, ""),
			p("event-type", "string", "filter by event type (LOGIN, VOID, CREATED)", false, ""),
			p("user", "string", "filter by user id", false, ""),
			p("limit", "int", "max events to return", false, "20"),
		}, true
	case "QBO.FEED.TXN_LOOKUP":
		return []paramDoc{
			p("query", "string", "substring match on id, olbTxnId, or description (hyphens fold to spaces)", true, ""),
			p("account-id", "string", "banking account id", false, "204"),
			p("limit", "int", "max rows per review state", false, "20"),
		}, true
	case "QBO.FEED.TXN_EXCLUDE", "QBO.FEED.TXN_UNDO_EXCLUDED":
		return []paramDoc{
			p("ids", "strings", "downloaded olbTxnIds only — not display ids like 32371:ofx, not manually added rows", true, ""),
			p("account-id", "string", "connected bank/credit-card account that owns the feed row", false, "204"),
		}, true
	case "QBO.FEED.TXN_CATEGORISE":
		return []paramDoc{
			p("ids", "strings", "pending downloaded olbTxnIds to categorise and post — never display ids like 32371:ofx", true, ""),
			p("category", "string", "account/category id to post to — drives P&L and BAS/GST reporting (e.g. 7)", true, ""),
			p("class", "string", "class id to assign (optional; classes segment reports by department or location)", false, ""),
			p("entity-type", "string", "QBO entity created for the row (default Expense; Deposit for income rows)", false, "Expense"),
			p("account-id", "string", "connected bank/credit-card account that owns the feed row", false, "204"),
		}, true
	case "QBO.FEED.TXN_MATCH":
		return []paramDoc{
			p("ids", "strings", "pending downloaded olbTxnIds to match — never display ids like 32371:ofx", true, ""),
			p("match-id", "strings", "existing QuickBooks record ids to match against (repeatable or comma-separated)", true, ""),
			p("txn-type", "string", "QBO record type of the match targets (Bill, Expense, Deposit, Cheque)", false, "Bill"),
			p("account-id", "string", "connected bank/credit-card account that owns the feed row", false, "204"),
		}, true
	case "QBO.FEED.TXN_SPLIT":
		return []paramDoc{
			p("ids", "strings", "pending downloaded olbTxnIds to split — never display ids like 32371:ofx", true, ""),
			p("lines", "string", "comma-separated categoryId=amount pairs; amounts must sum to the row total", true, ""),
			p("entity-type", "string", "QBO entity created for the row (default Expense; Deposit for income rows)", false, "Expense"),
			p("account-id", "string", "connected bank/credit-card account that owns the feed row", false, "204"),
		}, true
	case "QBO.FEED.TXN_BATCH_ACCEPT":
		return []paramDoc{
			p("ids", "strings", "pending downloaded olbTxnIds to accept in bulk — never display ids like 32371:ofx", true, ""),
			p("account-id", "string", "connected bank/credit-card account that owns the feed row", false, "204"),
		}, true
	case "QBO.FEED.TXN_IMPORT":
		return []paramDoc{
			p("account-id", "string", "CSV-import account only (209). Refuses live OAuth 204 and 93. Cannot import to a subaccount", true, ""),
			p("file", "string", "CSV bank-statement file staged via uploadCsvFile then processed row by row (optional)", false, ""),
			p("description", "string", "bank description for single-row import without --file (label test rows)", true, ""),
			p("amount", "string", "numeric amount for single-row import; income positive, expense negative", true, ""),
			p("date", "string", "dd/MM/yyyy AU locale (default today Australia/Sydney)", false, ""),
		}, true
	case "QBO.ACCOUNTING.REGISTER_READ":
		return []paramDoc{
			p("account-id", "string", "GL/bank account id. Register is booked-state ground truth (posted, not For review)", false, "204"),
			p("limit", "int", "max register rows", false, "20"),
		}, true
	case "QBO.REPORTS.AGED_PAYABLES_READ", "QBO.REPORTS.AGED_RECEIVABLES_READ", "QBO.REPORTS.GENERAL_LEDGER_READ", "QBO.REPORTS.TRIAL_BALANCE_READ", "QBO.REPORTS.PROFIT_LOSS_READ", "QBO.REPORTS.BALANCE_SHEET_READ", "QBO.REPORTS.VENDOR_BALANCE_READ", "QBO.REPORTS.CUSTOMER_BALANCE_READ", "QBO.REPORTS.CUSTOMER_SALES_READ", "QBO.REPORTS.VENDOR_EXPENSES_READ":
		return []paramDoc{
			p("report", "string", "v3 report name: AgedPayables, AgedReceivables, GeneralLedger, or TrialBalance (required)", true, ""),
			p("date-range", "string", "report period start,end as YYYY-MM-DD,YYYY-MM-DD (required)", true, ""),
			p("id", "string", "target report id (accepted, not forwarded to v3 reports API)", false, ""),
			p("limit", "int", "max rows (accepted for catalog parity; reports are not list endpoints)", false, "20"),
		}, true
	case "QBO.PAYROLL.EMPLOYEE_CREATE":
		return []paramDoc{
			p("name", "string", "employee display name, usually GivenName plus FamilyName combined (required)", true, ""),
			p("given-name", "string", "employee given name written to the v3 GivenName field (optional)", false, ""),
			p("family-name", "string", "employee family name written to the v3 FamilyName field (optional)", false, ""),
			p("email", "string", "employee primary email address recorded as PrimaryEmailAddr (optional)", false, ""),
		}, true
	case "QBO.PAYROLL.EMPLOYEE_EDIT":
		return []paramDoc{
			p("id", "string", "target employee id to rename via sparse update (required)", true, ""),
			p("name", "string", "new display name written to the v3 DisplayName field (optional)", false, ""),
		}, true
	case "QBO.COMPANY.TERM_CREATE":
		return []paramDoc{
			p("name", "string", "payment term display name shown on sales forms, e.g. Net 30 (required)", true, ""),
			p("type", "string", "term type Standard or DateDriven sent as v3 Type (optional)", false, ""),
			p("due-days", "string", "net days due sent as v3 DueDays; server 400s Standard terms without it (required)", true, ""),
		}, true
	case "QBO.COMPANY.PAYMENT_METHOD_CREATE":
		return []paramDoc{
			p("name", "string", "payment method display name shown on sales forms (required)", true, ""),
		}, true
	case "QBO.COMPANY.TERM_EDIT", "QBO.COMPANY.PAYMENT_METHOD_EDIT":
		return []paramDoc{
			p("id", "string", "target term or payment-method id to rename (required)", true, ""),
			p("name", "string", "new display name for the term or payment method (optional)", false, ""),
		}, true
	case "QBO.COMPANY.COMPANY_INFO_EDIT":
		return []paramDoc{
			p("id", "string", "company-info id from company company-info get, used to fetch SyncToken (required)", true, ""),
			p("name", "string", "new legal or trading name written to the v3 CompanyName field (optional)", false, ""),
		}, true
	default:
		return nil, false
	}
}

func inferredParams(id string) []paramDoc {
	suffix := id
	if i := strings.LastIndex(id, "."); i >= 0 {
		suffix = id[i+1:]
	}
	var out []paramDoc
	switch {
	case strings.HasSuffix(suffix, "_SEARCH") || suffix == "LOOKUP" || suffix == "SEARCH":
		out = append(out, p("query", "string", "search text", true, ""))
		out = append(out, p("limit", "int", "max rows", false, "20"))
	case strings.HasSuffix(suffix, "_READ"):
		out = append(out, p("limit", "int", "max rows when listing", false, "20"))
	case strings.HasSuffix(suffix, "_DELETE") || strings.HasSuffix(suffix, "_DEACTIVATE") ||
		strings.HasSuffix(suffix, "_EDIT") || strings.Contains(suffix, "_LIST_EDIT") ||
		strings.Contains(suffix, "_FORM_DELETE") || suffix == "UNPOST" || suffix == "VOID" ||
		suffix == "UNDO_EXCLUDED":
		out = append(out, p("id", "string", "target QBO txn/record id (not a display :ofx id)", true, ""))
	}

	switch {
	case strings.Contains(suffix, "INVOICE") || strings.Contains(suffix, "ESTIMATE") ||
		strings.Contains(suffix, "SALES_RECEIPT") || strings.Contains(suffix, "CREDIT_MEMO") ||
		strings.Contains(suffix, "REFUND") || strings.Contains(suffix, "SALES_ORDER") ||
		strings.Contains(suffix, "STATEMENT") || strings.Contains(suffix, "DELAYED"):
		if strings.Contains(suffix, "CREATE") || strings.Contains(suffix, "EDIT") {
			out = append(out,
				p("customer", "string", "customer display name or id", true, ""),
				p("date", "string", "dd/MM/yyyy", false, ""),
				p("amount", "string", "line amount (AUD)", false, ""),
				p("item", "string", "product/service name", false, ""),
				p("memo", "string", "memo / message on statement", false, ""),
			)
		}
	case strings.Contains(suffix, "EXPENSE") || strings.Contains(suffix, "BILL") ||
		strings.Contains(suffix, "CHEQUE") || strings.Contains(suffix, "VENDOR_CREDIT") ||
		strings.Contains(suffix, "MILEAGE") || strings.Contains(suffix, "PAY_BILLS"):
		if strings.Contains(suffix, "CREATE") || strings.Contains(suffix, "EDIT") || suffix == "PAY_BILLS" {
			out = append(out,
				p("payee", "string", "supplier/payee name", false, ""),
				p("account", "string", "category / expense account", false, ""),
				p("payment-account", "string", "bank/credit account paid from", false, ""),
				p("date", "string", "dd/MM/yyyy", false, ""),
				p("amount", "string", "AUD amount", true, ""),
				p("memo", "string", "memo", false, ""),
			)
		}
	case strings.Contains(suffix, "JOURNAL") || strings.Contains(suffix, "TRANSFER") ||
		strings.Contains(suffix, "DEPOSIT"):
		if strings.Contains(suffix, "CREATE") || strings.Contains(suffix, "EDIT") {
			out = append(out,
				p("date", "string", "dd/MM/yyyy", false, ""),
				p("amount", "string", "AUD amount (journal must balance)", true, ""),
				p("from-account", "string", "source account (transfer/deposit)", false, ""),
				p("to-account", "string", "destination account", false, ""),
				p("memo", "string", "memo", false, ""),
			)
		}
	case strings.Contains(suffix, "RECEIVE_PAYMENT"):
		if strings.Contains(suffix, "CREATE") || strings.Contains(suffix, "EDIT") {
			out = append(out,
				p("customer", "string", "customer name", true, ""),
				p("amount", "string", "payment amount", true, ""),
				p("date", "string", "dd/MM/yyyy", false, ""),
				p("deposit-to", "string", "undeposited funds or bank account", false, ""),
			)
		}
	case strings.Contains(id, "CUSTOMERS.") && (strings.Contains(suffix, "CREATE") || strings.Contains(suffix, "EDIT")):
		out = append(out,
			p("name", "string", "customer display name", true, ""),
			p("email", "string", "email", false, ""),
			p("phone", "string", "phone", false, ""),
		)
	case strings.Contains(suffix, "SUPPLIER") && (strings.Contains(suffix, "CREATE") || strings.Contains(suffix, "EDIT")):
		out = append(out,
			p("name", "string", "supplier display name", true, ""),
			p("email", "string", "email", false, ""),
		)
	case strings.Contains(suffix, "ITEM") && (strings.Contains(suffix, "CREATE") || strings.Contains(suffix, "EDIT")):
		out = append(out,
			p("name", "string", "product/service name", true, ""),
			p("type", "string", "Service | NonInventory | Inventory", false, "Service"),
			p("income-account", "string", "income account name", false, ""),
			p("rate", "string", "sales price", false, ""),
		)
	case strings.Contains(suffix, "IMPORT"):
		out = append(out, p("file", "string", "path to .csv/.xlsx", true, ""))
	case strings.HasPrefix(id, "QBO.TAX.") && !strings.HasSuffix(suffix, "_READ"):
		out = append(out, p("period", "string", "BAS/GST period (e.g. 2026-Q1)", false, ""))
	case strings.HasPrefix(id, "QBO.PAYROLL.") && strings.Contains(suffix, "CREATE"):
		out = append(out,
			p("employee", "string", "employee name", false, ""),
			p("date", "string", "dd/MM/yyyy", false, ""),
			p("amount", "string", "gross/net as required", false, ""),
		)
	}

	if strings.HasPrefix(id, "QBO.FEED.") && suffix != "READ_ACCOUNTS" {
		hasAcct := false
		for _, x := range out {
			if x.Name == "account-id" {
				hasAcct = true
			}
		}
		if !hasAcct {
			out = append([]paramDoc{p("account-id", "string", "banking account id", false, "204")}, out...)
		}
	}
	return dedupeParams(out)
}

func dedupeParams(in []paramDoc) []paramDoc {
	seen := map[string]bool{}
	out := make([]paramDoc, 0, len(in))
	for _, x := range in {
		if seen[x.Name] {
			continue
		}
		seen[x.Name] = true
		out = append(out, x)
	}
	return out
}

func usageFor(e primitiveEntry) string {
	if e.Command == "" {
		return ""
	}
	parts := []string{"qb", e.Command}
	for _, p := range paramsFor(e.ID) {
		flag := "--" + p.Name
		if p.Type == "strings" {
			flag += " ID"
		} else if p.Type != "bool" {
			flag += " " + strings.ToUpper(strings.ReplaceAll(p.Name, "-", "_"))
		}
		if !p.Required {
			flag = "[" + flag + "]"
		}
		parts = append(parts, flag)
	}
	return strings.Join(parts, " ")
}

func notesFor(e primitiveEntry) string {
	base := ""
	switch e.Mode {
	case modeWired:
		base = "Live mutation using the captured QBO session. Use record IDs from the corresponding read command."
		if strings.HasPrefix(e.ID, "QBO.FEED.") {
			base += " Bank-feed writes require olbTxnIds, not :ofx display IDs."
		}
	case modeRead:
		base = "Live read via the captured session. Secret-free JSON with --json."
	case modeBlocked:
		base = "Not wired. --dry-run emits a plan and does not POST. Without --dry-run: RequireSession then ErrMutationNotWired."

	case modeExcluded:
		base = "Policy-excluded. Listed only. Not a command."
	}
	if strings.HasPrefix(e.ID, "QBO.SALES.SALES_ORDER_") {
		return base + "\nUses the native sales-order service. Create supports SERVICE lines; updates preserve omitted values and reject unsupported tax, discount, shipping, or linked-line changes."
	}
	if extra := auHelpNote(e.ID); extra != "" {
		if base != "" {
			return base + "\n" + extra
		}
		return extra
	}
	return base
}

func auHelpNote(id string) string {
	if s := auNoteAll[id]; s != "" {
		return s
	}
	if s := auNoteOverlay[id]; s != "" {
		return s
	}
	return auNoteGaps[id]
}

func attachParamFlags(cmd *cobra.Command, id string) {
	params := paramsFor(id)
	if len(params) == 0 {
		return
	}
	for _, p := range params {
		help := p.Help
		if p.Required {
			help += " (required)"
		}
		switch p.Type {
		case "int":
			def := 0
			_, _ = fmt.Sscanf(p.Default, "%d", &def)
			cmd.Flags().Int(p.Name, def, help)
		case "bool":
			cmd.Flags().Bool(p.Name, false, help)
		case "strings":
			cmd.Flags().StringSlice(p.Name, nil, help)
		default:
			cmd.Flags().String(p.Name, p.Default, help)
		}
	}
}

func longHelp(e primitiveEntry) string {
	var b strings.Builder
	b.WriteString(notesFor(e))
	b.WriteString("\n\nUsage:\n  ")
	b.WriteString(usageFor(e))
	params := paramsFor(e.ID)
	if len(params) > 0 {
		b.WriteString("\n\nParameters:")
		for _, p := range params {
			req := ""
			if p.Required {
				req = " required"
			}
			def := ""
			if p.Default != "" {
				def = " default=" + p.Default
			}
			fmt.Fprintf(&b, "\n  --%s (%s%s%s) %s", p.Name, p.Type, req, def, p.Help)
		}
	}
	b.WriteString("\n\nGlobals: --json --home --timeout --relay-url --dry-run --yes --no-input --quiet --audit-dir")
	b.WriteString("\nAgent catalog: qb actions --json")
	return b.String()
}

func catalogByCommand(command string) (primitiveEntry, bool) {
	for _, e := range catalogPrimitives {
		if e.Command == command {
			return e, true
		}
	}
	return primitiveEntry{}, false
}

func catalogByID(id string) (primitiveEntry, bool) {
	for _, e := range catalogPrimitives {
		if e.ID == id {
			return e, true
		}
	}
	return primitiveEntry{}, false
}

func documentedEntry(e primitiveEntry) primitiveEntry {
	e.Usage = usageFor(e)
	e.Params = paramsFor(e.ID)
	e.Notes = notesFor(e)
	e.Globals = globalParamDocs()
	return e
}

func applyCatalogHelp(cmd *cobra.Command, id string) {
	e, ok := catalogByID(id)
	if !ok {
		return
	}
	cmd.Long = longHelp(e)
	cmd.Example = usageFor(e)
}
