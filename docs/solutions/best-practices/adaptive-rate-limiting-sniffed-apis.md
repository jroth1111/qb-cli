---
title: Adaptive Rate Limiting for Sniffed API CLIs
date: 2026-03-29
category: best-practices
module: generator
problem_type: best_practice
component: tooling
severity: medium
applies_when:
  - generating CLIs from sniffed or undocumented APIs
  - sniffing live sites for endpoint discovery
  - bulk pagination over reverse-engineered endpoints
tags:
  - rate-limiting
  - sniffed-api
  - adaptive-throttling
  - code-generation
  - proxy-pattern
---

# Adaptive Rate Limiting for Sniffed API CLIs

## Context

Sniffed APIs are undocumented public endpoints reverse-engineered from browser traffic. They are designed for a single browser making occasional calls, but printing-press CLIs make bulk sequential requests -- 100-500+ calls during sync and 10-30 during sniff discovery.

This was discovered building postman-explore-pp-cli. The Postman Explore site rate-limited after ~10 rapid XHR calls during endpoint discovery, and sync pagination across 12 categories triggered repeated 429s. The reactive retry-on-429 approach (already in the generator) was insufficient: each retry wasted 5+ seconds, and repeated 429s risked IP bans.

The core insight: **prevention is cheaper than recovery**. A proactive throttle that starts conservative and adapts is more reliable than detecting and recovering from rate limits.

## Guidance

The solution has two layers that use the same algorithm with different defaults:

### 1. Skill-level pacing (SKILL.md)

Instructions for Claude to pace API probing during the sniff gate:

- Start at 1 second between API calls
- After 5 consecutive successful calls, reduce delay by 20% (min 0.3s)
- On 429, double the delay and log the event
- On 3 consecutive 429s, pause 30 seconds
- Never abort discovery due to rate limits

This is behavioral guidance, not compiled code. Claude follows it during browser-use eval calls.

### 2. Generated CLI rate limiter (client.go.tmpl)

A stdlib adaptive rate limiter compiled into every generated CLI:

- For sniffed APIs (`spec_source: sniffed`), defaults to 2 req/s
- For official APIs, disabled by default (0 req/s = no limiter)
- Users override with `--rate-limit N` (0 to disable)
- Thread-safe via `sync.Mutex` for concurrent sync workers

The algorithm (TCP-congestion-control-inspired):

1. Start at conservative floor (2 req/s)
2. After 10 consecutive successes, increase rate by 25%
3. On 429, halve rate and record discovered ceiling
4. Cap future increases at 90% of ceiling
5. Per-session only -- not persisted across runs

### Design decisions

- **Same algorithm, two implementations**: Skill instructions and compiled Go code use identical logic. One mental model for both contexts.
- **`spec_source` as the signal**: The existing `spec_source: sniffed` catalog field (PR #61) gates whether rate limiting activates. No new schema fields needed.
- **Stdlib only**: Uses `time.Sleep`/`time.Since` and `sync.Mutex`. No `golang.org/x/time/rate` dependency added to generated CLIs.
- **Always present, default varies**: The limiter code exists in every generated CLI. Only the default rate changes based on `spec_source`. This means official API users can opt in with `--rate-limit 2`.

## Why This Matters

Without proactive rate limiting, every sniffed CLI sync is guaranteed to hit 429s. The impact compounds:

- Each 429 wastes 5+ seconds in retry waits
- Undocumented APIs may not send `Retry-After` headers (default fallback is 5s)
- Some APIs escalate to IP bans after repeated 429s
- Without auth, limits are per-IP -- shared IPs (corp networks, VPNs) share the budget
- Agents running CLIs in loops (search, browse, search) accumulate 429s invisibly

The adaptive approach finds the optimal speed per-session. A sync that would have hit the wall 10+ times at full speed completes with zero 429s at a small throughput cost.

## When to Apply

- When the API has `spec_source: sniffed` in the catalog
- When generating a CLI with `--spec-source sniffed`
- When the skill's sniff gate is probing a live site for endpoints
- When sync operations paginate across many categories/resources on an undocumented API

Do **not** apply for official APIs with published rate limits and `Retry-After` headers. Those should use the existing reactive retry logic.

## Examples

**Before (reactive only):**
```
sync start: 200 pages at full speed
page 8: 429 → wait 5s → retry
page 15: 429 → wait 5s → retry
page 23: 429 → wait 5s → retry
...
Total: 200 pages + 10 retries × 5s = 200 pages + 50s wasted
```

**After (adaptive rate limiter at 2 req/s):**
```
sync start: 2 req/s → no 429s through page 100
page 100: rate ramped to ~4 req/s via ceiling-finder
page 150: 429 → halve to 2.5 req/s, ceiling discovered at 5 req/s
remaining: steady at 4.5 req/s (90% of ceiling)
Total: 200 pages, 1 retry, ~60s total
```

**Generated CLI usage:**
```bash
# Sniffed API: rate limiting active by default (2 req/s)
postman-explore-pp-cli sync --resources collections

# Override to faster rate
postman-explore-pp-cli sync --rate-limit 5

# Disable for testing
postman-explore-pp-cli sync --rate-limit 0

# Sync progress shows effective rate
# {"event":"sync_progress","resource":"collections","fetched":50,"rate_rps":2.5}
```

## Related

- PR #61: Added `spec_source`, `auth_required`, `client_pattern` to catalog schema
- PR #62: Implemented adaptive rate limiter in generator templates
- `docs/plans/2026-03-29-001-feat-adaptive-rate-limiting-plan.md`: Implementation plan
- `docs/brainstorms/2026-03-29-sniffed-api-rate-limiting-requirements.md`: Requirements doc
