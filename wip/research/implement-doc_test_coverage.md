# Test Coverage Report: system-lib-backfill

**Date**: 2026-02-22
**Issues Completed**: 1864, 1865, 1866, 1867
**Issues Skipped**: None

## Coverage Summary

- **Total scenarios**: 10
- **Executed**: 9
- **Passed**: 9
- **Failed**: 0
- **Skipped**: 1

## Scenario Results

| Scenario | ID | Status | Notes |
|----------|-----|--------|-------|
| generate-registry.py passes after satisfies backfill | scenario-1 | PASSED | 362 recipes validated, no errors |
| all library recipes have satisfies metadata or documented exclusion | scenario-2 | PASSED | 17 recipes with satisfies, 4 with documented exclusions |
| libngtcp2 satisfies resolves homebrew "ngtcp2" alias | scenario-3 | PASSED | TOML confirmed, registry JSON contains correct mapping |
| abseil satisfies resolves homebrew "abseil-cpp" alias | scenario-4 | PASSED | TOML confirmed, registry JSON contains correct mapping |
| no duplicate satisfies claims across the full recipe set | scenario-5 | PASSED | Exit code 0, no duplicate satisfies entries |
| ranked library list exists with required format | scenario-6 | PASSED | File exists with required columns, 15 libraries ranked descending |
| discovery list includes known historical blockers | scenario-7 | PASSED | All 13 known blockers found with correct statuses |
| new library recipes have type=library and satisfies metadata | scenario-8 | PASSED | All 11 library recipes verified, generate-registry.py exit 0 |
| tree-sitter recipe includes versioned alias | scenario-9 | PASSED | tree-sitter@0.25 alias found, generate-registry.py exit 0 |
| test-recipe workflow validates new library recipes across platforms | scenario-10 | SKIPPED | Requires GitHub Actions runners |

## Gap Analysis

### scenario-10: Skipped

- **Reason**: Requires GitHub Actions runners and multi-platform infrastructure (Linux x86_64, Linux arm64, macOS x86_64, macOS arm64)
- **Impact**: Low - This is an environment-dependent scenario that CI handles via the test-recipe.yml workflow. Manual validation of workflow results will be done during PR review process.
- **When to revisit**: After PRs with library recipes are opened and their test-recipe workflows complete. Review the GHA job summaries to confirm pass/fail per platform.

## Implementation Verification

**Core deliverables verified**:

1. Issue #1865 (satisfies backfill):
   - 17 library recipes have satisfies metadata
   - 4 recipes documented (cuda-runtime, mesa-vulkan-drivers, vulkan-loader, libcurl)
   - generate-registry.py validates all entries with no errors

2. Issue #1866 (discovery + ranking):
   - docs/library-backfill-ranked.md exists with proper format
   - All 13 known historical blockers appear with correct status
   - 15 total libraries ranked by block count (descending)

3. Issue #1867 (new library recipes):
   - 11 new library recipes created (libgit2, bdw-gc, pcre2, oniguruma, dav1d, tree-sitter, libevent, libidn2, glib, ada-url, notmuch)
   - All declare type = "library" and include [metadata.satisfies]
   - tree-sitter includes versioned alias (tree-sitter@0.25)
   - generate-registry.py validates all with no errors

## Conclusion

All automatable scenarios passed. The backfill implementation is complete and validation metadata is correct. Issue #1864 (already completed) integrated properly. The single skipped scenario (scenario-10) is expected and will be validated via GitHub Actions CI during PR review.

**Coverage Status**: COMPLETE (9/9 automatable scenarios passed, 1 environment-dependent scenario deferred to CI)
