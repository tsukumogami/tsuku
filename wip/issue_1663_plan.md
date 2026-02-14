# Issue 1663 Implementation Plan

## Summary

Remove the 18 affected recipes from `execution-exclusions.json` since GitHub Actions `ubuntu-latest` now uses Ubuntu 24.04 with glibc 2.39, which is compatible with Homebrew bottles.

## Root Cause Analysis

The segfaults (exit 139) occurred because:
1. Homebrew Linux bottles are compiled against glibc 2.35+
2. CI was running on older Ubuntu with insufficient glibc version
3. Binaries crashed with SIGSEGV when trying to use missing glibc symbols

## Solution

GitHub updated `ubuntu-latest` to Ubuntu 24.04 in January 2025. Ubuntu 24.04 has glibc 2.39, which is compatible with all current Homebrew bottles. The fix is simply to remove the exclusions since the underlying infrastructure issue is resolved.

### Why Not Implement minimum_glibc?

The original plan proposed adding a `minimum_glibc` field to recipes. This remains a good idea for user-facing clarity (showing "requires glibc 2.35, found 2.31" instead of a cryptic segfault), but:

1. The immediate CI issue is already resolved by GitHub's infrastructure update
2. The minimum_glibc feature is a larger undertaking better suited for a dedicated issue
3. Removing exclusions unblocks CI validation immediately

## Implementation Steps

- [x] Remove 18 recipes linked to #1663 from `execution-exclusions.json`
  - act, buf, cliproxyapi, cloudflared, fabric-ai, gh, git-lfs, go-task
  - goreman, grpcurl, jfrog-cli, license-eye, mkcert, oh-my-posh
  - tailscale, temporal, terragrunt, witr

- [x] Also remove sqlite exclusion (was linked to #1663 for macOS abort trap)

## Testing Strategy

- Unit tests: Pass (no code changes)
- CI validation: PR will trigger CI which runs on ubuntu-latest (Ubuntu 24.04)
- If any recipes still fail, they can be re-added with updated tracking issues

## Success Criteria

- [x] Exclusions removed from execution-exclusions.json
- [ ] CI passes on PR
- [ ] Nightly validation passes for previously excluded recipes

## Future Work

Consider filing a separate issue for:
- Adding `minimum_glibc` field to recipes for better user error messages
- This would help users on older systems understand why recipes fail
