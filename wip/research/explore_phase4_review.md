# Architect Review: DESIGN-ci-build-essentials-consolidation.md

## Scope

This review evaluates the problem statement specificity, alternative completeness, rejection rationale fairness, unstated assumptions, and strawman risk for the proposed CI Build Essentials consolidation design.

## 1. Problem Statement Specificity

**Verdict: Sufficient.** The problem statement is concrete and measurable. It identifies:

- Exactly which 7 jobs are targeted (with a table of names, purposes, and durations)
- The setup overhead per job (1-2 minutes repeated 7 times)
- The queue pressure mechanism (7 concurrent jobs competing for slots)
- A specific CI run (22325545073) with timestamps showing actual execution windows
- The existing precedent that proves the pattern works (macOS jobs in the same file)

The problem statement also correctly scopes what's in and out. The No-GCC container test and sandbox-multifamily jobs are excluded with specific justification (different runner requirements). No vague language -- the costs are quantified and the scope boundaries are clear.

One minor gap: the problem statement says "Queue pressure from 7 concurrent jobs delays all of them and every other workflow waiting for runners" but doesn't quantify queue wait for these specific 7 jobs the way the upstream design did for integration-linux (7-11 minutes). The run data shows start/end timestamps but not queue-wait-vs-execution breakdown. This doesn't weaken the argument since the upstream design already established the queue pressure pattern, but it would be stronger with a direct measurement for these jobs.

## 2. Missing Alternatives

**Verdict: One notable absence, but not a gap that changes the decision.**

### Alternatives the design considered

1. **Split into 2 jobs by duration** -- Partial consolidation for shorter wall time
2. **Share binary via artifacts** -- Eliminate build overhead while keeping parallel execution

### Alternative not considered: Matrix with max-parallel: 1

GHA supports `max-parallel: 1` on matrix strategies, which would keep the matrix declaration (simpler YAML) while serializing execution to a single runner at a time. This gets some queue benefit (only 1 runner active at a time) but doesn't eliminate the per-job setup overhead since each matrix entry still runs checkout+Go+build independently. It also doesn't produce the same shared-cache benefit.

This alternative is strictly worse than the chosen option (still repeats setup, still creates 7 jobs in the queue backlog even if only 1 runs at a time) so omitting it doesn't bias the analysis. But acknowledging it as a "why not just throttle" straw-alternative would preempt the obvious question from anyone familiar with GHA matrix options.

### No other structural alternatives are missing

The design space for "how to reduce 7 identical-runner jobs" is genuinely narrow: consolidate into fewer jobs, share setup artifacts, or throttle parallelism. The design covers the first two and the third isn't worth a full section. I don't see a missing option that would change the outcome.

## 3. Rejection Rationale

### "Split into 2 jobs by duration" -- Fair rejection

The rejection says "the complexity of maintaining two groups with balanced timing isn't worth the ~7 minutes saved." This is reasonable. The macOS arm64 job already runs 8 tests in ~8 minutes without complaints, establishing that serial execution at this scale is acceptable. The ongoing maintenance question (which group gets new tests) is a real cost with no countervailing benefit.

One note: the rejection says "~7 minutes saved" comparing 2 groups (~11 min wall) to 1 group (~18 min wall). But the 18-minute estimate may be high since the individual job durations in the table sum to about 18 minutes total, and the consolidated job eliminates 6x setup overhead (~9 minutes). The actual serial test time would be closer to ~18 minutes minus ~9 minutes of saved setup = ~9 minutes of net test execution plus ~2 minutes of single setup = ~11 minutes total. The design's own "Decision Outcome" section acknowledges this implicitly ("total serial time would be roughly 18 minutes") but doesn't subtract the setup savings. This makes the 2-group alternative look more competitive than it actually is -- if total is ~11 minutes, not ~18, the wall-time argument against splitting is even stronger.

### "Share binary via artifacts" -- Fair rejection

The rejection correctly identifies that artifact sharing addresses runner-minute waste but not queue pressure. Seven jobs still enter the queue. The design also notes that the upstream CI consolidation design already rejected this approach for the same reason, which adds consistency. Fair.

## 4. Unstated Assumptions

### Assumption 1: Queue pressure is the dominant cost, not wall time

The entire design trades wall time for queue relief. This is stated as a decision driver ("Queue pressure is the dominant cost") but relies on the upstream design's measurements (7-11 minute queue waits) rather than measurements specific to these 7 jobs. If these jobs run at low-contention times (e.g., only on schedule or on main pushes to specific paths), queue pressure may be lower than for PRs that trigger test.yml.

However, the workflow triggers on PRs to main with matching paths, and any PR touching Go action code, recipes, or test scripts will trigger it alongside other workflows. So the queue contention scenario is realistic. The assumption holds but would be stronger stated explicitly: "These jobs trigger on the same PR events that activate other high-job-count workflows, so they compete for the same runner pool."

### Assumption 2: The macOS `run_test()` function can be copied with minimal adaptation

The design says the implementation "copies the `run_test()` function from the macOS arm64 job." Verified against the actual workflow file (`/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/.github/workflows/build-essentials.yml` lines 460-499), the macOS `run_test()` function is a close match to the proposed code, with one difference:

- The **existing test-homebrew-linux** job (line 87) sets `export PATH="$TSUKU_HOME/bin:$PATH"` inside its loop
- The **macOS `run_test()`** does NOT set PATH
- The **proposed Linux `run_test()`** in the design DOES include the PATH line

This difference exists because `tsuku install` on Linux needs the bin directory on PATH for dependency resolution during source builds (e.g., sqlite-source needs to find readline's pkg-config). The macOS homebrew installs don't have the same dependency chain behavior. The design's proposed code is correct to include PATH, but calling it "a direct copy" is slightly misleading. It's a copy with a necessary adaptation. This should be stated explicitly to avoid confusion during implementation.

### Assumption 3: Per-test timeouts are sufficient to prevent cascade failures

The design proposes `timeout 600` for source builds and `timeout 300` for others. These are per-test bash-level timeouts. The existing individual jobs have varying timeouts:

- test-homebrew-linux: `timeout-minutes: 60` (GHA job-level)
- test-tls-cacerts-linux: `timeout-minutes: 30` (GHA job-level)
- Other 5 jobs: no explicit timeout (defaults to 360 minutes)

The proposed consolidated job has `timeout-minutes: 90` at the GHA level plus per-test bash timeouts. This is actually stricter than the current setup where 5 of 7 jobs could theoretically run for 6 hours. The assumption that 600s per source build is sufficient is reasonable given the measured durations (max 4.5 minutes for git-source), but git-source includes downloading and building curl, openssl, zlib, expat, and git itself -- on a slow network day this could exceed 10 minutes. The 600s timeout provides ~2x headroom over the typical 4.5-minute duration, which seems adequate.

### Assumption 4: gettext installation is unconditionally safe

The design says "git-source's `apt-get install gettext` runs unconditionally" and calls this "negligible" overhead (~15 seconds). This is correct for the current test set, but it's an assumption about future tests: no tool in the current set would be affected by having gettext installed. If a future test needed to verify behavior without gettext, this could create a subtle environment difference. Low risk, but worth noting as an assumption.

## 5. Strawman Analysis

**Verdict: No strawmen detected.**

Both alternatives are plausible approaches that someone might reasonably propose:

- **2-group split**: This is the natural "meet in the middle" compromise. The rejection is based on maintenance cost and the precedent that macOS handles 8 tests serially, not on inflated difficulty.
- **Artifact sharing**: This is a real pattern used elsewhere in the project (test-recipe.yml, platform-integration.yml as the upstream design notes). The rejection targets the specific limitation (queue pressure, not runner minutes) rather than dismissing the approach generally.

Neither alternative is set up to fail. The rejections engage with the actual trade-offs rather than attacking weak formulations.

## 6. Structural Consistency with Upstream Design

The design correctly identifies itself as continuing work from DESIGN-ci-job-consolidation.md. The upstream design's topology table for build-essentials.yml shows:

```
test-homebrew-linux (1)              # was 4
test-meson-build-linux (1)           # stays 1
test-cmake-build-linux (1)           # stays 1
test-sqlite-source-linux (1)         # stays 1
test-git-source-linux (1)            # stays 1
test-tls-cacerts-linux (1)           # stays 1
test-zig-linux (1)                   # stays 1
test-no-gcc (1)                      # stays 1
test-sandbox-multifamily (2)         # was 10
test-macos-arm64 (1)                 # already aggregated
test-macos-intel (1)                 # already aggregated
```

This matches the current state of the workflow file (12 jobs). The upstream design explicitly left the 6 individual Linux tool-test jobs (meson, cmake, sqlite, git-source, tls-cacerts, zig) unconsolidated. This new design picks up exactly where the upstream left off. Good structural continuity.

One observation: the upstream design counted build-essentials at "12 jobs after" consolidation. This new design proposes going from 12 to 6. The combined effect would be that build-essentials goes from 23 (original pre-upstream) to 6. The new design tracks this correctly in its job count table.

## 7. Factual Accuracy Check

Verified against `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/.github/workflows/build-essentials.yml`:

| Design Claim | Actual | Match? |
|---|---|---|
| 7 separate Linux tool-test jobs | 7 jobs (lines 52, 127, 153, 179, 205, 237, 261) | Yes |
| All run on ubuntu-latest | All 7 use `runs-on: ubuntu-latest` | Yes |
| Total 12 jobs in workflow | 12 job definitions | Yes |
| macOS jobs already use `run_test()` pattern | Lines 460-499 (arm64), 551-590 (Intel) | Yes |
| git-source needs `apt-get install gettext` | Line 222-224 | Yes |
| tls-cacerts runs a test script | Line 258 (`./test/scripts/test-tls-cacerts.sh ./tsuku`) | Yes |
| homebrew-linux tests 4 tools in a loop | Lines 75-97 (pkg-config, cmake, gdbm, pngcrush) | Yes |
| macOS arm64 tests 8 tools | Lines 503-515 (3 homebrew + 4 source + 1 zig = 8) | Yes |
| Consolidated job timeout: 90 min | Matches macOS jobs (lines 438, 529) | Yes |

All factual claims verified.

## 8. Wall-Time Estimate Accuracy

The design estimates ~18 minutes total serial time. Breaking this down from the duration table:

| Test | Duration |
|---|---|
| homebrew tools (4 tests) | ~1 min each = ~4 min |
| libsixel-source | ~2.5 min |
| ninja | ~2 min |
| sqlite-source | ~3 min |
| git-source | ~4.5 min |
| tls-cacerts | ~3 min |
| zig | ~2 min |

Sum: ~21 minutes of raw test time, not 18. However, with shared download cache (some deps like zlib, openssl are used by multiple source builds), cache hits could reduce this. The ~18 minute estimate seems plausible as a realistic-with-cache number.

The design should note that the 1-2 minute setup overhead is paid once in the consolidated job rather than 7 times. Net runner minutes saved: ~(7 x 1.5) - 1.5 = ~9 minutes of pure setup elimination, plus whatever queue wait was consumed.

## Summary of Findings

### Strengths
1. Problem statement is specific, quantified, and grounded in actual CI run data
2. Alternatives are genuine options, not strawmen
3. Rejection rationale is specific and fair
4. Direct structural continuity with the upstream CI consolidation design
5. All factual claims verified against the actual workflow file

### Items to Address

1. **Wall-time estimate consistency** (minor): The design quotes ~18 minutes total but the individual durations sum to ~21 minutes. Clarify whether ~18 minutes accounts for cache sharing or is an underestimate. This doesn't change the decision but affects expectations.

2. **"Direct copy" claim for `run_test()`** (minor): The Linux version needs `PATH` management that the macOS version doesn't have. Calling it "a direct copy" understates the adaptation needed. State the PATH difference explicitly.

3. **Queue pressure measurement** (minor): The design relies on the upstream design's queue measurements (7-11 minutes) rather than measuring queue waits for these specific 7 jobs. Since these jobs share the same trigger conditions and runner pool, the extrapolation is reasonable, but stating this inference explicitly would strengthen the argument.

4. **No missing alternatives that would change the outcome.** The `max-parallel: 1` throttle option is the only notable omission, and it's strictly worse than the chosen option.

5. **No strawmen detected.** Both rejected alternatives are real approaches with genuine trade-offs.

### Recommendation

The design is well-constructed and ready to proceed. The three minor items above are documentation improvements, not structural issues. None of them change the decision or the implementation approach.
