# Issue 1111 Implementation Plan

## Summary

Add a `Dependencies []string` field to the Step struct and modify the dependency resolver to only resolve step-level dependencies when the step's When clause matches the target platform. This prevents "phantom dependencies" where platform-incompatible dependencies appear in installation plans.

## Approach

The implementation extends the existing dependency resolution system with step-level granularity. Currently, `ResolveDependenciesForPlatform` iterates over all steps regardless of whether they match the target. The fix adds a When-clause check before processing each step's dependencies.

Key insight from the codebase: Step-level dependencies are already partially supported through the Params mechanism (`dependencies`, `extra_dependencies` in Params), but the resolver doesn't filter steps by platform match first. This means a step targeting musl Linux can contribute Homebrew dependencies that only make sense on glibc systems.

### Alternatives Considered

1. **Filter in generateDependencyPlans only**: Less invasive but incomplete - the resolver would still aggregate wrong dependencies before plan generation filters them. Rejected because the root cause is in ResolveDependenciesForPlatform.

2. **Add step filtering in ResolveDependencies only, keep Params-based dependencies**: Smaller change but doesn't give recipe authors a dedicated field. The current Params-based approach is implicit and easy to miss. Rejected because the design doc calls for an explicit Dependencies field on Step.

3. **Create separate StepDependency struct**: More complex than needed. The existing pattern of string slices (`[]string`) for dependencies is well-established in MetadataSection. Rejected for unnecessary complexity.

## Files to Modify

- `internal/recipe/types.go` - Add `Dependencies []string` field to Step struct, update UnmarshalTOML to parse it, update ToMap() for serialization
- `internal/actions/resolver.go` - Modify ResolveDependenciesForPlatform to check step.When before processing step dependencies
- `internal/recipe/types_test.go` - Add tests for Step.Dependencies TOML parsing and serialization
- `internal/actions/resolver_test.go` - Add tests for step-level dependency resolution with When filtering

## Files to Create

None - all changes fit within existing files following established patterns.

## Implementation Steps

- [ ] Add `Dependencies []string` field to Step struct in types.go
- [ ] Update Step.UnmarshalTOML to parse `dependencies` from TOML into the new field (currently goes to Params)
- [ ] Update Step.ToMap() to serialize Dependencies field back to TOML
- [ ] Modify ResolveDependenciesForPlatform to accept a Matchable target parameter (currently only takes targetOS string)
- [ ] Add step.When.Matches(target) check before processing step dependencies in the resolver
- [ ] Ensure step deps from the new field are handled the same as Params["dependencies"] (replace semantics)
- [ ] Add unit tests for Step Dependencies field parsing in types_test.go
- [ ] Add unit tests for ToMap() serialization with Dependencies
- [ ] Add unit tests in resolver_test.go for step-level deps only resolved when When matches
- [ ] Add unit tests verifying deps are skipped when step.When doesn't match target
- [ ] Run `go test ./...` to verify all tests pass
- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...` for linting

## Testing Strategy

- **Unit tests (types_test.go)**:
  - Parse step with `dependencies = ["openssl", "zlib"]` - verify Dependencies field populated
  - Parse step without dependencies - verify Dependencies is nil/empty
  - ToMap() roundtrip preserves Dependencies array
  - Dependencies and Params["dependencies"] are mutually exclusive (struct field takes precedence)

- **Unit tests (resolver_test.go)**:
  - Step with Dependencies and matching When - dependencies resolved
  - Step with Dependencies and non-matching When - dependencies skipped
  - Step with Dependencies and no When (nil) - dependencies resolved for all platforms
  - Multiple steps: one matches, one doesn't - only matching step's deps included
  - Precedence: Step.Dependencies replaces action implicit deps (same as current Params behavior)
  - Recipe-level deps + step-level deps - both aggregated (additive), then deduplicated

- **Integration verification**:
  - Create a test recipe with platform-conditional step deps
  - Generate plans for different targets, verify correct deps in each

## Risks and Mitigations

- **Breaking change for existing recipes**: Low risk. The new field is additive - existing recipes using Params["dependencies"] continue to work. The resolver checks both locations with precedence given to the struct field.

- **Signature change to ResolveDependenciesForPlatform**: Medium risk. Currently takes `targetOS string`, needs to accept a Matchable for full When clause evaluation (which includes libc, linux_family, etc.). Mitigation: Create new function `ResolveDependenciesForTarget(r, target)` and have the existing function construct a minimal Matchable from targetOS. This maintains backward compatibility.

- **Performance**: Low risk. Adding one When.Matches() call per step during dependency resolution is negligible overhead.

## Success Criteria

- [ ] `Dependencies []string` field exists on Step struct
- [ ] TOML parsing populates Step.Dependencies from `dependencies = [...]` in step
- [ ] Step.ToMap() serializes Dependencies back to TOML correctly
- [ ] ResolveDependenciesForPlatform (or new ResolveDependenciesForTarget) only includes step deps when step.When matches
- [ ] Step with When that doesn't match target has its Dependencies ignored
- [ ] All existing tests continue to pass
- [ ] New unit tests cover the acceptance criteria from the issue
- [ ] `go vet` and `golangci-lint` pass with no new warnings

## Open Questions

None - the design doc and IMPLEMENTATION_CONTEXT.md provide sufficient detail for implementation.
