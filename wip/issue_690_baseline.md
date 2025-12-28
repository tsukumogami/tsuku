# Issue 690 Baseline

## Environment
- Date: 2025-12-27
- Branch: docs/when-clause-platform-tuples
- Base commit: f98d79f6fdba599750640c016cafbeb5b2d8f6d8

## Issue Summary
Extend step-level `when` clauses to support platform tuple conditions (`os/arch` format), matching the precision of recipe-level platform constraints and install_guide platform tuple support.

## Design Document
Following `docs/DESIGN-when-clause-platform-tuples.md` which defines:
- Replace `Step.When` map[string]string with structured `WhenClause` type
- Support platform tuples (["darwin/arm64"]) and OS arrays (["darwin"])
- Additive matching semantics (all matching steps execute)
- No backwards compatibility needed (only 2 recipes to migrate)

## Test Results
- Command: `go test -v -test.short ./...`
- Total: All packages tested
- Result: PASS
- All tests passing with no failures

## Build Status
- Command: `go build -o tsuku ./cmd/tsuku`
- Result: PASS
- No warnings or errors

## Pre-existing State
- Current branch already contains design document commits
- No implementation commits yet
- No workflow artifacts exist
- PR #700 already exists for this branch (will reuse)

## Implementation Scope
Based on design document phases:
1. Define WhenClause struct in types.go
2. Update TOML unmarshaling
3. Add Matches() method
4. Update plan_generator.go
5. Extend ValidateStepsAgainstPlatforms()
6. Migrate 2 existing recipes (gcc-libs.toml, nodejs.toml)
7. Add tests and documentation
