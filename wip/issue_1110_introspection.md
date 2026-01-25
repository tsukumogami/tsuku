# Issue 1110 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-platform-compatibility-verification.md`
- Sibling issues reviewed: #1109 (feat(platform): add libc detection for glibc vs musl)
- Prior patterns identified:
  - `Libc()` method added to `Matchable` interface (internal/recipe/types.go line 194)
  - `libc` field added to `Target` struct (internal/platform/target.go line 36)
  - `NewMatchTarget()` accepts libc parameter (internal/recipe/types.go line 206)
  - `MatchTarget` has `libc` field and `Libc()` accessor (internal/recipe/types.go lines 203, 225)
  - Existing WhenClause fields use array syntax for filtering (Platform, OS)
  - WhenClause uses string for single-value fields (Arch, LinuxFamily, PackageManager)
  - Test patterns established in when_test.go and types_test.go

## Gap Analysis

### Minor Gaps

1. **WhenClause.IsEmpty() needs libc check**: The issue mentions updating IsEmpty() but the design doc shows the specific check needed (`len(w.Libc) == 0`). The existing test file (when_test.go) has tests for IsEmpty() that should be extended.

2. **TOML parsing for libc array**: The UnmarshalTOML in types.go already handles array parsing for Platform and OS fields (lines 365-393). The same pattern should be used for libc - check for both []interface{} and single string forms.

3. **ToMap() serialization**: Step.ToMap() (lines 437-476) needs to include libc when serializing WhenClause back to TOML.

4. **wip/IMPLEMENTATION_CONTEXT.md exists**: A context file was already created at `wip/IMPLEMENTATION_CONTEXT.md` that summarizes the key design points. This should be used during implementation.

5. **platform/libc.go has ValidLibcTypes**: The ValidLibcTypes constant in internal/platform/libc.go (line 14) lists valid values as ["glibc", "musl"]. Validation should use this constant rather than hardcoding values.

### Moderate Gaps

None identified. The issue spec is complete and aligns with the design document.

### Major Gaps

None identified. The design document was updated after #1109 closed (PR #1122 marked it done), and the implementation context file captures all needed details.

## Recommendation

**Proceed**

The issue spec is complete. All acceptance criteria are explicit and align with the design document. The foundational work from #1109 (Matchable interface, MatchTarget, Target struct) is in place and ready for use.

## Implementation Notes

Key files to modify:
- `internal/recipe/types.go` - Add Libc field to WhenClause, update Matches(), IsEmpty(), UnmarshalTOML, ToMap()
- `internal/recipe/when_test.go` - Add tests for libc filtering behavior
- `internal/recipe/types_test.go` - Add TOML parsing tests for libc field
- `internal/recipe/platform.go` - Consider if ValidateStepsAgainstPlatforms needs libc validation

Existing patterns to follow:
- OS array filtering pattern (when_test.go lines 59-165)
- Arch/LinuxFamily dimension filtering (when_test.go lines 178-275)
- TOML array parsing pattern (types.go lines 365-393)

The acceptance criterion to "Error if libc specified with os = ["darwin"]" should be implemented in ValidateStepsAgainstPlatforms() or a new WhenClause.Validate() method. The design doc shows this validation explicitly.
