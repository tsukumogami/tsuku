# Issue 543 Summary

## What Was Implemented

Created three validation scripts for build essentials that verify binary relocatability, tool functionality, and dependency isolation across Linux and macOS platforms.

## Changes Made

- `scripts/verify-relocation.sh`: Checks RPATH (Linux) and install_name (macOS) for hardcoded system paths
- `scripts/verify-tool.sh`: Runs tool-specific functional tests (make, gdbm, zig, pngcrush, zlib, libpng)
- `scripts/verify-no-system-deps.sh`: Verifies binaries only depend on tsuku-provided libs or system libc
- `.github/workflows/build-essentials.yml`: Updated to run validation scripts after tool installation
- `docs/DESIGN-dependency-provisioning.md`: Marked issues #542, #546, #543 as complete

## Key Decisions

- Shell scripts instead of Go: Simpler implementation, aligns with existing scripts/*.sh pattern
- Separate scripts for each concern: Allows CI to run them independently
- Warning vs Error: Hardcoded RPATH is an error; string matches in binary data are warnings (may be benign)

## Trade-offs Accepted

- Scripts rely on platform tools (readelf, ldd, otool): Acceptable because these are standard on Linux/macOS
- Tool-specific tests are limited: Each tool has basic functional check; comprehensive testing is beyond scope

## Test Coverage

- Manual testing: Verified scripts work against zig, make installations
- CI integration: Scripts will be exercised by build-essentials workflow on all platforms

## Known Limitations

- verify-relocation.sh may report warnings for embedded strings that aren't actual paths
- verify-tool.sh has limited test coverage for some tools (e.g., pngcrush only checks help output)

## Future Improvements

- Add more comprehensive tool-specific tests as build essentials expand
- Consider adding verbose mode for CI debugging
