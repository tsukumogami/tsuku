# Issue 22 Implementation Summary

## Feature: Graceful Cancellation with Ctrl+C

### Overview

Implemented signal handling to support graceful cancellation when users press Ctrl+C, ensuring proper cleanup of partial installations and temp files.

### Changes Made

#### 1. Signal Handler in main.go
- Added `globalCtx` and `globalCancel` for application-level cancellation
- Signal handler intercepts SIGINT/SIGTERM and cancels the context
- Double Ctrl+C forces immediate exit
- Added `ExitCancelled` (130) exit code for cancelled operations

#### 2. Context Propagation Through Install Path
- `install.go` now passes `globalCtx` to the executor
- Executor's `Execute()` method accepts context parameter
- ExecutionContext carries the context to all actions

#### 3. Actions Updated for Context Support
- **download.go**: Uses `http.NewRequestWithContext` for cancellable HTTP requests
- **run_command.go**: Uses `exec.CommandContext` for cancellable shell commands
- **npm_install.go**: Uses `exec.CommandContext` for npm operations
- **cargo_install.go**: Uses `exec.CommandContext` for cargo operations
- **gem_install.go**: Uses `exec.CommandContext` for gem operations
- **pipx_install.go**: Uses `exec.CommandContext` for pipx operations
- **nix_install.go**: Uses `exec.CommandContext` for nix operations
- **nix_portable.go**: Added `EnsureNixPortableWithContext` with cancellable downloads

#### 4. Test Coverage
- Added `TestRunCommandAction_Execute_ContextCancellation` to verify context cancellation works

### Architecture

```
main.go
  |-- signal handler (SIGINT/SIGTERM)
  |       |
  v       v
globalCtx, globalCancel
        |
        v
install.go (passes globalCtx to executor)
        |
        v
executor.Execute(ctx)
        |
        v
ExecutionContext.Context
        |
        +-- download.go (http.NewRequestWithContext)
        +-- run_command.go (exec.CommandContext)
        +-- npm_install.go (exec.CommandContext)
        +-- cargo_install.go (exec.CommandContext)
        +-- gem_install.go (exec.CommandContext)
        +-- pipx_install.go (exec.CommandContext)
        +-- nix_install.go (exec.CommandContext)
```

### Behavior

1. When user presses Ctrl+C during an operation:
   - Signal handler cancels `globalCtx`
   - Running HTTP requests are cancelled immediately
   - Running shell commands receive SIGKILL from Go's CommandContext
   - Cleanup routines (defer statements) run normally
   - Program exits with code 130

2. Double Ctrl+C forces immediate exit for stuck operations

### Testing

- All unit tests pass
- Manual verification recommended:
  1. Run `tsuku install <large-tool>` and press Ctrl+C during download
  2. Verify operation stops promptly
  3. Verify no partial files in `$TSUKU_HOME/tools/`
  4. Verify temp directories are cleaned up

### Files Modified

| File | Changes |
|------|---------|
| cmd/tsuku/main.go | Signal handler, globalCtx |
| cmd/tsuku/exitcodes.go | ExitCancelled constant |
| cmd/tsuku/install.go | Pass globalCtx to executor |
| internal/actions/download.go | http.NewRequestWithContext |
| internal/actions/download_test.go | Context in tests |
| internal/actions/run_command.go | exec.CommandContext |
| internal/actions/run_command_test.go | Context cancellation test |
| internal/actions/npm_install.go | exec.CommandContext |
| internal/actions/cargo_install.go | exec.CommandContext |
| internal/actions/gem_install.go | exec.CommandContext |
| internal/actions/pipx_install.go | exec.CommandContext |
| internal/actions/nix_install.go | exec.CommandContext |
| internal/actions/nix_portable.go | EnsureNixPortableWithContext |

### Closes

- Closes #22
