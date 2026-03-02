# Issue 1986 Implementation Plan

## Summary

Modify `buildDeterministicCargoEnv()` to accept the full `ExecutionContext` and iterate `ctx.Dependencies.InstallTime` to set `PKG_CONFIG_PATH`, `C_INCLUDE_PATH`, and `LIBRARY_PATH` from library dependency paths, following the same discovery logic already used by `buildAutotoolsEnv()`.

## Approach

The fix applies the same dependency-directory discovery pattern that `buildAutotoolsEnv()` uses (checking `ctx.LibsDir` and `ctx.ToolsDir` for each dependency, then probing for `lib/pkgconfig`, `include/`, and `lib/` subdirectories). The difference is that for Rust/cargo builds we set `C_INCLUDE_PATH` and `LIBRARY_PATH` instead of `CPPFLAGS` and `LDFLAGS`, because cargo build scripts use the `cc` crate which reads these env vars directly rather than parsing shell-style flags.

The function signature changes from `buildDeterministicCargoEnv(cargoPath, workDir string, execPaths []string)` to `buildDeterministicCargoEnv(cargoPath, workDir string, ctx *ExecutionContext)`. This gives it access to `ExecPaths`, `Dependencies`, `LibsDir`, and `ToolsDir`. The `workDir` parameter stays separate because the lock_data mode passes `tempDir` instead of `ctx.WorkDir`.

### Alternatives Considered

- **Extract a shared helper function from buildAutotoolsEnv**: Both functions would call a common `discoverDepPaths()` helper. Not chosen because the two functions set different env vars (CPPFLAGS/LDFLAGS vs C_INCLUDE_PATH/LIBRARY_PATH) and buildAutotoolsEnv has extra git/curl-specific logic. The coupling would add complexity without saving much code.
- **Pass individual fields instead of *ExecutionContext**: Would keep the function decoupled from ExecutionContext but requires 4+ parameters. Not chosen because `buildAutotoolsEnv` already takes `*ExecutionContext` and it's the project convention.
- **Also fix cargo_install.go Execute()**: The non-decomposed `CargoInstallAction.Execute()` builds its own env without calling `buildDeterministicCargoEnv`. Recipes using `extra_dependencies` go through the decomposed cargo_build path, so this is out of scope. Noted as a future improvement.

## Files to Modify

- `internal/actions/cargo_build.go` - Change `buildDeterministicCargoEnv` signature, add dependency path discovery loop, update all 3 call sites
- `internal/actions/cargo_build_test.go` - Update existing tests for new signature, add tests for dependency env vars

## Files to Create

None.

## Implementation Steps

- [x] **Step 1: Change `buildDeterministicCargoEnv` signature** -- Change from `(cargoPath, workDir string, execPaths []string)` to `(cargoPath, workDir string, ctx *ExecutionContext)`. Inside the function, read `ctx.ExecPaths` where `execPaths` was used before. Guard against nil ctx: if ctx is nil, skip dependency discovery (keeps the function safe for the rust-version-check call site which passes a minimal context).

- [x] **Step 2: Add dependency path discovery** -- After the existing env setup (C compiler, etc.), iterate `ctx.Dependencies.InstallTime`. For each dep, resolve the directory using the same libs-then-tools fallback as `buildAutotoolsEnv`. Probe for `lib/pkgconfig`, `include/`, `lib/`, and `bin/` subdirectories. Collect paths into `pkgConfigPaths`, `includePaths`, `libraryPaths`, and `depBinPaths` slices.

- [x] **Step 3: Set environment variables from discovered paths** -- After the discovery loop, set:
  - `PKG_CONFIG_PATH` from `pkgConfigPaths` (colon-separated)
  - `C_INCLUDE_PATH` from `includePaths` (colon-separated)
  - `LIBRARY_PATH` from `libraryPaths` (colon-separated)
  - Prepend `depBinPaths` to PATH (for tools like pkg-config itself)

  Filter out any existing `PKG_CONFIG_PATH`, `C_INCLUDE_PATH`, and `LIBRARY_PATH` from the inherited environment to avoid stale values.

- [x] **Step 4: Update call sites** -- Update the 3 call sites in `cargo_build.go`:
  1. Line 151 (source_dir mode): `buildDeterministicCargoEnv(cargoPath, ctx.WorkDir, ctx)`
  2. Line 357 (rust version check in lock_data mode): `buildDeterministicCargoEnv(cargoPath, ctx.WorkDir, ctx)`
  3. Line 432 (lock_data build mode): `buildDeterministicCargoEnv(cargoPath, tempDir, ctx)`

- [x] **Step 5: Update existing tests** -- Change all `buildDeterministicCargoEnv(cargoPath, workDir, nil)` calls to pass either `nil` (if the function handles nil ctx gracefully) or a minimal `*ExecutionContext`. Verify existing assertions still pass.

- [x] **Step 6: Add test for dependency env vars** -- Add `TestBuildDeterministicCargoEnv_WithDependencies` that creates mock dependency directories with `lib/pkgconfig`, `include/`, and `lib/` subdirectories, then verifies `PKG_CONFIG_PATH`, `C_INCLUDE_PATH`, and `LIBRARY_PATH` are set correctly. Mirror the pattern from `TestBuildAutotoolsEnv_WithDependencies`.

- [x] **Step 7: Add test for no dependencies** -- Add `TestBuildDeterministicCargoEnv_NoDependencies` that verifies `PKG_CONFIG_PATH`, `C_INCLUDE_PATH`, and `LIBRARY_PATH` are NOT set when there are no install-time dependencies.

- [x] **Step 8: Add test for libs vs tools directory fallback** -- Add a test that creates a dependency in `LibsDir` and verifies it's found there (not in `ToolsDir`). Then test a dependency only in `ToolsDir` to verify the fallback.

- [x] **Step 9: Run tests and lint** -- Run `go test ./internal/actions/...`, `go vet ./...`, and `go build ./cmd/tsuku` to verify everything passes.

## Testing Strategy

- **Unit tests**: Test `buildDeterministicCargoEnv` directly with mock dependency directories:
  - With dependencies that have all subdirectories (lib/pkgconfig, include, lib, bin)
  - With dependencies missing some subdirectories (only lib, no pkgconfig)
  - With no dependencies (nil or empty map)
  - With nil ExecutionContext (backward compat)
  - With deps in LibsDir vs ToolsDir (fallback behavior)
- **Integration tests**: Not applicable -- the actual cargo build requires real Rust toolchain and crates.io access.
- **Manual verification**: Build a recipe with `extra_dependencies = ["openssl"]` using `cargo_build` and verify the build succeeds.

## Risks and Mitigations

- **Signature change breaks other callers**: Grep confirms `buildDeterministicCargoEnv` is only called within `cargo_build.go` (3 call sites) and `cargo_build_test.go` (3 test calls). No external callers exist. Risk is low.
- **Nil pointer on ctx.Dependencies or ctx fields**: The function should guard against nil `ctx` and nil/empty `Dependencies.InstallTime` map. The existing `buildAutotoolsEnv` doesn't guard against nil because it always receives a full context, but since `buildDeterministicCargoEnv` has a call site for version validation (line 357) that builds a minimal context, we should be defensive.
- **Environment variable ordering**: Multiple calls to append PATH could create duplicates. The function already builds PATH from scratch (cargo dir + execPaths + existing), so we need to integrate `depBinPaths` into this existing PATH construction rather than appending separately.

## Success Criteria

- [ ] `buildDeterministicCargoEnv` sets `PKG_CONFIG_PATH` when dependencies have `lib/pkgconfig` directories
- [ ] `buildDeterministicCargoEnv` sets `C_INCLUDE_PATH` when dependencies have `include` directories
- [ ] `buildDeterministicCargoEnv` sets `LIBRARY_PATH` when dependencies have `lib` directories
- [ ] Dependency `bin/` directories are added to PATH
- [ ] No env vars set when dependencies have no matching subdirectories
- [ ] No env vars set when there are no dependencies
- [ ] Existing tests pass with updated function signature
- [ ] `go test ./internal/actions/...` passes
- [ ] `go vet ./...` passes
- [ ] `go build ./cmd/tsuku` succeeds

## Open Questions

None -- the approach is well-defined by the existing `buildAutotoolsEnv` pattern and the IMPLEMENTATION_CONTEXT.
