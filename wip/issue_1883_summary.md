# Issue 1883 Summary

## What Was Implemented

Threaded the `--target-family` flag through the sandbox installation path so that container image selection respects the user's target family override. Previously, the sandbox always used Debian/Ubuntu images regardless of the flag value.

## Changes Made
- `cmd/tsuku/install.go`: Pass `installTargetFamily` to `runSandboxInstall`
- `cmd/tsuku/install_sandbox.go`: Accept `targetFamily` parameter; use `resolveTarget(targetFamily)` instead of `platform.DetectTarget()`; pass family to `ComputeSandboxRequirements`
- `internal/sandbox/requirements.go`: Add `targetFamily` parameter to `ComputeSandboxRequirements`; look up family-specific base image from existing `familyToBaseImage` map
- `internal/sandbox/requirements_test.go`: Add tests for all five families (with and without build actions); update existing test calls
- `internal/sandbox/executor_test.go`: Update `ComputeSandboxRequirements` call site
- `internal/sandbox/sandbox_integration_test.go`: Update `ComputeSandboxRequirements` call sites
- `internal/builders/orchestrator.go`: Update `ComputeSandboxRequirements` call site

## Key Decisions
- Reused the existing `familyToBaseImage` map from `container_spec.go` rather than introducing a separate mapping -- the map was already maintained and contains the correct images for all families
- For non-debian families with build actions, the family's own base image is used (not Ubuntu) since `augmentWithInfrastructurePackages` handles installing build tools via each family's package manager

## Trade-offs Accepted
- Unknown family values silently fall back to `DefaultSandboxImage` rather than returning an error -- validation happens earlier in `resolveTarget`, so an unknown value here means a programming error rather than user input error

## Test Coverage
- New tests added: 2 test functions (11 subtests total)
- Tests cover: all five families for binary installs, four families plus empty for build action plans, unknown family fallback

## Requirements Mapping

| AC | Status | Evidence |
|----|--------|----------|
| Pass `installTargetFamily` into `runSandboxInstall` | Implemented | `install.go:82`, `install_sandbox.go:21` |
| Use `resolveTarget(installTargetFamily)` instead of `platform.DetectTarget()` | Implemented | `install_sandbox.go:93` |
| `ComputeSandboxRequirements` accepts target family and uses `familyToBaseImage` | Implemented | `requirements.go:81-90` |
| Base image reflects target family even when sysReqs is nil | Implemented | `requirements.go:83-88` sets default image from family map |
