---
status: Accepted
problem: |
  The Build Essentials workflow allocates 7 separate Linux runners for tool
  tests that share identical setup (checkout, Go install, binary build). Each
  runner spends 1-2 minutes on setup before running a test that takes 1-5
  minutes. Queue pressure from 7 concurrent jobs delays all of them and every
  other workflow waiting for runners. The same workflow already proves the fix
  works: its macOS jobs aggregate 8 tests into a single runner with GHA groups.
decision: |
  Consolidate the 7 Linux tool-test jobs into a single aggregated job. Each test
  runs via tsuku install --sandbox, which handles containerized execution, system
  dependency installation, and recipe verification automatically. GHA groups
  provide per-tool log output. The TLS test stays host-level since it
  orchestrates across recipe boundaries. The No-GCC container test and
  sandbox-multifamily jobs stay separate since they have different runner
  requirements.
rationale: |
  Sandbox handles the hard parts: system dependencies (no more sudo apt-get
  install gettext), per-test isolation (each install runs in a fresh container),
  and verification (sandbox runs the recipe's verify command inside the
  container). This eliminates the manual TSUKU_HOME/PATH/cache management that
  the macOS run_test() pattern requires. PR #1935 already migrated 4 workflows
  to sandbox with the same pattern. The sandbox-multifamily jobs already prove
  sandbox works for cmake and ninja builds across 5 Linux families.
---

# DESIGN: CI Build Essentials Consolidation

## Status

**Status:** Accepted

## Upstream Design Reference

This design continues the work from [DESIGN-ci-job-consolidation.md](current/DESIGN-ci-job-consolidation.md), which consolidated family-per-job matrices and integration test serialization. That design reduced worst-case PR jobs from ~87 to ~46 but left individual Build Essentials Linux tool tests as separate jobs. This design addresses that remaining gap.

## Context and Problem Statement

The `build-essentials.yml` workflow tests source builds, homebrew installs, and specialized tool behavior across Linux and macOS. macOS tests are already consolidated: one job for Apple Silicon and one for Intel, each running 7-8 tests sequentially with GHA groups. But Linux has 7 separate jobs, each testing a single tool:

| Job Name | What It Tests | Typical Duration |
|----------|--------------|-----------------|
| Linux x86_64: homebrew tools | pkg-config, cmake, gdbm, pngcrush | ~1 min |
| Linux x86_64: libsixel-source | meson_build action | ~2.5 min |
| Linux x86_64: ninja | cmake_build action | ~2 min |
| Linux x86_64: sqlite-source | configure_make with dep chain | ~3 min |
| Linux x86_64: git-source | configure_make multi-dep build | ~4.5 min |
| Linux x86_64: tls-cacerts | TLS cert discovery | ~3 min |
| Linux x86_64: zig | zig installation + verification | ~2 min |

Every one of these jobs repeats the same setup: checkout the repo, install Go, build tsuku. That's 1-2 minutes of overhead per job, multiplied by 7 jobs. They all run on `ubuntu-latest`, so there's no runner-type difference justifying separate allocations.

Some tests require manual host setup: git-source needs `sudo apt-get install gettext` before the recipe runs. Each test manages its own `$TSUKU_HOME` and `PATH`. This manual dependency management is fragile and inconsistent with how the sandbox-multifamily jobs (already in the same workflow) handle things.

Data from run 22325545073 (2026-02-23) shows the 7 jobs ran between 21:31 and 21:37, with the first starting at 21:31:17 and the last completing at 21:37:13. The workflow also has sandbox-multifamily (2 jobs), No-GCC container (1 job), and macOS (2 jobs aggregated). That's 12 total jobs, 7 of which could become 1.

The CI job consolidation design (PR #1887) addressed the biggest offenders: family-per-job matrices and integration test serialization. It explicitly noted that Build Essentials Linux tool tests "stay 1" per its job topology table. Since then, PR #1935 added sandbox support to tsuku and migrated 4 Linux CI workflows to use `tsuku install --sandbox`. PR #1869 established the pattern for sandbox-based recipe testing: `--sandbox --force --json` with `--env` for token passthrough, JSON result parsing, and `$GITHUB_STEP_SUMMARY` tables. The sandbox-multifamily jobs in this same workflow already prove that sandbox handles cmake and ninja builds across 5 Linux families.

### Scope

**In scope:**
- Consolidating the 7 Linux tool-test jobs into 1 job using sandbox for execution
- Preserving the same test coverage (same tools, same recipes)

**Out of scope:**
- Changing the No-GCC container test (requires a custom container without gcc)
- Changing the sandbox-multifamily jobs (different purpose: cross-family coverage)
- Changing the macOS jobs (already consolidated; no sandbox on darwin)
- Adding or removing tools from the test suite

## Decision Drivers

- **Sandbox is already proven for these builds**: The sandbox-multifamily jobs run cmake and ninja via `--sandbox` across 5 Linux families in this same workflow. PR #1935 migrated 4 recipe validation workflows to sandbox.
- **Setup waste is measurable**: 7 jobs x ~1.5 min setup = ~10.5 min of runner time spent on checkout+Go+build that could be done once.
- **Queue pressure is the dominant cost**: Even with parallel execution, queue waits often exceed test execution time. Fewer jobs means less contention.
- **System dependency management is fragile**: git-source requires `sudo apt-get install gettext`, which is manually added to the workflow. Sandbox handles system dependencies automatically from the installation plan.
- **Failure signal must remain clear**: Per-tool pass/fail visibility matters for debugging. GHA groups preserve this.

## Considered Options

### Decision 1: Execution Model for Consolidated Linux Tests

The 7 Linux tool-test jobs all run on `ubuntu-latest`, all do the same setup, and all follow the same pattern: install a tool, verify it works. Once consolidated onto a single runner, the question is whether each test should run natively on the host or inside a sandbox container.

Build times vary from ~1 minute (homebrew tools) to ~4.5 minutes (git-source). The individual durations sum to roughly 21 minutes of raw test time, though shared download cache hits reduce this. Container image builds add some overhead on first run but are cached after that.

#### Chosen: Sandbox execution with GHA groups

Each test runs as `tsuku install --sandbox --force <tool>` inside the consolidated job. Sandbox creates a container, installs system dependencies from the plan, runs the install, and executes the recipe's verify command -- all without host-level setup. GHA `::group::` markers wrap each test for collapsible output.

The TLS test (`test-tls-cacerts`) is the one exception: it orchestrates across two recipes (ca-certificates + curl-source) and then tests HTTPS behavior with different environment configurations. This multi-recipe orchestration doesn't fit the sandbox model (one recipe per invocation). The TLS test stays host-level using the existing `test-tls-cacerts.sh` script.

The resulting job follows the pattern established by `test-recipe.yml` (PR #1869) for Linux sandbox testing: `--sandbox --force --json` with `--env` for token passthrough, JSON result parsing, and a `$GITHUB_STEP_SUMMARY` results table.

```yaml
name: "Linux: build essentials"
runs-on: ubuntu-latest
timeout-minutes: 90
steps:
  - checkout
  - setup-go
  - build tsuku
  - name: Test tools via sandbox
    env:
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      TSUKU_REGISTRY_URL: https://raw.githubusercontent.com/${{ github.repository }}/${{ github.head_ref || github.ref_name }}
    run: |
      TOTAL=0
      PASSED=0
      FAILED=0
      RESULTS=""

      sandbox_test() {
          local name="$1"
          shift
          TOTAL=$((TOTAL + 1))
          echo "::group::Sandbox: $name"
          RESULT_FILE=".result-${name}.json"

          ./tsuku install --sandbox --force "$@" \
            --env GITHUB_TOKEN="$GITHUB_TOKEN" \
            --json > "$RESULT_FILE" 2>/dev/null || true

          if [ -f "$RESULT_FILE" ] && jq -e . "$RESULT_FILE" > /dev/null 2>&1; then
            SANDBOX_PASSED=$(jq -r '.passed' "$RESULT_FILE")
            EXIT_CODE=$(jq -r '.install_exit_code' "$RESULT_FILE")
          else
            SANDBOX_PASSED="false"
            EXIT_CODE=1
          fi

          if [ "$SANDBOX_PASSED" = "true" ]; then
            PASSED=$((PASSED + 1))
            RESULTS="$RESULTS| $name | pass |\n"
          else
            FAILED=$((FAILED + 1))
            RESULTS="$RESULTS| $name | **FAIL** (exit $EXIT_CODE) |\n"
          fi
          echo "::endgroup::"
      }

      # Registry tools (homebrew bottles, cmake_build, binary)
      for tool in pkg-config cmake gdbm pngcrush ninja zig; do
        sandbox_test "$tool" "$tool"
      done

      # Testdata recipes (source builds)
      for recipe in libsixel-source sqlite-source git-source; do
        sandbox_test "$recipe" --recipe "testdata/recipes/${recipe}.toml"
      done

      # TLS test (host-level, multi-recipe orchestration)
      echo "::group::TLS CA certs"
      TOTAL=$((TOTAL + 1))
      if timeout 300 ./test/scripts/test-tls-cacerts.sh ./tsuku; then
        PASSED=$((PASSED + 1))
        RESULTS="$RESULTS| tls-cacerts | pass |\n"
      else
        FAILED=$((FAILED + 1))
        RESULTS="$RESULTS| tls-cacerts | **FAIL** |\n"
      fi
      echo "::endgroup::"

      # Job summary
      {
        echo "### Linux Build Essentials Results"
        echo ""
        echo "Passed: $PASSED / $TOTAL"
        echo ""
        echo "| Tool | Status |"
        echo "|------|--------|"
        echo -e "$RESULTS"
      } >> "$GITHUB_STEP_SUMMARY"

      if [ "$FAILED" -gt 0 ]; then
        echo "::error::$FAILED of $TOTAL tests failed"
        exit 1
      fi
```

What sandbox handles automatically:
- **System dependencies**: git-source's `gettext` is extracted from the installation plan and installed in the container image. No `sudo apt-get` needed on the host.
- **Build tools**: Source builds (configure_make, cmake_build, meson_build) trigger automatic resource upgrade to 4GB/4CPU/15min and installation of build toolchains (gcc, make, autotools).
- **Isolation**: Each `--sandbox` call creates a fresh container workspace. No `$TSUKU_HOME` or `PATH` management needed on the host.
- **Verification**: Sandbox runs the recipe's verify command (e.g., `cmake --version`) inside the container after install succeeds.
- **Structured output**: `--json` produces machine-readable results (`passed`, `install_exit_code`, `verified`, `duration_ms`) for result aggregation.

#### Alternatives Considered

**Native host execution with GHA groups (macOS run_test pattern)**: Serialize all 7 tests on the host using the `run_test()` function pattern from `test-macos-arm64`. Each test gets a fresh `$TSUKU_HOME`, shared download cache, and per-test timeout. Rejected because it requires manual dependency management (`sudo apt-get install gettext`), manual `$TSUKU_HOME`/`PATH` setup per test, and doesn't match the direction CI is moving (sandbox-based testing). The macOS jobs use this pattern because sandbox doesn't support darwin, but there's no reason to use it on Linux when sandbox is available.

**Keep separate jobs, share binary via artifacts**: Build tsuku once in a setup job, upload as an artifact, then download in each test job. Rejected because it addresses runner-minute waste but not queue pressure. 7 jobs still compete for slots.

## Decision Outcome

**Chosen: 1A (Sandbox execution)**

### Summary

The 7 separate Linux tool-test jobs in `build-essentials.yml` get replaced by a single job that runs each test via `tsuku install --sandbox`. The job builds tsuku once, then loops through each tool with GHA group markers for output separation. Sandbox handles containerized execution, system dependency installation, and recipe verification for each tool independently.

For 6 of the 7 tests, the pattern is identical: `./tsuku install --sandbox --force <tool-or-recipe>`. The sandbox executor detects whether the plan contains build actions (configure_make, cmake_build, meson_build), automatically upgrades container resources for source builds, and runs the recipe's verify command after installation.

The TLS test is the exception. It installs ca-certificates and curl-source in the same `$TSUKU_HOME`, then runs HTTPS connectivity tests with different SSL environment configurations. This cross-recipe orchestration runs on the host using the existing `test-tls-cacerts.sh` script. The TLS test needs its own `$TSUKU_HOME` but doesn't need manual dependency installation since both recipes it uses handle their own dependencies.

The No-GCC container test, sandbox-multifamily jobs, and macOS jobs are unchanged. The sandbox-multifamily jobs serve a different purpose (cross-family coverage for cmake and ninja across 5 Linux families) and are complementary to this consolidation.

### Rationale

Sandbox eliminates the category of problems the macOS pattern has to manage manually: system dependency installation, workspace isolation, PATH management, and verification scripting. The sandbox infrastructure already exists and is proven in this same workflow (sandbox-multifamily cmake/ninja) and in 4 other workflows (PR #1935). Using it here is consistent with the CI direction and reduces the amount of shell scripting in the workflow file.

The wall-time increase is real but bounded. Source builds with container image construction take roughly the same time as native builds since the container images are cached after first build. Binary installs (zig, homebrew tools) are faster in sandbox since there's no host setup overhead beyond the `tsuku install --sandbox` call itself.

### Trade-offs Accepted

- **Container image build overhead on first run.** The first sandbox invocation for a given package set builds a container image. Subsequent invocations with the same system requirements reuse the cached image. For CI, this means the first PR run after a dependency change is slower, but subsequent runs are cached.
- **No verify-binary.sh checks.** The current homebrew and zig tests run `verify-binary.sh` which checks ELF linking, RPATH, and dynamic dependencies using `readelf`/`ldd`. Sandbox doesn't include these host-level analysis tools. The recipe's built-in verify command (e.g., `zig version`) covers functionality. Binary quality checks could move to a separate focused job if needed, but they haven't caught issues that verify-tool.sh missed.
- **TLS test stays host-level.** It can't use sandbox because it orchestrates across recipe boundaries (installs two recipes, then tests their interaction). This is acceptable since the TLS test is an integration test, not a build test.
- **Coarser failure signal.** A failing tool won't show as a separate red job in PR checks. Failures appear in GHA group output and the job summary. This matches how macOS failures work.

## Solution Architecture

### Overview

One workflow file change: replace 7 `jobs:` entries in `build-essentials.yml` with a single `test-linux` job that runs each test via `tsuku install --sandbox`.

### Job Topology After Consolidation

```
build-essentials.yml (current: 12 jobs)
  test-homebrew-linux           -+
  test-meson-build-linux         |
  test-cmake-build-linux         |
  test-sqlite-source-linux       +-- consolidated into: test-linux (1 job)
  test-git-source-linux          |
  test-tls-cacerts-linux         |
  test-zig-linux                -+
  test-no-gcc                   -> stays (needs custom container without gcc)
  test-sandbox-cmake            -> stays (cross-family coverage)
  test-sandbox-ninja            -> stays (cross-family coverage)
  test-macos-arm64              -> stays (already aggregated, no sandbox on darwin)
  test-macos-intel              -> stays (already aggregated, no sandbox on darwin)

After: 6 jobs (was 12, save 6)
```

### Implementation Pattern

The consolidated job uses a `sandbox_test()` helper that wraps each test in a GHA group, captures `--json` output, and records results. This follows the same pattern as `test-recipe.yml` (PR #1869), which tests recipe changes via sandbox across 5 Linux families.

**Sandbox tests** (9 of 10 tests): Each tool gets a `sandbox_test()` call that runs `tsuku install --sandbox --force --json` and parses the JSON result for pass/fail. Sandbox automatically detects build actions (configure_make, cmake_build, meson_build) and upgrades container resources to 4GB/4CPU/15min for source builds.

**TLS integration test** (1 test): Runs host-level since it orchestrates across recipe boundaries (installs ca-certificates + curl-source, then tests HTTPS behavior). Gets its own `$TSUKU_HOME` but doesn't need manual dependency installation.

### Handling Special Cases

**git-source's gettext dependency**: Currently handled by `sudo apt-get install -y gettext` on the host before the recipe runs. With sandbox, the container image is built from the installation plan's system requirements. If gettext is declared as a dependency in the git-source recipe, sandbox installs it automatically. If it isn't, the recipe needs updating to declare it. Either way, no `sudo apt-get` on the host.

**Homebrew tools on Linux**: Sandbox handles homebrew bottle downloads, extraction, and relocation (via patchelf, which is automatically included in the container image). This is the same mechanism the sandbox-multifamily cmake job uses.

**Source builds**: Sandbox detects build actions and automatically installs build toolchains (gcc, make, autotools, etc.) and upgrades resource limits. No manual package installation needed.

### Expected Job Count Impact

| Component | Before | After | Change |
|-----------|--------|-------|--------|
| Linux tool tests | 7 | 1 | -6 |
| No-GCC container | 1 | 1 | 0 |
| Sandbox multifamily | 2 | 2 | 0 |
| macOS (arm64 + Intel) | 2 | 2 | 0 |
| **Total** | **12** | **6** | **-6** |

For a PR that triggers `build-essentials.yml`, this saves 6 runner allocations.

## Implementation Approach

This is a single-file change to `.github/workflows/build-essentials.yml`, with a possible recipe fix for git-source:

### Step 1: Verify git-source Recipe Dependencies

Check whether git-source's recipe declares `gettext` as a dependency. If not, add it so sandbox can install it automatically. This is a recipe correctness fix independent of this consolidation.

### Step 2: Add the Consolidated Job

Add a new `test-linux` job:
- Checkout, setup-go, build tsuku
- Loop through registry tools with `--sandbox`
- Loop through test recipes with `--sandbox`
- Run TLS test host-level
- Report failures

### Step 3: Remove the 7 Individual Jobs

Delete `test-homebrew-linux`, `test-meson-build-linux`, `test-cmake-build-linux`, `test-sqlite-source-linux`, `test-git-source-linux`, `test-tls-cacerts-linux`, and `test-zig-linux`.

### Step 4: Validate

The workflow self-triggers on changes to its own file. The PR's CI run will exercise the new consolidated job. Compare sandbox output against a recent passing run to confirm identical test coverage.

## Security Considerations

### Download Verification

Not applicable. This change modifies workflow structure (how tests are orchestrated) but doesn't alter how binaries are downloaded or verified. The same recipes, checksums, and verification commands run inside sandbox containers.

### Execution Isolation

Sandbox improves isolation compared to the current approach. Today, tests run on the native ubuntu-latest host and can affect each other through shared system state. With sandbox, each test runs in an independent container with its own filesystem. The TLS test (the one host-level exception) gets its own `$TSUKU_HOME` directory.

### Supply Chain Risks

Not applicable. No new dependencies or binary sources are introduced. Sandbox uses the same container images as the existing sandbox-multifamily jobs (debian:bookworm-slim as the base for the default family).

### User Data Exposure

CI workflows don't access user data. `GITHUB_TOKEN` is passed via `--env` to sandbox containers for API rate limits. This is the same pattern used by the existing sandbox-multifamily and recipe validation workflows.

## Consequences

### Positive

- **6 fewer runner allocations** per trigger of `build-essentials.yml`, reducing queue contention for all workflows.
- **~10 minutes less total runner time** from eliminating redundant checkout+Go+build setup across 7 jobs.
- **No manual dependency management**: Sandbox handles system deps (gettext, build tools, patchelf) automatically from installation plans. No more `sudo apt-get` in the workflow.
- **Better isolation**: Each test runs in an independent container. No shared host state between tests.
- **Consistent CI direction**: All Linux build testing uses sandbox, matching the pattern from PR #1935 and the existing sandbox-multifamily jobs.

### Negative

- **Container image build overhead**: First run builds container images from scratch. Subsequent runs with the same system requirements reuse cached images. For PRs that change recipes, the first CI run pays this cost.
- **Loss of verify-binary.sh**: ELF linking and RPATH checks don't run in sandbox containers (no readelf/ldd). These checks could move to a separate job if needed.
- **TLS test stays host-level**: The multi-recipe orchestration pattern doesn't fit sandbox's one-recipe-per-invocation model.
- **Coarser failure signal**: Individual tool failures show in GHA group output, not as separate checks. Same trade-off macOS already accepts.

### Mitigations

- GHA `::group::` markers provide collapsible, per-tool output in the workflow logs.
- Failure arrays collect all failing tests, so the summary reports every failure.
- Per-test timeouts (600s for source builds, 300s for others) prevent a single hanging test from blocking the entire job.
- Sandbox's `--json` flag provides structured output for machine-readable results if needed in the future.
