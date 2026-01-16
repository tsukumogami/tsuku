# Issue 866 Implementation Plan

## Summary

Implement `CaskBuilder` that auto-generates recipes from Homebrew Cask API metadata, following the existing `SessionBuilder` interface pattern. The builder will query the cask API, parse app and binary artifacts, and generate TOML recipes using the `cask` version provider and `app_bundle` action.

## Approach

Create a new deterministic builder that implements `SessionBuilder`, similar to ecosystem builders like `CargoBuilder`. Since cask metadata is deterministic (no LLM needed), we use `DeterministicSession` wrapper. The builder queries the Homebrew Cask JSON API which includes an `artifacts` array with heterogeneous object types for `app` and `binary` artifacts.

### Alternatives Considered

- **Extend HomebrewBuilder with cask mode**: Rejected because HomebrewBuilder is complex with LLM fallback logic, and casks have fundamentally different installation patterns (app_bundle vs homebrew action). Separate builder is cleaner.
- **Reuse CaskProvider for all metadata**: Rejected because CaskProvider doesn't expose artifacts data, only version/URL/checksum. CaskBuilder needs to parse artifacts separately.

## Files to Modify

- `cmd/tsuku/create.go` - Register CaskBuilder in the builder registry (line ~203, after HomebrewBuilder)

## Files to Create

- `internal/builders/cask.go` - CaskBuilder implementation
- `internal/builders/cask_test.go` - Unit tests with mocked API responses

## Implementation Steps

- [ ] **Step 1**: Create `cask.go` with struct definitions and interface methods
  - Define `CaskBuilder` struct with `httpClient` and `homebrewAPIURL` fields
  - Implement `Name()` returning "cask"
  - Implement `RequiresLLM()` returning `false`

- [ ] **Step 2**: Implement `CanBuild` method
  - Parse `SourceArg` to extract cask name (e.g., "visual-studio-code" from "cask:visual-studio-code")
  - Validate cask name using existing `isValidCaskName` pattern from version provider
  - Query cask API and check for supported artifacts (app or binary)
  - Return false for casks with `pkg` or unsupported artifact types

- [ ] **Step 3**: Implement cask API response parsing
  - Define `caskAPIResponse` struct with fields: `token`, `version`, `sha256`, `url`, `name[]`, `desc`, `homepage`, `artifacts`
  - Handle heterogeneous `artifacts` array: each element is an object with one key (`app`, `binary`, `pkg`, etc.)
  - Implement `extractArtifacts()` to parse app name and binary paths from artifacts

- [ ] **Step 4**: Implement recipe generation
  - Build `recipe.Recipe` with:
    - `[version] source = "cask"` and `cask = "<name>"`
    - `[[steps]] action = "app_bundle"` with url/checksum templates and app_name
    - Populate `binaries` array if binary artifacts exist
    - `[verify] command` based on first binary or app name
  - Handle `{{appdir}}` placeholder normalization in binary paths

- [ ] **Step 5**: Implement `NewSession` method
  - Validate cask name and check for supported artifacts
  - Return `DeterministicSession` wrapping the Build function
  - Report progress via `SessionOptions.ProgressReporter`

- [ ] **Step 6**: Add error types for unsupported cask patterns
  - Create `CaskUnsupportedArtifactError` for pkg, preflight, postflight
  - Create `CaskNotFoundError` for 404 responses

- [ ] **Step 7**: Register builder in `cmd/tsuku/create.go`
  - Add `builderRegistry.Register(builders.NewCaskBuilder(nil))` after HomebrewBuilder registration

- [ ] **Step 8**: Write unit tests in `cask_test.go`
  - Test `CanBuild` with valid cask (app artifact), invalid cask (pkg artifact), nonexistent cask
  - Test `extractArtifacts` with app-only, app+binary, and binary-only casks
  - Test recipe generation with proper TOML structure
  - Mock HTTP responses using `httptest.Server`

- [ ] **Step 9**: Add integration test case
  - Test full flow: `NewSession` -> `Generate` -> verify recipe structure
  - Use mock server with realistic cask API response

## Testing Strategy

### Unit Tests
- `TestCaskBuilder_Name` - returns "cask"
- `TestCaskBuilder_RequiresLLM` - returns false
- `TestCaskBuilder_CanBuild_ValidCask` - app artifact returns true
- `TestCaskBuilder_CanBuild_PkgCask` - pkg artifact returns false with clear error
- `TestCaskBuilder_CanBuild_NotFound` - 404 returns false
- `TestCaskBuilder_ExtractArtifacts_AppOnly` - parses app_name correctly
- `TestCaskBuilder_ExtractArtifacts_AppWithBinary` - parses app_name and binaries
- `TestCaskBuilder_ExtractArtifacts_BinaryOnly` - rare case handling
- `TestCaskBuilder_GenerateRecipe` - verify TOML structure and templates
- `TestCaskBuilder_NormalizeBinaryPath` - {{appdir}} replacement

### Integration Tests
- Test with mock server returning VS Code-like response (app + binary)
- Test with mock server returning Firefox-like response (app only)

### Manual Verification
```bash
# Build tsuku
go build -o tsuku ./cmd/tsuku

# Test cask recipe generation
./tsuku create --from cask:iterm2 iterm2
cat ~/.tsuku/recipes/iterm2.toml

# Test with binary cask
./tsuku create --from cask:visual-studio-code vscode
cat ~/.tsuku/recipes/vscode.toml
```

## Risks and Mitigations

- **Heterogeneous JSON artifacts parsing**: The cask API returns artifacts as an array of single-key objects (e.g., `[{"app": ["Name.app"]}, {"binary": ["path"]}]`). Mitigation: Parse as `[]map[string]interface{}` and extract by key.

- **Missing app artifact**: Some casks only have binary or pkg artifacts. Mitigation: Check for app artifact in `CanBuild`; return error for pkg-only casks.

- **Binary path normalization**: Binary paths use `{{appdir}}` placeholder. Mitigation: Strip prefix to get relative path within .app bundle.

## Success Criteria

- [ ] `CaskBuilder` implements `SessionBuilder` interface
- [ ] `tsuku create --from cask:<name>` generates valid recipe
- [ ] App artifacts detected and `app_name` populated correctly
- [ ] Binary artifacts detected and `binaries` array populated
- [ ] Casks with unsupported artifacts (pkg, preflight) return clear error
- [ ] Unit tests cover artifact detection edge cases
- [ ] E2E flow still works (existing tests pass)

## Open Questions

None - all design decisions are documented in the issue and IMPLEMENTATION_CONTEXT.md.
