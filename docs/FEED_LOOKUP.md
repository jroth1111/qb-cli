# Feed lookup coverage

`qb feed txn get --query <text> --account-id <id> --limit <n> --json` searches ID, downloaded transaction ID, original bank description and display description across pending, accepted and excluded rows. It keeps the existing case-insensitive and hyphen/underscore-normalized matching behavior. A differing display description is returned as `displayDescription`, so a match on that field can be explained without replacing the original bank text.

`--limit` caps returned matches, not the pages searched. JSON includes `lookup.complete`, `lookup.totalMatches`, `lookup.truncated`, and row/page counts for each review state. Each returned match includes `reviewState`. The older top-level `complete` field belongs to population verification; use `lookup.complete` to assess this command's search coverage.

All three states must reach an empty terminal page whose server total equals the collected unique row count. Neither a short page nor an early total ends the walk. Missing arrays or terminal totals, mismatched totals, wrong accounts, missing/repeated IDs, cross-state drift, network failures and the 60,000-row-per-state ceiling fail instead of returning a successful empty result. These sequential reads are not an atomic snapshot or occurrence-identity proof. An excluded hit does not establish active coverage.

This can take substantially longer than the former first-slice lookup. The earlier implementation fetched only `--limit` recent rows per state before filtering, so it could return zero for older transactions shown by the UI. The UI also has a native `searchFilter`; the CLI walks all pages to preserve its broader ID and normalized-description matching semantics.
