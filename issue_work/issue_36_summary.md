# Issue 36 Summary

## What Was Implemented

Added `TSUKU_HOME` environment variable support to allow users to override the default `~/.tsuku` installation directory. This enables running tsuku in containerized/CI environments with custom paths and testing with isolated directories.

## Changes Made

- `internal/config/config.go`: Added `EnvTsukuHome` constant and modified `DefaultConfig()` to check for `TSUKU_HOME` env var before falling back to `~/.tsuku`
- `internal/config/config_test.go`: Added two new tests for env var override behavior
- `internal/actions/action.go`: Added `ToolsDir` field to `ExecutionContext` struct
- `internal/actions/gem_install.go`: Updated to use `ctx.ToolsDir` instead of hardcoded `~/.tsuku/tools` path for zig-cc-wrapper
- `internal/install/manager.go`: Updated `fixPipxShebangs()` and `findPythonStandalone()` to accept `toolsDir` parameter instead of computing it from `os.UserHomeDir()`
- `internal/install/bootstrap.go`: Added `SetToolsDir()` call when creating executor
- `internal/executor/executor.go`: Added `toolsDir` field, `SetToolsDir()` method, and set `ToolsDir` in `ExecutionContext`
- `cmd/tsuku/main.go`: Added `SetToolsDir()` call when creating executor

## Key Decisions

- **ToolsDir field in ExecutionContext**: Rather than passing the full Config to ExecutionContext (too heavy), we added just the ToolsDir field since that's all actions need to find other installed tools.
- **Setter method pattern**: Used `SetToolsDir()` method on Executor (similar to existing `SetExecPaths()`) to maintain consistency with existing code patterns.

## Trade-offs Accepted

- **Actions cannot access full config**: Actions only have access to ToolsDir, not the full configuration. This is acceptable because actions should only need to locate other tools, not modify config.

## Test Coverage

- New tests added: 2 (TestDefaultConfig_WithTsukuHome, TestDefaultConfig_EmptyTsukuHome)
- Coverage change: 33.0% -> 33.1% overall, 84.6% -> 86.7% for config package

## Known Limitations

- None

## Future Improvements

- Consider adding TSUKU_HOME documentation to the README or help output
