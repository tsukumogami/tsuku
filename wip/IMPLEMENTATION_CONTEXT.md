---
summary:
  constraints:
    - Follow existing platform constraint patterns (supported_os, supported_arch, unsupported_platforms)
    - SupportedLibc is an allowlist (empty means all allowed, non-empty means only listed types)
    - Only valid libc values are "glibc" and "musl"
    - UnsupportedReason applies to ALL platform constraints, not just libc
  integration_points:
    - internal/recipe/types.go - MetadataSection struct (add SupportedLibc, UnsupportedReason fields)
    - internal/recipe/platform.go - SupportsPlatform(), SupportsPlatformRuntime(), GetSupportedPlatforms()
    - internal/recipe/platform.go - FormatPlatformConstraints() for display
    - cmd/tsuku/info.go - Display constraints including reason in tsuku info output
    - Runtime error messages when installing unsupported libc
  risks:
    - Ensure libc constraint validation mirrors existing OS/arch constraint patterns
    - Must integrate with existing Matchable interface which includes Libc() method (from #1110)
    - Need to update all places that format/display platform constraints
  approach_notes: |
    This is a straightforward extension of the existing platform constraint system.
    The design doc clearly specifies the new fields and their semantics.
    Key patterns to follow:
    - SupportedLibc []string like SupportedOS/SupportedArch
    - UnsupportedReason string (single field for all constraints)
    - Empty constraint = all allowed, non-empty = allowlist
    - Display in tsuku info with reason when present
    - Include reason in runtime errors
---

# Implementation Context: Issue #1113

**Source**: docs/designs/DESIGN-platform-compatibility-verification.md

## Key Design Decisions

### New Fields in MetadataSection

```go
SupportedLibc      []string `toml:"supported_libc,omitempty"`   // Allowed libc (default: all)
UnsupportedReason  string   `toml:"unsupported_reason,omitempty"` // Explanation for constraints
```

### Semantics

- Empty `supported_libc` = all libc types allowed (glibc and musl)
- Non-empty = only listed types allowed (allowlist)
- Valid values: "glibc", "musl"
- `unsupported_reason` applies to ALL platform constraints (os, arch, libc)

### User-Facing Display

`tsuku info` output:
```
Name: some-glibc-only-tool
Platforms: linux (glibc only), darwin
Constraints:
  - Libc: glibc only
  - Reason: Upstream only provides glibc binaries
```

### Runtime Error

When installing on unsupported libc:
```
Error: some-glibc-only-tool is not available for linux/musl

Platform constraints:
  Supported libc: glibc
  Reason: Upstream only provides glibc binaries

Suggestion: Check if upstream has added musl support, or use an alternative tool.
```

## Dependencies

- #1110 (libc filter) - DONE - Added Libc() to Matchable interface
- Builds on existing platform constraint system (SupportedOS, SupportedArch, UnsupportedPlatforms)
