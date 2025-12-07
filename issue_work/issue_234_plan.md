# Issue 234 Implementation Plan

## Summary

Create a new file `internal/actions/dependencies.go` with an `ActionDeps` struct and `ActionDependencies` map that declares implicit install-time and runtime dependencies for each action type.

## Approach

Create a separate file for the dependency registry to keep concerns separated. The registry is a simple map from action name to dependency specification. This follows the existing pattern in `action.go` where a registry map is used for action registration.

### Alternatives Considered

- **Embed deps in each action struct**: Would require modifying every action file and changing the Action interface. More invasive, less cohesive.
- **Use constants in each action file**: Spreads the dependency information across many files, making it harder to audit.

## Files to Create

- `internal/actions/dependencies.go` - ActionDeps struct and ActionDependencies map
- `internal/actions/dependencies_test.go` - Unit tests verifying registry contents

## Implementation Steps

- [ ] Create `ActionDeps` struct with InstallTime and Runtime string slices
- [ ] Create `ActionDependencies` map with all action dependencies
- [ ] Add helper function `GetActionDeps(actionName string) ActionDeps`
- [ ] Write unit tests verifying all ecosystem actions have correct deps
- [ ] Write unit tests verifying compiled binary actions have no runtime deps
- [ ] Write unit tests verifying download/extract actions have no deps

## Testing Strategy

- Unit tests: Verify each action has expected dependencies
  - Ecosystem actions (npm_install, pipx_install, gem_install, cpan_install) have both install and runtime
  - Compiled binary actions (go_install, cargo_install, nix_install) have install only
  - Download/extract/utility actions have none
- Table-driven tests for comprehensive coverage

## Risks and Mitigations

- **Risk**: Missing an action in the registry
  - **Mitigation**: Test that all registered actions have an entry in ActionDependencies

## Success Criteria

- [ ] `ActionDeps` struct exists with `InstallTime` and `Runtime` fields
- [ ] All 21 actions have entries in `ActionDependencies`
- [ ] Ecosystem actions: install + runtime deps
- [ ] Compiled binary actions: install only
- [ ] Download/extract actions: no deps
- [ ] All tests pass

## Open Questions

None
