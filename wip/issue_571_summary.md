# Issue 571 Summary

## What Was Implemented

Added unified `Executor.Sandbox()` method to the `internal/sandbox/` package that accepts an `InstallationPlan` and `SandboxRequirements` to run container-based sandbox tests. This replaces the need for separate `Validate()` and `ValidateSourceBuild()` methods.

## Changes Made
- `internal/sandbox/executor.go`: New file with:
  - `Executor` struct with runtime detector and tsuku binary path
  - `Sandbox()` method that runs installation plans in containers
  - `buildSandboxScript()` that generates simplified shell script
  - `SandboxResult` struct for test results
  - `WithLogger()` and `WithTsukuBinary()` option functions
- `internal/sandbox/executor_test.go`: Comprehensive unit tests

## Key Decisions
- **Reuse validate.RuntimeDetector**: Instead of duplicating runtime detection code, the sandbox package imports and uses the existing detector from validate package. This will be refactored when validate is deprecated in #572.
- **Simplified sandbox script**: The script no longer installs build tools via apt-get. Instead, only ca-certificates is installed for network builds. Build tool dependencies are handled by tsuku's ActionDependencies system.
- **Requirements-driven configuration**: Container image, network mode, and resource limits are all derived from `SandboxRequirements` computed from the plan.

## Trade-offs Accepted
- **Temporary coupling to validate package**: The sandbox executor uses `validate.RuntimeDetector` and related types. This creates coupling but avoids code duplication during the transition period. Will be cleaned up in #572.

## Test Coverage
- New tests added: 9 test functions
- Tests cover: executor creation, options, script generation for offline and network builds, result structure, constants

## Known Limitations
- Does not include recipe verification (pattern matching) - builders can check `SandboxResult.ExitCode` for success
- Runtime mocking is limited - tests rely on actual runtime detection

## Future Improvements
- #572 will migrate builders to use this unified Sandbox() method
- #573 will add --sandbox CLI flag using this executor
- Eventually, validate package can be deprecated once all builders migrate
