# Issue 961 Summary

## What Was Implemented

Fixed pip constraint pinning to preserve SHA256 hashes during constrained evaluation. The fix stores the full `locked_requirements` string instead of only extracting package versions, following the pattern used by other ecosystems (Go, Cargo, npm, gem, CPAN).

## Changes Made

- `internal/actions/decomposable.go`: Added `PipRequirements string` field to `EvalConstraints` struct to store the complete locked_requirements string with hashes.

- `internal/executor/constraints.go`: Modified `extractPipConstraintsFromSteps` to store the full requirements string (first-wins semantics). Added `HasPipRequirementsConstraint` helper function.

- `internal/actions/pipx_install.go`: Updated `decomposeWithConstraints` to use stored `PipRequirements` directly instead of reconstructing from version map. Removed unused functions: `generateLockedRequirementsFromConstraints`, `sortPackageNames`, `detectNativeAddonsFromConstraints`.

- `internal/executor/constraints_test.go`: Added test for hash preservation in `TestExtractConstraints_PipExec`. Added `TestHasPipRequirementsConstraint` test. Added helper functions `containsHash` and `containsSubstring`.

## Key Decisions

- **Store full string vs. parse hashes**: Chose to store the full `locked_requirements` string rather than parsing and storing hashes separately. This matches the pattern used by all other ecosystems and preserves all data (hashes, ordering, comments) without custom parsing logic.

- **Keep `PipConstraints` map**: Retained the existing `map[string]string` for version lookups since `GetPipConstraint` is still used elsewhere. The `PipRequirements` string is the source of truth for constrained evaluation.

## Trade-offs Accepted

- **Duplicate storage**: Both `PipConstraints` (version map) and `PipRequirements` (full string) are stored. Acceptable because it maintains backward compatibility and the string size is minimal.

## Test Coverage

- New tests added: 2 (TestHasPipRequirementsConstraint, enhanced TestExtractConstraints_PipExec)
- Modified tests: 1 (added hash verification to existing test)

## Known Limitations

- None identified for this fix.

## Future Improvements

- Consider removing `PipConstraints` map if version lookups are no longer needed elsewhere.
