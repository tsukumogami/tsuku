# Issue 794 Implementation Plan

## Summary

Add integration tests for CLI system dependency instruction display using testdata fixtures and the existing `--target-family` flag for platform-specific testing.

## Approach

The implementation will create a new test file `cmd/tsuku/install_sysdeps_integration_test.go` with table-driven tests that:
1. Use existing testdata recipes (build-tools-system.toml, ca-certs-system.toml, ssl-libs-system.toml)
2. Capture CLI output via a test helper that executes the install command
3. Validate output contains expected instructions for different target families
4. Test the `--quiet` flag suppresses instruction output

This approach follows the existing test patterns in `cmd/tsuku/install_test.go` and `cmd/tsuku/sysdeps_test.go` (unit tests for helper functions).

### Alternatives Considered

- **Alternative 1: Mock-based unit tests**: Would require significant refactoring to inject mocks into the install command flow. Rejected because integration tests provide better coverage of the full display logic.
- **Alternative 2: End-to-end Docker tests**: Would be more comprehensive but slower and harder to debug. Rejected because the `--target-family` flag already enables platform-specific testing without Docker.

## Files to Create

- `cmd/tsuku/install_sysdeps_integration_test.go` - Integration tests for system dependency instruction display

## Implementation Steps

- [ ] Create test file with helper to capture CLI output
- [ ] Add test case for default platform detection (shows current platform instructions)
- [ ] Add test case for `--target-family debian` (shows apt commands)
- [ ] Add test case for `--target-family rhel` (shows dnf commands)
- [ ] Add test case for `--quiet` flag (suppresses instruction output)
- [ ] Add test case verifying all expected instruction components appear
- [ ] Run tests and verify they pass

## Testing Strategy

The tests themselves are the testing strategy - they validate:
- CLI correctly filters steps by target family
- Instruction output includes all expected components (package names, commands)
- Platform-specific display names appear correctly
- `--quiet` flag suppresses output
- Multiple testdata recipes are exercised

Manual verification:
```bash
# Run new integration tests
go test -v ./cmd/tsuku -run TestInstallSystemDeps

# Verify with actual recipes
./tsuku install --recipe testdata/recipes/build-tools-system.toml --target-family debian
./tsuku install --recipe testdata/recipes/ca-certs-system.toml --target-family rhel --quiet
```

## Success Criteria

- [ ] All acceptance criteria from issue #794 are met
- [ ] Tests use testdata fixtures (build-tools-system.toml, ca-certs-system.toml)
- [ ] Tests verify output for debian and rhel families
- [ ] Test verifies `--quiet` suppresses instruction output
- [ ] Tests pass in CI
- [ ] No regression in existing tests

## Open Questions

None - the approach is straightforward and builds on existing test infrastructure.
