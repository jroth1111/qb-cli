# Native purchase inspection

`qb gql inspect-transaction --id PURCHASE_ID --json` reads the transaction model
used by the expense form. Add `--credit-card-credit` for a credit-card credit.
An enrolled/captured Ego session supplies the browser space, exact tab, company
and principal. An explicit space override that differs from that capture fails.
Missing pinned tabs or changed identity fail without selecting another page.

The command exposes account-line tax traits, transaction tax mode, entity
version, and staged bank links with their match modes and rule references.
It preserves nulls, rejects omitted required fields, and keeps absent/null/value
observations explicit. The returned node must match the requested company and
transaction, and credit inspection must return a credit-card credit.

## Important distinctions

- A native line's taxable flag is not its posted tax amount.
- A transaction's NOT_APPLICABLE tax mode does not prove statutory tax status.
- A nullable header clearing field does not replace an independent register read.
- Match provenance is not necessarily cosmetic: the observed form dispatches
  manual/automatic matches through an immediate backend Unmatch path, while
  added-and-matched modes take a stage-update path. Unknown modes remain unknown.
- This reader does not execute either path, restore metadata, edit rules, or
  certify downstream integrations or the books.

The purchase reader uses a restricted selection of fields observed in the
expense/credit forms rather than the broader discovery-catalog transaction
fragment. Read-only previews and observations are not permission to post writes.
