# Action Interface Pattern for `IsExternallyManaged()`

**Date:** 2026-01-17
**Purpose:** Verify how to add `IsExternallyManaged()` to actions

## Summary

**Verdict: ✅ HIGHLY FEASIBLE AND IDIOMATIC**

The existing `SystemAction` interface already has optional metadata methods like `ImplicitConstraint()`. Adding `IsExternallyManaged()` follows the same pattern.

## Current Interface Definition

**File:** `internal/actions/action.go:71-85`

```go
type Action interface {
    Name() string
    Execute(ctx *ExecutionContext, params map[string]interface{}) error
    IsDeterministic() bool
    Dependencies() ActionDeps
}
```

**File:** `internal/actions/system_action.go:49-65`

```go
type SystemAction interface {
    Action
    Validate(params map[string]interface{}) error
    ImplicitConstraint() *Constraint
    Describe(params map[string]interface{}) string
}
```

## Existing Pattern: `ImplicitConstraint()`

Actions declare their platform constraints:

```go
// apt_actions.go:41-44
var debianConstraint = &Constraint{OS: "linux", LinuxFamily: "debian"}

func (a *AptInstallAction) ImplicitConstraint() *Constraint {
    return debianConstraint
}

// brew_actions.go:42-45
var darwinConstraint = &Constraint{OS: "darwin"}

func (a *BrewInstallAction) ImplicitConstraint() *Constraint {
    return darwinConstraint
}
```

## System Package Actions

Actions that delegate to external package managers:

| Action | Package Manager | Platform | File |
|--------|-----------------|----------|------|
| `apt_install` | apt | Debian | apt_actions.go |
| `apt_repo` | apt | Debian | apt_actions.go |
| `apt_ppa` | apt | Debian | apt_actions.go |
| `brew_install` | Homebrew | macOS | brew_actions.go |
| `brew_cask` | Homebrew Cask | macOS | brew_actions.go |
| `dnf_install` | dnf | RHEL | dnf_actions.go |
| `dnf_repo` | dnf | RHEL | dnf_actions.go |
| `pacman_install` | pacman | Arch | linux_pm_actions.go |
| `apk_install` | apk | Alpine | linux_pm_actions.go |
| `zypper_install` | zypper | SUSE | linux_pm_actions.go |

**NOT externally managed** (system config, not package managers):
- `group_add`, `service_enable`, `service_start` - OS primitives
- `require_command`, `manual` - validation/documentation

## Recommended Implementation

### 1. Extend SystemAction Interface

```go
// internal/actions/system_action.go
type SystemAction interface {
    Action
    Validate(params map[string]interface{}) error
    ImplicitConstraint() *Constraint
    Describe(params map[string]interface{}) string
    IsExternallyManaged() bool  // NEW
}
```

### 2. Implement Per Action

```go
// apt_actions.go
func (a *AptInstallAction) IsExternallyManaged() bool {
    return true
}

// system_config.go (group_add, etc.)
func (a *GroupAddAction) IsExternallyManaged() bool {
    return false
}
```

### 3. Recipe-Level Query

```go
// internal/recipe/recipe.go or similar
func (r *Recipe) IsExternallyManagedFor(target platform.Target) bool {
    steps := FilterStepsByTarget(r.Steps, target)

    if len(steps) == 0 {
        return false
    }

    for _, step := range steps {
        action := actions.Get(step.Action)

        // Non-SystemAction = not externally managed
        sysAction, ok := action.(actions.SystemAction)
        if !ok {
            return false
        }

        if !sysAction.IsExternallyManaged() {
            return false
        }
    }
    return true
}
```

## Step Filtering Flow

**File:** `internal/executor/filter.go:18-51`

```go
func FilterStepsByTarget(steps []recipe.Step, target platform.Target) []recipe.Step {
    // Stage 1: Check action's implicit constraint
    if sysAction, ok := action.(actions.SystemAction); ok {
        if constraint := sysAction.ImplicitConstraint(); constraint != nil {
            if !constraint.MatchesTarget(target) {
                return false  // Skip - wrong platform
            }
        }
    }
    // Stage 2: Check explicit when clause
}
```

The filtering already handles platform-specific step selection. After filtering, we check if all remaining steps are externally managed.

## Implementation Checklist

- [ ] Add `IsExternallyManaged() bool` to `SystemAction` interface
- [ ] Implement `return true` for: `apt_install`, `apt_repo`, `apt_ppa`, `brew_install`, `brew_cask`, `dnf_install`, `dnf_repo`, `pacman_install`, `apk_install`, `zypper_install`
- [ ] Implement `return false` for: `group_add`, `service_enable`, `service_start`, `require_command`, `manual`
- [ ] Add `IsExternallyManagedFor(target)` helper on Recipe
- [ ] Integrate with Tier 2 recursive validation

## Edge Cases

| Case | Handling |
|------|----------|
| Recipe with mixed steps | If ANY step is not externally managed → recipe is tsuku-managed |
| Recipe with no steps for target | Return false (not installable) |
| Non-SystemAction steps | Return false (download, extract, etc. are tsuku-managed) |
| require_command action | Return false (validation, not package management) |
