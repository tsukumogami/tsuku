# Issue 862 Implementation Plan

## Summary

Add a minimal end-to-end walking skeleton for Homebrew Cask support by implementing a stub CaskProvider, extending template substitution for dotted-path syntax, adding AppsDir to ExecutionContext, and creating a basic AppBundleAction for ZIP-based .app installation.

## Approach

Follow existing patterns established by HomebrewProvider and other version providers. Create a hardcoded stub that returns iTerm2 metadata, allowing downstream issues (#863, #864, #865) to build on the established interface. The template expansion system will be extended minimally to support `{version.url}` syntax by detecting dotted keys and looking them up in a nested version metadata map.

### Alternatives Considered

1. **Extend VersionInfo struct directly**: Add URL/Checksum fields to the base `VersionInfo` struct. Not chosen because it pollutes the core type for all providers when only Cask providers need these fields.

2. **Create CaskVersionInfo as wrapper around VersionInfo**: Store additional metadata in a separate struct that embeds VersionInfo. Not chosen because Go's composition doesn't provide inheritance semantics, and downstream code would need type assertions.

3. **Store metadata in a map[string]string on VersionInfo**: Add a generic Metadata field to VersionInfo for provider-specific data. **Chosen approach** - cleanest extension point that allows any provider to attach additional metadata without struct changes.

## Files to Modify

- `internal/recipe/types.go` - Add `Cask` field to `VersionSection` struct
- `internal/version/provider_factory.go` - Register `CaskSourceStrategy` for `source = "cask"`
- `internal/actions/action.go` - Add `AppsDir` to `ExecutionContext`, register `AppBundleAction` in `init()`
- `internal/executor/plan_generator.go` - Extend `expandVarsInString()` to support dotted-path keys like `{version.url}`
- `internal/version/resolver.go` - Add Metadata field to `VersionInfo` struct
- `internal/actions/action_test.go` - Add `app_bundle` to registered action tests

## Files to Create

- `internal/version/provider_cask.go` - `CaskProvider` struct with hardcoded iTerm2 metadata
- `internal/version/provider_cask_test.go` - Unit tests for CaskProvider
- `internal/actions/app_bundle.go` - `AppBundleAction` implementation for .app installation
- `internal/actions/app_bundle_test.go` - Unit tests for AppBundleAction
- `internal/executor/plan_generator_cask_test.go` - Tests for dotted-path template expansion
- `recipes/iterm2.toml` - Example recipe for integration testing (macOS only)
- `internal/executor/cask_integration_test.go` - Integration test demonstrating full flow

## Implementation Steps

- [ ] Add `Metadata map[string]string` field to `VersionInfo` struct in `internal/version/resolver.go`
- [ ] Add `Cask` field to `VersionSection` in `internal/recipe/types.go` with TOML tag
- [ ] Create `internal/version/provider_cask.go` with `CaskProvider` struct
- [ ] Implement `CaskProvider.ResolveLatest()` returning hardcoded iTerm2 metadata (version, URL, SHA256)
- [ ] Implement `CaskProvider.ResolveVersion()` with basic version matching
- [ ] Implement `CaskProvider.SourceDescription()` returning `"Cask:iterm2"` format
- [ ] Create `CaskSourceStrategy` in `internal/version/provider_factory.go`
- [ ] Register `CaskSourceStrategy` in `NewProviderFactory()` at `PriorityKnownRegistry` (100)
- [ ] Add `AppsDir string` field to `ExecutionContext` in `internal/actions/action.go`
- [ ] Extend `expandVarsInString()` in `internal/executor/plan_generator.go` to handle dotted keys
- [ ] Update `GeneratePlan()` to populate version metadata vars (e.g., `version.url`, `version.checksum`)
- [ ] Create `internal/actions/app_bundle.go` with `AppBundleAction` struct
- [ ] Implement `AppBundleAction.Execute()` for ZIP extraction and .app copy to AppsDir
- [ ] Register `AppBundleAction` in `init()` function of `internal/actions/action.go`
- [ ] Write unit tests for `CaskProvider` in `internal/version/provider_cask_test.go`
- [ ] Write unit tests for `AppBundleAction` in `internal/actions/app_bundle_test.go`
- [ ] Write unit tests for dotted-path template expansion in `internal/executor/plan_generator_cask_test.go`
- [ ] Create `recipes/iterm2.toml` example recipe using cask version source
- [ ] Create integration test in `internal/executor/cask_integration_test.go` with macOS build tag
- [ ] Run `go vet ./...` and `go test ./...` to verify implementation
- [ ] Run `golangci-lint run --timeout=5m ./...` before commit

## Testing Strategy

### Unit Tests

1. **CaskProvider tests** (`internal/version/provider_cask_test.go`):
   - `TestCaskProvider_ResolveLatest` - Returns hardcoded iTerm2 metadata
   - `TestCaskProvider_ResolveVersion` - Exact version match
   - `TestCaskProvider_SourceDescription` - Returns correct format
   - `TestCaskProvider_Interface` - Implements VersionResolver

2. **AppBundleAction tests** (`internal/actions/app_bundle_test.go`):
   - `TestAppBundleAction_Name` - Returns "app_bundle"
   - `TestAppBundleAction_Execute_ZipExtraction` - Extracts .app from ZIP
   - `TestAppBundleAction_Execute_MissingAppsDir` - Error handling
   - `TestAppBundleAction_Dependencies` - Returns empty (no deps for stub)

3. **Template expansion tests** (`internal/executor/plan_generator_cask_test.go`):
   - `TestExpandVarsInString_DottedPath` - `{version.url}` expansion
   - `TestExpandVarsInString_MixedPaths` - `{version}` and `{version.url}` together

### Integration Tests

1. **Full flow test** (`internal/executor/cask_integration_test.go`):
   - Build tag: `//go:build darwin && integration`
   - Test: Load iterm2.toml recipe, generate plan, verify expanded URLs/checksums
   - Verify: AppsDir is set in execution context

### Manual Verification

1. On macOS: `go build -o tsuku ./cmd/tsuku && ./tsuku install iterm2`
2. Verify iTerm2.app appears in configured apps directory

## Risks and Mitigations

1. **Risk**: Dotted-path expansion could conflict with existing template vars containing dots
   - **Mitigation**: Check that no existing vars use dots in their names (grep confirms none do)

2. **Risk**: AppsDir may need different defaults on macOS vs Linux
   - **Mitigation**: Walking skeleton only targets macOS; Linux handling deferred to future issues

3. **Risk**: ZIP extraction security (path traversal)
   - **Mitigation**: Reuse existing `isPathWithinDirectory()` validation from `extract.go`

4. **Risk**: Hardcoded iTerm2 metadata becomes stale
   - **Mitigation**: This is a stub - full Cask API integration is in issue #863

## Success Criteria

- [ ] `go test ./...` passes with new unit tests
- [ ] `go vet ./...` reports no issues
- [ ] `golangci-lint run --timeout=5m ./...` passes
- [ ] CaskProvider returns hardcoded metadata for iTerm2
- [ ] Template expansion handles `{version.url}` and `{version.checksum}` syntax
- [ ] AppBundleAction extracts ZIP and copies .app to AppsDir
- [ ] Integration test demonstrates full recipe-to-installation flow
- [ ] iterm2.toml recipe validates with existing recipe validation tooling

## Open Questions

None - all design decisions resolved through introspection and codebase analysis.
