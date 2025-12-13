# Issue 491 Implementation Plan

## Summary

Extend HomebrewBuilder to support source-based recipe generation when bottles are unavailable. The LLM will parse Ruby formulas and map build patterns to tsuku actions.

## Approach

Extend the existing HomebrewBuilder with two new LLM tools (`fetch_formula_ruby` and `extract_source_recipe`) that enable the LLM to analyze Ruby formulas and generate source build recipes. The implementation follows the existing tool-call pattern established for bottle-based generation.

### Alternatives Considered

1. **Create a separate SourceBuilder**: Would duplicate infrastructure (LLM loop, validation, etc.). Rejected because the source build is an extension of the same builder, triggered when bottles are unavailable.

2. **Deterministic Ruby parsing without LLM**: Would require a Ruby parser in Go to extract install patterns. Rejected because Ruby formulas contain procedural logic that's hard to parse deterministically (conditionals, method calls, etc.).

## Files to Modify

- `internal/builders/homebrew.go` - Add source build tools, detection logic, and recipe generation
- `internal/builders/homebrew_test.go` - Add unit tests for source build functionality

## Files to Create

- `internal/actions/configure_make.go` - Autotools build action (./configure && make install)
- `internal/actions/configure_make_test.go` - Unit tests
- `internal/actions/cmake_build.go` - CMake build action
- `internal/actions/cmake_build_test.go` - Unit tests

## Implementation Steps

- [x] 1. Create `configure_make` action
  - Parameters: source_dir, configure_args, make_targets, executables
  - Run ./configure with args, then make, then make install
  - Copy executables to install_dir/bin

- [ ] 2. Create `cmake_build` action
  - Parameters: source_dir, cmake_args, executables
  - Create build directory, run cmake, run make
  - Copy executables to install_dir/bin

- [ ] 3. Add `fetch_formula_ruby` tool implementation
  - Fetch raw Ruby formula from `https://raw.githubusercontent.com/Homebrew/homebrew-core/HEAD/Formula/{first_letter}/{formula}.rb`
  - Sanitize content (control chars, max length)
  - Return Ruby source as string to LLM

- [ ] 2. Add `extract_source_recipe` tool definition
  - Parameters: build_system (enum), source_url, build_dependencies, configure_args, executables, verify_command
  - Schema validation for required fields
  - Build system enum: autotools, cmake, cargo, go, make, custom

- [ ] 3. Add source recipe data structures
  - `sourceRecipeData` struct to hold extracted source build info
  - Build system constants and mapping

- [ ] 4. Update Build() to handle source builds
  - When bottles are unavailable, switch to source build mode
  - Use extended tool set (add fetch_formula_ruby, extract_source_recipe)
  - Update system prompt for source build context

- [ ] 5. Implement `generateSourceRecipe` function
  - Map build_system to appropriate tsuku action sequence
  - Extract SHA256 from formula (not LLM-generated)
  - Generate recipe with download, extract, build, and install_binaries steps

- [ ] 6. Add build system action mapping
  - autotools → download + extract + configure_make
  - cmake → download + extract + cmake_build
  - cargo → download + extract + cargo_build
  - go → download + extract + go_build
  - make → download + extract + make

- [ ] 7. Add unit tests for source build detection
  - Test `fetch_formula_ruby` tool
  - Test `extract_source_recipe` validation
  - Test build system detection (mock LLM)
  - Test recipe generation for each build system

- [ ] 8. Add integration test with a source-only formula
  - Find or create a formula without bottles
  - Test end-to-end source recipe generation

## Testing Strategy

### Unit Tests
- `TestHomebrewBuilder_fetchFormulaRuby` - Test Ruby formula fetching
- `TestHomebrewBuilder_extractSourceRecipe_Validation` - Test parameter validation
- `TestHomebrewBuilder_generateSourceRecipe_Autotools` - Test autotools recipe generation
- `TestHomebrewBuilder_generateSourceRecipe_Cmake` - Test cmake recipe generation
- `TestHomebrewBuilder_generateSourceRecipe_Cargo` - Test cargo recipe generation
- `TestHomebrewBuilder_generateSourceRecipe_Go` - Test go recipe generation

### Integration Tests
- Test with a real formula that doesn't have bottles (or use mock server)

## Risks and Mitigations

1. **Ruby DSL complexity**: Some formulas have complex install methods
   - Mitigation: Start with common patterns (autotools, cmake, cargo, go, make)
   - Return error for unsupported patterns rather than generating broken recipes

2. **Source URL extraction**: URLs may have complex interpolation
   - Mitigation: Extract stable URL and SHA256 from formula JSON API (not Ruby parsing)

3. **Platform-specific install logic**: Some formulas have `on_macos`/`on_linux` blocks
   - Mitigation: Issue #492 will handle this; for now, generate basic recipe

## Success Criteria

- [ ] `fetch_formula_ruby` tool fetches raw Ruby formulas
- [ ] `extract_source_recipe` tool validates and extracts source recipe data
- [ ] Build system detection works for: autotools, cmake, cargo, go, make
- [ ] Generated recipes include correct action sequence for each build system
- [ ] SHA256 checksums come from formula JSON (not LLM)
- [ ] Unit tests pass for all build systems
- [ ] Integration test demonstrates end-to-end flow

## Open Questions

None - the design document provides clear guidance on the approach.
