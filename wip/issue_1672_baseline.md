# Issue 1672 Baseline

## Environment
- Date: 2026-02-14
- Branch: feature/1635-hardware-detection (reusing from issues #1635-#1638)
- Base commit: a5556d8c (docs(llm): add issue #1672 for HuggingFace model manifest)

## Test Results

### Go Tests
- Total: Multiple packages
- Status: 2 pre-existing failures (unrelated to this issue)
  - `internal/sandbox`: TestSandboxIntegration failures (container/family issues)
  - `internal/validate`: TestEvalPlanCacheFlow (404 on github_file download)
- Build: Passes

### Rust Tests (tsuku-llm)
- Total: 48 tests
- Passed: 48
- Failed: 0

## Build Status
- Go CLI: Passes (`go build ./cmd/tsuku`)
- Rust addon: Passes (`cargo build --release`)

## Pre-existing Issues
1. Sandbox integration tests fail due to container spec issues (unrelated)
2. Validate integration test fails due to 404 on github file (unrelated)
3. Integration tests in `internal/llm` skip when model CDN unavailable (this is what #1672 fixes)

## Current State
The model manifest in tsuku-llm uses placeholder URLs (`cdn.tsuku.dev/models/...`) that don't exist.
This issue updates the manifest to use real HuggingFace Hub URLs.
