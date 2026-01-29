# Phase 4 Review: Batch Operations Design Analysis

**Command**: /explore
**Phase**: 4 - Review
**Role**: Design Reviewer
**Date**: 2026-01-27

## Review Summary

The DESIGN-batch-operations.md document presents a solid foundation for operational controls, but has several gaps that need addressing before implementation. The problem statement is clear but the options analysis has blind spots around failure modes and operational sequencing.

---

## Question 1: Is the Problem Statement Specific Enough?

**Assessment**: Mostly yes, but missing quantification.

### Strengths

The problem statement clearly identifies four risk categories:
- Blast radius amplification
- No defined recovery path
- Cost runaway
- Incident response vacuum

These directly map to the decision drivers, creating good traceability.

### Gaps

**Missing quantification of "significant operational risk":**
- What's the expected batch size? (100? 500? 1000 recipes per run?)
- What's the expected merge rate? (10/hour? 100/hour?)
- What's the detection latency for problematic recipes?

Without these numbers, it's impossible to evaluate whether options are adequate. For example, Option 2A (Workflow Disable) might be sufficient if detection happens within minutes and batch sizes are <50, but wholly inadequate if batches are 500+ recipes and detection takes hours.

**Recommendation**: Add a "Scale Assumptions" section before evaluating options:
```
Expected batch size: 50-100 recipes per CI run
Expected run frequency: Daily (nightly schedule)
Expected detection latency: 1-6 hours (next health check)
Worst-case blast radius: ~100 recipes before detection
```

---

## Question 2: Are There Missing Alternatives?

**Yes.** Several relevant options are absent:

### Decision 1: Rollback Mechanism

**Missing Option 1D: Staged Rollback via Recipe Metadata**

Add a `batch_id` field to recipes during generation. Rollback becomes a query + bulk operation:

```bash
# Find all recipes from batch 2026-01-28-001
git log --all --name-only --grep="batch_id: 2026-01-28-001"
# Generate revert commit for those files
```

**Pros:**
- Surgical rollback of exactly what a batch introduced
- No dependency analysis needed (batch already captures the cohort)
- Works with any rollback mechanism (revert, deletion, soft delete)

**Cons:**
- Requires adding metadata to recipe format
- Batch ID must be recorded during generation

This is a significant oversight because Options 1A-1C all struggle with "identifying which commits to revert" - the problem the design explicitly calls out as a con. Batch IDs solve this directly.

### Decision 2: Emergency Stop

**Missing Option 2D: Circuit Breaker Auto-Stop**

The design mentions circuit breaker in the upstream (DESIGN-registry-scale-strategy.md Phase 1b: "auto-pause at <50% success") but doesn't include it as an emergency stop option. This is inconsistent.

A circuit breaker that auto-pauses when success rate drops provides:
- **Automatic** response (no human intervention needed)
- **Proportional** response (triggers on actual failure rate)
- **Self-documenting** (logs show why it tripped)

This should be Option 2D, distinct from manual mechanisms.

### Decision 3: Cost Control

**Missing Option 3D: Time-Windowed Budget**

Rather than sampling or absolute caps, use a rolling budget window:

```yaml
cost_limits:
  macos_minutes_per_week: 1000
  linux_minutes_per_week: 5000
  sampling_when_above: 80%  # Start sampling at 80% of budget
```

This allows bursting early in the week while ensuring budget compliance by week's end. The existing codebase already has this pattern (see `.github/workflows/r2-cost-monitoring.yml` which alerts at 80% of free tier).

### Decision 5: Data Storage

**Missing Option 5D: GitHub Actions Artifacts**

For short-term operational state (batch run results, control signals), GitHub Actions artifacts could work:

```yaml
- uses: actions/upload-artifact@v4
  with:
    name: batch-${{ github.run_id }}-status
    path: batch-status.json
    retention-days: 30
```

**Pros:**
- No external infrastructure
- Automatically linked to workflow runs
- Built-in retention policies

**Cons:**
- Not queryable (must download to inspect)
- 90-day maximum retention
- No cross-workflow visibility without API calls

This might be appropriate for control state while metrics go to D1.

---

## Question 3: Are the Pros/Cons Fair and Complete?

**Assessment**: Generally fair, but several omissions.

### Option 1A: Git Revert Commit

**Missing Con**: Merge conflicts with subsequent PRs. If a revert is created while other batch PRs are in flight, those PRs may conflict. This is a real operational hazard for high-frequency merging.

**Missing Pro**: Reverts preserve blame history. The original commit, revert, and any re-application are all visible, creating a clear audit trail.

### Option 1C: Soft Delete via Deprecation

**Missing Con**: Doesn't stop users who already have the recipe. Users who installed before deprecation keep using the problematic version. This isn't truly "rollback" - it's "stop bleeding."

**Missing Pro**: Can be automated in the CLI. The CLI could check deprecation status at install time and warn users, providing active protection.

### Option 2B: Control File in Repository

**Undersold Pro**: The control file can include structured data about pause reasons, expected resume time, and contact info. This self-documenting nature is very valuable for incident response:

```json
{
  "enabled": false,
  "reason": "Investigating homebrew validation failures",
  "incident_url": "https://github.com/tsukumogami/tsuku/issues/1234",
  "disabled_by": "operator@example.com",
  "disabled_at": "2026-01-28T10:00:00Z",
  "expected_resume": "2026-01-28T14:00:00Z"
}
```

### Option 3A: Hard Budget Caps via Workflow Logic

**Missing Con**: State persistence across workflow runs is non-trivial. GitHub Actions doesn't provide built-in state, so this requires either artifacts, repository files, or external storage - adding complexity the design doesn't acknowledge.

**Missing Pro**: Can implement gradual throttling, not just hard stops. At 80% budget, reduce batch size. At 90%, stop entirely. This is more graceful than binary on/off.

### Option 3C: Sampling Strategy

**Missing Pro**: Can be adaptive. Start with 100% validation, measure pass rate, reduce sampling as confidence grows. This isn't just a cost control - it's a maturity signal.

**Missing Con**: Creates second-class validation. A recipe that only passed sampled validation might fail on the skipped environment. Users on that environment experience failures that CI "approved."

### Option 4B: Per-Ecosystem Success Rates

**Missing Pro**: Enables ecosystem-specific circuit breakers. If Homebrew drops to 60% while Cargo stays at 99%, you can pause Homebrew only. This is a major operational advantage.

### Option 5C: Hybrid

**Missing Con**: Split-brain risk. If control file says "resume" but D1 says "paused" (due to partial update), what happens? The design doesn't address reconciliation.

---

## Question 4: Are There Unstated Assumptions?

**Yes.** Several critical assumptions are implicit:

### Assumption 1: Single Operator Model

The design assumes one operator handles incidents. Terms like "operators have no playbook" and access control questions suggest a single-person operation.

**Why this matters**: If multiple operators can intervene simultaneously, race conditions arise. Two operators creating rollback PRs, or one enabling while another disables. The options don't address coordination.

**Recommendation**: State explicitly: "This design assumes single-operator incidents. Multi-operator coordination is out of scope for v1."

### Assumption 2: GitHub Actions as Sole Execution Environment

All emergency stop options assume GitHub Actions as the execution environment. What if the batch runs locally for testing? What if it moves to a self-hosted runner?

**Why this matters**: Option 2A (Workflow Disable) only works for GitHub-hosted. Option 2B (Control File) works anywhere but requires polling.

**Recommendation**: State: "Batch processing executes exclusively in GitHub Actions."

### Assumption 3: Recipes Are Individually Revertable

Options 1A-1C assume each recipe is an independent unit that can be reverted without affecting others. But recipes can have dependencies. Reverting a library recipe while leaving dependent tool recipes creates broken state.

**Why this matters**: The design needs to address dependency-aware rollback or explicitly state that library recipes won't be auto-merged.

**Recommendation**: Add to scope: "Rollback of library recipes requires dependency analysis; this design covers tool recipes only."

### Assumption 4: Detection Happens Before Harm

The "emergency stop" framing assumes we detect problems and stop before users are affected. But what about users who installed a problematic recipe before detection?

**Why this matters**: Emergency stop prevents new harm but doesn't remediate existing harm. The design should acknowledge this gap.

**Recommendation**: Add to "Out of scope": "User notification of recalled recipes (future work)."

### Assumption 5: Cloudflare D1 Availability Matches Requirements

Option 5B proposes D1 for operational data. But the design hasn't validated:
- D1 read latency for control file checks (critical path)
- D1 write throughput for high-frequency metric recording
- D1 availability SLA vs GitHub Actions availability

**Why this matters**: If D1 is unavailable during a batch run, do we fail open (continue) or fail closed (stop)? This affects the entire operations posture.

**Recommendation**: Add to "Uncertainties": "D1 latency and availability under batch load unknown."

### Assumption 6: GitHub Spending Limits Behavior

Option 3B mentions GitHub spending limits but the Uncertainties section notes the behavior is unclear. This isn't just an uncertainty - it's a potential correctness bug.

If GitHub kills a workflow mid-run when the limit is hit, it could leave:
- Partial PRs open
- State files inconsistent
- Metrics half-written

**Recommendation**: This assumption must be validated before relying on 3B. Add explicit gate: "Validate GitHub spending limit behavior in Phase 0."

---

## Question 5: Is Any Option a Strawman?

**Assessment**: No obvious strawmen, but one option is undersupported.

### Option 1B: Recipe Deletion PR

This option is presented fairly but is practically never the right choice. It has:
- All cons of 1A (still requires PR, still needs CI)
- Loses history connection (worse than 1A)
- No unique pros that 1A lacks

The only stated pro ("Faster than revert - no dependency analysis") is misleading. Git revert doesn't do dependency analysis either. Both require identifying files; revert just uses commit history while deletion uses file listing.

This isn't a strawman (it's not designed to fail obviously), but it's a dominated option that clutters the decision space.

**Recommendation**: Merge 1B into 1A as a variant: "Git Revert or Deletion PR (same mechanism, different framing)."

### Option 2A: Workflow Disable via Repository Settings

This option is presented honestly but undersells its severity. Disabling workflows:
- Requires repository admin (high privilege)
- Affects ALL runs of that workflow (not just new triggers)
- Cannot target a specific in-progress run
- Has no machine-readable state (relies on GitHub UI)

This is really a "nuclear option" that should be positioned as last resort, not first-among-equals.

**Recommendation**: Rename to "Nuclear Stop: Workflow Disable" and position as escalation path, not primary mechanism.

---

## Additional Review Findings

### Missing: Runbook Integration

The design mentions "Runbook templates for common scenarios" in scope but none of the options address how runbooks interact with mechanisms. The R2 runbook (`docs/r2-golden-storage-runbook.md`) provides an excellent template that this design should follow:

- Explicit decision trees ("When to Investigate vs Wait")
- Step-by-step procedures with commands
- Troubleshooting diagnostics
- Escalation paths

**Recommendation**: Add a section "Runbook Requirements" that each chosen option must satisfy.

### Missing: Testing Strategy

The design doesn't address how operators will test these mechanisms. You can't wait for a real incident to discover the emergency stop doesn't work.

**Recommendation**: Add acceptance criteria: "Each mechanism must be testable via dry-run or simulation without affecting production."

### Missing: Observability Before Emergency

The decision drivers include "Incident response must be fast" but fast response requires fast detection. The SLI/SLO options address this somewhat, but there's no discussion of alerting latency.

If the SLI is "success rate per batch" and batches run nightly, detection latency is ~24 hours. This may not meet "within minutes" requirements.

**Recommendation**: Add explicit detection latency requirements to Decision 4 options.

### Inconsistency: Circuit Breaker Treatment

The upstream design (DESIGN-registry-scale-strategy.md) specifies circuit breaker as Phase 1b requirement: "Circuit breaker: Auto-pause ecosystem if success rate <50% for 10 consecutive attempts."

But the batch operations design doesn't include circuit breaker as an option in Decision 2 (Emergency Stop) or Decision 4 (SLI/SLO). This is a gap - the circuit breaker straddles both decisions.

**Recommendation**: Add explicit "Circuit Breaker" subsection that cross-references both decisions, or create Decision 6: "Automatic Response Mechanism."

---

## Summary of Recommendations

### High Priority (Before Options Selection)

1. **Quantify scale assumptions**: Batch size, frequency, detection latency
2. **Add Option 1D**: Batch ID metadata for surgical rollback
3. **Add Option 2D**: Circuit breaker as automatic emergency stop
4. **Validate Assumption 6**: GitHub spending limit behavior (blocking gate)
5. **Reconcile circuit breaker**: Currently missing despite upstream requirement

### Medium Priority (Before Implementation)

6. Add missing pros/cons identified above (merge conflicts, split-brain, etc.)
7. State assumptions explicitly in design document
8. Add "Runbook Requirements" section with R2 runbook as template
9. Add testing strategy for operational mechanisms
10. Address detection latency in SLI/SLO options

### Lower Priority (Can Iterate)

11. Merge Options 1A and 1B (1B is dominated)
12. Rename Option 2A to clarify it's a nuclear option
13. Add Option 3D (time-windowed budget)
14. Add Option 5D (GitHub Actions artifacts)

---

## Conclusion

The design captures the right problems but the options analysis needs strengthening before decisions can be made confidently. The most significant gaps are:

1. **Missing batch ID option** for rollback - this solves the explicitly stated "identifying which commits" problem
2. **Missing circuit breaker option** - required by upstream but absent here
3. **Unvalidated assumptions** about scale, dependencies, and GitHub behavior

The existing R2 runbook demonstrates that this codebase has strong operational documentation patterns. This design should follow that precedent.
