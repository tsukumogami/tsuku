# Issue 1676 Baseline

## Environment
- Date: 2026-02-14
- Branch: feature/1635-hardware-detection (reused from PR #1670)
- Base commit: bc9c56c1 (chore: clean up wip artifacts for issue #1675)

## Test Results

### Rust (tsuku-llm)
- Total: 48 tests
- Passed: 48
- Failed: 0

### Go (internal/llm)
- Total: All tests pass
- Passed: All non-skipped tests pass
- Skipped: Integration tests requiring API keys or addon binary
- Failed: 0

## Build Status
- Rust: Pass (cargo build succeeds)
- Go: Pass (go build succeeds)

## Pre-existing Issues

### Skipped Integration Test (intentional)
The `TestIntegration_gRPCComplete` test is skipped pending this fix:
- Test sends a simple completion request
- Error: "rpc error: code = Internal desc = Tokenization failed: tokenization failed: llama_tokenize returned negative count"

## Issue Summary

The tsuku-llm daemon fails to tokenize input when processing completion requests. The llama.cpp tokenizer returns a negative count, indicating an error condition. This needs to be fixed for the Complete RPC to work.
