# Issue 492 Summary

## What Was Implemented

Extended HomebrewBuilder to handle platform-conditional steps in source builds. The LLM can now analyze Ruby formula `on_macos`, `on_linux`, `on_arm`, and `on_intel` blocks and generate recipe steps with appropriate `when` clauses.

## Changes Made

- `internal/builders/homebrew.go`:
  - Added `platformStep` struct for LLM output
  - Added `PlatformSteps` and `PlatformDependencies` fields to `sourceRecipeData`
  - Added `validPlatformKeys` map for platform validation
  - Updated `validateSourceRecipeData` to validate platform keys and steps
  - Updated `buildSourceSystemPrompt` with platform conditional documentation
  - Updated `buildSourceToolDefs` with `platform_steps` and `platform_dependencies` parameters
  - Updated `buildSourceSteps` to generate steps with `When` clauses
  - Added `platformKeyToWhen` helper function

- `internal/builders/homebrew_test.go`:
  - Added 10 new unit tests for platform conditional validation
  - Added tests for `platformKeyToWhen` conversion
  - Added tests for step generation with `When` clauses
  - Added tests for system prompt and tool definitions

## Key Decisions

- **Use existing `Step.When` mechanism**: Leverages the recipe system's existing platform conditional support rather than creating a new mechanism.
- **Platform key mapping**: Maps Homebrew conventions (macos/linux) to Go runtime values (darwin/linux) in `platformKeyToWhen`.
- **x86_64 alias**: Added x86_64 as an alias for amd64 since formulas use both.

## Trade-offs Accepted

- **No nested conditional support**: Combined conditionals like `on_macos { on_arm { ... } }` are not explicitly handled. The LLM would need to output separate steps. This matches typical formula patterns where nested blocks are rare.
- **Platform dependencies not serialized**: While the struct accepts `PlatformDependencies`, the recipe system doesn't currently support platform-conditional dependencies in metadata. Steps can work around this with conditional actions.

## Test Coverage

- New tests added: 11
- Coverage: All new code paths covered by unit tests

## Known Limitations

- Nested platform conditionals require manual handling by the LLM
- Platform-specific dependencies are validated but not serialized to recipe metadata (recipe type would need enhancement)

## Future Improvements

- Issue #493: Combined conditionals (os + arch) could be supported with combined `When` clauses
- Recipe metadata could be extended to support platform-conditional dependencies
