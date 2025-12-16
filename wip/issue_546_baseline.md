# Issue 546 Baseline

## Environment
- Date: 2025-12-15
- Branch: feature/546-m4-recipe-no-gcc
- Base commit: 64074c4 (main after PR #602 merged)

## Test Results
- All tests pass with `go test -test.short ./...`
- Full test suite not run (short tests sufficient for baseline)

## Build Status
- Build successful: `go build -o tsuku ./cmd/tsuku`

## Existing Infrastructure
- `Dockerfile.integration` exists with minimal deps (no gcc)
- `test/scripts/verify-tool.sh` has verification framework
- `test-configure-make` CI job exists but runs on ubuntu-latest (HAS gcc)
- No current CI validation that source builds work WITHOUT system gcc

## Issue Analysis
The acceptance criteria requires:
1. m4 recipe uses configure_make action
2. m4 builds in minimal container with NO system gcc  
3. `m4 --version` works from tsuku bin
4. CI validates compilation uses zig cc, not gcc
5. Works on all 4 platforms

Key insight: Current `test-configure-make` (gdbm-source) runs on ubuntu-latest
which has gcc pre-installed. To validate "no system gcc", we need either:
- Container-based CI using `Dockerfile.integration`
- Or a job that validates no system compiler is used
