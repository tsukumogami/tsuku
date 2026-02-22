# Issue 1883 Implementation Plan

## Summary

Thread the `--target-family` flag through the sandbox installation path so that `ComputeSandboxRequirements` selects the correct family-specific base image, and `runSandboxInstall` uses `resolveTarget(installTargetFamily)` instead of `platform.DetectTarget()`.

## Approach

The fix touches three layers: the CLI entry point (`install_sandbox.go`), the requirements computation (`requirements.go`), and the executor fallback (`executor.go`). The `familyToBaseImage` mapping in `container_spec.go` already has the correct images for all five families. The key insight is that `ComputeSandboxRequirements` needs a target family parameter to pick the right base image, and `runSandboxInstall` needs to use `resolveTarget` (which already handles `--target-family` correctly) instead of `platform.DetectTarget()`.

### Alternatives Considered

- **Alternative 1: Only fix `runSandboxInstall` to pass the right target, rely on executor fallback**: The executor's `effectiveFamily` logic (line 159-161) and `DeriveContainerSpec` path (line 169-201) would select the right image only when `sysReqs != nil`. When `sysReqs` is nil (common for simple binary installs), the image stays as `reqs.Image` (always Debian/Ubuntu). This wouldn't solve the "no system deps" case.

- **Alternative 2: Move all image selection logic into the executor, remove it from `ComputeSandboxRequirements`**: This would centralize image selection but would break the existing design where `ComputeSandboxRequirements` provides a complete `SandboxRequirements` that callers can inspect and display before execution. The confirmation prompt and info messages both show `reqs.Image` before calling `Sandbox()`.

- **Alternative 3: Have the executor always create a ContainerSpec even with empty sysReqs**: The executor would use `DeriveContainerSpec` to pick the family-matched image even when there are no packages. This would require changes to `DeriveContainerSpec` (which currently returns nil for empty packages) and add unnecessary complexity. The simplest fix is to make `ComputeSandboxRequirements` pick the right default image up front.

## Files to Modify

- `cmd/tsuku/install_sandbox.go` - Pass `installTargetFamily` to `runSandboxInstall`; use `resolveTarget(installTargetFamily)` instead of `platform.DetectTarget()`
- `internal/sandbox/requirements.go` - Add `targetFamily` parameter to `ComputeSandboxRequirements`; use `familyToBaseImage` to select default image when a family is specified
- `internal/sandbox/requirements_test.go` - Add tests for family-aware image selection in `ComputeSandboxRequirements`
- `internal/sandbox/executor_test.go` - Update any tests that call `ComputeSandboxRequirements` with old signature

## Files to Create

None.

## Implementation Steps

- [x] 1. Update `ComputeSandboxRequirements` signature in `internal/sandbox/requirements.go` to accept a `targetFamily string` parameter. When `targetFamily` is non-empty, look it up in `familyToBaseImage` (from `container_spec.go`) to set the default `reqs.Image` instead of hardcoding `DefaultSandboxImage`. Similarly, when upgrading to the source build image, use the family-appropriate image (or fall back to `SourceBuildSandboxImage` for debian). This requires exporting or accessing the `familyToBaseImage` map from `container_spec.go` -- since both files are in the same package, it's already accessible.

- [x] 2. Update `runSandboxInstall` in `cmd/tsuku/install_sandbox.go`:
  - Change the function signature to accept `targetFamily string` as a parameter.
  - Replace `platform.DetectTarget()` (line 94) with `resolveTarget(targetFamily)` to honor the `--target-family` flag.
  - Pass `targetFamily` to `ComputeSandboxRequirements(plan, targetFamily)`.

- [x] 3. Update the call site in `cmd/tsuku/install.go` (line 82) to pass `installTargetFamily` to `runSandboxInstall`.

- [x] 4. Update all callers of `ComputeSandboxRequirements` to pass the new parameter. The only external caller is in `install_sandbox.go` (already covered in step 2). Verify no other callers exist via grep.

- [x] 5. Update tests in `internal/sandbox/requirements_test.go`:
  - Update all existing `ComputeSandboxRequirements` calls to pass `""` as the family (preserving current behavior).
  - Add new test cases: `TestComputeSandboxRequirements_TargetFamily_Alpine`, `TestComputeSandboxRequirements_TargetFamily_Suse`, `TestComputeSandboxRequirements_TargetFamily_Rhel`, `TestComputeSandboxRequirements_TargetFamily_Arch` that verify the correct base image is selected.
  - Add a test case for family + build actions (should use the family image, not `SourceBuildSandboxImage`).
  - Add a test case for empty/unknown family (should fall back to `DefaultSandboxImage`).

- [x] 6. Update tests in `internal/sandbox/executor_test.go` if any test calls `ComputeSandboxRequirements` (the `TestSandbox_NoRuntime` test uses it on line 342).

- [x] 7. Run `go vet ./...`, `go test ./...`, and `go build -o tsuku ./cmd/tsuku` to verify the changes compile and pass.

## Testing Strategy

- **Unit tests (requirements_test.go)**: Verify `ComputeSandboxRequirements` returns the correct image for each target family value: `""` (default/debian), `"alpine"`, `"rhel"`, `"arch"`, `"suse"`, `"debian"`. Test both the simple binary install path (no network, no build actions) and the source build upgrade path (with build actions or network requirements).

- **Unit tests (executor_test.go)**: Verify existing executor tests still pass with the updated `ComputeSandboxRequirements` call.

- **Manual verification**: Run the exact reproduction steps from the issue:
  ```
  tsuku eval --recipe recipes/b/b3sum.toml --install-deps --linux-family alpine 2>/dev/null > /tmp/b3sum-plan-alpine.json
  tsuku install --plan /tmp/b3sum-plan-alpine.json --sandbox --force --target-family alpine
  ```
  Verify the output shows `Container image: alpine:3.19` instead of `ubuntu:22.04`.

## Risks and Mitigations

- **Risk: Build/network image upgrade path might not use family-specific images correctly**: When a plan has build actions (e.g., `cargo_build`), the current code upgrades to `SourceBuildSandboxImage` (ubuntu:22.04). With the fix, it should upgrade to the build variant of the target family. For non-debian families, the same family base image is suitable since they install build tools via their own package manager (already handled by `augmentWithInfrastructurePackages`).
  - **Mitigation**: The `familyToBaseImage` mapping already provides appropriate base images for all families. The `augmentWithInfrastructurePackages` function already maps families to their correct package managers for build tools. No additional image variants are needed.

- **Risk: Changing `ComputeSandboxRequirements` signature breaks external callers**: This is an internal package, so there should be no external callers.
  - **Mitigation**: Grep for all callers before making the change. The function is only called in `install_sandbox.go` and test files.

- **Risk: `resolveTarget` returns an error for invalid family names**: The `resolveTarget` function already validates family names and returns an error for unknown values.
  - **Mitigation**: `runSandboxInstall` already handles errors from target detection. The same error handling path applies.

## Success Criteria

- [ ] `ComputeSandboxRequirements` returns family-appropriate images when a `targetFamily` is specified (e.g., `alpine:3.19` for alpine, `fedora:41` for rhel)
- [ ] `ComputeSandboxRequirements` returns `DefaultSandboxImage` (debian:bookworm-slim) when no family is specified (backward compatible)
- [ ] `runSandboxInstall` uses `resolveTarget(installTargetFamily)` instead of `platform.DetectTarget()`
- [ ] Running `tsuku install --plan <alpine-plan> --sandbox --force --target-family alpine` shows `Container image: alpine:3.19`
- [ ] All existing tests pass without modification (except adding the new `""` parameter)
- [ ] `go vet`, `go test ./...`, and `go build` all succeed

## Open Questions

None. All required mapping data (`familyToBaseImage`) already exists and is accessible within the `sandbox` package.
