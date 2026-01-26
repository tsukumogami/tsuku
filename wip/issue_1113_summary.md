# Issue 1113 Summary

## What Was Implemented

Extended the platform constraint system with `SupportedLibc` and `UnsupportedReason` fields. Recipes can now restrict which libc implementations they support (glibc or musl) and provide explanations for their platform restrictions.

## Changes Made

- `internal/recipe/types.go`: Added `SupportedLibc []string` and `UnsupportedReason string` fields to MetadataSection
- `internal/recipe/platform.go`:
  - Added `SupportsPlatformWithLibc()` method for libc-aware platform checking
  - Updated `SupportsPlatformRuntime()` to detect and check libc at runtime
  - Updated `UnsupportedPlatformError` struct with `CurrentLibc`, `SupportedLibc`, and `UnsupportedReason` fields
  - Updated `Error()` to display libc constraints and reason
  - Updated `NewUnsupportedPlatformError()` to detect and populate libc
  - Updated `FormatPlatformConstraints()` to show libc constraints and reason
  - Updated `ValidatePlatformConstraints()` to validate libc values against `platform.ValidLibcTypes`
- `internal/recipe/platform_test.go`: Added tests for libc constraint functionality
- `cmd/tsuku/info.go`: Added `SupportedLibc` and `UnsupportedReason` to JSON output and updated `hasConstraints` check

## Key Decisions

- **Allowlist semantics**: Empty `supported_libc` means all libc types allowed; non-empty restricts to listed types. This matches existing `supported_os` and `supported_arch` patterns.
- **Linux-only constraint**: Libc constraints are only checked when the target OS is Linux. On Darwin and other platforms, the constraint is ignored.
- **Single reason field**: `UnsupportedReason` applies to all platform constraints rather than having separate reason fields per constraint type. Most recipes with restrictions have a single overarching explanation.

## Trade-offs Accepted

- **No libc suffix in GetSupportedPlatforms() output**: The platform tuples remain "os/arch" format. Libc info is shown separately in error messages and info output. Adding libc to tuples would change the format and potentially break consumers.

## Test Coverage

- New tests added: 4 test functions
  - `TestSupportsPlatformWithLibc` (10 cases)
  - `TestValidatePlatformConstraintsLibc` (6 cases)
  - `TestFormatPlatformConstraintsWithLibc` (4 cases)
  - `TestUnsupportedPlatformErrorWithLibc` (3 cases)
- All existing tests continue to pass

## Known Limitations

- Libc detection uses `platform.DetectLibc()` which has graceful fallback behavior. On unusual systems where detection fails, it defaults to "glibc".
- The constraint only distinguishes between "glibc" and "musl". Other libc implementations (uclibc, bionic) aren't supported.

## Future Improvements

- Consider adding container integration tests that verify libc detection and constraint checking on real Alpine (musl) containers.
