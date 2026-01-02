# Issue 770 Introspection

## Context Reviewed

- **Design doc**: `docs/DESIGN-structured-install-guide.md`
- **Sibling issues reviewed**: #769 (container caching), #768 (container spec derivation), #767 (base container), #757 (container CI)
- **Prior patterns identified**:
  - Container building infrastructure in `internal/sandbox/` package
  - `ContainerSpec` struct with `BaseImage`, `LinuxFamily`, `Packages`, and `BuildCommands` fields
  - `DeriveContainerSpec(packages)` function that infers linux_family from package managers
  - `ContainerImageName(spec)` function that generates deterministic cache names
  - `ExtractPackages(plan)` already exists in `internal/sandbox/packages.go`
  - `Runtime` interface currently only has `Run()` method, needs extension
  - `Executor.Sandbox()` method exists in `internal/sandbox/executor.go` but doesn't use container building
  - `Target` struct in `internal/platform/target.go` contains `Platform` and `LinuxFamily` fields

## Gap Analysis

### Minor Gaps

**1. Runtime interface extension location**

The issue states: "Extend `Runtime` interface with `Build(spec *ContainerSpec) error` and `ImageExists(name string) bool`"

However, the `Runtime` interface is in `internal/validate/runtime.go` (lines 18-27), not in the sandbox package. The signature needs adjustment:
- `Build()` needs a context parameter: `Build(ctx context.Context, spec *ContainerSpec) error`
- `ImageExists()` needs a context parameter: `ImageExists(ctx context.Context, name string) (bool, error)`

**2. Executor.Sandbox signature change**

The current `Executor.Sandbox()` signature is:
```go
func (e *Executor) Sandbox(ctx context.Context, plan *executor.InstallationPlan, reqs *SandboxRequirements) (*SandboxResult, error)
```

The issue states it should accept: `Execute(recipe, platform Platform, linuxFamily string)`

This needs clarification - the actual signature should likely be:
```go
func (e *Executor) Execute(ctx context.Context, plan *executor.InstallationPlan, target platform.Target) (*SandboxResult, error)
```

Or we extend the existing `Sandbox()` method to accept a target parameter.

**3. Plan filtering integration**

The issue mentions "Plan filtering uses target platform+linux_family via FilterPlan()" but doesn't specify where this filtering happens. Based on dependency #761 being closed, `FilterPlan()` exists in the executor package. The sandbox executor needs to call this before extracting packages.

**4. Integration with existing buildSandboxScript**

The current `Executor.buildSandboxScript()` hardcodes apt-get commands (lines 280-291). Once container building is integrated, this logic should be removed since packages will be pre-installed in the custom container.

**5. Dockerfile generation**

The design shows `ContainerSpec` has a `BuildCommands` field (from PR #797), which contains Dockerfile RUN commands. However, the actual Dockerfile generation (creating FROM + RUN commands) needs to be implemented to pass to `Build()`.

### Moderate Gaps

**1. Error handling for incompatible package managers**

`DeriveContainerSpec()` returns an error for incompatible package managers (e.g., both apt and dnf). The executor integration needs to handle this error case gracefully - should it fail the test or fall back to a base image?

**Resolution**: This should fail the test with a clear error message, as it indicates a malformed recipe. Recipes should never have incompatible package managers in the same platform-filtered plan.

**2. Base image selection strategy**

The issue states "If no packages: use base container for target". However, `DeriveContainerSpec(nil)` returns `nil`, not a base container spec. The executor needs logic like:

```go
if packages == nil {
    // Use default base image based on target.LinuxFamily
    // Or fall back to existing DefaultSandboxImage
}
```

**Resolution**: When no packages are present, use the existing `DefaultSandboxImage` or `SourceBuildSandboxImage` based on `SandboxRequirements`, not target-specific containers.

**3. Multi-family execution integration test requirement**

The issue requires "Integration tests for multi-family execution" but doesn't specify what this means. Does it mean:
- Testing the same recipe on debian vs rhel targets?
- Testing that family detection works correctly?
- Testing that incompatible package managers are rejected?

**Resolution**: This likely means testing that the same recipe can be executed with different targets (e.g., debian/bookworm and fedora/41) and verify the correct container is built for each.

## Recommendation

**Proceed** with moderate amendments.

## Proposed Amendments

Based on review of completed work in this milestone:

1. **Runtime interface extension**: Add `Build(ctx context.Context, spec *ContainerSpec) error` and `ImageExists(ctx context.Context, name string) (bool, error)` to the `Runtime` interface in `internal/validate/runtime.go`. Both Podman and Docker implementations will need these methods.

2. **Executor signature clarification**: The executor should accept a `platform.Target` parameter rather than separate `platform` and `linuxFamily` strings. This matches the pattern established by FilterPlan() (issue #761).

3. **Plan filtering integration**: Call `executor.FilterPlan(recipe, target)` before extracting packages to ensure the plan is filtered for the target platform.

4. **Dockerfile generation**: Implement a helper function to generate a complete Dockerfile string from a `ContainerSpec` (FROM + BuildCommands).

5. **Base image fallback**: When `ExtractPackages()` returns nil (no system dependencies), use the existing `SandboxRequirements.Image` rather than building a custom container.

6. **Integration test scope**: Add tests that verify:
   - Same recipe with debian target builds debian-based container
   - Same recipe with rhel target builds fedora-based container
   - Container cache reuse works (same packages → same image name)
   - Incompatible package managers fail with clear error

These amendments ensure the implementation integrates correctly with the established patterns from #768, #769, and the existing sandbox executor.
