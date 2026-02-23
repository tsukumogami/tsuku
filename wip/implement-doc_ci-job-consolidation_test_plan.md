# Test Plan: CI Job Consolidation

Generated from: docs/designs/DESIGN-ci-job-consolidation.md
Issues covered: 7
Total scenarios: 14

---

## Scenario 1: sandbox-multifamily matrix removal
**ID**: [x] scenario-1
**Testable after**: #1891
**Category**: infrastructure
**Commands**:
- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build-essentials.yml'))"`
- `awk '/test-sandbox-multifamily:/,/^  [a-z]/' .github/workflows/build-essentials.yml | grep -c 'strategy:'`
**Expected**: The workflow file is valid YAML. The `test-sandbox-multifamily` job block no longer contains a `strategy:` section. Instead, the job(s) for cmake and ninja each iterate over the 5 families (debian, rhel, arch, alpine, suse) sequentially within a single runner. The total sandbox-multifamily job count is at most 2 (one per tool).
**Status**: passed

---

## Scenario 2: sandbox-multifamily container loop structure
**ID**: [x] scenario-2
**Testable after**: #1891
**Category**: infrastructure
**Commands**:
- `grep -A 100 'test-sandbox-cmake:\|test-sandbox-multifamily:' .github/workflows/build-essentials.yml | grep '::group::'`
- `grep -A 100 'test-sandbox-cmake:\|test-sandbox-multifamily:' .github/workflows/build-essentials.yml | grep 'timeout'`
- `grep -A 100 'test-sandbox-cmake:\|test-sandbox-multifamily:' .github/workflows/build-essentials.yml | grep 'FAILED'`
**Expected**: The consolidated sandbox job uses `::group::` / `::endgroup::` markers for collapsible per-family output, wraps each iteration in `timeout`, and collects failures in an array so all families execute even if one fails.
**Status**: passed

---

## Scenario 3: sandbox-multifamily CI passes on PR
**ID**: [x] scenario-3
**Testable after**: #1891
**Category**: use-case
**Environment**: manual (CI)
**Commands**:
- Push the #1891 PR branch and observe the `Build Essentials` workflow run.
- In the GHA UI, check that the sandbox-multifamily jobs (1 or 2 total) pass and that each family appears in collapsible `::group::` sections within the job log.
- Verify the job step summary shows per-family pass/fail results for both cmake and ninja across all 5 families.
**Expected**: The workflow runs with 2 or fewer sandbox-multifamily jobs instead of 10. All 5 families (debian, rhel, arch, alpine, suse) for both tools show passing results. The total job count for `build-essentials.yml` drops by 8. No existing jobs outside sandbox-multifamily are affected.
**Status**: passed

---

## Scenario 4: integration-linux serialization structure
**ID**: [x] scenario-4
**Testable after**: #1892
**Category**: infrastructure
**Commands**:
- `awk '/integration-linux:/,/^  [a-z]/' .github/workflows/test.yml | grep -c 'strategy:'`
- `grep -A 80 'integration-linux:' .github/workflows/test.yml | grep '::group::'`
- `grep -A 80 'integration-linux:' .github/workflows/test.yml | grep 'FAILED=()'`
- `grep -A 80 'integration-linux:' .github/workflows/test.yml | grep 'TSUKU_HOME.*runner.temp'`
- `grep -A 80 'integration-linux:' .github/workflows/test.yml | grep 'CACHE_DIR\|tsuku-cache/downloads'`
**Expected**: The `integration-linux` job has no matrix strategy. It uses `::group::` markers, failure array, per-test `$TSUKU_HOME` under `runner.temp`, and a shared download cache directory. The `integration-macos` job is unchanged.
**Status**: pending

---

## Scenario 5: integration-linux CI runs all 9 tests in one job
**ID**: [x] scenario-5
**Testable after**: #1892
**Category**: use-case
**Environment**: manual (CI)
**Commands**:
- Push the #1892 PR branch and observe the `Tests` workflow run.
- In the GHA UI, confirm that `integration-linux` appears as a single job (not 9 matrix jobs).
- Expand the job log and verify collapsible groups for all 9 tools: actionlint, btop, argo-cd, bombardier, golang, nodejs, ruff, perl, waypoint-tap.
**Expected**: One `integration-linux` job runs and tests all 9 tools sequentially. Each tool appears in its own `::group::` section. The job passes. The total job count for `test.yml` drops by 8 (from 19 to 11). Queue wait time decreases because 8 fewer jobs compete for runners.
**Status**: pending

---

## Scenario 6: sandbox-tests serialization structure
**ID**: [x] scenario-6
**Testable after**: #1893
**Category**: infrastructure
**Commands**:
- `grep -c 'strategy:' .github/workflows/sandbox-tests.yml`
- `grep -q '::group::' .github/workflows/sandbox-tests.yml && echo found`
- `grep -q 'FAILED' .github/workflows/sandbox-tests.yml && echo found`
- `grep -c 'go build.*tsuku' .github/workflows/sandbox-tests.yml`
**Expected**: The workflow has zero `strategy:` blocks. It contains `::group::` markers and a failure collection array. The `go build` command for tsuku appears exactly once (not repeated per test). The `code-changed` path filter gate and `test-matrix.json` reference are preserved.
**Status**: pending

---

## Scenario 7: sandbox-tests CI runs all 9 sandbox tests in one job
**ID**: [x] scenario-7
**Testable after**: #1893
**Category**: use-case
**Environment**: manual (CI)
**Commands**:
- Push the #1893 PR branch and observe the `Sandbox Tests` workflow run.
- Verify that the workflow creates 2 jobs total: `matrix` and `sandbox-linux`.
- Expand the `sandbox-linux` job log and confirm `::group::` sections for each test tool.
**Expected**: The workflow runs 2 jobs instead of 10, saving 8 runner slots. All 9 tools from `test-matrix.json` `ci.linux` are sandbox-tested in the single job. Each test gets its own `$TSUKU_HOME` so eval+install runs don't interfere.
**Status**: pending

---

## Scenario 8: checksum-pinning container loop structure
**ID**: [x] scenario-8
**Testable after**: #1894
**Category**: infrastructure
**Commands**:
- `awk '/^  checksum-pinning:/,/^  [a-z]/' .github/workflows/integration-tests.yml | grep -c 'matrix:'`
- `awk '/^  checksum-pinning:/,/^  [a-z]/' .github/workflows/integration-tests.yml | grep 'GITHUB_TOKEN'`
- `awk '/^  checksum-pinning:/,/^  [a-z]/' .github/workflows/integration-tests.yml | grep 'timeout'`
- `awk '/^  checksum-pinning:/,/^  [a-z]/' .github/workflows/integration-tests.yml | grep '::group::'`
**Expected**: The `checksum-pinning` job has no matrix strategy. It loops through all 5 families (debian, rhel, arch, alpine, suse) in a single runner. `GITHUB_TOKEN` and `TSUKU_REGISTRY_URL` are passed to each iteration. `timeout` and `::group::` markers are present. No other jobs in `integration-tests.yml` are modified.
**Status**: pending

---

## Scenario 9: checksum-pinning CI passes across all 5 families
**ID**: [x] scenario-9
**Testable after**: #1894
**Category**: use-case
**Environment**: manual (CI)
**Commands**:
- Push the #1894 PR branch and observe the `Integration Tests` workflow run.
- Verify the `checksum-pinning` job is a single job (not 5 matrix jobs).
- Expand the log and confirm per-family `::group::` sections.
- Check the step summary for per-family pass/fail results.
**Expected**: One `checksum-pinning` job tests all 5 families. Each family shows pass in the step summary. The other jobs in `integration-tests.yml` (homebrew-linux, library-integrity, dlopen jobs) remain unchanged with their current matrix strategies.
**Status**: pending

---

## Scenario 10: remaining integration-tests.yml consolidation
**ID**: [x] scenario-10
**Testable after**: #1895
**Category**: infrastructure
**Commands**:
- `grep -cE '^\s{2}[a-z][a-z0-9_-]+:\s*$' .github/workflows/integration-tests.yml`
- `grep -c 'strategy:' .github/workflows/integration-tests.yml`
**Expected**: The workflow file defines exactly 6 top-level jobs: checksum-pinning (1), homebrew-linux (1), library-integrity (1), library-dlopen-glibc (1), library-dlopen-musl (1), library-dlopen-macos (1). There are zero `strategy:` blocks remaining. Each consolidated job uses `::group::` markers, failure arrays, per-test `$TSUKU_HOME` isolation, and `timeout` wrappers.
**Status**: passed

---

## Scenario 11: remaining integration-tests.yml CI passes all tests
**ID**: [x] scenario-11
**Testable after**: #1895
**Category**: use-case
**Environment**: manual (CI)
**Commands**:
- Push the #1895 PR branch and observe the `Integration Tests` workflow run.
- Verify the workflow creates exactly 6 jobs.
- For each consolidated job, expand the log and confirm all expected entries appear in `::group::` sections:
  - homebrew-linux: 4 families (debian, rhel, arch, suse)
  - library-dlopen-glibc: 3 libraries (zlib, libyaml, gcc-libs) on debian
  - library-dlopen-macos: 3 libraries (zlib, libyaml, gcc-libs) on darwin
  - library-dlopen-musl: 3 libraries (zlib, libyaml, gcc-libs) on alpine
  - library-integrity: 2 libraries (zlib, libyaml) on debian
**Expected**: All 6 jobs pass. The total job count for `integration-tests.yml` is 6 (down from 20), saving 14 jobs. Specific checks: homebrew-linux passes GITHUB_TOKEN into Docker containers via `-e GITHUB_TOKEN`; library-dlopen-musl still uses GHA `container:` directive with `golang:1.23-alpine` (not Docker run); library-dlopen-glibc and dlopen-macos build Rust only once per job.
**Status**: passed

---

## Scenario 12: homebrew-linux test serialization in build-essentials
**ID**: [x] scenario-12
**Testable after**: #1896
**Category**: use-case
**Environment**: manual (CI)
**Commands**:
- Push the #1896 PR branch and observe the `Build Essentials` workflow run.
- Verify `test-homebrew-linux` is a single job (not 4 matrix jobs).
- Expand the job log and confirm `::group::` sections for all 4 tools: pkg-config, cmake, gdbm, pngcrush.
- Confirm that both `verify-tool.sh` and `verify-binary.sh` run for each tool.
**Expected**: One `test-homebrew-linux` job replaces 4 matrix jobs, saving 3 runner slots. All 4 tools pass installation and verification. Each tool gets a fresh `$TSUKU_HOME` with a shared download cache. The `make` exclusion comment (see #1581) is preserved in the workflow file. All other jobs in `build-essentials.yml` remain unchanged.
**Status**: passed

---

## Scenario 13: drift-prevention lint validates existing workflows
**ID**: scenario-13
**Testable after**: #1897
**Category**: infrastructure
**Commands**:
- Run the lint script (e.g., `.github/scripts/checks/ci-patterns-lint.sh` or equivalent) locally against all modified workflow files.
**Expected**: The lint check passes on all workflow files that contain container loops. Specifically, it verifies that each container loop includes: `timeout` around `docker run` commands, exit code capture (`$?`), and failure array pattern (`FAILED+=` or equivalent). The lint check also passes on workflows using GHA group serialization.
**Status**: pending

---

## Scenario 14: end-to-end job count reduction on a worst-case PR
**ID**: scenario-14
**Testable after**: #1897
**Category**: use-case
**Environment**: manual (CI)
**Commands**:
- After all issues (#1891-#1897) are merged to main, create a PR that touches Go code, recipes, sandbox code, and test scripts simultaneously (to trigger all four target workflows).
- Count the total jobs created across `test.yml`, `sandbox-tests.yml`, `integration-tests.yml`, and `build-essentials.yml`.
- Alternatively, review the documented before/after comparison in the docs created by #1897.
**Expected**: The worst-case PR job count is approximately 46 (down from ~87), matching the design document's prediction of a ~47% reduction. A typical Go-only PR (triggering `test.yml` and `sandbox-tests.yml`) creates roughly 13 jobs (down from 29). The documented comparison in the CI patterns guide confirms these numbers with actual data from CI runs.
**Status**: pending
