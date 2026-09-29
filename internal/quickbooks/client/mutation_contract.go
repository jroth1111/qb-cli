package client

import (
	"fmt"
	"sort"
	"strings"
)

// mutation_contract.go is the single source of truth for which CLI flags a
// mutation operation actually consumes. Three invariants are enforced by
// tests in both packages:
//
//   - every catalog-declared parameter is either in the contract or
//     explicitly meta-listed (cli package: declared ⊆ consumed ∪ meta)
//   - every consumed flag is reachable on the real cobra command, directly or
//     through an attached alias (cli package: consumed ⊆ attached)
//   - every flag literal read inside the mutation builders appears in some
//     contract entry or the control set (client package: drift guard)
//
// "Consumed" means the flag's value provably reaches the request/operation in
// a supported way for that entity and op — not merely that the name is read
// somewhere in a shared helper. Flags the builders read but semantically drop
// for an entity (e.g. --due-date on a Deposit) are excluded so the cli layer
// can reject them loudly instead of silently dropping them.
//
// Ops are the canonical ReplayMutate verbs: create, update, delete,
// deactivate, void, copy, discount, writeoff, send. Cheque is normalized to
// Purchase by ReplayMutate before dispatch.

var (
	// txnDateFlags are the aliases transactionDateFlag accepts for TxnDate.
	txnDateFlags = []string{
		"date", "txn-date", "invoice-date", "bill-date", "purchase-date",
		"quote-date", "payment-date", "refund-date", "charge-date", "credit-date",
	}
	// memoFlags feed PrivateNote on transaction bodies.
	memoFlags = []string{"memo", "notes"}
	// salesLineFlags are the JSON line arrays salesLines reads. Note that
	// --items is deliberately absent: salesLines only reads line-items/lines.
	salesLineFlags = []string{"line-items", "lines"}
	// expenseLineFlags are the JSON arrays lineItemsFlag reads (expense side
	// plus the purchase-order --items alias).
	expenseLineFlags = []string{"line-items", "lines", "items"}
	// contactFlags feed BillAddr/PrimaryPhone/CurrencyRef via
	// applyContactFields.
	contactFlags = []string{"address", "line1", "city", "state", "postcode", "postal", "phone", "currency"}
	// salesAddrFlags feed BillAddr on sales documents (no phone/currency).
	salesAddrFlags = []string{"address", "line1", "city", "state", "postcode", "postal"}
	// timeActivityFlags are read by applyTimeActivityFields on both create
	// and update.
	timeActivityFlags = []string{
		"hours", "minutes", "rate", "employee", "vendor", "supplier", "name-of",
		"customer", "customer-id", "item", "item-id", "description", "billable",
	}
	// itemFieldFlags are read by applyItemFields.
	itemFieldFlags = []string{
		"sku", "description", "sales-price", "purchase-cost", "initial-cost",
		"reorder-point", "preferred-supplier",
	}
	// expenseSynthFlags land on a synthesized expense line.
	expenseSynthFlags = []string{"billable", "project"}

	// Base sets shared by entity entries and compound ops (discount/writeoff
	// replay the update/create builders on a different entity).
	invoiceUpdateSet = concat(
		[]string{"id", "amount", "email", "due-date",
			"discount", "discount-percent", "percent"},
		expenseLineFlags, txnDateFlags, memoFlags)
	creditMemoCreateSet = concat(
		[]string{"customer", "amount", "item", "email", "to",
			"discount", "discount-percent", "percent", "invoice-id", "id"},
		salesAddrFlags, salesLineFlags, txnDateFlags, memoFlags)
)

// mutationConsumed maps "Entity:op" to the flag names that op consumes.
// Entity is the v3 entity name; Cheque commands normalize to Purchase.
var mutationConsumed = map[string][]string{
	// --- create ---
	"Account:create":  {"name", "title", "type", "subtype", "sub-type"},
	"Customer:create": cat(concat([]string{"name", "title", "email"}, contactFlags)),
	"Vendor:create":   cat(concat([]string{"name", "title", "email", "tpar", "supplier-id"}, contactFlags)),
	"Employee:create": {"name", "title", "email", "given-name", "family-name"},
	"Budget:create":   cat(concat([]string{"name", "title", "start-date", "end-date", "fiscal-year", "type", "entry-type", "interval", "account", "amount"}, txnDateFlags)),
	"RecurringTransaction:create": cat(concat(
		[]string{"name", "title", "customer", "amount", "item",
			"template-type", "recur-type", "recurring-type",
			"interval", "interval-type", "days-in-advance", "days-before",
			"start-date", "end-date", "next-date",
			"discount", "discount-percent", "percent"},
		salesLineFlags)),
	"Project:create": {"name", "title", "customer"},
	"CreditCardCredit:create": cat(concat(
		[]string{"credit-card-account", "cc-account", "supplier", "payee",
			"category-account", "account", "amount"},
		expenseLineFlags, expenseSynthFlags, txnDateFlags, memoFlags)),
	"Class:create":         {"name", "title"},
	"Department:create":    {"name", "title"},
	"TaxAgency:create":     {"name", "title"},
	"TaxRate:create":       {"name", "title", "rate", "agency", "agency-id"},
	"Term:create":          {"name", "title", "type", "due-days", "due_days", "duedays"},
	"PaymentMethod:create": {"name", "title"},
	"Item:create": cat(concat(
		[]string{"name", "title", "type", "income-account",
			"expense-account", "cogs-account", "asset-account", "inventory-asset-account",
			"qty", "quantity", "new-qty", "initial-qty"},
		itemFieldFlags, txnDateFlags)),
	"Invoice:create": cat(concat(
		[]string{"customer", "amount", "item", "email", "to",
			"discount", "discount-percent", "percent",
			"due-date", "time-activity", "time-activity-id"},
		salesAddrFlags, salesLineFlags, txnDateFlags, memoFlags)),
	"Estimate:create": cat(concat(
		[]string{"customer", "amount", "item", "email", "to",
			"discount", "discount-percent", "percent"},
		salesAddrFlags, salesLineFlags, txnDateFlags, memoFlags)),
	"CreditMemo:create": creditMemoCreateSet,
	"SalesReceipt:create": cat(concat(
		[]string{"customer", "amount", "item", "email", "to",
			"discount", "discount-percent", "percent",
			"deposit-to", "refund-from", "account"},
		salesAddrFlags, salesLineFlags, txnDateFlags, memoFlags)),
	"RefundReceipt:create": cat(concat(
		[]string{"customer", "amount", "item", "email", "to",
			"discount", "discount-percent", "percent",
			"deposit-to", "refund-from", "account"},
		salesAddrFlags, salesLineFlags, txnDateFlags, memoFlags)),
	"DelayedCharge:create": cat(concat(
		[]string{"customer", "amount", "item", "email", "to",
			"discount", "discount-percent", "percent"},
		salesAddrFlags, salesLineFlags, txnDateFlags, memoFlags)),
	"DelayedCredit:create": cat(concat(
		[]string{"customer", "amount", "item", "email", "to",
			"discount", "discount-percent", "percent"},
		salesAddrFlags, salesLineFlags, txnDateFlags, memoFlags)),
	"Payment:create": cat(concat(
		[]string{"customer", "amount", "invoice-id",
			"deposit-to", "refund-from", "account"},
		txnDateFlags, memoFlags)),
	"Bill:create": cat(concat(
		[]string{"supplier", "payee", "amount", "category-account", "account", "due-date"},
		expenseLineFlags, expenseSynthFlags, txnDateFlags, memoFlags)),
	"VendorCredit:create": cat(concat(
		[]string{"supplier", "payee", "amount", "category-account", "account"},
		expenseLineFlags, expenseSynthFlags, txnDateFlags, memoFlags)),
	"PurchaseOrder:create": cat(concat(
		[]string{"supplier", "payee", "amount", "category-account", "account"},
		expenseLineFlags, expenseSynthFlags, txnDateFlags, memoFlags)),
	"ItemReceipt:create": cat(concat(
		[]string{"supplier", "payee", "amount", "category-account", "account"},
		expenseLineFlags, expenseSynthFlags, txnDateFlags, memoFlags)),
	"BillPayment:create": cat(concat(
		[]string{"supplier", "payee", "bill-id", "bill-ids", "id",
			"payment-account", "bank-account", "account",
			"pay-type", "payment-type", "type", "amount",
			"payment-method", "supplier-credit-ids", "credit-ids"},
		txnDateFlags, memoFlags)),
	"Purchase:create": cat(concat(
		[]string{"payment-account", "bank-account", "account",
			"deposit-to", "refund-from", "asset-account", "category-account",
			"mode", "type", "payment-type",
			"supplier", "payee", "amount"},
		expenseLineFlags, expenseSynthFlags, txnDateFlags, memoFlags)),
	"JournalEntry:create": cat(concat(
		[]string{"lines", "line-items", "items-json", "amount", "from-account", "to-account"},
		txnDateFlags, memoFlags)),
	"Transfer:create": cat(concat(
		[]string{"from-account", "account", "to-account", "amount"},
		txnDateFlags, memoFlags)),
	"Deposit:create": cat(concat(
		[]string{"deposit-to", "refund-from", "account", "amount",
			"category-account", "from-account", "to-account", "payments"},
		salesLineFlags, txnDateFlags, memoFlags)),
	"TimeActivity:create": cat(concat(timeActivityFlags, txnDateFlags)),
	"InventoryAdjustment:create": cat(concat(
		[]string{"item", "account", "adjust-account",
			"qty", "quantity", "new-qty", "amount",
			"doc-number", "number"},
		txnDateFlags, memoFlags)),
	"CompanyCurrency:create": {"code", "name"},
	"TaxCode:create":         {"name", "title", "period"},

	// --- update ---
	"Attachable:update": {"id", "note", "memo", "name", "title"},
	"Transfer:update": cat(concat(
		[]string{"id", "from-account", "to-account", "amount"}, txnDateFlags, memoFlags)),
	"Deposit:update": cat(concat(
		[]string{"id", "account", "deposit-to", "to-account",
			"from-account", "category-account", "amount"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"JournalEntry:update": cat(concat(
		[]string{"id", "from-account", "to-account", "amount", "items-json"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"Item:update": cat(concat(
		[]string{"id", "name", "title", "active",
			"income-account", "expense-account", "cogs-account",
			"asset-account", "inventory-asset-account", "starting-value"},
		itemFieldFlags)),
	"Bill:update": cat(concat(
		[]string{"id", "supplier", "payee", "vendor", "amount", "due-date"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"VendorCredit:update": cat(concat(
		[]string{"id", "supplier", "payee", "vendor", "amount"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"PurchaseOrder:update": cat(concat(
		[]string{"id", "supplier", "payee", "vendor", "amount"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"ItemReceipt:update": cat(concat(
		[]string{"id", "supplier", "payee", "vendor", "amount"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"CreditCardCredit:update": cat(concat(
		[]string{"id", "amount"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"Project:update":     {"id", "customer", "name", "title"},
	"BillPayment:update": cat(concat([]string{"id", "amount"}, txnDateFlags, memoFlags)),
	"Payment:update":     cat(concat([]string{"id", "amount"}, txnDateFlags, memoFlags)),
	"Purchase:update": cat(concat(
		[]string{"id", "payee", "payee-id", "amount"},
		expenseLineFlags, expenseSynthFlags, txnDateFlags, memoFlags)),
	"TimeActivity:update":         cat(concat([]string{"id"}, timeActivityFlags, txnDateFlags)),
	"Class:update":                {"id", "name", "title", "active"},
	"Department:update":           {"id", "name", "title", "active"},
	"Budget:update":               {"id", "name", "title"},
	"RecurringTransaction:update": {"id", "name", "title", "amount"},
	"Invoice:update":              invoiceUpdateSet,
	"Estimate:update": cat(concat(
		[]string{"id", "amount", "email"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"CreditMemo:update": cat(concat(
		[]string{"id", "amount", "email"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"SalesReceipt:update": cat(concat(
		[]string{"id", "amount", "email"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"RefundReceipt:update": cat(concat(
		[]string{"id", "amount", "email"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"DelayedCharge:update": cat(concat(
		[]string{"id", "amount", "email"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"DelayedCredit:update": cat(concat(
		[]string{"id", "amount", "email"},
		expenseLineFlags, txnDateFlags, memoFlags)),
	"Term:update":            {"id", "name", "title", "active", "type", "due-days", "due_days", "duedays"},
	"PaymentMethod:update":   {"id", "name", "title", "active"},
	"TaxAgency:update":       {"id", "name", "title", "active"},
	"TaxCode:update":         {"id", "name", "title", "active"},
	"CompanyCurrency:update": {"id", "active"},
	"CompanyInfo:update":     {"id", "name", "title"},
	"Customer:update": cat(concat(
		[]string{"id", "name", "title", "email", "active"}, contactFlags)),
	"Vendor:update": cat(concat(
		[]string{"id", "name", "title", "email", "active", "tpar", "supplier-id"}, contactFlags)),
	"Employee:update": {"id", "name", "title", "email", "active"},
	"Account:update":  {"id", "name", "title", "active"},

	// --- non-body ops ---
	"*:delete":            {"id"},
	"Purchase:delete":     {"id", "mode"}, // --mode is a validated guard: v3 cannot void a Purchase
	"*:deactivate":        {"id"},
	"Employee:deactivate": concat([]string{"id", "employee"}, txnDateFlags), // date -> TerminationDate
	"*:void":              {"id", "txn-id", "entity", "txn-type"},
	"*:copy":              {"id", "customer"},
	"*:send":              {"id", "to", "email", "send-to"},
	"Invoice:discount": cat(concat(
		[]string{"id", "discount", "discount-percent", "percent"},
		invoiceUpdateSet)),
	"Invoice:writeoff": concat(creditMemoCreateSet, []string{"id"}),
}

// mutationEntityAlias maps CLI-level entity words onto the v3 entity the
// mutation path replays (Cheque is forced to a Check-typed Purchase).
var mutationEntityAlias = map[string]string{
	"Cheque": "Purchase",
}

// MutationFlagsConsumed reports the contract flag set for an entity/op after
// entity normalization. The second return value reports whether the pair has
// a contract entry at all.
func MutationFlagsConsumed(entity, op string) ([]string, bool) {
	if alias, ok := mutationEntityAlias[entity]; ok {
		entity = alias
	}
	if op == "edit" {
		op = "update"
	}
	v, ok := mutationConsumed[entity+":"+op]
	if !ok {
		v, ok = mutationConsumed["*:"+op]
	}
	return v, ok
}

// MutationContractEntities lists every entity with at least one contract
// entry — for the drift tests.
func MutationContractEntities() []string {
	seen := map[string]bool{}
	for k := range mutationConsumed {
		parts := strings.SplitN(k, ":", 2)
		if parts[0] != "*" {
			seen[parts[0]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// MutationContractOps lists the canonical ops that carry contract entries.
func MutationContractOps() []string {
	seen := map[string]bool{}
	for k := range mutationConsumed {
		seen[strings.SplitN(k, ":", 2)[1]] = true
	}
	out := make([]string, 0, len(seen))
	for op := range seen {
		out = append(out, op)
	}
	sort.Strings(out)
	return out
}

// mutationControlFlags are structural flags that are not body fields: target
// selection, the cheque marker injected by ReplayMutate, and cli-layer
// selectors read in newV3MutateCmd before dispatch.
var mutationControlFlags = map[string]bool{
	"id": true, "txn-id": true, "entity": true, "txn-type": true,
	"cheque": true, "ids": true, "items-json": true,
}

// MutationConsumedUnion returns every flag name consumed by any contract
// entry plus the control set — used by the drift guard to prove no builder
// read escapes the contract.
func MutationConsumedUnion() map[string]bool {
	out := map[string]bool{}
	for _, names := range mutationConsumed {
		for _, n := range names {
			out[n] = true
		}
	}
	for n := range mutationControlFlags {
		out[n] = true
	}
	return out
}

// MutationIsControl reports whether a flag name is a structural/control flag
// rather than a body field.
func MutationIsControl(name string) bool {
	return mutationControlFlags[name]
}

// ValidateMutationFlags rejects flags that were set on a command but are not
// consumed by the resolved entity:op — the runtime half of the contract that
// turns silent drops into loud errors. Two rejection classes:
//
//   - denied: catalog-declared params documented as not implemented (meta);
//     the message carries the documented reason.
//   - not consumed: flags the entity:op path never reads — mistyped or
//     unsupported for this operation.
//
// allowedExtra lets the cli layer permit declared params it knows are consumed
// by command-local plumbing outside ReplayMutate.
func ValidateMutationFlags(entity, op string, changed func(string) bool, names []string, allowedExtra map[string]bool, denied map[string]string) error {
	consumed, _ := MutationFlagsConsumed(entity, op)
	allowed := map[string]bool{}
	for _, n := range consumed {
		allowed[n] = true
	}
	for n := range mutationControlFlags {
		allowed[n] = true
	}
	for n := range allowedExtra {
		allowed[n] = true
	}
	var bad, dead []string
	for _, n := range names {
		if !changed(n) {
			continue
		}
		if reason, ok := denied[n]; ok {
			dead = append(dead, n+" ("+reason+")")
			continue
		}
		if !allowed[n] {
			bad = append(bad, n)
		}
	}
	sort.Strings(bad)
	sort.Strings(dead)
	msgs := append(bad, dead...)
	if len(msgs) > 0 {
		return fmt.Errorf("%s %s: unsupported flag(s) --%s", entity, op, strings.Join(msgs, " --"))
	}
	return nil
}

func concat(sets ...[]string) []string {
	var out []string
	for _, s := range sets {
		out = append(out, s...)
	}
	return out
}

// cat is concat for readability in table rows.
func cat(s []string) []string { return s }
