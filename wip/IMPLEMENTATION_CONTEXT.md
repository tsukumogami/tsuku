# Implementation Context: Issue #923

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Details

- **Title**: feat(eval): add cargo_install constraint support
- **Dependencies**: #921 (completed)
- **Tier**: testable (requires tests)

## Key Design Points

### What This Issue Does

Extend the constrained evaluation infrastructure (established in #921) to support cargo_install by:
1. Extracting `cargo_lock` from golden files containing cargo_install/cargo_build steps
2. Reusing the captured Cargo.lock during constrained evaluation instead of live resolution

### Pattern to Follow

From the design doc:

```go
func (a *CargoInstallAction) Decompose(ctx *EvalContext) ([]*Step, error) {
    if ctx.Constraints != nil && ctx.Constraints.CargoLock != "" {
        // Reuse captured Cargo.lock content
        // cargo install --locked
        return a.decomposeWithCargoLock(ctx, ctx.Constraints.CargoLock)
    }
    // Normal resolution (unconstrained)
    return a.decompose(ctx)
}
```

### Files to Modify

Based on the pattern from #921 and #922:
- `internal/executor/constraints.go` - Add extractCargoConstraintsFromSteps()
- `internal/actions/cargo_install.go` - Modify Decompose() to use constraints
- `internal/executor/constraints_test.go` - Add tests for cargo constraint extraction

### EvalConstraints Field

The EvalConstraints struct already has the CargoLock field defined:

```go
type EvalConstraints struct {
    PipConstraints map[string]string
    GoSum          string
    CargoLock      string  // This is what we need to populate and use
    NpmLock        string
    GemLock        string
    CpanMeta       string
}
```

### Constraint Extraction Logic

Extract `cargo_lock` parameter from cargo_install and cargo_build steps in golden files.
