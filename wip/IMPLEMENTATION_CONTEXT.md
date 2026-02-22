## Summary

When running `tsuku install --plan <plan.json> --sandbox --force --target-family <family>`, the sandbox always uses `ubuntu:22.04` (or `debian:bookworm-slim`) as the container image regardless of the `--target-family` flag value.

## Reproduction steps

1. Generate a plan targeting Alpine:
   ```
   tsuku eval --recipe recipes/b/b3sum.toml --install-deps --linux-family alpine 2>/dev/null > /tmp/b3sum-plan-alpine.json
   ```

2. Run sandbox install with `--target-family alpine`:
   ```
   tsuku install --plan /tmp/b3sum-plan-alpine.json --sandbox --force --target-family alpine
   ```
   **Expected:** Container image should be Alpine-based (e.g., `alpine:3.19`)
   **Actual:** Output says `Container image: ubuntu:22.04`

3. Same with `--target-family suse`:
   ```
   tsuku eval --recipe recipes/b/b3sum.toml --install-deps --linux-family suse 2>/dev/null > /tmp/b3sum-plan-suse.json
   tsuku install --plan /tmp/b3sum-plan-suse.json --sandbox --force --target-family suse
   ```
   **Expected:** Container image should be SUSE-based (e.g., `opensuse/leap:15`)
   **Actual:** Output says `Container image: ubuntu:22.04`

## Root cause

Two issues combine to produce this behavior:

### 1. `ComputeSandboxRequirements` ignores the target family

In `internal/sandbox/requirements.go`, `ComputeSandboxRequirements` picks the container image based solely on whether the plan has network-requiring or build actions. It always returns either `DefaultSandboxImage` (`debian:bookworm-slim`) or `SourceBuildSandboxImage` (`ubuntu:22.04`). There is no path to select a family-specific base image (Alpine, SUSE, Fedora, Arch) here.

### 2. `runSandboxInstall` does not pass `--target-family` to platform detection

In `cmd/tsuku/install_sandbox.go` line 95, the sandbox path calls `platform.DetectTarget()` directly, which detects the host system's family. The `installTargetFamily` flag variable (set by `--target-family`) is never read in this code path. It is only wired in `cmd/tsuku/install_deps.go` for the system dependency display flow.

As a result, even the fallback logic in `internal/sandbox/executor.go` line 159-161 (which prefers `plan.Platform.LinuxFamily` and falls back to `target.LinuxFamily()`) gets the host's family rather than the user-specified one.

### Where the family-to-image mapping exists but is not reached

The `familyToBaseImage` map in `internal/sandbox/container_spec.go` correctly maps families to images (e.g., `alpine` -> `alpine:3.19`, `suse` -> `opensuse/leap:15`). This mapping is used by `DeriveContainerSpec`, which is called from `executor.go` line 171 -- but only when system dependency packages exist (`sysReqs != nil`). When a plan has no explicit system dependency packages, the image stays as whatever `ComputeSandboxRequirements` set.

## Suggested fix

1. Pass `installTargetFamily` into `runSandboxInstall` and use `resolveTarget(installTargetFamily)` instead of `platform.DetectTarget()` in `cmd/tsuku/install_sandbox.go`.
2. Make `ComputeSandboxRequirements` accept a target family parameter (or accept the target), and use the `familyToBaseImage` mapping to select the correct base image when a non-default family is specified.
3. Ensure that even when `sysReqs` is nil (no system dependency packages), the base image still reflects the target family.

## Code pointers

- `cmd/tsuku/install_sandbox.go:95` -- calls `platform.DetectTarget()` ignoring `installTargetFamily`
- `internal/sandbox/requirements.go:78-124` -- `ComputeSandboxRequirements` hardcodes Debian/Ubuntu images
- `internal/sandbox/executor.go:159-161` -- `effectiveFamily` fallback uses host target
- `internal/sandbox/executor.go:168` -- `containerImage` starts from `reqs.Image` (always Debian/Ubuntu)
- `internal/sandbox/container_spec.go:43-49` -- `familyToBaseImage` has correct mapping but is not used in the default path
- `cmd/tsuku/install.go:223` -- `--target-family` flag definition
- `cmd/tsuku/install_deps.go:262` -- only place `installTargetFamily` is currently used
