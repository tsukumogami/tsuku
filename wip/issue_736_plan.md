# Implementation Plan: Issue #736

## Problem Analysis

Sandbox mode fails in fresh environments when `$TSUKU_HOME/cache/downloads` does not exist. The error occurs because the sandbox executor tries to mount a non-existent directory as a container volume.

**Root Cause:** In `cmd/tsuku/install_sandbox.go`, the sandbox executor is created with `WithDownloadCacheDir(cfg.DownloadCacheDir)` but `cfg.EnsureDirectories()` is never called beforehand. Container runtimes (podman/docker) fail with exit code 125 when mounting non-existent paths.

**Existing Pattern:** The fix already exists in `cmd/tsuku/create.go` (lines 305-309):
```go
if !effectiveSkipSandbox {
    // Ensure cache directories exist (needed for mounting into container)
    if err := cfg.EnsureDirectories(); err != nil {
        fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
        exitWithCode(ExitGeneral)
    }
    // ... create sandbox executor
}
```

## Solution

Apply the same pattern used in `create.go` to `install_sandbox.go`: call `cfg.EnsureDirectories()` before creating the sandbox executor.

## Changes

### File 1: `cmd/tsuku/install_sandbox.go`

**Location:** Lines 95-99, before creating the sandbox executor

**Change:** Add `EnsureDirectories()` call before sandbox executor creation

**Before:**
```go
	printInfo()

	// Create sandbox executor with download cache directory
	// This allows the sandbox to use pre-downloaded files from plan generation
	detector := validate.NewRuntimeDetector()
	sandboxExec := sandbox.NewExecutor(detector, sandbox.WithDownloadCacheDir(cfg.DownloadCacheDir))
```

**After:**
```go
	printInfo()

	// Ensure cache directories exist (needed for mounting into container)
	if err := cfg.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Create sandbox executor with download cache directory
	// This allows the sandbox to use pre-downloaded files from plan generation
	detector := validate.NewRuntimeDetector()
	sandboxExec := sandbox.NewExecutor(detector, sandbox.WithDownloadCacheDir(cfg.DownloadCacheDir))
```

## Testing

### Manual Testing
1. Remove `~/.tsuku` directory entirely:
   ```bash
   rm -rf ~/.tsuku
   ```

2. Build tsuku:
   ```bash
   go build -o tsuku ./cmd/tsuku
   ```

3. Run sandbox install with a local recipe:
   ```bash
   ./tsuku install --recipe testdata/recipes/gdbm-source.toml --sandbox --force
   ```

4. Expected: No "statfs ... no such file or directory" error. Sandbox either succeeds or skips if no container runtime is available.

### Automated Tests
Run existing tests to ensure no regressions:
```bash
go test ./...
```

## Implementation Steps

| Step | Description | Estimated Effort |
|------|-------------|------------------|
| 1 | Add `EnsureDirectories()` call in `install_sandbox.go` | 5 minutes |
| 2 | Run `go test ./...` to verify no regressions | 2 minutes |
| 3 | Manual test with fresh environment (if container runtime available) | 5 minutes |
| 4 | Create commit | 2 minutes |

## Risk Assessment

**Risk Level:** Low

- The change follows an established pattern already used in `create.go`
- `EnsureDirectories()` is idempotent (safe to call multiple times)
- No changes to core logic, only adding a prerequisite check
- No new dependencies or imports needed

## Commit Message

```
fix(sandbox): create cache directory before mounting in container

Add EnsureDirectories() call before creating sandbox executor in
install_sandbox.go to ensure $TSUKU_HOME/cache/downloads exists
before mounting it as a container volume.

This mirrors the pattern already used in create.go and fixes sandbox
failures in fresh environments where no tsuku directories exist yet.

Fixes #736
```
