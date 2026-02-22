# Issue 1888 Implementation Plan

## Problem

`install_gem_direct.go` installs gems to `<installDir>/.gem/bin/<name>` but `ExtractBinaries()` in `types.go` expects all gem executables to be at `<installDir>/bin/<name>`. This causes bundler installation to fail because the install manager cannot find the binaries at the expected location.

## Approach

Update `install_gem_direct.go` to follow the same pattern as `gem_install.go`:

1. Install gems directly to `<installDir>` instead of `.gem` subdirectory using `--install-dir <installDir>` and `--bindir <installDir>/bin`
2. Set environment variables `GEM_HOME=<installDir>` and `GEM_PATH=<installDir>` for isolation
3. Create wrapper scripts using `createGemWrapper()` from `gem_common.go` (same approach as `gem_install.go`)
4. Remove manual symlink creation code that bypasses the install manager
5. Find the ruby bin directory from the gem command path for wrapper creation

## Changes

### `internal/actions/install_gem_direct.go`

1. **Lines 66-89**: Replace the `.gem` subdirectory approach with direct installation to `<installDir>`
   - Remove `gemHome := filepath.Join(installDir, ".gem")`
   - Change `--install-dir` to use `installDir` directly
   - Change `--bindir` to use `filepath.Join(installDir, "bin")`
   - Update environment variables to use `installDir` instead of `gemHome`

2. **Lines 85-113**: Replace manual symlink creation with wrapper script creation
   - Keep the bin directory lookup (`filepath.Join(installDir, "bin")`)
   - Remove the `tsukuBinDir` symlink creation logic (lines 92-113)
   - Use `createGemWrapper()` for each executable like `gem_install.go` does (line 204)
   - Find ruby bin directory from gem command path (e.g., `filepath.Dir(gemPath)`)
   - Create wrapper scripts with `createGemWrapper(exePath, binDir, exe, gemDir, ".")`

3. **Import statement**: Already has `filepath` and `path/filepath`, no new imports needed

## Testing Strategy

1. **Unit test**: Verify `install_gem_direct` creates binaries at `<installDir>/bin/<name>` not `<installDir>/.gem/bin/<name>`
2. **Integration test**: Install bundler via `install_gem_direct` action and verify:
   - Binary exists at `<toolDir>/bin/bundle`
   - Binary is executable
   - Calling the wrapper sets `GEM_HOME` and `GEM_PATH` correctly
3. **Recipe test**: Run `tsuku install bundler` and verify it completes successfully and finds the bundler binary

## Risks

- **None identified**: The change follows the established `gem_install.go` pattern which is already working
- `createGemWrapper()` is already tested and used by `gem_install.go`
- No changes to public APIs or external behavior
- Existing tests should continue to pass
