# Issue 580 Implementation Plan

## Summary
Rename the `homebrew_bottle` action to `homebrew` since bottles are the only Homebrew integration option - a straightforward search-and-replace refactoring.

## Approach
This is a mechanical rename with no behavioral changes. The approach is a systematic search-and-replace across all files, followed by renaming the source file.

### Alternatives Considered
- **Gradual deprecation with aliases**: Not chosen because there are no external consumers - this is internal code
- **Keep the name**: The issue explicitly requests the rename and the design doc justifies it

## Files to Modify

### Source Code
- `internal/actions/homebrew_bottle.go` - rename file to `homebrew.go`, rename struct and methods
- `internal/actions/homebrew_bottle_test.go` - rename file to `homebrew_test.go`
- `internal/actions/action.go` - update registration reference
- `internal/recipe/validator.go` - update known actions list
- `internal/recipe/types.go` - update download actions list
- `internal/executor/plan_generator.go` - update action handling
- `cmd/tsuku/create.go` - update action references

### Test Files
- `internal/actions/composites_test.go` - update test references
- `internal/actions/decomposable_test.go` - update test references
- `internal/actions/dependencies_test.go` - update test references
- `internal/actions/action_test.go` - update test references
- `internal/recipe/validator_test.go` - update test references
- `internal/recipe/types_test.go` - update test references
- `internal/executor/plan_generator_test.go` - update test references
- `internal/executor/plan_test.go` - update test references
- `cmd/tsuku/create_test.go` - update test references
- `internal/builders/homebrew_test.go` - update test references

### Recipes
- `internal/recipe/recipes/l/libyaml.toml` - update action name
- `internal/recipe/recipes/z/zlib.toml` - update action name
- `internal/recipe/recipes/l/libpng.toml` - update action name
- `internal/recipe/recipes/p/pngcrush.toml` - update action name
- `internal/recipe/recipes/r/readline.toml` - update action name
- `internal/recipe/recipes/m/make.toml` - update action name
- `internal/recipe/recipes/g/gdbm.toml` - update action name
- `internal/recipe/recipes/b/bash.toml` - update action name

### Documentation
- `docs/DESIGN-homebrew-cleanup.md` - mark issue #580 complete
- `docs/DESIGN-dependency-provisioning.md` - update action name references
- `docs/DESIGN-relocatable-library-deps.md` - update action name references

### CI
- `.github/workflows/build-essentials.yml` - update action name if present
- `.github/workflows/homebrew-builder-tests.yml` - update action name if present

## Implementation Steps
- [ ] Rename `homebrew_bottle.go` to `homebrew.go` and update struct/method names
- [ ] Rename `homebrew_bottle_test.go` to `homebrew_test.go` and update references
- [ ] Update action registration in `action.go`
- [ ] Update all recipe TOML files to use `action = "homebrew"`
- [ ] Update validator, types, and plan_generator
- [ ] Update cmd/tsuku/create.go
- [ ] Update all test files
- [ ] Update documentation (design docs)
- [ ] Update CI workflows
- [ ] Build and test

## Testing Strategy
- Unit tests: Run `go test ./...` to verify all tests pass
- Manual verification: Build and run `./tsuku validate internal/recipe/recipes/l/libyaml.toml`

## Risks and Mitigations
- **Risk**: Missing a reference somewhere
  - **Mitigation**: Comprehensive grep search, build and test verification

## Success Criteria
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (excluding pre-existing LLM failures)
- [ ] All recipes validate successfully
- [ ] No references to `homebrew_bottle` remain in code (except documentation history)

## Open Questions
None - this is a straightforward rename.
