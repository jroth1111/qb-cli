# Mega Style Apartments categorisation decision model

This model was initially derived read-only on 2026-07-13 from posted QuickBooks lines dated from 2025-01-01 onward. It was then validated and refined against the authoritative two-year window documented in [`two-year-categorisation-audit.md`](two-year-categorisation-audit.md). Use the two-year audit's counts and counterexamples when they differ from the initial sample below.

- 5,082 purchase lines
- 2,720 deposit lines
- 132 journal-entry lines
- 61 invoice lines
- 17 sales-receipt lines

It describes repeated bookkeeping logic; it is not authority to mutate records. Pending-feed suggestions are excluded from the evidence because they are not posted decisions.

QuickBooks may display a populated account in `Match/Categorise`. Treat that value as an untrusted suggestion with zero evidentiary weight. It cannot establish account, class, property ownership, operating model or confidence.

## The consistent decision order

Categorise each line in this order. Do not collapse account and class into one prediction.

1. **Determine the accounting substance**: accommodation revenue, client revenue, security deposit, rent, cleaning, utilities, parking, repairs, payroll, owner distribution, fee, transfer, refund, or other purpose.
2. **Determine direction and lifecycle**: money in/out, purchase/deposit/journal entry, original/refund/reversal. Direction alone never determines the account.
3. **Determine operating model**: MSA-operated property, client-operated property, central company cost, or named-person cost.
4. **Choose the account** for economic purpose and operating model.
5. **Choose the class independently** from property/unit, address, linked transaction, or person evidence.
6. **Preserve splits** when one bank row contains multiple purposes, properties, or classes.
7. **Check related fees/refunds** against the parent transaction and inherit its class when the relationship is proven.
8. **Leave unresolved** when the evidence does not uniquely determine both dimensions.

Mnemonic: **account answers “what is it?”; class answers “whose result is it?”**

Operating-model assignments and fee terms explicitly provided by the user are recorded in [`property-operating-models.md`](property-operating-models.md) and override statistical inference. Units `10 Toorak Melody` and `14 Agatha` are landlord-management properties, not rent-to-rent properties.

## Revenue and money-in logic

| Substance | Account | Class logic |
|---|---|---|
| MSA accommodation/platform receipt | `MSA Property Revenue` | Specific property/unit |
| Client-owned accommodation receipt | `Client Property Revenue` | Specific client property/unit |
| Guest bond/security deposit receipt | `Guest Security Deposits` | Guest's property/unit |
| Management fee earned | `Management Income` | Client property/unit |
| Cleaning/amenity recovery earned | `Cleaning Amenities Income` | Client property/unit |
| Maintenance recovery earned | `Maintenance Income` | Property/unit |
| Product sale | `Sales of Product Income` | Property/person only when supported |

Platform/customer identity is not sufficient to choose MSA versus client revenue:

- Airbnb: 1,270 lines; 85.3% `MSA Property Revenue`, 13.1% `Client Property Revenue`, remainder other.
- Booking.com: 391 lines; 94.4% MSA revenue, 5.6% client revenue.
- Expedia: 114 lines; 92.1% MSA revenue, 6.1% client revenue, 1.8% travel expense.
- `CBA card`: 1,291 lines; 81.1% security-deposit liability, with MSA and client revenue also present.
- Generic `Bank transfer`: only 57.7% security deposits and 39.0% revenue.

Therefore channel/customer is a clue, not a final rule. Property ownership and transaction purpose decide the account.

For Airbnb bank receipts, inspect Airbnb in its own ego-browser task space and reconcile the payout to its component reservations, properties, adjustments and fees. Use exact listing/property evidence to select `MSA Property Revenue` versus `Client Property Revenue` and the property class. A net bank receipt may require a split; do not post the full net amount to one revenue account merely because Airbnb is the sender.

Item mappings are stronger:

- Item `MSA Accommodation` -> `MSA Property Revenue`: 32/32 observed.
- Item `Security Deposit` -> `Guest Security Deposits`: 5/5 observed.

## Expense and money-out logic

Apply the normative controls in [`expense-classification-policy.md`](expense-classification-policy.md) first. The historical mappings below help identify likely accounts and classes, but they do not justify repeating a broad, miscellaneous, or economically incorrect posting.

### Property operations

| Evidence | Account family | Class |
|---|---|---|
| Routine MSA cleaning/turn service | `MSA Property Expenses:Cleaning` | Serviced property |
| Cleaning/amenity work using the separate amenity workflow | `Cleaning Amenities Expense` | Serviced property |
| Internet provider/service | `MSA Property Expenses:Internet` or client-property internet | Connected property |
| Electricity, gas, water | `MSA Property Expenses:Utilities` | Utility account's property |
| Repairs/trade invoice | `MSA Property Expenses:Repairs and maintenance` or client equivalent | Repaired property |
| Parking/space rental | `MSA Property Expenses:Parking` | Benefiting property |
| Key handover/check-in service | `Meet and Greet Expense` or `MSA Property Expenses:Meet & Greet` | Guest property |
| Keys, fobs, remotes | `MSA Property Expenses:Key and Fobs` | Benefiting property |
| Rent/guaranteed payment by MSA | `Guaranteed Rental - MSA` | Rented property |
| Owner payout for client property | `Client Property Expenses:Owner Distribution` | Client property |

MSA versus client account families are selected by the property's operating model, not merely the supplier. Examples:

- TPG is split between MSA internet (33/54) and client-property internet (21/54).
- Cleaning suppliers span `MSA Property Expenses:Cleaning` and `Cleaning Amenities Expense`.
- Airbnb itself spans MSA revenue, client revenue, management income, and occasional expense lines.
- The word `parking` is insufficient: a property/space rental belongs to `MSA Property Expenses:Parking`, while ordinary parking consumed by a director/vehicle may belong to `Motor vehicle expenses` with the person class.

### Central and person-specific costs

| Evidence | Account | Class |
|---|---|---|
| Jacob salary | `Director Compensation` | `Jacob` |
| Maggie salary | `Director Compensation` | `Maggie` |
| Central software/technology | `IT` or relevant IT subaccount | Usually `Parent Entity` |
| Merchant/bank fee | `Bank charges` or `Bank charges:Merchant Fees` | Parent transaction's class, otherwise `Parent Entity` when central |
| Named contractor payment | `Contractor Payments` | Named person or benefiting property, based on work evidence |
| General motor vehicle cost for Maggie | `Motor vehicle expenses` | `Maggie` in the established pattern |

## High-confidence account signals

These are repeated posted mappings, not unconditional automation rules. Class still needs independent evidence.

| Party or description signal | Observed account | Evidence |
|---|---|---:|
| `Origin` / `Origin Energy` | `MSA Property Expenses:Utilities` | 95/95 |
| `TANGERINE` | `MSA Property Expenses:Internet` | 96/96 |
| `iiNet Ltd` signature | `MSA Property Expenses:Internet` | 80/80 |
| `Tango Energy` | `MSA Property Expenses:Utilities` | 10/10 |
| `Carepark` | `MSA Property Expenses:Parking` | 9/9 |
| `Winconnect` | `MSA Property Expenses:Utilities` | 8/8 |
| `South East Water` | `MSA Property Expenses:Utilities` | 5/5 |
| Jacob salary signature | `Director Compensation` | 80/80 |
| Maggie salary signature | `Director Compensation` | 80/80 |
| `KEYNEST ...` base charge | `Meet and Greet Expense` | 116/116 across two dominant signatures |
| `INTERNATIONAL FEE KEYNEST ...` | `Bank charges` | 63/63 dominant signature |
| `... PARKING` transfer signature | `MSA Property Expenses:Parking` | 54/55 |
| `MICM ... RENT` | `Guaranteed Rental - MSA` | 77/78 |
| generic rent transfer signature with stable property wording | `Guaranteed Rental - MSA` | 37/37 |
| `Jack Tai` | MSA repairs and maintenance | 114/120; client repair account on 5 |
| `Chunming Liu` | MSA cleaning | 618/651; amenity expense on 33 |

Signals below 98% must not be used without a second discriminator such as property ownership, memo purpose, related invoice, or repeated exact signature.

## Class selection logic

Across the analysed lines:

- 6,573 lines use property-like classes.
- 1,061 use person/operational classes.
- 378 have no class.

Class evidence priority:

1. Exact property/unit token and building name in description or memo.
2. Full address mapped to one class.
3. Related invoice, booking, payout, job, or parent transaction already carrying a class.
4. Utility/customer/account reference with an established one-property mapping.
5. Named person for director/staff-specific transactions.
6. `Parent Entity` for genuinely central costs.

Strong examples:

- `1705 Rent` -> `1705 Bella`.
- `2403 parking` -> `2403 Mainpoint`.
- `XYNERGY ... 6902` -> `6902 Victoria One` (18/18 observed signature).
- `AIRBNB SERVICE 2706W ...` -> `2706W MSQ` (19/19).
- `... 2600 ... TURN CLEAN` -> `2600 Epic` (28/28).
- `... 3003 ... INTERNATIONAL` -> `3003 International` (24/24).
- Jacob/Maggie salary descriptions -> matching person class (80/80 each).

Vendor alone is usually weak class evidence because the same supplier services many properties. Examples include Origin, TPG, iiNet, KeyNest, cleaning suppliers, and Airbnb. Some building-only wording is also ambiguous (`Victoria One`, `Mainpoint`, `Vision`, `Capri`, `Light House`); require the unit number.

## Journal-entry logic

Recent journal entries largely record paired client-property economics with the same property class on both sides:

- `Management Income` paired with `Client Property Expenses:...:Management Fee`.
- `Cleaning Amenities Income` paired with `Client Property Expenses:...:Commercial Cleaning Services`.

Do not classify one side independently. Verify equal/opposite debit-credit structure, the same property class, and the underlying client-property evidence.

For units `10 Toorak Melody` and `14 Agatha`, apply the more specific evidence and controls in [`landlord-statement-journal-patterns.md`](landlord-statement-journal-patterns.md): management fees are 20.9% of client-property revenue; cleaning charges create `Cleaning Amenities Income`; and marked-up landlord costs may be credited to `Management Income`. Never describe the entire recharge as margin unless the corresponding supplier cost has been found and linked.

## Refunds, reversals, deposits, and transfers

- A purchase line can post to revenue when it reverses/refunds revenue.
- A deposit can contain liabilities, expenses, assets, or compensation adjustments; it is not automatically income.
- Security-deposit receipts and refunds stay in `Guest Security Deposits`; cash direction changes, economic nature does not.
- Related international/merchant fees normally use a fee account and inherit the parent transaction's class only when the relationship is proven.
- Transfers between bank/clearing accounts must not be forced into revenue or expense.
- Owner distributions are not generic contractor expenses; use the client owner-distribution path when the payee/property evidence matches.

## Confidence policy

| Tier | Evidence | Allowed outcome |
|---|---|---|
| Deterministic | Exact signature plus exact property/person; at least 20 clean examples and >=98% consistency | May propose a ready-to-review categorisation; never auto-post without explicit authority |
| Strong | Account signal >=90% plus independent class evidence | Propose with evidence and one comparable posted example |
| Moderate | Supplier/category pattern but property ownership or class is ambiguous | Leave pending; list the missing discriminator |
| Weak | Cash direction, generic transfer text, platform name, AI suggestion, or vendor alone | Do not categorise |

Before creating a browser rule, test it against counterexamples from both MSA and client-property accounts and against multi-property suppliers. A rule is unsafe if it can select the right account but the wrong class. Party-only rules are forbidden for `CBA card`, Airbnb, Booking.com, Expedia, Pushkar, Urbanaide, TPG, generic bank transfers, and other conflicts named in the two-year audit.

## Known consistency gaps

- Cleaning has overlapping accounts (`MSA Property Expenses:Cleaning`, `Cleaning Amenities Expense`, client commercial cleaning); supplier alone does not resolve them.
- Internet providers can serve MSA and client properties.
- Posted purchases are 93.8% classed; missing-class lines require review rather than imitation.
- Invoice and sales-receipt lines have much lower line-level class coverage and may depend on item or surrounding object context.
- Some generic retail purchases and broad transfer descriptions lack enough evidence for a reliable class.
- Similar property/building names require unit-level discrimination.
