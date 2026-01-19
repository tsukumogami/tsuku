# Issue 1044 Implementation Plan

## Summary

Propagate the `RequireEmbedded` option through transitive dependency resolution, enabling validation that action dependencies can be loaded from embedded recipes.

## Approach

Add a `requireEmbedded bool` parameter to the transitive resolution functions and pass it through to `loader.GetWithContext`. This allows callers (like CLI commands with `--require-embedded` flag) to enforce embedded-only resolution for action dependencies.

## Files to Modify

| File | Changes |
|------|---------|
| `internal/actions/resolver.go` | Add `requireEmbedded` parameter to 3 functions |
| `internal/actions/resolver_test.go` | Update test calls to pass `false` for existing tests, add new test for RequireEmbedded |
| `cmd/tsuku/info.go` | Update `ResolveTransitive` call |
| `cmd/tsuku/check_deps.go` | Update `ResolveTransitive` call |

## Implementation Steps

### Step 1: Update resolver.go function signatures

1. Add `requireEmbedded bool` parameter to:
   - `ResolveTransitive()` (line 288)
   - `ResolveTransitiveForPlatform()` (line 309)
   - `resolveTransitiveSet()` (line 350)

2. Update `ResolveTransitive` to forward the parameter:
   ```go
   func ResolveTransitive(
       ctx context.Context,
       loader RecipeLoader,
       deps ResolvedDeps,
       rootName string,
       requireEmbedded bool,
   ) (ResolvedDeps, error) {
       return ResolveTransitiveForPlatform(ctx, loader, deps, rootName, runtime.GOOS, requireEmbedded)
   }
   ```

3. Update `ResolveTransitiveForPlatform` to forward to `resolveTransitiveSet`:
   ```go
   if err := resolveTransitiveSet(ctx, loader, result.InstallTime, []string{rootName}, 0, installVisited, false, targetOS, requireEmbedded); err != nil {
   ```

4. Update `resolveTransitiveSet` to use the flag when loading:
   ```go
   depRecipe, err := loader.GetWithContext(ctx, depName, recipe.LoaderOptions{RequireEmbedded: requireEmbedded})
   ```

### Step 2: Update callers

1. **cmd/tsuku/info.go:92** - Pass `false` (maintains current behavior):
   ```go
   resolvedDeps, err := actions.ResolveTransitive(context.Background(), loader, directDeps, toolName, false)
   ```

2. **cmd/tsuku/check_deps.go:78** - Pass `false` (maintains current behavior):
   ```go
   resolvedDeps, err := actions.ResolveTransitive(globalCtx, loader, directDeps, toolName, false)
   ```

### Step 3: Update tests

1. Update all existing `ResolveTransitive` calls in resolver_test.go to pass `false`
2. Update all `ResolveTransitiveForPlatform` calls to pass `false`
3. Add new test `TestResolveTransitive_RequireEmbedded` that:
   - Uses a mock loader that tracks LoaderOptions passed
   - Verifies RequireEmbedded is propagated to loader calls

## Test Strategy

1. All existing tests should pass with `false` parameter (no behavior change)
2. New test verifies RequireEmbedded propagation
3. Run `go test ./internal/actions/...` to verify

## Notes

- This issue only propagates the flag; the CLI flag (`--require-embedded`) is added in #1045
- Recipe-level dependencies are NOT affected - only transitive resolution
- The distinction between action deps and recipe-level deps is handled at the caller level (e.g., plan_generator)
