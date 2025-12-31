# Issue 761 Introspection

## Context Reviewed

- **Design doc**: `docs/DESIGN-system-dependency-actions.md`
- **Sibling issues reviewed**: #754, #755, #756, #759, #760 (all closed since issue creation)
- **Prior patterns identified**:
  - `Target` struct in `internal/platform/target.go` with `Platform` and `LinuxFamily` fields
  - `Constraint` struct in `internal/actions/system_action.go` with `MatchesTarget()` method
  - `SystemAction` interface with `ImplicitConstraint() *Constraint` method
  - All PM actions (apt, brew, dnf, pacman, apk, zypper) implement `ImplicitConstraint()`
  - `WhenClause` in `internal/recipe/types.go` with `Matches(os, arch string) bool`

## Gap Analysis

### Minor Gaps

1. **File location established**: The function should go in `internal/executor/` or `internal/platform/` based on context. Since it filters plans (which are executor concerns) but uses platform/Target types, `internal/executor/plan_filter.go` is the natural location, following the pattern of `plan_generator.go`.

2. **Existing plan filtering in plan_generator.go**: The `shouldExecuteForPlatform()` function already filters steps by `WhenClause`, but it only takes `(when *recipe.WhenClause, targetOS, targetArch string)`. The new `FilterPlan` function needs to also check `ImplicitConstraint()`.

3. **WhenClause.Matches() signature**: The existing method is `Matches(os, arch string) bool`. Issue says "check step's explicit `when` clause against target platform". Need to adapt - either use `target.OS()` and `target.Arch()` separately, or the issue's wording "target platform" (which is the `target.Platform` field like "linux/amd64") should be parsed.

4. **Action lookup pattern**: To call `ImplicitConstraint()`, need to cast `actions.Get(step.Action)` to `SystemAction` interface. Non-system actions return `nil` from the registry cast (standard Go pattern).

5. **Return type clarification**: Issue says `*Plan` but the codebase doesn't have a `Plan` type. There's `InstallationPlan` in `internal/executor/types.go`. The design doc pseudocode shows `Plan{Steps: steps}`, suggesting a simpler struct. Given this is for filtering (not full plan generation), a new lightweight `FilteredPlan` struct or returning `[]recipe.Step` is appropriate.

### Moderate Gaps

None identified. All requirements are clearly stated and align with implemented patterns.

### Major Gaps

None identified. Dependencies (#754, #760) are complete and provide all necessary infrastructure.

## Recommendation

**Proceed**

The issue spec is complete. All blocking dependencies are merged, patterns are well-established, and the implementation path is clear:

1. Create `internal/executor/plan_filter.go` (or add to existing file)
2. Define `FilterPlan(recipe *Recipe, target platform.Target) []recipe.Step` (adjust return type based on usage context)
3. For each step:
   - Get action via `actions.Get(step.Action)`
   - If action implements `SystemAction`, call `ImplicitConstraint()` and check via `constraint.MatchesTarget(target)`
   - If step has `When` clause, check via `step.When.Matches(target.OS(), target.Arch())`
   - Include step only if both pass (or constraint is nil / when is nil)
4. Add integration tests as specified

## Proposed Amendments

None needed. The issue spec is sufficient for implementation.

## Implementation Notes

Key code patterns to follow:

```go
// Pattern for checking ImplicitConstraint
action := actions.Get(step.Action)
if sysAction, ok := action.(actions.SystemAction); ok {
    if constraint := sysAction.ImplicitConstraint(); constraint != nil {
        if !constraint.MatchesTarget(target) {
            continue // Skip step
        }
    }
}

// Pattern for checking WhenClause
if step.When != nil && !step.When.Matches(target.OS(), target.Arch()) {
    continue // Skip step
}
```

Files to reference:
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/platform/target.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/system_action.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/types.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/executor/plan_generator.go`
