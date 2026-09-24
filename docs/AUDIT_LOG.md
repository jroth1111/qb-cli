# Reading historical audit events

`qb accounting audit-log get` and `search` accept `--from`, `--to`, and `--offset`. Supply both date bounds as RFC3339 timestamps with explicit timezones. Without bounds, the existing current-month default remains in effect.

```sh
qb accounting audit-log get --from '2026-06-01T00:00:00+10:00' --to '2026-06-30T23:59:59.999+10:00' --limit 50 --offset 0 --json
```

Use the profile for the intended company; check its realm before reading. These commands query the audit service and do not change accounting transactions.

The JSON response retains QBO's `pageInfo.totalLogs`, `startIndex`, and `size`. Increment `--offset` by the returned page size. Confirm stable totals, unique audit event IDs and agreement between collected IDs and the total. A response page is not an exhaustive history. User/query filters are applied locally; use `--limit 50` to avoid dropping filtered results within a fetched page. `truncated: true` explicitly reports local output truncation.

A broad query can report a 10,000-event ceiling. Split any window reporting 10,000 or more into smaller non-overlapping date ranges, repeat until each is below the ceiling, and verify each window separately. Do not request an out-of-range terminal page when a short final page and the stable server total already establish coverage. Preserve the date bounds, offsets, event IDs and response metadata as evidence.

`id` is the audit-event ID. `entityId` is the accounting transaction/entity ID, taken from `ELEMENTID`; it is not a bank-feed ID. `transactionDate` is the affected transaction's date, while `date` is the audit event's timestamp. `amount` preserves the audit value as text. Keep these namespaces separate when linking a mutation to a feed row.

Use `--entity-id <accounting-id>` for a server-side entity filter. `--id` filters audit-event IDs within the returned page; it is not an accounting-entity selector. Entity-filtered responses fail if any returned event belongs to another entity.

For revision evidence, add `--include-snapshots --json` to get or search. Each event's optional `transactionSnapshot` preserves the server's `what.entity.TRANSACTION` JSON, including original numeric literals, historical memo, revision metadata, and line-level feed matches. The default output remains summary-only. Non-transaction events may have no snapshot. This is an event-time snapshot, not current state or a guarantee that all revisions are present; verify pagination and the requested date window. Other raw audit context is not exposed.

The audit authorization differs from the banking authorization. Capture the audit UI's own request through an authenticated browser/OMP relay and store authorization only in the private credential profile. Unknown HTTP-200 response shapes now fail instead of appearing as empty history. No DevTools window is required for CDP capture.

The CLI's historical bounds, page offset, pagination metadata and event/entity IDs were verified against the same page captured from the UI. This does not establish that every other live API operation has been verified.

Adjacent pages may repeat audit IDs where timestamps tie. Verify distinct IDs, not just row counts. When totals and distinct IDs disagree, query the overlapping timestamp ranges separately and reconcile the union against each window total; keep all original pages as evidence. A matching total alone is not a completeness proof.

Audit bodies have a dedicated 16 MiB limit because embedded snapshots can exceed the ordinary API reader's 1 MiB limit. Exceeding the audit limit fails explicitly; request a smaller page rather than accepting truncated JSON. Non-transaction events without `qboAuditId` retain the server's `idempotenceKey` as their ID, so they can participate in coverage checks.

For update events, `date` uses `when.updatedDate` before `when.createdDate`. The latter can be the original record creation time, years before the audit action being investigated.
