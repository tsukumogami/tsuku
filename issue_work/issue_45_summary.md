# Issue 45 Summary

## What Was Implemented

Added toolchain availability checking to `tsuku create` that runs before API calls. When a required toolchain (cargo, gem, pipx, npm) is not found in PATH, the command fails early with a helpful error message suggesting how to install the toolchain.

## Changes Made

- `internal/toolchain/toolchain.go`: New package with Info struct, ecosystem-to-toolchain mapping, and CheckAvailable/IsAvailable functions
- `internal/toolchain/toolchain_test.go`: Comprehensive tests for all ecosystems and error messages
- `cmd/tsuku/create.go`: Added import and toolchain check call before builder initialization

## Key Decisions

- **Package variable for LookPath**: Used a package-level `LookPathFunc` variable to enable testing without filesystem manipulation
- **Error message format**: Messages follow pattern "{Name} is required to create recipes from {ecosystem}. Install {Language} or run: tsuku install {recipe}"
- **Unknown ecosystems pass silently**: If an ecosystem is not in the mapping, no check is performed (allows for future ecosystems)

## Trade-offs Accepted

- **Single binary check per ecosystem**: Only checks for primary binary (e.g., cargo, not rustc) which is sufficient for current needs
- **No PATH caching**: Checks PATH fresh each time, but since `create` is interactive this is acceptable

## Test Coverage

- New tests added: 18 test cases across 4 test functions
- Coverage: Full coverage of toolchain package

## Known Limitations

- Only checks if binary exists in PATH, not if it's functional
- pypi ecosystem suggests "pipx" which users might need to install separately from Python
