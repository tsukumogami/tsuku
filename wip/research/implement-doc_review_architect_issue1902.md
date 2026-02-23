# Architect Review: #1902 (ci: migrate workflow container images to centralized config)

## Summary

Issue #1902 migrates six CI workflow files and one test script from hardcoded container image references to reading from `container-images.json` via `jq`. This is Phase 2 of the sandbox image unification design.

## Design Alignment

The implementation follows the design doc's Decision 3 (jq extraction in workflow scripts) precisely. All three consumption patterns described in the design are present:

1. **Inline jq lookup**: `recipe-validation-core.yml` uses `jq -r '.${{ matrix.family }}' container-images.json` in a dedicated step, outputting to `GITHUB_OUTPUT`.
2. **Array-based loop**: `test-recipe.yml` and `batch-generate.yml` build bash arrays by looping over family names and calling `jq -r --arg f "$f" '.[$f]' container-images.json`.
3. **Matrix-based (load-images job)**: `platform-integration.yml` has a dedicated `load-images` job that reads images with `jq` and outputs a JSON matrix consumed by downstream jobs via `fromJson`.

The test script (`test-checksum-pinning.sh`) reads from the config file using `jq -r --arg f "$FAMILY" '.[$f] // empty' "$CONFIG_FILE"` with appropriate error handling for missing families and a `jq` availability check.

## Findings

### Advisory: Stale `alpine:3.19` in commented-out code (`.github/workflows/test.yml:206`)

The commented-out `rust-test-musl` job still references `alpine:3.19`. This is inactive code and won't cause runtime drift, but it could confuse someone who un-comments the block later. The Phase 3 drift-check CI job (#1903) would ideally catch this, but grep-based drift detection typically skips comments. Low priority.

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.github/workflows/test.yml:206`
**Severity**: Advisory

### Advisory: Consumption pattern consistency across workflows

The implementation uses three different patterns for reading images, which is expected given the three different workflow shapes (matrix job, inline docker run, bash array). Each pattern is appropriate for its context. No structural concern here -- just noting the patterns are consistent within each workflow type.

## Verification: No Remaining Hardcoded Images

Searched all active (non-commented) workflow YAML and test scripts for hardcoded image references (`debian:bookworm`, `fedora:`, `archlinux:base`, `alpine:`, `opensuse/`). The only match is the commented-out block in `test.yml:206`. All active consumers now read from `container-images.json`.

## Verification: Data Flow Direction

The data flow matches the design's architecture diagram:
- `container-images.json` (repo root) is the single source of truth
- Go code reads from `internal/containerimages/` package (embed from copied JSON, via #1901)
- CI workflows read the root file with `jq` (this issue)
- Test scripts read the root file with `jq` (this issue)

No circular dependencies introduced. CI and test scripts depend on the config file, not on Go code. The Go code depends on the config file via embed, not on CI.

## Verification: No Parallel Pattern

Before this change, each workflow hardcoded its own image strings -- a parallel pattern by definition. This change removes that parallelism and routes all consumers through the same file. No new parallel pattern introduced.

## Verification: Sparse Checkout Handling

`release.yml` and `platform-integration.yml` use `sparse-checkout` and correctly include `container-images.json` in the sparse checkout list. This ensures the file is available in jobs that don't do full checkouts.

## Overall Assessment

The implementation matches the design doc's intent for Phase 2. The consumption patterns are appropriate for each workflow's structure. All hardcoded image references in active code have been replaced. The single advisory finding (stale commented-out reference) is contained and non-compounding. No blocking issues.
