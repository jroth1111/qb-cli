package cli

// auParamGaps / auNoteGaps cover primitives added in the 3-level cutover
// and older creates that had notes but no flags. Sourced from AU learn-support.
var auParamGaps = map[string][]paramDoc{
	"QBO.ADVANCED.BATCH_RUN": {
		p("file", "string", "BatchItemRequest JSON array @path or inline, max 25 mixed ops", true, ""),
	},
	"QBO.REPORTS.ACCOUNT_LIST_READ": {
		p("limit", "int", "max account rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.MEMORIZED_RUN": {
		p("id", "string", "memorized report id — composite id or mem_rpt_id (e.g. 42); resolve ids via reports memorized list", true, ""),
		p("date-range", "string", "start,end as YYYY-MM-DD,YYYY-MM-DD; replays the saved filters through the equivalent v3 report for that window", false, ""),
		p("basis", "string", "cash or accrual; defaults to the saved report's basis", false, ""),
	},
	"QBO.REPORTS.AGED_PAYABLE_DETAIL_READ": {
		p("limit", "int", "max payable rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.AGED_RECEIVABLE_DETAIL_READ": {
		p("limit", "int", "max receivable rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.CLASS_SALES_READ": {
		p("limit", "int", "max class rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.CUSTOMER_BALANCE_DETAIL_READ": {
		p("limit", "int", "max customer rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.CUSTOMER_INCOME_READ": {
		p("limit", "int", "max customer rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.DEPARTMENT_SALES_READ": {
		p("limit", "int", "max department rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.INVENTORY_VALUATION_DETAIL_READ": {
		p("limit", "int", "max valuation rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.ITEM_SALES_READ": {
		p("limit", "int", "max item rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.VENDOR_BALANCE_DETAIL_READ": {
		p("limit", "int", "max vendor rows the report service returns before paging", false, "0"),
	},
	"QBO.COMPANY.ATTACHABLE_DOWNLOAD": {
		p("id", "string", "attachable id whose file bytes to download", true, ""),
		p("out", "string", "output file path to write the downloaded bytes", true, ""),
	},
	"QBO.ACCOUNTING.BUDGET_SEARCH": {
		p("query", "string", "substring match on budget name for the search filter", true, ""),
		p("limit", "int", "max budget rows to return from the query endpoint", false, "0"),
	},
	"QBO.ACCOUNTING.CLASS_SEARCH": {
		p("query", "string", "substring match on class name for the search filter", true, ""),
		p("limit", "int", "max class rows to return from the query endpoint", false, "0"),
	},
	"QBO.EXPENSES.BILL_PAYMENT_CREATE": {
		p("supplier", "string", "payee vendor id for the bill payment (required)", true, ""),
		p("bill-id", "string", "bill id to pay, exactly one per call (required)", true, ""),
		p("account", "string", "paying bank account id funding the payment (required)", true, ""),
		p("amount", "string", "payment amount in AUD currency (required)", true, ""),
	},
	"QBO.EXPENSES.BILL_PAYMENT_EDIT": {
		p("id", "string", "bill-payment id to update its allocation (required)", true, ""),
	},
	"QBO.TAX.CODE_CREATE": {
		p("name", "string", "tax-code name for the new v3 taxservice code (required)", true, ""),
		p("rates-json", "string", "TaxRateDetails JSON array @file allowed; needs a real tax agency", false, ""),
	},
	"QBO.TAX.TAX_AGENCY_CREATE": {
		p("name", "string", "display name for the new tax agency (required)", true, ""),
	},
	"QBO.TAX.TAX_RATE_CREATE": {
		p("name", "string", "name for the new standalone tax rate (required)", true, ""),
		p("rate", "string", "rate value as a number, e.g. 10 for ten percent", false, ""),
		p("agency", "string", "owning tax-agency id used for the AgencyRef link", false, ""),
	},
	"QBO.ACCOUNTING.CDC_READ": {
		p("entities", "string", "comma-separated v3 entity names to watch, e.g. Invoice,Bill", true, ""),
		p("since", "string", "changed-since timestamp RFC3339 or yyyy-MM-dd", true, ""),
	},
	"QBO.COMPANY.EXCHANGE_RATE_READ": {
		p("id", "string", "exchange-rate record id to fetch a single rate (omit to list)", false, ""),
		p("limit", "int", "max currency-pair rows to return from the query endpoint", false, "0"),
	},
	"QBO.COMPANY.EXCHANGE_RATE_SEARCH": {
		p("query", "string", "substring match on source/target currency code (case-insensitive)", true, ""),
		p("limit", "int", "max currency-pair rows to return from the query endpoint", false, "0"),
	},
	"QBO.COMPANY.WEBHOOK_VERIFY": {
		p("payload", "string", "raw webhook body file to verify the signature against", true, ""),
		p("signature", "string", "intuit-signature header value from the webhook delivery", true, ""),
		p("token", "string", "webhook verifier token from the Intuit developer app", true, ""),
	},
	"QBO.CUSTOMERS.CUSTOMER_TYPE_READ": {
		p("id", "string", "customer-type id to fetch a single record (omit to list)", false, ""),
		p("limit", "int", "max customer-type rows to return from the query endpoint", false, "0"),
	},
	"QBO.CUSTOMERS.CUSTOMER_TYPE_SEARCH": {
		p("query", "string", "substring match on customer-type name (case-insensitive)", true, ""),
		p("limit", "int", "max customer-type rows to return from the query endpoint", false, "0"),
	},
	"QBO.EXPENSES.CREDIT_CARD_PAYMENT_READ": {
		p("id", "string", "credit-card-payment id to fetch a single record (omit to list)", false, ""),
		p("limit", "int", "max credit-card-payment rows to return from the query endpoint", false, "0"),
	},
	"QBO.EXPENSES.CREDIT_CARD_PAYMENT_SEARCH": {
		p("query", "string", "substring match on credit-card-payment fields (case-insensitive)", true, ""),
		p("limit", "int", "max credit-card-payment rows to return from the query endpoint", false, "0"),
	},
	"QBO.REPORTS.PROFIT_LOSS_DETAIL_READ": {
		p("limit", "int", "max detail rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.INVENTORY_VALUATION_READ": {
		p("limit", "int", "max valuation rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.TRANSACTION_LIST_READ": {
		p("limit", "int", "max transaction rows the report service returns before paging", false, "0"),
	},
	"QBO.REPORTS.JOURNAL_REPORT_READ": {
		p("limit", "int", "max journal rows the report service returns before paging", false, "0"),
	},
	"QBO.SALES.INVOICE_SEND": {
		p("id", "string", "invoice id to email to the customer address", true, ""),
		p("to", "string", "override recipient email instead of the entity address", false, ""),
	},
	"QBO.SALES.ESTIMATE_SEND": {
		p("id", "string", "estimate id to email to the customer address", true, ""),
		p("to", "string", "override recipient email instead of the entity address", false, ""),
	},
	"QBO.SALES.CREDIT_MEMO_SEND": {
		p("id", "string", "credit-memo id to email to the customer address", true, ""),
		p("to", "string", "override recipient email instead of the entity address", false, ""),
	},
	"QBO.SALES.RECEIPT_SEND": {
		p("id", "string", "sales-receipt id to email to the customer address", true, ""),
		p("to", "string", "override recipient email instead of the entity address", false, ""),
	},
	"QBO.SALES.ESTIMATE_PDF": {
		p("id", "string", "estimate id to download as a PDF document", true, ""),
		p("out", "string", "output .pdf path to write the downloaded bytes", true, ""),
	},
	"QBO.SALES.CREDIT_MEMO_PDF": {
		p("id", "string", "credit-memo id to download as a PDF document", true, ""),
		p("out", "string", "output .pdf path to write the downloaded bytes", true, ""),
	},
	"QBO.SALES.RECEIPT_PDF": {
		p("id", "string", "sales-receipt id to download as a PDF document", true, ""),
		p("out", "string", "output .pdf path to write the downloaded bytes", true, ""),
	},
	"QBO.ACCOUNTING.COA_CREATE": {
		p("name", "string", "new chart-of-accounts name (unique). Account Type selects P&L vs balance sheet", true, ""),
		p("type", "string", "AccountType: Expense, Income, Bank, Other Current Asset, etc.", false, "Expense"),
	},
	"QBO.ACCOUNTING.COA_DEACTIVATE": {
		p("id", "string", "QBO account id to make inactive (not a bank-feed :ofx display id)", true, ""),
	},

	"QBO.ACCOUNTING.CLASS_CREATE": {
		p("name", "string", "class name (department, product line, or other segment). Turn on class tracking first", true, ""),
	},
	"QBO.ACCOUNTING.LOCATION_CREATE": {
		p("name", "string", "location name (office, region, outlet). Plus/Advanced only", true, ""),
	},
	"QBO.ACCOUNTING.DEPARTMENT_CREATE": {
		p("name", "string", "department name (office, region, outlet). Plus/Advanced only", true, ""),
	},
	"QBO.ACCOUNTING.DEPARTMENT_EDIT": {
		p("id", "string", "target department id for rename (required)", true, ""),
		p("name", "string", "new display name for the department record", false, ""),
	},
	"QBO.ACCOUNTING.DEPARTMENT_DELETE": {
		p("id", "string", "target department id for deactivation (required)", true, ""),
	},
	"QBO.ADVANCED.BACKUP_CREATE": {
		p("confirm", "bool", "acknowledge Advanced-only Online Backup and Restore; not all data is restorable", true, ""),
	},
	"QBO.ADVANCED.BACKUP_RESTORE": {
		p("when", "string", "backup timestamp or backup id to restore", true, ""),
	},
	"QBO.ADVANCED.CUSTOM_ROLES_CREATE": {
		p("name", "string", "custom firm role name (QBO Accountant)", true, ""),
		p("permissions", "string", "firm/client access set", false, ""),
	},
	"QBO.ADVANCED.EXPENSE_CLAIMS_REVIEW": {
		p("id", "string", "expense-claim or reimbursement txn id", true, ""),
	},
	"QBO.ADVANCED.REVENUE_RECOGNITION_SETUP": {
		p("templates", "bool", "use pre-built templates (false = custom templates only)", false, ""),
	},
	"QBO.ADVANCED.TASKS_CREATE": {
		p("name", "string", "task name", true, ""),
		p("due", "string", "due date YYYY-MM-DD or RFC3339", true, ""),
		p("assignee", "string", "assignee identity profile id (defaults to session user)", false, ""),
	},
	"QBO.ADVANCED.WORKFLOWS_CREATE": {
		p("template", "string", "workflow template name (Advanced-only; 60+ templates)", false, ""),
		p("name", "string", "workflow name", true, ""),
		p("description", "string", "workflow description", false, ""),
	},
	"QBO.ADVANCED.WORKFLOWS_EDIT": {
		p("id", "string", "appconnect workflow id (from advanced workflows get)", true, ""),
		p("name", "string", "new workflow name", false, ""),
		p("description", "string", "new workflow description", false, ""),
		p("action", "string", "status action: activate or deactivate", false, ""),
	},
	"QBO.ADVANCED.SPREADSHEET_SYNC_IMPORT": {
		p("file", "string", "Spreadsheet Sync workbook (Advanced/Accountant). Starred columns required", true, ""),
		p("template", "string", "list template or transaction template", true, ""),
	},
	"QBO.COMPANY.ATTACHABLE_CREATE": {
		p("file", "string", "local file to upload as a v3 Attachable (@path allowed)", true, ""),
	},
	"QBO.COMPANY.ATTACHABLE_DELETE": {
		p("id", "string", "attachable id to delete (find via attachable search)", true, ""),
	},
	"QBO.COMPANY.ATTACHABLE_EDIT": {
		p("id", "string", "attachable id to update (find via attachable search)", true, ""),
		p("note", "string", "attachable note text stored on the record", false, ""),
	},
	"QBO.COMPANY.BILLS_IMPORT": {
		p("file", "string", "bills CSV/XLSX. First row maps columns. No bulk undo", true, ""),
	},
	"QBO.COMPANY.CONTACT_FORM_CREATE": {
		p("name", "string", "contact form name", true, ""),
		p("fields", "string", "fields to collect from leads", false, ""),
	},
	"QBO.COMPANY.CURRENCY_CREATE": {
		p("code", "string", "ISO currency to add after multicurrency is on. Irreversible once enabled", true, ""),
	},
	"QBO.COMPANY.CURRENCY_REVALUE": {
		p("date", "string", "dd/MM/yyyy revaluation date", true, ""),
		p("rate", "string", "exchange rate override (optional; else downloaded rate)", false, ""),
	},
	"QBO.COMPANY.CUSTOM_FIELD_CREATE": {
		p("name", "string", "custom field name. Tags retire 16 May 2025 / 19 May 2028 — migrate here", true, ""),
		p("type", "string", "text | dropdown | number | date", false, "text"),
	},
	"QBO.COMPANY.ROLE_CREATE": {
		p("name", "string", "role name", true, ""),
		p("permissions", "string", "permission set; subscription tier limits billable roles", false, ""),
	},
	"QBO.COMPANY.TAG_CREATE": {
		p("name", "string", "tag name. New tags blocked from 16 May 2025; prefer custom fields", true, ""),
		p("group", "string", "tag group", false, ""),
	},
	"QBO.COMPANY.PREFERENCES_READ": {},
	"QBO.COMPANY.USER_CREATE": {
		p("email", "string", "invitee email; they accept via Intuit Account", true, ""),
		p("role", "string", "primary admin | company admin | standard | custom", true, ""),
	},
	"QBO.CUSTOMERS.CUSTOMER_MERGE": {
		p("keep", "string", "customer display name/id to keep", true, ""),
		p("merge", "string", "duplicate to fold in (becomes inactive). Recreate manually to undo", true, ""),
	},
	"QBO.CUSTOMERS.CUSTOMER_CREDIT_TRANSFER": {
		p("from", "string", "customer holding the credit", true, ""),
		p("to", "string", "customer receiving the credit", true, ""),
		p("amount", "string", "AUD amount (usually via journal)", true, ""),
	},
	"QBO.EXPENSES.BANK_FEE_CREATE": {
		p("account", "string", "bank account charged", true, ""),
		p("amount", "string", "fee amount AUD (expense)", true, ""),
		p("date", "string", "dd/MM/yyyy", false, ""),
		p("payee", "string", "bank/payee name", false, ""),
	},
	"QBO.EXPENSES.BILL_IMPORT": {
		p("file", "string", "bills CSV. Essentials/Plus/Advanced. Map supplier, date, amount, GST", true, ""),
	},
	"QBO.EXPENSES.CHEQUE_BOUNCE": {
		p("id", "string", "v3 Payment id for the bounced cheque", true, ""),
		p("date", "string", "reversal date dd/MM/yyyy (default today)", false, ""),
		p("fee", "string", "bank bounce fee amount — debits 'Bank charges and fees' (optional)", false, ""),
	},
	"QBO.EXPENSES.CHEQUE_REPRINT": {
		p("id", "string", "cheque id to reprint", true, ""),
	},
	"QBO.EXPENSES.SUPPLIER_MERGE": {
		p("keep", "string", "supplier to keep", true, ""),
		p("merge", "string", "duplicate to merge (inactive after). History moves to keep", true, ""),
	},
	"QBO.EXPENSES.TIME_ACTIVITY_CREATE": {
		p("name-of", "string", "Employee or Vendor — whose time is recorded", true, ""),
		p("employee", "string", "employee id when name-of is Employee (required for staff time)", false, ""),
		p("vendor", "string", "vendor id when name-of is Vendor (required for contractor time)", false, ""),
		p("customer-id", "string", "customer or project the time is recorded against", false, ""),
		p("item-id", "string", "service item id billed for the time entry", false, ""),
		p("hours", "string", "whole hours (combine with --minutes for partial hours)", false, ""),
		p("minutes", "string", "extra minutes recorded as minutes/60 of an hour", false, ""),
		p("rate", "string", "hourly rate recorded as HourlyRate (default 1)", false, ""),
		p("date", "string", "work date in dd/MM/yyyy AU locale (default today)", false, ""),
		p("description", "string", "work description recorded as Description", false, ""),
		p("billable", "string", "billable status: true/false (Billable/NotBillable)", false, ""),
	},
	"QBO.EXPENSES.TIME_ACTIVITY_EDIT": {
		p("id", "string", "target time-activity id to update (required)", true, ""),
		p("name-of", "string", "Employee or Vendor — whose time is recorded", false, ""),
		p("employee", "string", "employee id when name-of is Employee (required for staff time)", false, ""),
		p("vendor", "string", "vendor id when name-of is Vendor (required for contractor time)", false, ""),
		p("customer-id", "string", "customer or project the time is recorded against", false, ""),
		p("item-id", "string", "service item id billed for the time entry", false, ""),
		p("hours", "string", "whole hours (combine with --minutes for partial hours)", false, ""),
		p("minutes", "string", "extra minutes recorded as minutes/60 of an hour", false, ""),
		p("rate", "string", "hourly rate recorded as HourlyRate (default 1)", false, ""),
		p("date", "string", "work date in dd/MM/yyyy AU locale (default today)", false, ""),
		p("description", "string", "work description recorded as Description", false, ""),
		p("billable", "string", "billable status: true/false (Billable/NotBillable)", false, ""),
	},
	"QBO.EXPENSES.TIME_ACTIVITY_DELETE": {
		p("id", "string", "target time-activity id to delete (required)", true, ""),
	},
	"QBO.EXPENSES.PURCHASE_SEARCH": {
		p("query", "string", "substring match on name or doc number (case-insensitive)", false, ""),
		p("limit", "int", "max rows to return from the v3 query endpoint", false, "0"),
	},
	"QBO.INVENTORY.CONSIGNMENT_CREATE": {
		p("item", "string", "consigned item", true, ""),
		p("qty", "string", "quantity on consignment", true, ""),
		p("class", "string", "class used to track consignment vs owned stock", false, ""),
	},
	"QBO.PAYROLL.EMPLOYEE_TERMINATE": {
		p("employee", "string", "employee to terminate. All pay runs must be finalised first", true, ""),
		p("date", "string", "termination date dd/MM/yyyy", true, ""),
		p("reason", "string", "STP Phase 2 termination reason (required for ATO)", true, ""),
	},
	"QBO.PAYROLL.LEAVE_CATEGORY_CREATE": {
		p("name", "string", "leave category name", true, ""),
		p("type", "string", "annual | personal | LSL | other", false, ""),
	},
	"QBO.PAYROLL.LEAVE_CATEGORY_EDIT": {
		p("id", "string", "leave category id", true, ""),
	},
	"QBO.PAYROLL.LEAVE_CATEGORY_READ": {
		p("id", "string", "leave category id", false, ""),
	},
	"QBO.PAYROLL.LEAVE_BALANCE_EDIT": {
		p("employee", "string", "employee whose accrual is wrong", true, ""),
		p("category", "string", "leave category", true, ""),
		p("hours", "string", "corrected balance hours", true, ""),
	},
	"QBO.PAYROLL.DEDUCTION_CREATE": {
		p("employee", "string", "employee", true, ""),
		p("category", "string", "deduction category (set these up first)", true, ""),
		p("amount", "string", "amount or percent", true, ""),
		p("reference", "string", "CSA payment reference if Child Support", false, ""),
	},
	"QBO.PAYROLL.DEDUCTION_EDIT": {
		p("id", "string", "deduction id", true, ""),
	},
	"QBO.PAYROLL.DEDUCTION_READ": {
		p("employee", "string", "filter by employee", false, ""),
	},
	"QBO.PAYROLL.PAY_RUN_INCLUSION_CREATE": {
		p("employee", "string", "employee the recurring inclusion applies to", true, ""),
		p("category", "string", "pay category or deduction", true, ""),
		p("amount", "string", "recurring amount", true, ""),
	},
	"QBO.PAYROLL.STP_PAY_CREATE": {
		p("pay-run", "string", "finalised pay run to lodge as an STP pay event", true, ""),
	},
	"QBO.PAYROLL.STP_UPDATE_CREATE": {
		p("year", "string", "financial year of the update event", true, ""),
		p("employee", "string", "scope to one employee (optional)", false, ""),
	},
	"QBO.PAYROLL.STP_FINALISE": {
		p("year", "string", "financial year to finalise. Lodge last FY pay run first", true, ""),
	},
	"QBO.PAYROLL.STP_RESET": {
		p("year", "string", "FY whose incorrectly reported settings need an STP reset event", true, ""),
	},
	"QBO.PAYROLL.STP_CLOSELY_HELD_LODGE": {
		p("method", "string", "pay-day | quarterly | estimate — ATO options for closely held staff", true, ""),
		p("year", "string", "financial year", false, ""),
	},
	"QBO.PAYROLL.STP_MICRO_LODGE": {
		p("year", "string", "FY for micro-employer quarterly STP concession", true, ""),
	},
	"QBO.PAYROLL.PAYG_SUMMARY_CREATE": {
		p("year", "string", "FY. STP-exempt businesses only — do not use if you lodge STP", true, ""),
	},
	"QBO.PAYROLL.WORKCOVER_CREATE": {
		p("employee", "string", "employee", false, ""),
		p("category", "string", "WorkCover / workers-comp pay category", true, ""),
		p("amount", "string", "amount", false, ""),
	},
	"QBO.PAYROLL.BPAY_CREATE": {
		p("pay-run", "string", "pay run to generate employee BPAY/ABA file from", true, ""),
	},
	"QBO.PAYROLL.SUPER_RESC_EDIT": {
		p("employee", "string", "employee whose RESC is wrong", true, ""),
		p("amount", "string", "corrected reportable employer super contribution", true, ""),
	},
	"QBO.PAYROLL.LUMP_SUM_CREATE": {
		p("employee", "string", "employee", true, ""),
		p("method", "string", "A or B(ii) — ask the accountant which ATO method", true, ""),
		p("amount", "string", "gross lump sum; payroll calculates PAYG/STSL", true, ""),
	},
	"QBO.REPORTS.FORECAST_CREATE": {
		p("name", "string", "forecast name (Advanced)", true, ""),
		p("months", "int", "horizon", false, "12"),
	},
	"QBO.REPORTS.FORECAST_EDIT": {
		p("id", "string", "forecast id", true, ""),
	},
	"QBO.REPORTS.FORECAST_READ": {
		p("id", "string", "forecast id", false, ""),
	},
	"QBO.REPORTS.REPORT_EDIT": {
		p("id", "string", "saved report id sbg:… (GET+PUT keys by the sbg: id)", true, ""),
		p("name", "string", "new report name (resend keeps the cloned dataRequest)", true, ""),
	},
	"QBO.REPORTS.REPORT_DELETE": {
		p("id", "string", "saved report id sbg:… (soft-delete: active:false)", true, ""),
	},
	"QBO.REPORTS.MANAGEMENT_CREATE": {
		p("name", "string", "management report name (Accountant bundle)", true, ""),
	},
	"QBO.REPORTS.MANAGEMENT_EDIT": {
		p("id", "string", "management report id", true, ""),
	},
	"QBO.REPORTS.MANAGEMENT_READ": {
		p("id", "string", "management report id", false, ""),
	},
	"QBO.REPORTS.MANAGEMENT_AUDIT": {
		p("id", "string", "folio id (sbg:…) of the management report", true, ""),
		p("from", "string", "first statement month, YYYY-MM (inclusive)", true, ""),
		p("to", "string", "last statement month, YYYY-MM (inclusive)", true, ""),
	},
	"QBO.SALES.ESTIMATE_COPY": {
		p("id", "string", "quote/estimate to duplicate. Customer can be changed on the copy", true, ""),
		p("customer", "string", "override customer on the copy", false, ""),
	},
	"QBO.SALES.INVOICE_COPY": {
		p("id", "string", "invoice to duplicate. Prefer recurring txn if the same customer repeats", true, ""),
		p("customer", "string", "override customer on the copy", false, ""),
	},
	"QBO.SALES.INVOICE_DISCOUNT": {
		p("id", "string", "invoice/estimate/sales-receipt id", true, ""),
		p("percent", "string", "percent of subtotal (turn discounts on in settings first)", false, ""),
		p("amount", "string", "fixed AUD discount as a negative line if not percent", false, ""),
	},
	"QBO.SALES.INVOICE_PDF": {
		p("id", "string", "sales invoice id to download as PDF (see invoice get)", true, ""),
		p("out", "string", "output .pdf path to write the downloaded bytes", true, ""),
	},
	"QBO.SALES.INVOICE_PROGRESS": {
		p("estimate", "string", "source quote. Progress invoicing must be on; Airy template or custom", true, ""),
		p("percent", "string", "percent of quote to bill on this progress invoice", false, ""),
	},
	"QBO.SALES.INVOICE_REMIND": {
		p("id", "string", "invoice id (omit to run automatic reminder settings)", false, ""),
		p("automatic", "bool", "use scheduled reminders vs send one now", false, ""),
	},
	"QBO.SALES.INVOICE_WRITE_OFF": {
		p("id", "string", "unpaid invoice id. Accountant write-off tool; bad-debt expense", true, ""),
		p("account", "string", "bad-debt expense account", false, ""),
	},
	"QBO.SALES.TIME_INVOICE_CREATE": {
		p("customer", "string", "customer with unbilled time (Essentials/Plus/Advanced)", false, ""),
		p("auto", "bool", "turn on automatic invoices for unbilled activities", false, ""),
	},
	"QBO.TAX.GST_PAYMENT_CREATE": {
		p("period", "string", "BAS period being paid", true, ""),
		p("amount", "string", "AUD paid to ATO", true, ""),
		p("date", "string", "dd/MM/yyyy", false, ""),
		p("account", "string", "bank account the BAS payment left", false, ""),
	},
	"QBO.TAX.GST_PAYMENT_DELETE": {
		p("id", "string", "GST/BAS payment txn to delete", true, ""),
	},
	"QBO.TAX.GST_REFUND_CREATE": {
		p("period", "string", "BAS period the refund belongs to", true, ""),
		p("amount", "string", "AUD refund from ATO", true, ""),
		p("date", "string", "dd/MM/yyyy", false, ""),
	},
	"QBO.TAX.GST_DEFERRED_CREATE": {
		p("amount", "string", "deferred GST liability AUD", true, ""),
		p("date", "string", "dd/MM/yyyy", false, ""),
		p("account", "string", "GST liability account", false, ""),
	},
	"QBO.TAX.LODGEIT_EXPORT": {
		p("period", "string", "BAS period to pull GST/PAYGW into LodgeiT", true, ""),
	},

	// --- service map (2026-08-24): never-triggered production services ---
	"QBO.ACCOUNTING.RECONCILE_SERVICE_GET": {
		p("account", "string", "bank or credit-card account whose reconciliation state is read", false, ""),
	},
	"QBO.EXPENSES.BILL_PAY_SERVICE_CREATE": {
		p("payee", "string", "supplier paid through the online bill-pay service (bank rails)", true, ""),
		p("amount", "string", "AUD amount debited from the funding bank account", true, ""),
		p("date", "string", "dd/MM/yyyy send-by date; the service schedules the debit", false, ""),
	},
	"QBO.ACCOUNTING.BUDGET_PLANNING_GET": {
		p("fiscal-year", "string", "financial year of the business-planning budget (e.g. FY2026)", false, ""),
	},
	"QBO.ACCOUNTING.COA_TEMPLATES_GET": {
		p("industry", "string", "industry filter for suggested chart-of-accounts templates", false, ""),
	},
	"QBO.COMPANY.DIMENSIONS_GRAPHQL_GET": {
		p("dimension", "string", "custom dimension definition name to read", false, ""),
	},
	"QBO.COMPANY.CUSTOM_EXTENSIONS_LIST": {
		p("entity", "string", "entity type whose custom-extension recommendations are listed", false, ""),
	},
	"QBO.COMPANY.COMPANY_MANAGEMENT_GET": {
		p("company", "string", "company id within the multi-company group", false, ""),
	},
	"QBO.ACCOUNTING.DEFERRED_RECOGNITION_GET": {
		p("schedule-id", "string", "deferred recognition schedule id to read", false, ""),
	},
	"QBO.ACCOUNTANT.PORTFOLIO_STORE_GET": {
		p("client", "string", "client company in the accountant portfolio", false, ""),
	},
	"QBO.FEED.BANK_ACCOUNT_LOOKUP_GET": {
		p("query", "string", "institution or account name to look up for feed linking", true, ""),
	},
	"QBO.FEED.ACH_TRANSFER_CREATE": {
		p("from-account", "string", "source bank account id debited by the ACH transfer", true, ""),
		p("to-account", "string", "destination external bank account id credited", true, ""),
		p("amount", "string", "AUD/USD transfer amount moved between accounts", true, ""),
		p("date", "string", "dd/MM/yyyy effective date of the transfer", false, ""),
	},
	"QBO.INTEGRATIONS.APPFLOW_RUN": {
		p("app", "string", "third-party app connection name to synchronise now", true, ""),
	},
	"QBO.SALES.LATE_FEE_CREATE": {
		p("invoice", "string", "overdue invoice id that accrues the automatic late fee", true, ""),
		p("percent", "string", "fee as percent of the overdue balance per period", false, ""),
		p("flat", "string", "fixed AUD fee applied instead of a percentage", false, ""),
	},
	"QBO.SALES.KNOWLEDGE_BASE_SEARCH": {
		p("query", "string", "question text answered from the product knowledge base", true, ""),
	},
	"QBO.SALES.COMMERCE_CONTROL_GET": {
		p("order", "string", "commerce order id managed by the order-control service", false, ""),
	},
	"QBO.EXPENSES.B2B_BILLPAY_GET": {
		p("supplier", "string", "supplier enrolled in business-to-business bill pay", false, ""),
	},
	"QBO.EXPENSES.B2B_NETWORK_GET": {
		p("supplier", "string", "supplier identity resolved on the B2B network", false, ""),
	},
	"QBO.EXPENSES.BILL_PAY_ONBOARDING_GET": {
		p("status-only", "bool", "return enrolment status without starting onboarding", false, ""),
	},
	"QBO.COMPANY.CONSENT_GET": {
		p("scope", "string", "consent scope such as banking data sharing to check", false, ""),
	},
	"QBO.COMPANY.CONSUMER_NOTIFICATION_POST": {
		p("event", "string", "notification event key delivered to the browser channel", true, ""),
		p("payload", "string", "JSON payload carried with the notification event", false, ""),
	},
	"QBO.COMPANY.CONTENT_ACCESS_GET": {
		p("query", "string", "help/content search text checked against entitlements", true, ""),
	},
	"QBO.CUSTOMERS.PROPOSAL_DELETE": {
		p("id", "string", "numeric proposal id (DELETE rejects the refId UUID)", true, ""),
	},
	"QBO.CUSTOMERS.PROPOSAL_EDIT": {
		p("id", "string", "proposal refId UUID (PUT keys by refId, not the numeric id)", true, ""),
		p("title", "string", "new title (service keeps the auto 'Proposal {id}' display name)", false, ""),
	},
	"QBO.CUSTOMERS.PROPOSAL_SVC_CREATE": {
		p("customer", "string", "customer/lead contact id the proposal is issued against", true, ""),
		p("contact-type", "string", "contact type sent to crm-proposal-svc (CUSTOMER or LEAD)", false, "CUSTOMER"),
	},
	"QBO.CUSTOMERS.SALES_AGENT_GET": {
		p("conversation", "string", "crm-sales-agent conversation id to read; omit to list recent conversations", false, ""),
	},
	"QBO.CUSTOMERS.SALES_AGENT_PROMPT_POST": {
		p("prompt", "string", "natural-language instruction sent to the sales agent", true, ""),
		p("context", "string", "lead/opportunity context the agent reasons over", false, ""),
	},
	"QBO.COMPANY.CUSTOMIZATION_AGENT_POST": {
		p("prompt", "string", "natural-language customization request for the agent", true, ""),
		p("surface", "string", "UI surface (form, list, or report) the customization applies to", false, ""),
	},
	"QBO.COMPANY.DATA_EXPORT_GET": {
		p("token", "string", "report/list token (BAL_SHEET, GEN_LEDGER, JOURNAL, PANDL, TRIAL_BAL, CUST_CONTACT, EMP_CONTACT, VEND_CONTACT, or a classic token like TX_DET_BY_ACCT)", true, ""),
		p("out", "string", "output .xlsx path to write the downloaded bytes", true, ""),
		p("attr", "strings", "reportCustomizationAttributes key=value, repeatable (mem_rpt_id, klass, account, low_date, high_date, cash_basis, customized, date_macro, columns)", false, "[]"),
	},
	"QBO.COMPANY.DATA_MIRROR_GET": {
		p("dataset", "string", "non-production mirrored dataset name (data-dev)", true, ""),
	},
	// --- service map v2 (2026-08-24): remaining never-triggered production services ---
	"QBO.ACCOUNTING.FIT_TRANSACTIONS_GET": {
		p("feed-account-ids", "string", "comma-separated OLB bank-feed account ids (`realm:bank:accountId`) to inspect", true, ""),
	},
	"QBO.ACCOUNTING.LIBRO_GET": {
		p("view", "string", "ledger view to read (transactions or accounts)", false, ""),
	},
	"QBO.ACCOUNTING.MIDMARKET_SETTINGS_GET": {
		p("section", "string", "Mid-Market settings section to read (e.g. sales-forms, expenses)", false, ""),
	},
	"QBO.ACCOUNTING.MULTIENTITY_GET": {
		p("entity", "string", "linked entity id whose consolidation status is read", false, ""),
	},
	"QBO.ACCOUNTING.QBDATA_MIGRATION_POST": {
		p("source-company", "string", "source company realm id to migrate data from", true, ""),
		p("target-company", "string", "target company realm id receiving the migrated data", true, ""),
	},
	"QBO.ACCOUNTING.TAXCONFIG_MIRROR_GET": {
		p("config-key", "string", "tax configuration key to read from the test mirror", true, ""),
	},
	"QBO.ADVANCED.ACS_GET": {
		p("client", "string", "client company whose accountant-service records are read", false, ""),
	},
	"QBO.ADVANCED.ERP_BUILDER_LIST": {
		p("template", "string", "page template the builder starts from (list of available templates)", false, ""),
	},
	"QBO.COMPANY.CHORUS_GET": {
		p("surface", "string", "dimensions or agent-builder surface to query", false, ""),
	},
	"QBO.COMPANY.CUSTOMIZATION_AGENT_QA_POST": {
		p("prompt", "string", "natural-language customization request for the QA agent mirror", true, ""),
	},
	"QBO.COMPANY.ENVELOPE_POST": {
		p("envelope-id", "string", "approval envelope id to inspect before any authorisation flow", false, ""),
	},
	"QBO.COMPANY.EXPERIMENT_ASSIGNMENT_GET": {
		p("experiment", "string", "experiment id whose assignment bucket is read", true, ""),
	},
	"QBO.COMPANY.EXPERT_CREDENTIALS_GET": {
		p("expert", "string", "expert id whose credential verification is read", true, ""),
	},
	"QBO.COMPANY.EXPERT_ENGAGEMENT_POST": {
		p("engagement", "string", "expert-engagement session id to inspect or book against", true, ""),
	},
	"QBO.COMPANY.FRANCHISE_ORCH_POST": {
		p("franchise", "string", "franchise group id whose orchestration state is queried", false, ""),
	},
	"QBO.COMPANY.GENPLUGIN_REGISTRY_GET": {
		p("plugin", "string", "generated-plugin id to resolve from the registry", false, ""),
	},
	"QBO.COMPANY.GEOCODE_GET": {
		p("address", "string", "address text to geocode into coordinates", true, ""),
	},
	"QBO.COMPANY.IDENTITY_QA_GET": {
		p("subject", "string", "user or company subject to resolve in the identity graph", true, ""),
	},
	"QBO.COMPANY.INPUT_TO_ACTION_POST": {
		p("feed-item", "string", "business-feed item id to convert into an action item", false, ""),
	},
	"QBO.COMPANY.MODELEXECUTION_POST": {
		p("model", "string", "smart-compose model path (e.g. qbl-notes/2) to invoke", true, ""),
		p("draft", "string", "draft text the model completes or rewrites", true, ""),
	},
	"QBO.COMPANY.QBDATAMIGRATION_STATUS_GET": {
		p("job", "string", "migration job id whose status is read (from a prior migration run)", true, ""),
	},
	"QBO.COMPANY.QBLIVE_EXPERIENCE_GET": {
		p("surface", "string", "live-experience surface key to read (tour, banner, checklist)", false, ""),
	},
	"QBO.COMPANY.SWIFT_NONCAT_POST": {
		p("request", "string", "Swift non-catalogued request payload name to inspect", true, ""),
	},
	"QBO.CUSTOMERS.MAILCHIMP_NOTIFICATIONS_POST": {
		p("journey", "string", "customer-journey pipeline id the notification belongs to", false, ""),
	},
	"QBO.CUSTOMERS.MCF_BACKEND_GET": {
		p("funnel", "string", "multi-channel funnel id to read lead-attribution views for", false, ""),
	},
	"QBO.CUSTOMERS.MC_APPFABRIC_GET": {
		p("app", "string", "Mailchimp app-fabric resource path to proxy", false, ""),
	},
	"QBO.CUSTOMERS.PROMPT_EVALUATOR_POST": {
		p("prompt", "string", "candidate prompt text submitted for evaluation", true, ""),
	},
	"QBO.CUSTOMERS.REALTIME_ER_GET": {
		p("record", "string", "lead/contact record id to resolve against known entities", true, ""),
	},
	"QBO.CUSTOMERS.REVIEWS_GET": {
		p("app", "string", "app id whose reviews and ratings are listed", false, ""),
	},
	"QBO.EXPENSES.PAYMENTS_FUNDING_GET": {
		p("payout", "string", "payout id whose funding leg is inspected", true, ""),
	},
	"QBO.EXPENSES.PAYMENT_ACCOUNT_GET": {
		p("instrument", "string", "payment instrument id whose funding profile is read", false, ""),
	},
	"QBO.EXPENSES.PYMTS_APPS_ORCH_POST": {
		p("workflow", "string", "payments-app workflow id to inspect (payment app orchestration state)", false, ""),
	},
	"QBO.EXPENSES.RESOLUTION_CENTER_GET": {
		p("issue", "string", "payment issue id whose resolution steps are read", false, ""),
	},
	"QBO.EXPENSES.SPENDMGMT_GET": {
		p("scope", "string", "spend scope to read (company, vendor, card programme)", false, ""),
	},
	"QBO.EXPENSES.STAGETXN_TEST_GET": {
		p("entity", "string", "staged entity id to read from the test stage", true, ""),
	},
	"QBO.EXPENSES.SUBSCRIPTION_SBG_GET": {
		p("entitlement", "string", "subscription entitlement key to check (e.g. efiling-offers)", false, ""),
	},
	"QBO.EXPENSES.TXN_INGESTION_POST": {
		p("batch", "string", "receipt batch id whose ingested transactions are inspected", true, ""),
	},
	"QBO.EXPENSES.WALLET_GET": {
		p("owner", "string", "customer or vendor whose wallet entries are listed", true, ""),
	},
	"QBO.FEED.FINANCIAL_DATA_INTERACTION_POST": {
		p("interaction", "string", "interaction event name recorded against the financial data surface", true, ""),
	},
	"QBO.FEED.FINANCIAL_DOCUMENT_GET": {
		p("document", "string", "financial document/statement id to retrieve metadata for", true, ""),
	},
	"QBO.FEED.FINANCIAL_PROFILE_GET": {
		p("account", "string", "linked bank account whose financial profile is read", true, ""),
	},
	"QBO.FEED.FINANCIAL_PROFILE_ORCH_POST": {
		p("refresh", "string", "request a profile refresh as part of the orchestration", false, ""),
	},
	"QBO.FEED.FINANCIAL_PROVIDER_GET": {
		p("account", "string", "linked account whose feed provider is resolved", true, ""),
	},
	"QBO.FEED.TXNS_ORCHESTRATION_POST": {
		p("batch", "string", "transaction batch id the orchestration runs over", false, ""),
	},
	"QBO.FEED.VAULT_GET": {
		p("session", "string", "document-vault session id to inspect before any OAuth exchange", false, ""),
	},
	"QBO.INTEGRATIONS.IDX_GET": {
		p("lookup", "string", "IDX lookup key (institution or account token)", true, ""),
	},
	"QBO.INTEGRATIONS.V4_AWS_POST": {
		p("resource", "string", "v4 gateway resource path the request would target", false, ""),
	},
	"QBO.INTEGRATIONS.WORKFLOW_AUTOMATION_GET": {
		p("automation", "string", "automation id whose definition/status is read", false, ""),
	},
	"QBO.INVENTORY.INVENTORY_AGENTS_POST": {
		p("agent", "string", "inventory agent id whose suggestions are requested", false, ""),
	},
	"QBO.INVENTORY.INVENTORY_COST_GET": {
		p("method", "string", "costing method filter applied to the valuation report (fifo, average)", false, ""),
	},
	"QBO.INVENTORY.ITEM_MANAGEMENT_GET": {
		p("item", "string", "product/service item id managed by item-management-svc to read", false, ""),
	},
	"QBO.INVENTORY.QBBULKACTION_POST": {
		p("selection", "string", "saved selection id the bulk action applies to", true, ""),
		p("action", "string", "bulk action name (e.g. activate, archive)", true, ""),
	},
	"QBO.INVENTORY.WAREHOUSE_SERVICE_GET": {
		p("location", "string", "warehouse location id whose stock summary is read", false, ""),
	},
	"QBO.SALES.QBONLINE_GRAPHQL_GET": {
		p("op", "string", "gateway operation name that would be routed to the alternate host", false, ""),
	},
	"QBO.SALES.SALESTXN_RISK_GET": {
		p("txn", "string", "sales transaction id whose aggregate/risk view is read", false, ""),
	},
	"QBO.SALES.SALES_AI_CALL_POST": {
		p("invoice", "string", "invoice id the AI call is scheduled against", true, ""),
		p("call", "string", "existing call id whose details are fetched instead of scheduling", false, ""),
	},
	"QBO.SALES.SALES_CHECKOUT_GET": {
		p("sale", "string", "sale/checkout record id whose activity timeline is read", true, ""),
	},
	"QBO.SALES.SALES_SETTINGS_GET": {
		p("form", "string", "sales form type whose settings are read (invoice, estimate)", false, ""),
	},
	"QBO.SALES.SALES_TRAFFIC_ROUTER_POST": {
		p("route", "string", "logical sales route key the router would resolve", true, ""),
	},
	"QBO.SALES.TXNS_RENDERING_POST": {
		p("txn", "string", "transaction id whose document layout is rendered", true, ""),
	},
	"QBO.TAX.EXPERIMENT_ASSIGNMENT_GET": {
		p("experiment", "string", "indirect-tax experiment id whose assignment bucket is read", true, ""),
	},
	"QBO.TAX.INDIRECT_REPORTS_GET": {
		p("report", "string", "modernised liability report id or period (e.g. 2026-Q3) to render", false, ""),
	},
	"QBO.TAX.NEXUS_GET": {
		p("region", "string", "region/country whose nexus jurisdictions are listed", true, ""),
	},
	"QBO.TAX.TAX_CALCULATION_POST": {
		p("lines", "string", "JSON array of {amount,taxCode} lines to price", true, ""),
		p("date", "string", "yyyy-MM-dd tax point date for the calculation", false, ""),
	},
	"QBO.TAX.TAX_FILING_GET": {
		p("period", "string", "filing period to read liability for (e.g. 2026-Q3)", true, ""),
	},
	"QBO.FEED.ACCOUNT_REFRESH": {
		p("account-id", "string", "connected bank account to pull latest transactions for", false, "204"),
	},
	"QBO.EXPENSES.STAGETXN_GET": {
		p("batch", "string", "staged transaction batch id to inspect on the stagetransactions service", false, ""),
	},
	"QBO.COMPANY.IDENTITY_GET": {
		p("user", "string", "user id whose identity graph record is read from the identity service", false, ""),
	},
	"QBO.COMPANY.SETTINGS_FACADE_GET": {
		p("section", "string", "settings section served by the settingsfacade backend (optional)", false, ""),
	},
	"QBO.GQL.BILLS_WALK": {
		p("from", "string", "window start dd/MM/yyyy or yyyy-MM-dd for the paged bills walk (optional)", false, ""),
		p("to", "string", "window end dd/MM/yyyy or yyyy-MM-dd for the paged bills walk (optional)", false, ""),
		p("max-pages", "int", "hard ceiling on pages fetched, 0 walks to exhaustion (optional)", false, "0"),
		p("page-size", "int", "rows bound per page for the paged bills walk (optional)", false, "100"),
	},
	"QBO.GQL.TASKS_WALK": {
		p("from", "string", "window start dd/MM/yyyy or yyyy-MM-dd for the paged tasks walk (optional)", false, ""),
		p("to", "string", "window end dd/MM/yyyy or yyyy-MM-dd for the paged tasks walk (optional)", false, ""),
		p("max-pages", "int", "hard ceiling on pages fetched, 0 walks to exhaustion (optional)", false, "0"),
		p("page-size", "int", "rows bound per page for the paged tasks walk (optional)", false, "100"),
	},
	"QBO.GQL.ITEMS_WALK": {
		p("max-pages", "int", "hard ceiling on pages fetched, 0 walks to exhaustion (optional)", false, "0"),
		p("page-size", "int", "rows bound per page for the paged items walk (optional)", false, "100"),
	},
	"QBO.GQL.MUTATE_CREATE_ACCOUNT": {
		p("name", "string", "ledger account name, e.g. Payroll Clearing (required)", true, ""),
		p("subtype", "string", "detailType enum like Checking or Savings (required)", true, ""),
		p("type", "string", "accountType enum, defaults to Bank on the command (optional)", false, ""),
		p("description", "string", "description text stored on the ledger account (optional)", false, ""),
		p("number", "string", "general-ledger account number (optional)", false, ""),
		p("parent-id", "string", "parent account id for sub-accounts (optional)", false, ""),
		p("currency", "string", "currency code, defaults to company currency (optional)", false, ""),
	},
	"QBO.GQL.MUTATE_BANK_DISCONNECT": {
		p("olb-account-id", "strings", "OLB account ids to disconnect, repeatable; realm suffixes trimmed (required)", true, ""),
	},
	"QBO.GQL.MUTATE_BATCH_UPDATE_PRODUCTS": {
		p("products-json", "string", "JSON array of Commerce batch-update rows, @file allowed (required)", true, ""),
	},
	"QBO.GQL.MUTATE_ASSIGN_DIMENSIONS": {
		p("input-json", "string", "AssignProductDimensionsInput object as JSON, @file allowed (required)", true, ""),
	},
	"QBO.GQL.MUTATE_CANCEL_RECONCILE": {
		p("reconciliation-id", "string", "integration reconciliation global id to cancel (required)", true, ""),
		p("action", "string", "reconcileAction enum, defaults to CANCELLED (optional)", false, ""),
	},
	"QBO.GQL.MUTATE_RECEIVE_INVENTORY": {
		p("txn-id", "string", "source transaction id holding the inbound movement (required)", true, ""),
		p("txn-type", "string", "source transaction type Bill or Invoice (required)", true, ""),
		p("source-version", "int", "sourceTransaction.version, positive integer (required)", true, ""),
		p("txn-date", "string", "source transaction date yyyy-MM-dd (optional)", false, ""),
		p("vendor-id", "string", "vendor id for Bill-sourced inbound movements (optional)", false, ""),
		p("customer-id", "string", "customer id for Invoice-sourced movements (optional)", false, ""),
		p("lines-json", "string", "movement lines JSON array, @file allowed (required)", true, ""),
	},
	"QBO.GQL.MUTATE_CONSUME_INVENTORY": {
		p("txn-id", "string", "source transaction id holding the outbound movement (required)", true, ""),
		p("txn-type", "string", "source transaction type Bill or Invoice (required)", true, ""),
		p("source-version", "int", "sourceTransaction.version, positive integer (required)", true, ""),
		p("txn-date", "string", "source transaction date yyyy-MM-dd (optional)", false, ""),
		p("vendor-id", "string", "vendor id for Bill-sourced outbound movements (optional)", false, ""),
		p("customer-id", "string", "customer id for Invoice-sourced movements (optional)", false, ""),
		p("lines-json", "string", "movement lines JSON array, @file allowed (required)", true, ""),
	},
	"QBO.GQL.MUTATE_ACTIVATE_PRODUCTS": {
		p("entity-id", "strings", "product ids to activate, repeatable; or use entities-json (optional)", false, ""),
		p("entity-type", "string", "entity type enum, defaults to INVENTORY (optional)", false, ""),
		p("entity-version", "int", "entity version for optimistic locking (optional)", false, "0"),
		p("entities-json", "string", "full entity JSON array overriding --entity-id (optional)", false, ""),
	},
	"QBO.COSTGROUPS.GROUP_LIST": {
		p("status", "string", "list filter active, inactive, or all cost groups (optional)", false, ""),
		p("limit", "int", "max cost groups to return between 1 and 100 (optional)", false, "0"),
		p("type", "string", "cost group type filter, defaults to ITEM like the SPA (optional)", false, ""),
	},
	"QBO.COSTGROUPS.GROUP_CREATE": {
		p("name", "string", "cost group name sent as CreateCostGroup input (required)", true, ""),
		p("description", "string", "cost group description stored with the group (optional)", false, ""),
		p("type", "string", "cost group type, defaults to ITEM when omitted (optional)", false, ""),
	},
	"QBO.COSTGROUPS.GROUP_UPDATE": {
		p("id", "string", "target cost group id to rename (required)", true, ""),
		p("name", "string", "new cost group name sent as UpdateCostGroup input (required)", true, ""),
		p("description", "string", "replacement description stored with the group (optional)", false, ""),
		p("version", "int", "optimistic-concurrency version of the row being changed (optional)", false, "0"),
	},
	"QBO.COSTGROUPS.GROUP_DELETE": {
		p("id", "string", "target cost group id to soft-delete (required)", true, ""),
		p("version", "int", "optimistic-concurrency version of the row being changed (optional)", false, "0"),
		p("activate", "bool", "re-activate instead of deleting, toggling deleted=false", false, ""),
	},
}

var auNoteGaps = map[string]string{
	"QBO.ADVANCED.BATCH_RUN":                        "Native v3 batch: up to 25 mixed ops per call.",
	"QBO.REPORTS.ACCOUNT_LIST_READ":                 "Account list via the v3 report service.",
	"QBO.REPORTS.AGED_PAYABLE_DETAIL_READ":          "Aged-payable detail via the v3 report service.",
	"QBO.REPORTS.AGED_RECEIVABLE_DETAIL_READ":       "Aged-receivable detail via the v3 report service.",
	"QBO.REPORTS.CLASS_SALES_READ":                  "Class sales via the v3 report service.",
	"QBO.REPORTS.CUSTOMER_BALANCE_DETAIL_READ":      "Customer-balance detail via the v3 report service.",
	"QBO.REPORTS.CUSTOMER_INCOME_READ":              "Customer income via the v3 report service.",
	"QBO.REPORTS.DEPARTMENT_SALES_READ":             "Department sales via the v3 report service.",
	"QBO.REPORTS.INVENTORY_VALUATION_DETAIL_READ":   "Inventory-valuation detail via the v3 report service.",
	"QBO.REPORTS.ITEM_SALES_READ":                   "Item sales via the v3 report service.",
	"QBO.REPORTS.VENDOR_BALANCE_DETAIL_READ":        "Vendor-balance detail via the v3 report service.",
	"QBO.COMPANY.ATTACHABLE_DOWNLOAD":               "Download attachable bytes via pre-signed hop.",
	"QBO.ACCOUNTING.BUDGET_SEARCH":                  "Search budgets by name.",
	"QBO.ACCOUNTING.CLASS_SEARCH":                   "Search classes by name.",
	"QBO.EXPENSES.BILL_PAYMENT_CREATE":              "Pay one bill per call from a bank account.",
	"QBO.EXPENSES.BILL_PAYMENT_EDIT":                "Update a bill payment (sparse Id/SyncToken).",
	"QBO.TAX.CODE_CREATE":                           "Create a tax code via v3 taxservice; needs a real agency.",
	"QBO.TAX.TAX_AGENCY_CREATE":                     "Create a tax agency (DisplayName); anchor for rates and codes.",
	"QBO.TAX.TAX_RATE_CREATE":                       "Create a tax rate under an agency for taxservice codes. Live probes 2026-09-19: v3 POST /taxservice is platform-unsupported on the AU build; every indirectTax* mutation field is FieldUndefined on the v4 gateway; the taxconfig/taxfilingsvc hosts never fire on TC2 — the SBSEG-QBO-compliance-hub-indirect-tax flag is off (BAS/Add tax tabs render but issue no service calls). Stays blocked.",
	"QBO.ACCOUNTING.CDC_READ":                       "Changed-since entity stream for sync (v3 CDC).",
	"QBO.COMPANY.EXCHANGE_RATE_READ":                "Read currency-pair rates by id or list.",
	"QBO.COMPANY.EXCHANGE_RATE_SEARCH":              "Search currency-pair rates.",
	"QBO.COMPANY.WEBHOOK_VERIFY":                    "Local HMAC check of an Intuit webhook delivery.",
	"QBO.CUSTOMERS.CUSTOMER_TYPE_READ":              "Read customer types by id or list.",
	"QBO.CUSTOMERS.CUSTOMER_TYPE_SEARCH":            "Search customer types by name.",
	"QBO.EXPENSES.CREDIT_CARD_PAYMENT_READ":         "Read credit-card payments by id or list.",
	"QBO.EXPENSES.CREDIT_CARD_PAYMENT_SEARCH":       "Search credit-card payments.",
	"QBO.REPORTS.PROFIT_LOSS_DETAIL_READ":           "Profit-and-loss detail via the v3 report service.",
	"QBO.REPORTS.INVENTORY_VALUATION_READ":          "Inventory valuation via the v3 report service.",
	"QBO.REPORTS.ACCOUNT_LIST_DETAIL_READ":          "Account list detail via the v3 report service.",
	"QBO.REPORTS.SAVED_LIST":                        "Saved custom report definitions (universalreportinsights /v1/reports/qbo collection).",
	"QBO.REPORTS.SAVED_READ":                        "One saved report definition incl. its dataRequest filters.",
	"QBO.REPORTS.MEMORIZED_LIST":                    "Memorized custom report registry behind /app/customreports (v4/entities reportDefinitions; MEMORIZED+MEMORIZEDGROUP). --id tail is the classic mem_rpt_id.",
	"QBO.REPORTS.MEMORIZED_READ":                    "One memorized report definition incl. attributes, recurInfo, share scope, and child groups; --id accepts composite id or mem_rpt_id.",
	"QBO.REPORTS.MEMORIZED_RUN":                     "Execute a memorized report via v4/entities createReports_Report. No flags runs the saved period; --date-range/--basis replay the resolved saved filters (token, account whitelist, class) through the equivalent v3 report.",
	"QBO.REPORTS.TAX_SUMMARY_READ":                  "Tax summary via the v3 report service.",
	"QBO.REPORTS.TRANSACTION_LIST_BY_CUSTOMER_READ": "Transaction list by customer via the v3 report service.",
	"QBO.REPORTS.TRANSACTION_LIST_BY_VENDOR_READ":   "Transaction list by vendor via the v3 report service.",
	"QBO.REPORTS.TRANSACTION_LIST_WITH_SPLITS_READ": "Transaction list with splits via the v3 report service.",
	"QBO.REPORTS.TRIAL_BALANCE_FR_READ":             "French GAAP trial balance via the v3 report service.",
	"QBO.REPORTS.TRANSACTION_LIST_READ":             "Transaction list via the v3 report service.",
	"QBO.REPORTS.JOURNAL_REPORT_READ":               "Journal report via the v3 report service.",
	"QBO.SALES.INVOICE_SEND":                        "Email an invoice (POST /send); a 200 means mail went out.",
	"QBO.SALES.ESTIMATE_SEND":                       "Email an estimate (POST /send); a 200 means mail went out.",
	"QBO.SALES.CREDIT_MEMO_SEND":                    "Email a credit memo (POST /send); a 200 means mail went out.",
	"QBO.SALES.RECEIPT_SEND":                        "Email a sales receipt (POST /send); a 200 means mail went out.",
	"QBO.SALES.ESTIMATE_PDF":                        "Download an estimate as PDF bytes to --out.",
	"QBO.SALES.CREDIT_MEMO_PDF":                     "Download a credit memo as PDF bytes to --out.",
	"QBO.SALES.RECEIPT_PDF":                         "Download a sales receipt as PDF bytes to --out.",

	"QBO.ACCOUNTING.DEPARTMENT_CREATE":           "Department (location) tracking for Plus/Advanced; pairs with class tracking.",
	"QBO.ACCOUNTING.DEPARTMENT_DELETE":           "Delete a department; fails if in use.",
	"QBO.ACCOUNTING.DEPARTMENT_EDIT":             "Rename or reparent a department.",
	"QBO.ACCOUNTING.DEPARTMENT_READ":             "Read departments by id or list.",
	"QBO.ACCOUNTING.DEPARTMENT_SEARCH":           "Search departments by name.",
	"QBO.COMPANY.ATTACHABLE_CREATE":              "Upload a file as a v3 Attachable (multipart).",
	"QBO.COMPANY.ATTACHABLE_DELETE":              "Delete an attachable by id.",
	"QBO.COMPANY.ATTACHABLE_EDIT":                "Update attachable metadata (sparse Id/SyncToken).",
	"QBO.COMPANY.ATTACHABLE_READ":                "Read an attachable by id.",
	"QBO.COMPANY.ATTACHABLE_SEARCH":              "Search attachables.",
	"QBO.COMPANY.PREFERENCES_READ":               "Read company preferences (singleton query).",
	"QBO.EXPENSES.PURCHASE_SEARCH":               "Search Purchase transactions.",
	"QBO.EXPENSES.TIME_ACTIVITY_CREATE":          "Record vendor/employee time; drives time invoicing.",
	"QBO.EXPENSES.TIME_ACTIVITY_DELETE":          "Delete a time activity by id.",
	"QBO.EXPENSES.TIME_ACTIVITY_EDIT":            "Update a time activity (sparse Id/SyncToken).",
	"QBO.EXPENSES.TIME_ACTIVITY_READ":            "Read a time activity by id.",
	"QBO.EXPENSES.TIME_ACTIVITY_SEARCH":          "Search time activities.",
	"QBO.REPORTS.BALANCE_SHEET_READ":             "Balance Sheet via the v3 report service.",
	"QBO.REPORTS.CUSTOMER_BALANCE_READ":          "Customer balance report via the v3 report service.",
	"QBO.REPORTS.CUSTOMER_SALES_READ":            "Customer sales report via the v3 report service.",
	"QBO.REPORTS.PROFIT_LOSS_READ":               "Profit and Loss via the v3 report service.",
	"QBO.REPORTS.VENDOR_BALANCE_READ":            "Vendor balance report via the v3 report service.",
	"QBO.REPORTS.VENDOR_EXPENSES_READ":           "Vendor expenses report via the v3 report service.",
	"QBO.SALES.INVOICE_PDF":                      "Download a sales invoice as PDF bytes to --out.",
	"QBO.ADVANCED.SPREADSHEET_SYNC_IMPORT":       "Advanced/Accountant. Use Create or edit records templates; starred columns required. Validates on import. L0uPf2fZs.",
	"QBO.COMPANY.BILLS_IMPORT":                   "File import of bills (company lists). Pair with expenses bill import. First row maps. No bulk undo.",
	"QBO.COMPANY.CONTACT_FORM_CREATE":            "Lead-capture contact forms. AU help L4STWrFAS.",
	"QBO.CUSTOMERS.CUSTOMER_CREDIT_TRANSFER":     "Move credit between customers (usually a journal). L4vT5bz8K.",
	"QBO.CUSTOMERS.CUSTOMER_MERGE":               "Keep one profile; duplicate goes inactive and history moves. Recreate manually to undo. L5AOyW9kw / L6s6B3PQd.",
	"QBO.EXPENSES.BANK_FEE_CREATE":               "Record bank fees as an expense against the bank account. L83BBLKBK.",
	"QBO.EXPENSES.BILL_IMPORT":                   "CSV bill import on Essentials/Plus/Advanced. Map supplier, dates, GST. L4Q6QWsRw.",
	"QBO.EXPENSES.CHEQUE_BOUNCE":                 "Returned cheque: reverse via cheque or expense plus any bank fee. L4rxNrG0Y L4O144llZ.",
	"QBO.EXPENSES.CHEQUE_REPRINT":                "Reprint an existing cheque/sales form. Does not post a new payment. L6HMWrs2f.",
	"QBO.EXPENSES.GIFT_CERTIFICATE_CREATE":       "Vouchers bought from a supplier sit as an asset/prepaid until redeemed; wired as a v3 Purchase with the line on an Other Current Asset account (live-verified on TC2). L9Me3DImI.",
	"QBO.EXPENSES.SUPPLIER_MERGE":                "Transactions move to the kept supplier; duplicate becomes inactive. Choose keep carefully. L0NPj861i.",
	"QBO.INVENTORY.CONSIGNMENT_CREATE":           "Track consignment with classes + a custom report, not a separate stock type. L8z1aYyeA.",
	"QBO.PAYROLL.BPAY_CREATE":                    "AU pays via BPAY/ABA files, not US direct deposit. L7LkysE03.",
	"QBO.PAYROLL.DEDUCTION_CREATE":               "Create deduction categories first, then attach to the employee. CSA uses Payment Reference. L3nmCl2jx.",
	"QBO.PAYROLL.DEDUCTION_EDIT":                 "Edit an employee deduction; next pay run picks up the change. L3nmCl2jx.",
	"QBO.PAYROLL.DEDUCTION_READ":                 "List deductions on the employee or pay-run inclusion.",
	"QBO.PAYROLL.EMPLOYEE_TERMINATE":             "Finalise every open pay run first. Inactive after terminate; record turns pink with leave payout. STP reason required. L3ek5ukSc.",
	"QBO.PAYROLL.LEAVE_BALANCE_EDIT":             "Correct accrual hours; does not rewrite lodged STP by itself. L9ksKvxAc.",
	"QBO.PAYROLL.LEAVE_CATEGORY_CREATE":          "Leave categories including LSL. L53E6mh31 L9AJEJFec.",
	"QBO.PAYROLL.LEAVE_CATEGORY_EDIT":            "Edit leave category / LSL settings. L9AJEJFec.",
	"QBO.PAYROLL.LEAVE_CATEGORY_READ":            "Read leave categories configured for the company.",
	"QBO.PAYROLL.LUMP_SUM_CREATE":                "Method A or B(ii) for PAYG/STSL on lump sums. Employment Hero can calculate; ask the accountant which method. L1GYkpZ9U.",
	"QBO.PAYROLL.PAYG_SUMMARY_CREATE":            "STP-exempt employers only. After lodge, hover Status for the ATO receipt. L3FCpWWrh.",
	"QBO.PAYROLL.PAY_RUN_INCLUSION_CREATE":       "Recurring inclusion on future pay runs for one employee. L9ZSdRoiL.",
	"QBO.PAYROLL.STP_CLOSELY_HELD_LODGE":         "≤19 employees paying closely held staff must STP-report from 1 Jul 2021. Method: pay-day, quarterly, or estimate. L1oRPUfbZ.",
	"QBO.PAYROLL.STP_FINALISE":                   "EOFY finalisation. Lodge the last FY pay run first. Prefer a dedicated finalisation event over a bare update. L4B7SA6SB.",
	"QBO.PAYROLL.STP_MICRO_LODGE":                "Micro-employer quarterly STP concession. L8loR7gso.",
	"QBO.PAYROLL.STP_PAY_CREATE":                 "Lodge a pay event for a finalised pay run. Lodged events cannot be deleted. L5XLreOvf.",
	"QBO.PAYROLL.STP_RESET":                      "Reset incorrectly reported business/employee settings via an STP reset event. L5wfUkzSQ.",
	"QBO.PAYROLL.STP_UPDATE_CREATE":              "Lodge an update event (corrections / finalisation-via-update). L5kc65Mx6.",
	"QBO.PAYROLL.SUPER_RESC_EDIT":                "Correct reportable employer super that STP is sending wrong. L4RwsRmGD.",
	"QBO.PAYROLL.WORKCOVER_CREATE":               "Workers compensation / WorkCover via a pay category. L6fnya3rA.",
	"QBO.REPORTS.FORECAST_CREATE":                "Advanced forecasts — not the same as a budget. L7SlFsgsy.",
	"QBO.REPORTS.FORECAST_EDIT":                  "Edit an Advanced forecast. L7SlFsgsy.",
	"QBO.REPORTS.FORECAST_READ":                  "Read an Advanced forecast. L7SlFsgsy.",
	"QBO.REPORTS.MANAGEMENT_CREATE":              "Management reports are an Accountant bundle, not a standard P&L. L90RAh2XZ.",
	"QBO.REPORTS.MANAGEMENT_EDIT":                "Reorder management-report pages. L5sNxCAWt.",
	"QBO.REPORTS.MANAGEMENT_READ":                "View a management report — --id returns the ordered page table (cover → per-property detail reports → summary → distributions); reportToken is the mem_rpt_id each page executes. Stored page dates are last-saved config, not the generated period. L90RAh2XZ.",
	"QBO.REPORTS.MANAGEMENT_AUDIT":               "Execute every REPORT page of a management report per month and reconcile: per-unit detail nets must equal the combined summary; all-time distribution pages are sliced per period. Flags whitelist/class coverage drift between constituents. L90RAh2XZ.",
	"QBO.SALES.ESTIMATE_COPY":                    "Copy creates a new quote with the same lines; customer can change. Recurring is better for repeats. L1xisZLFD.",
	"QBO.SALES.INVOICE_COPY":                     "Duplicates the invoice. Change customer if charging someone else. L93xhzbxn L1xisZLFD.",
	"QBO.SALES.INVOICE_DISCOUNT":                 "Turn discounts on in settings. Percent of subtotal or a negative-rate item. Turning off mid-invoice disables the setting company-wide. L3VyP6cwJ.",
	"QBO.SALES.INVOICE_PROGRESS":                 "Split a quote into partial invoices. Enable progress invoicing first. L0Ymm6WjR.",
	"QBO.SALES.INVOICE_REMIND":                   "Automatic or one-off invoice reminders. L84cQjpxo.",
	"QBO.SALES.INVOICE_WRITE_OFF":                "Accountant write-off of an unpaid invoice to bad debt. L5tnLclDb.",
	"QBO.SALES.INVOICE_CUSTOMISE":                "Same surface as the wired `company form-style update` — the customise-forms editor (template, logo, fields, appearance) persists through txnsrendering.api.intuit.com/v2/customizations, proven live 2026-09-19. The invoice-context entry point hits the identical document; wiring a second command would duplicate the contract. Stays blocked as a semantic alias.",
	"QBO.SALES.TIME_INVOICE_CREATE":              "Auto-invoice unbilled time on Essentials/Plus/Advanced. Then Sales → Invoices. L0M6412kT.",
	"QBO.TAX.GST_DEFERRED_CREATE":                "Record deferred GST liability. L9K8N1rLK.",
	"QBO.PAYROLL.PAY_RUN_READ":                   "Live drive 2026-09-17: /app/team and /app/employees on TC2 (AU) render the shell only — AU payroll rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com contract. All payroll actions stay blocked.",
	"QBO.COMPANY.TAG_READ":                       "Live drive 2026-09-17: /app/tags renders an empty-state shell with no tag-service call; QBO retired tag creation 16 May 2025 (prefer custom fields). No read contract observed — stays blocked.",
	"QBO.ADVANCED.CUSTOM_ROLES_CREATE":           "Live probe 2026-09-19: the Roles tab on /app/usermgt does expose 'Add role' (/app/accesses/create renders a full permission matrix) and the real document was recovered from the access-management-ui bundle — createRole(with: RoleInput{name,description,roleType,accountId,subRoles,namespace,constraints,accountPermissionGroups}). Replay with a faithful input over the captured transport reaches the service and is denied: 403 PERMISSION_DENIED ClientErrorCode 1048. The UI's own check agrees — authz-decision.api.intuit.com/v2/authorize on resource irn:intuit::authz:rpas:roledefinition action=create returns DENY 'Insufficient privileges for role definition mutations' for userid 9411837596151089 on realm 9341457769756854. Identity/app-key scope lacks role-definition writes; no replayable header set changes it. Stays blocked.",
	"QBO.ADVANCED.CUSTOM_ROLES_EDIT":             "Same roles.api.intuit.com surface — updateRole(with: RoleInput, id: ID) document recovered; a faithful no-op update against live role 16525683 returns the same 403 PERMISSION_DENIED ClientErrorCode 1048 while reads return 200. Role writes are denied for the TC2 identity (authz-decision roledefinition:create = DENY). Stays blocked.",
	"QBO.COMPANY.ROLE_CREATE":                    "Same roles.api.intuit.com surface as advanced custom-roles — createRole document recovered and replayed; service denies writes (403 PERMISSION_DENIED / ClientErrorCode 1048) and authz-decision denies roledefinition:create for the TC2 identity. Stays blocked.",
	"QBO.COMPANY.ROLE_DELETE":                    "deleteRole(input: DeleteRoleInput{roleId,accountId}) document recovered; a probe with a bogus roleId passes RPAS authorization and fails downstream (400 'No such element is present', not PERMISSION_DENIED) — delete authz differs from create/update. It cannot be safely live-proven on a real role because createRole is denied for this identity, so a deletion would be unrecoverable. Stays blocked.",
	"QBO.COMPANY.ROLE_EDIT":                      "Same roles.api.intuit.com surface — updateRole document recovered and replayed as a no-op on role 16525683; service returns 403 PERMISSION_DENIED ClientErrorCode 1048 and authz-decision denies roledefinition mutations for the TC2 identity. Stays blocked.",
	"QBO.COMPANY.USER_CREATE":                    "User invites send a real Intuit Account email to the invitee — external side effect, never sent from this CLI. Stays blocked.",
	"QBO.COMPANY.USER_DELETE":                    "Removing a user revokes real access on the company — destructive account change. Live probe 2026-09-19: no user mutation exists in the access-management-ui bundle (only createRole/updateRole/deleteRole) and removeUser/updateUser are FieldUndefined on both identity.api and roles.api — no captured contract on the AU usermgt surface. Stays blocked.",
	"QBO.COMPANY.USER_EDIT":                      "Editing a user changes real access/roles on the company. Live probe 2026-09-19: the access-management-ui bundle carries no user-update mutation and updateUser is FieldUndefined on both identity.api/v2 and roles.api — no captured document on the AU build. Stays blocked.",
	"QBO.ADVANCED.TASKS_CREATE":                  "Live-proven 2026-09-19: taskManagementCreateTask on smallbusiness.api.intuit.com/graphql. Requires intuit_target_domain=QBO + intuit_target_usecase=TASK_MANAGER + tasks-ui apikey headers; input needs name, status=Open, assignee (identity profile id), RFC3339 dueDate.",
	"QBO.REPORTS.FORECAST_DELETE":                "Live-proven 2026-09-19: deleteBusinessForecasts(forecastIds:[ID!]!) on planningforecasting.api.intuit.com/graphql — hard delete returning per-id {status,description}.",
	"QBO.REPORTS.REPORT_EDIT":                    "Live-proven 2026-09-19: universalreportinsights GET+PUT /v1/reports/qbo/{sbg:id} — update resends the full {name,dataRequest}.",
	"QBO.REPORTS.REPORT_DELETE":                  "Live-proven 2026-09-19: universalreportinsights DELETE /v1/reports/qbo/{sbg:id} — soft-delete (active:false, name gets '(deleted - …)' suffix).",
	"QBO.ADVANCED.EXPENSE_CLAIMS_READ":           "Live drive 2026-09-17: /app/expenseclaims and /app/expense-claims both 404 on TC2 — feature absent. Stays blocked.",
	"QBO.ADVANCED.REVENUE_RECOGNITION_EDIT":      "Live probe 2026-09-17: catalog ops UPDATE_RECOGNITION_FREQUENCY/UpdateDeferredRecognitionScheduleOp/IMMEDIATE_RECOGNIZE_SCHEDULE answer PLT-2000 on v4 and 'Cannot query field updateRecognitionFrequency' on the real deferral host sbseggraphqlorch — the mutation fields are absent from this realm's schema. Stays blocked.",
	"QBO.SALES.DELAYED_CHARGE_CREATE":            "Live probe 2026-09-17: POST /delayedcharge on TC2 (AU) answers 'Operation delayedcharge is not supported' (code 500) — the entity is US-only and absent on this realm. Stays blocked.",
	"QBO.SALES.DELAYED_CREDIT_CREATE":            "Live probe 2026-09-17: POST /delayedcredit on TC2 (AU) answers 'Operation delayedcredit is not supported' (code 500) — the entity is US-only and absent on this realm. Stays blocked.",
	"QBO.TAX.BAS_LODGE":                          "indirect-tax-ui EfileTaxReturn (IndirectTax_UpdateReturnInput) exists in the captured catalog but executes a real ATO lodgement — irreversible regulatory submission; never wired from this CLI. Stays blocked.",
	"QBO.TAX.GST_AMENDMENTS_READ":                "Live probe 2026-09-17: TaxAdjustments node query resolves on a TaxReturn id but the adjustments field only exists on Businesstaxes_TaxCategory — AU realms have no businessTaxJurisdictions so no category node exists. Stays blocked.",
	"QBO.TAX.GST_PAYMENT_CREATE":                 "Record the BAS/GST payment leaving the bank. Not the same as lodge. L8uFzN3iU. Live probe 2026-09-17: indirectTaxCreatePayment fails PLT-2000 provider error on the v4 gateway regardless of input; the AU GST centre routes payments to help docs — no in-product mutation observed. Stays blocked.",
	"QBO.TAX.GST_PAYMENT_DELETE":                 "Delete a recorded GST payment. L8GIJyLoa.",
	"QBO.TAX.GST_REFUND_CREATE":                  "Record an ATO GST refund. L9ThKp2a1.",
	"QBO.TAX.LODGEIT_EXPORT":                     "Import QBO BAS GST/PAYGW into LodgeiT, then pre-lodge. L2MFMlqt5 L4EUJdqX3.",
	"QBO.ACCOUNTING.RECONCILE_SERVICE_GET":       "accountreconcile.api.intuit.com was mapped from the service catalog but never fired in live traffic — the reconcile UI actually rides qbo.intuit.com/api/v4/graphql reads + qbonline-aws.api.intuit.com mutations (Start/Finish/Cancel proven on TC2 2026-09-17). No standalone service endpoint observed; stays blocked.",
	"QBO.EXPENSES.BILL_PAY_SERVICE_CREATE":       "billpaysvc.api.intuit.com executes online bill payments on bank rails (qbo-billpay UI). Debit scheduling is service-side; not wired.",
	"QBO.ACCOUNTING.BUDGET_PLANNING_GET":         "budgeting.api.intuit.com/graphql (BusinessPlanning_* mutations) is the budgeting-ui backend the wired v3 budget fallback deliberately avoids after capture-era 403s.",
	"QBO.ACCOUNTING.COA_TEMPLATES_GET":           "coa-core.api.intuit.com/graphql serves shared chart-of-accounts templates and account purpose mappings (sharedcoa surface).",
	"QBO.COMPANY.DIMENSIONS_GRAPHQL_GET":         "dimensions.api.intuit.com/graphql (dimensions-ui) manages custom dimension definitions and assignments; /app/dimensions/assignment in the capture.",
	"QBO.COMPANY.CUSTOM_EXTENSIONS_LIST":         "customextensions.api.intuit.com/graphql (customization-ui, crm-leads-ui chunk 8429) lists per-entity custom extension recommendations (BulkUpdateCustomExtensionRecommendations).",
	"QBO.COMPANY.COMPANY_MANAGEMENT_GET":         "company-management-svc.api.intuit.com/graphql (customization-ui) reads multi-company group membership and company settings orchestration.",
	"QBO.ACCOUNTING.DEFERRED_RECOGNITION_GET":    "Fetch the recognition schedule for a source txn by --source-id via deferredrecognition GET_SCHEDULE (the ATS path this command replays). Needs a schedule-carrying txn id, not a display id.",
	"QBO.ACCOUNTANT.PORTFOLIO_STORE_GET":         "client-portfolio-store.api.intuit.com/graphql (tasks-ui) stores the accountant's client portfolio across companies.",
	"QBO.FEED.BANK_ACCOUNT_LOOKUP_GET":           "bankaccountlookup.api.intuit.com resolves institutions/accounts when linking a bank feed (qbo-billpay ipd bundle shares it).",
	"QBO.FEED.ACH_TRANSFER_CREATE":               "ach-aws.api.intuit.com moves money over ACH rails from the bill-pay flow. R4: real-money movement; never wired from this CLI.",
	"QBO.FEED.TXN_VERIFY":                        "Completeness oracle for the bank feed: compares fetched pending rows against the server numTxnToReview count. Exit non-zero on mismatch; run before and after any mutation round-trip.",
	"QBO.FEED.TXN_POPULATION_ALL":                "Full pending walk across every connected account with a per-account completeness manifest. Slow on large companies; cap with --max-pages-per-account.",
	"QBO.FEED.TXN_TAG":                           "Live drive 2026-09-19: the feed-row editor exposes no tag control; tagging a posted transaction rides the tag service whose writes are retirement-gated (QBO retired tag creation 16 May 2025 — prefer custom fields; COMPANY.TAG_* writes blocked on the same gate). Stays blocked.",
	"QBO.INTEGRATIONS.APPFLOW_RUN":               "appflow.api.intuit.com synchronises third-party app connections (crm-leads-ui chunk 4190 references it).",
	"QBO.EXPENSES.BILL_PAYMENT_READ":             "Read a bill-payment by id (v3 BillPayment query). Payments link bills to bank/credit accounts; deleting one unapplies it.",
	"QBO.EXPENSES.BILL_PAYMENT_DELETE":           "Delete a bill-payment by id, unapplying it from the bill. Added after the audit proved payments were otherwise orphaned (no read/delete path).",
	"QBO.ACCOUNTING.BUDGET_LIST":                 "List budgets from the budgeting backend (same store as budget get). Pair with a budget get to compare plan vs actuals.",
	"QBO.ACCOUNTING.CLASS_DELETE":                "Deactivate a class by id (soft-delete; pass active=false). Classes with history go inactive rather than disappearing.",
	"QBO.ACCOUNTING.LOCATION_DELETE":             "Deactivate a department/location by id (soft-delete; pass active=false).",
	"QBO.ACCOUNTING.LOCATION_EDIT":               "Rename or deactivate a department/location by id. Names must stay unique per company.",
	"QBO.ACCOUNTING.OPENING_BALANCE_CREATE":      "Book the trial balance as an opening-balance journal (v3 journalentry POST). R3: posts real balances; verify in the register after.",
	"QBO.ACCOUNTING.PREPAID_CREATE":              "Create a prepaid schedule from a JSON payload (GET_SCHEDULE path). Preview with --draft before writing.",
	"QBO.ACCOUNTING.DEFERRED_RECOGNITION_CREATE": "Create a deferred-revenue schedule from a JSON payload. Preview with --draft before writing.",
	"QBO.SALES.LATE_FEE_CREATE":                  "automaticlatefees.api.intuit.com (sales-delivery-ui) accrues late fees on overdue invoices automatically once configured.",
	"QBO.SALES.KNOWLEDGE_BASE_SEARCH":            "c1-kb-svc.api.intuit.com answers product questions (/v1/knowledge-base/questions-answers) for in-app help surfaces.",
	"QBO.SALES.COMMERCE_CONTROL_GET":             "commercecontrol.api.intuit.com/graphql hosts order-management/P&S list mutations; the gql gateway already routes captured commerce ops there — this row documents the service itself.",
	"QBO.EXPENSES.B2B_BILLPAY_GET":               "b2bbillpay.api.intuit.com (spend-overview-ui) is the business-to-business bill-pay enrolment and payment-status backend.",
	"QBO.EXPENSES.B2B_NETWORK_GET":               "b2bnetworkservice.api.intuit.com resolves supplier identities/network memberships for B2B bill pay.",
	"QBO.EXPENSES.BILL_PAY_ONBOARDING_GET":       "bill-pay-onboarding-svc.api.intuit.com tracks bill-pay enrolment state (spend-overview-ui).",
	"QBO.COMPANY.CONSENT_GET":                    "consent-aws.api.intuit.com records user consent grants (/v2/consent), e.g. banking data sharing, referenced by the reconcile UI.",
	"QBO.COMPANY.CONSUMER_NOTIFICATION_POST":     "consumernotification.api.intuit.com/v1/events/browser delivers browser-channel notification events (ecosystem-notes-plugin).",
	"QBO.COMPANY.CONTENT_ACCESS_GET":             "contentaccess.api.intuit.com/v2/content/search gates in-app help content by subscription entitlements.",
	"QBO.CUSTOMERS.PROPOSAL_SVC_CREATE":          "Live-proven 2026-09-19: crm-proposal-svc.api.intuit.com POST /v1/proposals {contactId,contactType} — same surface as CUSTOMERS.PROPOSAL_CREATE (service-map row).",
	"QBO.CUSTOMERS.PROPOSAL_EDIT":                "Live-proven 2026-09-19: crm-proposal-svc GET+PUT /v1/proposals/{refId} — update keys by the refId UUID and requires the full object, not a patch.",
	"QBO.CUSTOMERS.PROPOSAL_DELETE":              "Live-proven 2026-09-19: crm-proposal-svc DELETE /v1/proposals/{numericId} — delete keys by the numeric id (refId is rejected) and soft-flags isDeleted.",
	"QBO.CUSTOMERS.SALES_AGENT_GET":              "crm-sales-agent.api.intuit.com is the AI sales agent conversation store behind CRM lead follow-up.",
	"QBO.CUSTOMERS.SALES_AGENT_PROMPT_POST":      "crm-sales-agent-prompt.api.intuit.com evaluates prompts for the AI sales agent; prompt content leaves the company record.",
	"QBO.COMPANY.CUSTOMIZATION_AGENT_POST":       "customization-agent.api.intuit.com (cost-groups-ui/customization-ui chunks) turns natural-language requests into form/list customizations.",
	"QBO.COMPANY.DATA_EXPORT_GET":                "c7.qbo.intuit.com neo POST /reports/exportReport — the /app/exportdata Export-to-Excel surface; one POST per report token returns the xlsx attachment.",
	"QBO.COMPANY.DATA_MIRROR_GET":                "data-dev.api.intuit.com is a non-production mirror of the data platform observed in ecosystem-notes-plugin; listed for completeness.",
	// --- service map v2 (2026-08-24): remaining never-triggered production services ---
	"QBO.ACCOUNTING.FIT_TRANSACTIONS_GET":        "fitransactions.api.intuit.com/graphql (accounting-ledger-ui 5898) is the FIT GraphQL host banking-feed mutations route to; the gql gateway already targets it for BankingDisconnectOlbAccounts — this row documents the service.",
	"QBO.ACCOUNTING.LIBRO_GET":                   "libro-svc.api.intuit.com (accounting-ledger-ui 5898) backs the ledger bookkeeping views (libro = book/ledger store).",
	"QBO.ACCOUNTING.MIDMARKET_SETTINGS_GET":      "mid-market-settings-svc.api.intuit.com/graphql (accounting-ledger-ui 5898) reads Advanced/Mid-Market company settings the ledger UI toggles.",
	"QBO.ACCOUNTING.MULTIENTITY_GET":             "multientityorchs.api.intuit.com (accounting-ledger-ui 4326) orchestrates multi-entity consolidation across linked companies.",
	"QBO.ACCOUNTING.QBDATA_MIGRATION_POST":       "qbdatamigration.api.intuit.com/v1 (customization-ui 6882) drives QBO data migration jobs between companies.",
	"QBO.ACCOUNTING.TAXCONFIG_MIRROR_GET":        "taxconfig-test.api.intuit.com (accounting-ledger-ui 5898) is a non-prod mirror of the tax-config service captured inside a prod ledger bundle; listed for completeness.",
	"QBO.ADVANCED.ACS_GET":                       "accountantclientservices.api.intuit.com/v4/graphql (qbo-billpay ipd, accounting-ledger-ui 5898) serves accountant-client services shared by bill pay and ledger surfaces.",
	"QBO.ADVANCED.ERP_BUILDER_LIST":              "erp-builder-svc.api.intuit.com/v1 (customization-ui 6882) builds custom ERP-style app pages from templates.",
	"QBO.COMPANY.CHORUS_GET":                     "chorus.api.intuit.com/graphql (dimensions-ui) orchestrates dimensions and agent-builder flows alongside the dimensions service.",
	"QBO.COMPANY.CUSTOMIZATION_AGENT_QA_POST":    "customization-agent-qa.api.intuit.com/v1 (accounting-ledger-ui 5898) is the QA mirror of the natural-language customization agent; listed for completeness.",
	"QBO.COMPANY.ENVELOPE_POST":                  "envelope.api.intuit.com/v4/graphql (qbo-billpay ipd) wraps bill-pay requests in approval envelopes; envelope contents can authorise payments.",
	"QBO.COMPANY.EXPERIMENT_ASSIGNMENT_GET":      "experimentassignment.api.intuit.com/api/v3/assignments/ (indirect-tax-ui 7316) returns A/B experiment bucket assignments for the session.",
	"QBO.COMPANY.EXPERT_CREDENTIALS_GET":         "expertcredentials.api.intuit.com/graphql (ecosystem-notes-plugin 703) verifies QuickBooks Live expert credentials shown beside notes.",
	"QBO.COMPANY.EXPERT_ENGAGEMENT_POST":         "expertengagement.api.intuit.com/graphql (app-revx-ui 9159) books/updates engagement sessions with QuickBooks Live experts.",
	"QBO.COMPANY.FRANCHISE_ORCH_POST":            "franchise-orch-svc.api.intuit.com/v1 (dimensions-ui) orchestrates franchise (multi-location rollup) flows.",
	"QBO.COMPANY.GENPLUGIN_REGISTRY_GET":         "genpluginregistry.api.intuit.com (business-explorer 4919) is the generated-plugin registry the business explorer resolves plugins from.",
	"QBO.COMPANY.GEOCODE_GET":                    "geocode.api.intuit.com/geocode/v1 (mileage-page-ui) resolves addresses to coordinates for mileage tracking.",
	"QBO.COMPANY.IDENTITY_QA_GET":                "identity-qa.api.intuit.com/v2/graphql (accounting-ledger-ui 5898) is a non-prod identity graph mirror captured in a prod bundle; listed for completeness.",
	"QBO.COMPANY.INPUT_TO_ACTION_POST":           "inputtoaction.api.intuit.com/v1/business_feed (business-explorer 4919) turns business-feed inputs into notification/action items.",
	"QBO.COMPANY.MODELEXECUTION_POST":            "modelexecution.api.intuit.com/v2/smart-compose-*/predict (ecosystem-notes-plugin 703, crm-leads-ui 8429, order-management-ui 2137) runs smart-compose text-prediction models over draft content.",
	"QBO.COMPANY.QBDATAMIGRATION_STATUS_GET":     "qbdatamigration.api.intuit.com/v1 (customization-ui 6882) migration-job status endpoint observed beside the customization surface.",
	"QBO.COMPANY.QBLIVE_EXPERIENCE_GET":          "qbliveexperience.api.intuit.com (business-explorer 6758) powers live in-app guidance/experience surfaces.",
	"QBO.COMPANY.SWIFT_NONCAT_POST":              "swift-non-cat-request.api.intuit.com/graphql (ecosystem-notes-plugin 703) proxies non-categorised Swift helper requests from notes surfaces.",
	"QBO.CUSTOMERS.MAILCHIMP_NOTIFICATIONS_POST": "mailchimpnotifications.api.intuit.com (qbo-mailchimp-crm-ui 7148) delivers Mailchimp journey/notification events into QBO.",
	"QBO.CUSTOMERS.MCF_BACKEND_GET":              "mcfbackend.api.intuit.com (crm-leads-ui 4190) backs the Multi-Channel-Funnel CRM lead views.",
	"QBO.CUSTOMERS.MC_APPFABRIC_GET":             "mc-appfabricproxy.api.intuit.com (crm-leads-ui 4190, order-management-ui 3275) proxies Mailchimp app-fabric calls into QBO surfaces.",
	"QBO.CUSTOMERS.PROMPT_EVALUATOR_POST":        "prompt-evaluator-svc.api.intuit.com (crm-leads-ui 4190) scores/validates prompts before they reach CRM AI agents.",
	"QBO.CUSTOMERS.REALTIME_ER_GET":              "realtimeentityresolution.api.intuit.com (crm-leads-ui 4190, crm-bulk-import) dedupes leads/contacts by resolving entities in real time during import.",
	"QBO.CUSTOMERS.APPOINTMENT_READ":             "Live drive 2026-09-19: /app/appointments on TC2 renders the 'Start Scheduling Now' landing only — the scheduling feature was never onboarded; only a CORS preflight to calendarmgmt.api.intuit.com/graphql fired, no data request. Onboarding mutates company config (public booking page) — stays blocked on the fixture.",
	"QBO.CUSTOMERS.APPOINTMENT_SEARCH":           "Same surface as APPOINTMENT_READ — calendarmgmt.api.intuit.com requires the scheduling onboarding TC2 never completed. Stays blocked.",
	"QBO.CUSTOMERS.REVIEWS_GET":                  "reviews.api.intuit.com/v4/graphql (app-revx-ui 9159) serves app-store style reviews/ratings for apps in the ecosystem.",
	"QBO.EXPENSES.PAYMENTS_FUNDING_GET":          "paymentsfunding-aws.api.intuit.com (money-payments-overview-ui 8117) orchestrates payout funding legs. R3: money-movement plumbing; never wired.",
	"QBO.EXPENSES.PAYMENT_ACCOUNT_GET":           "paymentaccount.api.intuit.com (money-payments-overview-ui 8117) reads payment-account instruments used to fund payouts.",
	"QBO.EXPENSES.PYMTS_APPS_ORCH_POST":          "pymtsappsorchestration.api.intuit.com (money-payments-overview-ui 8117) orchestrates payments-app workflows across payment surfaces.",
	"QBO.EXPENSES.RESOLUTION_CENTER_GET":         "resolution-center-svc.api.intuit.com (money-payments-overview-ui 8117) lists payment issues and their resolution steps.",
	"QBO.EXPENSES.SPENDMGMT_GET":                 "spendmgmt.api.intuit.com/graphql (qbo-billpay ipd, integrations-banking-reconcile-ui 1775) is the spend-management GraphQL backend behind bill pay and reconcile.",
	"QBO.EXPENSES.STAGETXN_TEST_GET":             "stagetransactions-test.api.intuit.com/stage/entities (spend-overview-ui 1945) is a non-prod transaction-staging mirror; listed for completeness.",
	"QBO.EXPENSES.SUBSCRIPTION_SBG_GET":          "subscription-sbg-aws.api.intuit.com/v4/graphql (money-payments-overview-ui 8117, indirect-tax-experiences, qbo-billpay ipd) resolves small-business subscription entitlements incl. e-filing eligible offers.",
	"QBO.EXPENSES.TXN_INGESTION_POST":            "txn-ingestion-svc.api.intuit.com/v1 (integrations-receipts-ui batch-ingestion bundle) ingests batches of receipt-derived transactions.",
	"QBO.EXPENSES.WALLET_GET":                    "wallet-aws.api.intuit.com (qbo-billpay ipd, qbo-contacts-v2 6098) stores customer/vendor wallets holding stored payment credentials. R2: PCI-sensitive surface.",
	"QBO.FEED.FINANCIAL_DATA_INTERACTION_POST":   "financialdatainteraction-aws.api.intuit.com (integrations-banking-reconcile-ui 8120) logs user interactions with financial data surfaces.",
	"QBO.FEED.FINANCIAL_DOCUMENT_GET":            "financialdocumentpci.api.intuit.com (integrations-banking-reconcile-ui 8120, smartdocs-web-platform 9206) fetches PCI-guarded financial documents (statements).",
	"QBO.FEED.FINANCIAL_PROFILE_GET":             "financialprofile-aws.api.intuit.com (integrations-banking-reconcile-ui 8120) reads the financial profile backing linked bank accounts.",
	"QBO.FEED.FINANCIAL_PROFILE_ORCH_POST":       "financialprofileorchestration-aws.api.intuit.com (integrations-apptransactions-ui 12357) orchestrates financial-profile refresh flows for linked apps.",
	"QBO.FEED.FINANCIAL_PROVIDER_GET":            "financialprovider-aws.api.intuit.com (integrations-banking-reconcile-ui 8120) identifies the provider/institution behind each linked feed account.",
	"QBO.FEED.TXNS_ORCHESTRATION_POST":           "txnsorchestrationsvc.api.intuit.com/graphql (integrations-banking-reconcile-ui 1775) orchestrates bank-transaction classification/posting pipelines. Live drive 2026-09-19: internal orchestration facade, never user-facing — the posting contract it fronts is batchAcceptTransactions, wired as TXN_BATCH_ACCEPT/CATEGORISE/TRANSFER. Stays blocked.",
	"QBO.FEED.VAULT_GET":                         "vault*.api.intuit.com/v1[/oauth] (smartdocs-web-platform 5225; -dev/-awse2e/-awsprf/-awsqal mirrors captured) guards document-vault OAuth sessions. R1: credential-adjacent surface; never wired.",
	"QBO.INTEGRATIONS.IDX_GET":                   "idx.api.intuit.com (integrations-apptransactions-ui 12357) fronts Intuit Data Exchange lookups for app transactions.",
	"QBO.INTEGRATIONS.V4_AWS_POST":               "v4-aws.api.intuit.com (sales-view-ui invoice-drawer en-au) is an AWS v4-gateway variant of the app-transactions API; the CLI uses qbo.intuit.com/api/v4 instead — this row documents the host.",
	"QBO.INTEGRATIONS.WORKFLOW_AUTOMATION_GET":   "workflowautomation.api.intuit.com (qbo-workflows-taskmanager-ui) is the workflow-automation backend behind task-manager automations.",
	"QBO.INVENTORY.INVENTORY_AGENTS_POST":        "inventory-agents.api.intuit.com (products-and-services-core 6952; -perf sibling captured) hosts inventory assistant agents (reorder/restock suggestions).",
	"QBO.INVENTORY.INVENTORY_COST_GET":           "inventorycostaccounting.api.intuit.com (products-and-services-core 6952; -perf sibling captured) computes inventory cost layers/valuation (FIFO/average).",
	"QBO.INVENTORY.ITEM_MANAGEMENT_GET":          "item-management-svc.api.intuit.com/graphql (products-and-services-core 6952) serves product/service item CRUD behind the P&S list.",
	"QBO.INVENTORY.QBBULKACTION_POST":            "qbbulkaction.api.intuit.com/v4/graphql (business-explorer 4919 PRODUCT_BULK_ACTION) executes bulk actions over product/inventory selections. R2: batch mutations; never wired.",
	"QBO.INVENTORY.WAREHOUSE_SERVICE_GET":        "warehouse-management-svc.api.intuit.com/graphql (inventory-addon-ui 1156, order-management-ui 3275, products-and-services-core) hosts Commerce_* inventory ops the gql gateway already routes there — this row documents the service itself.",
	"QBO.SALES.QBONLINE_GRAPHQL_GET":             "qbonline.api.intuit.com[/v4/graphql] (products-and-services-core 6952/7565, crm-doc-builder-ui 4574; -stg/-perf siblings captured) is an alternate QBO v4 gateway host; the CLI drives qbo.intuit.com/api/v4 instead — this row documents the host.",
	"QBO.SALES.SALESTXN_RISK_GET":                "salestransactions.api.intuit.com[/v4/graphql] (money-payments-overview-ui 8117, order-management-ui 6502, sales-view-ui) aggregates sales transactions incl. risk signals across payment surfaces.",
	"QBO.SALES.SALES_AI_CALL_POST":               "sales-ai-svc.api.intuit.com/api/v1 (sales-view-ui invoice-drawer en-au) schedules AI sales calls and fetches call details per invoice.",
	"QBO.SALES.SALES_CHECKOUT_GET":               "salescheckout.api.intuit.com/v2/sale/{id}/activities (sales-view-ui invoice-drawer en-au) tracks sale/checkout activities per invoice.",
	"QBO.SALES.SALES_SETTINGS_GET":               "salesettings.api.intuit.com (sales-delivery-ui 258, money-payments-overview-ui 8117, business-explorer SALES_SETTINGS key) stores sales-form settings (late-fee policy, numbering, defaults).",
	"QBO.SALES.SALES_TRAFFIC_ROUTER_POST":        "sales-traffic-router.api.intuit.com[/v4/graphql] (sales-delivery-ui 258, sales-view-ui) routes sales-surface requests to backing services.",
	"QBO.SALES.TXNS_RENDERING_POST":              "txnsrendering.api.intuit.com (qbo-contacts-v2 4168/7542, order-management-ui 6502) renders sales documents (PDF/layout) for transactions.",
	"QBO.TAX.EXPERIMENT_ASSIGNMENT_GET":          "experimentassignment.api.intuit.com/api/v3/assignments/ (indirect-tax-ui 7316) returns A/B buckets steering which tax UX the SPA renders.",
	"QBO.TAX.INDIRECT_REPORTS_GET":               "indirecttaxreports.api.intuit.com (indirect-tax-experiences widget `modernizedTaxLiabilityApiUrl`) serves modernised indirect-tax liability reports.",
	"QBO.TAX.NEXUS_GET":                          "salestax-nexus.api.intuit.com (indirect-tax-experiences widget `nexusApiUrl`) resolves sales-tax nexus jurisdictions for the company.",
	"QBO.TAX.TAX_CALCULATION_POST":               "taxcalculationsvc.api.intuit.com (indirect-tax-experiences widget, order-management-ui 6502; -perf sibling captured) computes transaction tax. R1: sends line data off-company.",
	"QBO.TAX.TAX_FILING_GET":                     "taxfilingsvc.api.intuit.com[/graphql] (indirect-tax-experiences widget `filingServiceTaxLiabilityApiUrl`, indirect-tax-ui 4204) reads tax liability/filing state. R2: lodging adjacent.",
	"QBO.COMPANY.COMPANY_INFO_READ":              "CompanyInfo singleton holds legal name, address, and fiscal-year settings; the v3 query returns one row for the connected company.",
	"QBO.TAX.TAX_AGENCY_READ":                    "Tax agencies back GST/VAT and payroll tax filings; read agencies before creating tax codes or lodging BAS returns.",
	"QBO.TAX.TAX_RATE_READ":                      "Tax rates carry RateValue and agency references used by sales and payroll lines; verify the rate before posting taxed transactions.",
	"QBO.REPORTS.AGED_PAYABLES_READ":             "AgedPayables report ages unpaid bills by due-date bucket; v3 name is plural (singular 400s on TC2 2026-09-08). Pair with a fiscal-period date-range.",
	"QBO.REPORTS.AGED_RECEIVABLES_READ":          "AgedReceivables report ages open invoices by due-date bucket; v3 name is plural (singular 400s on TC2 2026-09-08). Use it to chase overdue balances.",
	"QBO.REPORTS.GENERAL_LEDGER_READ":            "GeneralLedger report lists every posted txn line by account; the company-wide detail backing the trial balance totals.",
	"QBO.REPORTS.TRIAL_BALANCE_READ":             "TrialBalance report proves debits equal credits per account; run it after journals and before BAS preparation.",
	"QBO.PAYROLL.EMPLOYEE_CREATE":                "Create an employee with DisplayName plus GivenName/FamilyName; verify with payroll employee get before rostering shifts.",
	"QBO.PAYROLL.EMPLOYEE_EDIT":                  "Rename an employee DisplayName by id via sparse update; confirm the new name with payroll employee get afterwards.",
	"QBO.COMPANY.TERM_CREATE":                    "Payment terms need Name plus DueDays (server 400s Name-only as Date-driven on TC2 2026-09-08); create the term before quoting customers.",
	"QBO.COMPANY.TERM_EDIT":                      "Rename a payment term by id; the update carries DueDays/Type or the server 400s (proven on TC2 2026-09-08 term 1000000031).",
	"QBO.COMPANY.TERM_DELETE":                    "Delete deactivates via Active=false (v3 Delete op unsupported for Term); terms leave the default list but stay fetchable with --active all.",
	"QBO.COMPANY.TERM_READ":                      "List payment terms available on sales forms; pair with company payment-method get for checkout setup.",
	"QBO.COMPANY.PAYMENT_METHOD_CREATE":          "Payment methods label how customers pay (cash, card, bank transfer); create before enabling checkout options.",
	"QBO.COMPANY.PAYMENT_METHOD_EDIT":            "Rename a payment method by id via sparse update; confirm with company payment-method get afterwards.",
	"QBO.COMPANY.PAYMENT_METHOD_DELETE":          "Delete deactivates via Active=false (v3 Delete op unsupported); methods leave the default list but stay fetchable with --active all.",
	"QBO.COMPANY.PAYMENT_METHOD_READ":            "List payment methods offered at checkout; verify new methods appear here right after creation.",
	"QBO.COMPANY.COMPANY_INFO_EDIT":              "Edit the CompanyInfo singleton CompanyName field; read it back with company company-info get to confirm the round-trip.",
	"QBO.FEED.ACCOUNT_REFRESH":                   "Pull latest bank transactions for an account (manualUpdate); fetched rows land For review. Live drive 2026-09-19: TC2 feed acct 44 is a CSV-upload Standard Feed — no Update/refresh control on the banking card; manualUpdate exists only on bank-connected feeds (blocked account-link lane). Stays blocked.",
	"QBO.EXPENSES.STAGETXN_GET":                  "stagetransactions.api.intuit.com/stage/entities is the production transaction-staging backend; the test mirror has its own row. Not wired.",
	"QBO.COMPANY.IDENTITY_GET":                   "identity.api.intuit.com/v2/graphql serves session and user bootstrap traffic seen on banking cold load; listed for completeness. Not wired.",
	"QBO.COMPANY.SETTINGS_FACADE_GET":            "The settingsfacade backend serves company settings surfaces; distinct from the v3 Preferences read. Not wired.",
	"QBO.GQL.BILLS_WALK":                         "Walk all bills via GetBills across pages with an optional transaction-date window; relay-executed live read.",
	"QBO.GQL.TASKS_WALK":                         "Walk all tasks via TaskManagementTasks with an optional due-date window; relay-executed live read.",
	"QBO.GQL.ITEMS_WALK":                         "Walk the product and service catalog via Items with paging; relay-executed live read.",
	"QBO.GQL.MUTATE_CREATE_ACCOUNT":              "Create a ledger account via CreateAccount at coa-core.api.intuit.com/graphql with the SPA lower-camel input shape; name and subtype are required.",
	"QBO.GQL.MUTATE_BANK_DISCONNECT":             "Disconnect bank-feed OLB accounts via the FIT transactions gateway; feeds stop syncing for those accounts.",
	"QBO.GQL.MUTATE_BATCH_UPDATE_PRODUCTS":       "Bulk-update products from a JSON row array via Commerce batchUpdateProducts; verify with a walk first.",
	"QBO.GQL.MUTATE_ASSIGN_DIMENSIONS":           "Assign custom dimension values to products; the input object passes through verbatim to the mutation.",
	"QBO.GQL.MUTATE_CANCEL_RECONCILE":            "Cancel an in-flight banking reconciliation session; prefer finishing the reconcile instead.",
	"QBO.GQL.MUTATE_RECEIVE_INVENTORY":           "Record inbound warehouse stock against a source Bill via CommerceReceiveInventory with movement lines.",
	"QBO.GQL.MUTATE_CONSUME_INVENTORY":           "Record outbound warehouse stock against a source Invoice via CommerceConsumeInventory with movement lines.",
	"QBO.GQL.MUTATE_ACTIVATE_PRODUCTS":           "Reactivate inactive products or services by id or full JSON batch; confirm with an items walk.",
	"QBO.COSTGROUPS.GROUP_LIST":                  "List cost groups via GetCostGroupsWithFilter. Live-proven 2026-09-19: 200 with real rows (deleted=true groups need --status all; default filter is active-only).",
	"QBO.COSTGROUPS.GROUP_CREATE":                "Create a cost group via CreateCostGroup; name required, type defaults to ITEM like the command.",
	"QBO.COSTGROUPS.GROUP_UPDATE":                "Rename a cost group via UpdateCostGroup; id and name required, version guards concurrent edits.",
	"QBO.COSTGROUPS.GROUP_DELETE":                "Soft-delete a cost group via ToggleCostGroupDeletion; pass --activate to undo the delete.",
	"QBO.CUSTOMOBJECTS.OBJECT_CREATE":            "Live drive 2026-09-19: /app/customobjects 404s on TC2 and appFoundationsCustomObjectDefinitions is FieldUndefined on the v4 gateway — the app-foundations schema is unprovisioned on this realm. Stays blocked.",
	"QBO.CUSTOMOBJECTS.OBJECT_UPDATE":            "Live drive 2026-09-19: /app/customobjects 404s on TC2; the app-foundations schema is unprovisioned on this realm (FieldUndefined), so no definition-edit surface exists. Stays blocked.",
	"QBO.CUSTOMOBJECTS.OBJECT_DELETE":            "Live drive 2026-09-19: /app/customobjects 404s on TC2; the app-foundations schema is unprovisioned on this realm (FieldUndefined), so no definition-delete surface exists. Stays blocked.",
	"QBO.ACCOUNTING.MY_ACCOUNTANT_READ":          "My-accountant surfaces are gated on the accountant link for the fixture company; the read returns the unlinked state but never reaches a richer payload. Not wired.",
	"QBO.ADVANCED.EXPENSE_CLAIMS_CREATE":         "Expense-claims feature is absent on the fixture company — the claims routes return 404 with no module load. Create has no reachable surface; stays blocked.",
	"QBO.ADVANCED.EXPENSE_CLAIMS_REVIEW":         "Expense-claims feature is absent on the fixture company — the claims routes return 404 with no module load. Review has no reachable surface; stays blocked.",
	"QBO.ADVANCED.REVENUE_RECOGNITION_SETUP":     "Revenue-recognition mutation fields are absent from the fixture realm's GraphQL schema — the feature flag never enabled them. Setup has no reachable surface; stays blocked.",
	"QBO.COMPANY.ACCOUNTANT_INVITE":              "Inviting an accountant sends a real external email invitation through the Intuit account flow — a live side effect on a person-facing surface with no captured in-tenant contract. Stays blocked.",

	"QBO.COMPANY.TAG_CREATE":           "Tags were retired from QBO on 16 May 2025; the tag service and UI affordances no longer exist. Stays blocked — no live endpoint.",
	"QBO.COMPANY.TAG_DELETE":           "Tags were retired from QBO on 16 May 2025; the tag service and UI affordances no longer exist. Stays blocked — no live endpoint.",
	"QBO.COMPANY.TAG_EDIT":             "Tags were retired from QBO on 16 May 2025; the tag service and UI affordances no longer exist. Stays blocked — no live endpoint.",
	"QBO.CUSTOMERS.OPPORTUNITY_CREATE": "Opportunity create is the same crm-proposal-svc lead surface already wired under qb crm (create/update/delete live-proven); this catalog row is a semantic alias. Stays blocked to avoid duplicate wiring.",
	"QBO.CUSTOMERS.OPPORTUNITY_READ":   "Opportunity read is the same crm-proposal-svc lead surface already wired under qb crm; this catalog row is a semantic alias. Stays blocked to avoid duplicate wiring.",
	"QBO.CUSTOMERS.OPPORTUNITY_SEARCH": "Opportunity search is the same crm-proposal-svc lead surface already wired under qb crm; this catalog row is a semantic alias. Stays blocked to avoid duplicate wiring.",
	"QBO.EXPENSES.BILL_LIST_CREATE":    "Batch transactions (multi-form list create/edit) is a US-only Advanced feature; the AU Advanced fixture's batch routes soft-404 with no module. Stays blocked — feature absent by region.",
	"QBO.EXPENSES.BILL_LIST_EDIT":      "Batch transactions (multi-form list create/edit) is a US-only Advanced feature; the AU Advanced fixture's batch routes soft-404 with no module. Stays blocked — feature absent by region.",
	"QBO.EXPENSES.EXPENSE_LIST_CREATE": "Batch transactions (multi-form list create/edit) is a US-only Advanced feature; the AU Advanced fixture's batch routes soft-404 with no module. Stays blocked — feature absent by region.",
	"QBO.EXPENSES.EXPENSE_LIST_EDIT":   "Batch transactions (multi-form list create/edit) is a US-only Advanced feature; the AU Advanced fixture's batch routes soft-404 with no module. Stays blocked — feature absent by region.",
	"QBO.SALES.INVOICE_LIST_EDIT":      "Batch transactions (multi-form list create/edit) is a US-only Advanced feature; the AU Advanced fixture's batch routes soft-404 with no module. Stays blocked — feature absent by region.",
	"QBO.FEED.ACCOUNT_LINK":            "Bank-link launches a live financial-institution connection flow (FI OAuth + consent) against a real external institution — an external side effect outside the test company's data boundary. Stays blocked.",
	"QBO.PAYROLL.OVERVIEW_SEARCH":      "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com endpoint; no in-tenant read contract exists. Stays blocked.",
	"QBO.PAYROLL.PAY_CATEGORY_CREATE":  "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com endpoint; no in-tenant create contract exists. Stays blocked.",
	"QBO.PAYROLL.PAY_CATEGORY_EDIT":    "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com endpoint; no in-tenant update contract exists. Stays blocked.",
	"QBO.PAYROLL.PAY_CATEGORY_READ":    "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com endpoint; no in-tenant read contract exists. Stays blocked.",
	"QBO.PAYROLL.PAY_RUN_CREATE":       "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface; creating a pay run is a real wage-payment side effect with no in-tenant contract. Stays blocked.",
	"QBO.PAYROLL.SETUP_RUN":            "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface; setup mutates real payroll configuration with no in-tenant contract. Stays blocked.",
	"QBO.PAYROLL.SUPER_PAY":            "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface; super payments are real remittance side effects with no in-tenant contract. Stays blocked.",
	"QBO.PAYROLL.SUPER_READ":           "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com endpoint; no in-tenant read contract exists. Stays blocked.",
	"QBO.PAYROLL.TEAM_READ":            "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com endpoint; no in-tenant read contract exists. Stays blocked.",
	"QBO.PAYROLL.TEAM_SEARCH":          "AU payroll on the fixture rides the external quickbooks.yourpayroll.com.au (KeyPay) surface, not an api.intuit.com endpoint; no in-tenant read contract exists. Stays blocked.",
	"QBO.TAX.ATO_CONNECT":              "Connecting to the ATO is a real regulatory authority link (TFN/ABN credentialing) with an external government side effect — outside the test company's data boundary. Stays blocked.",
	"QBO.TAX.CODE_DELETE":              "The indirect-tax service family is unreachable on the fixture — the compliance-hub feature flag is off and the v3 taxservice route is unsupported on the AU build. No mutation surface exists; stays blocked.",
	"QBO.TAX.CODE_EDIT":                "The indirect-tax service family is unreachable on the fixture — the compliance-hub feature flag is off and the v3 taxservice route is unsupported on the AU build. No mutation surface exists; stays blocked.",
	"QBO.TAX.GST_SETTINGS_EDIT":        "GST settings ride the same indirect-tax service family — unreachable on the fixture (compliance-hub flag off, taxservice unsupported on the AU build). Stays blocked.",
	"QBO.TAX.GST_SETUP":                "GST setup mutates the company's tax registration and rides the unreachable indirect-tax service family (compliance-hub flag off on the fixture). Stays blocked.",
	"QBO.TAX.TPAR_EXPORT":              "TPAR export rides the indirect-tax/contractor surface which is unreachable on the fixture (compliance-hub flag off), and a real export is a regulatory filing artifact. Stays blocked.",
}
