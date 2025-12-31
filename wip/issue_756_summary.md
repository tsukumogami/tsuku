# Issue 756 Summary

## What Was Implemented

Created Go structs for system configuration and verification actions, completing the configuration/verification portion of the system dependency action vocabulary defined in DESIGN-system-dependency-actions.md.

## Changes Made

- `internal/actions/system_config.go`: New file with 5 action structs:
  - `GroupAddAction`: Adds user to a system group
  - `ServiceEnableAction`: Enables a systemd service
  - `ServiceStartAction`: Starts a systemd service
  - `RequireCommandAction`: Verifies a command exists in PATH (with optional version checking)
  - `ManualAction`: Displays manual installation instructions

- `internal/actions/system_config_test.go`: Comprehensive unit tests (55+ test cases)

- `internal/actions/action.go`: Registered all 5 new actions in init()

## Key Decisions

1. **Used existing Action/Preflight interfaces**: The acceptance criteria mentioned a `SystemAction` interface, but issue #755 (which would define it for package installation actions) is still open. Used the established `Action` interface pattern instead - if #755 introduces a new interface, these actions can adopt it then.

2. **No host-side effects in Execute()**: Per the design doc, these actions at this phase do NOT execute on the host. Execute() displays what would be done and returns nil.

3. **Preflight() over Validate()**: The acceptance criteria mentioned `Validate() error` but the codebase uses `Preflight(params) *PreflightResult`. Used the established pattern.

4. **Full RequireCommandAction implementation**: Unlike the stub approach for other actions, `RequireCommandAction.Execute()` actually checks if commands exist and optionally verifies versions. This enables the `--verify` flag use case from #764.

## Trade-offs Accepted

- **Simple version comparison**: `versionMeetsMinimum()` does basic numeric comparison rather than full semver parsing. Sufficient for most use cases; can be enhanced later if needed.

- **No service state checking**: `ServiceStartAction` and `ServiceEnableAction` don't verify actual service state. They're documentation/sandbox actions, not host executors.

## Test Coverage

- New tests added: 55+ test cases covering:
  - All 5 action structs (Name, IsDeterministic, Preflight, Execute)
  - Parameter validation (required fields, type checking)
  - Group/service/command name validation
  - Version comparison logic
  - Action registration verification

## Known Limitations

1. Actions don't have `ImplicitConstraint()` method - that's for package manager actions only (per design doc D6)
2. No `Describe()` method yet - that's issue #763
3. Version comparison is basic (works for X.Y.Z format, may not handle all edge cases)

## Future Improvements

- Add `Describe()` method when #763 is implemented
- Enhance version comparison if needed (semver library)
- Add integration with sandbox container building when #765 is implemented
