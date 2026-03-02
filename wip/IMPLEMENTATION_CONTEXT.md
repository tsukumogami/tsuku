## Context Summary

- **Goal**: Make `cargo_build` action set `PKG_CONFIG_PATH` and related env vars so that crates with native C library dependencies (like openssl-sys) can find libraries installed as `extra_dependencies`.
- **Key constraint**: The fix must be general -- not openssl-specific. Any library dependency installed to `$TSUKU_HOME/libs/` or `$TSUKU_HOME/tools/` should be discoverable.
- **Integration point**: `buildDeterministicCargoEnv()` in `internal/actions/cargo_build.go` is the single function that builds the env for both execution modes (source_dir and lock_data).
- **Existing pattern**: `buildAutotoolsEnv()` in `configure_make.go` already iterates `ctx.Dependencies.InstallTime`, locates `lib/pkgconfig`, `include/`, and `lib/` directories, and sets `PKG_CONFIG_PATH`, `CPPFLAGS`, `LDFLAGS`. The cargo fix should follow the same discovery logic.
- **Dependencies flow**: Libraries are installed to `$TSUKU_HOME/libs/{name}-{version}/`, tools to `$TSUKU_HOME/tools/{name}-{version}/`. The `ExecutionContext.Dependencies.InstallTime` map holds resolved dependency name-version pairs. `ctx.LibsDir` and `ctx.ToolsDir` hold the root directories.

## Issue

When a cargo recipe declares `extra_dependencies = ["openssl", "pkg-config"]`, the sandbox installs them correctly but `cargo_build` doesn't set `PKG_CONFIG_PATH` before running `cargo build --release`. Build scripts that use `pkg-config` to find native C libraries fail.

## Affected code

- `internal/actions/cargo_build.go` -- `buildDeterministicCargoEnv()` needs to accept dependency info and set library env vars
- Both call sites in `Execute()` (source_dir mode, line 151) and `executeLockDataMode()` (lines 357, 432)

## Env vars to set

1. `PKG_CONFIG_PATH` -- from `{dep}/lib/pkgconfig` directories
2. `C_INCLUDE_PATH` -- from `{dep}/include` directories (for cc crate)
3. `LIBRARY_PATH` -- from `{dep}/lib` directories (for linker)
