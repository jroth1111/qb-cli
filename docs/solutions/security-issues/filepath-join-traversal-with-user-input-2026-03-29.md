---
title: User input in filepath.Join needs traversal protection
date: 2026-03-29
category: security-issues
module: publish-workflow
problem_type: security_issue
component: tooling
symptoms:
  - publish package --category '../../../escape' created directories outside the target root
  - No error raised — filepath.Join silently resolves .. segments
  - staged_dir in JSON output showed the escaped path
root_cause: missing_validation
resolution_type: code_fix
severity: high
tags:
  - path-traversal
  - filepath-join
  - input-validation
  - security
  - publish
---

# User input in filepath.Join needs traversal protection

## Problem

User-supplied input passed directly to `filepath.Join` as a path segment enables directory traversal. The `publish package` command's `--category` flag was concatenated into `filepath.Join(target, "library", category, cliName)`. A value like `../../../escape` caused the staging directory to be created outside the target directory entirely.

## Symptoms

- Running `publish package --category "../../../escape"` created the staged directory at an unexpected absolute path (e.g., `/escape/cli` instead of `/tmp/staging/library/escape/cli`)
- No error was raised — `filepath.Join` silently resolves `..` segments, and `os.MkdirAll` creates any directory the process has write permission for
- The `PackageResult.StagedDir` in JSON output showed the escaped path, but nothing stopped the operation

## What Didn't Work

- **Non-empty check only**: Checking `category == ""` catches blank input but permits any string including path traversal sequences
- **Relying on the OS to prevent writes**: Go's `filepath.Join` resolves `..` without error. `os.MkdirAll` happily creates directories anywhere. There is no built-in guardrail

## Solution

Belt-and-suspenders: input validation AND resolved-path verification.

**Layer 1 — Input validation:** Reject values containing `/`, `\`, or `..` before they reach `filepath.Join`:

```go
if strings.Contains(category, "/") || strings.Contains(category, "\\") || strings.Contains(category, "..") {
    return fmt.Errorf("--category must be a simple slug (no path separators or '..')")
}
```

**Layer 2 — Resolved-path verification:** After `filepath.Join`, verify the absolute path is under the target:

```go
absTarget, _ := filepath.Abs(target)
absStaging, _ := filepath.Abs(stagingCLIDir)
if !strings.HasPrefix(absStaging, absTarget+string(filepath.Separator)) {
    return fmt.Errorf("resolved path %s escapes target %s", absStaging, absTarget)
}
```

The `+ string(filepath.Separator)` suffix prevents false positives where a sibling directory shares a prefix (e.g., `/tmp/staging-evil` would falsely match `/tmp/staging` without the separator).

## Why This Works

- **Layer 1 catches obvious cases** at the input boundary with a clear error message. Fast, user-friendly.
- **Layer 2 catches anything Layer 1 misses.** It operates on the resolved path, not raw input, so encoding variations and platform-specific normalization can't bypass it.
- **Either check alone has edge cases; together they cover each other's gaps.**

## Prevention

- **Treat all user-provided strings used in path construction as untrusted.** Even "name-like" inputs (categories, slugs, project names) become path components the moment they enter `filepath.Join`.
- **Use the belt-and-suspenders pattern.** Validate raw input for traversal characters AND verify the resolved absolute path is contained within the expected root.
- **Codify the containment assertion.** Any function that accepts user input and feeds it to `filepath.Join` should include: `strings.HasPrefix(absResult, absRoot + sep)`. This is cheap, stateless, and catches classes of bugs that input validation alone cannot.
- **Never assume `filepath.Join` is safe.** Unlike URL path joining in some frameworks, Go's `filepath.Join` performs lexical `..` resolution without any security boundary. It is a string operation, not an access-control mechanism.
- **Existing precedent in this codebase:** `sanitizeResourceName()` in `internal/openapi/parser.go` addresses the same vulnerability class for OpenAPI resource names. The `publish package` path through `internal/cli/publish.go` is a separate attack surface that needed its own protection.

## Related Issues

- `internal/cli/publish.go` — `newPublishPackageCmd` category validation and path containment check
- `internal/openapi/parser.go` — `sanitizeResourceName()` for the same vulnerability class in OpenAPI parsing
