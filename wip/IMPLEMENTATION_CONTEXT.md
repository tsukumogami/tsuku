## Summary

`tools/current/` symlinks are dangling for directory-mode installs (most tools). `createBinarySymlink` in `internal/install/manager.go` creates symlinks like `tools/current/patchelf -> tools/patchelf-0.18.0/patchelf` but the actual binary lives at `tools/patchelf-0.18.0/bin/patchelf`. The symlink target is missing the `bin/` path component.

This is masked in normal usage because the executor's in-process `ExecPaths` mechanism adds `tools/<tool>-<ver>/bin` directly to PATH, bypassing the broken symlinks. But it causes real problems in foundation Docker images where each dependency is installed in a separate `RUN` command (fresh process, empty ExecPaths), making previously installed tools undiscoverable via PATH.

## Reproduction

```bash
tsuku install patchelf
ls -la $TSUKU_HOME/tools/current/patchelf
# Symlink points to: tools/patchelf-0.18.0/patchelf (does NOT exist)
# Actual binary at:  tools/patchelf-0.18.0/bin/patchelf
```

This applies to any tool with directory-mode layout (binaries under `bin/` subdirectory). Flat-layout tools like zig are not affected.

## Root Cause

Three code paths fall back to using just the tool name as the binary path when `Binaries` is empty:

1. **`createSymlink`** (line ~295): calls `createBinarySymlink(name, version, name)` -- passes just `name` without `bin/` prefix
2. **`Activate`** (line ~258): fallback `binaries = []string{name}` -- just the tool name
3. **`RemoveVersion`** (line ~124): fallback `binaries = []string{name}` -- just the tool name

When `Binaries` IS populated (e.g., `[]string{"bin/mytool"}`), the `bin/` prefix is included and the symlink target resolves correctly. The bug only manifests in the empty-binaries fallback paths.

In `createBinarySymlink` (line ~300-320), the target path is computed as:

```go
targetPath := filepath.Join(m.config.ToolDir(toolName, version), binaryPath)
```

When `binaryPath` is just `"patchelf"` instead of `"bin/patchelf"`, this produces `tools/patchelf-0.18.0/patchelf` which doesn't exist.

## Impact

- `tools/current/` symlinks are dangling for most tools
- The `$TSUKU_HOME/env` PATH setup (which adds `$TSUKU_HOME/tools/current` to PATH) doesn't work for finding these tools
- Foundation image builds need per-dependency PATH workarounds (see PR #1956)

## Relevant Code

- `internal/install/manager.go` -- `createBinarySymlink` (line ~300), `createSymlink` (line ~294), `Activate` (line ~254)
- `internal/install/remove.go` -- `RemoveVersion` (line ~122)

## Fix Direction

The fallback paths should use `"bin/" + name` instead of just `name` for directory-mode tools. Alternatively, ensure that `Binaries` is always populated during install so the fallback paths are never hit. The latter is safer since it avoids assuming the `bin/` convention.
