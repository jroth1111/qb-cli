# Mega Style Apartments categorisation patterns

Initial live read-only analysis run 2026-07-13 over posted QuickBooks objects dated from 2025-01-01 onward. For authoritative full-window counts and refined rules, use [`two-year-categorisation-audit.md`](two-year-categorisation-audit.md), covering 2024-07-13 through 2026-07-13.

## Evidence scope

- 2,878 purchases, 1,467 deposits, 26 invoices, and 7 sales receipts
- 5,079 purchase lines and 2,720 deposit lines
- 163 active accounts and 160 active classes
- Pending bank-feed rows are separate UI-only evidence; suggestions are not treated as posted accounting decisions

## Core model

The dominant bookkeeping model is two-dimensional:

1. **Account = economic purpose**: revenue, security deposit liability, cleaning, rent, utilities, parking, payroll, IT, and so on.
2. **Class = attribution target**: usually the property/unit that earned the revenue or incurred the cost. Staff classes such as `Maggie` and `Jacob` are used for director compensation; `Parent Entity` is used for central costs such as IT.

Do not infer a class from the account alone. Infer it from the transaction description, supplier/customer context, property code/address, and repeated posted examples.

## Revenue patterns

- Ordinary accommodation/platform receipts overwhelmingly post to `MSA Property Revenue`, classed to the specific property/unit.
- Client-owned property receipts use `Client Property Revenue`, again classed to the property/unit.
- Refunds or reversals may appear as negative purchase lines against `MSA Property Revenue`; direction and transaction type matter.
- Security-deposit receipts and returns use `Guest Security Deposits`, a liability account, classed to the guest's property/unit. Do not call these revenue.
- Management fees use `Management Income`; maintenance recoveries use `Maintenance Income`; product sales use `Sales of Product Income`.
- Deposits can contain expense, liability, asset, or compensation lines as well as income. Classify each deposit line by substance rather than treating every deposit as revenue.

Most frequent deposit-line accounts since 2025:

| Account | Lines |
|---|---:|
| MSA Property Revenue | 1,837 |
| Guest Security Deposits | 561 |
| Client Property Revenue | 230 |
| Management Income | 26 |
| Sales of Product Income | 14 |

## Expense patterns

- Routine property operations use the `MSA Property Expenses:*` family: cleaning, internet, utilities, repairs and maintenance, parking, meet and greet, key and fobs, management fees, and capital expenses.
- Client-property costs use the deeper `Client Property Expenses:*` hierarchy, separating fixed operating expenses, initial costs, owner distributions, and short-term operating expenses.
- Rent paid by MSA commonly uses cost-of-sales account `Guaranteed Rental - MSA`, classed to the rented unit.
- Cleaning jobs commonly use `MSA Property Expenses:Cleaning`; some cleaning/amenity jobs use the separate `Cleaning Amenities Expense`. Follow repeated supplier/job evidence, not description keywords alone.
- Staff/director payments use `Director Compensation` with a person class (`Maggie` or `Jacob`). Contractor payments use `Contractor Payments`; staff classes may be appropriate when the posted pattern supports them.
- Central operating costs use general accounts such as `IT`, `Bank charges`, `Accountancy`, travel, motor vehicle, office/admin, healthcare, or client gifts. These often use a person or `Parent Entity` class instead of a property class.
- `Guest Security Deposits` is also frequent on purchase lines for refunds/returns; it remains a liability movement, not an expense.

Most frequent purchase-line accounts since 2025:

| Account | Lines |
|---|---:|
| MSA Property Expenses:Cleaning | 1,391 |
| Guest Security Deposits | 585 |
| Guaranteed Rental - MSA | 470 |
| MSA Property Expenses:Internet | 286 |
| Meet and Greet Expense | 199 |
| Contractor Payments | 194 |
| Director Compensation | 182 |
| Cleaning Amenities Expense | 177 |
| IT | 164 |
| MSA Property Expenses:Utilities | 159 |
| Bank charges | 141 |
| Client Property Expenses:Owner Distribution | 136 |
| MSA Property Expenses:Repairs and maintenance | 133 |
| MSA Property Expenses:Parking | 111 |

## Class-selection rules

Use this priority order:

1. Explicit unit/property identifier in the bank description, memo, job record, invoice, payout detail, or existing matched transaction.
2. Exact recurring supplier/customer plus unit pattern from posted transactions.
3. Address-to-class mapping when the class name is a unit/property label and the address evidence is unambiguous.
4. Person class for director/staff-specific compensation or costs when the posted pattern supports it.
5. `Parent Entity` for central/company-wide costs when there is no property or person attribution and repeated examples support it.
6. Leave unresolved rather than guessing when evidence conflicts or multiple properties are plausible.

High-frequency pair examples:

- `MSA Property Revenue` + property class
- `MSA Property Expenses:Cleaning` + property class
- `Guest Security Deposits` + property class
- `Guaranteed Rental - MSA` + property class
- `Director Compensation` + `Maggie` or `Jacob`
- `IT` + `Parent Entity`
- `Motor vehicle expenses` + `Maggie`

Recent confirmed examples:

- Rent description naming `1705` -> `Guaranteed Rental - MSA` + `1705 Bella`
- Origin Energy references mapped to specific units -> `MSA Property Expenses:Utilities` + corresponding property class
- `2403 parking` -> `MSA Property Expenses:Parking` + `2403 Mainpoint`
- `Maggie Salary` -> `Director Compensation` + `Maggie`
- `Jacob Salary` -> `Director Compensation` + `Jacob`

## Completeness and exceptions

Class coverage is strong but not perfect:

| Object | Lines | Missing class | Coverage |
|---|---:|---:|---:|
| Purchases | 5,079 | 316 | 93.8% |
| Deposits | 2,720 | 27 | 99.0% |
| Invoices | 61 | 27 | 55.7% |
| Sales receipts | 17 | 8 | 52.9% |

Do not treat missing class on invoices or sales receipts as proof that classes are optional for property attribution; those objects may use item-level or surrounding context differently. Inspect the full object and related payment/deposit before deciding.

## Guardrails for future categorisation

- Suggestions in Pending are proposals, not evidence of posted policy.
- Never classify security deposits as revenue or expense merely from cash direction.
- Avoid generic or legacy duplicates when the active posted pattern uses an `MSA Property Expenses:*` or `Client Property Expenses:*` account.
- Preserve split transactions when one bank row covers multiple properties, accounts, or classes.
- A class answers **who/which property owns the result**; the account answers **what the transaction economically is**.
- Before writing, compare at least two recent, genuinely similar posted examples when automation or a rule is proposed.
