# Issue 130 Implementation Plan

## Summary

Create a `cpan_install` action that installs Perl distributions via cpanm with local::lib isolation, following the gem_install pattern with wrapper scripts that set PERL5LIB at runtime.

## Approach

The cpan_install action will mirror the gem_install implementation:
1. Validate distribution name and version
2. Locate cpanm from installed Perl dependency
3. Run cpanm with `--local-lib` for per-tool isolation
4. Generate bash wrapper scripts that set PERL5LIB before execution
5. Rename original cpanm scripts to `.cpanm` suffix

### Alternatives Considered

- **System Perl with cpanm**: Rejected because it violates tsuku's self-contained philosophy
- **Hidden Perl bootstrap**: Rejected for inconsistency with go/npm patterns (explicit dependencies)

## Files to Modify

- `internal/actions/action.go` - Register CpanInstallAction in init()

## Files to Create

- `internal/actions/cpan_install.go` - Main implementation
- `internal/actions/cpan_install_test.go` - Unit tests

## Implementation Steps

- [ ] Create cpan_install.go with CpanInstallAction struct
- [ ] Implement distribution name validation (isValidDistribution)
- [ ] Implement version validation (isValidCpanVersion)
- [ ] Implement executable name validation (reuse pattern from gem_install)
- [ ] Add ResolvePerl() and ResolveCpanm() helper functions to util.go
- [ ] Implement Execute() method with cpanm invocation
- [ ] Implement wrapper script generation
- [ ] Register action in action.go
- [ ] Create comprehensive unit tests
- [ ] Verify all tests pass

## Testing Strategy

### Unit Tests
- Test isValidDistribution() with valid/invalid distribution names
- Test isValidCpanVersion() with valid/invalid version formats
- Test validation error paths in Execute()
- Test action.Name() returns "cpan_install"

### Manual Verification
- Once Perl recipe exists in registry, create manual test recipe:
  ```toml
  [metadata]
  name = "ack"
  dependencies = ["perl"]

  [[steps]]
  action = "cpan_install"
  distribution = "App-Ack"
  executables = ["ack"]
  ```

## Risks and Mitigations

- **Perl not installed**: Execute will fail gracefully with clear error message
- **cpanm failure**: Surface cpanm stderr to user for debugging
- **Bash not available**: Verify /bin/bash exists before creating wrappers

## Success Criteria

- [ ] All existing tests pass (no regressions)
- [ ] cpan_install action registered and accessible via Get("cpan_install")
- [ ] Distribution name validation rejects shell metacharacters
- [ ] Version validation rejects shell metacharacters
- [ ] Executable name validation rejects path separators and shell chars
- [ ] Wrapper scripts use PERL5LIB for module isolation
- [ ] Code follows existing patterns (gem_install reference)

## Open Questions

None - design document provides complete specification.
