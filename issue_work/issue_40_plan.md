# Issue 40 Implementation Plan

## Summary

Implement local recipe support in the Loader and create the first recipe builder (Cargo) for generating recipes from crates.io metadata. This enables `tsuku create <tool> --from crates.io` to generate recipes that `tsuku install` can then use.

## Approach

Follow the design in `docs/DESIGN-recipe-builders.md`:
1. Modify Loader to check `~/.tsuku/recipes/` before registry
2. Create Builder interface and registry infrastructure
3. Implement CargoBuilder that queries crates.io API and parses Cargo.toml for executables
4. Add `tsuku create` command to invoke builders

### Alternatives Considered
- **Automatic builder invocation**: Rejected per design - builders should be explicit via `tsuku create` for transparency
- **In-memory only recipes**: Rejected - writing to disk enables inspection and sharing

## Files to Create
- `internal/builders/builder.go` - Builder interface and BuildResult types
- `internal/builders/registry.go` - Builder registry
- `internal/builders/cargo.go` - CargoBuilder implementation
- `internal/builders/cargo_test.go` - CargoBuilder unit tests
- `cmd/tsuku/create.go` - `tsuku create` command

## Files to Modify
- `internal/recipe/loader.go` - Add local recipe lookup before registry
- `cmd/tsuku/main.go` - Register create command
- `cmd/tsuku/helpers.go` - Add `loader` global variable (if needed)

## Implementation Steps
- [x] 1. Add local recipe lookup to Loader.GetWithContext()
- [x] 2. Add warning when local recipe shadows registry recipe
- [x] 3. Create Builder interface and BuildResult types
- [x] 4. Create Builder registry
- [x] 5. Implement CargoBuilder with crates.io API integration
- [x] 6. Add Cargo.toml parsing for executable discovery
- [x] 7. Add fallback to crate name when Cargo.toml unavailable
- [x] 8. Create `tsuku create` command
- [x] 9. Add unit tests for CargoBuilder with mocked API responses
- [x] 10. Run full test suite and verify end-to-end flow

## Testing Strategy
- **Unit tests**:
  - CargoBuilder with mocked crates.io API responses
  - Cargo.toml parsing for `[[bin]]` sections
  - Fallback behavior when repository unavailable
  - Loader local recipe priority
- **Integration test**:
  - `tsuku create ripgrep --from crates.io` generates valid recipe
  - `tsuku install ripgrep` uses the local recipe

## Risks and Mitigations
- **crates.io API rate limiting**: Use same HTTP client with proper User-Agent
- **Repository URL formats vary**: Support GitHub initially, fallback for others
- **Cargo.toml parsing edge cases**: Conservative parsing, fallback to crate name

## Success Criteria
- [x] `Loader.Get()` checks `~/.tsuku/recipes/{name}.toml` before registry
- [x] Warning displayed when local recipe shadows registry recipe
- [x] Builder interface defined with `Name()`, `CanBuild()`, `Build()` methods
- [x] `tsuku create ripgrep --from crates_io` generates valid recipe
- [x] Generated recipe written to `~/.tsuku/recipes/ripgrep.toml`
- [x] `tsuku install ripgrep` executes the generated local recipe
- [x] Generated recipe is version-agnostic (uses `source = "crates_io"`)
- [x] Unit tests with mocked crates.io API responses

## Open Questions
None - design document is comprehensive.
