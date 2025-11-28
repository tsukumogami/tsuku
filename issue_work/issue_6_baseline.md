# Issue 6 Baseline

## Issue Summary
Create Go-based integration tests that:
- Use `//go:build integration` build tag
- Run inside Docker container (same as CI environment)
- Leverage existing test-matrix.json
- Allow local execution with `go test -tags=integration ./...`

## Environment
- Date: 2025-11-28
- Branch: feature/6-integration-tests-build-tag
- Base commit: cb3f49e307d2d508479cc386e9428ade67b5cbf7

## Test Results
- All unit tests pass
- Build succeeds

## Existing Infrastructure
- `Dockerfile`: Ubuntu 22.04 minimal environment (wget, curl, ca-certificates)
- `test-matrix.json`: Defines test cases with tool names, tiers, and features
- `.github/workflows/test.yml`: CI workflow running integration tests on ubuntu-latest and macos-latest

## Current CI Integration Tests
The workflow builds tsuku and runs `./tsuku install <tool>` for each tool in the matrix.
Tests run on:
- Linux: T1, T2, T3, T4, T5, T16, T18, T19, T23, T25, T27, T30
- macOS: T1, T3, T4, T16, T18, T19, T23, T25, T27, T30

## Goal
Create Go tests that can:
1. Build and run in Docker container
2. Execute same tests as CI workflow
3. Be invoked with `go test -tags=integration`
