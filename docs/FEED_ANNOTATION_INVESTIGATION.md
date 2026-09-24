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
