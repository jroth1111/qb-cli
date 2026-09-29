# Two-year categorisation audit

Authoritative read-only audit window: **2024-07-13 through 2026-07-13**, inclusive. Generated against the locked production company Mega Style Apartments on 2026-07-13.

## Coverage

The Accounting API returned 6,454 posted objects and 11,208 extracted lines:

| Transaction family | Objects | Lines | Split objects | Date coverage |
|---|---:|---:|---:|---|
| Purchases | 4,328 | 7,222 | 424 | 2024-07-13 to 2026-07-10 |
| Deposits | 1,948 | 3,635 | 734 | 2024-07-13 to 2026-06-07 |
| Journal entries | 57 | 210 | 57 | 2024-07-31 to 2026-04-30 |
| Invoices | 43 | 108 | 10 | 2024-07-18 to 2026-06-01 |
| Sales receipts | 7 | 17 | 3 | 2025-07-24 to 2026-05-18 |
| Payments | 16 | 16 | 0 | 2024-07-19 to 2025-10-22 |
| Transfers | 55 | no line categorisation | 0 | 2024-07-21 to 2026-05-11 |

No bills, vendor credits, refund receipts, credit memos, or bill payments were returned in the window.

This is the complete set of supported posted transaction families queried for the window, not a claim about pending bank-feed rows. Pending rows remain UI-only and were not used as evidence of accounting policy.

Description-only, subtotal, payment-application, and transfer rows may not carry line account/class fields. Do not count those as categorisation failures without inspecting their detail type and surrounding object.

## Structural findings

- Purchases and deposits dominate the bookkeeping model.
- Splits are normal, not exceptional: 9.8% of purchases and 37.7% of deposits contain multiple posting lines.
- Account and class are independent. The largest accounts span dozens of property classes.
- Property classes dominate operational revenue/expense lines; person classes dominate director and some central costs.
- Transfers are top-level balance-sheet movements and should not be forced into revenue/expense line logic.
- All 7,222 extracted purchase lines exposed tax code `NON` in this window. Treat this as observed company data, not a general tax rule; confirm tax policy before any write.

## Strong account rules across the full window

These signals meet at least 98% consistency over ten or more examples. They may support a proposal, but class still requires separate proof.

| Signal | Account | Evidence |
|---|---|---:|
| iiNet description | `MSA Property Expenses:Internet` | 162/162 across dominant signatures |
| Tangerine party | `MSA Property Expenses:Internet` | 105/105 |
| Origin direct-debit signature | `MSA Property Expenses:Utilities` | 140/141 |
| Origin party variants | MSA utilities | 123/124 combined |
| Winconnect party | `MSA Property Expenses:Utilities` | 50/50 |
| MICM BPAY rent signatures | `Guaranteed Rental - MSA` | 186/188 across dominant signatures |
| CBA transfer explicitly containing rent | `Guaranteed Rental - MSA` | 103/104 |
| Generic transfer explicitly containing stable rent/property wording | `Guaranteed Rental - MSA` | 49/49 |
| Jacob salary signature | `Director Compensation` | 103/103 |
| Maggie salary signature | `Director Compensation` | 103/103 |
| O2 Technology | `MSA Property Expenses:Meet & Greet` | 75/75 |
| KeyNest base-charge signatures | `Meet and Greet Expense` | 170/170 across dominant signatures |
| KeyNest international fee, Rabat MT signature | `Bank charges` | 63/63 |
| Explicit parking/car-park transfer signatures | `MSA Property Expenses:Parking` | 69/69 across dominant signatures |
| Client owner distribution transfer signature | `Client Property Expenses:Owner Distribution` | 74/74 |
| CBA merchant fee signature | `Bank charges:Merchant Fees` | 23/23 |
| Maxo signature | `IT` | 23/23 |
| Bookkeeping transfer signature | `Contractor Payments` | 22/22 |

Cleaning signatures are often account-deterministic only after the address/workflow is included:

- Mainpoint, Victoria One, City Road, Kavanagh Street, Elizabeth Street, Light House and similar turn-clean signatures repeatedly use `MSA Property Expenses:Cleaning`.
- Canterbury Road/Toorak and Haig Street Airbnb-service signatures repeatedly use `Cleaning Amenities Expense`.
- Supplier alone is insufficient because the same cleaning supplier can serve both workflows.

## Known account conflicts that must block automatic rules

| Signal | Observed alternatives | Why it conflicts |
|---|---|---|
| `CBA card` | security deposits, MSA revenue, client revenue, management income, parking | Party label describes payment channel, not substance |
| Pushkar | MSA cleaning 531; cleaning amenities 76 | Supplier spans workflows |
| Urbanaide | MSA cleaning 251; amenities 68; rent 11; client expense 3 | Supplier/service bundle spans purposes |
| TPG | MSA internet 64; client-property internet 27 | Property operating model decides family |
| Airbnb/Booking.com/Expedia | MSA revenue and client revenue | Channel does not determine property ownership |
| Generic bank transfer | security deposit and revenue | Transfer label has no economic meaning |
| KeyNest international-fee variants | bank charges on dominant signature; some MEL/Jacob variants posted as meet-and-greet | Historical inconsistency; related-parent evidence required |
| Generic `parking` merchant text | property parking or motor vehicle expense | Explicit property/space rent uses the property-parking account; ordinary parking-card charges can be `Motor vehicle expenses` with a person class |

When a signal appears in this table, do not create a party-only bank rule.

## Class logic across the full window

No major property account maps to one class:

- `MSA Property Revenue`: 2,610 lines; largest single class only 6.4%.
- `MSA Property Expenses:Cleaning`: 1,817 lines; largest class 5.8%.
- `Guest Security Deposits`: 1,473 lines; largest class 7.3%.
- `Guaranteed Rental - MSA`: 654 lines; largest class 7.5%.
- MSA utilities, repairs, parking, meet-and-greet, keys/fobs and capital expenses similarly span properties.

Therefore account-to-class defaulting is invalid for property operations.

### Reliable class signals

- Exact unit plus building name: e.g. `6902 Victoria One`, `2403 Mainpoint`, `2706W MSQ`.
- Exact address when it maps to one live class.
- Stable external reference known to one utility/property account.
- Linked booking, job, invoice, payout, or parent transaction.
- Named salary recipient: `Jacob` or `Maggie`.
- Named central contractor/person where repeated posted examples support that person class.
- `Parent Entity` only for genuinely central costs; do not use it merely because property evidence is missing.

### Class conflicts

Building or supplier text without a unit is often ambiguous:

- `VISION` -> 5308 or 5310.
- `LIGHTHOUSE` -> 2207 or 3909.
- `CAPRI` -> 1105 or 1303.
- Elizabeth Street/Victoria One signatures span multiple units.
- TPG, iiNet, Origin, Winconnect, KeyNest, Airbnb and cleaning suppliers span many classes.
- `Parking space` spans several properties.

If the unit/reference cannot be recovered, leave the class unresolved.

## Person and central-cost patterns

- `Director Compensation` is perfectly split by the named person: Maggie 151 lines and Jacob 129 lines; the description/payee selects the class.
- `Motor vehicle expenses` -> Maggie in all 119 observed lines.
- A fresh 2024 probe found `SECURE PARKING MELBOURNE` posted to `Motor vehicle expenses` + `Maggie`; do not confuse parking consumption by a person/vehicle with rent or acquisition of a guest-property parking space.
- `Entertainment expenses` -> Maggie in all 28 observed lines.
- Travel spans Jacob (115), Maggie (26), and Parent Entity (7); no single default.
- Office/admin, healthcare, gifts, research, contractor payments, IT and accountancy show mixed or missing classes. Use person/project evidence, not the account alone.
- `Bank charges:Merchant Fees` used `Parent Entity` on all 24 observed lines, but related transaction fees may need the parent's property class instead.

## Journal-entry pairing

All 57 journal entries are multi-line. Dominant patterns pair both sides under the same property class:

- `Management Income` against `Client Property Expenses:...:Management Fee`.
- `Cleaning Amenities Income` against `Client Property Expenses:...:Commercial Cleaning Services`.

Validate debit/credit balance, paired accounts, common class, and underlying client-property evidence as one unit. Never categorise or update one journal-entry line in isolation.

## Split logic

For every multi-line purchase or deposit:

1. Preserve line count, amount totals, account, class, tax code, description and linked entity per line.
2. Identify whether the split represents multiple properties, multiple economic purposes, fees, refunds, or clearing movements.
3. Never flatten a split because one dominant account or class appears elsewhere.
4. Verify sum of lines equals transaction total before and after any approved write.

## Decision confidence after the audit

- **Deterministic proposal**: exact full signature, direction/type, operating model and exact property/person evidence; >=98% historical account consistency and no unresolved counterexample.
- **Strong proposal**: >=90% account consistency plus a second independent discriminator and exact class evidence.
- **Manual review**: party-only mapping, building-only class, MSA/client ambiguity, related-fee ambiguity, split uncertainty, or missing class evidence.
- **Never infer**: account from money direction alone; class from account alone; revenue ownership from platform alone; property from supplier alone.

Even deterministic proposals require explicit authority before posting. Historical consistency is evidence, not permission.
