# Issue #1113 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-platform-compatibility-verification.md`
- Sibling issues reviewed: #1109 (libc detection), #1110 (libc filter), #1111 (step-level deps), #1112 (enhanced *_install)
- Prior patterns identified: Platform constraint fields in MetadataSection, SupportsPlatform()/SupportsPlatformRuntime() in platform.go, FormatPlatformConstraints() for display

## Gap Analysis

### Minor Gaps

1. **Error type extension**: The issue doesn't mention updating `UnsupportedPlatformError` struct to include libc fields. Prior sibling issues established the pattern in `internal/recipe/platform.go`:
   ```go
   type UnsupportedPlatformError struct {
       SupportedLibc      []string  // NEW
       UnsupportedReason  string    // NEW
       // ... existing fields
   }
   ```

2. **JSON output in `tsuku info`**: The `infoOutput` struct in `cmd/tsuku/info.go` should include `SupportedLibc` and `UnsupportedReason` fields for JSON output consistency with existing `SupportedOS`/`SupportedArch` pattern.

3. **hasConstraints check update**: In `cmd/tsuku/info.go` line 163-165, the `hasConstraints` condition needs to include `len(r.Metadata.SupportedLibc) > 0` to trigger display.

4. **ValidatePlatformConstraints() update**: The validation method in `platform.go` should validate libc constraints (valid values, semantic consistency).

5. **TOML serialization in ToTOML()**: The `Recipe.ToTOML()` method in `types.go` doesn't serialize metadata fields. While this may be intentional (metadata section uses direct toml.Encode), verify this works for new fields.

### Moderate Gaps

None identified. All required locations and patterns are established by prior issues.

### Major Gaps

None identified. The issue spec is complete for implementing the core functionality.

## Recommendation

**Proceed**

The issue specification is complete. The minor gaps are resolvable from examining existing code patterns without requiring user input or issue amendments.

## Implementation Context

The following patterns from completed sibling issues should be followed:

1. **Field location**: Add to `MetadataSection` in `internal/recipe/types.go` (per existing `SupportedOS`, `SupportedArch` pattern)

2. **Libc validation**: Use `platform.ValidLibcTypes` slice (established in #1109) for value validation

3. **SupportsPlatform() extension**: Follow existing pattern of checking allowlist, then denylist. Libc is an additional filter dimension applied after OS/arch checks.

4. **Runtime detection**: Use `platform.DetectLibc()` (established in #1109) when checking at runtime

5. **Error messages**: Follow the existing `UnsupportedPlatformError.Error()` pattern - show constraints, then suggestion

6. **Display format**: Follow `FormatPlatformConstraints()` pattern for `tsuku info` output

## Files to Modify

Based on sibling issue patterns:
- `internal/recipe/types.go` - Add SupportedLibc, UnsupportedReason to MetadataSection
- `internal/recipe/platform.go` - Update SupportsPlatform(), GetSupportedPlatforms(), FormatPlatformConstraints(), UnsupportedPlatformError, ValidatePlatformConstraints()
- `internal/recipe/platform_test.go` - Add tests for libc constraint validation
- `cmd/tsuku/info.go` - Update hasConstraints check and JSON output struct
- `cmd/tsuku/install.go` - Pass reason through error chain (if needed)
