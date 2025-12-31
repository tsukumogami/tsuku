# Issue 761 Implementation Plan

## Goal

Implement `FilterPlan(recipe, target)` that filters recipe steps based on a target tuple.

## Analysis

### Existing Infrastructure

1. **Target struct** (`internal/platform/target.go`):
   - `Target{Platform: "linux/amd64", LinuxFamily: "debian"}`
   - `OS()` and `Arch()` helper methods

2. **Constraint and MatchesTarget** (`internal/actions/system_action.go`):
   - `Constraint{OS: "linux", LinuxFamily: "debian"}`
   - `MatchesTarget(target Target) bool` - checks if constraint is satisfied

3. **SystemAction interface** (`internal/actions/system_action.go`):
   - `ImplicitConstraint() *Constraint` - returns nil for actions with no constraint

4. **WhenClause** (`internal/recipe/types.go`):
   - `Matches(os, arch string) bool` - checks platform/OS conditions
   - Empty clause matches all platforms

5. **actions.Get()** - retrieves action by name, returns `Action` interface

### Implementation Strategy

The function needs two-stage filtering:
1. Check action's `ImplicitConstraint()` against target
2. Check step's explicit `when` clause against target platform

Location: `internal/executor/filter.go` (new file in executor package where plan-related code lives)

### Return Type

The issue spec says `*Plan` but the design doc uses a simplified `Plan{Steps: steps}`. Looking at the existing code, there's no generic `Plan` type - the main plan structure is `InstallationPlan`.

For this issue, I'll return `[]recipe.Step` (the filtered steps) since:
- The function filters recipe steps, not resolved steps
- `InstallationPlan` is for resolved steps after version resolution
- This aligns with how the CLI will use it (iterate filtered steps to generate docs)

## Implementation

### File: `internal/executor/filter.go`

```go
package executor

import (
    "github.com/tsukumogami/tsuku/internal/actions"
    "github.com/tsukumogami/tsuku/internal/platform"
    "github.com/tsukumogami/tsuku/internal/recipe"
)

// FilterStepsByTarget filters recipe steps based on a target platform and linux family.
// Returns only steps that match the target according to two-stage filtering:
//
// Stage 1: Check action's implicit constraint (if any) against target.
// Stage 2: Check step's explicit when clause (if any) against target platform.
//
// A step is included only if both stages pass.
func FilterStepsByTarget(steps []recipe.Step, target platform.Target) []recipe.Step {
    var result []recipe.Step
    for _, step := range steps {
        // Stage 1: Check action's implicit constraint
        action := actions.Get(step.Action)
        if sysAction, ok := action.(actions.SystemAction); ok {
            if constraint := sysAction.ImplicitConstraint(); constraint != nil {
                if !constraint.MatchesTarget(target) {
                    continue // Action's implicit constraint doesn't match target
                }
            }
        }
        // Actions without implicit constraint (non-SystemAction) pass stage 1

        // Stage 2: Check explicit when clause
        if step.When != nil && !step.When.Matches(target.OS(), target.Arch()) {
            continue // Explicit when clause doesn't match target
        }
        // Steps without when clause pass stage 2

        result = append(result, step)
    }
    return result
}
```

### File: `internal/executor/filter_test.go`

Test cases:
1. Empty steps returns empty
2. Steps with no constraints pass through
3. apt_install filtered out for rhel target
4. brew_cask filtered out for linux/amd64 target
5. apt_install passes for debian target
6. brew_cask passes for darwin target
7. Step with explicit when clause filtered correctly
8. Step with both implicit constraint and when clause (both must pass)

## Dependencies

- `internal/actions` - for `Get()` and `SystemAction` interface
- `internal/platform` - for `Target` type
- `internal/recipe` - for `Step` type

## Acceptance Criteria Mapping

- [x] Function: `FilterPlan(recipe *Recipe, target Target) *Plan` -> `FilterStepsByTarget(steps, target)`
- [x] Two-stage filtering: check action's `ImplicitConstraint()`, then check step's explicit `when` clause
- [x] Step included only if both checks pass
- [x] Actions without implicit constraint pass stage 1 automatically
- [x] Steps without explicit `when` pass stage 2 automatically
- [ ] Integration test: `apt_install` step filtered out when target is `rhel`
- [ ] Integration test: `brew_cask` step filtered out when target is `linux/amd64`
