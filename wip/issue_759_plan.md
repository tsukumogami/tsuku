# Issue 759 Implementation Plan

## Summary

Implement `/etc/os-release` parsing and linux_family detection in `internal/platform/family.go`, with `DetectFamily()` and `DetectTarget()` functions.

## Approach

Follow the design doc closely. Parse `/etc/os-release` for ID and ID_LIKE fields, map to family via lookup table, fall back to ID_LIKE chain. Handle missing file gracefully.

### Alternatives Considered

- **Binary detection only (microdnf/dnf/apt)**: More robust for containers but less informative - can't distinguish between distros in same family. Keep binary detection as supplementary for RHEL.
- **Runtime parsing only**: The design doc clearly specifies a lookup table approach with ID_LIKE fallback, which is deterministic and testable.

## Files to Create

- `internal/platform/family.go` - Detection functions
- `internal/platform/family_test.go` - Unit tests
- `internal/platform/testdata/os-release/` - Fixture files for testing

## Implementation Steps

- [ ] Create OSRelease struct for parsed data
- [ ] Implement ParseOSRelease() to parse /etc/os-release format
- [ ] Add distroToFamily mapping table
- [ ] Implement MapDistroToFamily() for ID -> family lookup with ID_LIKE fallback
- [ ] Implement DetectFamily() wrapping the above
- [ ] Implement DetectTarget() using runtime.GOOS/GOARCH + DetectFamily()
- [ ] Create test fixtures for ubuntu, debian, fedora, arch, alpine, rocky
- [ ] Write unit tests for all functions

## Testing Strategy

- Unit tests:
  - ParseOSRelease() with fixture files
  - MapDistroToFamily() with direct ID, ID_LIKE fallback, unknown distro
  - DetectTarget() for Linux and non-Linux platforms
- Test fixtures from real distros copied from actual /etc/os-release files

## Risks and Mitigations

- **Missing /etc/os-release**: Return empty family + nil error per issue spec
- **Unknown distro**: Return error with distro name for debugging
- **ID_LIKE with multiple values**: Parse as space-separated list, try each in order

## Success Criteria

- [ ] DetectFamily() returns correct family for all test fixtures
- [ ] DetectTarget() returns Target with correct Platform and LinuxFamily
- [ ] Non-Linux platforms return Target with empty LinuxFamily
- [ ] Missing file returns empty family without error
- [ ] All unit tests pass
- [ ] go vet and go build succeed

## Open Questions

None - design doc provides complete specification.
