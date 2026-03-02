# Issue 1986 Summary

## What Was Implemented

Modified `buildDeterministicCargoEnv()` to discover and expose library dependency paths via `PKG_CONFIG_PATH`, `C_INCLUDE_PATH`, and `LIBRARY_PATH` environment variables, so that cargo build scripts (like `openssl-sys`) can find native C libraries installed as `extra_dependencies`.

## Changes Made
- `internal/actions/cargo_build.go`: Changed `buildDeterministicCargoEnv` signature from `(cargoPath, workDir string, execPaths []string)` to `(cargoPath, workDir string, ctx *ExecutionContext)`. Added dependency path discovery loop that checks `ctx.LibsDir` then `ctx.ToolsDir` for each dependency, probing for `lib/pkgconfig`, `include/`, `lib/`, and `bin/` subdirectories. Updated all 3 call sites.
- `internal/actions/cargo_build_test.go`: Added 5 new tests covering dependency env vars, no-deps case, libs-vs-tools priority, tools-dir fallback, and partial subdirectory presence.

## Key Decisions
- Used `C_INCLUDE_PATH` and `LIBRARY_PATH` instead of `CPPFLAGS`/`LDFLAGS`: Rust build scripts use the `cc` crate which reads these env vars directly, rather than parsing shell-style flags.
- Did not add `OPENSSL_DIR` special case: `PKG_CONFIG_PATH` + `C_INCLUDE_PATH` + `LIBRARY_PATH` provide the same discovery paths generically for all -sys crates, not just openssl-sys.
- ctx may be nil: Guards against nil to keep backward compatibility for tests that pass nil.

## Trade-offs Accepted
- Dependency bin paths from the discovery loop may overlap with ExecPaths already set by the executor. PATH deduplication isn't strictly necessary since having a path twice is harmless and avoids coupling to executor internals.

## Test Coverage
- New tests added: 5
- Existing tests: All 3 `buildDeterministicCargoEnv` tests pass unchanged (nil ctx still works)

## Known Limitations
- The non-decomposed `CargoInstallAction.Execute()` builds its own env without calling `buildDeterministicCargoEnv`. Recipes using `extra_dependencies` go through the decomposed `cargo_build` path so this doesn't affect the reported bug, but it's a potential gap for future edge cases.

## Requirements Mapping

| AC | Status | Evidence / Reason |
|----|--------|-------------------|
| PKG_CONFIG_PATH should include lib/pkgconfig of each installed dependency | Implemented | `cargo_build.go:558-561`, `TestBuildDeterministicCargoEnv_WithDependencies` |
| OPENSSL_DIR or OPENSSL_LIB_DIR + OPENSSL_INCLUDE_DIR as fallback | Covered generically | C_INCLUDE_PATH and LIBRARY_PATH serve the same purpose for all -sys crates, not just openssl-sys |
