# Issue 904 Implementation Plan

## Approach

Use Option A from the issue: Pass `GITHUB_TOKEN` to Docker containers via `-e` flag.

This is simpler than Option B (mounting pre-generated plans) and follows the principle of least change.

## Files to Modify

1. `test/scripts/test-checksum-pinning.sh` - Add `-e GITHUB_TOKEN="$GITHUB_TOKEN"` to docker run commands

## Changes

### test-checksum-pinning.sh

Add the environment variable to docker run commands in Tests 1, 2, and 3:

- Line 125: `docker run --rm "$IMAGE_TAG"` → `docker run --rm -e GITHUB_TOKEN="$GITHUB_TOKEN" "$IMAGE_TAG"`
- Line 157: Same change
- Line 208: Same change

Test 4 does not need the token (it creates a fake state.json, no GitHub API calls).

## Validation

The fix cannot be tested locally without Docker. CI will validate that:
- All 5 matrix jobs (debian, rhel, arch, alpine, suse) pass
- No rate limit errors occur

## Risk Assessment

Low risk - this is an additive change that passes an existing environment variable into containers. If GITHUB_TOKEN is not set (e.g., local dev), the empty string is harmless.
