# Coles annotation and fee-inheritance transport findings

`TransactionDetail.Memo` maps to Neo `addAsQboTxn.txnMemo`.
`ClassRef` maps to each detail's `klassId`: nil preserves the existing
draft value, an empty value removes it, and a nonempty value sets it.
The CLI exposes memo and explicit Class removal with `--memo` and
`--class none`. Paying-card labels are memo metadata, not accounting Class.

Successful categorisation requires exact per-target `acceptedTxns` receipts
including the expected account and accounting references. HTTP 200 with
`errorDetails`, absent receipts, duplicate IDs or wrong IDs is not success.
A transport failure or invalid receipt requires current-state inspection
before retry; the operation may already have happened.

Neo numeric tax identifiers and V3 TaxCodeRef values are not interchangeable.
Do not insert V3 `NON` as Neo `taxCodeId`. Preserve the captured feed payload
and independently verify resulting tax treatment.

Verified attachment fallback: upload a PDF, sparse-update its AttachableRef,
then independently read both the attachment and accounting entity. The
accounting entity may change SyncToken and LastUpdatedTime, but financial
fields, original memo and payment type must remain unchanged. Multi-file
uploads and V3 batch Attachable updates were also verified. Attachments do
not change the bank-description column and do not establish persistent
pending-row memo storage. Pending annotations remain an external queue until
genuine posting; never post solely to store metadata.

Existing Check-type Purchases funded from a credit-card account can reject
memo-only edits. A BillPayment memo edit can normalize payment type even if
the client did not request it. Attachment-only updates avoid resubmitting
these accounting objects; do not silently convert their payment types.

After explicit user approval, nine Check-funded credit-card fees were changed
to CreditCard with preserved record IDs, amounts, dates, tax and feed links.
QBO also changed the internal PurchaseEx TxnType3 to1, materialized Credit=false,
removed unused cheque PrintStatus=NotSet and could remove empty line extensions.
These are recorded type-related differences, not a byte-identical update.
`expenses expense update recategorise --repair-credit-card-type` requires an
explicit opt-in and independently reads the funding Account to verify its
Credit Card type. The default preserves PaymentType. Independent financial
and feed readback remains mandatory; the flag does not promise reversibility.

Neo requests use the UI-observed Accept header (`*/*`); V3 uses JSON.
Rule mutation headers must come from the actual matching UI request.
The existing feed/register helper pagination ceilings have not been removed
by these fixes. Exhaustive audits must still paginate to an empty terminal
page, compare both sort directions, and independently check the register.
