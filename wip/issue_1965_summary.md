# Issue 1965 Summary

## What Was Implemented

Added dependency library RPATH entries to macOS Mach-O tool binaries during
the `homebrew_relocate` step. When a tool has install-time dependencies with
lib directories, those paths are now baked into the binary's RPATHs so the
dyld loader can find them at runtime.

## Changes Made

- `internal/actions/homebrew_relocate.go`:
  - Changed `fixMachoRpath` signature to accept `*ExecutionContext` (matching `fixElfRpath`)
  - Added dependency lib RPATH iteration after the self-relative RPATH
  - Updated `fixBinaryRpath` call site to pass ctx
  - Removed debug print statements from `fixLibraryDylibRpaths`
- `internal/actions/homebrew_test.go`:
  - Added 3 tests: with dependencies, without dependencies, nil context

## Key Decisions

- Used absolute RPATHs (like `fixLibraryDylibRpaths` already does) rather than DYLD_LIBRARY_PATH wrappers. SIP strips DYLD_LIBRARY_PATH on macOS, making wrappers unreliable.
- Skipped dependencies whose lib directory doesn't exist on disk, avoiding broken RPATHs.
- Protected against nil ctx to avoid panics when the function is called from contexts without dependency info.

## Trade-offs Accepted

- Absolute RPATHs become stale if a dependency is upgraded. This is accepted behavior -- it's the same pattern used by `fixLibraryDylibRpaths` for library dylibs, and upgrading a dependency already requires reinstalling dependent tools.

## Test Coverage

- New tests added: 3
- Tests verify graceful handling on Linux (no install_name_tool) and nil context safety

## Known Limitations

- Linux ELF binaries have the same theoretical gap (no dependency RPATHs), but it doesn't cause issues in practice because LD_LIBRARY_PATH handling is less restrictive. Could be addressed in a follow-up.
- Full macOS verification requires running on macOS hardware (CI covers it via macOS runners).
