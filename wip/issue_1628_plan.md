# Issue 1628 Implementation Plan

## Summary

Refine the existing prototype into a complete walking skeleton by adding a ServerLifecycle type with lock file protocol for reliable daemon detection, updating LocalProvider to use ServerLifecycle in Complete(), restructuring AddonManager to match the design (stub with AddonPath/IsInstalled), adding factory integration when local_enabled=true, and adding unit tests for lifecycle plus a mock gRPC server integration test.

## Approach

The prototype already provides a solid foundation with LocalProvider, gRPC proto definitions, and AddonManager. However, it doesn't match the design document's architecture. The key gaps are:

1. **No lock file protocol** - Current daemon detection uses socket probing which is unreliable for detecting stale sockets
2. **AddonManager has too much responsibility** - Contains lifecycle management that should be in ServerLifecycle
3. **LocalProvider doesn't use ServerLifecycle** - Complete() doesn't call lifecycle.EnsureRunning()
4. **No factory integration** - LocalProvider isn't registered in the factory when local_enabled=true
5. **Missing tests** - No lifecycle unit tests or mock gRPC server integration tests

The approach is to refactor the existing code to match the design rather than starting from scratch.

### Alternatives Considered

- **Start from scratch**: Rejected because the prototype has working gRPC client code, proto definitions, and test infrastructure that should be preserved.
- **Keep AddonManager with lifecycle logic**: Rejected because the design explicitly separates AddonManager (binary management) from ServerLifecycle (daemon lifecycle).
- **Use PID files instead of lock files**: Rejected because the design specifies lock files with non-blocking exclusive locks for reliable daemon detection (kernel automatically releases on process death).

## Files to Modify

- `internal/llm/local.go` - Add ServerLifecycle field, update Complete() to call EnsureRunning()
- `internal/llm/local_test.go` - Add tests for integration with mock gRPC server
- `internal/llm/addon/manager.go` - Remove lifecycle logic, make it a pure stub (AddonPath, IsInstalled only)
- `internal/llm/addon/manager_test.go` - Simplify tests to match reduced scope
- `internal/llm/factory.go` - Register LocalProvider as fallback when local_enabled=true
- `internal/llm/factory_test.go` - Add tests for local provider registration
- `internal/userconfig/userconfig.go` - Add LocalEnabled config option if not present

## Files to Create

- `internal/llm/lifecycle.go` - ServerLifecycle type with EnsureRunning, IsRunning, Stop, lock file protocol
- `internal/llm/lifecycle_test.go` - Unit tests for lifecycle manager

## Implementation Steps

- [ ] **Step 1: Add LocalEnabled config to userconfig**
  - Add `LocalEnabled *bool` to LLMConfig struct
  - Add `LLMLocalEnabled()` method returning bool (default true)
  - Add to Get/Set/AvailableKeys
  - Add tests

- [ ] **Step 2: Create lifecycle.go with ServerLifecycle type**
  - Define ServerLifecycle struct with fields for socket path, lock path, process
  - Implement `LockPath()` helper returning `$TSUKU_HOME/llm.sock.lock`
  - Implement `IsRunning()` using non-blocking exclusive lock attempt
  - Implement `EnsureRunning()` that checks lock, starts addon if needed, waits for ready
  - Implement `Stop()` for graceful shutdown via gRPC then SIGTERM
  - Handle stale socket cleanup (if lock acquired but socket exists, remove it)

- [ ] **Step 3: Simplify addon/manager.go to stub**
  - Keep only `AddonPath()` and `IsInstalled()` functions
  - Remove Manager struct, EnsureRunning, Shutdown, waitForReady, startAddon
  - These responsibilities move to lifecycle.go

- [ ] **Step 4: Update LocalProvider to use ServerLifecycle**
  - Add `lifecycle *ServerLifecycle` field to LocalProvider struct
  - Update NewLocalProvider to accept config and create ServerLifecycle
  - Update Complete() to call `p.lifecycle.EnsureRunning(ctx)` before gRPC call
  - Add Close() cleanup that calls lifecycle.Stop() if needed

- [ ] **Step 5: Add factory integration for LocalProvider**
  - Update NewFactory to accept LLMConfig with LocalEnabled method
  - When local_enabled=true and no API keys, register LocalProvider as fallback
  - LocalProvider priority: after Claude, after Gemini (lowest fallback)
  - Add circuit breaker for local provider

- [ ] **Step 6: Add lifecycle unit tests**
  - Test IsRunning returns false when no lock file
  - Test IsRunning returns true when lock held
  - Test EnsureRunning starts addon when not running
  - Test stale socket cleanup when lock acquired
  - Test Stop graceful shutdown

- [ ] **Step 7: Add mock gRPC server integration test**
  - Create in-process mock gRPC server implementing InferenceService
  - Test LocalProvider.Complete() with mock server
  - Test ServerLifecycle.EnsureRunning() detects running server
  - Use short timeouts for test reliability

- [ ] **Step 8: Update existing tests**
  - Update addon/manager_test.go for simplified stub
  - Update local_test.go for new LocalProvider signature
  - Update factory_test.go for local provider integration

## Testing Strategy

### Unit Tests
- `lifecycle_test.go`: Lock file protocol, IsRunning, EnsureRunning, Stop
- `addon/manager_test.go`: AddonPath, IsInstalled (existing, simplified)
- `factory_test.go`: LocalProvider registration with local_enabled config
- `userconfig_test.go`: LocalEnabled config getter/setter

### Integration Tests
- Mock gRPC server test: Start in-process server, test LocalProvider.Complete()
- Lifecycle integration: Test EnsureRunning with mock server

### Manual Verification
- Build and verify `go test ./internal/llm/...` passes
- Verify `go vet` and `golangci-lint` pass

## Risks and Mitigations

- **Lock file cross-platform**: The design focuses on Unix domain sockets. Windows uses named pipes. **Mitigation**: Document that lock file protocol is Unix-only for now, Windows support deferred.

- **Test isolation**: Tests that acquire locks or create sockets may interfere with each other. **Mitigation**: Use t.TempDir() for TSUKU_HOME in tests, run tests with `-p 1` if needed.

- **gRPC server mocking complexity**: Mock server setup can be verbose. **Mitigation**: Create a minimal bufconn-based mock that just implements health check and Complete.

- **Prototype code removal**: Removing lifecycle logic from addon/manager.go might break imports. **Mitigation**: Verify no external packages depend on Manager type before removing.

## Success Criteria

- [ ] ServerLifecycle type exists with EnsureRunning, IsRunning, Stop methods
- [ ] Lock file protocol at $TSUKU_HOME/llm.sock.lock implemented
- [ ] LocalProvider.Complete() calls ServerLifecycle.EnsureRunning()
- [ ] AddonManager reduced to stub with AddonPath(), IsInstalled()
- [ ] Factory registers LocalProvider when local_enabled=true and no cloud keys
- [ ] Unit tests for lifecycle manager pass
- [ ] Integration test with mock gRPC server passes
- [ ] All existing tests pass
- [ ] `go vet` and `golangci-lint` pass

## Open Questions

None - the design document and implementation context provide sufficient detail for all acceptance criteria.
