# Issue 580 Summary

## What Was Implemented
Renamed the `homebrew_bottle` action to `homebrew` since bottles are the only Homebrew integration option, making the "_bottle" qualifier unnecessary.

## Changes Made
- `internal/actions/homebrew_bottle.go` -> `internal/actions/homebrew.go`: Renamed file, struct (`HomebrewBottleAction` -> `HomebrewAction`), and action name
- `internal/actions/homebrew_bottle_test.go` -> `internal/actions/homebrew_test.go`: Renamed file and updated test references
- `internal/actions/action.go`: Updated action registration
- `internal/recipe/validator.go`: Updated known actions list and validation messages
- `internal/recipe/types.go`: Updated comments referencing homebrew recipes
- `internal/executor/plan_generator.go`: Updated download action handling
- `cmd/tsuku/create.go`: Updated action references in URL extraction and step descriptions
- `internal/builders/homebrew.go`: Updated builder to generate recipes with `homebrew` action
- 8 recipe files: Updated to use `action = "homebrew"`
- 10 test files: Updated references throughout
- 2 CI workflow files: Updated action name in comments and test recipes
- `docs/DESIGN-homebrew-cleanup.md`: Updated status to reflect completed issues

## Key Decisions
- **Direct rename without deprecation**: Since tsuku is pre-GA with no external users, a clean rename is preferred over maintaining backwards compatibility aliases

## Trade-offs Accepted
- **Design docs keep historical references**: Design documents explain the rationale for the rename and reference the old name in context - this is intentional for documentation clarity

## Test Coverage
- New tests added: 0 (existing tests renamed and updated)
- All existing tests pass

## Known Limitations
- None

## Future Improvements
- None needed for this refactoring
