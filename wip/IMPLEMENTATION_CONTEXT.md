# Implementation Context for Issue #984

## Design Reference
Design: `docs/designs/DESIGN-library-verify-deps.md`

## Issue Summary
Add `IsExternallyManagedFor(target)` method to Recipe struct that determines whether all applicable steps for a given target delegate to external package managers.

## Key Design Decisions

### Why This Matters
The Tier 2 dependency validation system needs to know whether to recursively validate dependencies. For recipes that delegate entirely to external package managers (apt, brew, dnf, etc.), we validate the dependency exists but don't recurse into its transitive dependencies - the external package manager handles that.

### Classification Categories
- **PURE_SYSTEM**: System libraries (libc, libm) - never recurse
- **TSUKU_MANAGED**: Libraries with tsuku recipes - recurse into them
- **EXTERNALLY_MANAGED**: Recipes using apt/brew/etc - validate but don't recurse

### Method Signature
```go
func (r *Recipe) IsExternallyManagedFor(target Matchable, actionLookup func(string) interface{}) bool
```

The `actionLookup` function parameter avoids circular imports between `recipe` and `actions` packages.

### Logic
1. Filter steps by target (check `when` clause matches)
2. For each applicable step, look up the action
3. If action implements `SystemAction` interface with `IsExternallyManaged() == true`, continue
4. If any step is NOT externally managed, return false
5. If all steps are externally managed (or no steps apply), return true

## Dependencies
- Issue #979 (IsExternallyManaged on SystemAction) - COMPLETED (PR #992)

## Files to Modify
- `internal/recipe/types.go` - Add method
- `internal/recipe/types_test.go` - Add tests

## Integration Point
Called from `internal/verify/deps.go` (Issue #989) during recursive dependency validation.
