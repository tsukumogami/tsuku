## Problem

Four tests in `internal/install` fail on macOS due to how temporary directories interact with symlink resolution:

- `TestComputeBinaryChecksums`
- `TestComputeBinaryChecksums_WithSymlink`
- `TestVerifyBinaryChecksums_AllMatch`
- `TestVerifyBinaryChecksums_Mismatch`

## Error Message

```
binary bin/tool1 resolves outside tool directory: /private/var/folders/3q/.../001/bin/tool1
```

## Root Cause

On macOS, the system temp directory (`/var/folders/...`) is actually a symlink to `/private/var/folders/...`. When the tests:

1. Create a temp directory (returns `/var/folders/...` path)
2. Create symlinks within it
3. Resolve the symlink to get the real path

The resolved path includes `/private/` prefix, which makes it appear to be "outside" the tool directory (which still has the `/var/folders/...` path without `/private/`).

## Expected Behavior

Tests should pass on macOS, handling the `/var` -> `/private/var` symlink correctly.

## Observed Behavior

Tests fail with "resolves outside tool directory" error because path comparison doesn't account for macOS temp directory symlinks.

## Suggested Fix

In the path comparison logic, canonicalize both paths (the tool directory and the resolved binary path) before comparing, so `/var/folders/...` and `/private/var/folders/...` are recognized as equivalent.

## Environment

- macOS (Darwin 24.6.0)
- Tests pass in CI (Linux)
