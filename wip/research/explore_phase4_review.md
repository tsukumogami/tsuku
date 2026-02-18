# Design Review: DESIGN-unified-batch-pipeline.md

## Review Scope

Analysis of the problem statement, options analysis, and upstream alignment for the Unified Batch Pipeline design. Evaluated against five review questions: problem specificity, missing alternatives, rejection fairness, unstated assumptions, and strawman detection.

---

## 1. Is the problem statement specific enough to evaluate solutions against?

**Verdict: Yes, with one gap.**

The problem statement is well-grounded in observable evidence:
- Quantified stall: "189 consecutive runs with 0 packages generated"
- Identified root cause: `selectCandidates()` line 158 uses `HasPrefix(entry.Source, prefix)` with `prefix = o.cfg.Ecosystem + ":"`, confirmed in code at `internal/batch/orchestrator.go:158-165`
- Quantified scope: "261 packages re-routed to non-homebrew sources during Bootstrap Phase B"
- Mechanism traced: cron defaults to `ecosystem: homebrew`, workflow sets `ECOSYSTEM: ${{ inputs.ecosystem || 'homebrew' }}` (batch-generate.yml line 33)

The one gap: the statement says "the remaining homebrew-sourced packages all fail deterministic generation" but doesn't provide evidence for this claim. If some homebrew-sourced packages can still generate, the urgency of the fix is the same (261 packages are invisible), but the characterization of "zero recipes" being solely caused by this filter would be slightly misleading. It could be that the filter is one of two problems (the other being that homebrew packages genuinely fail). The design should clarify whether zero output is entirely explained by the filter or partly by homebrew generation failures. This distinction matters because if homebrew generation is also broken, fixing only the filter won't unblock the pipeline -- it'll just add non-homebrew packages to the mix while homebrew ones continue failing.

**Recommendation**: Add a sentence stating how many homebrew-sourced entries exist in the queue and what their failure modes are. Something like "Of the remaining N homebrew-sourced entries, all have status 'failed' with categories X, Y, Z" would close this gap.

---

## 2. Are there missing alternatives we should consider?

**Verdict: One alternative worth discussing, one clarification needed.**

### Missing alternative: Keep the ecosystem flag but make cron dispatch multiple runs

Instead of removing the ecosystem concept entirely, the cron could dispatch one workflow run per known ecosystem using `workflow_dispatch` events. This is different from the "8 separate cron schedules" alternative already rejected, because it uses a single cron entry that programmatically triggers N dispatch events. The differences:
- One cron schedule, not 8
- CI cost scales with queue contents, not ecosystem count (empty ecosystems don't run)
- Keeps the simpler single-ecosystem-per-batch model
- Avoids the mixed-ecosystem PR problem the design acknowledges

However, this alternative has real downsides: it requires knowing the ecosystem list upfront (or scanning the queue first), and it can't handle entries that don't match any predefined ecosystem. The chosen approach is genuinely simpler. This alternative probably belongs in the "Alternatives Considered" section with a brief rejection, not as the chosen option.

### Clarification needed on "make ecosystem optional" rejection

The rejection says it "creates two code paths." This is true but overstated. The "optional" pattern is common (e.g., SQL WHERE clauses that conditionally apply filters) and isn't inherently harder to test. The real reason to reject it is that it preserves a concept (batch-level ecosystem) that no longer has a purpose once the queue is unified. The design should say that rather than citing complexity -- removing a field is conceptually cleaner than making it optional because the batch-level ecosystem concept is fundamentally obsolete.

---

## 3. Is the rejection rationale for each alternative specific and fair?

**Verdict: Mostly fair, with one inaccuracy and one weakness.**

### Decision 1 rejections

**"Run 8 separate cron schedules"**: The rejection says "it doesn't solve mixed-source batches (a package like `bat` with `source: 'github:sharkdp/bat'` still won't match the homebrew schedule)." This is correct and decisive. A `github:` source entry won't match a `homebrew` schedule. The 8x CI cost claim is also valid. Fair rejection.

**"Make ecosystem optional in Config"**: As noted above, the "two code paths" argument is weak. The stronger argument is that the batch-level ecosystem concept is obsolete. But the rejection isn't unfair -- it just picks a less compelling reason.

### Decision 2 rejections

**"Global circuit breaker"**: Rejection is specific and fair. "A cargo API outage shouldn't block homebrew processing" is the correct principle. No issues here.

**"Remove circuit breakers entirely"**: Rejection correctly distinguishes per-package backoff from API-level circuit breaking. The example of crates.io being down affecting all cargo entries is concrete. Fair rejection.

### Decision 3 rejections

**"Keep single ecosystem, pick dominant"**: "It loses information" is correct -- a 50/50 cargo/github batch would misrepresent half the results. Fair rejection.

**"Remove ecosystem from runs entirely"**: Rejection appeals to operator need for ecosystem-level visibility. This is reasonable, though it's worth noting that the ecosystem breakdown could exist at the package level without being aggregated on the run. The design's choice is fine -- the aggregation is useful.

### One inaccuracy

The frontmatter says "Running 8 separate cron jobs would add CI cost and not solve the core problem of mixed-source batches." This conflates the 8 cron alternative with the actual core problem. The core problem isn't "mixed-source batches" -- it's that 261 re-routed entries are invisible. You could solve the invisibility problem without mixed batches (by dispatching per-ecosystem). The mixed-batch model is a simplification improvement, not the only fix. The rationale should frame the chosen option as "the simplest fix that also simplifies the overall model" rather than implying alternatives can't fix the core problem.

---

## 4. Are there unstated assumptions that need to be explicit?

**Verdict: Three assumptions should be stated explicitly.**

### Assumption 1: Rate limits in `ecosystemRateLimits` are sufficient for mixed batches

The current code applies rate limiting based on `o.cfg.Ecosystem` once before the loop. The design proposes per-entry rate limiting using `ecosystemRateLimits[eco]`. But the existing rate limit map (`orchestrator.go:33-42`) sets 1 second for most ecosystems and 6 seconds for RubyGems. In a mixed batch, the ordering of entries determines whether rate limits are respected per-ecosystem or just per-entry.

Example: If a batch has [cargo, homebrew, cargo, homebrew], the current design sleeps 1s between each entry. But from cargo's perspective, there's only ~2s between its two requests (sleep before homebrew + sleep before second cargo). If the batch were [cargo, cargo, homebrew, homebrew], cargo would see 1s between its requests. The design assumes sequential processing with interleaved ecosystems provides adequate spacing. This is probably fine for 1s limits, but should be stated.

### Assumption 2: Batch size stays the same for mixed batches

The design removes `Config.Ecosystem` but keeps `Config.BatchSize`. Currently a batch of 10 means 10 homebrew packages. After the change, 10 means 10 packages across all ecosystems. If the queue has 200 homebrew, 50 cargo, and 11 github entries, a batch of 10 will likely be dominated by the highest-priority entries from whatever ecosystem appears first. The design should state whether the batch size should be adjusted or whether priority ordering naturally distributes entries across ecosystems.

### Assumption 3: The workflow's queue status update step (lines 1116-1128) is dead code or broken

The design mentions line 1118 references `priority-queue-$ECOSYSTEM.json` (legacy per-ecosystem queue filename). This implies the update step is already broken (the file doesn't exist -- the unified queue is `priority-queue.json`). The design should state whether this step is currently doing nothing (and thus removal is a cleanup, not a behavior change) or whether it's failing silently and causing data issues.

---

## 5. Is any option a strawman (designed to fail)?

**Verdict: No clear strawmen, but Decision 3's alternatives are somewhat weak.**

None of the alternatives are obviously constructed to fail. They represent real design patterns:
- Running per-ecosystem cron is a legitimate approach used in many CI systems
- Optional config fields are a standard migration pattern
- Global vs. per-resource circuit breakers is a real architectural choice
- Removing circuit breakers is a valid simplification question

For Decision 3, both alternatives ("pick dominant" and "remove from runs") are somewhat weak opponents. A stronger alternative would be "add `Ecosystems` as a derived field computed at read time from per-package data, without changing the write format." This would avoid schema migration while providing the same information. But it's not a dramatically different approach, so the absence doesn't indicate strawman construction.

---

## 6. Upstream Design Alignment Verification

**Verdict: The design's claims about upstream intent are accurate.**

The design claims the upstream intended the unified queue to eliminate ecosystem filtering. Verified:

1. **"Replace per-ecosystem queues with a single unified queue"** -- Found verbatim at DESIGN-pipeline-dashboard-2.md line 253. Accurate.

2. **"Multi-ecosystem coverage: Unified queue naturally includes packages from all ecosystems"** -- This specific text isn't in the upstream Consequences section, but the principle is established in the Decision 3 rationale (lines 453-454): "The unified queue approach solves the root cause directly."

3. **"Batch generation uses the source directly"** -- Found verbatim in the Decision Outcome at line 445: "Batch generation uses the source directly: `tsuku create --from github:sharkdp/bat`." Accurate.

4. **"[MODIFY] Read source from queue entry, not ecosystem flag"** -- Found verbatim in the Solution Architecture at line 582. Accurate.

5. **Issue #1699 scope** -- The upstream Issue 3 (lines 1783-1795) says "Update orchestrator to read `pkg.Source` directly instead of hardcoding ecosystem." The design's claim that Issue #1699's acceptance criteria "didn't mention removing the ecosystem prefix filter in `selectCandidates()`" is plausible. The upstream issue's deliverables say "Modified `internal/batch/orchestrator.go`" and "Modified queue reading/writing logic" but don't explicitly list removing the prefix filter as a deliverable. The issue description says "read `pkg.Source` directly" which could be read as only affecting `generate()`, not `selectCandidates()`. The gap is real.

**One nuance**: The upstream design's architecture box shows changes only to `generate()`, `parseInstallJSON()`, and `FailureRecord` in `orchestrator.go`. It does NOT list `selectCandidates()` as a modification target. This supports the claim that the upstream design intended the filter removal but didn't explicitly spec it as a deliverable for Issue 3. The modification of `batch-generate.yml` to "[MODIFY] Read source from queue entry, not ecosystem flag" implies the ecosystem flag goes away, which logically requires `selectCandidates()` changes, but the implication wasn't made explicit.

---

## Summary of Recommendations

1. **Strengthen the problem statement** by quantifying how many homebrew-sourced entries exist and what their failure modes are. The 261 re-routed entries being invisible is sufficient to justify the fix, but the "all fail deterministic generation" claim for remaining homebrew entries needs evidence.

2. **Reframe the "make ecosystem optional" rejection** around conceptual obsolescence rather than code path complexity.

3. **Fix the inaccuracy in the frontmatter rationale** -- alternatives CAN solve the core problem of invisible entries; the chosen approach is preferred because it's simpler and aligns with the upstream unified queue design, not because alternatives fundamentally can't work.

4. **Add the three unstated assumptions** (rate limit ordering in mixed batches, batch size semantics across ecosystems, workflow line 1118 being already broken).

5. **Add the "dispatch per ecosystem from single cron" alternative** to the considered options with a brief rejection.

6. No options are strawmen. The alternatives represent real trade-offs and are rejected on defensible grounds.
