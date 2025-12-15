# Issue 590 Implementation Summary

## Summary
Implemented deterministic bottle inspection in the HomebrewBuilder, allowing recipe generation without LLM for most Homebrew formulas.

## Changes Made

### 1. Bottle Inspection Functions (internal/builders/homebrew.go)
- Added `listBottleBinaries(ctx, formula, version, platformTag)` - Downloads and inspects bottle to get binary names
- Added `getBlobSHAFromManifest(manifest, version, platformTag)` - Extracts blob SHA from GHCR manifest
- Added `downloadBottleBlob(ctx, formula, blobSHA, token, destPath)` - Downloads bottle with SHA256 verification
- Added `extractBottleBinaries(tarballPath)` - Parses tar.gz to find binaries in bin/ directory
- Added `getCurrentPlatformTag()` - Returns platform tag for current runtime
- Updated `inspectBottle()` to actually download and inspect bottle contents (with fallback if no version info)

### 2. Deterministic Recipe Generation (internal/builders/homebrew.go)
- Added `generateDeterministicRecipe(ctx, packageName, genCtx)` - Generates recipe without LLM:
  - Gets binaries from bottle inspection
  - Generates verify command as `<binary> --version`
  - Uses dependencies from formula JSON

### 3. Updated Generate Flow (internal/builders/homebrew.go)
- Modified `Generate()` to try deterministic generation first for bottle mode
- Added `generateDeterministic()` method to HomebrewSession
- Added `usedDeterministic` flag to track generation method
- Updated `Repair()` to use LLM from scratch if deterministic recipe failed validation

### 4. Tests Added (internal/builders/homebrew_test.go)
- `TestHomebrewBuilder_getBlobSHAFromManifest` - Tests manifest parsing
- `TestHomebrewBuilder_extractBottleBinaries` - Tests tarball binary extraction
- `TestHomebrewBuilder_generateDeterministicRecipe` - Tests error handling
- `TestHomebrewBuilder_getCurrentPlatformTag` - Tests platform tag detection

## Flow Change

### Before
```
formula JSON -> LLM guesses -> validate -> LLM repairs (if fail) -> recipe
```

### After
```
formula JSON -> inspect bottle -> deterministic recipe -> validate
    |                                                       |
    v (if inspection fails)                                 v (if validation fails)
LLM guesses -> validate -> LLM repairs (if fail) -> recipe
```

## Key Design Decisions

1. **Reused existing GHCR infrastructure**: Used `getGHCRToken` and `fetchGHCRManifest` from the builder, added new `getBlobSHAFromManifest` and `downloadBottleBlob` methods.

2. **Graceful fallback**: If bottle inspection fails for any reason (network, missing binaries, etc.), the system falls back to LLM-based generation.

3. **Verify command pattern**: Uses `<binary> --version` as default - works for most CLI tools.

4. **Deterministic flag tracking**: The session tracks whether deterministic generation was used, so `Repair()` can decide whether to repair an LLM recipe or generate from scratch.

## Files Modified
- `internal/builders/homebrew.go` - Core implementation (~250 lines added)
- `internal/builders/homebrew_test.go` - Tests (~200 lines added)

## Testing
- All existing Homebrew tests pass
- New unit tests for bottle inspection and deterministic generation
- Integration testing will be done in CI
