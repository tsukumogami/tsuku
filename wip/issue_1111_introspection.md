# Issue 1111 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-platform-compatibility-verification.md`
- Sibling issues reviewed: #1109 (closed), #1110 (closed)
- Prior patterns identified:
  - Libc detection via ELF interpreter parsing (`internal/platform/libc.go`)
  - `Libc` field added to `WhenClause` struct with array syntax
  - `Matchable` interface extended with `Libc()` method
  - `MatchTarget` struct has `libc` field and constructor includes it
  - Libc filtering only applies when OS is "linux"

## Gap Analysis

### Minor Gaps

1. **Step struct location confirmed**: The `Step` struct is in `internal/recipe/types.go` (lines 320-329). The issue correctly identifies the file. The struct currently has: `Action`, `When`, `Note`, `Description`, `Params`, and `analysis` fields. Adding `Dependencies []string` follows the existing pattern.

2. **TOML parsing pattern established**: The `UnmarshalTOML` method (lines 364-466) shows the pattern for extracting fields from `stepMap`. Step-level dependencies should be extracted similarly to how `when` is handled.

3. **Step params exclusion list**: The `UnmarshalTOML` method excludes `action`, `when`, `note`, `description` from Params (line 460-462). Step-level `dependencies` should be added to this exclusion list so it goes into a dedicated field rather than Params.

4. **Dependency resolution happens in actions/resolver.go**: The `ResolveDependenciesForPlatform` function already processes step-level params for `dependencies` at line 89 via `getStringSliceParam(step.Params, "dependencies")`. This means step-level dependencies are already partially supported through params - but they apply regardless of the step's `When` clause matching.

5. **Plan generator needs modification**: The `generateDependencyPlans` function in `internal/executor/plan_generator.go` (lines 653-699) uses `ResolveDependenciesForPlatform` which iterates over ALL steps without checking `When` clause matching. This is the core change needed - dependencies from non-matching steps should be excluded.

### Moderate Gaps

1. **Dependency resolution asymmetry**: The design doc specifies "Dependencies tied to steps that need them - Only resolved if the step matches the target." However, the current implementation in `actions/resolver.go` iterates ALL steps without filtering by target. The issue's acceptance criteria mention this but the implementation details should clarify:
   - The change should be in `ResolveDependenciesForPlatform` to accept a `Matchable` target parameter
   - OR the plan generator should filter dependencies post-resolution

   **Recommended approach**: Modify `ResolveDependenciesForPlatform` to accept an optional target filter and skip steps whose `When` clause doesn't match.

2. **Precedence rules need explicit handling**: The design doc specifies "Dependencies are additive (step inherits recipe-level + its own)" with deduplication. The current resolver already has this precedence:
   - Phase 1: Step-level replaces action implicit
   - Phase 2: Recipe-level replaces ALL step-level
   - Phase 3: Recipe extra extends

   For hybrid recipes, the design recommends "prefer step-level dependencies only." The issue should clarify that when a step has `dependencies = [...]`, it replaces the action's implicit deps for THAT STEP ONLY, not the entire recipe.

### Major Gaps

None identified. The issue spec is complete and aligns with the design document. The two closed sibling issues (#1109, #1110) established the foundation correctly:
- Libc detection works
- Libc filter in WhenClause works
- MatchTarget includes libc parameter

## Recommendation

**Proceed** - The issue specification is complete. Minor gaps can be incorporated during implementation.

## Implementation Notes (from introspection)

The following patterns from prior work should be followed:

1. **Field location**: Add `Dependencies []string` to the `Step` struct in `types.go` (after `Params`)

2. **TOML parsing**: In `UnmarshalTOML`, extract `dependencies` from `stepMap` before adding to `Params`:
   ```go
   // Parse step-level dependencies
   if depsData, ok := stepMap["dependencies"]; ok {
       // similar pattern to libc parsing
   }
   ```

3. **Params exclusion**: Add `"dependencies"` to the exclusion check at line 460

4. **Resolver modification**: Update `ResolveDependenciesForPlatform` signature to accept optional target:
   ```go
   func ResolveDependenciesForPlatform(r *recipe.Recipe, targetOS string, target recipe.Matchable) ResolvedDeps
   ```

   Then in the step loop, skip steps whose `When` clause doesn't match:
   ```go
   if target != nil && step.When != nil && !step.When.Matches(target) {
       continue // Step doesn't match target, skip its dependencies
   }
   ```

5. **Plan generator update**: Pass the target to dependency resolution:
   ```go
   deps := actions.ResolveDependenciesForPlatform(r, targetOS, target)
   ```

6. **Backward compatibility**: When target is nil, resolve ALL dependencies (existing behavior)
