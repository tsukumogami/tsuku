# Issue #924 Introspection

**Issue**: feat(eval): add npm_install constraint support
**Recommendation**: **Proceed**

## Current State Analysis

### EvalConstraints Infrastructure (Ready)

The `NpmLock` field already exists in `EvalConstraints` struct (`internal/actions/decomposable.go:74-76`):

```go
// NpmLock contains package-lock.json content for npm_install steps.
// Future: will be populated by issue #924.
NpmLock string
```

### Sibling Issues Status

All three sibling issues from Phase 2 are closed:
- #921 (pip): CLOSED - established skeleton and pattern
- #922 (go_build): CLOSED - implemented GoSum extraction
- #923 (cargo_install): CLOSED - implemented CargoLock extraction

### What Needs Implementation

Based on the pattern from sibling issues, issue #924 requires:

1. **Constraint Extraction** (`internal/executor/constraints.go`):
   - Add `extractNpmConstraintsFromSteps()` function
   - Look for `npm_exec` steps with `package_lock` parameter
   - Add helper function `HasNpmLockConstraint()`

2. **Decompose Modification** (`internal/actions/npm_install.go`):
   - Modify `Decompose()` to check `ctx.Constraints.NpmLock`
   - If constraints present, return npm_exec step with the captured lockfile instead of generating new one

3. **Tests** (`internal/executor/constraints_test.go`):
   - `TestExtractConstraints_NpmInstall`
   - `TestExtractConstraints_NpmInstallInDependency`
   - `TestExtractConstraints_NpmInstallFirstWins`
   - `TestExtractConstraints_NpmInstallEmptyLockData`
   - `TestHasNpmLockConstraint`

### Implementation Context

The `wip/IMPLEMENTATION_CONTEXT.md` file already exists with detailed guidance for this issue, confirming the approach.

### Existing Code to Leverage

- `npm_install.go:236` already generates `package_lock` in npm_exec params
- `npm_exec.go` already supports Mode 2 (package install with package_lock)
- Golden files exist with `package_lock` content (wrangler, vercel, cdk, serve, etc.)

### Gap Analysis

| Component | Status |
|-----------|--------|
| EvalConstraints.NpmLock field | Exists |
| ExtractConstraints support | Missing |
| HasNpmLockConstraint helper | Missing |
| npm_install.Decompose constraint check | Missing |
| Tests | Missing |

## Key Finding

The infrastructure is fully in place. The implementation follows the exact same pattern as #922 (go_build) and #923 (cargo_install). The npm_install action already produces `package_lock` in its output; this issue just needs to enable reading that content back during constrained evaluation.

## Blocking Concerns

None. The design is clear, the pattern is established, and the infrastructure is ready.

## Estimated Complexity

Low to Medium. The implementation is straightforward pattern-following:
- ~40 lines for constraint extraction
- ~15 lines for Decompose modification
- ~150 lines for tests

## Files to Modify

1. `internal/executor/constraints.go` - Add npm extraction
2. `internal/actions/npm_install.go` - Use constraints in Decompose
3. `internal/executor/constraints_test.go` - Add npm tests
