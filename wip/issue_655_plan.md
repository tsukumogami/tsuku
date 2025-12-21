# Implementation Plan: Issue #655 - Update curl Recipe for Dynamic Versioning

## Summary

This issue modernizes the curl recipe to use dynamic versioning with template variables, eliminating the hardcoded version (8.11.1) and static checksum. The recipe will transition from the `download_file` action to the `download` action, which supports the `{version}` template variable for both URLs and archive filenames.

**Key Benefit:** Once updated, the curl recipe can be upgraded to new versions by simply changing the Homebrew formula without editing the recipe file itself. This brings curl into alignment with the established pattern used by terraform, boundary, vault, and other modern recipes.

**Note on Checksums:** curl.se provides PGP signatures (.asc files) but not SHA256SUMS files. The `download` action will compute and verify checksums by downloading the file during plan generation, similar to how ruby and rust recipes work.

## Approach

**Migration Strategy:**
- Keep existing Homebrew version source (already configured correctly)
- Replace `download_file` with `download` action, using `{version}` template in URL
- Update `extract` action's archive parameter to use `{version}` template
- Update `configure_make` source_dir parameter to use `{version}` template
- Remove static checksum (will be computed during plan generation via Decompose)
- All other steps (setup_build_env, configure_make, set_rpath, install_binaries, verify) remain unchanged

**Recipe Location:** `internal/recipe/recipes/c/curl.toml`

**Pattern Reference:** terraform recipe (uses `download` with `{version}` and `checksum_url`)

**Checksum Strategy:** Since curl.se doesn't provide SHA256SUMS files (only PGP signatures), we'll follow the ruby/rust pattern: use `download` action without `checksum_url`, and let the action's Decompose method compute the checksum by downloading the file during plan generation.

## Alternatives Considered

### 1. Checksum Verification Approach

**Option A: Use PGP Signatures (.asc files) (NOT CHOSEN)**
- Pros: Cryptographically stronger than SHA256, officially provided by curl.se
- Cons:
  - Requires PGP key management and verification infrastructure
  - No existing tsuku recipes use PGP verification
  - Would need to implement new action or extend download action
  - Adds complexity for marginal security benefit (HTTPS + SHA256 is sufficient)
- **Decision:** Not chosen due to implementation complexity and lack of precedent

**Option B: Use checksum_url Parameter (NOT CHOSEN)**
- Pros: Follows terraform/boundary/vault pattern exactly
- Cons: curl.se doesn't provide SHA256SUMS files at predictable URLs
- **Decision:** Not applicable since checksums aren't available

**Option C: Compute Checksum During Plan Generation (CHOSEN)**
- Pros:
  - Follows established pattern (ruby, rust recipes)
  - Works with download action's Decompose method
  - No changes needed to action infrastructure
  - Downloads cached for actual installation
- Cons: Requires downloading file during plan generation
- **Decision:** Use this approach - it's the standard pattern for sources without checksum files

### 2. Version Template Variable Usage

**Option A: Keep Static Version with Template (NOT CHOSEN)**
- Example: Keep `version = "8.11.1"` in recipe but use `{version}` in URLs
- Pros: Explicit version control in recipe file
- Cons: Defeats the purpose of dynamic versioning, still requires recipe edits for updates
- **Decision:** Not chosen - doesn't achieve the goal

**Option B: Use Homebrew Dynamic Versioning (CHOSEN)**
- Example: Use `source = "homebrew"` with `{version}` templates throughout
- Pros:
  - Single source of truth for version (Homebrew formula)
  - No recipe edits needed for version updates
  - Consistent with other modern recipes
  - Recipe already has Homebrew configured correctly
- Cons: None
- **Decision:** Use this approach - it's the modern standard

### 3. Archive Format Selection

**Option A: Switch to .tar.xz (NOT CHOSEN)**
- Pros: Better compression than .tar.gz
- Cons: Unnecessary change, .tar.gz works fine
- **Decision:** Keep .tar.gz - no reason to change

**Option B: Keep .tar.gz (CHOSEN)**
- Pros: Already working, universally supported, consistent with existing recipe
- Cons: None
- **Decision:** Keep .tar.gz format

## Files to Modify

### 1. curl Recipe File
**Path:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/c/curl.toml`

**Current State (lines 12-19):**
```toml
[[steps]]
action = "download_file"
url = "https://curl.se/download/curl-8.11.1.tar.gz"
checksum = "a889ac9dbba3644271bd9d1302b5c22a088893719b72be3487bc3d401e5c4e80"

[[steps]]
action = "extract"
archive = "curl-8.11.1.tar.gz"
format = "tar.gz"
```

**Current State (lines 25-29):**
```toml
[[steps]]
action = "configure_make"
source_dir = "curl-8.11.1"
configure_args = ["--with-openssl", "--with-zlib", "--disable-silent-rules", "--without-libpsl"]
executables = ["curl"]
```

**Updated State:**
```toml
[[steps]]
action = "download"
url = "https://curl.se/download/curl-{version}.tar.gz"

[[steps]]
action = "extract"
archive = "curl-{version}.tar.gz"
format = "tar.gz"

# ... setup_build_env step unchanged ...

[[steps]]
action = "configure_make"
source_dir = "curl-{version}"
configure_args = ["--with-openssl", "--with-zlib", "--disable-silent-rules", "--without-libpsl"]
executables = ["curl"]
```

**Changes:**
1. Replace `download_file` with `download` (line 13)
2. Change URL from `curl-8.11.1.tar.gz` to `curl-{version}.tar.gz` (line 14)
3. Remove `checksum` parameter (line 15) - will be computed automatically
4. Update archive name from `curl-8.11.1.tar.gz` to `curl-{version}.tar.gz` (line 18)
5. Update source_dir from `curl-8.11.1` to `curl-{version}` (line 26)

**Note:** The version source block (lines 8-10) already has the correct Homebrew configuration and requires no changes.

## Implementation Steps

### Phase 1: Verify Current State
1. Read current curl recipe to confirm structure matches expected state
2. Verify Homebrew version source is configured correctly (should be `source = "homebrew"`, `formula = "curl"`)
3. Run `tsuku install curl` to establish baseline functionality
4. Verify current installed version matches 8.11.1 (or latest Homebrew version)

### Phase 2: Update Recipe File
1. Open `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/c/curl.toml`
2. Locate the `download_file` step (line 13)
3. Change `action = "download_file"` to `action = "download"`
4. Change `url = "https://curl.se/download/curl-8.11.1.tar.gz"` to `url = "https://curl.se/download/curl-{version}.tar.gz"`
5. Remove the `checksum = "..."` line entirely
6. Locate the `extract` step (line 18)
7. Change `archive = "curl-8.11.1.tar.gz"` to `archive = "curl-{version}.tar.gz"`
8. Locate the `configure_make` step (line 26)
9. Change `source_dir = "curl-8.11.1"` to `source_dir = "curl-{version}"`
10. Save file

### Phase 3: Local Testing
1. Build tsuku binary: `go build -o tsuku ./cmd/tsuku`
2. Clear any cached state: `rm -rf ~/.tsuku/tools/curl-*` (if testing fresh install)
3. Resolve current Homebrew version: Check what version Homebrew formula currently provides
4. Install curl recipe: `./tsuku install curl --force`
5. Verify installation succeeds
6. Check installed path matches version: `~/.tsuku/tools/curl-{version}/bin/curl`
7. Run verification: `curl --version` should show correct version
8. Test HTTPS: `curl -sS https://example.com` should succeed

### Phase 4: Validation Tests
1. **Version Resolution Test:**
   - Verify the recipe resolves to the current Homebrew curl version
   - Check that the version in `~/.tsuku/tools/curl-*` directory matches Homebrew

2. **Checksum Computation Test:**
   - Verify the download action computed a checksum during plan generation
   - Check that cached download includes checksum metadata

3. **Functional Test:**
   - Run `curl --version` and verify output
   - Check that OpenSSL backend is present
   - Check that zlib support is present
   - Test HTTPS request to validate TLS functionality

4. **Cross-Platform Test:**
   - Test on Linux (ubuntu-latest or current system if Linux)
   - Test on macOS (if available)
   - Verify behavior is consistent across platforms

### Phase 5: CI Verification
1. Push changes to feature branch
2. Wait for CI to run existing curl tests in `.github/workflows/build-essentials.yml`
3. Verify test-curl job passes on all platforms:
   - Linux x86_64
   - macOS Intel
   - macOS Apple Silicon
4. Check CI logs for any warnings or unexpected behavior
5. Verify all verification steps pass (version check, OpenSSL check, HTTPS test)

## Testing Strategy

### Unit Tests
**Scope:** Recipe parsing and template expansion

**Existing Tests:** Recipe loading tests in `internal/recipe/types_test.go` should pass without changes

**Template Expansion Tests:**
- Verify `{version}` expands correctly in download URL
- Verify `{version}` expands correctly in archive name
- Verify `{version}` expands correctly in source_dir

**Note:** These are already covered by existing action tests, no new tests needed

### Functional Tests
**Scope:** End-to-end installation

**Test 1: Fresh Installation**
- Remove any existing curl installation
- Run `tsuku install curl`
- Verify:
  - Homebrew version resolved correctly
  - Download URL constructed with correct version
  - Archive extracted to directory with correct version
  - configure_make runs in correct source directory
  - Binary installed to versioned directory

**Test 2: Version Consistency**
- Query Homebrew for expected curl version
- Install curl via tsuku
- Verify installed version matches Homebrew version
- Check `curl --version` output matches expected version

**Test 3: Checksum Verification**
- Install curl (checksum computed during plan generation)
- Verify no checksum errors during download
- Verify cached download has checksum metadata
- Re-install with `--force` and verify cached file used (checksum validation)

**Test 4: TLS Functionality**
- Install curl
- Run `curl --version` and verify OpenSSL backend present
- Run `curl https://example.com` and verify success
- Verify certificate validation works (no --insecure flag needed)

### Platform Tests
**Scope:** Cross-platform compatibility (existing CI coverage)

**Platforms Tested by CI:**
- Linux x86_64 (ubuntu-latest)
- macOS Intel (macos-15-intel)
- macOS Apple Silicon (macos-14)

**Test Coverage:**
- Recipe installs successfully on all platforms
- Template expansion works correctly on all platforms
- Version resolution works on all platforms
- Verification script passes on all platforms

**Note:** Existing test-curl job in `.github/workflows/build-essentials.yml` already covers these platforms

### Regression Tests
**Scope:** Verify existing functionality preserved

**Test 1: Dependencies Still Work**
- Verify openssl dependency still installed automatically
- Verify zlib dependency still installed automatically
- Verify setup_build_env still configures build correctly
- Verify configure detects openssl and zlib

**Test 2: RPATH Still Correct**
- Verify set_rpath step still works
- Check curl binary has correct RPATH (Linux) or install_name (macOS)
- Verify curl links to tsuku-provided openssl and zlib
- Verify curl does NOT link to system libraries

**Test 3: Verification Still Works**
- Verify `[verify]` section still works with `{version}` template
- Check pattern matching: `"curl {version}"` should match actual version

## Risks and Mitigations

### Risk 1: Homebrew Version Might Not Be Available at curl.se
**Impact:** Download fails if Homebrew formula points to version not published at curl.se
**Likelihood:** Low - curl.se is the authoritative source, Homebrew formula should always reference published versions
**Mitigation:**
- Test with current Homebrew version before merging
- If issue occurs, check Homebrew formula to understand version mapping
- Fallback: Use static version temporarily while investigating mismatch

### Risk 2: Checksum Computation Takes Extra Time
**Impact:** Plan generation slower because file must be downloaded to compute checksum
**Likelihood:** Certain - this is expected behavior for recipes without checksum_url
**Mitigation:**
- This is acceptable tradeoff for dynamic versioning
- Download is cached and reused for actual installation
- Follows established pattern (ruby, rust) proven in production
- Users won't notice delay during normal install (file downloaded anyway)

### Risk 3: URL Pattern Might Change
**Impact:** Download fails if curl.se changes URL structure
**Likelihood:** Low - curl.se has maintained stable URL pattern for years
**Mitigation:**
- Monitor for errors in CI and user reports
- URL pattern simple and well-established: `https://curl.se/download/curl-{version}.tar.gz`
- Can update recipe if pattern changes (one-time fix)

### Risk 4: Template Variable Not Expanded Correctly
**Impact:** Download or build fails due to literal `{version}` in path
**Likelihood:** Very Low - template expansion well-tested in other recipes
**Mitigation:**
- Test locally before pushing
- CI will catch any expansion failures
- Reference recipes (terraform, rust) prove pattern works

### Risk 5: RPATH Paths Might Break with Version Change
**Impact:** Runtime linking fails because RPATH uses wrong version
**Likelihood:** Very Low - RPATH uses `{libs_dir}` template, not version-specific paths
**Mitigation:**
- Current RPATH already uses `{libs_dir}/openssl-3.6.0/lib` (still hardcoded per issue #653)
- This issue is out of scope for #655
- Version change in curl won't affect dependency versions
- Future fix tracked in #653

### Risk 6: CI Tests Might Fail on Some Platforms
**Impact:** PR blocked until platform-specific issues resolved
**Likelihood:** Low - change is minimal and follows proven patterns
**Mitigation:**
- Test locally on Linux first
- Monitor CI logs carefully for each platform
- Template expansion is platform-independent
- Existing tests already validate cross-platform behavior

## Success Criteria

### Recipe Updates
- [ ] `download_file` action replaced with `download` action
- [ ] Download URL uses `{version}` template: `https://curl.se/download/curl-{version}.tar.gz`
- [ ] Static checksum removed from recipe file
- [ ] Extract archive parameter uses `{version}` template: `curl-{version}.tar.gz`
- [ ] configure_make source_dir uses `{version}` template: `curl-{version}`
- [ ] All other recipe sections unchanged (version source, dependencies, other steps, verify)

### Version Resolution
- [ ] Recipe queries Homebrew formula for curl version
- [ ] Download URL constructed with resolved version (e.g., curl-8.11.1.tar.gz or whatever Homebrew provides)
- [ ] Archive extracted to directory matching version
- [ ] configure_make runs in correctly named source directory
- [ ] Binary installed to version-specific directory (e.g., `~/.tsuku/tools/curl-8.11.1/`)

### Checksum Verification
- [ ] Download action computes checksum during plan generation (Decompose phase)
- [ ] Checksum stored in download cache metadata
- [ ] Download verification passes (no checksum mismatch errors)
- [ ] Cached download reused on subsequent installs (with checksum validation)

### Build Success
- [ ] Recipe downloads source tarball without errors
- [ ] Source extracts to correctly named directory
- [ ] Configure script runs successfully (OpenSSL and zlib detected)
- [ ] Make compiles curl without errors
- [ ] Binary installed to `$TSUKU_HOME/tools/curl-{version}/bin/curl`

### Dependency Integration (Regression Check)
- [ ] openssl dependency still installed automatically
- [ ] zlib dependency still installed automatically
- [ ] setup_build_env still configures PKG_CONFIG_PATH
- [ ] setup_build_env still configures CPPFLAGS and LDFLAGS
- [ ] Configure script still detects openssl and zlib

### TLS Verification (Regression Check)
- [ ] `curl --version` shows correct version (matches Homebrew)
- [ ] `curl --version` output contains "OpenSSL" (TLS backend present)
- [ ] `curl --version` output contains "zlib" or "libz" (compression present)
- [ ] `curl https://example.com` succeeds (functional TLS test)
- [ ] Certificate validation works (no --insecure needed)

### Platform Compatibility
- [ ] Recipe installs successfully on Linux x86_64
- [ ] Recipe installs successfully on macOS Intel (x86_64)
- [ ] Recipe installs successfully on macOS Apple Silicon (arm64)
- [ ] All verification tests pass on all platforms
- [ ] Template expansion works consistently across platforms

### CI Integration
- [ ] Existing test-curl job passes without modifications
- [ ] CI tests all 3 platforms successfully
- [ ] CI verifies OpenSSL backend present
- [ ] CI performs functional HTTPS test
- [ ] No new CI warnings or errors introduced

## Open Questions

### Q1: Should we verify the Homebrew version matches an available curl release?
**Context:** Homebrew formula might point to a version that doesn't exist at curl.se (though unlikely)

**Options:**
1. Trust Homebrew formula always references valid versions
2. Add validation to check URL before attempting download
3. Document expected behavior if mismatch occurs

**Recommendation:** Trust Homebrew (Option 1). If issues arise, they'll be caught during testing and can be addressed case-by-case.

**Status:** Not blocking - proceed with trust-based approach, monitor for issues

### Q2: Should we add a comment explaining why there's no checksum_url?
**Context:** terraform/boundary/vault all use `checksum_url`, curl recipe won't have one

**Options:**
1. Add comment: `# Note: curl.se provides PGP signatures but not SHA256SUMS; checksum computed during plan generation`
2. No comment - rely on code/docs to explain pattern
3. Add reference to issue #655 in comment

**Recommendation:** Add brief comment (Option 1) for clarity - helps future maintainers understand intentional difference from other recipes

**Status:** Not blocking - can be added during implementation

### Q3: Should we test with a specific version or trust current Homebrew version?
**Context:** Testing needs a known version, but Homebrew formula might update during development

**Options:**
1. Test with whatever version Homebrew currently provides
2. Temporarily pin to specific version for testing (8.11.1)
3. Test with multiple versions to verify flexibility

**Recommendation:** Test with current Homebrew version (Option 1). This validates the dynamic versioning works with real-world version resolution.

**Status:** Not blocking - use current Homebrew version

## Implementation Sequence

1. **Verify Baseline** (5 min)
   - Confirm current curl recipe structure
   - Check Homebrew formula current version
   - Verify existing tests pass

2. **Update Recipe File** (10 min)
   - Change download_file to download action
   - Update URL to use {version} template
   - Remove checksum parameter
   - Update archive parameter to use {version} template
   - Update source_dir to use {version} template
   - Add explanatory comment about checksum computation

3. **Local Testing** (20 min)
   - Build tsuku binary
   - Test recipe installation
   - Verify version resolution
   - Check functional tests (curl --version, HTTPS request)
   - Verify dependencies still work
   - Check RPATH/linking still correct

4. **Code Review** (10 min)
   - Review changes line by line
   - Verify template syntax correct
   - Check no unintended changes
   - Ensure consistency with reference recipes

5. **Commit Changes** (5 min)
   - Stage recipe file changes
   - Create commit with conventional commit message
   - Reference issue #655 in commit body

6. **Push and Monitor CI** (15 min)
   - Push to feature branch
   - Wait for CI jobs to start
   - Monitor test-curl job on all platforms
   - Check logs for any errors or warnings

7. **Verify Success** (5 min)
   - Confirm all CI checks pass
   - Review test output for correctness
   - Verify version resolution logged correctly
   - Check no unexpected changes in behavior

**Total Estimated Time:** ~70 minutes

## Dependencies

**Required Before Implementation:**
- ✅ Homebrew version source support - EXISTS
- ✅ download action with template expansion - EXISTS
- ✅ Template variable system - EXISTS
- ✅ Checksum computation in download.Decompose() - EXISTS
- ✅ curl recipe with build infrastructure - EXISTS (issue #554)

**Blocks:**
- No blocking dependencies - this is a refactoring of existing functionality

**Related Issues:**
- Issue #653: RPATH dependency version hardcoding (future enhancement, out of scope)
- Issue #554: Original curl recipe creation (already completed)

## References

### Recipe Examples
- terraform recipe: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/t/terraform.toml`
  - Shows `download` action with `checksum_url` and `{version}` template
- boundary recipe: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/b/boundary.toml`
  - Another example of `download` with `checksum_url` pattern
- rust recipe: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/r/rust.toml`
  - Shows `download` action without `checksum_url` (similar to curl)
- ruby recipe: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/r/ruby.toml`
  - Another example without `checksum_url`
- ncurses recipe: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/n/ncurses.toml`
  - Shows `download_file` with `{version}` template (already uses dynamic versioning)

### Code References
- download action: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/download.go`
  - Decompose method (lines 31-123) computes checksums when not provided
  - Execute method (lines 125-252) handles template expansion
- download_file action: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/download_file.go`
  - Primitive action used by download after decomposition
  - Requires checksum (computed during decompose if not inline)

### Current curl Recipe
- Recipe path: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/recipe/recipes/c/curl.toml`
- CI workflow: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/.github/workflows/build-essentials.yml`
  - test-curl job already exists and tests all platforms

### curl.se Information
- Download site: https://curl.se/download/
- URL pattern: `https://curl.se/download/curl-{version}.tar.gz`
- Checksums: Not provided (PGP signatures available at `curl-{version}.tar.gz.asc`)
- Archive formats: .tar.gz, .tar.bz2, .tar.xz, .zip

### Related Issues
- Issue #655: This implementation (update curl to dynamic versioning)
- Issue #653: RPATH dependency version hardcoding (future enhancement)
- Issue #554: Original curl recipe creation (completed)
