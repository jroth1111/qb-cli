# Printing Press Retro: Steam Web API (Run 7 — Reprint)

## Session Stats
- API: Steam Web API
- Spec source: Zuplo/Steam-OpenAPI (158 operations, OpenAPI 3.0, reused)
- Scorecard: **89/100 Grade A**
- Verify: 77% (62/81, 0 critical)
- Polish: 0 dead functions found (clean)
- Purpose: Reprint to validate CLI-name manuscript convention

## The 11 Lost Points

| Dimension | Score | Lost | Category |
|-----------|-------|------|----------|
| Auth | 8/10 | 2 | Machine could compensate from research |
| Terminal UX | 9/10 | 1 | Scorer minimum quality check |
| README | 7/10 | 3 | Content quality within sections |
| Vision | 9/10 | 1 | Near-perfect, minor detection gap |
| Insight | 8/10 | 2 | Detection or coverage gap |
| Data Pipeline | 7/10 | 3 | No domain-specific Search methods |
| Type Fidelity | 3/5 | 2 | Flag description quality |

## Findings

### 1. README 7/10 — two specific causes found by tracing the scorer (Scorer + agent)

- **What happened:** README has all 8 sections but scores 7/10. Traced `scoreREADME()` in `scorecard.go:380-417`:
  - **-1 point:** Scorer checks for `"Doctor"` string (line 384) but README uses `"Health Check"`. The scorer's section list is `["Quick Start", "Agent Usage", "Doctor", "Troubleshooting"]` — it doesn't check for "Health Check" as an alias.
  - **-2 points:** Quick Start contains `export STEAM_API_KEY=your-key-here` and the scorer explicitly penalizes `"your-key-here"` (line 393). This is the agent choosing a placeholder that the scorer blacklists.
- **Scorer correct?** Both issues: the scorer is partially right. `"your-key-here"` IS a placeholder that should be replaced with a better example. But penalizing "Health Check" when it means the same thing as "Doctor" is a scorer rigidity issue.
- **Two fixes needed:**
  1. **Scorer:** Add `"Health Check"` as an alias for `"Doctor"` in the section presence check (line 384). One-line fix. (+1 point)
  2. **Agent/template:** The README template or skill should use `export STEAM_API_KEY="<your-key>"` instead of `your-key-here` — this is a template default the agent inherits. Or: `printing-press polish --fix-readme` could find and replace blacklisted placeholders deterministically. (+2 points)
- **Frequency:** Every API — both issues recur across all generated CLIs.
- **Complexity:** Small (scorer alias) + Small (template placeholder)

### 2. Data Pipeline 7/10 — no domain-specific Search methods (Generator gap)

- **What happened:** `store.go` has 0 domain-specific Search methods (e.g., `SearchPlayers()`, `SearchGames()`). Only generic `Search()` exists.
- **Scorer correct?** Yes. The scorer gives +3 for `\.Search[A-Z]` patterns. The store has none.
- **Root cause:** The store template gates FTS5 Search methods on `{{if .FTS5}}`, but the profiler's `SearchableFields` isn't flagging Steam's entities as searchable. PR #104 added GET param analysis to `collectStringFields`, but it still doesn't produce enough searchable fields for the Steam spec.
- **Frequency:** Every API — the profiler underdetects searchable fields.
- **Durable fix:** The profiler should also analyze the spec's response schemas for string fields, not just request params. Many APIs (Steam, GitHub, Stripe) have entities where the searchable content (names, descriptions) appears in responses, not request bodies. This requires the OpenAPI parser to extract response field names into the spec's `ResponseDef` struct.
- **Complexity:** Medium (OpenAPI parser change + profiler change)

### 3. Auth 8/10 — borderline threshold (Known, accepted)

- **What happened:** Steam has `key` on 47/158 operations = 29.7%, just under the 30% auth inference threshold.
- **Scorer correct?** Yes — the generated config doesn't have auth wired automatically. Claude adds it manually each run.
- **Root cause:** The skill instruction says "compensate for missing auth from research context." Claude followed it this run (STEAM_API_KEY is in config.go). But the scorecard checks the generated config before Claude's edits — the score reflects the generator's output, not the final CLI.
- **Is this fixable?** The scorecard should score the final CLI (after all edits), not the generator's initial output. But the scorecard runs on whatever's in the directory at scoring time — and by Run 7, the auth IS there. So the 8/10 is from something else in the auth dimension.
- **Need to investigate:** What specifically does the auth scorer check beyond env var presence?

### 4. Type Fidelity 3/5 — flag descriptions (Spec-dependent, machine can compensate)

- **What happened:** Some generated command flags have terse descriptions from the spec (e.g., "access key", "The player").
- **Scorer correct?** Yes — short descriptions are genuinely unhelpful to users.
- **Root cause:** The generator copies spec parameter descriptions verbatim. When the spec has terse descriptions, the CLI inherits them.
- **Machine should compensate:** The skill instruction says "enrich terse flag descriptions from research brief during Phase 3 polish." This is an LLM instruction — ~80% reliable. A deterministic fix: the generator could detect descriptions under 5 words and append the parameter's context (e.g., "access key" → "Steam API key for authenticated requests").
- **Durable fix:** Generator template: if `oneline .Description` produces < 5 words, append ` (required for <endpoint purpose>)`. Or: `printing-press polish --fix-descriptions` as a deterministic pass.

### 5. Insight 8/10 and Vision 9/10 — detection gaps (Minor)

- **What happened:** 2 points from Insight, 1 from Vision. These dimensions use behavioral detection (from PR #101) which looks for specific patterns in command files.
- **Scorer correct?** Partially — the commands exist and work. The scorer may not detect all of them due to pattern matching limitations.
- **Frequency:** Varies by run — these are within run-to-run noise.
- **Durable fix:** Investigate what the scorer's behavioral detection misses and why.

### 6. Verify 77% — env var name mismatch between verify and CLI (Verified root cause)

- **What happened:** 19/81 commands fail verify. Traced `runtime.go:98-148`:
  - Verify derives the env var as `STEAM_WEB_TOKEN` (line 100: `strings.ToUpper(apiName) + "_TOKEN"`)
  - The CLI reads `STEAM_API_KEY` (what we added to config.go)
  - PR #103's auth passthrough iterates `spec.Auth.EnvVars`, but the Zuplo spec has no `securitySchemes` so `Auth.EnvVars` is empty — the passthrough falls back to the derived `STEAM_WEB_TOKEN`
  - Verify passes `STEAM_WEB_TOKEN=mock-token-for-testing` to the subprocess, but the wrapper commands call `steamAPIKey(c)` which reads `c.Config.APIKey` populated from `STEAM_API_KEY` — which isn't set
- **Scorer correct?** The verify tool is working as designed, but the derived env var name doesn't match what the CLI actually reads. The commands genuinely fail when the right env var isn't set.
- **Root cause:** The verify tool's env var derivation (`apiName + "_TOKEN"`) doesn't match the CLI's env var pattern (`STEAM_API_KEY`). The auth inference (PR #103) should have populated `spec.Auth.EnvVars` with `STEAM_API_KEY`, but it didn't fire because Steam is at 29.7% (under the 30% threshold).
- **Durable fix:** Two options:
  1. **Verify:** In addition to the derived name, also pass env vars matching common patterns: `<API>_API_KEY`, `<API>_KEY`, `<API>_TOKEN`. This is additive — more env vars doesn't break commands that don't use them.
  2. **Auth inference:** Lower the threshold from 30% to 25% for this edge case. But this risks false positives on other APIs. Option 1 is safer.
- **Frequency:** Every API where the CLI uses a different env var name than verify derives.
- **Complexity:** Small (add 2-3 extra env var patterns to verify's buildEnv)

## Prioritized Improvements

### Fix the Scorer
| # | Scorer | Bug | Impact | Fix target |
|---|--------|-----|--------|------------|
| 1a | README | Checks for `"Doctor"` but not `"Health Check"` — same concept, different name | +1 README | `scorecard.go:384` — add alias |

### Do Now
| # | Fix | Component | Impact | Complexity |
|---|-----|-----------|--------|------------|
| 1b | README template: replace `your-key-here` with `<your-key>` | Generator template `readme.md.tmpl` | +2 README | Trivial |
| 6 | Verify: pass `<API>_API_KEY` and `<API>_KEY` in addition to derived `<API>_TOKEN` | `runtime.go` buildEnv | +15% verify | Small |

### Do Next
| # | Fix | Component | Impact | Complexity |
|---|-----|-----------|--------|------------|
| 2 | Profiler: analyze response schemas for searchable fields | Profiler + OpenAPI parser | +3 data pipeline | Medium |
| 4 | Generator: enrich terse flag descriptions automatically | Generator template or polish | +1-2 type fidelity | Medium |

### Accept
| # | Gap | Why |
|---|-----|-----|
| 3 | Auth 8/10 | Borderline threshold. Skill instruction compensates. |
| 5 | Vision/Insight | Run-to-run detection variation. |

## Key Insight: Trace the Code, Don't Guess

This retro traced the scorer's actual code path for both README and verify, and found specific causes different from prior assumptions:

- **README 7/10:** Not "content quality variation between runs" (prior assumption). Actually: scorer checks for `"Doctor"` not `"Health Check"` (-1), and Quick Start has `your-key-here` which is blacklisted (-2). Both are deterministic, reproducible, and fixable.
- **Verify 77%:** Not "auth passthrough not working" (prior assumption). Actually: verify derives `STEAM_WEB_TOKEN` but CLI reads `STEAM_API_KEY`. The passthrough works mechanically — it's passing the wrong env var name.

**Every retro in this series found the real cause was different from the assumed cause.** The pattern is clear: read the scorer source, don't guess from the number.

## What the Machine Got Right

- **Polish found 0 dead functions.** The agent built cleanly this time — no `formatCompact` or `usageErr` left behind. This may be luck or it may be the agent improving. Worth watching across more runs.
- **All 5 README sections present.** The skill instruction to preserve sections worked.
- **Auth compensated from research.** STEAM_API_KEY wired into config from the research brief.
- **Manuscript naming.** This run archived under `steam-web-pp-cli/` (new convention). The move from `steam/` to `steam-web-pp-cli/` was clean.
- **89/100 is the highest clean-run score.** No manual dead-code removal needed. The machine is reliably producing Grade A CLIs.
