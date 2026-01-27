# Issue 1148 Baseline

## Environment
- Date: 2026-01-26
- Branch: fix/1148-regenerate-library-golden-files
- Base commit: 53f6ffc120e29c1b6c30f46dc00a086b13f5fd8d

## Test Results
- All tests pass (short mode)
- Build succeeds

## Current Golden Files State
- `gcc-libs`: has `v15.2.0-linux-amd64.json` (generic, needs per-family)
- `libyaml`: has `v0.2.5-linux-amd64.json` (generic, needs per-family)
- `openssl`: has `v3.6.0-linux-amd64.json` (generic, needs per-family)

## Pre-existing Issues
None - this is cleanup work to regenerate golden files after M47 hybrid libc migration.
