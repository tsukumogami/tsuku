# Issue 866 Summary

## What Was Implemented

Added `CaskBuilder` that auto-generates recipes from Homebrew Cask metadata. The builder queries the Homebrew Cask API, parses artifact information to detect app bundles and CLI binaries, and generates TOML recipes using the `cask` version provider and `app_bundle` action.

## Changes Made

- `internal/builders/cask.go`: New file - CaskBuilder implementation
  - Implements `SessionBuilder` interface with deterministic generation
  - Queries Homebrew Cask JSON API for metadata and artifacts
  - Parses heterogeneous `artifacts` array to extract app and binary information
  - Normalizes binary paths by removing `{{appdir}}` placeholder
  - Generates recipes with `cask` version provider and `app_bundle` action
  - Returns clear errors for unsupported artifact types (pkg, preflight, postflight)

- `internal/builders/cask_test.go`: New file - Unit tests with mocked API responses
  - 17 tests covering all functionality
  - Tests for CanBuild, artifact extraction, recipe generation, path normalization

- `cmd/tsuku/create.go`: Builder registration and help text
  - Register CaskBuilder in builder registry
  - Add `cask:name` to supported sources list
  - Add examples for cask recipe generation

## Key Decisions

- **Separate builder from HomebrewBuilder**: The cask installation pattern (app_bundle action) is fundamentally different from formula bottles (homebrew action), making a separate builder cleaner than extending HomebrewBuilder.

- **Parse artifacts in builder, not version provider**: The CaskProvider only returns version/URL/checksum. Artifact parsing is specific to recipe generation, so it belongs in CaskBuilder.

- **Binary path normalization**: Homebrew uses `{{appdir}}/App.app/Contents/...` format. We normalize to `Contents/...` (relative to app bundle) which matches what the app_bundle action expects.

## Trade-offs Accepted

- **Duplicate cask name validation**: Both CaskBuilder and CaskProvider have `isValidCaskName` functions. Accepted because they serve different packages and keeping them separate avoids coupling.

- **No checksum warning only**: For casks using `:no_check`, we emit a warning rather than failing. Users can proceed with `--allow-no-checksum` flag.

## Test Coverage

- New tests added: 17
- All tests pass (24 packages in short mode)
- Tests use `httptest.Server` for mocked API responses

## Known Limitations

- Only supports `app` and `binary` artifacts. Casks with `pkg`, `preflight`, or `postflight` artifacts are rejected with clear error messages.
- Architecture selection relies on runtime.GOARCH; no cross-platform recipe generation.

## Future Improvements

- Could share artifact types with a cask API client package if more cask-related functionality is added.
- Could add support for `suite` artifact type (multiple apps in one archive) if needed.
