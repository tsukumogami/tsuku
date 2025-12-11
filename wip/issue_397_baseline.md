# Issue 397 Baseline

## Environment
- Date: 2025-12-10
- Branch: refactor/397-split-resolver
- Base commit: bb47dfdd6b62ed0ffe09583518ce1bfed0e436ae

## Test Results
- Total: 18 packages tested
- Passed: 16 packages
- Failed: 2 packages (pre-existing, unrelated to this work)

### Pre-existing Failures
1. `internal/builders` - TestLLMGroundTruth: LLM integration tests (flaky/environment-dependent)
2. `internal/validate` - TestCleaner_CleanupStaleLocks: Permission denied on stale temp directories

## Build Status
- `go build -o tsuku ./cmd/tsuku`: PASS
- `go vet ./...`: PASS (no warnings)

## Target File Analysis

`internal/version/resolver.go`: 1,269 lines

Current structure:
- Lines 46-110: NewHTTPClient() + validateIP() - HTTP client factory
- Lines 149-364: NewWith*Registry constructors (7 duplicated functions)
- Lines 384-577: GitHub resolution (ResolveGitHub, resolveFromTags, ListGitHubVersions)
- Lines 579-867: npm resolution (isValidNpmPackageName, ListNpmVersions, ResolveNpm)
- Lines 869-925: Node.js resolution (ResolveNodeJS)
- Lines 927-1076: Go toolchain resolution
- Lines 1078-1248: Go proxy resolution
- Lines 744-820: Version utilities (normalizeVersion, compareVersions, isValidVersion)

## Pre-existing Issues
Test failures noted above are pre-existing and unrelated to this refactoring work.
