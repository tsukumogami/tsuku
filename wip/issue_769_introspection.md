# Issue #769 Introspection: Container Image Caching

**Issue**: [#769 feat(sandbox): implement container image caching](https://github.com/tsukumogami/tsuku/issues/769)
**Milestone**: Sandbox Container Building
**Created**: 2025-12-31
**Age**: 1 day

## Staleness Signals

- **3 sibling issues closed** since creation (#768, #767, #757)
- **Milestone position**: middle (3/6 issues in milestone)
- **1 referenced file modified**: `docs/DESIGN-structured-install-guide.md`

## Current Repository State

### Blocking Dependency Status

Issue #768 (feat(sandbox): implement container spec derivation) was **CLOSED** on 2026-01-02:
- ✅ Merged in PR #797
- ✅ Implementation complete in `internal/sandbox/container_spec.go`
- ✅ `DeriveContainerSpec()` function exists and tested
- ✅ `ContainerSpec` struct defined with all required fields

**Key finding**: The blocking dependency is now resolved. Issue #769 is unblocked.

### Recent Implementation Work

Since issue creation, the following related work was completed:

1. **#768 implementation** (merged 2026-01-02):
   - Added `internal/sandbox/container_spec.go` with `DeriveContainerSpec()`
   - Added `internal/sandbox/packages.go` with `ExtractPackages()`
   - `ContainerSpec` includes: `BaseImage`, `LinuxFamily`, `Packages`, `BuildCommands`
   - Family-to-base-image mappings implemented (debian, rhel, arch, alpine, suse)
   - Dockerfile generation via `generateBuildCommands()`

2. **Test fixtures** added for system dependency actions (PR #796):
   - `testdata/recipes/build-tools-system.toml`
   - `testdata/recipes/ca-certs-system.toml`
   - `testdata/recipes/ssl-libs-system.toml`

### Gap Analysis: Issue Specification vs Current State

**Issue #769 acceptance criteria**:
```
- [ ] Function: `ContainerImageName(linuxFamily string, packages map[string][]string) string`
- [ ] Returns deterministic name: `tsuku/sandbox-cache:<family>-<hash>`
- [ ] Hash includes linux_family + sorted packages
- [ ] Hash is stable (SHA256)
- [ ] Check if image exists before building
- [ ] Unit tests for hash stability and uniqueness
```

**Identified gaps**:

1. ❌ **Function signature mismatch**:
   - Spec says: `ContainerImageName(linuxFamily string, packages map[string][]string) string`
   - Should be: `ContainerImageName(spec *ContainerSpec) string`
   - **Rationale**: The design doc (line 726-740) shows the correct signature takes `*ContainerSpec`, not separate parameters. The spec already contains `LinuxFamily` and `Packages`.

2. ✅ **Image naming - keep family prefix**:
   - Spec says: `tsuku/sandbox-cache:<family>-<hash>`
   - Design doc (line 739) says: `tsuku/sandbox-cache:<hash>` (no family prefix)
   - **Decision**: Keep family prefix for human readability despite technical redundancy. Makes debugging and image management easier.

3. ❌ **Missing Runtime interface methods**:
   - Current `Runtime` interface (in `internal/validate/runtime.go`) only has `Run()`
   - Spec says "Check if image exists before building" but doesn't specify where this happens
   - Design doc (line 717-719) explicitly notes: "Container building requires adding: `Build(ctx, dockerfile, imageName)` and `ImageExists(ctx, imageName)`"
   - **Impact**: Issue #769 cannot implement image existence checking without these Runtime methods
   - **Dependency**: This is actually blocked by #770 (executor integration), which has "Extend Runtime interface with Build() and ImageExists()" in its acceptance criteria

4. ✅ **Hash algorithm**: SHA256 specified and matches design (line 738)

5. ✅ **Deterministic ordering**: Design shows sorting packages and PMs (lines 730-736)

6. ✅ **Hash scope**: Design correctly hashes package manager + package combinations (lines 731-736)

### Design Document Changes

The design doc was modified on 2026-01-01 (commit 60a1a5c) to update issue statuses. The core container caching design (lines 721-743) remains unchanged since issue creation, so the spec is still aligned with design intent.

### Dependency Chain Analysis

```
#768 (container spec) → CLOSED ✅
  ↓
#769 (image caching) → READY (but spec needs amendment)
  ↓
#770 (executor integration) → BLOCKED by #769, #761, #767, #765
  ↓
#771 (action execution) → BLOCKED by #770
```

**Critical finding**: Issue #770 is blocked by #769, BUT #770's acceptance criteria include extending the Runtime interface with `Build()` and `ImageExists()`. This creates a circular dependency:
- #769 needs `ImageExists()` to check cache
- #770 is supposed to add `ImageExists()` to Runtime
- #770 is blocked by #769

**Resolution**: The spec ordering is correct - #769 should focus on the `ContainerImageName()` function (hash computation) and unit tests. The actual cache checking will be implemented in #770 when Runtime interface is extended.

## Assessment Summary

### Current Specification Issues

1. **Function signature**: Spec uses wrong parameters (should take `*ContainerSpec`)
2. **Scope confusion**: Spec conflates hash generation (#769) with cache checking (#770)

### Recommendation: **AMEND**

The issue specification needs amendment to:

1. **Fix function signature**:
   ```diff
   - Function: ContainerImageName(linuxFamily string, packages map[string][]string) string
   + Function: ContainerImageName(spec *ContainerSpec) string
   ```

2. **Keep image tag format** (no change needed - original spec is correct for readability)

3. **Clarify scope** (remove cache checking from #769):
   ```diff
   - Check if image exists before building
   + (moved to #770 - requires Runtime interface extension)
   ```

4. **Updated acceptance criteria**:
   ```
   - [ ] Function: `ContainerImageName(spec *ContainerSpec) string`
   - [ ] Returns deterministic name: `tsuku/sandbox-cache:<hash>`
   - [ ] Hash includes packages (with PM names) in deterministic order
   - [ ] Hash is stable (SHA256, first 16 hex chars)
   - [ ] Unit tests for hash stability across equivalent specs
   - [ ] Unit tests for hash uniqueness across different specs
   ```

### Why Amendment (Not Re-plan)

The core goal remains valid and achievable:
- ✅ Dependency #768 is complete
- ✅ Design is clear and unchanged
- ✅ Implementation path is straightforward
- ❌ Only the AC details need correction (signature, tag format, scope)

The amendments align the issue with:
1. The actual `ContainerSpec` structure from #768
2. The design doc's hash algorithm (lines 726-740)
3. The dependency ordering (#769 generates names, #770 uses them)

## Blocking Concerns

**None** - Once spec is amended, implementation can proceed immediately. The `ContainerSpec` type exists, package extraction is implemented, and the hash algorithm is well-defined in the design.

## Recommended Changes

### Issue Body Amendment

Replace acceptance criteria section with:

```markdown
## Acceptance Criteria

- [ ] Function: `ContainerImageName(spec *ContainerSpec) string`
- [ ] Returns deterministic name: `tsuku/sandbox-cache:<family>-<hash>` where hash is first 16 hex chars of SHA256
- [ ] Hash input: sorted list of `<pm>:<package>` strings (e.g., `["apt:curl", "apt:jq"]`)
- [ ] Hash is stable (same packages → same hash)
- [ ] Unit tests verify hash stability with equivalent specs
- [ ] Unit tests verify hash uniqueness with different specs
- [ ] Unit tests verify deterministic ordering (packages and PMs sorted)
```

### Context Addition

Add note clarifying scope boundary:

```markdown
**Note**: This issue focuses on deterministic image name generation. Cache checking (`ImageExists()`) is handled in #770 when the Runtime interface is extended.
```
