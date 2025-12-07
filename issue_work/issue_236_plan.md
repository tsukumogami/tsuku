# Issue 236 Implementation Plan

## Summary

Extend the dependency resolver to support step-level and recipe-level mechanisms for replacing or extending implicit dependencies.

## Approach

Update the existing `ResolveDependencies` function to implement the full algorithm from the design doc. Add new fields to `MetadataSection` for recipe-level extension.

### Alternatives Considered

- **Separate replace/extend functions**: More modular but adds complexity for a straightforward algorithm
- **New types for override specs**: Over-engineered; string slices are sufficient

## Files to Modify

- `internal/recipe/types.go` - Add ExtraDependencies and ExtraRuntimeDependencies to MetadataSection
- `internal/actions/resolver.go` - Implement full resolution algorithm with replace/extend logic
- `internal/actions/resolver_test.go` - Add tests for override/extension cases

## Implementation Steps

- [x] Add ExtraDependencies and ExtraRuntimeDependencies fields to MetadataSection
- [x] Update resolver: step-level dependencies replaces install deps
- [x] Update resolver: step-level runtime_dependencies replaces runtime deps
- [x] Update resolver: recipe-level Dependencies replaces all install deps
- [x] Update resolver: recipe-level RuntimeDependencies replaces all runtime deps
- [x] Update resolver: recipe-level ExtraDependencies adds to install deps
- [x] Update resolver: recipe-level ExtraRuntimeDependencies adds to runtime deps
- [x] Write tests for step-level replace behavior
- [x] Write tests for recipe-level replace behavior
- [x] Write tests for recipe-level extend behavior

## Testing Strategy

- Unit tests covering:
  - Step with runtime_dependencies=[] → empty runtime deps for that step
  - Step with dependencies=["custom"] → only custom in install deps
  - Recipe with Dependencies set → replaces all implicit
  - Recipe with RuntimeDependencies set → replaces all implicit
  - Recipe with ExtraDependencies → adds to resolved
  - Recipe with ExtraRuntimeDependencies → adds to resolved
  - Combined: recipe replace + extend

## Risks and Mitigations

- **Risk**: Precedence confusion
  - **Mitigation**: Clear algorithm in design doc; tests verify precedence

## Success Criteria

- [x] Step-level runtime_dependencies replaces action implicit
- [x] Step-level dependencies replaces action implicit install deps
- [x] Recipe-level Dependencies replaces all install deps
- [x] Recipe-level RuntimeDependencies replaces all runtime deps
- [x] Recipe-level ExtraDependencies adds without replacing
- [x] Recipe-level ExtraRuntimeDependencies adds without replacing
- [x] All tests pass

## Open Questions

None
