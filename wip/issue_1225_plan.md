# Issue 1225 Implementation Plan

## Summary

Fix the cmake 4.2.3 OpenSSL mismatch by updating the cmake recipe's hardcoded `openssl-3.6.0` rpath to match the actual installed OpenSSL version, and add Build Essentials to the weekly schedule trigger so upstream Homebrew bottle changes are caught even when no repo files change.

## Approach

The immediate fix addresses two problems: (1) the cmake recipe rpath may reference a stale OpenSSL version directory, and (2) the Build Essentials workflow only triggers on file path changes, missing upstream Homebrew bottle updates that break binary compatibility. The fix updates the cmake recipe and broadens CI triggers. The dependency-version-refs design (DESIGN-dependency-version-refs.md) will eliminate hardcoded versions long-term, but this issue needs a short-term fix.

### Alternatives Considered
- **Pin cmake to 4.2.1**: Avoids the OpenSSL issue but just delays it. The recipe has no version pinning mechanism and pinning fights the "latest version" design intent. Not chosen because it masks the real problem.
- **Implement dependency-version-refs now**: The proper long-term fix (use `{deps.openssl.version}` in rpath). Not chosen because it's a separate design effort already tracked, and this bug needs a quick targeted fix.
- **Build cmake from source**: Would avoid Homebrew bottle ABI issues entirely. Not chosen because it's a large scope change and the Homebrew approach works when rpath is correct.

## Files to Modify
- `internal/recipe/recipes/cmake.toml` - Update the hardcoded `openssl-3.6.0` rpath to match the current OpenSSL version resolved by Homebrew, or make the cmake recipe work with tsuku's bundled OpenSSL
- `.github/workflows/build-essentials.yml` - Add `go.mod` to path triggers so Go version bumps (which can change runner behavior) also trigger Build Essentials; document the weekly schedule as the catch-all for upstream changes
- `testdata/golden/plans/embedded/cmake/*.json` - Update golden files to reflect recipe changes
- `testdata/golden/plans/embedded/ninja/*.json` - Update golden files if ninja's cmake dependency path changes

## Files to Create
None.

## Implementation Steps
- [x] Determine the current OpenSSL version that Homebrew bottles for cmake 4.2.3 expect (check Homebrew formula dependencies)
- [x] Update `internal/recipe/recipes/cmake.toml` rpath from `openssl-3.6.0` to `{deps.openssl.version}` template variable
- [x] Verify the openssl recipe (`internal/recipe/recipes/openssl.toml`) resolves to a version whose libraries provide `OPENSSL_3.2.0` symbols
- [x] Add `go.mod` to the `paths` trigger list in `.github/workflows/build-essentials.yml` for both push and pull_request events
- [x] Update golden plan files to match the recipe changes (cmake + ninja)
- [x] Run `go test ./...` to confirm all unit tests pass
- [ ] Push branch and verify Build Essentials workflow runs and passes

## Testing Strategy
- Unit tests: `go test ./...` -- golden file tests will validate the updated plan JSON matches recipe changes
- Integration tests: The Build Essentials CI workflow itself is the integration test -- cmake, ninja, and libsixel-source must all pass on Linux x86_64 and sandbox containers
- Manual verification: Check the PR's Build Essentials workflow results for all 8 previously-failing jobs

## Risks and Mitigations
- **OpenSSL version mismatch persists**: The Homebrew openssl@3 formula may resolve to a different version than what cmake 4.2.3 expects. Mitigation: check the exact Homebrew bottle dependency chain before updating the rpath.
- **Golden file churn**: Updating the rpath changes golden files. Mitigation: straightforward find-and-replace, low risk of error.
- **Future version drift recurrence**: The hardcoded version will go stale again when OpenSSL updates. Mitigation: the weekly schedule in Build Essentials catches this; the dependency-version-refs design will eliminate the problem permanently.

## Success Criteria
- [ ] Build Essentials passes for cmake, ninja, and libsixel-source on Linux x86_64
- [ ] Sandbox jobs (alpine, arch, debian, rhel, suse) pass for ninja and cmake
- [ ] `go test ./...` passes locally
- [ ] Root cause documented in PR description: Build Essentials didn't trigger because the Go version bump only changed `go.mod`, which wasn't in the workflow's path filter
- [ ] CI improvement: `go.mod` added to Build Essentials path triggers

## Open Questions
- What exact OpenSSL version does the current Homebrew openssl@3 formula resolve to? This must be checked during implementation (step 1) to set the correct rpath. If tsuku's openssl recipe already installs the right version, the fix is just an rpath string update.
