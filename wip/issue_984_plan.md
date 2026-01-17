# Issue #984 Implementation Plan

## Goal
Add `IsExternallyManagedFor(target Matchable, actionLookup func(string) interface{}) bool` method to the Recipe struct.

## Understanding

### What "Externally Managed" Means
A recipe is "externally managed" for a target when ALL applicable steps delegate to external package managers (apt, brew, dnf, pacman, apk, zypper). This affects dependency recursion in `tsuku verify`:
- Externally managed dependencies are validated but not recursed into
- Tsuku-managed dependencies are recursively validated

### Method Signature
```go
func (r *Recipe) IsExternallyManagedFor(target Matchable, actionLookup func(string) interface{}) bool
```

**Why `actionLookup`?**
- The `recipe` package cannot import `actions` package (would create circular dependency)
- The caller provides a lookup function that returns `interface{}` (to be type-asserted to `SystemAction`)

### Logic
1. For each step in recipe:
   - If step has `When` clause that doesn't match target, skip step
   - Look up the action using `actionLookup(step.Action)`
   - If action doesn't implement `SystemAction`, return `false` (not externally managed)
   - If action implements `SystemAction` but `IsExternallyManaged() == false`, return `false`
2. If all applicable steps are externally managed (or no steps apply), return `true`

## Implementation

### File: `internal/recipe/types.go`

Add method at the end of the file (after `HasChecksumVerification`):

```go
// SystemActionChecker is the interface that SystemAction implements.
// Defined here to avoid importing the actions package.
type SystemActionChecker interface {
    IsExternallyManaged() bool
}

// IsExternallyManagedFor returns true if all steps that apply to the given target
// delegate to external package managers (apt, brew, dnf, etc.).
//
// The actionLookup function should return the action implementation for a given
// action name, or nil if the action doesn't exist. Typically this is actions.Get.
//
// Returns true if:
// - All applicable steps use actions that implement SystemAction with IsExternallyManaged() == true
// - No steps apply to the target (empty recipe for this platform)
//
// Returns false if any applicable step:
// - Uses an action that doesn't implement SystemAction (e.g., download, extract)
// - Uses a SystemAction with IsExternallyManaged() == false (e.g., manual, require_command)
func (r *Recipe) IsExternallyManagedFor(target Matchable, actionLookup func(string) interface{}) bool {
    for _, step := range r.Steps {
        // Skip steps that don't apply to this target
        if step.When != nil && !step.When.Matches(target) {
            continue
        }

        // Look up the action
        action := actionLookup(step.Action)
        if action == nil {
            // Unknown action - conservatively assume not externally managed
            return false
        }

        // Check if action implements SystemAction with IsExternallyManaged
        sysAction, ok := action.(SystemActionChecker)
        if !ok {
            // Action doesn't implement SystemAction - not externally managed
            return false
        }

        if !sysAction.IsExternallyManaged() {
            return false
        }
    }

    // All applicable steps are externally managed (or no steps apply)
    return true
}
```

### File: `internal/recipe/types_test.go`

Add comprehensive tests:

```go
func TestRecipe_IsExternallyManagedFor_AllExternallyManaged(t *testing.T) {...}
func TestRecipe_IsExternallyManagedFor_MixedActions(t *testing.T) {...}
func TestRecipe_IsExternallyManagedFor_NoSteps(t *testing.T) {...}
func TestRecipe_IsExternallyManagedFor_WhenClauseFiltering(t *testing.T) {...}
func TestRecipe_IsExternallyManagedFor_UnknownAction(t *testing.T) {...}
```

## Test Plan

1. All externally managed: Recipe with only brew_install steps returns true
2. Mixed actions: Recipe with brew_install + download steps returns false
3. No steps: Empty recipe returns true
4. When clause filtering: Recipe with platform-specific steps correctly filters
5. Unknown action: Returns false for unknown actions (conservative)
6. Non-SystemAction: Recipe with download action returns false

## Validation Commands

```bash
go test ./internal/recipe/... -v -run IsExternallyManagedFor
go test ./... -count=1
```
