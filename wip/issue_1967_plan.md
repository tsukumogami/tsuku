# Issue 1967 Plan

## Problem

Three fallback code paths create symlinks targeting `tools/<name>-<ver>/<name>` when `Binaries` is empty, but the correct target is `tools/<name>-<ver>/bin/<name>`. The wrapper fallback already uses the correct path (`filepath.Join("bin", toolName)`), making this an inconsistency.

## Approach

Fix the three fallback paths to use `filepath.Join("bin", name)` instead of just `name`, matching the existing correct behavior in `createWrappersForBinaries`.

## Files to Modify

1. `internal/install/manager.go` - Two fixes:
   - `createSymlink` (line 295): change `name` to `filepath.Join("bin", name)`
   - `Activate` (line 258): change `[]string{name}` to `[]string{filepath.Join("bin", name)}`

2. `internal/install/remove.go` - One fix:
   - `RemoveVersion` (line 124): change `[]string{name}` to `[]string{filepath.Join("bin", name)}`

## Testing

Add unit tests that verify the fallback paths produce correct symlink targets. The existing test infrastructure in `internal/install/` should support this.

## Steps

1. Fix the three fallback paths
2. Add/update tests
3. Run `go test ./...`, `go vet ./...`, `go build`
