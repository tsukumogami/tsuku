# Issue 880 Summary

## What Was Implemented

Fixed all tool resolve functions (`ResolvePythonStandalone`, `ResolvePipx`, `ResolveCargo`, etc.) to respect the `$TSUKU_HOME` environment variable instead of hardcoding `~/.tsuku/tools/`.

## Root Cause

The `ResolvePythonStandalone()` function (and all similar functions) hardcoded the tools directory path to `~/.tsuku/tools/`. When `$TSUKU_HOME` was set to a custom location (as CI does with per-test directories), the function couldn't find freshly installed eval-time dependencies.

## Changes Made

- `internal/actions/util.go`:
  - Added `GetToolsDir()` helper that checks `$TSUKU_HOME` first
  - Updated all `Resolve*` functions to use `GetToolsDir()`

- `internal/actions/eval_deps.go`:
  - Removed duplicate `getToolsDir()` function
  - Updated `CheckEvalDeps()` to use the shared `GetToolsDir()`

- `.github/workflows/build-essentials.yml`:
  - Re-enabled `libsixel-source` test on macOS Apple Silicon

## Key Decisions

- **Centralize tools directory resolution**: Rather than duplicating the `$TSUKU_HOME` check in multiple places, created a single `GetToolsDir()` function
- **Fix all resolve functions**: Updated all 9 resolve functions for consistency, not just the ones causing the immediate issue

## Test Coverage

- All existing resolve function tests pass
- Manual verification with custom `$TSUKU_HOME` confirms the fix works
- CI will verify the full integration

## Known Limitations

None
