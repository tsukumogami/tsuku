# Implementation Context: Issue #924

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Details

- **Title**: feat(eval): add npm_install constraint support
- **Dependencies**: #921 (completed)
- **Tier**: testable (requires tests)

## Key Design Points

### What This Issue Does

Extend the constrained evaluation infrastructure (established in #921) to support npm_install by:
1. Extracting package_lock from golden files containing npm_install/npm_exec steps
2. Reusing the captured package-lock.json during constrained evaluation instead of live resolution

### Pattern to Follow

From the design doc:

```go
func (a *NpmInstallAction) Decompose(ctx *EvalContext) ([]*Step, error) {
    if ctx.Constraints != nil && ctx.Constraints.NpmLock != "" {
        // Reuse captured package-lock.json content
        return a.decomposeWithConstraints(ctx, ctx.Constraints.NpmLock)
    }
    // Normal resolution (unconstrained)
    return a.decompose(ctx)
}
```

### Files to Modify

Based on the pattern from #921, #922, and #923:
- `internal/executor/constraints.go` - Add extractNpmConstraintsFromSteps()
- `internal/actions/npm_install.go` - Modify Decompose() to use constraints
- `internal/executor/constraints_test.go` - Add tests for npm constraint extraction

### EvalConstraints Field

The EvalConstraints struct already has the NpmLock field defined:

```go
type EvalConstraints struct {
    PipConstraints map[string]string
    GoSum          string
    CargoLock      string
    NpmLock        string  // This is what we need to populate and use
    GemLock        string
    CpanMeta       string
}
```

### Constraint Extraction Logic

Extract `package_lock` parameter from npm_install and npm_exec steps in golden files.
