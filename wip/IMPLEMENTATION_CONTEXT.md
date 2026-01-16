# Implementation Context: Issue #925

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Information

- **Title**: feat(eval): add gem_install constraint support
- **Tier**: simple
- **Dependencies**: #921 (closed - implemented constrained evaluation skeleton)

## Pattern to Follow

This follows the same pattern as:
- #921 (pip) - PipConstraints extraction
- #922 (go_build) - GoSum extraction  
- #923 (cargo_install) - CargoLock extraction
- #924 (npm_install) - NpmLock extraction

## Implementation Requirements

1. **Constraint Extraction** (`internal/executor/constraints.go`):
   - Add `extractGemConstraintsFromSteps()` function
   - Look for `gem_exec` steps with `lock_data` parameter (Gemfile.lock content)
   - Add helper function `HasGemLockConstraint()`

2. **Decompose Modification** (`internal/actions/gem_install.go`):
   - Modify `Decompose()` to check `ctx.Constraints.GemLock`
   - If constraints present, return gem_exec step with the captured lockfile instead of generating new one

3. **Tests** (`internal/executor/constraints_test.go`):
   - TestExtractConstraints_GemExec
   - TestExtractConstraints_GemExecInDependency
   - TestExtractConstraints_GemExecFirstWins
   - TestExtractConstraints_GemExecEmptyLockData
   - TestHasGemLockConstraint

## EvalConstraints Struct

The `GemLock` field already exists in `EvalConstraints` struct (`internal/actions/decomposable.go`):

```go
// GemLock contains Gemfile.lock content for gem_install steps.
// Future: will be populated by issue #925.
GemLock string
```

## Key Insight

gem_exec steps (in golden files) contain a `lock_data` parameter with the Gemfile.lock content. The constraint extraction needs to extract this and populate `constraints.GemLock`. Then `gem_install.Decompose()` uses this instead of running bundler to generate a fresh lockfile.
