# Issue 36 Baseline

## Environment
- Date: 2025-11-28
- Branch: feature/36-tsuku-home-env-var
- Base commit: 3f6ea40369ddb7148f1a8e1a553bf9a111917ed1

## Test Results
- Total: 10 packages tested
- Passed: All
- Failed: None

## Build Status
Pass - no warnings

## Coverage
- Overall: 33.0%
- Key packages:
  - internal/config: 84.6%
  - internal/recipe: 97.0%
  - internal/registry: 78.3%
  - internal/install: 12.4%
- Command used: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`

## Pre-existing Issues
None observed.

## Issue Summary
Allow users to override the default `~/.tsuku` installation directory via the `TSUKU_HOME` environment variable.

### Acceptance Criteria
- `TSUKU_HOME` environment variable is checked in `DefaultConfig()` before falling back to `~/.tsuku`
- Hardcoded path in `internal/install/manager.go:325` (`findPythonStandalone`) uses config instead
- Hardcoded path in `internal/actions/gem_install.go:132` (zig-cc-wrapper) uses config instead
- Unit tests verify env var override behavior
- `TSUKU_HOME=/tmp/test-tsuku tsuku install <tool>` installs to the specified directory
