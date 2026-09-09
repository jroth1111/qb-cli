---
title: "Scorecard accuracy: broadened pattern matching and verify calibration"
date: 2026-03-27
category: logic-errors
module: internal/pipeline
problem_type: logic_error
component: tooling
symptoms:
  - "CLI scoring 57/100 (Grade C) despite 91% runtime verify pass rate and 100% live API tests"
  - "Sync correctness scoring 0/10 when sync logic lives in files other than sync.go"
  - "Dead code dimension scoring 0/5 due to false positives from definition self-matching"
  - "Workflow and insight dimensions undercounting domain-specific commands by 4-6 points"
root_cause: logic_error
resolution_type: code_fix
severity: high
tags:
  - scoring
  - pattern-matching
  - false-positives
  - calibration
  - dead-code-detection
  - verify-integration
  - steinberger
---

# Scorecard accuracy: broadened pattern matching and verify calibration

## Problem

The Steinberger scorecard (`internal/pipeline/scorecard.go`) used static file-pattern analysis to score generated CLIs 0-100 across 18 dimensions. For CLIs that went through GOAT build phases -- where hand-written workflow commands, renamed files, and domain-specific store rewrites are introduced -- the scorecard produced scores diverging 30+ points from actual quality.

## Symptoms

- A CLI scored 57/100 (Grade C) while achieving 91% runtime verify pass rate and 100% live API tests
- `sync_correctness` scored 0/10 because sync logic lived in `channel_workflow.go` instead of the hardcoded `sync.go`
- `dead_code` scored 0/5 from false positives: flags passed as struct arguments and helper functions calling other helpers within the same file
- `workflows` scored 6/10 instead of 10/10 because only 2 of 10 workflow commands matched the narrow 8-prefix list
- `insight` scored 2/10 because only `health.go` matched the 6-prefix list, despite 5 genuine insight commands
- Verify runtime results had zero influence on the final score

## What Didn't Work

The original scorecard approach had 10 distinct bugs:

1. **Hardcoded `sync.go`** -- `scoreSyncCorrectness` and `scoreDataPipelineIntegrity` only read `internal/cli/sync.go`. Any file renamed by GOAT phases scored 0.
2. **`{` always true** -- `strings.Contains(content, "{")` was meant to detect URL path params like `/{guild_id}`. After broadening to all .go files, every Go file contains `{`. Free 3 points with no signal.
3. **`hasNonEmptySyncResources` false-negative** -- checked `strings.Contains(content, "return nil")` globally. When content was all .go files, virtually any Go codebase has `return nil` somewhere.
4. **Dead code self-matching** -- `strings.Contains(allContent, name+"(")` always found the function definition itself, making it impossible to detect dead helpers.
5. **Substring false-positives** -- `strings.Contains(otherCLI, "flags,")` matched `featureFlags,` and `ldflags,`. `"rate"` matched `generate`, `moderate`.
6. **DataPipelineIntegrity cap after Total** -- capping the dimension at 5 happened after Total was already computed with the uncapped value.
7. **Narrow workflow prefix list** -- only 8 prefixes biased toward project-management APIs. Scheduling, payment, and communication domains missed entirely.
8. **Narrow insight prefix list** -- only 6 prefixes. `stats`, `conflicts`, `stale` scored 0 despite being genuine insights.
9. **No structural detection** -- relied solely on filename prefixes, missing store-using commands and aggregation queries.
10. **Verify not incorporated** -- scorecard and verify were completely independent systems.

## Solution

### 1. Search all CLI files instead of hardcoded filename

```go
// Before
content := readFileContent(filepath.Join(dir, "internal", "cli", "sync.go"))

// After
func readAllGoFiles(dir string) string { /* concatenates all .go files */ }
content := readAllGoFiles(filepath.Join(dir, "internal", "cli"))
```

### 2. Fix pattern checks broken by broadened scope

```go
// {  always true -> detect URL path params specifically
if strings.Contains(content, "/{") { score += 3 }

// hasNonEmptySyncResources: check for defaultSyncResources first, not global patterns
func hasNonEmptySyncResources(content string) bool {
    if !strings.Contains(content, "defaultSyncResources") && !strings.Contains(content, "syncResources") {
        return false
    }
    // ... check for non-empty []string{...} literals
}
```

### 3. Dead code detection -- Count >= 2 excludes definition self-match

```go
// Before -- definition self-matches (func filterFields( matches filterFields()
if !strings.Contains(allContent, name+"(") { deadFunctions++ }

// After -- definition = 1 occurrence, call = 2+
if strings.Count(allContent, name+"(") < 2 { deadFunctions++ }
```

### 4. Word-boundary regex for identifier matching

```go
// Before -- false positives on featureFlags, ldflags, comments
flagsPassedAsArg := strings.Contains(otherCLI, "flags,")

// After
flagsPassedRe := regexp.MustCompile(`\bflags[,)]`)
flagsPassedAsArg := flagsPassedRe.MatchString(otherCLI)

// Same for rate detection
rateRe := regexp.MustCompile(`\brate\b|\bRate\b`)
```

### 5. Expanded prefix lists + structural detection

```go
// Workflows: any command using the store IS a workflow command
if strings.Contains(content, "/store") || strings.Contains(content, "store.Open") {
    compoundCommands++
    continue
}

// Insights: store + aggregation patterns = insight
hasAggregation := strings.Contains(content, "COUNT(") || strings.Contains(content, "SUM(") ||
    strings.Contains(content, "GROUP BY") || rateRe.MatchString(content)
if usesStore && hasAggregation { found++ }
```

### 6. Verify calibration with floor formula

```go
// Dimension caps BEFORE tier calculation (keeps Total consistent)
if verifyReport != nil && !verifyReport.DataPipeline && sc.DataPipelineIntegrity > 5 {
    sc.DataPipelineIntegrity = 5
}
// ... compute tiers and Total ...

// Floor AFTER Total (91% verify → 72 minimum score)
// PassRate is already 0-100 (not 0.0-1.0) — do NOT multiply by 100 again
verifyScore := int(verifyReport.PassRate)
floor := (verifyScore * 80) / 100
if sc.Total < floor { sc.Total = floor }
```

### 7. Extracted duplicate infrastructure maps

```go
// Package-level vars replace 4 diverging local copies
var infraCoreFiles = map[string]bool{
    "helpers.go": true, "root.go": true, "doctor.go": true, "auth.go": true,
}
var infraAllFiles = map[string]bool{
    // infraCoreFiles + export.go, import.go, search.go, sync.go, tail.go, analytics.go
}
```

### Design decision: workflow/insight prefix overlap is intentional

Per the Steinberger visionary research plan, analytics/insights ARE compound commands. The plan lists "analytics" alongside "backup" and "moderate" as workflow examples. 6 prefixes (`stale`, `conflicts`, `stats`, `trends`, `health`, `noshow`) intentionally appear in both lists.

## Why This Works

The root cause was a mismatch between the scorecard's assumptions and GOAT-phase reality. The scorecard assumed: predictable file layouts (`sync.go`), no structural rewrites, and that substring matching on concatenated source is reliable. GOAT phases violate all three.

- Fixes 1-2 remove the assumption of fixed filenames and single-file content
- Fixes 3-4 remove the assumption that substring matching is sufficient for identifier detection in concatenated multi-file content
- Fix 5 adds structural detection beyond filename-prefix matching
- Fix 6 bridges static analysis and runtime reality -- when the two disagree, runtime evidence sets a floor
- Fix 7 eliminates consistency drift from duplicated data structures

The verify floor is the critical safety net: no matter how badly static analysis misjudges a GOAT-phase CLI, the score cannot fall more than 20 points below what runtime testing demonstrates.

## Prevention

1. **When broadening file scope, audit every downstream pattern check.** The `readAllGoFiles` change broke 3 separate checks that assumed single-file content (`{` in non-URL contexts, `return nil` from non-sync functions, definition self-matching). Treat scope broadening as a breaking change to every consumer.

2. **Use `Count >= N` not `Contains` when searching content that includes the definition.** Any regex extracting identifiers from source and searching the same source will self-match. The definition itself contains `name(`. General principle for any grep-over-own-source pattern.

3. **Use word-boundary regex (`\b`) for identifier matching in concatenated source.** `strings.Contains(content, "flags,")` will substring-match `featureFlags,`. This applies to any language where identifiers can be substrings of other identifiers.

4. **Apply dimension caps BEFORE computing totals, floors AFTER.** If you modify a dimension after Total is computed, the two become inconsistent. This ordering invariant should be enforced by code structure.

5. **Document intentional overlap in scoring dimensions.** Without a comment, future maintainers will assume shared prefixes are a bug and "fix" them.

6. **Extract repeated data structures as package-level vars.** Four copies of the same map with diverging contents is a consistency landmine.

7. **Check units at the boundary between systems.** `VerifyReport.PassRate` is 0-100 (percentage), not 0.0-1.0 (ratio). Multiplying by 100 again produces values in the thousands. When consuming a value from another module, read its source to confirm the scale.

8. **Gate bonus points on prerequisite signals.** A pattern like `/{` (URL path parameters) exists in most CLIs. Awarding sync-correctness points for it only makes sense when other sync signals (resources, state tracking, pagination) are already present. Otherwise any parameterized API route inflates the score.

## Related Issues

- `docs/plans/2026-03-27-fix-scorecard-accuracy-plan.md` -- the source plan for this work
- `docs/plans/2026-03-25-fix-scorecard-too-easy-real-quality-plan.md` -- predecessor plan addressing the opposite direction (scorecard too easy). Cross-reference: that plan's redesign introduced the patterns that this fix corrects
- `docs/plans/2026-03-27-feat-printing-press-quality-overhaul-plan.md` -- builds the verify infrastructure that Issue 6 (verify calibration) depends on
- `docs/plans/2026-03-25-feat-visionary-research-phase-plan.md` -- defines the Steinberger vision scoring and workflow/insight semantics that guided the overlap decision
