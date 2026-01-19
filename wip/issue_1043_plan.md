# Issue 1043 Implementation Plan

## Summary

Add a `LoaderOptions` struct with `RequireEmbedded` field to the recipe loader, modifying `Get` and `GetWithContext` to accept options. When `RequireEmbedded=true`, the loader skips local and registry lookups, only checking the embedded FS.

## Approach

Modify the existing loader methods to accept an options struct as a parameter. This is a signature change that requires updating all callers to pass `LoaderOptions{}` for default behavior. The approach is additive - existing behavior is preserved when `RequireEmbedded=false` (the zero value).

### Alternatives Considered

- **Separate method (e.g., `GetEmbeddedOnly`)**: Cleaner API but doesn't compose well for future options and requires consumers to know which method to call in different contexts.
- **Builder pattern or functional options**: More flexible but overkill for a simple boolean flag; adds complexity without benefit for this use case.

## Files to Modify

- `internal/recipe/loader.go` - Add `LoaderOptions` struct, update `Get` and `GetWithContext` signatures
- `internal/recipe/loader_test.go` - Update all test calls to pass `LoaderOptions{}`, add tests for `RequireEmbedded=true`
- `internal/actions/resolver.go` - Update `RecipeLoader` interface to match new signature
- `internal/verify/deps.go` - Update `RecipeLoader` interface (`LoadRecipe` method)
- `cmd/tsuku/helpers.go` - Update `loader.Get` call
- `cmd/tsuku/versions.go` - Update `loader.Get` call
- `cmd/tsuku/verify.go` - Update `loader.Get` call
- `cmd/tsuku/install_lib.go` - Update `loader.Get` call
- `cmd/tsuku/remove.go` - Update `loader.Get` calls (2 locations)
- `cmd/tsuku/install.go` - Update `loader.Get` call
- `cmd/tsuku/outdated.go` - Update `loader.Get` call
- `cmd/tsuku/info.go` - Update `loader.Get` call
- `cmd/tsuku/verify_deps.go` - Update `loader.Get` call
- `cmd/tsuku/install_deps.go` - Update `loader.Get` calls (2 locations)
- `cmd/tsuku/search.go` - Update `loader.Get` call
- `cmd/tsuku/check_deps.go` - Update `loader.Get` call
- `cmd/tsuku/eval.go` - Update `loader.Get` call
- `internal/executor/plan_generator.go` - Update `RecipeLoader.GetWithContext` call

## Files to Create

None.

## Implementation Steps

- [ ] Add `LoaderOptions` struct to `internal/recipe/loader.go` with `RequireEmbedded bool` field
- [ ] Modify `Get(name string)` to `Get(name string, opts LoaderOptions)` and update implementation
- [ ] Modify `GetWithContext(ctx, name)` to `GetWithContext(ctx, name, opts LoaderOptions)` and update implementation
- [ ] Add embedded-only lookup logic in `GetWithContext` when `RequireEmbedded=true`
- [ ] Create clear error type/message for "recipe not found in embedded FS"
- [ ] Update `RecipeLoader` interface in `internal/actions/resolver.go`
- [ ] Update `RecipeLoader` interface in `internal/verify/deps.go` (if needed, or document why it differs)
- [ ] Update all cmd/* callers to pass `LoaderOptions{}`
- [ ] Update `internal/executor/plan_generator.go` to pass `LoaderOptions{}`
- [ ] Update all tests in `loader_test.go` to pass `LoaderOptions{}`
- [ ] Add tests for `RequireEmbedded=true` behavior (success and failure cases)
- [ ] Run full test suite and verify no regressions

## Testing Strategy

**Unit tests (loader_test.go):**
- Test `Get` with `LoaderOptions{}` returns same behavior as before
- Test `Get` with `RequireEmbedded=true` on an embedded recipe - should succeed
- Test `Get` with `RequireEmbedded=true` on a non-embedded recipe - should fail with clear error
- Test `GetWithContext` with same scenarios
- Test error message contains actionable information

**Regression tests:**
- All existing tests must pass with `LoaderOptions{}` parameter
- Verify in-memory cache still works
- Verify local recipes directory still works when `RequireEmbedded=false`
- Verify registry fallback still works when `RequireEmbedded=false`

**Manual verification:**
- Build tsuku and verify `tsuku install <tool>` still works

## Risks and Mitigations

- **Breaking interface change**: Many callers must be updated. Mitigation: Use compiler to find all call sites; Go will not compile with mismatched signatures.
- **Interface divergence**: Two `RecipeLoader` interfaces exist (actions, verify). Mitigation: Update both, or document why they differ (verify uses `LoadRecipe` not `Get`).
- **Error message clarity**: Users seeing "not in embedded FS" may not understand. Mitigation: Include explanation in error ("This error occurs because --require-embedded is set...").

## Success Criteria

- [ ] `LoaderOptions` struct exists with `RequireEmbedded bool` field
- [ ] `Get` and `GetWithContext` methods accept `LoaderOptions` parameter
- [ ] When `RequireEmbedded=true`, loader only checks embedded FS (no local, no registry)
- [ ] Clear error message returned when recipe not found in embedded FS
- [ ] All existing tests pass with `LoaderOptions{}` for default behavior
- [ ] Existing behavior unchanged when `RequireEmbedded=false`
- [ ] `go test ./...` passes
- [ ] `go build ./cmd/tsuku` succeeds

## Open Questions

None - the implementation context and issue description are clear. The `RecipeLoader` interface in `internal/verify/deps.go` uses a different method name (`LoadRecipe` vs `Get/GetWithContext`) and may not need updating for this issue since it's not used in the dependency resolution path that needs `RequireEmbedded`. This can be addressed in issue #1044 (resolver propagation) if needed.
