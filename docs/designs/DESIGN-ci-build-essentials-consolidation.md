---
status: Proposed
problem: |
  The Build Essentials workflow allocates 7 separate Linux runners for tool
  tests that share identical setup (checkout, Go install, binary build). Each
  runner spends 1-2 minutes on setup before running a test that takes 1-5
  minutes. Queue pressure from 7 concurrent jobs delays all of them and every
  other workflow waiting for runners. The same workflow already proves the fix
  works: its macOS jobs aggregate 8 tests into a single runner with GHA groups.
decision: |
  Consolidate the 7 Linux tool-test jobs into a single aggregated job using the
  GHA group serialization pattern that macOS already uses. Each test gets its
  own ::group:: section, fresh $TSUKU_HOME, shared download cache, and per-test
  timeout. git-source keeps its apt-get install gettext step inside the loop.
  The No-GCC container test and sandbox-multifamily jobs stay as separate jobs
  since they have different runner requirements.
rationale: |
  The macOS jobs prove this pattern works for exactly these tests. Wall-time
  cost is modest (~20 minutes serial vs ~5 minutes for the longest parallel
  job, offset by eliminating ~9 minutes of redundant setup and variable queue
  waits). The pattern adapts the macOS run_test() implementation with
  Linux-specific additions (PATH management, per-test timeouts, gettext),
  so the risk of novel failure modes is low.
---

# DESIGN: CI Build Essentials Consolidation

## Status

**Status:** Proposed

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

Data from run 22325545073 (2026-02-23) shows the 7 jobs ran between 21:31 and 21:37, with the first starting at 21:31:17 and the last completing at 21:37:13. The workflow also has sandbox-multifamily (2 jobs), No-GCC container (1 job), and macOS (2 jobs aggregated). That's 12 total jobs, 7 of which could become 1.

The CI job consolidation design (PR #1887) addressed the biggest offenders: family-per-job matrices and integration test serialization. It explicitly noted that Build Essentials Linux tool tests "stay 1" per its job topology table. But the macOS side of the same workflow already demonstrates that serializing these exact tests into one job works well and produces clear output.

### Scope

**In scope:**
- Consolidating the 7 Linux tool-test jobs into 1 serialized job
- Preserving the same test coverage (same tools, same verification scripts)
- Handling git-source's special `gettext` dependency within the consolidated job

**Out of scope:**
- Changing the No-GCC container test (requires a different container environment)
- Changing the sandbox-multifamily jobs (already consolidated per the previous design)
- Changing the macOS jobs (already consolidated)
- Adding or removing tools from the test suite
- Changing test scripts or verification logic

## Decision Drivers

- **Proven pattern in same file**: The macOS jobs already serialize 7-8 of these exact tests with GHA groups. The Linux consolidation is a copy of that pattern.
- **Setup waste is measurable**: 7 jobs x ~1.5 min setup = ~10.5 min of runner time spent on checkout+Go+build that could be done once.
- **Queue pressure is the dominant cost**: Even with parallel execution, queue waits often exceed test execution time. Fewer jobs means less contention.
- **Failure signal must remain clear**: Per-tool pass/fail visibility is important for debugging. GHA groups preserve this.
- **git-source needs special treatment**: It requires `sudo apt-get install gettext` before the test runs. This must work inside the serialized loop.

## Considered Options

### Decision 1: How to Consolidate Linux Tool Tests

The 7 Linux tool-test jobs all run on `ubuntu-latest`, all do the same setup, and all follow the same pattern: install a tool, verify it works. The question is whether to serialize them all into one job or split them into groups.

Build times vary from ~1 minute (homebrew tools) to ~4.5 minutes (git-source). The individual durations sum to roughly 21 minutes of raw test time, though shared download cache hits reduce this. Adding ~2 minutes of one-time setup, total wall time should be around 20 minutes. The current parallel approach completes in ~5 minutes for the longest job, but each parallel job also carries 1-2 minutes of setup and variable queue wait.

#### Chosen: Single aggregated job with GHA groups

Put all 7 test sequences into one job, following the exact `run_test()` pattern from `test-macos-arm64`. Each test gets a `::group::` wrapper, its own `$TSUKU_HOME` directory, and a shared download cache. Failures are collected into an array and reported at the end.

The git-source test needs `gettext` installed via `apt-get`. This runs as a one-time step before the test loop, or as a conditional step inside `run_test()` when the tool is `git-source`.

The resulting job looks like this:

```yaml
name: "Linux x86_64: build essentials"
runs-on: ubuntu-latest
timeout-minutes: 90
steps:
  - checkout
  - setup-go
  - build tsuku
  - install gettext (for git-source)
  - run_test "pkg-config" ...
  - run_test "cmake" ...
  - run_test "gdbm" ...
  - run_test "pngcrush" ...
  - run_test "libsixel-source" ... (recipe)
  - run_test "ninja" ...
  - run_test "sqlite-source" ... (recipe)
  - run_test "git-source" ... (recipe)
  - run_test "tls-cacerts" ... (script)
  - run_test "zig" ... (binary verify)
  - report failures
```

#### Alternatives Considered

**Split into 2 jobs by duration**: Group fast tests (homebrew, libsixel, ninja, zig ~7 min total) and slow tests (sqlite, git-source, tls-cacerts ~11 min total) into 2 jobs. This caps wall time at ~11 minutes while still saving 5 runners. Rejected because the complexity of maintaining two groups with balanced timing isn't worth the ~7 minutes saved. The macOS arm64 job already runs 8 tests sequentially for ~8 minutes with no complaints. And the split creates an ongoing maintenance question of which group gets new tests.

**Keep separate jobs, share binary via artifacts**: Build tsuku once in a setup job, upload as an artifact, then download in each test job. This eliminates the per-job build overhead (~1.5 min each, ~10.5 min total) while keeping parallel execution. Rejected because it addresses runner-minute waste but not queue pressure. 7 jobs still compete for slots, just with slightly shorter execution. The CI job consolidation design already considered and rejected this approach for the same reason.

## Decision Outcome

**Chosen: 1A**

### Summary

The 7 separate Linux tool-test jobs in `build-essentials.yml` get replaced by a single job that runs each test sequentially with GHA group markers. The implementation copies the `run_test()` function from the macOS arm64 job, which already handles install, verification, optional binary checking, and failure collection.

The consolidated job starts with the usual setup (checkout, Go, build tsuku), then installs `gettext` once (needed only for git-source, but harmless to install unconditionally). It then calls `run_test()` for each of the current test entries in order:

1. Homebrew tools: pkg-config, cmake, gdbm, pngcrush (individually, with binary verification)
2. Source builds: libsixel-source (recipe), ninja, sqlite-source (recipe), git-source (recipe)
3. TLS CA certs (calls existing test script instead of install+verify)
4. Zig (with binary verification)

Each `run_test()` call creates a fresh `$TSUKU_HOME` directory with a symlinked shared download cache (identical to the macOS pattern). Per-test timeouts use `timeout 600` for source builds and `timeout 300` for others.

The tls-cacerts test is a bit different: it runs a shell script (`test/scripts/test-tls-cacerts.sh`) rather than doing install+verify. The `run_test()` function handles this by accepting an optional `script` parameter, or tls-cacerts just gets its own inline block after the `run_test()` calls.

The No-GCC container test, sandbox-multifamily jobs, and macOS jobs are unchanged.

### Rationale

This is a mechanical transformation. The macOS arm64 job already runs `run_test()` with the same tools (minus git-source and tls-cacerts which have macOS exclusions). Adapting that pattern to Linux and adding the two extra tests is straightforward. The Linux version adds PATH management and per-test timeouts that the macOS version doesn't need, but these are proven patterns from the existing `test-homebrew-linux` job.

The wall-time increase is real but bounded. All 10 tests run sequentially at roughly 20 minutes total (individual durations sum to ~21 minutes, reduced by cache hits, plus ~2 minutes of one-time setup). The current parallel approach completes in ~5 minutes for the longest job, but only after queue waits. These jobs trigger on the same PR events that activate other high-job-count workflows, so they compete for the same runner pool. The CI job consolidation design measured 7-11 minute queue waits for similar test patterns, making serialization faster end-to-end in most cases.

### Trade-offs Accepted

By choosing this option, we accept:

- **~20 minutes wall time for the consolidated job** vs ~5 minutes for the longest parallel job. This is mitigated by eliminating queue wait for 6 runners and ~9 minutes of redundant setup.
- **Single-tool failures are less visible at a glance.** A failing tool won't show as a separate red job in the PR checks. The failure appears in the GHA group output and the job summary. This matches how macOS failures already work.
- **git-source's `apt-get install gettext` runs unconditionally.** This adds ~15 seconds to every run, even when git-source isn't being debugged. The overhead is negligible.
- **A hanging test blocks subsequent tests.** If sqlite-source hangs on a download, zig and tls-cacerts won't run. Per-test timeouts (600s for source builds) mitigate this. The macOS jobs accept the same risk.

These are acceptable because the same trade-offs already exist in the macOS jobs and haven't caused issues.

## Solution Architecture

### Overview

One workflow file change: replace 7 `jobs:` entries in `build-essentials.yml` with a single `test-linux` job that mirrors the existing `test-macos-arm64` structure.

### Job Topology After Consolidation

```
build-essentials.yml (current: 12 jobs)
  test-homebrew-linux           ─┐
  test-meson-build-linux        │
  test-cmake-build-linux        │
  test-sqlite-source-linux      ├── consolidated into: test-linux (1 job)
  test-git-source-linux         │
  test-tls-cacerts-linux        │
  test-zig-linux                ─┘
  test-no-gcc                   → stays (needs custom container)
  test-sandbox-cmake            → stays (container loop)
  test-sandbox-ninja            → stays (container loop)
  test-macos-arm64              → stays (already aggregated)
  test-macos-intel              → stays (already aggregated)

After: 6 jobs (was 12, save 6)
```

### Implementation Pattern

The `run_test()` function adapts the `test-macos-arm64` pattern with three Linux-specific changes: (1) `PATH` management for `$TSUKU_HOME/bin` (needed for dependency resolution in source builds), (2) per-test `timeout` wrapping (an improvement over the macOS version, which relies only on the job-level timeout), and (3) higher timeout for recipe-based source builds (600s vs 300s for binary installs):

The consolidated step's `env:` block must include `GITHUB_TOKEN` and `TSUKU_REGISTRY_URL` (pointing to the PR branch), same as the macOS steps.

```bash
run_test() {
    local name="$1"
    local tool="$2"
    local recipe="$3"
    local verify_binary="${4:-false}"

    echo "::group::Testing $name"

    # Fresh TSUKU_HOME per test with shared download cache
    export TSUKU_HOME="${{ runner.temp }}/tsuku-$name"
    mkdir -p "$TSUKU_HOME/cache"
    ln -s "$CACHE_DIR" "$TSUKU_HOME/cache/downloads" 2>/dev/null || true
    export PATH="$TSUKU_HOME/bin:$PATH"

    # Install
    if [ -n "$recipe" ]; then
        if ! timeout 600 ./tsuku install --recipe "$recipe" --force; then
            FAILED+=("$name")
            echo "::endgroup::"
            return
        fi
    else
        if ! timeout 300 ./tsuku install --force "$tool"; then
            FAILED+=("$name")
            echo "::endgroup::"
            return
        fi
    fi

    # Verify functionality
    if ! ./test/scripts/verify-tool.sh "$tool"; then
        FAILED+=("$name (verify)")
    fi

    # Optional: verify binary quality
    if [ "$verify_binary" = "true" ]; then
        ./test/scripts/verify-binary.sh "$tool" || FAILED+=("$name (binary)")
    fi

    echo "::endgroup::"
}
```

### Handling Special Cases

**Homebrew tools**: Currently tested as a group (4 tools in a loop). In the consolidated job, each gets its own `run_test()` call with `verify_binary=true`, matching the macOS pattern.

**git-source**: Needs `gettext` installed. Add `sudo apt-get update && sudo apt-get install -y gettext` as a step before the test loop.

**tls-cacerts**: Runs a test script instead of install+verify. Add a separate block:
```bash
echo "::group::Testing tls-cacerts"
export TSUKU_HOME="${{ runner.temp }}/tsuku-tls-cacerts"
mkdir -p "$TSUKU_HOME/cache"
ln -s "$CACHE_DIR" "$TSUKU_HOME/cache/downloads" 2>/dev/null || true
if ! timeout 300 ./test/scripts/test-tls-cacerts.sh ./tsuku; then
    FAILED+=("tls-cacerts")
fi
echo "::endgroup::"
```

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

This is a single-file change to `.github/workflows/build-essentials.yml`:

### Step 1: Add the Consolidated Job

Add a new `test-linux` job that follows the `test-macos-arm64` pattern:
- Checkout, setup-go, build tsuku
- Install gettext for git-source
- Define `run_test()` function
- Call `run_test()` for each tool
- Handle tls-cacerts as a special case
- Report failures

### Step 2: Remove the 7 Individual Jobs

Delete `test-homebrew-linux`, `test-meson-build-linux`, `test-cmake-build-linux`, `test-sqlite-source-linux`, `test-git-source-linux`, `test-tls-cacerts-linux`, and `test-zig-linux`.

### Step 3: Validate

The workflow self-triggers on changes to its own file. The PR's CI run will exercise the new consolidated job. Compare results against a recent passing run to confirm identical test coverage.

## Security Considerations

### Download Verification

Not applicable. This change modifies workflow structure (how tests are orchestrated) but doesn't alter how binaries are downloaded or verified. The actual `tsuku install` commands and verification scripts stay the same.

### Execution Isolation

Each test still gets its own `$TSUKU_HOME` directory, same as today. The shared download cache is read-only for practical purposes (each test writes to its own cache location within it). No change to isolation boundaries.

### Supply Chain Risks

Not applicable. No new dependencies or binary sources are introduced. The same tools are tested from the same sources. The `gettext` package was already installed by `apt-get` in the `test-git-source-linux` job; now it's installed in the consolidated job instead.

### User Data Exposure

CI workflows don't access user data. The only secret involved is `GITHUB_TOKEN` (for API rate limits during downloads). Token exposure is unchanged: the same environment variable is passed to the same test commands.

## Consequences

### Positive

- **6 fewer runner allocations** per trigger of `build-essentials.yml`, reducing queue contention for all workflows.
- **~10 minutes less total runner time** from eliminating redundant checkout+Go+build setup across 7 jobs.
- **Consistent pattern** across the workflow: both Linux and macOS now use the same GHA group serialization approach.
- **Simpler workflow file**: 7 job definitions replaced by 1, removing ~150 lines of duplicated YAML.

### Negative

- **Higher wall time for the consolidated job**: ~20 minutes serial vs ~5 minutes for the longest parallel job. Offset by queue wait savings (the original CI consolidation design measured 7-11 minutes for similar patterns) and ~9 minutes of eliminated redundant setup.
- **Coarser failure signal**: A failing tool shows in the job logs and summary rather than as a separate red check. This matches the macOS behavior and hasn't been a problem there.
- **Correlated failure risk**: If the runner hits a network issue during one test, subsequent tests may also fail. Mitigated by per-test `$TSUKU_HOME` isolation and per-test timeouts.

### Mitigations

- GHA `::group::` markers provide collapsible, per-test output in the workflow logs.
- Failure arrays collect all failing tests, so the summary reports every failure even if multiple tests fail.
- Per-test timeouts (600s for source builds, 300s for others) prevent a single hanging test from blocking the entire job.
