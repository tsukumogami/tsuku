# Issue 1110 Implementation Plan

## Summary

Add `Libc []string` field to `WhenClause` struct and update all related methods (Matches, IsEmpty, UnmarshalTOML, ToMap) to support libc-conditional recipe steps on Linux systems. Include validation to ensure libc is only used when OS includes or implies Linux.

## Approach

Follow the established patterns from existing WhenClause fields (OS, Arch, LinuxFamily). The libc filter uses array syntax like OS and Platform (rather than string like Arch and LinuxFamily) because recipes may target both glibc and musl. Validation will be added to `ValidateStepsAgainstPlatforms()` since it already performs when clause validation.

### Alternatives Considered

- **Add WhenClause.Validate() method**: The design document shows a separate Validate() method, but the existing codebase uses `ValidateStepsAgainstPlatforms()` in platform.go for when clause validation. Keeping validation in one place is more maintainable.
- **String field instead of array**: Using `Libc string` would be simpler but less flexible. The array approach (`Libc []string`) matches the design doc and allows recipes to explicitly target both libc types in a single step if needed.

## Files to Modify

- `internal/recipe/types.go` - Add Libc field to WhenClause, update Matches(), IsEmpty(), UnmarshalTOML(), ToMap()
- `internal/recipe/platform.go` - Add libc validation to ValidateStepsAgainstPlatforms()
- `internal/recipe/when_test.go` - Add tests for libc matching behavior and IsEmpty() with libc
- `internal/recipe/types_test.go` - Add TOML parsing tests for libc field

## Files to Create

None.

## Implementation Steps

- [ ] Add `Libc []string` field to `WhenClause` struct in types.go (line 242)
- [ ] Update `IsEmpty()` method to check `len(w.Libc) == 0` (line 245-249)
- [ ] Update `Matches()` method to check libc filter when target OS is "linux" (line 254-299)
- [ ] Update `UnmarshalTOML()` to parse libc array using same pattern as OS (line 379-393)
- [ ] Update `ToMap()` to serialize libc field when non-empty (line 451-467)
- [ ] Add libc validation to `ValidateStepsAgainstPlatforms()` in platform.go:
  - Error if libc specified with OS that doesn't include "linux"
  - Error if libc values are not in `platform.ValidLibcTypes`
- [ ] Add unit tests for WhenClause.Matches() with libc filter in when_test.go
- [ ] Add unit tests for WhenClause.IsEmpty() with libc in when_test.go
- [ ] Add unit tests for TOML parsing of libc field in types_test.go
- [ ] Add unit tests for validation scenarios in when_test.go
- [ ] Run `go test ./internal/recipe/...` to verify all tests pass
- [ ] Run `go vet ./...` and `golangci-lint run --timeout=5m ./...`

## Testing Strategy

### Unit Tests

**Matching behavior (when_test.go):**
- Empty libc array matches all targets (glibc and musl)
- Libc = ["glibc"] matches glibc target on Linux, not musl
- Libc = ["musl"] matches musl target on Linux, not glibc
- Libc = ["glibc", "musl"] matches both
- Libc filter on non-Linux OS is ignored (darwin target always matches regardless of libc filter)
- Combined filters: OS = ["linux"], Libc = ["glibc"], Arch = "amd64"

**IsEmpty tests (when_test.go):**
- Clause with only libc is not empty
- Clause with empty libc array is still empty (if other fields empty)

**TOML parsing (types_test.go):**
- Parse `when = { libc = ["glibc"] }`
- Parse `when = { libc = ["glibc", "musl"] }`
- Parse `when = { os = ["linux"], libc = ["glibc"] }`
- Single string conversion: `when = { libc = "musl" }` becomes ["musl"]

**Validation (when_test.go):**
- Error: libc = ["glibc"] with os = ["darwin"]
- Error: libc = ["glibc"] with os = ["darwin", "windows"]
- Error: libc = ["invalid"]
- Valid: libc = ["glibc"] with os = ["linux"]
- Valid: libc = ["glibc"] with no OS specified (implies linux compatibility)
- Valid: libc = ["glibc"] with os = ["linux", "darwin"]

### Manual Verification

Run full test suite with `go test ./...` and verify no regressions.

## Risks and Mitigations

- **Breaking existing recipes**: Mitigated by ensuring empty libc array (default) matches all platforms, preserving backward compatibility.
- **Import cycle with platform package**: The validation needs `platform.ValidLibcTypes`. This is safe since recipe already imports platform (checked: no existing import, but platform.ValidLibcTypes is just a string slice, can duplicate if needed).
- **TOML parsing edge cases**: Follow existing patterns for OS array parsing which handle both array and single-string forms.

## Success Criteria

- [ ] `go test ./internal/recipe/...` passes with all new tests
- [ ] `go vet ./...` reports no issues
- [ ] `golangci-lint run --timeout=5m ./...` passes
- [ ] Existing recipe tests still pass (no regressions)
- [ ] Recipes with `when = { libc = ["glibc"] }` parse correctly
- [ ] Validation rejects libc with darwin-only OS
- [ ] Validation rejects invalid libc values

## Open Questions

None. The design document and implementation context provide clear guidance on all aspects.
