# Issue 228 Implementation Plan

## Summary
Implement platform-aware recipe support using a complementary hybrid approach with coarse allowlists (`supported_os`, `supported_arch`) and fine-grained denylist (`unsupported_platforms`). Platform validation happens before installation begins with clear error messages showing current vs supported platforms.

## Approach
Following the comprehensive design in `wip/DESIGN-platform-aware-recipes.md`, we'll implement in 3 phases:

1. **Phase 1**: Add schema fields and validation logic to the recipe package
2. **Phase 2**: Integrate with CLI (install and info commands)
3. **Phase 3**: Update existing recipes and documentation

This approach was chosen because:
- Complementary hybrid scales to tsuku's mission of supporting "all tools in the world"
- Preflight validation provides fail-fast UX without wasted downloads
- Simple error messages ship quickly and can be enhanced later
- Backwards compatible (missing fields = universal support)

### Alternatives Considered
- **Pure allowlist**: Doesn't scale when new platforms are added, verbose for "all except X" patterns
- **Pure denylist**: Verbose for OS-only tools (must enumerate all non-Linux platforms)
- **Executor validation**: Fails after work directory created, requires cleanup

## Files to Modify

### Phase 1: Schema and Validation
- `internal/recipe/types.go` - Add three fields to `Metadata` struct: `SupportedOS`, `SupportedArch`, `UnsupportedPlatforms`
- `internal/recipe/recipe.go` or new `internal/recipe/platform.go` - Add `SupportsPlatform()` and `ValidatePlatformConstraints()` methods
- `internal/recipe/recipe_test.go` or new `internal/recipe/platform_test.go` - Add comprehensive unit tests for platform validation

### Phase 2: CLI Integration
- `cmd/tsuku/install.go` - Add preflight check before executor creation
- `cmd/tsuku/info.go` - Display platform constraints in recipe info output
- `internal/recipe/errors.go` or `cmd/tsuku/errors.go` - Define `UnsupportedPlatformError` type

### Phase 3: Recipe Updates
- `internal/recipe/recipes/b/btop.toml` - Add `supported_os = ["linux"]`
- `internal/recipe/recipes/h/hello-nix.toml` - Add `supported_os = ["linux"]`

## Files to Create
- `internal/recipe/platform.go` (optional) - Platform validation logic if splitting from recipe.go
- `internal/recipe/platform_test.go` (optional) - Platform tests if splitting from recipe_test.go

## Implementation Steps

### Phase 1: Schema and Validation
- [ ] Add three optional fields to `Metadata` struct in `internal/recipe/types.go`
- [ ] Implement `allKnownOS()` helper returning all known OS values
- [ ] Implement `allKnownArch()` helper returning all known arch values
- [ ] Implement `Recipe.SupportsPlatform(targetOS, targetArch string) bool` with Cartesian product logic
- [ ] Implement `Recipe.ValidatePlatformConstraints()` for edge case detection
- [ ] Write unit tests for all constraint combinations
- [ ] Write unit tests for edge cases (contradictory constraints, empty result sets)
- [ ] Run `go test ./internal/recipe/...` to verify all tests pass

### Phase 2: CLI Integration
- [ ] Define `UnsupportedPlatformError` struct with formatted output
- [ ] Add preflight check in `install.go` before executor creation
- [ ] Update `info.go` to display platform constraints
- [ ] Add integration test for unsupported platform error
- [ ] Run `go test ./...` to verify all tests pass
- [ ] Manual test: verify error message clarity

### Phase 3: Recipe Updates
- [ ] Update `btop.toml` with `supported_os = ["linux"]`
- [ ] Update `hello-nix.toml` with `supported_os = ["linux"]`
- [ ] Run `go test ./...` to ensure no regressions
- [ ] Manual test: verify btop/hello-nix show platform errors on macOS (if on macOS) or use test recipe

## Testing Strategy

### Unit Tests
- **Validation logic** (`SupportsPlatform`):
  - Missing fields (default to all platforms) ✓
  - Empty arrays (override to empty set) ✓
  - OS-only constraints (`supported_os = ["linux"]`) ✓
  - Arch-only constraints (`supported_arch = ["amd64"]`) ✓
  - Denylist-only (`unsupported_platforms = ["darwin/arm64"]`) ✓
  - Combined allowlist + denylist ✓

- **Edge cases** (`ValidatePlatformConstraints`):
  - Contradictory constraints (should warn in strict mode) ✓
  - Empty result set (should error) ✓
  - No-op exclusions (should warn) ✓

- **Error formatting**:
  - Shows current platform ✓
  - Shows allowed OS and arch ✓
  - Shows denylist when present ✓

### Integration Tests
- Create test recipe with `supported_os = ["linux"]`
- Attempt install on unsupported platform
- Verify error message format and content
- Verify `tsuku info` displays platform constraints correctly

### Manual Verification
- Test `tsuku info btop` shows platform constraints
- Test `tsuku install btop` on macOS shows clear error (if testing on macOS)
- Verify backwards compatibility: existing recipes without constraints work normally

## Risks and Mitigations

**Risk**: Recipe TOML parsing fails with new optional fields
- **Mitigation**: Use `omitempty` tag, test with existing recipes without fields

**Risk**: Edge case validation too strict, breaks valid recipes
- **Mitigation**: Warnings (not errors) for no-op constraints, only error on empty result set

**Risk**: Error messages unclear to users
- **Mitigation**: Include concrete examples in tests, manual verification step

**Risk**: Backwards compatibility broken
- **Mitigation**: Default behavior (missing fields) = universal support, comprehensive testing

**Risk**: Platform detection fails on unusual systems
- **Mitigation**: Use standard Go `runtime.GOOS`/`runtime.GOARCH`, well-tested across platforms

## Success Criteria
- [ ] All unit tests pass for platform validation logic
- [ ] Integration test verifies unsupported platform error behavior
- [ ] `tsuku info` displays platform constraints
- [ ] Install command fails fast with clear error when platform unsupported
- [ ] Backwards compatibility: recipes without constraints work normally
- [ ] btop and hello-nix recipes correctly declare Linux-only support
- [ ] All existing tests continue to pass (no regressions)
- [ ] `go vet ./...` passes
- [ ] `go build -o tsuku ./cmd/tsuku` succeeds

## Open Questions
None - design document is comprehensive and approved.
