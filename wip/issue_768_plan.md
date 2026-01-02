# Issue 768 Implementation Plan

## Summary

Implement `DeriveContainerSpec()` function that maps extracted packages to a container specification, including base image selection based on linux_family and Dockerfile generation for building containers with required system dependencies.

## Approach

The function takes the output of `ExtractPackages()` (already implemented in #765) and derives a container specification. Based on the design document, the implementation needs to:

1. Map package managers to linux_family (apt→debian, dnf→rhel, pacman→arch, etc.)
2. Select appropriate base images for each family
3. Generate Dockerfile content with package installation commands
4. Validate that all packages use compatible package managers

This is a pure function that transforms package requirements into container build specifications. The actual container building and caching (issues #769 and #770) will consume this output.

### Alternatives Considered

**Alternative 1: Return docker build commands directly**
- Create shell commands instead of structured data
- Pros: Simpler initial implementation
- Cons: Hard to test, couples container runtime choice, can't be analyzed/validated

**Alternative 2: Support multiple package managers in one container**
- Allow mixing apt and dnf in same container
- Pros: More flexible
- Cons: Violates linux_family constraint - a container is either debian-based or rhel-based, not both. The design explicitly requires validation that packages don't require incompatible PMs.

**Alternative 3: Return just base image name (chosen)**
- Derive full ContainerSpec with base image, packages, and Dockerfile generation
- Pros: Structured, testable, separates concerns, enables validation
- Cons: More code than alternative 1

## Files to Modify

None - this is new functionality.

## Files to Create

- `internal/sandbox/container_spec.go` - ContainerSpec type and DeriveContainerSpec function
- `internal/sandbox/container_spec_test.go` - Unit tests for all package manager families

## Implementation Steps

- [x] Define ContainerSpec struct with fields: BaseImage, LinuxFamily, Packages, BuildCommands
- [x] Implement family-to-base-image mapping (debian→bookworm-slim, rhel→fedora:41, arch→archlinux:base, alpine→alpine:3.19, suse→opensuse/leap:15)
- [x] Implement DeriveContainerSpec function that extracts linux_family from packages map
- [x] Add validation to detect incompatible package managers (e.g., apt + dnf in same plan)
- [x] Implement Dockerfile generation method on ContainerSpec
- [x] Add unit tests for single package manager cases (apt, brew, dnf, pacman, apk, zypper)
- [x] Add unit tests for incompatible package manager detection
- [x] Add unit test for nil/empty packages input
- [x] Document the function with examples in godoc

## Testing Strategy

### Unit Tests

**Family mapping tests:**
- apt packages → debian family, bookworm-slim base
- dnf packages → rhel family, fedora:41 base
- pacman packages → arch family, archlinux:base base
- apk packages → alpine family, alpine:3.19 base
- zypper packages → suse family, opensuse/leap:15 base
- brew packages → error (macOS PM not applicable to containers)

**Validation tests:**
- Mixing apt + dnf → error
- Mixing apt + pacman → error
- Empty packages map → nil ContainerSpec
- nil packages → nil ContainerSpec

**Dockerfile generation tests:**
- Debian: `RUN apt-get update && apt-get install -y <packages>`
- RHEL: `RUN dnf install -y <packages>`
- Arch: `RUN pacman -Sy --noconfirm <packages>`
- Alpine: `RUN apk add --no-cache <packages>`
- SUSE: `RUN zypper install -y <packages>`

**Edge cases:**
- Package list with duplicates (should be deduplicated)
- Package names with special characters (should be validated)
- Multiple steps adding to same PM (aggregation already handled by ExtractPackages)

### Integration Tests

Not needed at this layer - integration happens in #770 when executor uses DeriveContainerSpec.

### Manual Verification

After implementation:
1. Call `DeriveContainerSpec(map[string][]string{"apt": ["curl", "jq"]})` → verify returns debian base
2. Call `DeriveContainerSpec(map[string][]string{"dnf": ["wget"]})` → verify returns fedora base
3. Call `DeriveContainerSpec(map[string][]string{"apt": ["curl"], "dnf": ["wget"]})` → verify returns error
4. Generate Dockerfile and verify syntax with `docker build`

## Risks and Mitigations

**Risk 1: Base image versions may become outdated**
- Mitigation: Use latest stable releases (bookworm, fedora:41, etc.). Document version choices in constants with comments explaining selection.

**Risk 2: Package manager command syntax may vary across distros**
- Mitigation: Reference package manager documentation for each family. Add comments with links to official docs.

**Risk 3: Brew packages in ExtractPackages output**
- Mitigation: DeriveContainerSpec should return error for brew packages since macOS package managers don't apply to Linux containers. Document this constraint.

**Risk 4: Dockerfile generation might need escaping**
- Mitigation: Use simple string templates for now since package names are already validated by ExtractPackages. Add validation for shell metacharacters if needed.

**Risk 5: linux_family detection may be ambiguous**
- Mitigation: The package manager name directly maps to family (apt→debian, dnf→rhel, etc.). This is unambiguous - a recipe either uses apt OR dnf, never both (enforced by validation).

## Success Criteria

- [ ] DeriveContainerSpec function exists with correct signature
- [ ] Family-to-base-image mapping covers all supported families (debian, rhel, arch, alpine, suse)
- [ ] ContainerSpec contains base image, linux_family, packages map, and can generate Dockerfile content
- [ ] Returns error if packages require incompatible package managers
- [ ] Returns nil for nil/empty packages input
- [ ] Unit tests pass for each family mapping
- [ ] Unit tests pass for incompatible PM detection
- [ ] go vet passes
- [ ] golangci-lint passes
- [ ] All existing tests still pass

## Open Questions

None - the design document and existing ExtractPackages implementation provide clear guidance.
