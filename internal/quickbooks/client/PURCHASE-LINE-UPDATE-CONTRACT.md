# Purchase update: explicit lines and memo

The expense update primitive must distinguish two cases:

- Memo only: preserve existing accounting lines, payment account and payment type.
- Explicit `--line-items`: send the supplied V3 line objects, including their
  category/Class/tax fields, whether or not a memo is also supplied.

Previously the Purchase update builder copied the existing `Line` but never
applied the explicit line-items flag. The server therefore returned a successful
memo update while the requested category change was silently omitted. An
independent accounting readback caught this during a fee-inheritance operation.

The builder now validates and honors explicit JSON lines, consistent with the
other supported line-bearing entities. Regression tests cover explicit lines
with a memo, the shared flag aliases, preservation of payment identity, no input
mutation, and invalid/empty line arrays. Memo-only preservation remains tested.

The corrected primitive was exercised against the same existing expense using
its fresh version. Only the previously omitted category correction was applied;
independent entity and feed reads verified the result, amount/date/tax/payment
identity preservation, and continued bank-feed linkage. No duplicate expense
was created or blind replay attempted.

The result envelope's `account` refers to the payment account, not necessarily
the category on an expense line. Its HTTP status or projected summary cannot
prove a line-level account or Class change. Read the actual accounting lines.

Private target identities and before/command/receipt/readback artifacts remain
in the user's reconciliation workspace. No credentials or private transaction
data are stored in this contract or public regression fixtures.
