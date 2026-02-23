# Pragmatic Review: Issue #1939 (PyPI wheel-based executable discovery)

**Reviewer**: pragmatic-reviewer
**Commit**: 120fdbceb5438da25b8d84813f584816e045bfad
**Files reviewed**: `internal/builders/artifact.go`, `internal/builders/artifact_test.go`, `internal/builders/pypi.go`, `internal/builders/pypi_wheel_test.go`

## Summary

Clean implementation. The wheel-based discovery is the simplest correct approach for extracting console_scripts from published PyPI artifacts. The fallback chain (wheel -> pyproject.toml -> package name) is well-structured.

One advisory finding on the `artifact.go` extraction, but the design doc explicitly plans RubyGems reuse (phase 5), so the extraction is justified.

## Findings

### 1. [Advisory] `ExpectedContentTypes` -- currently single-caller but design-justified

`internal/builders/artifact.go:26` -- `ExpectedContentTypes` accepts a slice of prefixes, but only one call site exists today (pypi.go:302, passing 3 prefixes). The design doc (DESIGN-binary-name-discovery.md:308) explicitly states RubyGems will reuse this helper with different content types, so the configurability is not speculative -- it's a planned next-phase requirement. No action needed.

### 2. [Advisory] `cachedWheelExecutables` creates temporal coupling on `PyPIBuilder`

`internal/builders/pypi.go:87-88` -- `Build()` populates `cachedWheelExecutables`, then the orchestrator reads it via `AuthoritativeBinaryNames()`. This is the same pattern used by Cargo and npm builders (cargo.go:51, npm.go:60), so it's consistent within the codebase. The comment on line 86-87 documents the coupling. Consistent with existing pattern -- no change needed.

### 3. Non-finding: `downloadArtifact` as separate file

`internal/builders/artifact.go` -- single production caller today (pypi.go:299). Normally I'd flag this as a single-caller abstraction. However: (a) the design doc explicitly plans RubyGems reuse in phase 5 (DESIGN-binary-name-discovery.md:322), and (b) the helper provides clear naming value over inlining 40 lines of HTTP+hash+size logic into `discoverFromWheel`. Not blocking.

### 4. Non-finding: test coverage scope

`artifact_test.go` has 10 test functions for a 60-line helper. Thorough but bounded -- each test covers a distinct code path (HTTPS enforcement, size limit, SHA256, content-type, context cancellation, etc.). The tests are simple and don't introduce test infrastructure that leaks.

## Verdict

**0 blocking, 2 advisory** (both acknowledged as consistent with existing patterns).

The implementation matches the issue scope: wheel download, ZIP extraction, entry_points.txt parsing, BinaryNameProvider integration, pyproject.toml fallback. No scope creep detected -- no refactoring of unrelated code, no speculative features beyond what the design requires.
