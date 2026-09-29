# Property operating models

These definitions and property assignments are explicit user-provided business rules. They take precedence over statistical inference from historical categorisation.

## Operating models

### Arbitrage / rent-to-rent

MSA leases the property from the landlord for an agreed rent and operates it as short-stay accommodation on its own economic account.

- Accommodation revenue -> `MSA Property Revenue`
- Rent paid to landlord/agent -> `Guaranteed Rental - MSA`
- Operating costs -> the relevant `MSA Property Expenses:*` account
- Guest security deposits -> `Guest Security Deposits`
- Class -> exact property/unit

MSA receives the operating upside and bears the rent obligation and operating risk.

### Landlord management

The landlord retains the property's accommodation economics. MSA acts as manager/agent and earns management and service charges.

- Gross accommodation receipts belonging to landlord -> `Client Property Revenue`
- Landlord property costs -> relevant `Client Property Expenses:*` account
- Owner remittance -> `Client Property Expenses:Owner Distribution`
- MSA management fee earned -> `Management Income`
- Cleaning/amenity charge earned by MSA -> `Cleaning Amenities Income`
- Guest security deposits -> `Guest Security Deposits`
- Class -> exact landlord property/unit

MSA's own economic revenue is its management fee and approved service/cost margin, not the landlord's gross accommodation receipts.

## Confirmed landlord-management properties

| Unit | QuickBooks class | Model | Management fee | Cost treatment | Cleaning fee |
|---|---|---|---|---|---|
| 10 | `10 Toorak Melody` | Landlord management | 20.9% of revenue | MSA charges a margin on costs | Use the same cleaning fee as the immediately preceding month |
| 14 | `14 Agatha` | Landlord management | 20.9% of revenue | MSA charges a margin on costs | Use the same cleaning fee as the immediately preceding month |

## Application rules for units 10 and 14

1. Determine the applicable gross revenue base from the property's source records and established accounting treatment.
2. Calculate management income as **20.9% of that revenue base**.
3. Record the property side using the corresponding client-property management-fee expense and record MSA's side as `Management Income`, with the same property class on both sides.
4. Apply the established margin to eligible landlord costs. Do not invent the margin rate: retrieve it from the prior approved treatment or ask when it is not evidenced.
5. Carry forward the cleaning fee from the immediately preceding month for the same property. Fetch and verify that prior-month amount before preparing the current entry; never copy the other property's fee.
6. Preserve paired journal-entry balance, line descriptions, tax treatment, and the exact property class.
7. Before any write, save the prior-period source, calculation, proposed lines, pre-state, and rollback path; after writing, read the journal entry back live.

“Same cleaning fee as last month” defines the amount source, not a fixed perpetual amount. If the preceding month has no approved cleaning-fee entry or has multiple conflicting amounts, leave the item unresolved and request clarification.

Historical owner statements and journals provide a secondary reasonableness check, not authority to override the explicit carry-forward instruction. Unit 14 commonly shows $210 per standard clean, $440 for a long-stay clean, and monthly totals that vary with activity. If the prior-month approved total conflicts with a newly calculated activity total, surface both calculations and leave the proposed amount unresolved unless the user confirms which rule governs that period.

See [`landlord-statement-journal-patterns.md`](landlord-statement-journal-patterns.md) for the statement-to-journal mapping and cost-margin evidence requirements.
