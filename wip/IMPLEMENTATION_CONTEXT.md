# Implementation Context: Issue #922

**Source**: docs/designs/DESIGN-non-deterministic-validation.md

## Issue Summary

**Title**: feat(eval): add go_build constraint support
**Tier**: testable
**Dependencies**: #921 (completed)

## What This Issue Accomplishes

Extend the constrained evaluation infrastructure (implemented in #921) to support go_build actions. When constraints are provided, the go_build Decompose() method should use the GoSum content from the constraints instead of generating new go.sum from live module resolution.

### Key Components to Implement

1. **GoSum constraint extraction**: Parse `go_sum` field from `go_build` steps in golden files
2. **go_build.Decompose() constraint support**: When ctx.Constraints.GoSum is populated, use it instead of live resolution
3. **Tests for go constraint extraction and constrained evaluation**

### Design References

From the design document:

```go
// go_build
func (a *GoBuildAction) Decompose(ctx *EvalContext) ([]*Step, error) {
    if ctx.Constraints != nil && ctx.Constraints.GoSum != "" {
        // Reuse captured go.sum content
        // go mod download with existing go.sum
        return a.decomposeWithGoSum(ctx, ctx.Constraints.GoSum)
    }
    // Normal resolution (unconstrained)
    return a.decompose(ctx)
}
```

The EvalConstraints struct (already added in #921) has a GoSum field:
```go
type EvalConstraints struct {
    PipConstraints map[string]string
    GoSum string           // <- This issue: populate and use this
    CargoLock string
    NpmLock string
    GemLock string
    CpanMeta string
}
```

### What Needs to Be Done

1. Update ExtractConstraints() to parse `go_sum` from `go_build` steps
2. Modify go_build's Decompose() to use constraints when available
3. Add tests for go constraint extraction
4. Ensure existing unconstrained go_build evaluation continues to work

### Exit Criteria

1. `go_sum` is extracted from golden files containing go_build steps
2. When constraints are provided with GoSum populated, go_build produces deterministic output
3. Tests cover constraint extraction and constrained go_build evaluation
4. Existing unconstrained go_build evaluation continues to work
