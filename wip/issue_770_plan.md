# Issue 770 Implementation Plan

## Summary

Extend the Runtime interface with Build() and ImageExists() methods, integrate container building into the sandbox executor by accepting a target parameter, filtering the plan, and building custom containers based on extracted packages. The executor will use DeriveContainerSpec() and ContainerImageName() to create or reuse cached containers when packages are present, falling back to existing SandboxRequirements.Image when no packages exist.

## Approach

This implementation integrates the container building infrastructure (from #768, #769) with the sandbox executor by:

1. **Extending Runtime interface** with Build() and ImageExists() methods to support container image building
2. **Adding target-aware execution** to the sandbox executor by accepting a platform.Target parameter
3. **Integrating plan filtering** using FilterStepsByTarget() to ensure packages are extracted for the correct platform
4. **Building custom containers** when packages are present, or using existing base images when packages are nil
5. **Implementing image caching** to avoid rebuilding containers with the same package set

This approach was chosen because:
- It leverages existing FilterStepsByTarget(), ExtractPackages(), DeriveContainerSpec(), and ContainerImageName() functions
- It maintains backward compatibility by falling back to existing SandboxRequirements.Image when no packages are present
- It follows the established pattern of platform.Target as the targeting parameter (from #761)
- It keeps the Runtime interface minimal with just two new methods

### Alternatives Considered

- **Alternative 1: Add target to SandboxRequirements struct**: This would avoid changing the Sandbox() signature, but it conflates requirements computation (which happens before filtering) with execution (which happens after filtering). The chosen approach keeps these concerns separate.

- **Alternative 2: Build containers outside the executor**: This would require callers to handle container building logic, duplicating code across cmd/tsuku and internal/builders. The chosen approach centralizes container building within the executor where it logically belongs.

- **Alternative 3: Always build custom containers**: This would simplify the logic but break existing tests and use cases that don't need custom containers. The chosen approach maintains backward compatibility.

## Files to Modify

- `internal/validate/runtime.go` - Add Build() and ImageExists() methods to Runtime interface, implement in podmanRuntime and dockerRuntime
- `internal/sandbox/executor.go` - Modify Sandbox() to accept target, filter plan, extract packages, and build/use custom containers
- `cmd/tsuku/install_sandbox.go` - Update Sandbox() call to pass target parameter
- `internal/builders/orchestrator.go` - Update Sandbox() call to pass target parameter

## Files to Create

None - all functionality uses existing code from #765, #768, #769

## Implementation Steps

- [x] Extend Runtime interface in `internal/validate/runtime.go`:
  - Add `Build(ctx context.Context, imageName, baseImage string, buildCommands []string) error` method to interface
  - Add `ImageExists(ctx context.Context, name string) (bool, error)` method to interface
  - Implement Build() in podmanRuntime (generate Dockerfile, run podman build)
  - Implement Build() in dockerRuntime (generate Dockerfile, run docker build)
  - Implement ImageExists() in podmanRuntime (run podman image exists)
  - Implement ImageExists() in dockerRuntime (run docker image inspect)
  - Update mockRuntime in tests to implement new methods

- [x] Add generateDockerfile() helper in `internal/validate/runtime.go`:
  - Create function that takes baseImage and buildCommands, returns Dockerfile string
  - Format: FROM baseImage + BuildCommands lines
  - Note: Changed from ContainerSpec parameter to primitives to avoid import cycle

- [ ] Modify Sandbox() signature in `internal/sandbox/executor.go`:
  - Change from `Sandbox(ctx, plan, reqs)` to `Sandbox(ctx, plan, target, reqs)`
  - Add platform.Target import
  - Update method documentation

- [ ] Add container building logic to Sandbox() in `internal/sandbox/executor.go`:
  - After runtime detection, filter plan using executor.FilterStepsByTarget(plan.Steps, target)
  - Create new InstallationPlan with filtered steps
  - Call ExtractPackages() on filtered plan
  - If packages != nil: derive spec, check cache with ImageExists(), build if needed with Build()
  - If packages == nil: use reqs.Image (existing behavior)
  - Update opts.Image to use the determined image

- [ ] Update Sandbox() call in `cmd/tsuku/install_sandbox.go`:
  - Detect current platform using platform.Target
  - Pass target to sandboxExec.Sandbox()

- [ ] Update Sandbox() call in `internal/builders/orchestrator.go`:
  - Detect current platform using platform.Target
  - Pass target to o.sandbox.Sandbox()

- [ ] Add unit tests in `internal/validate/runtime_test.go`:
  - Test Build() method with valid ContainerSpec
  - Test ImageExists() returns true for existing images
  - Test ImageExists() returns false for non-existent images
  - Test error handling for Build() failures

- [ ] Add unit tests in `internal/sandbox/executor_test.go`:
  - Test Sandbox() with packages present (builds custom container)
  - Test Sandbox() with nil packages (uses base image)
  - Test plan filtering integration with target
  - Test image caching (second call with same packages reuses image)

- [ ] Add integration tests for multi-family execution:
  - Test same recipe with debian target builds debian-based container
  - Test same recipe with rhel target builds fedora-based container
  - Test container cache reuse (same packages → same image name)
  - Test incompatible package managers fail with clear error

## Testing Strategy

### Unit Tests

- **Runtime interface extensions** (`internal/validate/runtime_test.go`):
  - Build() creates images with correct Dockerfile content
  - ImageExists() correctly detects presence/absence of images
  - Error handling for build failures and missing runtimes

- **Executor integration** (`internal/sandbox/executor_test.go`):
  - Plan filtering applies target correctly before package extraction
  - Custom container built when packages present
  - Base image used when packages nil
  - Image caching prevents rebuilds for same package sets

### Integration Tests

- **Multi-family execution** (new file or extend `internal/sandbox/sandbox_integration_test.go`):
  - Create test recipe with apt_install action
  - Execute with target{Platform: "linux/amd64", LinuxFamily: "debian"}
  - Verify debian-based container is built
  - Execute same recipe with target{Platform: "linux/amd64", LinuxFamily: "rhel"}
  - Verify fedora-based container is built
  - Execute with same packages again, verify cached image is reused
  - Test that incompatible package managers (apt + dnf) fail with clear error message

### Manual Verification

- Build tsuku: `go build -o tsuku ./cmd/tsuku`
- Run sandbox test for recipe with system dependencies
- Verify custom container is built and cached
- Run again with same recipe, verify cache reuse (no rebuild)

## Risks and Mitigations

- **Risk 1: Runtime.Build() signature requires importing sandbox.ContainerSpec into validate package**
  - Mitigation: This is acceptable - validate.Runtime is the interface for container operations, so it naturally needs to know about ContainerSpec. The import dependency is validate -> sandbox, which is clean (sandbox already imports validate).

- **Risk 2: Changing Sandbox() signature breaks existing callers**
  - Mitigation: There are only two callers (cmd/tsuku and internal/builders), both updated in this issue. The signature change makes target explicit, which improves clarity.

- **Risk 3: Container building may fail if runtime doesn't support build**
  - Mitigation: Both podman and docker support build. If build fails, return clear error message. Sandbox test will be skipped (existing pattern for "no runtime" case).

- **Risk 4: Plan filtering may produce empty plan, causing extract packages to return nil**
  - Mitigation: This is correct behavior - if no steps match the target, there are no packages to install, so use base image. Add test coverage for this case.

- **Risk 5: generateDockerfile() duplicated between test and production code**
  - Mitigation: Move generateDockerfile() to internal/validate/runtime.go as a package-level helper function. Tests can use it too.

## Success Criteria

- [ ] Runtime interface has Build() and ImageExists() methods
- [ ] podmanRuntime and dockerRuntime implement both methods correctly
- [ ] Sandbox() accepts target parameter and filters plan before extracting packages
- [ ] Custom containers built when packages present, verified by integration test
- [ ] Base image used when packages nil, verified by unit test
- [ ] Debian and rhel targets build different containers, verified by integration test
- [ ] Container cache reuse works (same packages → no rebuild), verified by integration test
- [ ] All existing tests pass (no regressions)
- [ ] go test ./... passes
- [ ] go build ./cmd/tsuku succeeds

## Open Questions

None - all dependencies (#761, #765, #767, #768, #769) are complete and the integration approach is clear from the introspection amendments.
