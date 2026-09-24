# Reclassification receipts and legacy credit-card Deposits

A live, single-line UI/CDP capture confirmed that the Reclassify Transactions
screen can submit both `accountId` and `classId` in one request. This is a
wire-contract observation; the current CLI command still exposes account mode
only. Its batch selection is not an exact-transaction repair interface.

The observed production plugin route differed from the earlier test-company
route. Do not assume the route number is a company ID or universally stable.
The captured request used `text/plain;charset=UTF-8`, the reclassification
plugin header, the reclassification-page Referer, and accounting line sequence1.
The register's sequence0 was a different line and must not be substituted.

For the legacy Deposit funded from a credit-card account, the service returned
HTTP200 with an array containing `result: "FailedChangeNotAllowed"` and a
message requiring a different account type. Independent reads proved both the
fee and parent unchanged. V3 and the register-save route also rejected this
combination. This is not a demonstrated type-preserving repair path.

The client now rejects explicit failed, missing-value or unknown `result`
statuses within a receipt array or nested items/results. A partial batch failure
is an inspection stop, never authorization to repeat the batch. HTTP success
and a generic acknowledgement still do not replace independent target readback;
this patch does not claim complete success verification for every response shape.

Private transaction identities, source documents and request/readback receipts
remain in the user's reconciliation workspace, not in public test fixtures.
No authentication material is included here. No production transaction was
created, converted or deleted to test this behavior.
