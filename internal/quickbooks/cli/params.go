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
	if id == "QBO.FEED.TXN_LOOKUP" {
		params, _ := wiredParams(id)
		return params
	}
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
	if id == "QBO.EXPENSES.EXPENSE_EDIT" {
		out = append(out, p("memo", "string", "replace the non-empty expense memo; omitted preserves it", false, ""))
	}
	// Fields the v3 builders already honour (TxnDate via transactionDateFlag,
	// PrivateNote via --memo) that the generated catalog never declared —
	// omitting them forced backdated expenses and bill memo edits into the UI.
	for _, x := range map[string][]paramDoc{
		"QBO.EXPENSES.EXPENSE_CREATE": {
			p("txn-date", "string", "posting date dd/MM/yyyy or yyyy-MM-dd; defaults to today — required for prior-period expenses", false, ""),
			p("memo", "string", "PrivateNote memo on the expense — link provider account, service unit, invoice/receipt", false, ""),
			p("payment-type", "string", "Cash, Check, or CreditCard; set CreditCard when the funding account is a card", false, ""),
		},
		"QBO.EXPENSES.CHEQUE_CREATE": {
			p("txn-date", "string", "posting date dd/MM/yyyy or yyyy-MM-dd; defaults to today — required for prior-period cheques", false, ""),
			p("memo", "string", "PrivateNote memo on the cheque — link provider account, service unit, invoice/receipt", false, ""),
			p("billable", "string", "true/false — marks the synthesized expense line BillableStatus", false, ""),
			p("project", "string", "project/customer-ref id applied to the synthesized line's ProjectRef", false, ""),
		},
		"QBO.EXPENSES.EXPENSE_EDIT": {
			p("txn-date", "string", "move the posting date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("payee", "string", "move the expense to a different payee entity id (vendor/customer/employee)", false, ""),
		},
		"QBO.EXPENSES.BILL_CREATE": {
			p("memo", "string", "PrivateNote memo on the bill — link provider account, service unit, invoice reference", false, ""),
			p("billable", "string", "true/false — marks the synthesized expense line BillableStatus", false, ""),
			p("project", "string", "project/customer-ref id applied to the synthesized line's ProjectRef", false, ""),
		},
		"QBO.EXPENSES.BILL_EDIT": {
			p("txn-date", "string", "move the bill date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("due-date", "string", "bill due date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing due date", false, ""),
			p("memo", "string", "PrivateNote memo on the bill — link provider account, service unit, invoice reference", false, ""),
		},
		"QBO.EXPENSES.BILL_PAYMENT_CREATE": {
			p("txn-date", "string", "payment date dd/MM/yyyy or yyyy-MM-dd; defaults to today — required for prior-period payments", false, ""),
			p("pay-type", "string", "Check or CreditCard; omitted is inferred from the funding account type", false, ""),
			p("payment-method", "string", "payment method id (PaymentMethodRef) recorded on the payment", false, ""),
			p("supplier-credit-ids", "string", "comma-separated vendor credit ids applied to the payment as LinkedTxn lines", false, ""),
		},
		"QBO.EXPENSES.BILL_PAY": {
			p("txn-date", "string", "payment date dd/MM/yyyy or yyyy-MM-dd; defaults to today — required for prior-period payments", false, ""),
			p("pay-type", "string", "Check or CreditCard; omitted is inferred from the funding account type", false, ""),
			p("memo", "string", "PrivateNote memo; existing bracketed ledger annotations must be retained", false, ""),
		},
		"QBO.EXPENSES.BILL_PAYMENT_EDIT": {
			p("txn-date", "string", "move the payment date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo; existing bracketed ledger annotations must be retained", false, ""),
		},
		"QBO.EXPENSES.VENDOR_CREDIT_CREATE": {
			p("memo", "string", "PrivateNote memo on the vendor credit — link the originating bill or return", false, ""),
			p("billable", "string", "true/false — marks the synthesized expense line BillableStatus", false, ""),
			p("project", "string", "project/customer-ref id applied to the synthesized line's ProjectRef", false, ""),
		},
		"QBO.EXPENSES.VENDOR_CREDIT_EDIT": {
			p("txn-date", "string", "move the credit date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo; existing bracketed ledger annotations must be retained", false, ""),
			p("supplier", "string", "move the vendor credit to a different supplier id — sparse update, lines preserved", false, ""),
		},
		"QBO.INVENTORY.PURCHASE_ORDER_CREATE": {
			p("memo", "string", "PrivateNote memo on the purchase order — internal note, not sent to the supplier", false, ""),
			p("billable", "string", "true/false — marks the synthesized expense line BillableStatus", false, ""),
			p("project", "string", "project/customer-ref id applied to the synthesized line's ProjectRef", false, ""),
		},
		"QBO.INVENTORY.PURCHASE_ORDER_EDIT": {
			p("txn-date", "string", "move the PO date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo on the purchase order — internal note, not sent to the supplier", false, ""),
			p("supplier", "string", "move the purchase order to a different supplier id — sparse update, lines preserved", false, ""),
		},
		"QBO.EXPENSES.CHEQUE_EDIT": {
			p("txn-date", "string", "move the posting date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo; existing bracketed ledger annotations must be retained", false, ""),
			p("billable", "string", "true/false — BillableStatus on a single-line cheque (use --line-items for multi-line)", false, ""),
			p("project", "string", "project/customer-ref id applied to a single-line cheque's ProjectRef", false, ""),
		},
		"QBO.EXPENSES.BANK_FEE_CREATE": {
			p("memo", "string", "PrivateNote memo on the bank fee — e.g. statement period the fee covers", false, ""),
			p("billable", "string", "true/false — marks the synthesized expense line BillableStatus", false, ""),
			p("project", "string", "project/customer-ref id applied to the synthesized line's ProjectRef", false, ""),
		},
		"QBO.EXPENSES.GIFT_CERTIFICATE_CREATE": {
			p("memo", "string", "PrivateNote memo on the purchase — link supplier or invoice reference", false, ""),
			p("billable", "string", "true/false — marks the synthesized expense line BillableStatus", false, ""),
			p("project", "string", "project/customer-ref id applied to the synthesized line's ProjectRef", false, ""),
		},
		"QBO.ACCOUNTING.TRANSFER_CREATE": {
			p("memo", "string", "PrivateNote memo on the transfer — describe what the movement is for", false, ""),
		},
		"QBO.ACCOUNTING.DEPOSIT_CREATE": {
			p("line-items", "string", "JSON array of full v3 Deposit line objects — add-funds lines appended after --payments links", false, ""),
			p("memo", "string", "PrivateNote memo on the deposit — add-funds lines may also carry their own memos", false, ""),
		},
		"QBO.ACCOUNTING.DEPOSIT_EDIT": {
			p("account", "string", "move the deposit into a different bank account id (DepositToAccountRef)", false, ""),
			p("line-items", "string", "JSON array of full v3 Deposit line objects; replaces all existing lines", false, ""),
		},

		"QBO.ACCOUNTING.JOURNAL_EDIT": {
			p("line-items", "string", "JSON array of full v3 JournalEntry line objects; replaces all existing lines", false, ""),
		},
		"QBO.ACCOUNTING.RECURRING_CREATE": {
			p("customer", "string", "customer id the recurring invoice template bills (required by the builder)", true, ""),
			p("amount", "string", "AUD amount for the generated invoice line", false, ""),
			p("name", "string", "template name stored on RecurringInfo.Name", false, ""),
			p("item", "string", "item id for the generated sales line (defaults to a generic service item)", false, ""),
			p("line-items", "string", "JSON array of full v3 sales line objects for the template", false, ""),
			p("end-date", "string", "schedule end date (ScheduleInfo.EndDate) dd/MM/yyyy", false, ""),
			p("next-date", "string", "next run date (ScheduleInfo.NextDate) dd/MM/yyyy", false, ""),
		},
		"QBO.SALES.INVOICE_CREATE": {
			p("memo", "string", "PrivateNote memo on the invoice — internal note, not shown to the customer", false, ""),
			p("email", "string", "BillEmail billing address on the invoice; omitted preserves the existing one", false, ""),
			p("address", "string", "billing street line 1 — combined with --city/--state/--postcode into BillAddr", false, ""),
			p("city", "string", "billing address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "billing address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "billing address postcode — combined with --address/--city/--state into BillAddr", false, ""),
		},
		"QBO.SALES.INVOICE_EDIT": {
			p("txn-date", "string", "move the invoice date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("due-date", "string", "invoice due date dd/MM/yyyy or yyyy-MM-dd", false, ""),
			p("memo", "string", "PrivateNote memo on the invoice — internal note, not shown to the customer", false, ""),
			p("email", "string", "BillEmail billing address on the invoice; omitted preserves the existing one", false, ""),
			p("line-items", "string", "JSON array of full v3 Line objects; replaces all existing lines", false, ""),
		},
		"QBO.SALES.ESTIMATE_CREATE": {
			p("memo", "string", "PrivateNote memo on the estimate — internal note, not shown to the customer", false, ""),
			p("email", "string", "BillEmail address on the estimate; omitted preserves the existing one", false, ""),
			p("address", "string", "billing street line 1 — combined with --city/--state/--postcode into BillAddr", false, ""),
			p("city", "string", "billing address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "billing address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "billing address postcode — combined with --address/--city/--state into BillAddr", false, ""),
		},
		"QBO.SALES.ESTIMATE_EDIT": {
			p("txn-date", "string", "move the estimate date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo on the estimate — internal note, not shown to the customer", false, ""),
			p("email", "string", "BillEmail address on the estimate; omitted preserves the existing one", false, ""),
			p("line-items", "string", "JSON array of full v3 Line objects; replaces all existing lines", false, ""),
		},
		"QBO.SALES.CREDIT_MEMO_CREATE": {
			p("memo", "string", "PrivateNote memo on the credit memo — link the originating invoice or return", false, ""),
			p("address", "string", "billing street line 1 — combined with --city/--state/--postcode into BillAddr", false, ""),
			p("city", "string", "billing address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "billing address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "billing address postcode — combined with --address/--city/--state into BillAddr", false, ""),
			p("email", "string", "customer BillEmail address set on the credit memo", false, ""),
		},
		"QBO.SALES.CREDIT_MEMO_EDIT": {
			p("txn-date", "string", "move the credit date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo on the credit memo — link the originating invoice or return", false, ""),
			p("line-items", "string", "JSON array of full v3 Line objects; replaces all existing lines", false, ""),
		},
		"QBO.SALES.RECEIPT_CREATE": {
			p("memo", "string", "PrivateNote memo on the sales receipt — link the payment or register entry", false, ""),
			p("address", "string", "billing street line 1 — combined with --city/--state/--postcode into BillAddr", false, ""),
			p("city", "string", "billing address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "billing address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "billing address postcode — combined with --address/--city/--state into BillAddr", false, ""),
			p("email", "string", "customer BillEmail address set on the sales receipt", false, ""),
		},
		"QBO.SALES.RECEIPT_EDIT": {
			p("txn-date", "string", "move the receipt date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo on the sales receipt — link the payment or register entry", false, ""),
			p("line-items", "string", "JSON array of full v3 Line objects; replaces all existing lines", false, ""),
		},
		"QBO.SALES.REFUND_RECEIPT_CREATE": {
			p("memo", "string", "PrivateNote memo on the refund receipt — link the original charge being refunded", false, ""),
			p("address", "string", "billing street line 1 — combined with --city/--state/--postcode into BillAddr", false, ""),
			p("city", "string", "billing address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "billing address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "billing address postcode — combined with --address/--city/--state into BillAddr", false, ""),
			p("email", "string", "customer BillEmail address set on the refund receipt", false, ""),
		},
		"QBO.SALES.REFUND_RECEIPT_EDIT": {
			p("txn-date", "string", "move the refund date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo on the refund receipt — link the original charge being refunded", false, ""),
			p("line-items", "string", "JSON array of full v3 Line objects; replaces all existing lines", false, ""),
		},
		"QBO.SALES.PAYMENT_CREATE": {
			p("memo", "string", "PrivateNote memo on the payment — e.g. deposit batch or remittance reference", false, ""),
		},
		"QBO.SALES.PAYMENT_EDIT": {
			p("txn-date", "string", "move the payment date dd/MM/yyyy or yyyy-MM-dd; omitted preserves the existing date", false, ""),
			p("memo", "string", "PrivateNote memo on the payment — e.g. deposit batch or remittance reference", false, ""),
			p("amount", "string", "payment amount; the existing single-line allocation is rescaled to match", false, ""),
		},
		"QBO.EXPENSES.SUPPLIER_CREATE": {
			p("city", "string", "supplier address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "supplier address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "supplier address postcode — combined with --address/--city/--state into BillAddr", false, ""),
			p("phone", "string", "supplier phone (PrimaryPhone); omitted preserves the existing phone", false, ""),
			p("email", "string", "supplier email (PrimaryEmailAddr); omitted preserves the existing email", false, ""),
		},
		"QBO.EXPENSES.SUPPLIER_EDIT": {
			p("email", "string", "supplier email (PrimaryEmailAddr); omitted preserves the existing email", false, ""),
			p("phone", "string", "supplier phone (PrimaryPhone); omitted preserves the existing phone", false, ""),
			p("city", "string", "supplier address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "supplier address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "supplier address postcode — combined with --address/--city/--state into BillAddr", false, ""),
			p("active", "string", "true/false — deactivate or reactivate the supplier; omitted leaves status unchanged", false, ""),
		},
		"QBO.CUSTOMERS.CUSTOMER_CREATE": {
			p("address", "string", "billing address line 1 — combined with --city/--state/--postcode into BillAddr", false, ""),
			p("city", "string", "billing address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "billing address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "billing address postcode — combined with --address/--city/--state into BillAddr", false, ""),
		},
		"QBO.CUSTOMERS.CUSTOMER_EDIT": {
			p("name", "string", "display name (renames the customer); omitted preserves the existing name", false, ""),
			p("email", "string", "customer email (PrimaryEmailAddr); omitted preserves the existing email", false, ""),
			p("phone", "string", "customer phone (PrimaryPhone); omitted preserves the existing phone", false, ""),
			p("address", "string", "billing address line 1 — combined with --city/--state/--postcode into BillAddr", false, ""),
			p("city", "string", "billing address city — combined with --address/--state/--postcode into BillAddr", false, ""),
			p("state", "string", "billing address state — combined with --address/--city/--postcode into BillAddr", false, ""),
			p("postcode", "string", "billing address postcode — combined with --address/--city/--state into BillAddr", false, ""),
			p("active", "string", "true/false — deactivate or reactivate the customer; omitted leaves status unchanged", false, ""),
		},
		"QBO.PAYROLL.EMPLOYEE_EDIT": {
			p("email", "string", "employee email (PrimaryEmailAddr); omitted preserves the existing email", false, ""),
			p("active", "string", "true/false — deactivate or reactivate the employee; omitted leaves status unchanged", false, ""),
		},
		"QBO.INVENTORY.ITEM_EDIT": {
			p("name", "string", "item name (renames the item); omitted preserves the existing name", false, ""),
			p("sku", "string", "stock keeping unit (Sku); omitted preserves the existing sku", false, ""),
			p("income-account", "string", "income account id for the item (IncomeAccountRef); omitted preserves it", false, ""),
			p("expense-account", "string", "COGS/expense account id for the item (ExpenseAccountRef); omitted preserves it", false, ""),
			p("sales-price", "string", "unit sales price (UnitPrice); omitted preserves the existing price", false, ""),
			p("purchase-cost", "string", "unit purchase cost (PurchaseCost); omitted preserves the existing cost", false, ""),
			p("description", "string", "item sales description; omitted preserves the existing description", false, ""),
			p("active", "string", "true/false — deactivate or reactivate the item; omitted leaves status unchanged", false, ""),
		},
		"QBO.INVENTORY.ITEM_CREATE": {
			p("expense-account", "string", "COGS/expense account id (Inventory type); defaults to account 40", false, ""),
			p("asset-account", "string", "inventory asset account id (alias of --inventory-asset-account); defaults to 42", false, ""),
			p("qty", "string", "opening quantity on hand (alias of --initial-qty); defaults to 1 for Inventory", false, ""),
			p("description", "string", "item sales description; omitted preserves the existing description", false, ""),
			p("txn-date", "string", "inventory tracking start date (InvStartDate) for Inventory items; defaults to today", false, ""),
		},
		"QBO.INVENTORY.ADJUST_CREATE": {
			p("account", "string", "adjustment GL account id (required) — the offset account for the quantity change", true, ""),
			p("doc-number", "string", "adjustment reference number (DocNumber); defaults to QB-ADJ-<HHMMSS>", false, ""),
		},

		// Drift-audit declarations: flags the v3 builders consume that the
		// catalog never declared (consumed-but-unattached was the silent-drop
		// class; these make the surface match the builder).
		"QBO.ACCOUNTING.BUDGET_CREATE": {
			p("end-date", "string", "budget end date dd/MM/yyyy; defaults to the AU financial year end after start", false, ""),
			p("date", "string", "budget start fallback dd/MM/yyyy when neither --start-date nor --fiscal-year is set", false, ""),
		},
		"QBO.COMPANY.TERM_EDIT": {
			p("active", "string", "true/false — deactivate or reactivate the term; omitted leaves status unchanged", false, ""),
			p("type", "string", "term type (e.g. NET/DATE_DRIVEN) — sparse overwrite of the existing Type", false, ""),
			p("due-days", "string", "days until due (DueDays); omitted preserves the existing value", false, ""),
		},
		"QBO.COMPANY.PAYMENT_METHOD_EDIT": {
			p("active", "string", "true/false — deactivate or reactivate the payment method", false, ""),
		},
		"QBO.ACCOUNTING.DEPARTMENT_EDIT": {
			p("active", "string", "true/false — deactivate or reactivate the department", false, ""),
		},
		"QBO.PAYROLL.TIMESHEET_EDIT": {
			p("date", "string", "activity date dd/MM/yyyy (TxnDate); omitted preserves the existing date", false, ""),
			p("name-of", "string", "worker kind: Employee or Vendor (NameOf); switches which ref flag applies", false, ""),
			p("description", "string", "activity description text on the timesheet row", false, ""),
		},
	}[id] {
		if !hasParam(out, x.Name) {
			out = append(out, x)
		}
	}
	if id == "QBO.SALES.INVOICE_REMIND" {
		out = append(out, p("send", "bool", "explicitly send a real email; default is preview; verification harnesses refuse sending", false, "false"), p("to", "string", "explicit recipient email; otherwise invoice/customer email", false, ""))
	}
	if id == "QBO.FEED.TXN_CATEGORISE" {
		for i := range out {
			if out[i].Name == "class" {
				out[i].Help = "class id; none clears the existing suggestion; omitted preserves it"
			}
		}
	}
	return canonicalTimeHelp(canonicalFileHelp(canonicalLineItemsHelp(out, id), id), id)
}

// ruleFlagDocs returns the save flags shared by rule create/edit; withID
// swaps --name to required-on-edit semantics via the --id flag.
func ruleFlagDocs(withID bool) []paramDoc {
	out := []paramDoc{}
	if withID {
		out = append(out, p("id", "string", "bank rule id to update (required)", true, ""))
		out = append(out, p("name", "string", "rule name; omitted keeps the existing name", false, ""))
	} else {
		out = append(out, p("name", "string", "display name for the bank rule (required)", true, ""))
	}
	return append(out,
		p("money", "string", "transaction direction the rule matches: in or out (default out)", false, ""),
		p("match", "string", "condition combination: all (default) or any", false, ""),
		p("account-ids", "string", "comma-separated bank-feed account ids the rule applies to; omit for all accounts", false, ""),
		p("category-id", "string", "GL account id applied as the transaction category", false, ""),
		p("payee-id", "string", "vendor or customer id applied as the transaction payee", false, ""),
		p("customer-id", "string", "customer id applied to matching transactions", false, ""),
		p("class-id", "string", "class id applied to matching transactions", false, ""),
		p("location-id", "string", "location id applied to matching transactions", false, ""),
		p("memo", "string", "memo text applied to matching transactions", false, ""),
		p("auto-add", "string", "true auto-posts matching pending transactions to the register", false, ""),
		p("exclude", "string", "true makes this an exclude rule for matching pending rows", false, ""),
		p("disabled", "string", "true saves the rule in a disabled state (off)", false, ""),
	)
}

// ruleConditionFlagDocs returns the condition flags (bank text, description,
// amount) shared by rule create/edit.
func ruleConditionFlagDocs() []paramDoc {
	return []paramDoc{
		p("bank-text", "string", "match on bank text (statement detail as sent by the bank)", false, ""),
		p("bank-text-op", "string", "bank-text operator: contains (default), is_exactly, does_not_contain", false, ""),
		p("bank-text-exclude", "string", "additional bank-text Does not contain condition (for example FEE); combines with --bank-text using ALL", false, ""),
		p("bank-text-exclude-2", "string", "second bank-text Does not contain condition; requires --bank-text and --bank-text-exclude", false, ""),
		p("description", "string", "match on the bank transaction description text", false, ""),
		p("description-op", "string", "description operator: contains (default), is_exactly, does_not_contain", false, ""),
		p("amount", "string", "match on the bank transaction amount value", false, ""),
		p("amount-op", "string", "amount operator: equals (default), does_not_equal, is_greater_than, is_less_than", false, ""),
	}
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
		p("limit", "int", "unsupported for reports; explicit use is rejected (reports are not paginated entity lists)", false, "0"),
	}
	for _, rf := range reportFilterFlags {
		help := rf.help
		if report == "TAXABLE_PAYMENTS" {
			help = "unsupported by TAXABLE_PAYMENTS; explicit non-empty use is rejected"
		}
		ps = append(ps, p(rf.flag, "string", help, false, ""))
	}
	ps = append(ps, p("param", "strings", "additional report param key=value, repeatable — forwarded verbatim to the v3 report service (klass, account, customer, vendor, department, item, term, doc_num, group_by, sort_by, sort_order, date_macro, columns, cleared, printed, qzoom, subcolumns, aging_period, num_periods, past_due, aging_method, apaccount, araccount, appaid, payment_method, source_account_type, transaction_type, name, memo, custom1..3, start/end_created, start/end_moddate, start/end_duedate, duedate_macro, moddate_macro, percent_change); unknown keys are rejected", false, "[]"))
	if report == "TAXABLE_PAYMENTS" {
		ps[2].Help = "unsupported by TAXABLE_PAYMENTS; explicit non-empty use is rejected"
		ps[3].Help = "unsupported by TAXABLE_PAYMENTS; explicit non-empty use is rejected"
		ps[len(ps)-1].Help = "unsupported by TAXABLE_PAYMENTS; explicit non-empty use is rejected"
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
// Delayed charge/credit line-items are also corrected here, but to their own
// truth: {item, qty?, rate?, amount?} rows parsed by parseDelayedLines —
// not v3 line objects, and not the generated "item or account ... GST" text
// (the v4 non-posting contract carries products/services only).
func canonicalLineItemsHelp(ps []paramDoc, id string) []paramDoc {
	delayed := id == "QBO.SALES.DELAYED_CHARGE_CREATE" || id == "QBO.SALES.DELAYED_CREDIT_CREATE"
	for i, prm := range ps {
		if id == "QBO.SALES.CREDIT_CARD_CREDIT_CREATE" && prm.Name == "line-items" {
			ps[i].Help = "JSON array of {account, amount, description?} rows (v4 credit-card-credit contract)"
			continue
		}
		if delayed && prm.Name == "line-items" {
			ps[i].Help = "JSON array of {item, qty?, rate?, amount?, description?} — item is a product/service name or id; amount defaults to qty*rate, qty to 1"
			continue
		}
		if prm.Name == "line-items" || (prm.Name == "lines" && id != "QBO.FEED.TXN_SPLIT") ||
			(prm.Name == "items" && (id == "QBO.INVENTORY.PURCHASE_ORDER_CREATE" || id == "QBO.INVENTORY.PURCHASE_ORDER_EDIT")) {
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
		p("limit", "int", "max rows to return from the v3 query endpoint", false, "0"),
	}, true
}

func hasParam(list []paramDoc, name string) bool {
	for _, x := range list {
		if x.Name == name {
			return true
		}
	}
	return false
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
	case "QBO.GQL.MUTATE_BANK_DISCONNECT":
		// The live contract is neo lists/account/save on GL ids, not the
		// captured fitransactions op's olbAccountIds (403s in AU builds).
		return []paramDoc{
			p("id", "strings", "GL account id to disconnect, repeatable or comma-separated (required)", true, ""),
		}, true
	case "QBO.COMPANY.MARKETING_READ":
		// In-product marketing offers ride personalization.api
		// /v1/experience/ipd/placement/{placement} — captured 2026-09-19 on
		// /app/usermgt (AdvancedShellBanner → live offer row). --id selects
		// the placement; --limit maps to numberOfRecommendations.
		return []paramDoc{
			p("id", "string", "placement name (AdvancedShellBanner, UserMgtTop, QBOModalInterrupters, ...) — defaults to AdvancedShellBanner", false, ""),
			p("query", "string", "substring match on offer name, CTA text, or body copy (case-insensitive)", false, ""),
			p("limit", "int", "max offers to return (maps to the service's numberOfRecommendations)", false, "0"),
		}, true
	case "QBO.FEED.TXN_PENDING", "QBO.FEED.TXN_POSTED", "QBO.FEED.TXN_EXCLUDED":
		return []paramDoc{
			p("account-id", "string", "banking account id (see feed account list)", false, "204"),
			p("limit", "int", "max rows to fetch (X-Range page)", false, "0"),
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
	case "QBO.FEED.REC_AUTO_ADJUST":
		// FinishReconcile on the account's open session — the UI's
		// "accept the difference, book an adjusting entry" path. Field set
		// decoded from the reconcile-ui bundle and proven live TC2
		// 2026-09-19: reconcileAction FINISH + adjustingEntryDate +
		// adjustingEntryAccount{id} + adjustingEntryExchangeRate +
		// effectiveStatementEndingDate; session closes and a DocNumber ADJ
		// "Reconcile Adjustment" txn posts.
		return []paramDoc{
			p("account-id", "string", "bank account id with an open reconcile session (required)", true, ""),
			p("date", "string", "adjusting entry date yyyy-MM-dd (defaults to the statement ending date)", false, ""),
			p("adjust-account", "string", "adjusting entry account — v3 account id or Relay node id (defaults to the account's Reconciliation Discrepancies account)", false, ""),
			p("exchange-rate", "string", "adjusting entry exchange rate (defaults to 1, the home-currency rate)", false, ""),
			p("ending-date", "string", "statement ending date yyyy-MM-dd (defaults to the open session's date)", false, ""),
		}, true
	case "QBO.FEED.TXN_UNPOST":
		// undoTransactions with nextTxnInfo.reviewState ACCEPTED — the
		// Posted tab's Undo contract (verified live TC2 2026-09-19).
		return []paramDoc{
			p("ids", "strings", "posted transaction ids to undo back to pending — olbTxnId only, never display :ofx ids", true, ""),
			p("account-id", "string", "connected bank/credit-card account that owns the rows", false, "204"),
		}, true
	case "QBO.FEED.TXN_TRANSFER":
		// batchAcceptTransactions?acceptOnly=true with acceptType TRANSFER,
		// transfer:true, addAsQboTxn.txnTypeId 26 and details[0].categoryId
		// as the destination account (verified live TC2 2026-09-19).
		return []paramDoc{
			p("ids", "strings", "pending transaction ids to post as transfers — olbTxnId only, never display :ofx ids", true, ""),
			p("to-account-id", "string", "destination bank/credit card account id (required)", true, ""),
			p("account-id", "string", "connected bank/credit-card account that owns the rows", false, "204"),
		}, true
	case "QBO.FEED.TXN_ATTACH":
		// v3 Attachable link on a posted transaction: --file rides
		// POST /upload + sparse POST /attachable AttachableRef; --note
		// alone creates a note attachable already linked (verified live
		// TC2 2026-09-19 on Invoice 7).
		return []paramDoc{
			p("id", "string", "posted transaction id to attach to — target QBO record id (not a bank-feed :ofx display id)", true, ""),
			p("txn-type", "string", "v3 entity type of --id: Invoice, Bill, Purchase, Transfer, ... (required)", true, ""),
			p("note", "string", "note text to attach (optional when --file is given)", false, ""),
			p("file", "string", "file to upload and link — PDF/PNG/JPG/CSV/TXT; at least one of --note/--file is required", false, ""),
		}, true
	case "QBO.FEED.TXN_POPULATION_ALL":
		return []paramDoc{
			p("max-pages-per-account", "int", "page ceiling per account at 300 rows per page", false, "0"),
		}, true
	case "QBO.INVENTORY.ITEM_RECEIPT_CREATE":
		// Standalone single-line receipt; the catalog's PO-linked flag text
		// predates the captured warehouse contract (2026-09-16).
		return []paramDoc{
			p("vendor-id", "string", "supplier id the goods were received from (required)", true, ""),
			p("item-id", "string", "inventory item id being received (required)", true, ""),
			p("qty", "string", "received quantity recorded on line 1 (required)", true, ""),
			p("rate", "string", "per-unit cost applied to line 1 (required)", true, ""),
			p("date", "string", "receipt date dd/MM/yyyy (defaults to today)", false, ""),
			p("ref-no", "string", "receipt reference number shown on the receipt", false, ""),
			p("memo", "string", "free-text memo stored on the receipt record", false, ""),
			p("currency", "string", "ISO 4217 code; omitted uses the company home currency", false, ""),
		}, true
	case "QBO.INVENTORY.ITEM_RECEIPT_EDIT":
		return []paramDoc{
			p("id", "string", "item receipt id (required)", true, ""),
			p("qty", "string", "corrected received quantity applied to line 1", false, ""),
			p("rate", "string", "corrected per-unit cost applied to line 1", false, ""),
			p("date", "string", "receipt date dd/MM/yyyy to move the receipt to", false, ""),
			p("ref-no", "string", "receipt reference number shown on the receipt", false, ""),
			p("memo", "string", "free-text memo stored on the receipt record", false, ""),
		}, true
	case "QBO.INVENTORY.PURCHASE_ORDER_PARTIAL":
		// Partial receipt rides CommerceReceiveInventory against the PO's
		// lines; the catalog's bill-linked flag text predates the warehouse
		// contract (2026-09-16).
		return []paramDoc{
			p("id", "string", "purchase order id to receive against (required)", true, ""),
			p("items", "string", `JSON array of received lines, [{"line-id"|"item-id","qty":N}] (required)`, true, ""),
		}, true
	case "QBO.FEED.RULE_CREATE":
		// Neo lists/olbrules/save upsert; the catalog's free-text conditions
		// flag predates the captured conditionList/actionList wire model
		// (2026-09-16).
		return append(ruleFlagDocs(false), ruleConditionFlagDocs()...), true
	case "QBO.FEED.RULE_EDIT":
		return append(ruleFlagDocs(true), ruleConditionFlagDocs()...), true
	case "QBO.FEED.RULE_DELETE":
		return []paramDoc{
			p("id", "string", "bank rule id to delete (required)", true, ""),
		}, true
	case "QBO.EXPENSES.EXPENSE_RECATEGORISE":
		// v3 Purchase sparse update swapping AccountRef on account-based
		// lines; the catalog's generic flag text predates the wired contract.
		return []paramDoc{
			p("id", "string", "v3 Purchase (posted expense) id to recategorise (required)", true, ""),
			p("category-id", "string", "GL account id applied as the new category on account-based lines", false, ""),
			p("class-id", "string", "Class id on the selected line; none clears its Class; requires --line-id", false, ""),
			p("expected-sync-token", "string", "abort before mutation if the Purchase SyncToken differs from this expected value", false, ""),
			p("expected-category-id", "string", "abort if the selected line category changed", false, ""),
			p("expected-class-id", "string", "abort if the selected line Class changed; none for blank", false, ""),
			p("payee-id", "string", "vendor or customer id applied as the new payee (EntityRef)", false, ""),
			p("line-id", "string", "only recategorise this Purchase line id; omit for all account-based lines", false, ""),
			p("repair-credit-card-type", "bool", "explicitly convert a legacy Check funded by a verified credit-card account to CreditCard; may not be reversible", false, "false"),
		}, true
	case "QBO.ACCOUNTING.PREPAID_READ":
		// Schedule fetch keyed solely by --source-id; the cobra command
		// defines no --limit/--id.
		return []paramDoc{
			p("source-id", "string", "source transaction id carrying the schedule (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.BUDGET_LIST":
		return []paramDoc{
			p("limit", "int", "max budgets to fetch from the budgeting backend", false, "0"),
		}, true
	case "QBO.ACCOUNTING.CLASS_DELETE":
		return []paramDoc{
			p("id", "string", "target class id for deactivation (required)", true, ""),
		}, true
	case "QBO.ACCOUNTING.LOCATION_DELETE":
		return []paramDoc{
			p("id", "string", "target department id for deactivation (required)", true, ""),
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
	case "QBO.EXPENSES.GIFT_CERTIFICATE_CREATE":
		// v3 Purchase cash expense whose line posts to an Other Current
		// Asset (prepaid voucher) account instead of an expense account.
		// Verified live TC2 2026-09-19: supplier 3, cash side bank 44,
		// OCA line account 80, PaymentType Cash — deleted afterwards via
		// the form-delete path (v3 delete/void unsupported on AU build).
		return []paramDoc{
			p("supplier", "string", "supplier/vendor id the voucher was bought from (required)", true, ""),
			p("amount", "string", "AUD face value, held as a prepaid asset until redeemed (required)", true, ""),
			p("payment-account", "string", "bank or credit card account the voucher was paid from (required)", true, ""),
			p("asset-account", "string", "Other Current Asset account carrying the voucher until redemption (required)", true, ""),
			p("date", "string", "purchase date dd/MM/yyyy (defaults to today)", false, ""),
		}, true
	case "QBO.EXPENSES.BILL_PAYMENT_DELETE":
		return []paramDoc{
			p("id", "string", "target bill-payment id to delete, unapplying it from the bill (required)", true, ""),
		}, true
	case "QBO.SALES.CREDIT_CARD_CREDIT_CREATE":
		// v4 transaction-form contract (v3 creditcardcredit unsupported on
		// this build): one header account — the card — and per-line category
		// accounts. --account is a cash-side alias like cheque/expense;
		// --category-account defaults line rows that omit `account`.
		return []paramDoc{
			p("credit-card-account", "string", "credit card account the credit is recorded against (required)", true, ""),
			p("date", "string", "date of the credit as dd/MM/yyyy (required)", true, ""),
			p("account", "string", "cash-side alias for the card account — must resolve to the same account when both are given", false, ""),
			p("category-account", "string", "default expense/category account for --line-items rows that omit account", false, ""),
			p("line-items", "string", "JSON array of {account, amount, description?} rows (required)", true, ""),
		}, true
	case "QBO.SALES.INVOICE_PROGRESS":
		// v4 progress-invoice contract: a partial SALE_INVOICE consuming an
		// estimate via links.sources. --estimate carries the source quote;
		// --id is a record-id alias for it; --percent picks the billed share.
		return []paramDoc{
			p("estimate", "string", "source quote id or doc number — progress invoicing must be enabled in Sales settings (required)", true, ""),
			p("percent", "string", "percent of the quote to bill on this invoice (default 100)", false, ""),
			p("id", "string", "record-id alias for --estimate", false, ""),
		}, true
	case "QBO.EXPENSES.CHEQUE_BOUNCE":
		// v3 JournalEntry reversal (AU doc procedure — no form bounce action
		// exists): --id is the v3 Payment id whose cheque bounced. The
		// declared set {id,date,fee} matches firstParams, so only the help
		// text is authored here — and the gen help takes precedence anyway;
		// keep this case so the wired contract is explicit.
		return []paramDoc{
			p("id", "string", "v3 Payment id for the bounced cheque (required)", true, ""),
			p("date", "string", "reversal date dd/MM/yyyy (default today)", false, ""),
			p("fee", "string", "bank bounce fee amount — debits the 'Bank charges and fees' account (optional)", false, ""),
		}, true
	case "QBO.ACCOUNTING.JOURNAL_CREATE":
		// Bulk-create path: one --items-json array, not per-entry flags.
		return []paramDoc{
			p("items-json", "string", "JSON array of {date, amount, from-account, to-account, memo, description, class}; description sets both line descriptions (required)", true, ""),
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
			p("limit", "int", "max events to return", false, "0"),
		}, true
	case "QBO.FEED.TXN_LOOKUP":
		return []paramDoc{
			p("query", "string", "case-insensitive substring match on id, olbTxnId, description or cheque number (hyphens fold to spaces)", true, ""),
			p("account-id", "string", "banking account id", false, "204"),
			p("limit", "int", "max matching rows returned after all pages are searched", false, "0"),
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
			p("memo", "string", "memo to persist on the posted transaction; omitted preserves existing text", false, ""),
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
			p("account-id", "string", "explicit CSV-import account from feed account list; TC2 test account 44 works, 209 is inactive there; refuses 204 and 93", true, ""),
			p("file", "string", "CSV bank-statement file staged via uploadCsvFile then processed row by row (optional)", false, ""),
			p("description", "string", "bank description for single-row import without --file (label test rows)", true, ""),
			p("amount", "string", "numeric amount for single-row import; income positive, expense negative", true, ""),
			p("date", "string", "dd/MM/yyyy AU locale (default today Australia/Sydney)", false, ""),
		}, true
	case "QBO.ACCOUNTING.REGISTER_READ":
		return []paramDoc{
			p("account-id", "string", "GL/bank account id. Register is booked-state ground truth (posted, not For review)", false, "204"),
			p("limit", "int", "max register rows", false, "0"),
			p("offset", "int", "zero-based register row offset; a page is not a complete census", false, "0"),
		}, true
	case "QBO.REPORTS.AGED_PAYABLES_READ", "QBO.REPORTS.AGED_RECEIVABLES_READ", "QBO.REPORTS.GENERAL_LEDGER_READ", "QBO.REPORTS.TRIAL_BALANCE_READ", "QBO.REPORTS.PROFIT_LOSS_READ", "QBO.REPORTS.BALANCE_SHEET_READ", "QBO.REPORTS.VENDOR_BALANCE_READ", "QBO.REPORTS.CUSTOMER_BALANCE_READ", "QBO.REPORTS.CUSTOMER_SALES_READ", "QBO.REPORTS.VENDOR_EXPENSES_READ":
		return []paramDoc{
			p("report", "string", "v3 report name: AgedPayables, AgedReceivables, GeneralLedger, or TrialBalance (required)", true, ""),
			p("date-range", "string", "report period start,end as YYYY-MM-DD,YYYY-MM-DD (required)", true, ""),
			p("id", "string", "target report id (accepted, not forwarded to v3 reports API)", false, ""),
			p("limit", "int", "max rows (accepted for catalog parity; reports are not list endpoints)", false, "0"),
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
		out = append(out, p("limit", "int", "max rows", false, "0"))
	case strings.HasSuffix(suffix, "_READ"):
		out = append(out, p("limit", "int", "max rows when listing", false, "0"))
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
	if id == "QBO.FEED.TXN_LOOKUP" {
		return "Searches feed IDs, bank descriptions and cheque-number labels across Pending, Posted and Excluded. This is text matching, not a structured amount/date filter."
	}
	if id == "QBO.EXPENSES.RECEIPT_DELETE" || id == "QBO.EXPENSES.RECEIPT_READ" || id == "QBO.EXPENSES.RECEIPT_SEARCH" {
		return "OCR receipts use financialdocument/stagetransactions, not v3 Attachable IDs. This alias is blocked until a complete staged-receipt read/delete contract is verified. Use the receipts UI; company attachable commands operate on a different resource."
	}
	if id == "QBO.FEED.TXN_UNPOST" {
		return "Posted Undo returns the feed row to Pending and can delete the expense created when it was posted. It is not the expense editor's preserving Unmatch action. When the accounting record must remain, use the expense editor's online banking match control and verify the retained record. Undoing a reconciled row can unbalance the reconciliation."
	}
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
	if e.Mode == modeWired && e.Risk != "R0" {
		e.Verification = "independent_readback_required"
		e.Notes += " Live submission is blocked unless this command has a reviewed readback adapter; dry-run remains available. Receipt-only success is not verified completion."
	}
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
