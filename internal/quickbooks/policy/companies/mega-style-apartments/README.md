# Mega Style Apartments

Company manifest for the verified production company this skill operates.

## Identity

- The verified production company is **Mega Style Apartments**. Confirm that name in the QuickBooks Online banner before reading business data or acting.
- Country: Australia.
- The UI renders dates as `DD/MM/YYYY`.

## Live bank-feed accounts

- `Cole's Credit Cards - Coles MasterCard`
- `Commonwealth Bank of Australia - Bank Transaction Account`
- `Jacob Reimbursement Transactions - Jacob Reimbursement Transactions`

Counts and balances are live and may change during a session. Re-read them after each meaningful navigation and never attribute a count change to the agent without causal evidence.

## Reference index

- [chart-of-accounts.md](chart-of-accounts.md): full live account and class inventory
- [categorisation-patterns.md](categorisation-patterns.md): evidence-based revenue, expense, and class-selection patterns
- [categorisation-decision-model.md](categorisation-decision-model.md): ordered decision logic, measured merchant/signature mappings, confidence tiers, and exceptions
- [two-year-categorisation-audit.md](two-year-categorisation-audit.md): authoritative 2024-07-13 through 2026-07-13 transaction-family coverage, stable mappings, counterexamples, split logic, and class conflicts
- [property-operating-models.md](property-operating-models.md): authoritative rent-to-rent versus landlord-management definitions and confirmed property-specific fee rules
- [policies.yaml](policies.yaml): structured temporal policy registry — every rule with effective window, authority, and evidence (management fee 20.9%, cleaning-fee carry-forward dependency, cost-margin evidence, audit window)
- [landlord-statement-journal-patterns.md](landlord-statement-journal-patterns.md): owner-statement equations, paired monthly journals, cleaning-rate evidence, cost-recharge treatment, and margin verification rules for units 10 and 14
- [expense-classification-policy.md](expense-classification-policy.md): normative expense decision tree, operating-model routing, recoverability and margin logic, splitting, capital-versus-repair tests, and controls against inheriting historically broad categorisation

## Signal lookup

Start here when a pending-row description needs routing to the right reference:

| Description signal | Load first | Then check |
|---|---|---|
| `KEYNEST` base charge | decision-model (Meet and Greet Expense, 116/116) | audit KeyNest conflicts; `INTERNATIONAL FEE KEYNEST` -> `Bank charges` (63/63) |
| `TPG` / `TANGERINE` / `iiNet` internet | expense-classification-policy (Utilities and internet) | decision-model TPG split; party-only rules forbidden |
| `Origin` / `Tango Energy` / `South East Water` utilities | decision-model high-confidence table | class needs property evidence |
| unit `10 Toorak Melody` / `14 Agatha` fees | landlord-statement-journal-patterns (journal mechanics) | property-operating-models (20.9% terms); same class both sides |
| Jacob / Maggie salary | decision-model (Director Compensation) | person class |
| generic transfer / platform name / vendor alone | — | weak evidence; do not categorise without a second discriminator |

Every classification still applies the decision model's ordered logic and the supersession rules below.

## Supersession rules

- The two-year audit supersedes smaller-sample counts in older references when they differ.
- Explicit property assignments in `property-operating-models.md` override inferred historical patterns.
- For every money-out classification, apply `expense-classification-policy.md` before consulting historical merchant mappings. Historical postings are evidence, not policy; primary documents and economic substance take precedence over an accountant's prior use of a broad account.
- Account and class are separate decisions; a strong account match never proves the class.
- For landlord-management fees, cleaning, cost recharges, or owner distributions, check `landlord-statement-journal-patterns.md`. A billed recharge is not proof of its margin: locate the underlying supplier cost before calculating profit.
- The generic authority model — feed vs ledger vs treatment, state facts vs treatment norms, the treatment and mutation ladders — is in [`../../references/authority-model.md`](../../references/authority-model.md).

## Untrusted suggestions

A populated account in Pending `Match/Categorise` is an untrusted suggestion with zero evidentiary weight. It cannot establish account, class, property ownership, operating model, or confidence.

## Snapshot dates

- Chart of accounts: live read-only snapshot generated 2026-07-13.
- Two-year categorisation audit: window 2024-07-13 through 2026-07-13, generated 2026-07-13.
