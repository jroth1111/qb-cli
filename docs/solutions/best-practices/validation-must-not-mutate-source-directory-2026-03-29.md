---
title: Validation must not mutate the source directory
date: 2026-03-29
category: best-practices
module: publish-workflow
problem_type: best_practice
component: tooling
symptoms:
  - go mod tidy validation silently passed in non-git directories while modifying go.mod/go.sum
  - Compiled binaries left in CLI directory after validation were staged into library payload by CopyDir
  - PRs opened from publish package contained checked-in compiled binaries
root_cause: logic_error
resolution_type: code_fix
severity: high
tags:
  - validation
  - mutation
  - go-mod-tidy
  - copydir
  - binary-cleanup
  - publish
---

# Validation must not mutate the source directory

## Problem

Validation steps in `publish validate` and `publish package` wrote artifacts into the CLI source directory. Since `publish package` copies the entire directory via `CopyDir`, any files created or modified during validation leaked into the packaged output — including compiled binaries and modified `go.mod`/`go.sum` files.

## Symptoms

- `go mod tidy` validation always passed in non-git directories, even when modules were untidy, because the git-based detection silently errored
- `go.mod` and `go.sum` were permanently modified after running `publish validate` on standalone CLI directories
- `publish package` staged a compiled binary alongside source code, bloating the payload and shipping a platform-specific binary into a cross-platform source distribution

## What Didn't Work

- **`git diff --name-only go.mod go.sum`** to detect tidy changes: returns non-zero in non-git directories. Error was swallowed, check reported PASS regardless
- **`git checkout -- go.mod go.sum`** to restore after tidy: also fails in non-git directories, leaving mutation permanent
- **Not tracking whether `findBuiltBinary` created the binary vs. found an existing one**: no way to know what to clean up without destroying user's pre-existing builds

## Solution

Two patterns applied:

**Snapshot-compare-restore for go mod tidy:**

Read `go.mod` and `go.sum` into byte slices before running `go mod tidy`. After tidy completes, compare the new content against the snapshots. Always restore originals regardless of outcome. If `go.sum` didn't exist before and tidy created it, remove it.

```go
// checkGoModTidy snapshots, runs tidy, compares, restores
origMod, _ := os.ReadFile(modPath)
origSum, _ := os.ReadFile(sumPath)

// run go mod tidy...

newMod, _ := os.ReadFile(modPath)
modChanged := string(origMod) != string(newMod)

// Always restore (validation is non-destructive)
_ = os.WriteFile(modPath, origMod, 0o644)
```

**Build to temp directory for binary checks:**

`buildValidationBinary` creates a temp directory inside the CLI directory (`.publish-validate-*`), builds the binary there, and returns a cleanup function. The binary never exists in the source tree proper.

```go
func buildValidationBinary(dir, cliName string) (path string, cleanup func(), err error) {
    tempDir, err := os.MkdirTemp(dir, ".publish-validate-*")
    // build into tempDir, not dir
    // cleanup = func() { os.RemoveAll(tempDir) }
}
```

Additionally, `snapshotFiles` captures the state of known build artifact locations before validation and restores them after, as a defense-in-depth measure.

## Why This Works

- **No git dependency.** The snapshot pattern uses only `os.ReadFile`/`os.WriteFile`. Works identically in git repos, standalone directories, or mounted volumes.
- **Unconditional restore.** Originals are always written back, even if tidy or build fails.
- **Precise cleanup.** Building to a temp directory means the binary never appears in the source tree. The `defer cleanup()` pattern ensures removal even on error paths.
- **Ordering guarantee.** In `publish package`, validation runs first, deferred cleanups fire when validation returns, then `CopyDir` runs against a clean directory.

## Prevention

- **Treat the source directory as read-only during validation.** Any function that writes to the directory being validated must restore original state before returning. This is the implicit contract that `CopyDir` depends on.
- **Build validation artifacts to temp directories, not the source tree.** Use `os.MkdirTemp` inside the directory (so the Go build's module context is correct) with a dot-prefixed name (excluded by `.gitignore`).
- **Functions that create temporary artifacts must report what they created.** The `(value, cleanup func())` return pattern makes cleanup responsibility explicit at the call site.
- **Never depend on git for validation of non-repo directories.** Library CLIs at `~/printing-press/library/` are standalone. Validation logic must work with pure filesystem operations.

## Related Issues

- `internal/cli/publish.go` — `checkGoModTidy`, `buildValidationBinary`, `snapshotFiles`, `runValidation`
- Tangentially related: `docs/solutions/best-practices/checkout-scoped-printing-press-output-layout-2026-03-28.md` — same "don't mutate in place" principle applied to emboss workflows
