# Live Neo rule deletion contract

The authenticated Neo rule list uses `Accept: */*`. A captured ATS
`Accept: application/json` can return HTTP 500 for `getRules`. The client
now applies the UI-observed header to Neo reads while leaving V3 JSON reads
alone. A current live CLI read successfully returned zero rules after the
user-requested deletion.

`feed rule delete` no longer treats HTTP 200 alone as success. It requires
the requested ID to exist beforehand and an independent complete rule-list
readback to show exactly that ID removed, with no other ID lost. A failed
readback is an inspection stop, never an invitation to retry the POST blind.

The old CLI delete path (`/lists/olbrules/delete` with `{"id":...}`)
returned HTTP 401. A fresh browser-rule GET showed a separately keyed Neo
`Intuit_APIKey`, distinct from the ATS key; auth capture had discarded
qbo.intuit.com as a service host and preserved stale mutation headers.
The UI's shipped rule client revealed the actual delete contract:
`POST lists/olbrules/batchDelete` with a space-joined ID body. The CLI now
captures the Neo host key on login and uses that route. A successful
batchDelete response may not be a JSON object, so exact before/after rule
membership is the success condition.

Live contract proof used disabled, nonmatching Coles-only rules 213 and
214. The CLI created and edited each with independent QBO readback. Rule
213 was removed by the corrected route but the first CLI version returned
a false error because of the response shape; readback proved it absent.
After correcting that parser, the CLI deleted rule214 with status200 and
target-absent readback. A fresh CLI and independent API read both returned
zero rules. No production transaction could match either test marker,
and auto-post remained off.

The user's confirmed browser UI single and bulk delete flows removed all
53 original bank rules before these tests. The CLI authentication and
delete-contract fixes were derived from fresh UI/CDP request headers and
the UI's own service implementation, then tested against live QBO. Do not
mistake historical rule IDs in posted transactions for surviving rule
definitions.

The saved rule definitions and receipts are outside this public repo under
the user's Coles reconciliation workspace. They include the full original
rules and a SHA-256 manifest; they are not a QBO-native undo, and original
rule IDs may not be recoverable by recreation. No test rule was created in
production after the requested zero-rule end state.
