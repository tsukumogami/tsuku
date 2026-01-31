# Phase 4 Review: DESIGN-batch-recipe-generation.md

## 1. Problem Statement Specificity

The problem statement is well-scoped. It names the three concrete challenges (validation cost asymmetry, ecosystem isolation, merge safety) and the scope section clearly delineates boundaries. One gap: it doesn't quantify the target throughput. "At scale" is vague -- how many recipes per week is the goal? Without a throughput target, it's hard to evaluate whether manual dispatch (1A) is sufficient or will become a bottleneck. The macOS budget (1000 min/week) is stated but the expected batch frequency and size aren't connected to a weekly target.

**Recommendation:** Add a target like "generate and merge N recipes/week across all ecosystems" to make the problem measurable.

## 2. Missing Alternatives

### Decision 1 (Trigger Model)
The three options cover the reasonable spectrum (manual, scheduled, event-driven). No major gaps.

### Decision 2 (Validation Strategy)
Missing option: **Sampling-based macOS validation.** Instead of validating all Linux-passing recipes on macOS, validate a random sample (e.g., 20%) per ecosystem per batch. This would cut macOS cost further while still catching platform-specific issues statistically. The current 2A already saves 80% by filtering Linux failures, but sampling could save another 60-80% of the remaining macOS minutes.

### Decision 3 (Merge Strategy)
Missing option: **Batch PR with sub-batches.** Split a 100-recipe batch into 4-5 smaller PRs (20-25 recipes each). This balances the "100 PRs = spam" problem of 3B with the "one large PR" problem of 3A/3C. Easier to review, easier to rollback partially.

### Decision 4 (Failure Recording)
The options are reasonable. One missing consideration: 4A and 4B aren't mutually exclusive. JSONL in repo for durable records plus artifacts for detailed logs would combine strengths.

## 3. Pros/Cons Fairness

### Option 1C (Event-Driven)
Slightly strawmanned. The con "over-engineers Phase 1" is a framing judgment rather than a technical trade-off. The real con is operational complexity (debugging distributed triggers). The "requires external infrastructure" is fair but understated -- this is a Cloudflare Worker project, so the infrastructure already exists in the telemetry component.

### Option 2B (Full Matrix)
Fairly evaluated. The budget math makes this clearly impractical given stated constraints.

### Option 2C (Linux-Only with Sweeps)
The con "recipes merged without macOS validation may break users" is significant but underweighted. This option has a real risk of shipping broken recipes to macOS users for days. The pros/cons are technically accurate but the severity asymmetry isn't called out.

### Option 3A (One PR Per Batch)
The con "one failing recipe blocks the entire batch" is stated, but this is exactly why 3C was created. This makes 3A look like a setup for 3C. However, 3A has a simpler implementation, and the blocking behavior could be acceptable if batch sizes are small (5-10). The document doesn't explore batch size as a variable that affects this trade-off.

### Option 3C (Selective Exclusion)
The con "more complex PR assembly logic" is understated. Assembling a PR from partial results across multiple ecosystem jobs, tracking which recipes were excluded and why, and maintaining batch ID coherence is non-trivial workflow orchestration in GitHub Actions.

### Option 4A (JSONL in Repo)
The con about merge conflicts is real but the mitigation (per-ecosystem files) is mentioned only in the Mitigations table, not in the cons. The growth concern is dismissed as "kilobytes" but at hundreds of failures per run with multiple runs per week, this could reach megabytes within months.

## 4. Unstated Assumptions

1. **Builder CLI interface exists.** The design assumes `tsuku create --from <eco>:<pkg> --deterministic` works. If this CLI surface doesn't exist yet, it's a significant prerequisite.

2. **Sandbox validation infrastructure exists.** The validation flow references `--sandbox` mode and container execution. If this is new infrastructure, the implementation scope is much larger than "workflow + shell scripts."

3. **Single concurrent run.** The design assumes only one batch runs at a time (otherwise JSONL merge conflicts, circuit breaker state races). This isn't stated as a constraint.

4. **GitHub Actions permissions.** Auto-merge from a workflow requires specific repository settings (branch protection rules allowing workflow merges, `GITHUB_TOKEN` with write permissions). These are assumed but not documented.

5. **Queue is pre-populated.** The priority queue must exist and be populated before this pipeline runs. The out-of-scope section mentions this but doesn't clarify whether this is already done or is a blocking dependency.

6. **Rate limits are per-CI-run, not per-day.** The rate limiting table shows per-request delays but doesn't address cumulative daily/hourly limits across multiple batch runs.

## 5. Strawman Assessment

**Option 1C (Event-Driven)** leans toward strawman territory. The "over-engineers Phase 1" framing assumes the conclusion. However, the external infrastructure dependency is a legitimate concern, so it's not purely a strawman -- just somewhat skewed.

**Option 2B (Full Matrix)** is not a strawman but is effectively ruled out by the budget constraint stated in Decision Drivers. It serves as a useful reference point.

**Option 3B (One PR Per Recipe)** is the closest to a strawman. The "100 PRs = notification spam" framing makes it sound absurd, but with GitHub's auto-merge and branch protection, this is actually how many large-scale automation systems work (Dependabot creates one PR per dependency). The framing is unfair.

No option is purely designed to fail, but 3B deserves a more balanced treatment.

## Summary of Recommendations

1. **Add a throughput target** to the problem statement (recipes/week goal).
2. **Consider sampling-based macOS validation** as a Decision 2 option -- could save significant additional macOS budget.
3. **Reconsider 3B (one PR per recipe)** more fairly -- Dependabot proves this pattern works at scale. Alternatively, add a sub-batch option.
4. **Make the single-concurrent-run assumption explicit** and document how it's enforced (GitHub Actions concurrency groups).
5. **Clarify prerequisites** -- does the `--deterministic` CLI flag exist? Does sandbox validation exist? These affect implementation scope significantly.
6. **Acknowledge 3C complexity** more honestly -- the "more complex PR assembly logic" con understates the orchestration difficulty in GitHub Actions.
