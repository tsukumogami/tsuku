# Issue 1968 Summary

## What Was Implemented

Added `$TSUKU_HOME/env` file creation as a side effect of `tsuku install`, and removed the per-dependency PATH workaround from the sandbox Dockerfile generator.

## Changes Made

- `internal/config/config.go`: Added `EnvFile()` path helper and `EnsureEnvFile()` method with idempotent env file creation
- `internal/config/config_test.go`: Added 4 tests covering path, creation, idempotency, and content format
- `internal/install/manager.go`: Call `EnsureEnvFile()` after `EnsureDirectories()` in `InstallWithOptions` (non-fatal on error)
- `internal/sandbox/foundation.go`: Removed per-dep `ENV PATH` lines from `GenerateFoundationDockerfile` loop
- `internal/sandbox/foundation_test.go`: Updated expectations to not assert per-dep ENV PATH lines, added negative assertions

## Key Decisions

- Env file content uses `${TSUKU_HOME:-$HOME/.tsuku}` fallback syntax to match `website/install.sh` format, making the file portable regardless of whether `$TSUKU_HOME` is set
- `EnsureEnvFile()` is idempotent: it reads the file first and only writes if content differs, avoiding unnecessary disk writes
- Env file creation failure is non-fatal (logged as warning) since it's a convenience, not critical to installation
- No `source $TSUKU_HOME/env` added to the Dockerfile because Docker ENV directives (which persist across layers) already handle PATH -- the global `ENV PATH=...tools/current...bin:$PATH` is sufficient

## Trade-offs Accepted

- The env file content is duplicated between `config.go` and `website/install.sh`. A comment in `config.go` references `install.sh` to help future maintainers keep them in sync.

## Test Coverage

- New tests added: 4 (config package)
- Updated tests: 2 (foundation_test.go)
- Manual sandbox test: cargo-audit with zig dependency passed on debian family

## Requirements Mapping

| AC | Status | Evidence |
|----|--------|----------|
| `tsuku install` creates/updates `$TSUKU_HOME/env` with PATH entries | Implemented | `config.go:EnsureEnvFile()` called from `manager.go:InstallWithOptions()` |
| `GenerateFoundationDockerfile` removes per-dep ENV PATH workarounds | Implemented | `foundation.go:GenerateFoundationDockerfile()` loop simplified |
| Per-dep ENV PATH lines removed | Implemented | `foundation.go` lines 122-127 removed |
| tools/current/ symlinks work correctly | Verified | Sandbox test confirmed: zig and rust found via tools/current during cargo-audit build |
