# Issue 742 Implementation Plan

## Summary

Create expat and libcurl library recipes using Homebrew bottles to enable git-source test and re-enable the test-git-source CI workflow.

## Approach

Create two new library recipes (expat and libcurl) using the Homebrew action pattern, following the same structure as existing library recipes (openssl, zlib, readline, ncurses). This approach:

- Reuses proven Homebrew bottle infrastructure for library provisioning
- Enables git-source to build successfully with all required dependencies
- Maintains consistency with existing library recipes
- Requires minimal code changes (only recipe creation)

The recipes will be added to `internal/recipe/recipes/` following the alphabetical directory structure (e/ for expat, l/ for libcurl).

### Alternatives Considered

- **Alternative 1: Remove git-source test entirely**: Would unblock CI immediately but loses validation of complex multi-dependency chains. Rejected because git-source provides valuable integration testing for the dependency provisioning system.

- **Alternative 2: Create source-build recipes for expat/libcurl**: Would provide complete control but requires significantly more work (configure_make setup, versioning, checksums). Rejected because Homebrew bottles already provide these libraries reliably and other library recipes use this pattern successfully.

- **Alternative 3: Simplify git-source to remove expat/curl dependencies**: Would make the test pass but defeats the purpose of testing complex dependency chains. Rejected because git-source is specifically designed as a "ground truth" test for multi-dependency scenarios.

## Files to Create

- `internal/recipe/recipes/e/expat.toml` - Expat XML parser library recipe using Homebrew action
- `internal/recipe/recipes/l/libcurl.toml` - libcurl HTTP client library recipe using Homebrew action (separate from curl tool)

## Files to Modify

- `.github/workflows/build-essentials.yml` - Re-enable test-git-source job by removing `if: false` condition

## Implementation Steps

- [x] Create expat recipe at `internal/recipe/recipes/e/expat.toml`
  - Use Homebrew action with formula "expat"
  - Set type = "library" in metadata
  - Include library binaries (.so/.dylib files) in install_binaries
  - Add appropriate verify command

- [x] Create libcurl recipe at `internal/recipe/recipes/l/libcurl.toml`
  - Use Homebrew action with formula "curl" but focus on library files
  - Set type = "library" in metadata
  - Declare dependencies on openssl and zlib
  - Install only libcurl library files (not curl binary)
  - Add appropriate verify command

- [ ] Update git-source recipe dependencies
  - Change "curl" dependency to "libcurl" in testdata/recipes/git-source.toml
  - Verify all dependencies are now library recipes: libcurl, openssl, zlib, expat

- [ ] Re-enable test-git-source CI job
  - Remove `if: false` condition from test-git-source job in .github/workflows/build-essentials.yml

- [ ] Test locally across platforms
  - Build tsuku: `go build -o tsuku ./cmd/tsuku`
  - Test installation: `./tsuku install --recipe testdata/recipes/git-source.toml --force`
  - Verify git binary works: `$TSUKU_HOME/tools/git-source-*/bin/git --version`

## Testing Strategy

### Unit tests
- No new unit tests required (using existing Homebrew action)
- Existing recipe validation tests will validate TOML structure

### Integration tests
- Test expat recipe installation independently
- Test libcurl recipe installation independently
- Test git-source installation (full dependency chain)
- Verify git binary can execute basic commands

### Manual verification
- Test on Linux x86_64 (primary development platform)
- Test on macOS Intel (if available)
- Test on macOS Apple Silicon (if available)
- Verify no undefined symbol errors when running git

### CI validation
- Re-enabled test-git-source job will validate on all platforms (Linux x86_64, macOS Intel, macOS Apple Silicon)
- Existing verify-tool.sh script will validate git functionality
- Existing verify-relocation.sh and verify-no-system-deps.sh will validate library relocation

## Risks and Mitigations

- **Risk 1: Homebrew bottles may not have expat/libcurl for all platforms**
  - Mitigation: Test locally first, check Homebrew GHCR manifest for platform availability
  - Fallback: Exclude problematic platforms from test matrix if needed

- **Risk 2: libcurl recipe may conflict with existing curl recipe**
  - Mitigation: Use different recipe name (libcurl vs curl), install only library files not binaries
  - Verification: Both recipes should be installable simultaneously

- **Risk 3: RPATH configuration in git-source may need adjustment**
  - Mitigation: Current set_rpath already includes libcurl path, verify after testing
  - Note: RPATH uses version-pinned paths which may need updating

- **Risk 4: git-source build may require additional configure flags**
  - Mitigation: Test build process, adjust configure_args if needed
  - Reference: curl recipe already provides --with-openssl/--with-zlib patterns

## Success Criteria

- [ ] expat recipe installs successfully on Linux x86_64, macOS Intel, and macOS Apple Silicon
- [ ] libcurl recipe installs successfully on Linux x86_64, macOS Intel, and macOS Apple Silicon
- [ ] git-source builds successfully with all dependencies (expat, libcurl, openssl, zlib)
- [ ] git binary from git-source executes `git --version` successfully
- [ ] test-git-source CI job passes on all platforms
- [ ] No circular dependency errors or missing recipe errors
- [ ] Both curl (tool) and libcurl (library) recipes can be installed simultaneously

## Open Questions

None - the approach is well-defined and follows existing patterns. If Homebrew doesn't provide bottles for expat or curl libraries on certain platforms, we can adjust the test matrix accordingly.
