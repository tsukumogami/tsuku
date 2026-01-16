# Issue 923 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-non-deterministic-validation.md`
- Sibling issues reviewed: #921 (pip constraint skeleton), #922 (go_build constraint support)
- Prior patterns identified:
  1. EvalConstraints struct in `internal/actions/decomposable.go` with CargoLock field
  2. Constraint extraction pattern in `internal/executor/constraints.go`
  3. Decompose() with constraints pattern in `internal/actions/go_install.go`

## Gap Analysis

### Minor Gaps

**Pattern to follow from #921 and #922:**

1. **Constraint extraction in `internal/executor/constraints.go`**: Must add `extractCargoConstraintsFromSteps()` function following the exact pattern of `extractGoConstraintsFromSteps()` and `extractPipConstraintsFromSteps()`. Should look for `cargo_build` steps (not `cargo_install` - those get decomposed) with `lock_data` parameter.

2. **Helper functions in constraints.go**: Add `HasCargoLockConstraint()` following the pattern of `HasGoSumConstraint()` and `HasPipConstraints()`.

3. **Decompose with constraints in cargo_install.go**: The current `Decompose()` method at line 170 does NOT check for constraints. Must add a pattern similar to `go_install.go:329-331`:
   ```go
   if ctx.Constraints != nil && ctx.Constraints.CargoLock != "" {
       return a.decomposeWithConstraints(ctx, ...)
   }
   ```

4. **Add decomposeWithConstraints method**: Create a new method on `CargoInstallAction` that:
   - Uses the constrained Cargo.lock instead of generating one via `generateCargoLock()`
   - Follows the exact pattern from `go_install.go:451-500`

**Field name alignment:**
- Design doc specifies `CargoLock` field in EvalConstraints (already exists in code)
- The cargo_build primitive uses `lock_data` parameter (maps to `CargoLock` in constraints)

### Moderate Gaps

None. The issue scope is clear and the implementation pattern is well-established by #922.

### Major Gaps

None. The infrastructure is fully in place:
- EvalConstraints.CargoLock field exists
- cargo_install.Decompose() exists and generates Cargo.lock
- cargo_build execution handles lock_data parameter

## Recommendation

**Proceed**

## Implementation Checklist

Based on the patterns from #921 and #922:

1. **internal/executor/constraints.go**:
   - Add `extractCargoConstraintsFromSteps()` function
   - Call it from `ExtractConstraintsFromPlan()` and `extractConstraintsFromDependency()`
   - Add `HasCargoLockConstraint()` helper function

2. **internal/actions/cargo_install.go**:
   - Modify `Decompose()` to check for `ctx.Constraints.CargoLock`
   - Add `decomposeWithConstraints()` method that uses constrained Cargo.lock
   - The constrained path should skip `generateCargoLock()` and use `ctx.Constraints.CargoLock` directly

3. **Tests**: Add test case for constrained evaluation with cargo_install (follow pattern from go_install tests)

## Notes

The cargo_build primitive step stores the lockfile as `lock_data` parameter. During constraint extraction, we need to look for this field in cargo_build steps and populate `EvalConstraints.CargoLock`.
