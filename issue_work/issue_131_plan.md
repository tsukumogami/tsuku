# Issue 131 Implementation Plan

## Summary

Implement `internal/builders/cpan.go` that generates recipes for CPAN distributions, following the pattern established by gem_builder and other ecosystem builders.

## Approach

Follow the existing GemBuilder pattern: create a CPANBuilder struct with an HTTP client for MetaCPAN API calls, implement Builder interface (Name, CanBuild, Build), and use distribution name heuristics for executable inference.

### Alternatives Considered
- **Fetch and parse Makefile.PL for executables**: More accurate but requires downloading/extracting tarballs - rejected for complexity and speed
- **Query MetaCPAN /_search for script files**: Adds API complexity and may not work for all distributions - rejected for reliability concerns

## Files to Create
- `internal/builders/cpan.go` - CPAN builder implementation
- `internal/builders/cpan_test.go` - Comprehensive tests

## Implementation Steps
- [x] Create CPANBuilder struct with HTTP client and metacpanBaseURL
- [x] Implement Name() returning "cpan"
- [x] Implement CanBuild() that queries MetaCPAN to verify distribution exists
- [x] Implement Build() that fetches metadata and generates recipe
- [x] Add distribution name validation (isValidDistribution)
- [x] Add module-to-distribution normalization (App::Ack -> App-Ack)
- [x] Add executable name inference (App-Ack -> ack, Perl-Critic -> perlcritic)
- [x] Add comprehensive unit tests with mock HTTP server
- [x] Verify tests pass and coverage is adequate

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy
- Unit tests: Mock MetaCPAN API responses using httptest
- Test Name(), CanBuild() for valid/invalid/not-found distributions
- Test Build() for recipe generation with proper metadata
- Test distribution name validation
- Test module-to-distribution normalization
- Test executable name inference heuristics

## Key Design Decisions

### Distribution Name Validation
Use same regex as provider_metacpan.go: `^[A-Za-z][A-Za-z0-9]*(-[A-Za-z0-9]+)*$`

### Executable Inference
Transform distribution name to executable:
1. Remove "App-" prefix if present
2. Convert to lowercase
3. Replace remaining hyphens with empty string
Example: App-Ack -> ack, Perl-Critic -> perlcritic

### Recipe Structure
```toml
[metadata]
name = "<executable>"
description = "<abstract from MetaCPAN>"
homepage = "https://metacpan.org/dist/<distribution>"
dependencies = ["perl"]

[version]
source = "metacpan"
distribution = "<distribution>"

[[steps]]
action = "cpan_install"
distribution = "<distribution>"
executables = ["<executable>"]

[verify]
command = "<executable> --version"
```

## Risks and Mitigations
- **Executable inference may be wrong**: Add warning when inference is uncertain (e.g., when distribution doesn't follow App-* pattern)
- **MetaCPAN API changes**: Use same pattern as existing metacpan.go; changes would affect version provider too

## Success Criteria
- [ ] CPANBuilder implements Builder interface
- [ ] `tsuku create App-Ack --from cpan` generates working recipe
- [ ] Module names with `::` are normalized to distribution format
- [ ] Unit tests achieve >80% coverage
- [ ] All existing tests continue to pass
