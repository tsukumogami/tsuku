# Issue 1629 Implementation Plan

## Summary

Implement AddonManager struct with EnsureAddon() method that downloads platform-specific tsuku-llm binary with SHA256 verification at download time and before each execution. Uses embedded manifest for checksums and URLs.

## Approach

Build on the existing stub at `internal/llm/addon/manager.go` by adding an AddonManager struct that mirrors the proven patterns from `internal/actions/nix_portable.go` (hardcoded checksums, atomic downloads, file locking). The manifest will be embedded via `//go:embed` directive as a JSON file, following the pattern in `internal/recipe/embedded.go`. Verification happens twice: after download (reject corrupted downloads) and before execution (detect post-download tampering).

### Alternatives Considered

- **External manifest file (download from CDN)**: Rejected because the design doc explicitly requires "manifest embedded in tsuku" for security. Downloading manifest defeats the purpose of having a known-good checksum embedded in the verified binary.

- **TOML manifest format**: Rejected in favor of JSON. The manifest is a simple data structure (version, platforms with URLs and checksums). JSON is simpler to parse and matches common CDN metadata patterns. TOML's strengths (human-authored config, comments) aren't needed here.

- **Verify only at download time**: Rejected per design doc which requires verification "at download time AND before each execution" to prevent post-download binary replacement attacks.

## Files to Modify

- `internal/llm/addon/manager.go` - Transform from function-based stub to AddonManager struct with download, verification, and platform detection logic
- `internal/llm/addon/manager_test.go` - Add tests for new AddonManager functionality
- `internal/llm/local.go` - Update LocalProvider to use AddonManager.EnsureAddon() before lifecycle operations
- `internal/llm/lifecycle.go` - Add verification hook before starting addon

## Files to Create

- `internal/llm/addon/manifest.go` - Manifest types and embedded JSON loading
- `internal/llm/addon/manifest.json` - Embedded manifest with placeholder values (real URLs/checksums come after CI pipeline #1633)
- `internal/llm/addon/download.go` - Download logic with retry and progress display
- `internal/llm/addon/verify.go` - SHA256 verification functions (wrapper around actions.VerifyChecksum)
- `internal/llm/addon/platform.go` - Platform detection (GOOS-GOARCH key generation)

## Implementation Steps

- [x] **Create manifest types and embedding** (`manifest.go`, `manifest.json`)
  - Define Manifest struct with Version, Platforms map[string]PlatformInfo
  - PlatformInfo has URL and SHA256 fields
  - Use `//go:embed manifest.json` directive
  - Add GetManifest() function that returns parsed manifest
  - Use placeholder checksums until CI pipeline exists

- [x] **Add platform detection** (`platform.go`)
  - PlatformKey() function returns "GOOS-GOARCH" (e.g., "darwin-arm64", "linux-amd64")
  - Handle Windows .exe extension in binary name
  - GetPlatformInfo() returns PlatformInfo for current platform or error if unsupported

- [x] **Implement download logic** (`download.go`)
  - Download() function downloads file to temp location
  - Uses httputil.NewSecureClient for HTTPS enforcement and SSRF protection
  - Adds retry logic with exponential backoff (mirror nix_portable.go pattern)
  - Shows progress bar during download
  - Atomic rename to final location after verification

- [x] **Add verification functions** (`verify.go`)
  - VerifyChecksum(path, expectedSHA256) wraps actions.VerifyChecksum
  - ComputeChecksum(path) returns hex-encoded SHA256 for testing

- [x] **Create AddonManager struct** (update `manager.go`)
  - Fields: homeDir (for TSUKU_HOME), manifest (cached parsed manifest)
  - AddonDir() returns versioned path: $TSUKU_HOME/tools/tsuku-llm/<version>/
  - BinaryPath() returns full path to binary including version subdirectory
  - IsInstalled() checks if binary exists at versioned path
  - EnsureAddon() - main entry point:
    1. Get platform info from manifest
    2. If binary exists, verify checksum before returning
    3. If verification fails or binary missing, download
    4. Verify downloaded binary, fail if mismatch
    5. Return path to verified binary

- [x] **Integrate with LocalProvider** (update `local.go`)
  - Add addonManager field to LocalProvider struct
  - In Complete(), call addonManager.EnsureAddon() before lifecycle.EnsureRunning()
  - Pass verified addon path to lifecycle

- [x] **Add pre-execution verification to lifecycle** (update `lifecycle.go`)
  - Before starting addon in EnsureRunning(), verify checksum again
  - This catches tampering between EnsureAddon() and process start

- [x] **Write comprehensive tests** (update `manager_test.go`)
  - Test platform key generation for different GOOS/GOARCH
  - Test manifest parsing
  - Test checksum verification (correct, incorrect, missing file)
  - Test EnsureAddon with mock HTTP server
  - Test re-download on verification failure
  - Test versioned path structure

- [x] **Verify E2E skeleton still works**
  - Run existing tests: `go test ./internal/llm/...`
  - Verify LocalProvider flow still functions with stub manifest

## Testing Strategy

- **Unit tests**:
  - Platform detection for darwin-arm64, darwin-amd64, linux-amd64, windows-amd64
  - Manifest parsing with valid and invalid JSON
  - Checksum verification with matching and mismatched hashes
  - AddonManager.EnsureAddon() with mock server returning test binary
  - Re-download behavior when existing binary fails verification

- **Integration tests**:
  - Use httptest.Server to serve a fake addon binary
  - Verify download, verification, and path structure end-to-end
  - Test that tampering a binary after download triggers re-download

- **Manual verification**:
  - Run `go test ./internal/llm/...` to ensure all existing tests pass
  - Verify lifecycle tests still work with updated integration

## Risks and Mitigations

- **No real addon binary exists yet**: The CI pipeline (#1633) hasn't shipped real binaries. Mitigation: Use placeholder checksums and URLs in manifest.json. Tests use mock HTTP server with test binaries. Real values are updated when CI pipeline completes.

- **Platform detection edge cases**: Some platforms (e.g., FreeBSD, Windows ARM64) may not be supported. Mitigation: Return clear "unsupported platform" error. Start with darwin-arm64, darwin-amd64, linux-amd64 as the design specifies.

- **Circular import with actions package**: AddonManager needs VerifyChecksum from actions package. Mitigation: Either duplicate the verification code or import actions (which is acceptable since actions is a utility package with no llm imports).

- **TOCTOU between verification and execution**: There's a brief window between verifying the binary and execve(). Mitigation: The design accepts this residual risk. The lock file protocol and verification before each execution significantly raises the bar for attacks.

## Success Criteria

- [x] AddonManager struct exists at `internal/llm/addon/manager.go`
- [x] EnsureAddon() downloads addon if not present and returns path to binary
- [x] Platform detection returns correct key for darwin-arm64, darwin-amd64, linux-amd64
- [x] SHA256 verification occurs at download time and rejects mismatched binaries
- [x] SHA256 verification occurs before execution in lifecycle.EnsureRunning()
- [x] Addon stored at `$TSUKU_HOME/tools/tsuku-llm/<version>/tsuku-llm`
- [x] Manifest is embedded in binary via `//go:embed`
- [x] Failed verification returns clear error (not panic)
- [x] All existing tests in internal/llm pass
- [x] New unit tests achieve reasonable coverage of new code

## Open Questions

None blocking. The placeholder manifest approach allows development to proceed independently of the CI pipeline (#1633).
