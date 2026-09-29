# Expense classification policy

This is the normative decision policy for future Mega Style Apartments expense classification. It intentionally does not treat historical accountant postings as binding precedent. Historical transactions are supporting evidence only; invoices, receipts, contracts, service addresses, linked jobs and economic substance have higher authority.

This policy is classification guidance, not permission to post. All QuickBooks mutations still require explicit user authority, a pre-state record, rollback path and live read-back verification.

## Required decision sequence

For every transaction or split line, decide in this order:

1. **Is it an expense at all?** Exclude assets, liabilities, transfers, loans, owner distributions, deposits, refunds and revenue reversals before selecting an expense account.
2. **Who bears the economics?** Identify an MSA-operated property, landlord-managed property, central MSA operation, or named person.
3. **What was purchased?** Use the most specific defensible economic category.
4. **Is it recoverable?** Mark it as absorbed, direct landlord cost, pass-through at cost, or recharge with margin.
5. **Should it be capitalised?** Distinguish repair/consumption from an enduring asset or improvement.
6. **Which class owns it?** Determine class independently from the account.
7. **Does it require splits?** Preserve separate purposes, properties, recovery treatments and tax treatments.
8. **What is the evidence and confidence?** Record the source and leave unresolved when it does not uniquely support the result.

Mnemonic: **account says what; class says whose; recovery says who ultimately pays.**

## Non-expenses

Do not force these into operating expenses:

| Substance | Treatment |
|---|---|
| Bank-to-bank movement | Transfer or balance-sheet movement |
| Guest bond received or returned | `Guest Security Deposits` liability |
| Owner remittance | Owner distribution/payable path, excluded from operating profitability |
| Loan advance or repayment | Relevant loan asset or liability |
| Supplier refund | Reduction or reversal of original expense |
| Guest revenue refund | Reversal of original revenue |
| Recoverable amount not yet billed | Receivable when the accounting facts support it |
| Enduring furniture/equipment/improvement | Assess fixed-asset treatment |
| Personal/director spending | Loan, compensation or reimbursement according to evidence |

QuickBooks currently types `Client Property Expenses:Owner Distribution` as an expense. That chart configuration does not make an owner distribution an economic operating expense.

## Operating-model routing

### MSA rent-to-rent property

MSA earns the accommodation revenue and bears the operating risk:

| Substance | Preferred account |
|---|---|
| Cleaning | `MSA Property Expenses:Cleaning` |
| Internet | `MSA Property Expenses:Internet` |
| Electricity, gas or water | `MSA Property Expenses:Utilities` |
| Repairs | `MSA Property Expenses:Repairs and maintenance` |
| Property parking | `MSA Property Expenses:Parking` |
| Laundry | `MSA Property Expenses:Laundry` |
| Check-in service | `MSA Property Expenses:Meet & Greet` |
| Keys, fobs or remotes | `MSA Property Expenses:Key and Fobs` |
| Photography | `MSA Property Expenses:Photography` |
| Storage | `MSA Property Expenses:Storage` |
| Contractual rent | `Guaranteed Rental - MSA` |

Use the exact property class.

### Landlord-management property

The landlord retains the accommodation economics. Route costs through the most specific `Client Property Expenses:*` child account:

| Substance | Preferred client-property path |
|---|---|
| Cleaning | `Short-Term Business Operating Expenses:Commercial Cleaning Services` |
| Laundry | `Short-Term Business Operating Expenses:Commercial Laundry Services` |
| Internet | `Short-Term Business Operating Expenses:Internet` |
| Repairs | `Short-Term Business Operating Expenses:Repairs and maintenance` |
| Parking | `Short-Term Business Operating Expenses:Parking` |
| Meet and greet | `Short-Term Business Operating Expenses:Meet & Greet` |
| Amenities | `Short-Term Business Operating Expenses:Amenities` |
| Advertising/channel fees | `Short-Term Business Operating Expenses:Advertising & Channel Fees` |
| Storage | `Short-Term Business Operating Expenses:Storage` |
| Management fee | `Short-Term Business Operating Expenses:Management Fee` |
| Body corporate, council rates, insurance or utilities | Corresponding `Fixed Operating Expenses:*` child |
| Initial linen or repairs | Corresponding `Initial Costs:*` child |

Use the exact landlord-property class. Units 10 and 14 follow the more specific rules in [`property-operating-models.md`](property-operating-models.md) and [`landlord-statement-journal-patterns.md`](landlord-statement-journal-patterns.md).

### Central and person costs

- Use specific central accounts such as `IT`, `Accountancy`, legal/professional fees, marketing, office/admin, printing, insurance, research or training.
- Use `Parent Entity` only when the cost is genuinely central; missing property evidence does not make it central.
- Jacob or Maggie salary uses `Director Compensation` with the matching person class.
- Contractor payments use `Contractor Payments` only when the invoice is genuinely labour/contractor compensation rather than a specific property service.
- Personal expenditure paid by MSA may be a director-loan movement rather than an expense.

## Recoverability and margin

Keep these treatments distinct:

1. **MSA-absorbed cost:** expense to the relevant MSA or central account; no recovery income.
2. **Direct landlord cost:** client-property expense; no MSA income is implied merely because it appears on an owner statement.
3. **Pass-through at cost:** preserve the supplier cost and corresponding recovery; margin is zero.
4. **Recharge with margin:** preserve supplier cost and landlord charge separately. Calculate both dollar margin and the stated percentage basis.

Definitions:

- `dollar margin = landlord charge - linked supplier cost`
- `markup on cost = dollar margin / supplier cost`
- `gross margin percentage = dollar margin / landlord charge`

Never call the full recharge amount “margin.” Never invent a percentage when the supplier cost or approved pricing rule is missing. Follow [`landlord-statement-journal-patterns.md`](landlord-statement-journal-patterns.md) for the observed unit 10/14 journal treatment.

## Specific category tests

### Cleaning and laundry

- Select MSA, client-property or cleaning-amenity workflow from operating model and service purpose, not cleaner identity.
- Keep laundry separate from cleaning when source detail permits.
- Record number/type of cleans where available.
- For unit 14, historical $210 standard and $440 long-stay rates are reasonableness evidence; the explicit prior-month approved-amount rule remains controlling until clarified.

### Repair versus capital

Use repairs and maintenance for work that restores existing condition. Assess capital or fixed-asset treatment when expenditure creates an asset, materially improves capability, or extends useful life. Furniture, whitegoods and durable equipment require this test even if the current chart contains expense-typed “capital” accounts. Tax and depreciation treatment requires an approved accounting policy.

### Parking

- Guest/property parking space or rent -> property parking account and property class.
- Director travel parking -> motor-vehicle/general-parking account and person/central class.
- Landlord-recharged permit -> client expense plus separately evidenced recovery.
- Enduring transferable parking right -> assess asset treatment.

The word `parking` alone is insufficient.

### Utilities and internet

- Operating model selects MSA versus client family.
- Service address or utility account reference selects class.
- Split bills covering multiple properties.
- Preserve original category for supplier credits/refunds with reversed direction.

Supplier alone is insufficient because providers such as TPG serve both operating models.

### Rent

- MSA property rent -> `Guaranteed Rental - MSA`.
- Use `Guaranteed Rental - Client` only when a client contract supports that substance.
- Storage, parking, equipment hire and body corporate are not rent merely because they recur.
- Rental bonds are balance-sheet deposits, not rent expense.

### OTA, merchant and bank fees

- Separate gross accommodation revenue, OTA commission, merchant fee, refunds and cash settlement when the source payout permits.
- Ordinary bank charge -> `Bank charges`.
- Merchant-processing fee -> `Bank charges:Merchant Fees` or established OTA merchant-fee account.
- International fee -> bank charge linked to the parent transaction when proven.
- Inherit a property class from a parent transaction only when that relationship is evidenced; otherwise use central treatment only when genuinely central.

### Guest operations

Meet and greet, keys/fobs, amenities, cleaning, laundry, guest reimbursement and property repair are distinct economic categories. Split bundled invoices when line detail and amounts exist.

## Mandatory split rules

Split a transaction when it covers any of:

- multiple properties;
- multiple economic purposes;
- expense plus capital purchase;
- supplier cost plus bank/international fee;
- business plus personal use;
- MSA plus landlord-property costs;
- recoverable plus non-recoverable components; or
- different tax treatments.

Preserve amount, account, class, description, source, recovery treatment and tax treatment per line. The lines must reconcile exactly to the transaction total.

## Generic-account control

These are exception accounts, not defaults:

- `General Expenses - Various`
- `Miscellaneous Property Expense`
- client-property miscellaneous expense
- `Purchases`
- `Uncategorised Expense`
- broad parent expense accounts

Use one only when source detail is genuinely insufficient, no specific existing account fits, or an approved immateriality policy allows it. Record why, mark it for follow-up, and never cite historical accountant lumping as the sole justification. Recurring miscellaneous items indicate that a dedicated account or subaccount should be proposed.

Do not create a new QuickBooks account without explicit authority. First identify recurring substance, expected volume, reporting value, overlap with existing accounts and the migration/reclassification implications.

## Evidence hierarchy

Use evidence in this order:

1. Invoice or receipt line items.
2. Contract, owner statement or approved pricing schedule.
3. Exact service address, property/unit or utility reference.
4. Linked booking, job, work order or payout.
5. Genuinely comparable recent posted transactions.
6. Merchant description.
7. Bank-feed suggestion.

When historical treatment conflicts with primary evidence, propose the economically correct classification and document the variance. Do not silently imitate the old entry.

## Required proposal record

Every future expense proposal should state:

- transaction identity and amount;
- economic substance;
- MSA/client/central/person operating model;
- most specific account;
- exact class;
- absorbed, direct, pass-through or marked-up status;
- linked source cost and recovery where applicable;
- repair-versus-capital conclusion;
- tax treatment or unresolved tax question;
- supporting evidence and confidence;
- splits and reconciliation; and
- any departure from historical treatment and why.

Leave the proposal unresolved when account, class, recoverability, capital treatment or tax treatment cannot be supported. Historical consistency is never permission to post.
