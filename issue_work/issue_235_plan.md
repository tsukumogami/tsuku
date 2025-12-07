# Issue 235 Implementation Plan

## Summary

Create a dependency resolver that collects implicit dependencies from action registry and merges with step-level extensions.

## Approach

Add a new file `resolver.go` in the actions package with a `ResolveDependencies` function that takes a recipe and returns collected install-time and runtime dependencies. The resolver uses the ActionDependencies map (from #234) and supports step-level `extra_dependencies` and `extra_runtime_dependencies`.

### Alternatives Considered

- **Add to types.go in recipe package**: Would create import cycle since we need actions.ActionDependencies
- **Create new deps package**: Over-engineered for the current scope; can refactor later if needed

## Files to Create

- `internal/actions/resolver.go` - Dependency resolution algorithm
- `internal/actions/resolver_test.go` - Unit tests for resolver

## Implementation Steps

- [ ] Create `ResolvedDeps` struct with InstallTime and Runtime maps
- [ ] Implement `ResolveDependencies(recipe)` function collecting from action registry
- [ ] Add step-level extra_dependencies handling
- [ ] Add step-level extra_runtime_dependencies handling
- [ ] Write unit tests for basic resolution
- [ ] Write unit tests for step-level extensions

## Testing Strategy

- Unit tests: Table-driven tests with mock recipes
  - Recipe with npm_install action → nodejs in both install and runtime
  - Recipe with go_install action → go in install only
  - Recipe with download action → empty deps
  - Recipe with extra_dependencies on step → merged into install deps
  - Recipe with extra_runtime_dependencies on step → merged into runtime deps

## Risks and Mitigations

- **Risk**: Recipe package import cycle
  - **Mitigation**: Keep resolver in actions package, accept recipe.Recipe as interface

## Success Criteria

- [ ] `ResolveDependencies(recipe)` returns correct install-time deps from actions
- [ ] `ResolveDependencies(recipe)` returns correct runtime deps from actions
- [ ] Step-level extra_dependencies are merged into install deps
- [ ] Step-level extra_runtime_dependencies are merged into runtime deps
- [ ] All tests pass

## Open Questions

None
