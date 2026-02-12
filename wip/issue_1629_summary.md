# Issue 1629 Summary

## What Was Implemented

Implemented AddonManager with platform-specific addon download and SHA256 verification. The addon binary is downloaded on demand with verification happening at two points: immediately after download (to reject corrupted files) and before each execution (to detect post-download tampering).

## Changes Made

- `internal/llm/addon/manager.go`: Transformed from stub functions to AddonManager struct with EnsureAddon(), VerifyBeforeExecution(), and versioned path support
- `internal/llm/addon/manifest.go`: Embedded manifest types with GetManifest() and GetPlatformInfo()
- `internal/llm/addon/manifest.json`: Placeholder manifest with version and per-platform URLs/checksums
- `internal/llm/addon/platform.go`: Platform detection (PlatformKey, BinaryName, GetCurrentPlatformInfo)
- `internal/llm/addon/download.go`: Download logic with retry, exponential backoff, and progress display
- `internal/llm/addon/verify.go`: SHA256 verification functions (VerifyChecksum, ComputeChecksum)
- `internal/llm/addon/manager_test.go`: Comprehensive tests with mock HTTP server
- `internal/llm/local.go`: Integrated AddonManager with LocalProvider
- `internal/llm/lifecycle.go`: Added pre-execution verification hook via AddonManager

## Key Decisions

- **Embedded manifest**: Manifest is embedded in tsuku binary via `//go:embed` for supply chain security. Downloading manifest from CDN would defeat the purpose of having checksums.
- **JSON format**: Chose JSON over TOML for manifest since it's a simple data structure without need for comments or human editing.
- **Versioned paths**: Addon stored at `$TSUKU_HOME/tools/tsuku-llm/<version>/tsuku-llm` to support version upgrades.
- **File locking**: Uses file lock during download to prevent race conditions from concurrent tsuku processes.

## Trade-offs Accepted

- **Placeholder checksums**: The manifest contains placeholder checksums (zeros) since the CI pipeline (#1633) hasn't shipped real binaries yet. Tests use mock server with computed checksums.
- **TOCTOU window**: Brief window exists between verification and execve(). Accepted because the lock file protocol and pre-execution verification significantly raises the bar for attacks.

## Test Coverage

- New tests added: 15 test functions covering manifest parsing, platform detection, checksum verification, download with mock server, re-download on failure, and pre-execution verification
- Coverage: All new code paths tested including error conditions

## Known Limitations

- Manifest contains placeholder URLs and checksums until CI pipeline (#1633) ships real binaries
- No download progress callback for external integrations (downstream #1642 will add this)
- Windows support included in manifest but not tested on Windows

## Future Improvements

- Issue #1633 will provide real addon binaries and checksums
- Issue #1642 will add download permission prompts and progress UX
- Issue #1643 will add `tsuku llm download` CLI command for pre-download
