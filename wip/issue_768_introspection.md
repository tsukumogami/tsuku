# Issue 768 Introspection

## Context Reviewed
- Design doc: `docs/DESIGN-structured-install-guide.md`
- Sibling issues reviewed: #757 (container CI), #767 (minimal base container), #765 (ExtractPackages)
- Prior patterns identified:
  - `internal/sandbox/` package structure established
  - `ExtractPackages()` function signature and implementation pattern from #765
  - Minimal base container created at `sandbox/Dockerfile.minimal` from #767
  - Container build CI workflow from #757
  - Existing `SandboxRequirements` type and `ComputeSandboxRequirements()` from earlier work
  - Package family detection in `internal/platform/family.go`

## Gap Analysis

### Minor Gaps

**1. ContainerSpec type definition missing**
The issue references `*ContainerSpec` as a return type but this type is not defined anywhere in the codebase. From the design doc (lines 703-712), we can see the expected structure:
```go
type ContainerSpec struct {
    Base     string              // Base image
    Packages map[string][]string // Packages by PM
}
```

However, the issue also mentions "linux_family, packages, build commands" which suggests a more complete structure may be needed.

**2. Family-to-base-image mapping not fully specified**
The issue acceptance criteria mentions "debian→bookworm-slim, rhel→fedora:41, etc." but:
- The design doc shows `fedora:41` in the empirical testing table but doesn't establish this as the canonical mapping
- The existing code uses `debian:bookworm-slim` and `ubuntu:22.04` as base images
- No mapping exists yet for rhel, arch, alpine, or suse families

From the design doc empirical testing section (lines 191-198), the following distro base images were tested:
- debian: `debian:bookworm-slim`
- fedora: `fedora:41`
- alpine: `alpine:3.19`
- arch: `archlinux:base`

From `internal/platform/family.go`, the families are: debian, rhel, arch, alpine, suse

**3. Dockerfile generation logic not specified**
The issue says "Generates Dockerfile content appropriate for family's package manager" but doesn't specify:
- What format should this take (string? []string? dedicated type?)
- Should it be part of ContainerSpec or a separate function?
- The design doc mentions "Build or retrieve container" (line 651) but doesn't show the Dockerfile generation code

**4. Error handling for incompatible packages not detailed**
The acceptance criteria says "Returns error if packages require incompatible PM for target family" but:
- This scenario shouldn't happen if plans are properly filtered
- ExtractPackages receives already-filtered plans
- The validation logic location isn't specified

### Moderate Gaps

**5. Integration point with existing SandboxRequirements unclear**
The codebase already has:
- `SandboxRequirements` type (in `internal/sandbox/requirements.go`)
- `ComputeSandboxRequirements()` function
- `DefaultSandboxImage` and `SourceBuildSandboxImage` constants

The issue creates `DeriveContainerSpec()` which overlaps with this existing infrastructure. The relationship between:
- `ComputeSandboxRequirements()` (existing) - determines network needs, resource limits, image
- `DeriveContainerSpec()` (new) - maps family to base image, generates Dockerfile

This needs clarification. Looking at the design doc more carefully:
- Line 703-712: `DeriveContainerSpec()` takes `packages` map and returns spec with base image
- But it doesn't show how `linux_family` is passed to the function

The issue acceptance criteria says the function signature is:
```go
DeriveContainerSpec(linuxFamily string, packages map[string][]string) *ContainerSpec
```

But the design doc shows:
```go
DeriveContainerSpec(packages map[string][]string) *ContainerSpec
```

**This is a signature mismatch that needs resolution.**

### Major Gaps

None identified. The core implementation pattern is clear from #765 (ExtractPackages), and the container infrastructure exists from #767 and #757.

## Recommendation

**Clarify** - Seek user input on the moderate gap before proceeding.

## Questions for User

1. **Function signature**: The issue says `DeriveContainerSpec(linuxFamily string, packages map[string][]string)` but the design doc shows `DeriveContainerSpec(packages map[string][]string)`. Which is correct?

2. **Integration with SandboxRequirements**: Should `DeriveContainerSpec()` replace `ComputeSandboxRequirements()`, augment it, or be a separate code path? The existing code already selects base images based on network/build requirements.

3. **Family-to-image mapping**: Should we support all families (debian, rhel, arch, alpine, suse) in the initial implementation, or start with just debian and rhel as the acceptance criteria suggests?

## User Responses

**Function signature decision**: Omit the `linuxFamily` parameter. Rationale:
- Package manager keys in the map already encode the family information (apt→debian, dnf→rhel, pacman→arch, apk→alpine, zypper→suse)
- The mapping is already established in `internal/platform/family.go` via `distroToFamily` map
- Inferring family from package managers is more flexible and follows the existing pattern where `ExtractPackages()` returns PM-keyed maps
- The design doc shows this signature, and the design is authoritative for architectural decisions
- If packages contain multiple incompatible PMs (e.g., both apt and dnf), we'll return an error

**Integration approach**: Separate code path. Rationale:
- `ComputeSandboxRequirements()` handles different concerns (network, resource limits, pre-built images)
- `DeriveContainerSpec()` is specifically for building custom containers from packages
- They can coexist: use `DeriveContainerSpec` for custom builds, `ComputeSandboxRequirements` for pre-built images

**Family support**: Debian and RHEL only (user selected this option)

## Finalized Implementation Plan

Based on user responses, the implementation will:

1. **ContainerSpec type**: Define in `internal/sandbox/spec.go` with fields:
   - `BaseImage string` (selected based on inferred linux_family)
   - `LinuxFamily string` (inferred from package manager keys)
   - `Packages map[string][]string` (from ExtractPackages)
   - `Dockerfile string` (generated Dockerfile content)

2. **Function signature**: `DeriveContainerSpec(packages map[string][]string) (*ContainerSpec, error)`
   - Infers family from package manager keys (apt→debian, dnf→rhel)
   - Returns error if packages contain multiple incompatible PMs

3. **Family mapping**: Create PM-to-family and family-to-base-image maps with initial support for:
   - debian (apt) → debian:bookworm-slim
   - rhel (dnf) → fedora:41

4. **Dockerfile generation**: Populate `Dockerfile` field in ContainerSpec with appropriate RUN commands for the family's package manager

5. **Validation**: Return error if packages map contains multiple package managers from different families

6. **Unit tests**: Follow pattern from `packages_test.go` with table-driven tests for debian and rhel families

## Resolution

Proceed to Phase 3: Analysis with the clarified implementation approach.
