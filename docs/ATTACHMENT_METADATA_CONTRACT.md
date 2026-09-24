# Transaction note attachments and revision metadata

Observed live on23September2026 using `qb feed txn run attach --id <QBO ID> --txn-type Purchase --note ...` and a separately verified Deposit note attachment.

Creating a note-only Attachable can advance the referenced transaction's
`SyncToken` and `MetaData.LastUpdatedTime`. In the observed Purchase case the
token advanced from1 to2. Independent transaction readback showed every other
field, including financial lines, payment type and native `PrivateNote`, unchanged.

Do not require byte-for-byte equality of the entire transaction after attaching
a note. Whitelist those two revision metadata fields, then compare every other
field. This is not permission to ignore account, Class, amount, date, tax, payment
type or native memo changes.

An upload/attach response is not sufficient proof: read the returned Attachable
ID, require its exact Note and typed EntityRef, and independently read the target
transaction. If readback fails after a successful create response, resume those
reads against the saved ID; do not repeat the create and produce duplicate notes.

Any later transaction update must fetch the current SyncToken rather than reuse
the pre-attachment version. Source evidence is retained in the Coles
`group-allocation-20260923/notes/26209` reconciliation receipt directory.
