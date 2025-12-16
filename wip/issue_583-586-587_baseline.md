# Issue 583, 586, 587 Baseline

Combined PR for Homebrew source build cleanup.

## Environment
- Date: 2025-12-16
- Branch: refactor/583-586-587-homebrew-source-cleanup
- Base commit: 545c5cd814082a6f0e3fba1506ca79c7f537ca34

## Test Results
- All packages pass
- Key packages:
  - github.com/tsukumogami/tsuku/internal/actions: 120.860s
  - github.com/tsukumogami/tsuku/internal/builders: 0.529s
  - github.com/tsukumogami/tsuku/internal/executor: 2.117s

## Build Status
- `go build ./cmd/tsuku`: PASS
- `go vet ./...`: PASS

## Scope

This PR combines three related issues:

1. **#583**: Move and convert source build fixtures (bash.toml, python.toml, readline.toml)
2. **#586**: Remove homebrew_source action
3. **#587**: Remove HomebrewBuilder source build code (~1,500 lines)

## Pre-existing Issues
None identified.
