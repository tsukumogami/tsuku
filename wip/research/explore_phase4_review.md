# Architect Review: DESIGN-recipe-ci-batching.md

## Review Scope

Problem statement, options analysis, and rejection rationale in `docs/designs/DESIGN-recipe-ci-batching.md`. Evaluated against the actual CI workflow files on `main`.

---

## 1. Problem Statement Specificity

**Verdict: Strong, with one gap.**

The problem statement is grounded in real data (PR #1770, 153 per-recipe jobs, 264 total checks) and correctly identifies the structural cause: each runner pays ~30-45s of cold-start overhead for a single recipe. The scope section cleanly separates what's in and out.

**Gap: `validate-golden-execution.yml` has three more per-recipe matrix jobs that aren't mentioned.**

The design's scope says it covers `test-changed-recipes.yml` (Linux path) and `validate-golden-recipes.yml`. But `validate-golden-execution.yml` has these per-recipe matrix jobs:

- `validate-coverage` (line 377): `matrix.recipe` from `fromJson(needs.detect-changes.outputs.recipes)`
- `validate-linux` (line 516): per-file matrix from `fromJson(needs.detect-changes.outputs.linux_matrix)`
- `execute-registry-linux` (line 709): `matrix.recipe` from `fromJson(needs.detect-changes.outputs.registry_recipes)`

These are exactly the same pattern being fixed in the other two workflows. If a 200-recipe PR triggers `validate-golden-execution.yml`, it spawns per-recipe jobs there too.

The design lists `validate-golden-execution.yml` in "Out of scope" with the note "container-family batching already works." That's true for `validate-linux-containers`, which batches by family. But the three other jobs in that workflow don't batch at all. The out-of-scope note conflates "one job in the workflow already batches" with "the workflow is handled." This should either be explicitly acknowledged as a known gap with a follow-up item, or brought into scope.

**Recommendation:** Add a note to the scope section acknowledging that `validate-golden-execution.yml` has three additional per-recipe matrix jobs (`validate-coverage`, `validate-linux`, `execute-registry-linux`) that exhibit the same problem. Either include them in Phase 2/3 or add a "Future Work" section explaining why they're deferred (e.g., lower volume, different matrix structure).

---

## 2. Missing Alternatives

**Two alternatives worth considering are absent.**

### a. Build artifact caching (partial mitigation, not alternative)

The design explicitly puts "build artifact caching across workflows" out of scope. Fair enough as a full alternative, but there's a lighter variant: **upload the built `tsuku` binary as a workflow artifact from the detection job, and download it in each matrix job instead of rebuilding**. This eliminates the Go setup + `go build` cold start (~20-30s) from every matrix entry without changing the matrix structure at all.

This isn't a replacement for batching, but it's complementary and mechanically simple. The `batch-generate.yml` workflow already does exactly this pattern (builds binaries once, uploads as artifacts, downloads in validation jobs). The design should at least mention why this was not considered as a complementary optimization or first step.

### b. GitHub Actions `max-parallel` throttling

GitHub Actions supports `strategy.max-parallel` to cap concurrent matrix jobs. Setting `max-parallel: 20` on the existing per-recipe matrix would limit the queue flood to 20 concurrent jobs without any batching changes. It doesn't reduce total job count (billing impact stays), but it addresses the "long tail of queued-then-running jobs" problem described in the problem statement.

This could be dismissed quickly (doesn't reduce job count, still pays N cold starts) but should be acknowledged as a considered-and-rejected option since it directly addresses part of the stated problem with zero code changes.

---

## 3. Rejection Rationale Fairness

### Time-estimated batching: Fair rejection.

The reasoning is specific ("we don't have reliable per-recipe timing data") and honest about the maintenance burden. Not a strawman.

### Single-job aggregation: Fair but slightly oversimplified.

The rejection says "it doesn't parallelize at all" and estimates 30+ minutes for 200 recipes. This is accurate for the worst case, but omits that the macOS path already accepts this tradeoff and works. The rejection should note that the macOS path's acceptable duration is a function of having fewer recipes (macOS-compatible subset is smaller) and fewer available runners, not an endorsement of sequential execution at scale. This would make the rejection more precise.

### Nested composite actions: Fair.

Short and to the point. Correctly identifies the debugging cost.

### Per-family batch sizing: Fair.

Good reasoning that slow families should be addressed by profiling, not per-family knobs.

**No strawmen detected.** All alternatives are reasonable approaches that a team member might propose. The rejections cite specific costs rather than vague dismissals.

---

## 4. Unstated Assumptions

### a. GitHub Actions matrix limit of 256

GitHub Actions has a hard cap of 256 matrix entries per job. The design proposes `ceil(N / BATCH_SIZE)` batches. With BATCH_SIZE=15 and 300 recipes, that's 20 batches -- well within limits. But with the cross-product for `validate-golden-execution.yml` (batches x 5 families), the multiplier could approach the limit for very large PRs. The design should state the 256-entry constraint and confirm the chosen batch size keeps the cross-product safely below it.

### b. Download cache sharing within a batch works identically to macOS pattern

The design references "shared download cache" and "like macOS pattern" but doesn't spell out the mechanics. The macOS path (lines 228-243 of `test-changed-recipes.yml`) creates a per-recipe `TSUKU_HOME` with a symlinked download cache. This assumes the download cache directory structure doesn't conflict between recipes. That assumption holds today (downloads are keyed by URL hash), but it's an implicit dependency the design inherits without stating.

### c. BATCH_SIZE of 15 assumes roughly uniform recipe durations

The design acknowledges "some recipes are fast (~10s), others are slow (~2min)" in the decision drivers but then picks a fixed batch size without accounting for the variance. If a batch of 15 happens to contain several slow recipes, it could exceed the 15-minute timeout. The math in the rationale ("assuming ~30s average per recipe") produces ~7.5 minutes per batch, leaving headroom. But the worst case (15 slow recipes at 2min each) is 30 minutes, which exceeds the 15-minute timeout on `test-linux` jobs. The design should either acknowledge this or note that the timeout needs to increase for batched jobs.

### d. `validate-golden-recipes.yml` has a different data shape

The design groups both target workflows under the same implementation approach, but they have different matrix structures:

- `test-changed-recipes.yml` matrix items are `{tool, path}` objects
- `validate-golden-recipes.yml` matrix items are `{recipe, category}` objects

The batch splitting logic in the design uses `.path` (`jq -r '.[].path'`), which works for the first workflow but not the second. The implementation will need to handle both shapes. This isn't a blocking issue but should be called out in the Solution Architecture section to avoid surprises during implementation.

### e. `validate-golden-recipes.yml` uses `actions/cache` per recipe

Each `validate-golden` job currently has a cache step keyed by `${{ matrix.item.recipe }}` (line 166). Batching multiple recipes into one job means the cache key strategy needs to change -- a single cache entry per batch won't get cache hits from previous runs where the batch composition was different. The design doesn't mention this. It's not critical (golden validation is lightweight), but it's a behavioral change worth noting.

---

## 5. Strawman Check

**No strawmen.** All four alternatives are patterns that exist in real CI systems:

- Time-estimated batching: used by large monorepos (Bazel test sharding, CircleCI's test splitting)
- Single-job aggregation: already used in this codebase for macOS
- Nested composite actions: a reasonable GitHub Actions pattern
- Per-family batch sizing: a natural extension of the family-based containerization

Each is rejected with a specific cost, not a caricature. The chosen option (ceiling division) is genuinely the simplest approach that works, not a predetermined winner.

---

## Summary of Findings

| # | Finding | Severity | Action |
|---|---------|----------|--------|
| 1 | `validate-golden-execution.yml` has 3 per-recipe matrix jobs not in scope | Advisory | Acknowledge gap explicitly, add to future work or expand scope |
| 2 | Build artifact caching (upload binary from detection job) is a complementary alternative not mentioned | Advisory | Add as a considered complementary optimization or explain why deferred |
| 3 | `max-parallel` throttling not mentioned as a considered option | Advisory | Brief mention + rejection is sufficient |
| 4 | GitHub Actions 256-entry matrix limit not stated | Advisory | Add constraint to rationale section |
| 5 | Batch of 15 slow recipes (2min each) exceeds current 15-minute timeout | Advisory | Note that timeout may need adjustment for batched jobs |
| 6 | `validate-golden-recipes.yml` has different matrix item shape (`{recipe, category}` vs `{tool, path}`) | Advisory | Note in Solution Architecture section |
| 7 | Per-recipe `actions/cache` keys won't work after batching | Advisory | Mention cache strategy change for `validate-golden-recipes.yml` |

No blocking findings. The design's core decision (ceiling-division batching extending the proven macOS aggregation pattern) is architecturally sound. It doesn't introduce a parallel pattern -- it converges the Linux path toward the existing macOS pattern.

The main structural concern is the incomplete scope mapping: the design correctly identifies two workflows but misses three more per-recipe matrix jobs in a third workflow that has the identical problem. This won't cause divergence (the same batching approach applies), but leaving it unstated could lead to a second design round for the same issue.
