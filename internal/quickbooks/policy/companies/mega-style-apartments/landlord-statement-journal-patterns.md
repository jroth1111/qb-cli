# Landlord statement and journal classification patterns

Read-only evidence reviewed 2026-07-13 from Mega Style Apartments QuickBooks records and Julie Melo & Gingelly owner statements. The inspected statement history runs from 2023 through February 2026; journal patterns were checked across the available two-year QuickBooks audit window. This reference records classification evidence, not permission to post or change transactions.

## Property identity

| Statement property | QuickBooks class | Operating model |
|---|---|---|
| Melo / unit 10 | `10 Toorak Melody` | Landlord management |
| Gingelly / unit 14 | `14 Agatha` | Landlord management |

The property class belongs on both sides of every paired fee journal. Do not infer unit 10 versus 14 from the account alone.

## Owner-statement equation

For each property and statement period:

`owner net = Client Property Revenue - management fee - cleaning - internet - direct or recharged property costs`

The combined statement adds both properties. The resulting owner distribution is recorded separately through `Client Property Expenses:Owner Distribution`; it is not another expense journal and must not be classified as contractor pay.

## Management-fee journal

For both units, the repeated approved treatment is exactly 20.9% of that property's `Client Property Revenue`:

- Debit `Client Property Expenses:Short-Term Business Operating Expenses:Management Fee`.
- Credit `Management Income`.
- Use the same property class, amount, date-period logic and description on both sides.
- Verify the journal balances and independently recompute `revenue x 20.9%`, using the established rounding treatment.

These journals are usually month-end entries. Unit 10 generally contains only the management-fee pair because its observed occupancy is longer-term; unit 14 commonly also contains cleaning and internet/property costs.

## Cleaning classification

The repeated paired journal is:

- Debit `Client Property Expenses:Short-Term Business Operating Expenses:Commercial Cleaning Services`.
- Credit `Cleaning Amenities Income`.
- Apply the same property class to both lines.

Observed historical pricing includes $210 per standard clean and $440 for a long-stay clean. Monthly totals vary with clean count: for example, $650 represented one $440 long-stay clean plus one $210 standard clean, while other observed totals include $210, $420, $630, $840 and $1,050.

The explicit business rule in [`property-operating-models.md`](property-operating-models.md) says to use the immediately preceding month's approved cleaning-fee amount. Treat historical activity pricing only as a reasonableness check. If carry-forward and activity calculations conflict, do not choose silently: present both and request confirmation.

## Direct costs, recharges and margin

Use three distinct concepts:

1. **Direct/pass-through landlord cost**: debit the relevant `Client Property Expenses:*` account. Internet is a recurring example. No paired MSA income is implied merely because the cost appears on the owner statement.
2. **Recharge billed to landlord**: debit `Client Property Expenses` and credit the established MSA income account, commonly `Management Income` in the observed exceptional-cost journal.
3. **Margin**: `landlord recharge - linked underlying supplier cost`. The credited recharge amount is gross income/recovery, not proof that the whole amount is profit.

May 2025 unit 14 evidence:

| Description | Landlord charge | Journal treatment |
|---|---:|---|
| Parking permit | $300.00 | Debit `Client Property Expenses`; credit `Management Income` |
| Oil heater | $220.00 | Debit `Client Property Expenses`; credit `Management Income` |

The owner statement confirms both charges and the journal confirms income recognition. No explicitly labelled supplier purchase or bill was located in the inspected records, so the underlying costs and actual margins remain unresolved. Do not invent a percentage or label $300/$220 as net margin.

Before proposing any future cost-plus classification:

1. Identify the property and operating model.
2. Find the supplier transaction using date, amount, merchant, attachment, memo and property evidence; labels may differ from the landlord-facing description.
3. Confirm whether the prior approved treatment was pass-through or recharge.
4. Calculate and show supplier cost, landlord charge, dollar margin and percentage basis.
5. Use the exact client-expense and income accounts demonstrated by a comparable approved entry.
6. If the source cost cannot be linked, classify confidence as unresolved and do not assert a margin.

## Classification guardrails

- `Client Property Revenue` is landlord gross accommodation revenue, not MSA revenue.
- MSA economic revenue on these properties is management fees plus approved cleaning/service income and evidenced cost margins.
- Management, cleaning and recharge journals must be assessed as complete balanced pairs, never line-by-line.
- Preserve the exact property class across both sides.
- Do not copy unit 14 cleaning behavior onto unit 10 without property-specific evidence.
- Do not treat an owner-statement deduction as MSA income unless QuickBooks shows the corresponding income side or the approved accounting instruction requires it.
- Reconcile the statement net to the owner distribution separately; timing differences, split payments and historic adjustments require investigation rather than forced matching.
