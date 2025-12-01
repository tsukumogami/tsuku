# Issue 144 Implementation Plan

## Summary

Add Perl integration tests to the test matrix and verify end-to-end Perl tool installation with cpan_install action.

## Approach

Follow the existing integration test pattern: add Perl test cases to test-matrix.json, which are picked up by the Docker-based integration test framework.

### Alternatives Considered
- **Separate Perl-specific test file**: Rejected for consistency with existing pattern
- **Non-Docker local tests**: Rejected because Docker provides isolation and consistent environment

## Files to Modify
- `test-matrix.json` - Add Perl test cases

## Implementation Steps
- [ ] Add T50: perl runtime test (tier 2, basic runtime installation)
- [ ] Add T51: ack test (tier 5, cpan_install with App-Ack distribution)
- [ ] Add Perl tests to CI linux array
- [ ] Verify test-matrix.json is valid JSON
- [ ] Run integration tests locally to verify

Mark each step [x] after it is implemented and committed.

## Test Cases

### T50: perl (Perl Runtime)
- Tool: perl
- Tier: 2 (like other runtimes: golang, nodejs, rust)
- Features: action:download_archive, install_mode:directory, bootstrap:relocatable_perl
- Description: relocatable perl runtime

### T51: ack (CPAN Tool)
- Tool: ack (grep-like text finder)
- Tier: 5 (like other package manager installs)
- Features: action:cpan_install, version_detection:metacpan
- Description: cpan_install with dependency

Note: The acceptance criteria mentions multiple tests but the key scenarios can be verified with these two tests:
- T50 verifies Perl installation works
- T51 verifies cpan_install action, dependency resolution, wrapper script generation, and PERL5LIB isolation

## Testing Strategy
- Run integration tests with `-tool=perl` and `-tool=ack` flags
- Verify both tests pass in Docker container
- Verify CI includes the new tests

## Risks and Mitigations
- **Large download size (~50MB perl)**: Acceptable for integration tests; CI has good bandwidth
- **CPAN network dependency**: MetaCPAN is reliable; same risk as other package manager tests

## Success Criteria
- [ ] test-matrix.json has perl and ack test cases
- [ ] `go test -tags=integration -tool=perl` passes
- [ ] `go test -tags=integration -tool=ack` passes
- [ ] CI linux array includes new tests
