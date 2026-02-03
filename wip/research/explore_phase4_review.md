# Phase 4 Review: Pipeline Dashboard

## Problem Statement Assessment

The problem statement is adequately specific for evaluating solutions. It clearly identifies:

1. **Data sources**: Queue status, failure records, run metrics - all with concrete file paths and schemas
2. **Current pain points**: Manual jq queries, no correlation, no historical view
3. **Scope boundaries**: What's in (visualization) vs what's out (re-queue triggers, real-time updates)

**What's missing:**

- **User personas**: Who are the "operators" needing this dashboard? Is it just maintainers, or could external contributors benefit? This affects deployment location decisions.

- **Access frequency**: How often do operators check pipeline status? Daily? After each batch run? This impacts whether CI generation is sufficient or if on-demand refresh matters.

- **Mobile/offline requirements**: The design assumes browser access. Are operators ever checking status from mobile or offline contexts where a CLI-first approach would be better?

- **Success criteria**: No measurable outcomes defined. What would make this dashboard "successful"? Reduced time-to-fix for blockers? Fewer manual jq queries? This makes it harder to evaluate whether the chosen approach delivers value.

## Missing Alternatives

### Decision 1: Visualization Format

**GitHub Actions Workflow Summary (missing)**: GitHub Actions supports `$GITHUB_STEP_SUMMARY` for markdown output directly in the workflow run page. The batch pipeline already uses this. A summary could include queue status, top blockers, and failure breakdown without any additional infrastructure. Users already check the Actions page after batch runs.

- Pros: Zero new infrastructure, integrates with existing workflow, no additional files to maintain
- Cons: Only visible after navigating to Actions tab, no single-page overview

**Markdown Report in Repo (missing)**: Generate `docs/pipeline-status.md` with tables and embedded Mermaid charts. GitHub renders markdown natively.

- Pros: No HTML/JS maintenance, works with GitHub's rendering, version-controlled
- Cons: Less interactive than HTML, Mermaid chart limitations

### Decision 2: Data Processing

**Make/Makefile target (missing)**: A Makefile target wrapping jq commands provides documentation and discoverability without adding a new script. Projects already use `make` for builds.

**Embedded jq in workflow YAML (missing)**: For simple aggregations, inline jq in the workflow step may be simpler than maintaining a separate script. The batch-generate workflow could output dashboard.json as a step artifact.

### Decision 4: Dashboard Content

**Tier-based breakdown (missing)**: The queue has tier 1/2/3 priority. Showing success/failure/blocked by tier would answer "are high-priority packages getting processed?" which is a key operator question.

**Time-to-resolution tracking (missing)**: How long do packages stay in "failed" or "blocked" status? This would answer "is the backlog growing or shrinking?"

## Rejection Rationale Review

### Decision 1: CLI Only

> "Rejected because shareable visualizations and historical trends are hard to express in terminal output."

This rationale is **partially fair but overstated**.

- Terminal output can be saved and shared (paste into Slack/Discord, pipe to file)
- Historical trends are genuinely hard in CLI (valid point)
- The existing `batch-metrics.sh` already shows the pattern works for quick checks
- A better framing: "CLI is retained for quick checks; HTML adds visualization for trends and sharing"

The design actually chooses "both" (Static HTML + CLI scripts), so this isn't really a rejection - it's a comparison to justify adding HTML.

### Decision 1: Real-time Dashboard

> "Rejected because this requires infrastructure (#1190 scope)."

This is **fair and well-scoped**. The design explicitly frames itself as an intermediate solution before #1190. No strawman here.

### Decision 2: Python Script

> "Rejected because existing data processing scripts use shell+jq consistently. Python would add a dependency and break the pattern."

This is **fair but could be stronger**. The reasoning could add:
- CI environments already have jq; Python would need verification
- Shell scripts are copy-pasteable for debugging
- The data transformations are simple aggregations, not complex logic

### Decision 2: Go Tool

> "Rejected because this is a reporting tool, not a core CLI feature. Shell scripts are easier to iterate on and sufficient for the data volume."

This is **fair**. A Go tool would be overkill for JSON aggregation that jq handles well. The "easier to iterate" point is valid for a new feature with evolving requirements.

### Decision 3: Manual Generation

> "Rejected because it defeats the purpose of quick visibility."

This is **fair**. If operators need to remember to run a script, they might as well run jq directly.

### Decision 3: Scheduled Cron

> "Rejected because it would commit unchanged files and add noise."

This is **partially fair but solvable**. A cron job could:
- Check if data files changed before regenerating
- Skip commit if dashboard.json is unchanged
- Only run if batch pipeline ran since last dashboard update

The rejection should note that cron adds complexity for marginal benefit over "regenerate on batch completion."

### Decision 4: Minimal Status Page

> "Knowing there are 8 failures is less useful than knowing which 2 dependencies block 6 of them."

This is **excellent rationale**. It's specific, actionable, and demonstrates understanding of operator needs.

### Decision 4: Comprehensive Dashboard

> "Rejected for initial scope. Start simple, add detail if operators find the basic dashboard useful."

This is **fair and demonstrates good scope discipline**. However, it could note what specific features would be added in a "phase 2" if the basic dashboard proves useful.

## Unstated Assumptions

1. **jq availability**: The design assumes jq is available in CI and on operator machines. This is likely true for GitHub Actions (ubuntu-latest has jq) but should be documented.

2. **JSONL append-only model is stable**: The design reads JSONL files that grow over time. There's no discussion of what happens when these files get large (thousands of lines). The "Uncertainties" section mentions this but doesn't quantify thresholds.

3. **Operators use web browsers**: The HTML dashboard assumes operators can access `website/pipeline/`. If tsuku.dev is public, this data becomes public. Is queue status and failure data sensitive?

4. **GitHub Pages/Cloudflare Pages deployment**: The design mentions `website/pipeline/index.html` following the `website/stats/` pattern, implying it will be deployed. The deployment mechanism should be explicit.

5. **Single ecosystem focus**: The design says "focus on homebrew initially" but the data files support multiple ecosystems. The dashboard should either:
   - Explicitly filter to homebrew only (and document why)
   - Support ecosystem selection from the start

6. **PR #1422 for batch-runs.jsonl**: The design references an unmerged PR for metrics data. The dashboard should handle missing metrics gracefully (noted in Uncertainties, but implementation detail is missing).

7. **Website styling reuse**: The design mentions following `website/stats/` patterns. This assumes the CSS and structure there are suitable for a data-heavy dashboard with tables and charts.

## Strawman Check

**No options appear to be strawmen.** Each rejected alternative has a plausible use case and the rejection rationale is specific to this project's constraints.

The closest to a strawman is "Comprehensive dashboard" since it's vaguely defined as "include per-package details, historical trends, drill-down" without specifics. But the rejection is reasonable scope discipline rather than a setup to fail.

**Potential concern**: The "Real-time dashboard" rejection could be seen as framing #1190 as unnecessarily complex. However, the design explicitly positions itself as complementary to (not replacing) #1190, so this seems fair.

## Recommendations

### 1. Add Success Criteria

Define measurable outcomes:
- "Operator can identify top 3 blocking dependencies in under 30 seconds"
- "Dashboard generation adds less than 10 seconds to batch pipeline runtime"
- "No manual jq queries needed for standard status checks"

### 2. Consider GitHub Actions Summary as Primary Output

Before adding HTML, evaluate whether enhanced `$GITHUB_STEP_SUMMARY` output meets 80% of needs:
- Add a summary table to the merge job
- Include top blockers and failure categories
- Link to the Actions run from Slack/Discord notifications

This is zero-infrastructure and operators already check the Actions page.

### 3. Clarify Data Privacy

If the dashboard is deployed to tsuku.dev:
- Is queue data (package names, failure messages) appropriate for public visibility?
- Should the dashboard be in a separate, non-indexed directory?
- Consider adding to `.gitignore` if sensitive

### 4. Add Tier Breakdown to Dashboard Content

The queue has tier 1/2/3 priority. The dashboard should show:
- Status breakdown by tier (are tier 1 packages being prioritized?)
- Which tier has the most blockers?

This directly answers "what should I fix next?" with priority context.

### 5. Document JSONL Growth Strategy

The design notes JSONL files grow but doesn't specify:
- When to rotate/archive old records
- Maximum expected size before performance degrades
- Whether the dashboard should only read recent records (last 30 days?)

### 6. Specify Graceful Degradation

Define behavior when:
- `batch-runs.jsonl` is missing (PR #1422 not merged)
- `priority-queue.json` has no entries for an ecosystem
- Failure records have schema version mismatch

### 7. Consider Makefile Integration

Instead of a standalone script:
```makefile
dashboard:
    @./scripts/generate-dashboard.sh

dashboard-serve: dashboard
    @python3 -m http.server -d website/pipeline 8080
```

This provides discoverability and integrates with existing developer workflows.
