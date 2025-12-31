# Issue 754 Implementation Plan

## Summary

Create a new `Target` struct in `internal/platform/target.go` representing the platform+linux_family tuple for plan generation, with helper methods to parse OS and Arch from the combined platform string.

## Approach

The `Target` struct is distinct from the existing `executor.Platform` struct. While `executor.Platform` has separate `OS` and `Arch` fields for static plan JSON data, the new `Target` uses a combined `Platform` string ("linux/amd64") and adds `LinuxFamily` for targeting PM-specific actions during plan generation.

### Alternatives Considered

- **Extend executor.Platform**: Adding `LinuxFamily` to the existing struct would mix targeting (dynamic) with plan data (static). The design doc explicitly separates these concerns.
- **Use separate OS/Arch fields**: The combined format "linux/amd64" is cleaner for targeting and matches the design doc examples.

## Files to Create

- `internal/platform/target.go` - Target struct definition with OS(), Arch() methods
- `internal/platform/target_test.go` - Unit tests for Target struct

## Implementation Steps

- [ ] Create `internal/platform/` directory
- [ ] Implement `Target` struct with `Platform` and `LinuxFamily` fields
- [ ] Implement `OS()` method - parse OS from "linux/amd64" format
- [ ] Implement `Arch()` method - parse Arch from "linux/amd64" format
- [ ] Add package documentation explaining the purpose
- [ ] Write unit tests for Target struct

## Testing Strategy

- Unit tests:
  - `OS()` parsing: "linux/amd64" -> "linux", "darwin/arm64" -> "darwin"
  - `Arch()` parsing: "linux/amd64" -> "amd64", "darwin/arm64" -> "arm64"
  - Edge cases: empty platform, invalid format
  - LinuxFamily values: verify valid families are accepted
  - LinuxFamily is empty for non-Linux platforms

## Risks and Mitigations

- **Edge case: malformed platform string**: Return empty string for OS/Arch if format is invalid (no slash). Document behavior in method comments.
- **Future extension**: Keep struct simple now; additional validation can be added in #759 (linux_family detection).

## Success Criteria

- [ ] `internal/platform/target.go` exists with `Target` struct
- [ ] `Target` has `Platform string` and `LinuxFamily string` fields
- [ ] `OS()` and `Arch()` helper methods work correctly
- [ ] LinuxFamily is documented to be empty for non-Linux platforms
- [ ] Valid LinuxFamily values documented: debian, rhel, arch, alpine, suse
- [ ] All unit tests pass
- [ ] `go vet ./...` and `go build ./...` succeed

## Open Questions

None - the design doc provides clear specification.
