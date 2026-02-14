# Issue 1663 Implementation Plan

## Summary

Add a glibc version constraint field to recipes (`minimum_glibc`) and implement glibc version checking in the homebrew action. Recipes using Homebrew bottles will declare their minimum glibc version (2.35 for recent bottles), and the platform check will skip incompatible recipes on older runners.

## Approach

The root cause is a glibc version mismatch: Homebrew Linux bottles are built on recent Ubuntu (glibc 2.35+), but CI runners may have older glibc versions where these binaries crash with SIGSEGV. The solution is to:

1. Detect the runtime glibc version
2. Allow recipes to declare their minimum glibc requirement
3. Skip execution on systems with incompatible glibc versions

This approach treats glibc version compatibility as a first-class platform constraint, similar to how `supported_libc` distinguishes glibc from musl systems.

### Alternatives Considered

- **Use newer CI runner images (ubuntu-24.04)**: The 18 affected recipes already have `unsupported_platforms` entries for specific distros, suggesting the issue is well-known. However, ubuntu-24.04 has glibc 2.39, which is sufficient. This is the simplest fix but doesn't address the underlying problem for users on older systems.

- **Skip homebrew recipes on linux-debian-amd64 in CI**: Would paper over the issue but doesn't help users understand why recipes fail on their systems.

- **Build tsuku's own bottles**: Would require significant infrastructure investment and doesn't address the underlying glibc compatibility issue.

**Chosen approach rationale**: The glibc version check provides transparent feedback to users, works with existing exclusion mechanisms, and scales to handle future recipes. It's also the most informative solution - users see "requires glibc 2.35, found 2.31" rather than a cryptic segfault.

## Files to Modify

- `internal/platform/libc.go` - Add `DetectGlibcVersion()` function
- `internal/recipe/types.go` - Add `MinimumGlibc` field to Metadata struct
- `internal/recipe/platform.go` - Extend `SupportsPlatformWithLibc` to check glibc version
- `internal/actions/homebrew.go` - Add glibc version preflight check
- `cmd/tsuku/install.go` - Include glibc version in platform support error

## Files to Create

- `internal/platform/glibc.go` - Glibc version detection logic
- `internal/platform/glibc_test.go` - Tests for glibc version detection

## Implementation Steps

- [ ] Add glibc version detection to `internal/platform/glibc.go`
  - Parse `/lib/x86_64-linux-gnu/libc.so.6` or similar for version
  - Use `strings libc.so.6 | grep GLIBC_` as fallback
  - Return semantic version (major.minor) for comparison

- [ ] Add `MinimumGlibc` field to recipe types in `internal/recipe/types.go`
  - Optional field: `minimum_glibc = "2.35"` in TOML
  - Only meaningful when `supported_libc = ["glibc"]`

- [ ] Extend platform checking in `internal/recipe/platform.go`
  - New method: `MeetsGlibcRequirement(version string) bool`
  - Update `SupportsPlatformRuntime` to check glibc version
  - Add glibc version to `UnsupportedPlatformError`

- [ ] Add preflight check in `internal/actions/homebrew.go`
  - Check glibc version before attempting download
  - Return clear error with required vs available versions

- [ ] Update install command error messaging in `cmd/tsuku/install.go`
  - Include glibc version in platform mismatch error
  - Suggest alternatives (use different recipe, upgrade system)

- [ ] Add `minimum_glibc = "2.35"` to affected recipes
  - act.toml, buf.toml, cliproxyapi.toml, cloudflared.toml
  - fabric-ai.toml, gh.toml, git-lfs.toml, go-task.toml
  - goreman.toml, grpcurl.toml, jfrog-cli.toml, license-eye.toml
  - mkcert.toml, oh-my-posh.toml, tailscale.toml, temporal.toml
  - terragrunt.toml, witr.toml

- [ ] Update CI workflow to use ubuntu-24.04 for homebrew tests
  - Modify `.github/workflows/nightly-registry-validation.yml`
  - Update container images in `validate-linux-containers` job
  - This ensures CI has sufficient glibc for testing

- [ ] Remove affected recipes from execution-exclusions.json
  - Once the proper glibc checks are in place

## Testing Strategy

- Unit tests:
  - `DetectGlibcVersion()` returns correct version on glibc systems
  - `MeetsGlibcRequirement()` correctly compares versions
  - `SupportsPlatformRuntime()` rejects incompatible glibc versions
  - Recipe parsing correctly reads `minimum_glibc` field

- Integration tests:
  - Homebrew recipe with `minimum_glibc` skips cleanly on older systems
  - Error message includes helpful information about glibc requirement

- Manual verification:
  - Test on ubuntu-20.04 container (glibc 2.31) - should skip
  - Test on ubuntu-24.04 runner (glibc 2.39) - should succeed

## Risks and Mitigations

- **Risk: False negatives from version detection**
  - Mitigation: Multiple detection methods (ELF parsing, ldd output, strings on libc)
  - Fallback to "unknown" version which allows execution (fail open)

- **Risk: Homebrew bottles require different glibc versions**
  - Mitigation: Start with 2.35 as default, can be refined per-recipe as needed
  - Batch validation workflow can help identify version requirements

- **Risk: Breaking existing workflows**
  - Mitigation: Feature flag to disable glibc checking initially
  - Gradual rollout by updating recipes one at a time

## Success Criteria

- [ ] All 18 affected recipes have `minimum_glibc = "2.35"` specified
- [ ] `tsuku install gh` on glibc 2.31 system shows clear error message
- [ ] `tsuku install gh` on glibc 2.39 system succeeds
- [ ] CI tests pass on ubuntu-24.04 runners
- [ ] Recipes removed from execution-exclusions.json
- [ ] No segfaults in nightly validation

## Open Questions

None - the approach is well-defined based on existing platform constraint patterns.
