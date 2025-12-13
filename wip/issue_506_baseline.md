# Issue 506 Baseline

## Environment
- Date: 2025-12-13
- Branch: feature/506-plan-loading-utilities
- Base commit: fa00cb2f54ce4ba8beb0aa753d1a879ed7aa03a4

## Test Results
- Total: 18 packages tested
- Passed: 17 packages
- Failed: 1 package (internal/builders - LLM integration tests)

## Build Status
Pass - `go build -o tsuku ./cmd/tsuku` succeeds

## Pre-existing Issues
- `internal/builders` LLM integration tests fail with "no LLM provider available"
- These tests require external LLM providers and are expected to fail without API keys
- Unrelated to this issue's scope

## Relevant Files
The work will primarily involve:
- `cmd/tsuku/install.go` - CLI flag handling
- New file for plan loading utilities (likely `cmd/tsuku/plan_loading.go`)
- Tests in corresponding `_test.go` files
