# Maintainer Review: Issue #1791

**Issue**: fix(ci): align tsuku-llm release artifacts with recipe asset patterns
**File**: `.github/workflows/llm-release.yml` (new file, 338 lines)
**Reviewer focus**: Can the next developer understand and modify this with confidence?

## Summary

The workflow is well-structured with strong documentation. The header comment (lines 1-25) establishes the artifact naming convention, gives concrete examples, explains the macOS suffix omission, and documents the CUDA version coupling with a cross-reference to `cuda-runtime.toml`. The `finalize-release` job's expected artifact list (lines 308-321) acts as a contract that catches drift between the build matrix and the recipe. This is a solid CI file.

Two findings: one advisory naming issue that creates dead configuration, and one advisory note about duplicated computation across jobs.

## Findings

### 1. `matrix.target` declared but never referenced -- dead configuration

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/.github/workflows/llm-release.yml`
**Lines**: 48, 53, 60, 65, 69, 77, 82, 88
**Severity**: Advisory

Every matrix entry declares a `target` field containing the Rust triple (e.g., `aarch64-apple-darwin`, `x86_64-unknown-linux-gnu`), but no step in the `build-llm` job references `${{ matrix.target }}`. The `cargo build` commands at line 164-178 use `cargo build --release` without `--target`, so they build for the host architecture of whatever runner they're on.

The next developer will see these Rust triples and assume they're wired into the build, then waste time searching for where `matrix.target` is consumed. They'll either (a) think it's broken and try to "fix" it by adding `--target`, which would break cross-compilation setup, or (b) wonder if a step was accidentally deleted.

**Suggestion**: Either use `matrix.target` in the cargo build step (`cargo build --release --target ${{ matrix.target }}`) to make the build explicit about its target architecture, or remove the field and add a one-line comment explaining that builds rely on the runner's native architecture. The former is preferable since it makes the workflow resilient to runner image changes.

### 2. Artifact name computation duplicated across three jobs

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/.github/workflows/llm-release.yml`
**Lines**: 101-109 (build-llm), 216-223 (integration-test), 303-304 + 308-321 (finalize-release)
**Severity**: Advisory

The version extraction (`TAG="${GITHUB_REF_NAME}"; VERSION="${TAG#tsuku-llm-v}"`) and artifact name construction (`tsuku-llm-v${VERSION}-${{ matrix.artifact_suffix }}`) appear in three separate jobs. The build and integration-test steps are nearly identical "Compute artifact name" blocks. The finalize-release job reconstructs the same names a third time in an array.

This is a known limitation of GitHub Actions -- jobs can't share shell step definitions. The duplication is a platform constraint, not a design choice, so it's not blocking. But if the naming pattern changes (e.g., adding a Windows variant), three places need updating in sync. The finalize-release `EXPECTED_ARTIFACTS` array at least acts as a safety net that would catch a missed update.

No action needed; noting for awareness.

## What reads well

- The header comment (lines 1-25) is the best part of this file. It gives the naming convention with concrete examples, explains the macOS exception, documents the CUDA version coupling, and cross-references the recipe that must stay in sync. A developer touching this file for the first time knows exactly what the artifact names must look like and why.

- The `finalize-release` job's artifact verification (lines 306-333) is a good defensive measure. It acts as a compile-time assertion for the release: if a matrix entry is added to `build-llm` but not to `EXPECTED_ARTIFACTS`, the release fails before publishing.

- The `artifact_suffix` naming is clear. It separates the human-readable `platform` label (used in job names) from the filename component, even though they currently match. This is the right separation for when they eventually diverge (e.g., adding a display name like "Linux AMD64 (CUDA)" while keeping the suffix as `linux-amd64-cuda`).
