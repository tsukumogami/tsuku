# Issue 686 Implementation Plan

## Summary

Extend install_guide to accept platform tuple keys (os/arch format) with hierarchical fallback (exact tuple → OS key → fallback key), maintaining full backwards compatibility with existing OS-only keys.

## Approach

Chosen approach: Extend existing install_guide field (no new fields). This approach:
- Maintains backwards compatibility (existing OS-only keys continue to work)
- Provides intuitive fallback semantics
- Minimizes API surface (no new schema fields)
- Reuses existing platform computation from Recipe.GetSupportedPlatforms()

### Alternatives Considered

- **Separate arch_install_guide field**: Rejected because it adds unnecessary API complexity, introduces precedence rules, and doesn't align with "extend existing patterns" principle

- **Status quo with documentation**: Rejected because it doesn't solve the architectural inconsistency and forces recipe proliferation

## Files to Modify

- `internal/actions/require_system.go` - Update getPlatformGuide() signature to accept os and arch separately, implement 3-level fallback
- `internal/actions/require_system_test.go` - Add tests for platform tuple lookup with all fallback scenarios
- `internal/recipe/platform.go` - Update ValidateStepsAgainstPlatforms() to validate tuple-aware coverage
- `internal/recipe/platform_test.go` - Add validation tests for tuple coverage patterns

## Files to Create

None (extending existing functionality only)

## Implementation Steps

### Phase 1: Update getPlatformGuide() Function
- [ ] Modify `getPlatformGuide()` signature from `(map[string]string, platform string)` to `(map[string]string, os, arch string)`
- [ ] Implement hierarchical fallback logic: check `os/arch` tuple → check `os` → check `fallback` → return empty
- [ ] Update call site in `require_system.go:Execute()` to pass `runtime.GOOS, runtime.GOARCH`
- [ ] Add unit tests for:
  - Exact tuple match
  - OS fallback when no tuple match
  - Generic fallback when no OS match
  - Mixed keys (tuple + OS)
  - No match (returns empty string)

### Phase 2: Update Validation Logic
- [ ] Modify `ValidateStepsAgainstPlatforms()` to implement tuple-aware validation
- [ ] Add helper to detect tuple format (contains `/` and matches `os/arch` pattern)
- [ ] Iterate over supported platforms from `Recipe.GetSupportedPlatforms()` and validate coverage using fallback logic
- [ ] Validate tuple keys exist in supported platforms
- [ ] Validate OS-only keys exist in supported OS set
- [ ] Add unit tests for:
  - Complete tuple coverage (valid)
  - Complete OS coverage (valid)
  - Mixed tuple and OS coverage (valid)
  - Missing coverage for some platforms (error)
  - Invalid tuple format like `darwin/` (error)
  - Tuple key not in supported platforms (error)

### Phase 3: Documentation
- [ ] Create user-facing documentation explaining platform tuple support
- [ ] Include examples: OS-only keys, tuple keys, mixed pattern
- [ ] Document fallback hierarchy
- [ ] Show validation error examples

### Phase 4: Integration Testing
- [ ] Verify backwards compatibility with existing docker.toml and cuda.toml recipes
- [ ] Create test recipe with platform tuple keys
- [ ] Run validation in strict mode to test edge cases
- [ ] Test on multiple platforms if available (darwin/arm64, darwin/amd64, linux/amd64)

## Testing Strategy

**Unit tests:**
- getPlatformGuide() with all fallback scenarios (5+ test cases)
- ValidateStepsAgainstPlatforms() with all coverage patterns (6+ test cases)
- Edge cases: malformed tuples, empty maps, nil values

**Integration tests:**
- Existing recipes continue to work (backwards compat)
- New recipe with tuple keys validates correctly
- Runtime lookup works on actual platforms

**Manual verification:**
- Run `go test ./...` before and after
- Run `golangci-lint run --timeout=5m ./...`
- Build succeeds: `go build -o tsuku ./cmd/tsuku`

## Risks and Mitigations

- **TOML parsing of slash-containing keys**: Mitigation - Slash is valid in TOML keys when quoted (e.g., `"darwin/arm64" = "..."`). BurntSushi/toml library handles this correctly.

- **Validation complexity**: Mitigation - Validation logic is isolated to ValidateStepsAgainstPlatforms() which already exists. Clear error messages guide recipe authors.

- **Backwards compatibility**: Mitigation - Existing OS-only keys are checked in fallback path. No changes required to existing recipes. Comprehensive tests verify compat.

## Success Criteria

- [ ] All existing tests continue to pass
- [ ] New tests added for tuple support (getPlatformGuide and validation)
- [ ] Backwards compatible: docker.toml and cuda.toml recipes still validate
- [ ] golangci-lint passes with no new warnings
- [ ] Documentation created explaining feature with examples
- [ ] TODO comment at platform.go:319 removed (references issue #686)

## Open Questions

None - design document provides comprehensive specification.

## Reference

Full design document available at: `wip/DESIGN-platform-tuple-support.md`
