# Architect Review: DESIGN-ci-job-consolidation.md

## 1. Problem Statement Specificity

The problem statement is strong in some areas and weak in others.

**What works:** The concrete examples are good. Naming specific workflows (`integration-tests.yml`, `build-essentials.yml`, `test.yml`) with specific job counts grounds the problem in verifiable facts. The contrast with `test-recipe.yml` (which already solves the problem) gives a clear target state.

**What's missing:**

- **No baseline measurement of queue wait times.** The entire argument rests on the claim that "queue wait often exceeds [test run time]" (line 124), but no data supports this. The design should include at least one example: "On PR X, the checksum-pinning jobs waited Y minutes in queue and ran for Z minutes." Without this, the queue-time benefit is assumed, not demonstrated.

- **No definition of "typical PR touching Go code and recipes."** The claim "triggers 50-80 jobs" (line 9) is unverified. A PR touching only Go code wouldn't trigger `build-essentials.yml` (gated on specific paths) or `integration-tests.yml` (also path-gated) or `test-recipe.yml` (gated on recipe file changes). The actual number depends heavily on which paths change. The design should distinguish between worst-case (touches everything) and typical-case (touches Go code only).

- **The workflow file count is wrong.** The design says "53 GitHub Actions workflow files" (line 9). The actual count is 51. Minor, but it undermines precision claims.

## 2. Missing Alternatives

### 2a. Caching the Go binary as a shared artifact

The design identifies "checkout + Go install + binary build" as redundant setup costing 1-2 minutes per job. The chosen solution serializes tests to amortize this cost. But there's a simpler option the design doesn't consider: build tsuku once in a setup job, upload it as an artifact, and download it in each matrix job.

`test-recipe.yml` already does exactly this (lines 42-65): it cross-compiles tsuku for all targets in a `build` job, uploads the binaries, and downstream jobs download them. `platform-integration.yml` does the same thing (lines 13-35). This pattern eliminates the setup overhead while preserving parallel execution. The amortized setup cost per downstream job becomes ~10 seconds (artifact download) instead of 1-2 minutes.

This alternative deserves analysis because it preserves the parallelism advantage while removing most of the overhead that motivates consolidation. It would weaken the queue-pressure argument (same number of jobs) but fully address the runner-minutes argument (no repeated builds).

### 2b. `sandbox-tests.yml` is a consolidation target but isn't mentioned

`sandbox-tests.yml` uses the same `test-matrix.json` linux list as `test.yml` and expands it into 9 separate jobs, each doing checkout + Go setup + build. It triggers on every PR that changes Go code or recipes. The design's inventory of consolidation targets doesn't include it.

Verifiable from `sandbox-tests.yml` lines 37-45: the `sandbox-tests` job uses `matrix.test: ${{ fromJson(needs.matrix.outputs.linux) }}` which expands to 9 entries. That's 9 more jobs the design could consolidate into 1, bringing the total savings closer to 42 instead of 33.

### 2c. GitHub Actions `concurrency` groups as a lighter intervention

For the queue pressure problem specifically, concurrency groups with `cancel-in-progress: true` would ensure that when a new push arrives, the previous run's jobs are cancelled immediately rather than competing for runners. Some workflows already use this (e.g., `test-recipe.yml` line 30-31). Applying it to `test.yml`, `integration-tests.yml`, and `build-essentials.yml` would reduce queue contention from sequential pushes without changing job structure. Not a replacement for consolidation, but an additive measure the design doesn't discuss.

## 3. Rejection Rationale Evaluation

### "Family-per-job matrix" (Decision 1, rejected)

The rejection says "the parallelism benefit is minimal since most family tests complete in under 5 minutes and the queue wait often exceeds that." This is an empirical claim stated without evidence. If the tests complete in 2 minutes each and queue wait is 30 seconds, the parallelism benefit is significant. If queue wait is 5 minutes, it isn't. The rejection should include actual timing data, or at least acknowledge the claim is unverified.

### "Batch into groups of 3" (Decision 2, rejected)

The rejection says "partial consolidation captures less benefit and adds complexity deciding which tests go in which batch." This is fair but incomplete. It doesn't address the batching approach's advantage: if one batch fails, the others still show green, giving faster signal about which subset of tests is broken. Full consolidation loses this granularity. The rejection should acknowledge this trade-off.

### "Create new reusable workflows first" (Decision 3, rejected)

The rejection says the container loop is "~30 lines of inline bash, not complex enough to warrant a reusable workflow." This contradicts the design's own trade-off section (line 131) which notes "More complex bash in workflow files" as an accepted cost. If the same ~30 lines will be copied into 6+ workflow files, that's exactly the pattern where a reusable workflow or composite action prevents drift. The rejection is arguing against extraction based on current size while planning to create 6 copies. That's the exact scenario where extraction has the highest value.

The design even acknowledges the variation problem: "Each workflow has slightly different test execution needs (different env vars, different verification scripts)." But the container loop scaffolding (image array, family array, docker run, exit code capture, failure collection) is identical. The variation is in what runs inside the container. A reusable workflow could accept the inner command as an input parameter.

This rejection feels like it's optimizing for "easy to write the first one" over "easy to keep them consistent." Given the project already has a divergence problem (the whole point of the design is that some workflows use the matrix pattern while test-recipe.yml uses the loop pattern), creating 6 new copies of a pattern with no enforcement mechanism will produce the same drift over time.

### "Docker Compose multi-container" (Decision 1, rejected)

Fair rejection. Docker Compose is genuinely unnecessary here and adds a dependency.

### "All-at-once refactor" (Decision 3, rejected)

Fair rejection. Incremental migration is clearly better for validation and revertability.

### "Keep tool-per-job matrix" (Decision 2, rejected)

The rejection says "9 parallel runners competing for queue slots is worse than 1 runner completing all 9 tests in ~15 minutes." This repeats the queue-pressure assumption without data. It also doesn't address the max-parallel limit already in place: `test.yml` line 528 sets `max-parallel: 4` on integration-linux, which means only 4 of the 9 jobs compete for slots simultaneously. The design doesn't mention this existing mitigation.

## 4. Unstated Assumptions

### 4a. All consolidation targets share a trigger pattern

The design assumes all target workflows trigger on the same PRs. But `build-essentials.yml` only triggers on specific paths (`internal/actions/homebrew*.go`, `internal/sandbox/**/*.go`, etc.), while `test.yml` triggers on any Go file change. A "typical PR" that changes `internal/providers/github.go` triggers `test.yml` but not `build-essentials.yml` or `integration-tests.yml`. The "76 total before" count assumes a PR that triggers all three workflows simultaneously, which may be uncommon.

The design should state what fraction of PRs actually trigger each workflow. If most PRs only trigger `test.yml`, then the effective savings are 8 jobs (integration-linux), not 33.

### 4b. Sequential execution doesn't increase flakiness

When 9 tests run in parallel, a transient network issue (e.g., GitHub API rate limit, download timeout) affects only the test running at that moment. When 9 tests run sequentially on one runner, the same runner's network state affects all 9. If the runner hits a rate limit during test 3, tests 4-9 also fail. The design's failure handling ("continue on error, collect failures") mitigates the reporting problem but not the root cause: correlated failures due to shared state.

### 4c. Docker image pull time is negligible

The container loop pulls 5 Docker images sequentially. On a cold runner (no image cache), pulling `debian:bookworm-slim`, `fedora:41`, `archlinux:base`, `opensuse/tumbleweed`, and `alpine:3.21` takes non-trivial time. The design's wall-time estimates don't account for image pull overhead. In the matrix approach, all 5 images pull in parallel across separate runners.

### 4d. GHA Go cache works correctly across serialized tests

When 9 integration tests run on one runner, they share the same Go module cache. The first test populates it, subsequent tests reuse it. This is fine for `go build`, but `TSUKU_HOME` state could leak between tests if cleanup is imperfect. The design mentions "fresh $TSUKU_HOME per test" but doesn't address other environment state (e.g., `GITHUB_PATH` appends are cumulative across steps within a single job -- see `test.yml` line 544 where `$HOME/.tsuku/bin` is appended to `$GITHUB_PATH`).

## 5. Strawman Check

No option appears designed to fail. The Docker Compose and all-at-once alternatives are genuinely inferior, not strawmen. The "batch into groups of 3" option is a legitimate middle ground that gets somewhat superficial treatment, but it's not set up to lose.

## 6. Job Count Estimates

### test.yml

The design claims "Before: 18, After: 10, Saved: 8." Let me verify the before count:
- check-artifacts: 1
- lint-workflows: 1
- unit-tests: 1
- lint-tests: 1
- functional-tests: 1
- rust-test: 2 (matrix with ubuntu + macos)
- llm-integration: 1 (conditional on LLM code changes)
- llm-quality: 1 (conditional on prompt/baseline changes)
- matrix: 1
- validate-recipes: 1 (conditional on recipe changes)
- integration-linux: 9 (matrix from test-matrix.json, 9 linux entries)
- integration-macos: 1

Total job definitions: 12, but with matrix expansion: 20 (or fewer depending on conditions).

The design says 18 "before." This is approximately right if you exclude llm-integration and llm-quality (which rarely trigger). But it should be explicit about which jobs it's counting and which conditions it assumes.

The "after" count of 10 means integration-linux goes from 9 to 1, saving 8. This checks out.

### integration-tests.yml

Design claims "Before: 20, After: 6, Saved: 14."

Actual job definitions with matrix expansion:
- checksum-pinning: 5 (5 families)
- homebrew-linux: 4 (4 families)
- library-integrity: 2 (2 libraries x 1 family = 2)
- library-dlopen-glibc: 3 (3 libraries)
- library-dlopen-musl: 3 (3 libraries)
- library-dlopen-macos: 3 (3 libraries)

Total: 20. Checks out.

After consolidation per the design:
- checksum-pinning-linux: 1
- homebrew-linux: 1
- library-integrity: 1
- library-dlopen-glibc: 1
- library-dlopen-musl: 1
- library-dlopen-macos: 1

Total: 6. But wait -- can `library-dlopen-glibc` actually be consolidated? Each library test sets up Rust toolchain and builds `tsuku-dltest` from source (`integration-tests.yml` lines 112-124). This build step takes significant time and is shared across the 3 library tests. Moving from 3 parallel builds to 1 sequential build that tests all 3 libraries saves 2 Rust build+cache setups. This is valid consolidation.

However, `library-dlopen-macos` runs on `macos-latest`, not `ubuntu-latest`. The design's "after" topology (line 224) shows "library-dlopen-macos (1) # was 3." This is consolidating 3 macOS jobs into 1, which is consistent with the pattern. But the scope section (line 28) says "Changing macOS runner allocation" is out of scope. Consolidating 3 macOS dlopen jobs into 1 IS changing macOS runner allocation. The design contradicts itself.

### build-essentials.yml

Design claims "Before: 23, After: 12, Saved: 11."

Actual job definitions with matrix expansion:
- test-homebrew-linux: 4 (4 tools)
- test-meson-build-linux: 1
- test-cmake-build-linux: 1
- test-sqlite-source-linux: 1
- test-git-source-linux: 1
- test-tls-cacerts-linux: 1
- test-zig-linux: 1
- test-no-gcc: 1
- test-sandbox-multifamily: 10 (5 families x 2 tools)
- test-macos-arm64: 1
- test-macos-intel: 1

Total: 23. Checks out.

After: 12 seems right (homebrew-linux 4->1, sandbox-multifamily 10->2). Checks out.

### Overall

The per-workflow counts are accurate. The "total before: 76" assumes all three workflows trigger on the same PR, which (as noted in section 4a) requires a PR touching recipes + sandbox code + test scripts simultaneously.

## 7. Wall-Time vs Queue-Time Trade-off

The analysis is directionally correct but lacks rigor.

**What the design says:** "Queue wait often exceeds [test run time]" and "a 2-minute test that waits 5 minutes in the queue loses more time to queuing than it saves from parallelism" (line 124).

**What's missing:**

1. **No actual queue-time measurements.** This is the design's central trade-off and it's argued entirely by assertion. One table showing queue-wait-time vs test-execution-time for recent PRs would either validate or invalidate the entire premise.

2. **No wall-time estimate for consolidated jobs.** The design says integration-linux goes from 9 parallel jobs to 1 sequential job. If each test takes ~2 minutes (as claimed), the consolidated job takes ~18 minutes. If the 9-job parallel approach completes in ~7 minutes (2-minute test + 5-minute queue), the consolidated approach is slower in wall time. The design should include a concrete comparison.

3. **No analysis of runner pool size.** Queue pressure depends on runner pool capacity. If the pool has 20 concurrent runners and the three workflows create 30 jobs, 10 jobs queue. If the pool has 5 runners, 25 jobs queue. The design doesn't mention the runner pool size, which is the denominator that determines whether queue pressure is actually a problem.

4. **The `max-parallel: 4` mitigation in test.yml is unmentioned.** The integration-linux job already caps parallelism at 4, meaning only 4 of 9 jobs compete for slots at once. This existing mitigation weakens the queue-pressure argument for that specific job.

## 8. Test Reliability Risks

### 8a. Correlated failures from shared runner state

As noted in 4b, sequential tests on one runner share network state, disk state, and time. A rate-limit response from GitHub's API (triggered by test 3's download) will cascade to tests 4-9. In the parallel model, each runner has its own rate-limit budget.

### 8b. The `GITHUB_PATH` accumulation problem

In `test.yml`, the integration-linux job currently does `echo "$HOME/.tsuku/bin" >> $GITHUB_PATH` (line 544). If consolidated, each iteration would append to `$GITHUB_PATH` in the same job. But since `$GITHUB_PATH` is only read between steps (not within a step), and the consolidated version uses a shell loop within a single step, this specific issue doesn't apply. The design handles this correctly by using `TSUKU_HOME` per test rather than `$GITHUB_PATH`.

### 8c. Docker image layer conflicts

When running 5 different containers sequentially on one runner, Docker's storage driver handles layer deduplication. This is standard and unlikely to cause issues. No reliability risk here.

### 8d. Timeout handling

The design doesn't mention timeouts. If one family test hangs (e.g., a package manager blocks waiting for input), the sequential approach blocks all subsequent families. In `test-recipe.yml`, this is handled with `timeout 300` (line 166). The design should specify that per-test timeouts are required in the consolidated jobs.

## 9. Security Claim Error

The design states (line 282): "The volume mount (`-v "$PWD:/workspace"`) is read-only for the tsuku binary and recipes."

This is incorrect. The mount `-v "$PWD:/workspace"` is read-write. Looking at the actual `test-recipe.yml` (line 153), the container writes `.tsuku-exit-code` to `/workspace` (line 167), which requires write access. The mount is intentionally read-write, not read-only. The design should not claim read-only isolation when the mount is read-write.

## Summary of Findings

| Finding | Severity |
|---------|----------|
| No queue-time data to support the central premise | Gap - weakens confidence in the trade-off |
| Missing alternative: artifact-based binary sharing | Gap - could address setup overhead without serialization |
| Missing consolidation target: `sandbox-tests.yml` (9 jobs) | Gap - understates total savings |
| Reusable workflow rejection contradicts the problem being solved | Rationale weakness - 6 copies of the same pattern will drift |
| "Read-only" mount claim is factually wrong | Error - mount is read-write |
| macOS dlopen consolidation contradicts "macOS out of scope" | Contradiction in the design |
| `max-parallel: 4` on integration-linux not mentioned | Omission - weakens the queue-pressure argument |
| No wall-time comparison between before/after | Gap - can't evaluate the trade-off |
| No per-test timeout requirement specified | Reliability risk |
| Correlated failure risk from sequential execution not analyzed | Reliability risk |
