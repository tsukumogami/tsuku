# Issue 1113 Implementation Plan

## Summary

Add `SupportedLibc` and `UnsupportedReason` fields to the MetadataSection struct, update platform constraint functions to incorporate libc checking, and enhance display/error formatting to show reasons.

## Approach

This extends the existing platform constraint system following the established patterns for `SupportedOS`, `SupportedArch`, and `UnsupportedPlatforms`. The libc constraint uses the same allowlist semantics (empty = all allowed, non-empty = only listed types). The reason field applies to all constraints uniformly.

### Alternatives Considered

- **Separate reason fields per constraint type**: Rejected because it adds complexity for rare use cases. Most recipes that need a reason have a single overarching explanation for all their constraints.
- **Libc as part of denylist tuples (e.g., "linux/amd64/musl")**: Rejected because it would require changing the existing tuple format and doesn't match the allowlist pattern used for OS/arch.

## Files to Modify

- `internal/recipe/types.go` - Add `SupportedLibc []string` and `UnsupportedReason string` fields to MetadataSection
- `internal/recipe/platform.go` - Update `SupportsPlatform()`, `SupportsPlatformRuntime()`, add new `SupportsPlatformWithLibc()`, update `UnsupportedPlatformError`, `NewUnsupportedPlatformError()`, `GetSupportedPlatforms()`, `FormatPlatformConstraints()`, and `ValidatePlatformConstraints()`
- `internal/recipe/platform_test.go` - Add tests for libc constraints and reason display
- `cmd/tsuku/info.go` - Update display to show libc constraints and reason

## Files to Create

None - all changes fit within existing files.

## Implementation Steps

- [ ] 1. Add new fields to MetadataSection in `types.go`
  - Add `SupportedLibc []string` with TOML tag `supported_libc,omitempty`
  - Add `UnsupportedReason string` with TOML tag `unsupported_reason,omitempty`

- [ ] 2. Add `SupportsPlatformWithLibc()` method to `platform.go`
  - New method accepting os, arch, and libc parameters
  - Check existing OS/arch constraints first
  - If `SupportedLibc` is empty, all libc types allowed (pass)
  - If non-empty, check if target libc is in the allowlist
  - Return false if libc constraint fails

- [ ] 3. Update `SupportsPlatformRuntime()` to use libc detection
  - Call `platform.DetectLibc()` to get current system's libc
  - Call `SupportsPlatformWithLibc(runtime.GOOS, runtime.GOARCH, libc)`

- [ ] 4. Update `UnsupportedPlatformError` struct
  - Add `CurrentLibc string` field
  - Add `SupportedLibc []string` field
  - Add `UnsupportedReason string` field
  - Update `Error()` method to display libc constraints and reason

- [ ] 5. Update `NewUnsupportedPlatformError()` method
  - Detect current libc using `platform.DetectLibc()`
  - Populate new fields in returned error

- [ ] 6. Update `GetSupportedPlatforms()` to incorporate libc
  - If `SupportedLibc` is non-empty, generate tuples with libc suffix (for JSON output clarity)
  - Consider returning a struct with separate libc info vs modifying tuple format

- [ ] 7. Update `FormatPlatformConstraints()` for libc display
  - Add "Libc: glibc" or "Libc: glibc, musl" section when constraints present
  - Add reason display at the end when `UnsupportedReason` is set

- [ ] 8. Update `ValidatePlatformConstraints()` to validate libc values
  - Check that all values in `SupportedLibc` are in `platform.ValidLibcTypes`
  - Return error for invalid libc values

- [ ] 9. Update `cmd/tsuku/info.go` display
  - JSON output: Add `SupportedLibc` and `UnsupportedReason` fields to output struct
  - Text output: Show libc constraints and reason in platform constraints section

- [ ] 10. Add unit tests in `platform_test.go`
  - `TestSupportsPlatformWithLibc` - test libc constraint checking
  - `TestSupportsPlatformRuntimeWithLibc` - test runtime detection integration
  - `TestValidatePlatformConstraintsLibc` - test libc value validation
  - `TestGetSupportedPlatformsWithLibc` - test platform list with libc constraints
  - `TestFormatPlatformConstraintsLibc` - test display formatting with libc and reason
  - `TestUnsupportedPlatformErrorLibc` - test error message includes libc and reason

- [ ] 11. Run validation script from issue
  - Execute `go test -v ./internal/recipe/... -run TestPlatform`
  - Create test recipe with libc constraint and verify parsing
  - Test `tsuku info` output shows constraints correctly

## Testing Strategy

- **Unit tests**: Cover all new methods and updated methods with libc constraint variations
  - Empty `supported_libc` (all allowed)
  - Single libc constraint (`["glibc"]`)
  - Both libc types (`["glibc", "musl"]`)
  - Invalid libc values
  - Reason display in errors and formatting

- **Integration tests**: Use the validation script from the issue
  - Create test recipe with `supported_libc = ["glibc"]` and `unsupported_reason`
  - Verify `tsuku validate-recipe` passes
  - Verify `tsuku info` displays constraints

- **Manual verification**: Test on actual glibc and musl systems if available

## Risks and Mitigations

- **Breaking existing platform checks**: The new libc check is additive. Empty `SupportedLibc` defaults to all allowed, preserving existing behavior. All existing tests should continue to pass.

- **Libc detection failure**: `platform.DetectLibc()` has fallback behavior (defaults to "glibc"). Runtime checks will work even if detection fails, though may produce incorrect results on edge cases.

- **Display format changes**: The `FormatPlatformConstraints()` output format change could affect tools parsing this output. Since this is meant for human consumption, format changes are acceptable. JSON output provides stable structure.

## Success Criteria

- [ ] `SupportedLibc []string` field added to MetadataSection and TOML parsing works
- [ ] `UnsupportedReason string` field added to MetadataSection and TOML parsing works
- [ ] `SupportsPlatform()`/`SupportsPlatformRuntime()` check libc constraints
- [ ] `GetSupportedPlatforms()` incorporates libc constraints
- [ ] `FormatPlatformConstraints()` displays libc constraints and reason
- [ ] `tsuku info <recipe>` displays constraints including reason
- [ ] Runtime error when installing unsupported libc includes reason
- [ ] Unit tests cover libc constraint validation
- [ ] Unit tests verify constraint display formatting
- [ ] Validation script from issue runs successfully

## Open Questions

None - the design document and implementation context provide clear guidance on all aspects.
