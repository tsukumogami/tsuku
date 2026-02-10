# Issue 1573 Implementation Plan

## Summary

Extend `tsuku info` command with `--deps-only`, `--system`, and `--family` flags to extract system package names from recipes and their transitive dependencies. This reuses existing transitive resolution infrastructure and creates a shared extraction library for code reuse with sandbox mode.

## Approach

The implementation follows the design doc's architecture: extend `tsuku info` rather than create a new command because `info` already resolves transitive dependencies via `ResolveTransitive()`. The key insight is that `--deps-only` is symmetrical with existing `--metadata-only` and reuses JSON output infrastructure.

The solution requires platform-aware transitive resolution because dependencies can be declared in platform-filtered steps. We pass the target platform to `ResolveTransitiveForPlatform()` so only dependencies from matching steps are included.

### Alternatives Considered

- **New `tsuku deps` command**: Rejected because it duplicates transitive resolution that `info` already has, and adds CLI surface area. The existing prototype in `deps.go` will be removed in issue #1578 after workflow migrations.

- **Extend existing sandbox/packages.go**: Rejected because that package is specific to container building. A shared library in `internal/executor` serves both consumers better.

## Files to Modify

- `cmd/tsuku/info.go` - Add `--deps-only`, `--system`, `--family` flags; add `--deps-only` output mode; pass target to `ResolveTransitiveForPlatform()`; add system package extraction from dependency tree
- `cmd/tsuku/sysdeps.go` - Refactor `buildTargetFromFlags()` to use `LibcForFamily()` for correct libc derivation (currently uses detect which is wrong for cross-platform queries)

## Files to Create

- `internal/executor/system_deps.go` - Shared extraction library (~100 LOC):
  - `ExtractSystemPackagesFromSteps(steps []recipe.Step) []string` - Extract packages from filtered steps
  - `ExtractSystemPackagesForFamily(r *recipe.Recipe, target platform.Target) []string` - High-level wrapper
  - `IsSystemPackageAction(actionName string) bool` - Check if action is a package install action

- `internal/executor/system_deps_test.go` - Unit tests (~150 LOC):
  - Test extraction from apk_install, apt_install, dnf_install steps
  - Test filtering by target
  - Test deduplication
  - Test empty case

- `cmd/tsuku/info_test.go` - Command tests (~200 LOC):
  - Test `--deps-only` output format (text and JSON)
  - Test `--system` flag extracts packages instead of recipe names
  - Test `--family` flag builds correct target
  - Test mutual exclusivity with `--metadata-only`
  - Test with `--recipe` flag for local files

## Implementation Steps

- [ ] 1. Create `internal/executor/system_deps.go` with package extraction functions
- [ ] 2. Create `internal/executor/system_deps_test.go` with unit tests
- [ ] 3. Fix `buildTargetFromFlags()` in `cmd/tsuku/sysdeps.go` to use `LibcForFamily()` instead of runtime detection
- [ ] 4. Add `--deps-only` and `--system` flags to `cmd/tsuku/info.go` init()
- [ ] 5. Add `--family` flag to `cmd/tsuku/info.go` init() with validation
- [ ] 6. Add mutual exclusivity check between `--deps-only` and `--metadata-only`
- [ ] 7. Add deps-only output mode to runInfo() using existing JSON/text patterns
- [ ] 8. Add system package extraction from transitive dependency tree
- [ ] 9. Create `cmd/tsuku/info_test.go` with integration tests
- [ ] 10. Run full test suite and verify no regressions

## Testing Strategy

- **Unit tests**: Test extraction functions in `internal/executor/system_deps_test.go`:
  - Extract packages from single recipe
  - Filter by target platform
  - Handle multiple package manager actions
  - Deduplicate across steps and recipes

- **Command tests**: Test flag behavior in `cmd/tsuku/info_test.go`:
  - Text output: one package per line
  - JSON output: `{"packages": [...], "family": "..."}`
  - Mutual exclusivity error message
  - Family validation error message
  - Integration with `--recipe` flag

- **Manual verification**:
  - `./tsuku info --deps-only zlib` shows recipe names
  - `./tsuku info --deps-only --system --family alpine zlib` shows Alpine packages
  - `./tsuku info --deps-only --system --family alpine --json zlib` shows JSON output
  - Verify output works with `apk add $(...)` pattern

## Risks and Mitigations

- **Target construction complexity**: The libc must be derived from family, not detected. The existing `deps.go` has a bug where it detects libc from the host system. Fixed in step 3 by using `LibcForFamily()`.

- **Transitive resolution with platform filtering**: The current `ResolveTransitiveForPlatform()` takes targetOS as a string, but we need full Target for step filtering. Review if we need to modify the resolver or if we can filter post-resolution.

- **Existing deps.go prototype**: The prototype command in `deps.go` uses different flag names (`--format` vs `--json`). This is fine because it will be removed in issue #1578. No need to maintain compatibility.

## Success Criteria

- [ ] `tsuku info --deps-only curl` outputs recipe dependency names (one per line)
- [ ] `tsuku info --deps-only --system --family alpine zlib` outputs Alpine package names
- [ ] `tsuku info --deps-only --system --family alpine --json zlib` outputs JSON with `packages` array and `family` field
- [ ] `tsuku info --deps-only --metadata-only curl` returns error about mutual exclusivity
- [ ] `tsuku info --deps-only --system --family bogus curl` returns error about invalid family
- [ ] Command works with `--recipe path/to/recipe.toml` for local recipe files
- [ ] Transitive dependencies are included (test with a recipe that has dependencies)
- [ ] All existing tests pass (`go test ./...`)
- [ ] New tests provide coverage for extraction and command logic

## Open Questions

None. The design doc and IMPLEMENTATION_CONTEXT.md provide clear direction on all aspects.

## Design References

- Design doc: `docs/designs/DESIGN-recipe-driven-ci-testing.md`
- Implementation context: `wip/IMPLEMENTATION_CONTEXT.md`
- Existing patterns: `cmd/tsuku/deps.go` (prototype), `internal/sandbox/packages.go` (extraction)
- Platform types: `internal/platform/target.go`, `internal/platform/family.go`, `internal/platform/libc.go`
