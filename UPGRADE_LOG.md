# Dependency Upgrade Log

**Date:** 2026-04-28 12:38 PDT | **Project:** cli-printing-press | **Language:** Go

## Summary

- **Updated/retained:** 21 | **Skipped/pruned:** 10 | **Failed:** 0 | **Needs attention:** 0

## Scope

- Manifest: `go.mod`
- Lock file: `go.sum`
- Fixture manifests under `testdata/golden/` are intentionally left unchanged unless root dependency behavior requires an intentional golden update.
- Baseline toolchain: `go1.26.2`
- Final `go mod tidy` is treated as authoritative for whether a transitive-only module should stay pinned in the project manifest. Modules tested during the sweep but pruned by tidy are listed under **Skipped**.

## Pending Updates

- `github.com/enetx/surf`: v1.0.198 -> v1.0.199
- `github.com/getkin/kin-openapi`: v0.133.0 -> v0.137.0
- `golang.org/x/mod`: v0.33.0 -> v0.35.0
- `golang.org/x/text`: v0.35.0 -> v0.36.0
- Explicit indirect dependencies will be refreshed after direct dependency updates if still stale.

## Updates

### `github.com/enetx/surf`: v1.0.198 -> v1.0.199

- **Research:** GitHub release v1.0.199 adds `SecureTLS()` and `WebSocketGuard()` builder methods. TLS defaults remain backward-compatible; WebSocket guard behavior is now opt-in.
- **Migration:** No code changes needed. Updated `go.mod`, `go.sum`, and `internal/generator/templates/go.mod.tmpl` so newly printed CLIs use the same surf pin.
- **Transitive movement:** `github.com/enetx/g` v1.0.223 -> v1.0.224 and `github.com/andybalholm/brotli` v1.2.0 -> v1.2.1 moved with `surf`.
- **Tests:** `go test ./...` passed.

### `github.com/getkin/kin-openapi`: v0.133.0 -> v0.137.0

- **Research:** GitHub releases from v0.134.0 through v0.137.0 include stricter request-body handling, document-scoped format validators, OpenAPI 3.1 support, origin-tracking fixes, deterministic traversal fixes, and a Go-version revert in v0.137.0.
- **Migration:** Initial compile failed because v0.137.0 imports `github.com/santhosh-tekuri/jsonschema/v6`; ran `go get github.com/getkin/kin-openapi/openapi3@v0.137.0` to record the new dependency and checksum. No project code changes needed.
- **Transitive movement:** `github.com/oasdiff/yaml` moved to v0.0.9, `github.com/oasdiff/yaml3` moved to v0.0.12, and `github.com/santhosh-tekuri/jsonschema/v6` v6.0.2 was added.
- **Tests:** First `go test ./...` failed at compile time due to missing `go.sum` entry; second `go test ./...` passed.

### `golang.org/x/mod`: v0.33.0 -> v0.35.0

- **Research:** The Go project v0.35.0 release notes identify this as a tagged dependency refresh for the module.
- **Migration:** No code changes needed.
- **Transitive movement:** `golang.org/x/crypto` v0.48.0 -> v0.49.0, `golang.org/x/net` v0.50.0 -> v0.52.0, `golang.org/x/sys` v0.41.0 -> v0.42.0, and `golang.org/x/tools` v0.42.0 -> v0.43.0 moved with `x/mod`.
- **Tests:** `go test ./...` passed.

### `golang.org/x/text`: v0.35.0 -> v0.36.0

- **Research:** The Go project v0.36.0 release notes identify this as a tagged dependency refresh for the module.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/go-openapi/jsonpointer`: v0.21.0 -> v0.23.1

- **Research:** Package notes announce RFC6901 trailing `-` array support and optional alternate JSON name providers. The documented API exception affects direct `Pointer.Set` array mutation semantics; this project does not call that API directly.
- **Migration:** No code changes needed.
- **Transitive movement:** Added `github.com/go-openapi/swag/jsonname` v0.26.0.
- **Tests:** `go test ./...` passed.

### `github.com/go-openapi/swag`: v0.23.0 -> v0.26.0

- **Research:** Release notes describe a module split/addition of helper packages, dependency updates, and a Go directive increase to Go 1.25. The project toolchain is Go 1.26.
- **Migration:** No code changes needed.
- **Transitive movement:** Added the new `github.com/go-openapi/swag/*` helper modules at v0.26.0.
- **Tests:** `go test ./...` passed.

### `github.com/mailru/easyjson`: v0.7.7 -> v0.9.2

- **Research:** Release notes describe bug fixes for generated JSON with non-finite floats, `MarshalText` null handling, and support for `json:",omitzero"`.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/spf13/pflag`: v1.0.9 -> v1.0.10

- **Research:** Release notes describe deprecation-comment cleanup and compatibility cleanup around `errors.Is`.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/woodsbury/decimal128`: v1.3.0 -> v1.4.0

- **Research:** Release notes call out a simplified binary marshaller.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `golang.org/x/crypto`: v0.49.0 -> v0.50.0

- **Research:** The Go project v0.50.0 release notes identify this as a tagged dependency refresh for the module.
- **Migration:** No code changes needed.
- **Transitive movement:** `golang.org/x/sys` moved to v0.43.0.
- **Tests:** First `go test ./...` failed because the filesystem ran out of space while writing Go build artifacts. Cleared Go build/test cache with `go clean -cache -testcache`; retry passed.

### `golang.org/x/net`: v0.52.0 -> v0.53.0

- **Research:** The Go project v0.53.0 release notes identify this as a tagged dependency refresh for the module.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `golang.org/x/tools`: v0.43.0 -> v0.44.0

- **Research:** The Go project v0.44.0 release notes identify this as a tagged dependency refresh for the module.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/cpuguy83/go-md2man/v2`: v2.0.6 -> v2.0.7

- **Research:** Transitive-only module; latest stable tag is v2.0.7. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/dave/jennifer`: v1.5.0 -> v1.7.1

- **Research:** Transitive-only module via `github.com/dave/dst`; latest stable tag is v1.7.1. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/dlclark/regexp2`: v1.11.0 -> v1.12.0

- **Research:** Transitive-only module; upstream documents .NET-compatible regex semantics. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/go-openapi/testify/v2`: v2.4.2 -> v2.5.0

- **Research:** Upstream docs describe v2.5.0 work around non-flaky async assertions and standalone internal helper modules; Go 1.24+ is expected. This project uses Go 1.26.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/go-openapi/testify/enable/yaml/v2`: v2.4.2 -> v2.5.0

- **Research:** Sibling module for `github.com/go-openapi/testify/v2`; same v2.5.0 compatibility notes apply. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/go-test/deep`: v1.0.8 -> v1.1.1

- **Research:** Transitive-only test helper; latest stable tag is v1.1.1. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/gorilla/mux`: v1.8.0 -> v1.8.1

- **Research:** Transitive-only HTTP router dependency; latest stable tag is v1.8.1. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/rogpeppe/go-internal`: v1.12.0 -> v1.14.1

- **Research:** Transitive-only helper dependency; latest stable tag is v1.14.1. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/sergi/go-diff`: v1.2.0 -> v1.4.0

- **Research:** Transitive-only diff helper; latest stable tag is v1.4.0. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/stretchr/objx`: v0.5.2 -> v0.5.3

- **Research:** Transitive-only assertion helper; latest stable tag is v0.5.3. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/ugorji/go/codec`: v1.2.7 -> v1.3.1

- **Research:** Transitive-only codec helper; latest stable tag is v1.3.1. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/xyproto/randomstring`: v1.0.5 -> v1.2.0

- **Research:** Transitive-only random string helper; latest stable tag is v1.2.0. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/yuin/goldmark`: v1.4.13 -> v1.8.2

- **Research:** Transitive-only Markdown parser; latest stable tag is v1.8.2. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `go.uber.org/mock`: v0.5.2 -> v0.6.0

- **Research:** Transitive-only mock helper; latest stable tag is v0.6.0. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

### `github.com/kr/pty`: v1.1.1 -> v1.1.8

- **Research:** Transitive-only PTY helper; latest stable tag is v1.1.8. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Transitive movement:** Added `github.com/creack/pty` v1.1.7.
- **Tests:** `go test ./...` passed.

### `github.com/kr/text`: v0.1.0 -> v0.2.0

- **Research:** Transitive-only text helper; latest stable tag is v0.2.0. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Transitive movement:** `github.com/creack/pty` moved to v1.1.9.
- **Tests:** `go test ./...` passed.

### `github.com/creack/pty`: v1.1.9 -> v1.1.24

- **Research:** Transitive-only PTY helper exposed by the post-`kr/text` dependency graph; latest stable tag is v1.1.24. No direct imports in this repo.
- **Migration:** No code changes needed.
- **Tests:** `go test ./...` passed.

## Skipped

### `github.com/jordanlewis/gcassert`: v0.0.0-20250430164644-389ef753e22e

- **Reason:** Available update is another pseudo-version (`v0.0.0-20260313214104-ad3fae17affe`), not a stable release tag. Preserved under the skill's git/pseudo-ref rule.

### `golang.org/x/telemetry`: v0.0.0-20260409153401-be6f6cb8b1fa

- **Reason:** Available update is another pseudo-version (`v0.0.0-20260428171046-76f71b9afea0`), not a stable release tag. Preserved under the skill's git/pseudo-ref rule.

### Tidy-pruned transitive-only modules

- **Reason:** These were updated and tested during the sweep, but final `go mod tidy` removed their explicit pins. `go mod why -m` reports that the main module does not need them, so retaining them would artificially pin metadata-only transitive modules.
- **Modules:** `github.com/cpuguy83/go-md2man/v2`, `github.com/creack/pty`, `github.com/go-openapi/swag`, `github.com/go-openapi/testify/enable/yaml/v2`, `github.com/gorilla/mux`, `github.com/kr/pty`, `github.com/stretchr/objx`, `github.com/yuin/goldmark`.

## Failed

## Needs Attention

## Final Validation

- `go mod tidy` applied.
- `go fmt ./...` passed.
- `go test ./...` passed.
- `go build -o ./printing-press ./cmd/printing-press` passed.
- `scripts/golden.sh verify` passed with 8 golden cases.
- `go vet ./...` passed.
- `golangci-lint run ./...` passed.
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` passed; no vulnerabilities found.
