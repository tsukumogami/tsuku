# Issue 862 Summary

## What Was Implemented

Added a minimal end-to-end walking skeleton for Homebrew Cask support. This establishes foundational interfaces and creates a working flow from recipe to installed application, enabling parallel development of full implementations in downstream issues (#863, #864, #865).

## Changes Made

- `internal/version/resolver.go`: Added `Metadata map[string]string` field to `VersionInfo` struct for provider-specific data
- `internal/recipe/types.go`: Added `Cask` field to `VersionSection` for specifying Homebrew Cask names
- `internal/version/provider_cask.go`: Created `CaskProvider` with hardcoded iTerm2 metadata
- `internal/version/provider_cask_test.go`: Added unit tests for CaskProvider
- `internal/version/provider_factory.go`: Registered `CaskSourceStrategy` for `source = "cask"`
- `internal/executor/plan_generator.go`: Extended template expansion to populate `version.*` vars from metadata
- `internal/actions/action.go`: Added `AppsDir` field to `ExecutionContext`, registered `AppBundleAction`
- `internal/actions/app_bundle.go`: Created `AppBundleAction` for ZIP-based .app installation
- `internal/actions/app_bundle_test.go`: Added unit tests for AppBundleAction
- `testdata/recipes/iterm2-test.toml`: Created test recipe demonstrating the complete flow

## Key Decisions

- **Metadata map on VersionInfo**: Chosen over embedding `CaskVersionInfo` struct to allow any provider to attach metadata without type changes. This is cleaner than polluting the base type with cask-specific fields.

- **Dotted-path template expansion**: Implemented by adding `version.*` keys to the vars map (e.g., `version.url`, `version.checksum`). This reuses the existing `expandVarsInString()` function without modification.

- **Stub provider approach**: Hardcoded iTerm2 metadata in the provider rather than making API calls. This proves the flow works before investing in real API integration (#863).

## Trade-offs Accepted

- **ZIP-only extraction**: The walking skeleton only supports ZIP archives. DMG support is deferred to issue #864.

- **Single hardcoded cask**: Only iTerm2 is supported in the stub. Real Homebrew Cask API integration comes in issue #863.

- **macOS-only testing**: Integration tests skip on non-macOS platforms. Unit tests for parameter validation run anywhere.

## Test Coverage

- New tests added: 12 (7 for CaskProvider, 5 for AppBundleAction)
- Tests verify:
  - CaskProvider returns expected metadata format
  - CaskSourceStrategy correctly routes recipes
  - AppBundleAction validates required parameters
  - AppBundleAction is registered in action registry

## Known Limitations

- Only iTerm2 is supported in the stub provider
- ZIP archives only (no DMG support)
- No binary symlink creation (deferred to #865)
- AppsDir must be set in ExecutionContext by caller

## Future Improvements

The following are tracked in downstream issues:
- #863: Real Homebrew Cask API integration
- #864: DMG extraction support
- #865: Binary symlink creation from .app bundles
