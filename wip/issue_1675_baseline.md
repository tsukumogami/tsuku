# Issue 1675 Baseline

## Environment
- Date: 2026-02-14
- Branch: feature/1635-hardware-detection (reused from PR #1670)
- Base commit: 95a0c230 (docs(llm): expand #1640 scope to include local E2E validation)

## Test Results

### Rust (tsuku-llm)
- Total: 48 tests
- Passed: 48
- Failed: 0

### Go (internal/llm)
- Total: 237 tests (PASS + SKIP)
- Passed: All non-skipped tests pass
- Skipped: Integration tests requiring API keys or addon binary
- Failed: 0

## Build Status
- Rust: Pass (cargo build succeeds)
- Go: Pass (go build succeeds)

## Pre-existing Issues

### Skipped Integration Tests (intentional)
Three integration tests are skipped pending this fix:
1. `TestIntegration_SIGTERMTriggersGracefulShutdown` - Socket cleanup after SIGTERM
2. `TestIntegration_MultipleSIGTERMIsSafe` - Exit status after multiple SIGTERMs
3. `TestIntegration_gRPCComplete` - Tokenization issue (#1676, separate issue)

These were skipped in commit 7b9a671d as part of issue #1672 work when switching to HuggingFace URLs exposed these pre-existing bugs.

## Issue Summary

The tsuku-llm daemon does not properly clean up socket and lock files on SIGTERM, and exits with "signal: terminated" status instead of status 0.
