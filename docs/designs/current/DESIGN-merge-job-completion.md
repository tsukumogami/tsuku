---
status: Current
problem: |
  The batch-generate merge job has working constraint derivation and PR creation but lacks structured commit messages with batch metadata, SLI metrics collection, circuit breaker state updates, and auto-merge gating required by DESIGN-batch-recipe-generation.md for pipeline observability and scale.
decision: |
  Add four shell-based steps to the merge job: batch_id generation, structured commit with git trailers, SLI metrics appended to data/metrics/batch-runs.jsonl, and circuit breaker update via scripts/update_breaker.sh. Auto-merge is enabled only for clean batches with zero exclusions.
rationale: |
  Shell-first approach matches the existing merge job pattern and avoids adding Go build dependencies. Conservative auto-merge (fail-open) is appropriate for an unproven pipeline. Generating batch_id in the merge job keeps it loosely coupled from the Go orchestrator while using the same format for rollback compatibility.
---

# DESIGN: Merge Job Completion

## Status

Current

## Implementation Issues

### Milestone: [Merge Job Completion](https://github.com/tsukumogami/tsuku/milestone/63)

Core merge job capabilities that make the pipeline functionally complete: batch_id for rollback, recipe tracking for queue updates, circuit breaker protection.

| Issue | Dependencies | Tier |
|-------|--------------|------|
| ~~[#1349: ci(batch): add batch_id generation and structured commit message](https://github.com/tsukumogami/tsuku/issues/1349)~~ | ~~None~~ | ~~testable~~ |
| _Generates a `BATCH_ID` in `{date}-{ecosystem}` format with sequence numbers for same-day batches, and restructures the commit message to include git trailers (`batch_id`, `ecosystem`, `batch_size`, `success_rate`) for rollback support._ | | |
| ~~[#1350: ci(batch): add recipe list tracking to merge job](https://github.com/tsukumogami/tsuku/issues/1350)~~ | ~~None~~ | ~~simple~~ |
| _Accumulates `INCLUDED_RECIPES` and `EXCLUDED_RECIPES` name lists during the constraint derivation loop, making recipe-level data available to queue updates and auto-merge gating._ | | |
| ~~[#1352: ci(batch): add circuit breaker and queue status updates](https://github.com/tsukumogami/tsuku/issues/1352)~~ | ~~[#1350](https://github.com/tsukumogami/tsuku/issues/1350)~~ | ~~testable~~ |
| _Calls `scripts/update_breaker.sh` with batch outcome and updates `data/priority-queue.json` with per-recipe success/failed statuses using the recipe lists from #1350._ | | |

### Milestone: [Merge Job Observability](https://github.com/tsukumogami/tsuku/milestone/64)

Observability and scale improvements: SLI metrics for trend analysis and auto-merge for throughput. The pipeline works without these but they're needed at target scale (200+ recipes/week).

| Issue | Dependencies | Tier |
|-------|--------------|------|
| ~~[#1351: ci(batch): add SLI metrics collection to merge job](https://github.com/tsukumogami/tsuku/issues/1351)~~ | ~~[#1349](https://github.com/tsukumogami/tsuku/issues/1349)~~ | ~~testable~~ |
| _Appends a structured JSON line to `data/metrics/batch-runs.jsonl` with per-platform tested/passed/failed counts, recipe totals, and duration. Uses the batch_id from #1349 as the primary key._ | | |
| ~~[#1353: ci(batch): add auto-merge gating for clean batches](https://github.com/tsukumogami/tsuku/issues/1353)~~ | [#1350](https://github.com/tsukumogami/tsuku/issues/1350) | testable |
| _Enables `gh pr merge --auto --squash` when `EXCLUDED_COUNT=0` and leaves a PR comment explaining why auto-merge was skipped otherwise. Fail-open policy: the PR is always created regardless._ | | |

## Upstream Design Reference

This design completes the merge job (Job 4) from [DESIGN-batch-recipe-generation.md](DESIGN-batch-recipe-generation.md). The platform validation foundation was delivered by [DESIGN-batch-platform-validation.md](DESIGN-batch-platform-validation.md) (M60).

**Relevant upstream sections:**
- Job Architecture (Job 4: merge)
- Batch ID and Commit Format
- Circuit Breaker Integration
- SLI Collection
- Data Flow

**This design delivers:** #1256 (platform constraints in merge job), partially #1257 (SLI metrics collection)

**Note on #1256's validation script:** Issue #1256 includes a validation script that references `cmd/batch-merge` -- a Go tool. The batch pipeline uses shell steps in GitHub Actions rather than a separate Go merge tool. The merge job already performs the same operations (aggregation, constraint derivation, PR creation) in shell. This design completes the merge job in shell rather than introducing a new Go binary. The #1256 validation script should be updated when the issue is closed.

## Context and Problem Statement

The batch-generate workflow has a working merge job that aggregates platform validation results from 11 environments, derives platform constraints for partial-coverage recipes via `scripts/write-platform-constraints.sh`, writes failure records to `data/failures/<eco>.jsonl`, and creates a PR with a detailed body. This was built as part of M60 (Batch Multi-Platform Validation Foundation).

What's missing is the operational metadata that makes the pipeline auditable, controllable, and observable at scale:

1. **No batch provenance in commits.** The merge job creates a generic commit message (`feat(recipes): add batch <date> <eco> recipes`) without the `batch_id`, `success_rate`, or platform breakdown trailers specified in DESIGN-batch-recipe-generation.md. Without batch_id in the commit, `scripts/rollback-batch.sh` can't find commits to revert via `git grep`, and operators can't correlate a merged PR to a specific batch run.

2. **No SLI metrics.** The upstream design specifies per-platform metrics in `data/metrics/batch-runs.jsonl` (tested/passed/failed counts per platform, duration, merged count). This data doesn't exist yet, so there's no way to track success rates over time or detect degradation.

3. **No circuit breaker updates.** The `batch-control.json` file and `scripts/update_breaker.sh` exist, but the merge job doesn't call the breaker update script after aggregating results. A broken ecosystem can run indefinitely without tripping the breaker.

4. **No auto-merge.** The upstream design specifies `gh pr merge --auto --squash` when all recipes pass validation and none contain `run_command`. Currently the PR is always left open for manual merge.

These gaps block #1258 (PR CI platform filtering) which depends on #1256, and prevent the batch pipeline from operating at the target scale of 200+ recipes/week.

### Scope

**In scope:**
- Structured commit messages with batch_id and platform metrics trailers
- SLI metrics collection to `data/metrics/batch-runs.jsonl`
- Circuit breaker state updates via existing `scripts/update_breaker.sh`
- Auto-merge gating with configurable gates
- Queue status updates (in_progress -> success/failed)

**Out of scope:**
- Circuit breaker implementation changes (the script exists; we just wire it)
- Telemetry Worker integration (D1 upload is Phase 2 infrastructure)
- Progressive validation (deferred per DESIGN-batch-platform-validation.md)
- Preflight job (#1252, separate issue)

## Decision Drivers

- **Existing infrastructure**: `batch-control.json`, `scripts/update_breaker.sh`, `scripts/rollback-batch.sh`, `internal/batch/` Go package all exist and define the interfaces. The design should wire these together, not reinvent them.
- **Rollback requires batch_id**: `scripts/rollback-batch.sh` uses `git grep` to find commits containing a specific batch_id. Without trailers in the commit message, rollback is manual.
- **Observability before scale**: The pipeline can't move to 200+ recipes/week without metrics showing success rates and platform coverage trends.
- **Conservative auto-merge**: Auto-merge should fail open (leave PR for human review) rather than fail closed (block the pipeline). A recipe that passes validation on at least one platform is safe to merge with constraints.
- **Shell-first**: The merge job runs in GitHub Actions shell steps. Adding Go tools would require build steps and increase complexity. Shell scripts with `jq` match the existing pattern.

## Implementation Context

### Existing Patterns

**Batch orchestrator** (`internal/batch/orchestrator.go`): Generates `batch_id` as `{YYYY-MM-DD}-{ecosystem}`. Tracks results per package. Updates queue statuses. The merge job in the workflow should produce the same `batch_id` format.

**Circuit breaker scripts** (`scripts/check_breaker.sh`, `scripts/update_breaker.sh`): Read and update `batch-control.json`. The update script takes positional args `<ecosystem> <success|failure>` -- a binary outcome per batch run. It tracks consecutive failures internally (threshold: 5, configurable via `FAILURE_THRESHOLD`). State transitions: closed + success resets failures to 0; closed + failure increments (opens at threshold); half-open + success closes; half-open + failure reopens with fresh timeout (60 min default).

**Rollback script** (`scripts/rollback-batch.sh`): Finds commits containing `batch_id: <id>` via `git grep` and creates a revert branch. Requires the batch_id trailer in commit messages.

**Failure JSONL schema** (`data/schemas/failure-record.schema.json`): Defines the structure for `data/failures/<eco>.jsonl`. The merge job already writes to this format.

**SLI format** (from DESIGN-batch-recipe-generation.md): One JSON line per batch run in `data/metrics/batch-runs.jsonl` with per-platform breakdown.

### Conventions to Follow

- Commit trailers use `key: value` format (lowercase, no quotes)
- Shell scripts in `scripts/` with `set -euo pipefail`
- `jq` for JSON manipulation in workflow steps
- `batch-control.json` at repo root
- Metrics files append-only in `data/metrics/`

## Considered Options

### Decision 1: How to generate batch_id in the workflow

The Go orchestrator generates batch_id as `{date}-{ecosystem}`, but the GitHub Actions merge job runs after the orchestrator. The merge job needs a batch_id for commit trailers, metrics, and rollback support. The question is whether the orchestrator should pass the batch_id as an artifact, or the merge job should generate its own.

#### Chosen: Merge job generates batch_id independently

The merge job generates `batch_id` using the same format (`$(date -u +%Y-%m-%d)-${{ inputs.ecosystem }}`) as the Go orchestrator. The `-u` flag uses UTC to avoid timezone-dependent dates. If multiple batches run on the same day for the same ecosystem, the batch_id includes a sequence number derived from existing commits: `2026-01-29-homebrew-002`.

This avoids artifact coupling between the generate and merge jobs. The merge job already downloads recipe and result artifacts; adding a batch_id artifact for a single string is overhead.

One edge case: if the generate job starts before midnight UTC and the merge job runs after midnight, they'd compute different dates. The concurrency group `batch-generate` serializes runs, so this only happens if a single run spans midnight -- unlikely given typical 30-60 minute runtimes, and the consequence is cosmetic (batch_id date is off by a day).

#### Alternatives Considered

**Pass batch_id via artifact**: The generate job writes `batch_id.txt` as an artifact, merge job downloads it. Rejected because it adds an artifact upload/download round-trip for a single string that can be derived from the workflow inputs and date.

**Use GitHub run ID**: Use `${{ github.run_id }}` as the batch_id. Rejected because it's an opaque number that doesn't convey ecosystem or date, making rollback queries harder and breaking compatibility with the Go orchestrator's format.

### Decision 2: Where to collect SLI metrics

The upstream design specifies `data/metrics/batch-runs.jsonl` as an append-only file committed to the repo. The question is whether metrics collection belongs in the merge job (shell step) or in a separate post-merge workflow.

#### Chosen: Inline in the merge job

The merge job already has all the data needed for metrics: platform result counts, recipe counts, timing. Adding a `jq` step that constructs the metrics line and appends it to `data/metrics/batch-runs.jsonl` keeps everything in one place. The file gets committed alongside the recipes and failure records.

#### Alternatives Considered

**Separate metrics workflow**: A post-merge workflow triggered by the batch PR reads commit trailers and computes metrics. Rejected because it requires parsing trailers back out of the commit (fragile), runs after merge (can't include pre-merge data like duration), and adds workflow complexity.

**Telemetry Worker only**: Send metrics to `https://telemetry.tsuku.dev/batch-metrics` without local storage. Rejected because the Worker endpoint exists but depends on external infrastructure, and the upstream design explicitly specifies repo-committed JSONL as the Phase 1 approach.

### Decision 3: Auto-merge gating strategy

The upstream design says auto-merge is enabled when all recipes pass on at least one platform, no `run_command` actions are present, and CI checks pass. The question is how strict the gates should be and what happens when they fail.

#### Chosen: Conservative auto-merge with fail-open

Enable `gh pr merge --auto --squash` only when:
1. All included recipes passed on >= 1 platform (already enforced by exclusion logic)
2. No recipes were excluded due to `run_command` (the PR itself is clean, but the exclusion signals elevated risk)
3. The `EXCLUDED_COUNT` is 0 (no recipes were dropped for any reason)

If any gate fails, the PR is left open with a comment explaining why auto-merge was skipped. This is fail-open: the pipeline creates the PR regardless, but only enables auto-merge for clean batches.

**Note: open-PR guard.** If a previous batch PR is still open (not yet merged or closed), triggering a new batch run creates a second PR that may conflict. The preflight job (#1252) should check for open batch PRs and skip the run if one exists. This isn't implemented here since it belongs in the preflight job, but it's a prerequisite for safe auto-merge at scale.

#### Alternatives Considered

**Always auto-merge**: Enable auto-merge on every batch PR since excluded recipes are already removed. Rejected because a batch with many exclusions may indicate a systemic issue worth human review, and `run_command` exclusions specifically should draw attention.

**Never auto-merge**: Always leave PRs for manual merge. Rejected because the target scale of 200+ recipes/week makes manual merge unsustainable, and the validation pipeline provides sufficient confidence for clean batches.

### Uncertainties

- Whether the sequence number for same-day batch_ids will collide if multiple workflows run concurrently (mitigated by the concurrency group `batch-generate` which serializes runs)
- How often auto-merge gates will block in practice (depends on ecosystem `run_command` prevalence, unknown until first batches run)
- Whether `data/metrics/batch-runs.jsonl` will cause merge conflicts if concurrent runs happen (mitigated by concurrency group)

## Decision Outcome

**Chosen: Shell-based merge job additions with conservative auto-merge**

### Summary

The merge job in `batch-generate.yml` gains four new steps after the existing constraint derivation and PR creation logic. First, a batch_id generation step produces an ID in the format `{date}-{ecosystem}` (matching the Go orchestrator) and derives a sequence number from existing same-day commits. Second, the commit message is restructured to include git trailers (`batch_id`, `ecosystem`, `batch_size`, `success_rate`) that `scripts/rollback-batch.sh` can find via `git grep`. Third, an SLI metrics step appends a JSON line to `data/metrics/batch-runs.jsonl` with per-platform tested/passed/failed counts, total recipe counts, and duration. Fourth, a circuit breaker update step calls `scripts/update_breaker.sh` with the ecosystem and pass/fail counts.

Auto-merge is enabled via `gh pr merge --auto --squash` only for clean batches where no recipes were excluded for any reason. When auto-merge is skipped, a PR comment explains why. The queue status update step sets processed packages to `success` or `failed` in `data/priority-queue.json`.

All additions use shell and `jq`, matching the existing merge job pattern. No Go code changes are needed. The `data/metrics/` directory is created automatically if missing.

### Rationale

The shell-first approach avoids adding build dependencies to the merge job. The Go orchestrator already handles generation and initial validation; the merge job's role is aggregation, metadata writing, and PR management. These are text manipulation tasks where shell and jq are the right tools.

The conservative auto-merge policy reflects the current stage of the pipeline: it's unproven at scale. Once success rates stabilize and operators gain confidence, the gates can be loosened (e.g., allowing auto-merge even with some exclusions). Starting strict and relaxing is safer than starting loose and discovering problems.

Batch_id generation in the merge job (rather than passing it from the orchestrator) keeps the jobs loosely coupled. The orchestrator may evolve independently, and the merge job doesn't need to know about orchestrator internals beyond the recipe and result artifacts.

### Trade-offs Accepted

- **Duplicate batch_id logic**: The merge job and Go orchestrator both generate batch_ids. If the format changes, both must be updated. Acceptable because the format is simple and unlikely to change.
- **SLI metrics in repo**: `batch-runs.jsonl` grows the repo. At ~200 bytes per line and weekly batches, this is ~10KB/year. Acceptable, and a cleanup script can archive old records.
- **Conservative auto-merge delays**: Clean batches auto-merge, but any exclusion triggers manual review. This may slow throughput initially. Acceptable as a safety measure for an unproven pipeline.

## Solution Architecture

### Overview

Four new steps are added to the merge job in `batch-generate.yml`, inserted between the existing constraint derivation step and the PR creation step. A new directory `data/metrics/` holds the SLI metrics file.

### Batch ID Generation

```bash
# Generate batch_id matching Go orchestrator format
BATCH_DATE=$(date -u +%Y-%m-%d)
BATCH_ID="${BATCH_DATE}-${{ inputs.ecosystem }}"

# Check for existing same-day batches to add sequence number
EXISTING=$(git log --oneline --all --grep="batch_id: ${BATCH_ID}" | wc -l)
if [ "$EXISTING" -gt 0 ]; then
  SEQ=$((EXISTING + 1))
  BATCH_ID="${BATCH_ID}-$(printf '%03d' $SEQ)"
fi
```

### Structured Commit Message

```
feat(recipes): add batch 2026-01-29-homebrew recipes

Batch generation of 25 packages from homebrew ecosystem.
- 23 passed validation across 11 platforms
- 2 excluded (recorded to data/failures/homebrew.jsonl)

batch_id: 2026-01-29-homebrew
ecosystem: homebrew
batch_size: 25
success_rate: 0.92
```

The trailers are machine-parseable by `scripts/rollback-batch.sh` and `git log --grep`.

### SLI Metrics Format

Appended to `data/metrics/batch-runs.jsonl`:

```json
{
  "batch_id": "2026-01-29-homebrew",
  "ecosystem": "homebrew",
  "total": 25,
  "generated": 23,
  "platforms": {
    "linux-x86_64": {"tested": 23, "passed": 22, "failed": 1},
    "linux-arm64": {"tested": 23, "passed": 20, "failed": 3},
    "darwin-arm64": {"tested": 23, "passed": 21, "failed": 2},
    "darwin-x86_64": {"tested": 23, "passed": 21, "failed": 2}
  },
  "merged": 20,
  "excluded": 3,
  "constrained": 2,
  "timestamp": "2026-01-29T10:30:00Z",
  "duration_seconds": 450
}
```

The `platforms` object uses the validation job names (not individual family IDs) as keys. Each job aggregates its family results. This matches the 4 result artifacts the merge job downloads.

### Circuit Breaker Integration

After metrics collection, determine the batch outcome and call the breaker script:

```bash
# Binary outcome: success if any recipes were included, failure if all excluded
if [ "$INCLUDED_COUNT" -gt 0 ]; then
  OUTCOME="success"
else
  OUTCOME="failure"
fi

scripts/update_breaker.sh "${{ inputs.ecosystem }}" "$OUTCOME"
```

The script takes positional args `<ecosystem> <success|failure>`. It tracks consecutive failures internally (opens at 5 consecutive failures, recovers after 60 min half-open). The preflight job (#1252) reads `batch-control.json` and skips ecosystems with open breakers.

### Auto-Merge Gating

```bash
if [ "$EXCLUDED_COUNT" = "0" ]; then
  gh pr merge --auto --squash "$PR_URL"
  echo "Auto-merge enabled: clean batch with no exclusions"
else
  gh pr comment "$PR_URL" --body "Auto-merge skipped: $EXCLUDED_COUNT recipe(s) excluded. Manual review recommended."
  echo "Auto-merge skipped: $EXCLUDED_COUNT exclusions"
fi
```

### Recipe List Tracking

The existing aggregation loop already tracks `INCLUDED_COUNT` and `EXCLUDED_COUNT`. For queue updates and commit trailers, we also need the recipe names. These are collected during the constraint derivation loop:

```bash
INCLUDED_RECIPES=""
EXCLUDED_RECIPES=""
# ... inside the existing recipe processing loop:
# On include: INCLUDED_RECIPES="$INCLUDED_RECIPES $recipe"
# On exclude: EXCLUDED_RECIPES="$EXCLUDED_RECIPES $recipe"
```

### Queue Status Updates

After PR creation (committed alongside recipes so queue stays in sync), update `data/priority-queue.json`:

```bash
# Mark included recipes as success
for recipe in $INCLUDED_RECIPES; do
  jq --arg pkg "${{ inputs.ecosystem }}:${recipe}" \
    '(.packages[] | select(.id == $pkg)).status = "success"' \
    data/priority-queue.json > tmp.json && mv tmp.json data/priority-queue.json
done

# Mark excluded recipes (zero-pass) as failed
for recipe in $EXCLUDED_RECIPES; do
  jq --arg pkg "${{ inputs.ecosystem }}:${recipe}" \
    '(.packages[] | select(.id == $pkg)).status = "failed"' \
    data/priority-queue.json > tmp.json && mv tmp.json data/priority-queue.json
done
```

Queue updates are committed alongside the recipes so the queue file stays in sync with the PR.

### Data Flow

```
merge job receives:
  ← validation-results-*.json (4 platform artifacts)
  ← passing-recipes (recipe TOML files)

merge job produces:
  → recipes/*.toml (with platform constraints)
  → data/failures/<eco>.jsonl (platform failures)
  → data/metrics/batch-runs.jsonl (SLI metrics line)
  → batch-control.json (circuit breaker state)
  → data/priority-queue.json (queue status updates)
  → PR (with structured commit + auto-merge if clean)
```

## Implementation Approach

### Single Phase

All changes are in `.github/workflows/batch-generate.yml` (merge job steps) and a new `data/metrics/` directory. No Go code changes needed.

1. Add batch_id generation step
2. Restructure commit message with trailers
3. Add SLI metrics collection step
4. Add circuit breaker update step
5. Add auto-merge gating step
6. Add queue status update step
7. Create `data/metrics/.gitkeep`

Dependencies: M60 foundation (done), `scripts/update_breaker.sh` (exists)

## Security Considerations

### Download Verification

Not applicable -- this design doesn't change download behavior. The merge job aggregates existing validation results and writes metadata. All binary downloads happen in the validation jobs (covered by DESIGN-batch-platform-validation.md).

### Execution Isolation

The merge job runs on `ubuntu-latest` with `contents: write` and `pull-requests: write` permissions. The new steps use `jq`, `git`, and `gh` -- no additional permissions needed. Auto-merge uses the existing `PAT_BATCH_GENERATE` secret.

One consideration: the auto-merge step calls `gh pr merge --auto --squash`, which enables merge without human approval. This is gated by the conservative exclusion check and by GitHub's branch protection rules (required CI checks must pass before merge completes).

### Supply Chain Risks

The circuit breaker update modifies `batch-control.json`, which controls whether future batch runs proceed. A compromised merge job could disable the circuit breaker, allowing a broken ecosystem to continue generating. This is mitigated by:
- Branch protection requires CI to pass
- The circuit breaker script validates inputs
- The concurrency group prevents parallel modifications

### User Data Exposure

No user data is accessed or transmitted. The new steps produce:
- Commit trailers (public, in git history)
- SLI metrics (public, in repo)
- Circuit breaker state (public, in repo)
- Queue status updates (public, in repo)

All data is derived from CI results, not user activity.

## Consequences

### Positive

- Batch commits become auditable and rollback-able via batch_id trailers
- SLI metrics enable trend analysis and degradation detection
- Circuit breaker prevents runaway failures at scale
- Auto-merge removes manual bottleneck for clean batches
- Queue status stays synchronized with PR outcomes

### Negative

- Auto-merge reduces human oversight for batch recipes
- SLI and queue files grow the repo (marginally)
- Batch_id logic duplicated between Go orchestrator and shell

### Mitigations

- Auto-merge is conservative (fail-open) and gated by branch protection
- Append-only files grow slowly (~10KB/year for metrics)
- Batch_id format is simple and unlikely to change; both implementations are < 5 lines
