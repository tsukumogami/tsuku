# Issue #759 Introspection

## Context Reviewed

- **Design doc**: docs/DESIGN-system-dependency-actions.md (updated 2025-12-31 16:38 by PR #777)
- **Sibling issues reviewed**: #754 (Target struct - CLOSED, merged via PR #777)
- **Prior patterns identified**:
  - Target struct established in internal/platform/target.go (from #754)
  - Platform package convention established with proper test structure
  - ValidLinuxFamilies constant already defined: ["debian", "rhel", "arch", "alpine", "suse"]

## Gap Analysis

### Minor Gaps Identified

1. **Missing error handling specification for missing `/etc/os-release`**
   - Issue AC states: "Handles missing file gracefully (returns empty, no error)"
   - Design doc (section D2) states different failure mode: "If `/etc/os-release` is missing or family cannot be determined, steps with `linux_family` conditions are skipped"
   - Current AC expectation doesn't align with design intent
   - **Resolution**: Clarify whether DetectFamily() should:
     - Option A: Return ("", nil) on missing file (current AC)
     - Option B: Return ("", error) on missing file but allow graceful degradation elsewhere (design intent)

2. **ParseOSRelease() helper function not in scope**
   - Design doc and issue both reference parsing `/etc/os-release`
   - Issue AC only mentions: DetectFamily(), DetectTarget(), distroToFamily mapping, fallback logic
   - **Who builds it**: Not explicitly stated if parsing is in this issue or deferred
   - **Resolution**: Clarify scope - is ParseOSRelease() in #759 or considered a prerequisite?

3. **microdnf detection implementation approach unclear**
   - Issue AC mentions: "For RHEL: detects microdnf as equivalent to dnf"
   - Design doc (D2) notes: "check for microdnf in addition to dnf" and "Detection order: dnf > microdnf > yum"
   - Design shows this as part of detection mechanism but issue AC doesn't specify where/when this check happens
   - **Question**: Is microdnf detection part of DetectFamily() or a separate helper?
   - **Resolution**: Should be clarified in implementation notes

4. **Design doc shows MapDistroToFamily() helper function**
   - Design doc example (line 619) shows: `return MapDistroToFamily(osRelease.ID, osRelease.IDLike)`
   - Issue AC references: "Maps distro IDs to families via distroToFamily lookup table" (the constant)
   - These are two different levels of abstraction
   - **Resolution**: Clarify if MapDistroToFamily() is a public function or internal helper

### Moderate Gaps Identified

1. **DetectTarget() error handling for non-Linux is incomplete**
   - Issue AC: `DetectTarget() (Target, error)`
   - Design doc shows: Returns Target with empty LinuxFamily for non-Linux (no error)
   - But current AC doesn't specify this non-error case
   - **Impact on implementation**: Affects error handling in calling code
   - **Proposed amendment**: Clarify that DetectTarget() returns error only when:
     - On Linux AND family detection fails (cannot determine distro)
     - NOT when on non-Linux platforms (returns valid Target with empty LinuxFamily)

2. **No specification for handling unrecognized distros**
   - Design doc example returns: `fmt.Errorf("unknown distro: %s", osRelease.ID)`
   - Issue AC states: "Falls back to ID_LIKE chain if ID not in table"
   - But what if both ID and entire ID_LIKE chain fail to match anything?
   - **Current spec**: Returns error with "unknown distro"
   - **Question**: Should this be tested? Which unrecognized distros should be in fixtures?
   - **Proposed amendment**: Unit tests should include at least one fixture for unrecognized distro to validate error handling

### Major Gaps Identified

None identified. The issue is well-scoped and the design doc provides sufficient detail.

## Dependency Analysis

**Blocking dependency (#754) is resolved:**
- Target struct merged via PR #777 (2025-12-31)
- ValidLinuxFamilies constant available for use
- internal/platform/ package structure established

**This issue unblocks:**
- #760: Implicit constraints (depends on linux_family detection)
- #761: Plan filtering (depends on DetectTarget output)
- #766: CLI instructions (depends on family detection for formatting)

## Recommendation

**Proceed with Moderate Gap Clarifications**

The issue is ready to implement with clarifications on:
1. Error handling behavior when `/etc/os-release` is missing
2. Scope of ParseOSRelease() helper (in this issue or prerequisite?)
3. microdnf detection integration point
4. Unrecognized distro handling in tests

## Proposed Amendments

Based on design doc patterns and #754 implementation:

### Amendment 1: Clarify Error Handling for DetectFamily()
On Linux systems where `/etc/os-release` is missing or unreadable, DetectFamily() should:
- First attempt to parse `/etc/os-release`
- If file missing/unreadable and we're on Linux: return error (cannot determine family on Linux)
- This allows graceful degradation at higher levels (CLI or sandbox can log warning and proceed)

**Rationale**: Design doc's "graceful degradation" happens in the calling code (plan filtering), not in DetectFamily() itself. The detection function should report when it cannot determine a required value.

### Amendment 2: Add Unit Test for Unrecognized Distro
Include fixture `os-release.unknown` with `ID=notareallinux` to test:
- Unknown distro ID with no ID_LIKE fallback → returns error
- Error message format: `fmt.Errorf("unknown distro: %s", osRelease.ID)`

### Amendment 3: ParseOSRelease() Scope Declaration
Add note to AC:
- ParseOSRelease() helper function: internal to family.go (not exported)
- Accepts path parameter to enable testing with fixture files
- Signature: `func ParseOSRelease(path string) (*OSRelease, error)`

## Implementation Notes from Design Review

Key details from design doc that should guide implementation:

1. **distroToFamily mapping** (design doc lines 210-224):
   - Debian family: debian, ubuntu, linuxmint, pop, elementary, zorin
   - RHEL family: fedora, rhel, centos, rocky, almalinux, ol
   - Arch family: arch, manjaro, endeavouros
   - Alpine family: alpine
   - SUSE family: opensuse, opensuse-leap, opensuse-tumbleweed, sles

2. **Test fixtures required** (design doc + issue AC):
   - ubuntu (ID="ubuntu", ID_LIKE=["debian"])
   - debian (ID="debian")
   - fedora (ID="fedora")
   - arch (ID="arch")
   - alpine (ID="alpine")
   - rocky (ID="rocky")

3. **Detection strategy** (design doc D2, lines 256-260):
   - Use `exec.LookPath()` not `which` for binary detection
   - For RHEL: check dnf > microdnf > yum (prefer modern)
   - HOWEVER: This appears to be future-work contextual note, not required for DetectFamily() in #759

## Cross-Reference Validation

- **vs #754 (Target struct)**: No conflicts. Target already exists, this issue uses it.
- **vs design doc**: Fully aligned. Minor clarifications needed on error cases.
- **vs code style**: Follows pattern from target.go tests (comprehensive test coverage, clear test naming)

## Summary

The issue is well-defined and the blocking dependency (#754) is complete. Minor gaps exist around error handling edge cases and scope of helper functions, but these can be resolved with the proposed amendments. The design doc provides clear guidance on implementation details (family mapping, test fixtures, etc.).

No blocking concerns. Recommend proceeding with amendments noted above.
