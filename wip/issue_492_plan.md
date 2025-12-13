# Issue 492 Implementation Plan

## Summary

Extend the source build LLM prompts and tool definitions to handle Homebrew formula platform conditionals (`on_macos`, `on_linux`, `on_arm`, `on_intel`) by generating recipe steps with `when` clauses.

## Approach

The recipe system already supports platform conditionals via `Step.When` with `os` (darwin/linux) and `arch` (amd64/arm64). The implementation focuses on:

1. **LLM prompt enhancement**: Update the source build system prompt to instruct the LLM to identify platform-specific blocks in Ruby formulas
2. **Tool schema update**: Extend `extract_source_recipe` to accept platform-conditional steps and dependencies
3. **Recipe generation**: Generate steps with appropriate `when` clauses based on LLM output

### Alternatives Considered

1. **Deterministic Ruby parsing**: Would require a Ruby parser in Go. Rejected because Ruby formulas have procedural logic (method calls, conditionals) that's complex to parse deterministically.

2. **Separate platform-specific recipe files**: Would require multiple recipe files per tool. Rejected because the existing `when` clause approach is simpler and already supported.

## Files to Modify

- `internal/builders/homebrew.go` - Update source build prompts, tool schema, and recipe generation
- `internal/builders/homebrew_test.go` - Add unit tests for platform conditional handling

## Implementation Steps

- [x] 1. Add platform-conditional step structure to `sourceRecipeData`
  - Add `PlatformSteps` field: `map[string][]platformStep` (keyed by "macos", "linux", "arm64", "amd64")
  - Each `platformStep` contains action and params
  - Add `PlatformDependencies` field: `map[string][]string` for platform-specific deps
  - Add `validPlatformKeys` map for validation

- [x] 2. Update `buildSourceSystemPrompt` to instruct LLM about platform conditionals
  - Document `on_macos do ... end`, `on_linux do ... end` patterns
  - Document `on_arm do ... end`, `on_intel do ... end` patterns
  - Explain how to map these to `platform_steps` in the tool output

- [x] 3. Update `buildSourceToolDefs` to add platform conditional parameters
  - Add `platform_steps` object parameter with platform keys
  - Add `platform_dependencies` object parameter

- [x] 4. Update `validateSourceRecipeData` for new fields
  - Validate platform keys are valid (macos, linux, arm64, amd64, x86_64)
  - Validate step structures within platform blocks

- [x] 5. Update `buildSourceSteps` to generate platform-conditional steps
  - Base steps remain unconditional
  - Platform-specific steps get `When` clause: `os: darwin/linux` or `arch: arm64/amd64`
  - Add `platformKeyToWhen` helper function

- [x] 6. Add unit tests for platform conditional handling
  - Test validation with valid/invalid platform steps
  - Test validation with valid/invalid platform dependencies
  - Test `platformKeyToWhen` conversion
  - Test recipe step generation with `when` clauses
  - Test system prompt includes platform documentation
  - Test tool definitions include platform parameters

## Testing Strategy

### Unit Tests
- `TestValidateSourceRecipeData_PlatformSteps` - Validate platform step structure
- `TestHomebrewBuilder_buildSourceSteps_PlatformConditionals` - Test step generation with `when`
- `TestHomebrewBuilder_buildSourceSteps_NestedConditionals` - Test combined os+arch
- `TestHomebrewBuilder_buildSourceSystemPrompt_PlatformDocs` - Verify prompt includes platform docs

### Manual Verification
- Generate recipe for a formula with platform conditionals
- Verify generated TOML has correct `when` clauses
- Test recipe execution on both Linux and macOS (if available)

## Risks and Mitigations

1. **LLM may misidentify conditionals**: Mitigate with clear prompt examples and validation
2. **Nested conditionals are complex**: Start with simple os/arch, log warning for deeply nested
3. **Platform dependency handling**: May require recipe type enhancement - document limitation if so

## Success Criteria

- [x] LLM prompt documents platform conditional patterns
- [x] `extract_source_recipe` tool accepts `platform_steps` parameter
- [x] Generated steps include `when` clauses for platform-specific code
- [x] Unit tests cover all platform conditional scenarios
- [x] `platformKeyToWhen` helper maps platform keys to recipe `When` clauses

## Open Questions

None - the existing `Step.When` mechanism provides the infrastructure needed.
