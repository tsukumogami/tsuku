# Pragmatic Review: Issue #1902

## Overview

Issue #1902 migrates six CI workflow files and one test script to read container images from `container-images.json` via `jq` instead of hardcoding them. The implementation uses three consumption patterns: array-based (jq loop over families), matrix-based (load-images job with fromJson), and inline (jq -r lookup).

## Files Changed

1. `.github/workflows/recipe-validation-core.yml`
2. `.github/workflows/test-recipe.yml`
3. `.github/workflows/batch-generate.yml`
4. `.github/workflows/validate-golden-execution.yml`
5. `.github/workflows/platform-integration.yml`
6. `.github/workflows/release.yml`
7. `test/scripts/test-checksum-pinning.sh`

## Findings

### Blocking: None

### Advisory

#### 1. `platform-integration.yml:114-138` -- load-images job adds a full checkout step for one file

The `load-images` job uses `sparse-checkout: container-images.json` to check out a single file, then runs jq to build a matrix. This is a separate job that downstream jobs (`integration`) depend on via `needs: [build, build-dltest, build-dltest-musl, load-images]`. The pattern adds a job serialization point and a full runner allocation for reading one small JSON file.

However, this is the correct approach for GitHub Actions when you need matrix values computed from a file -- the matrix must be defined at the job level from `needs.*.outputs`, so a setup job is required. The sparse checkout minimizes cost. Acceptable.

**Severity**: Advisory -- the pattern is correct for the constraint (GHA matrix must come from job outputs).

#### 2. `test-checksum-pinning.sh:24-28` -- jq availability check is defensive but appropriate

The script now validates `jq` is available at the top with a clear error message. This is slightly defensive for a test script that runs in CI (where jq is always present), but since the script also runs locally (`./scripts/test-checksum-pinning.sh [family]`), the check is warranted. No action needed.

**Severity**: Advisory -- appropriate for a locally-runnable script.

#### 3. No remaining hardcoded images in any target file

Verified via grep: no hardcoded `debian:bookworm`, `fedora:4`, `archlinux:base`, `alpine:3.`, or `opensuse/` references remain in any of the seven target files. The only match across all workflows is a comment in `test.yml:206` (`# image: alpine:3.19`), which is out of scope for this issue. The migration is complete.

## Correctness Assessment

The three consumption patterns are each appropriate for their use site:

- **recipe-validation-core.yml**: Uses `jq -r '.${{ matrix.family }}' container-images.json` in a "Load container image" step, storing the result in `GITHUB_OUTPUT`. The matrix still defines `family` and `install_cmd` inline (those aren't in the JSON), and only the image reference is externalized. Correct -- these matrix entries define per-family behaviors beyond just the image.

- **test-recipe.yml, batch-generate.yml**: Uses the array pattern (`FAMILIES=(...); for f; IMAGES+=(jq ...)`). This builds parallel bash arrays. Correct.

- **platform-integration.yml**: Uses a dedicated `load-images` job with sparse checkout, building a full JSON matrix with `jq -n`. The matrix includes non-image fields (libc, family, arch, runner, dltest_artifact) alongside the container image. Correct -- only the image references are externalized.

- **validate-golden-execution.yml**: Uses inline `jq -r --arg f "$FAMILY" '.[$f] // empty' container-images.json` with error handling for unknown families. Correct.

- **release.yml**: Uses sparse checkout to include `container-images.json` alongside `cmd/tsuku-dltest`, then reads Alpine image inline. Correct.

- **test-checksum-pinning.sh**: Reads the config file with jq, validates jq availability upfront, validates the family key exists in the JSON with a clear error listing valid families. Correct and well-handled.

## Overall

The implementation is straightforward and matches the design doc's Phase 2 intent. No over-engineering, no dead code, no unnecessary abstractions. Each file uses the simplest jq pattern appropriate for its consumption context. The hardcoded images are fully eliminated from the target files.
