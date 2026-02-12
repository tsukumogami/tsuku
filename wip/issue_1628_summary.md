# Issue #1628: Local Provider Skeleton with Lifecycle Management

## What was done

Implemented the foundation for local LLM runtime integration:

### 1. ServerLifecycle (`internal/llm/lifecycle.go`)
- Lock file protocol for reliable daemon state detection using `syscall.Flock`
- `EnsureRunning(ctx)` - starts addon server if not running, waits for socket availability
- `IsRunning()` - checks if lock is held by another process
- `Stop(ctx)` - graceful shutdown via gRPC, falls back to SIGTERM
- Stale socket cleanup when lock is acquired but socket exists

### 2. AddonManager Stub (`internal/llm/addon/manager.go`)
- Simplified to two functions: `AddonPath()` and `IsInstalled()`
- Path management for the tsuku-llm binary at `$TSUKU_HOME/tools/tsuku-llm/tsuku-llm`

### 3. LocalProvider Updates (`internal/llm/local.go`)
- Added `lifecycle *ServerLifecycle` field
- `Complete()` calls `lifecycle.EnsureRunning(ctx)` before gRPC calls
- Added `LockPath()` function for external use

### 4. Factory Integration (`internal/llm/factory.go`)
- Extended `LLMConfig` interface with `LLMLocalEnabled()` method
- Added `WithLocalEnabled()` option
- LocalProvider registered as lowest priority fallback when `local_enabled=true`

### 5. Configuration (`internal/userconfig/userconfig.go`)
- Added `local_enabled` config option under `[llm]` section
- Default: `true` (local LLM enabled by default)

### 6. Tests
- `lifecycle_test.go`: Unit tests for lock file protocol, stale socket cleanup
- `local_test.go`: Mock gRPC server integration test using bufconn
- `factory_test.go`: Updated for new interface, added local-only test

## Files changed

- `internal/llm/lifecycle.go` (new)
- `internal/llm/lifecycle_test.go` (new)
- `internal/llm/local.go` (modified)
- `internal/llm/local_test.go` (modified)
- `internal/llm/addon/manager.go` (simplified)
- `internal/llm/addon/manager_test.go` (simplified)
- `internal/llm/factory.go` (modified)
- `internal/llm/factory_test.go` (modified)
- `internal/userconfig/userconfig.go` (modified)

## Validation

- All tests pass: `go test ./internal/llm/...`
- Build succeeds: `go build ./...`
- Lint passes: `golangci-lint run`
