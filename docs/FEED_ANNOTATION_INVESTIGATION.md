# Feed annotation investigation — 2026-09-22

Read-only investigation of the currently loaded AU banking UI found that `setCheckNum` sets both the string `checkNum` and `txnTypeId`: nonempty text chooses PURCHASE_CHECK (3), empty chooses PURCHASE (54). Do not implement cardholder tags by replaying this handler assuming it is metadata-only. Server-side credit-card posting behavior has not been tested.

The feed `checkNum`, `addAsQboTxn.referenceNumber`, posted accounting `DocNumber`, `notes`/memo, display description and original bank description are separate fields. Preserve transaction type, original bank text, existing references and Class in any future annotation command. Credit/refund and transfer UI availability differs. No write command was added by this investigation.

Evidence: `/Users/gwizz/Documents/MSA MegaStyle Apartments/authoritive-coles-statements/live-comparisons/annotation-field-investigation-20260922/check-number-handler-evidence.json` and adjacent README. Use a controlled nonproduction pending → fresh read → posted → fresh read test before claiming durability or exposing a production annotation command.


## Verified rollout findings

- Purchase memo updates require existing PaymentType, AccountRef and Line even with sparse:true. Preserve them and independently compare the readback. Some no-tax lines materialize TaxInclusiveAmt equal to Amount; this is recorded separately rather than hidden in a blanket comparison exception.
- Legacy Check purchases and Deposits funded from the Coles credit-card account can reject updates with code 6430. Do not change account/payment type merely to force annotations through.
- BillPayment7251 normalized PayType Check to CreditCard during a sparse memo update. Explicitly requesting its old Check payment fields did not restore that legacy type; original memo was restored. Treat this path as unsafe for preservation until the caller has explicitly accepted the normalization.
- Recategorisation must include existing PrivateNote explicitly: omitting it caused bank identifier masking during a line change. The client now preserves this field and has a regression test.
- Bank-rule class removal is now supported with `--class-id none`; it removes actionType2 and preserves unrelated actions and conditions. Tested and verified live on rules93/116.
- Rule saves succeeded using the UI-captured qbo.intuit.com host header map through qb-cli's URI-host transport. A generic Node fetch returned500; do not replace the working transport with a raw replay on that evidence. Authentication headers remain private.

Evidence: `/Users/gwizz/Documents/MSA MegaStyle Apartments/authoritive-coles-statements/live-comparisons/annotation-rollout-20260922/`.


## Expense form and attachment follow-up

The native expense form GraphQL UpdateTransactions_Transaction path also rejected Check-on-credit-card Purchase25288 with TXN-13009; independent readback was unchanged. Do not advertise the v4 form as a memo-update bypass.

A visible PDF attachment on that Purchase succeeded without altering financial fields or PaymentType. AttachableRef can link one PDF to both Deposit25812 (fee) and26863 (parent); both remained unchanged except SyncToken/LastUpdatedTime. A note-only Attachable was durable but not usefully visible in the expense UI, so the pilot note was removed and a PDF retained. This proves posted metadata association only, not pending-row persistence or classification updates.

Evidence: `authoritive-coles-statements/live-comparisons/expense-slip-investigation-20260922/`.

## Pending cheque-number pilot — 2026-09-24

User-authorised one-row test on Coles feed33184:ofx confirmed that typing
JacobCard changed the client draft type54→3 without a captured QBO save request.
The dropdown offered Expense/Transfer; selecting Expense displayed Expense but
left the underlying draft type3. Independent getTransactions readback while the
label was visible still returned blank checkNum, type54 and no accounting match.
Reload discarded the label and restored the original client state. The final
server row and Purchases for the target date were unchanged; no posting occurred.

Do not expose this client-state sequence as a durable pending-reference command.
No pending store/update endpoint was established. Existing Pending rows with
server-persisted JacobCard/MaggieCard values prove the representation exists, not
that this UI path safely edits it. Preserve original bank text and references,
and require independent pending-only persistence/type checks before any rollout.

Evidence: `authoritive-coles-statements/live-comparisons/pending-cheque-pilot-20260924/`.

## Frontend decompilation — 2026-09-24

CDP inspected all 51 currently loaded `integrations-datain-ui` scripts and
decompiled the relevant action, request mapper, and transaction-service modules.
The Cheque No. onBlur path is Redux-only: `setCheckNum` changes in-memory
`txnTypeId` to PURCHASE_CHECK (3) for nonempty text, updates `checkNum`, and
dispatches `SET_ONLY_TXNS`. It makes no network call.

The batch-accept mapper later serializes `notes` to
`addAsQboTxn.txnMemo`, `txnTypeId` to `addAsQboTxn.txnTypeId`, and `checkNum`
to `olbTxns[].checkNum`. The service sends that model only via
`olb/ng/batchAcceptTransactions` (including the `acceptOnly=true` variant),
which books the transaction. A complete loaded endpoint inventory found no
`saveDraft`, `updateDraft`, `storeTransactions`, `persistTransactions`, or
other pending-row update route. The unrelated `updateTransactions` symbol is a
print/template customisation service.

Therefore do not implement a qb-cli pending-reference update command from the
current frontend. Existing server-returned Pending checkNum values prove the
representation exists, not an in-place edit operation. Accept-then-undo solely
to persist a label is also forbidden: it temporarily books/deletes an accounting
transaction and violates pending/unclassified semantics.

Evidence: `authoritive-coles-statements/live-comparisons/pending-cheque-decompile-20260924-v2/`.

## Native Pending filter and rule-memo pilot — 2026-09-24 UTC

Correction to the broad negative conclusion above: no standalone inline-save
contract was found, but server-side bank rules CAN supply persistent Pending
memos. That does not mean those memos are indexed by the banking search.

Fresh getTransactions searchFilter and the actual Pending UI returned283
JacobCard rows and236MaggieCard rows from existing checkNum values. Thus existing
saved Cheque No. labels are searchable natively; results are not all cardholder
transactions and labels still require independent source validation.

One user-approved temporary rule232selected only account204, money-out and exact
bank text containing sourceCS21112015-T0031/feed34570. Its only action was MEMO1,
with auto-post off and no account/Class/payee/type actions. Its unique marker
persisted in addAsQboTxn.txnMemo while Pending after reload, but both native UI
and server searchFilter returned zero matches for that marker. It also displaced
the prior account/Class suggestions with blank suggestions; nothing was posted.

The rule was deleted and independent readback proved the complete target Pending
row and date-scoped Purchases matched before-state. Other rule definitions and
relative priorities were unchanged. Do not deploy memo-only rules as a verified
native cardholder-filter solution. APPEND_MEMO9 exists in the frontend, but its
presence does not establish memo search indexing or harmless rule precedence.

The first CLI attempt rejected captured HTTP/2 pseudoheaders locally. Readback
proved no rule existed before one retry with colon-prefixed header names removed
in an isolated temporary credentials copy. The original credentials were not
rewritten; the URI-host transport should eventually filter these header names.

Evidence: `authoritive-coles-statements/live-comparisons/pending-column-review-20260924/`.

## Label-bearing Pending import/replacement pilot — 2026-09-25 Melbourne

The user's explicitly approved one-row replacement test proved that the native
CSV upload wizard can persist `data[].checkNum` as JacobCard on an unposted row.
The observed processCsvFile payload includes checkNum both in data and column
mappings. Receipt: one success, zero duplicates, zero overwrites. New feed35697
remained Pending/type54 with the original date, amount, bank text and suggestions;
reload and native JacobCard search independently found it. No accounting record
was created. This is an import/new-ID route, not an existing-ID metadata update.

Cleanup completed after renewed login: GET-only recovery proved the prior
uncertain exclusion had succeeded, original34570was restored to Pending, and
only test import35697was deleted from Excluded. Final readback proves original
row exact restoration, no temporary row in any feed state, and unchanged
date-scoped accounting, rules and Jacob209 populations. No bulk rollout occurred.

The current single-row CSV CLI model omits data[].checkNum and its column mapping.
Any implementation must preserve connected-account import safeguards, require
explicit replacement authority, prevent duplicate active source occurrences,
verify auto-post off and verify final Pending/ledger state independently. Never
silently treat replacement as an in-place field edit.

Evidence: `authoritive-coles-statements/live-comparisons/pending-replacement-pilot-20260925/`.

### Reviewed cohort rollout, 25 September 2026

Native CSV `processCsvFile` replacement, with `data[].checkNum` and
`columnMappings.checkNum`, was subsequently applied to 888 source-tagged,
source-unique Pending merchant debits. All remain Pending with JacobCard and
native server search finds every replacement. This is not an in-place update.
The originals remain recoverable Excluded; a consolidated old→new→source crosswalk
is mandatory. No posting/matching operation is authorised by an annotation task.

CSV replacement can reset unposted category, Payee and Class suggestions. Class
loss was detected on original 35331 / temporary 36351; the original was restored
exactly and only the failed temporary was deleted. The user subsequently explicitly
prioritised card labels over all Pending suggestions, allowing that row and the
Class-held cohort to proceed. This permission does not extend to Posted records,
tax, transaction type, source facts, card-identity guessing or Jacob account209.

Combined final readback: all888 source mappings one-active-representation;
46 protected Posted feed rows and2383 protected accounting entities unchanged;
Jacob feed populations unchanged; rules unchanged and auto-post off. The source
authority/linked-view checks and16 local safety tests pass. Five unrelated rows
were posted and five new feed rows appeared externally; do not infer global
unchanged state from these scoped invariants. Full Pending label coverage is not
claimed:1401 labelled,1343 blank cheque fields and9 preserved numeric references.

Native UI search applies after reload, but its displayed651-row pagination figure
is not a complete count. Use terminal API pagination for coverage proof.
Evidence: `authoritive-coles-statements/live-comparisons/pending-card-rollout-20260925/combined-final/`.
