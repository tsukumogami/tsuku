---
# Context Summary (filled in during Phase 0)

problem: Per-recipe failure format written by CI lacks blocked_by field; dashboard blockers section shows stale data

key_files:
  - .github/workflows/batch-generate.yml (lines 722-731 - writes per-recipe failures)
  - internal/batch/results.go (FailureRecord struct has BlockedBy field)
  - internal/batch/orchestrator.go (sets status to "blocked" for missing_dep)
  - internal/dashboard/dashboard.go (reads both formats, builds blocker map)

integration_points:
  - CI workflow writes per-recipe failures to data/failures/<ecosystem>-<timestamp>.jsonl
  - Go orchestrator writes batch failures with blocked_by to same directory
  - Dashboard reads both formats, aggregates blockers from both

design_constraints:
  - Per-recipe format used for platform validation failures (exit_code mapping)
  - Batch format used for generation failures (blocked_by for missing_dep)
  - Both formats coexist in same directory, dashboard handles both

approach_summary: |
  The issue is that the CI workflow's per-recipe failure format (lines 722-731 of
  batch-generate.yml) doesn't include blocked_by. However, this format is for
  validation failures AFTER generation, not for missing_dep detection DURING
  generation.

  The blocked_by field should come from the Go orchestrator when it detects
  exit code 8 (missing_dep). Looking at orchestrator.go, this already sets
  blocked_by via parseInstallJSON().

  The problem seems to be that the queue status update to "blocked" isn't
  happening in the CI workflow - only the Go orchestrator sets this.

  Need to verify:
  1. Is the Go orchestrator actually running and writing failure records?
  2. Is the queue being updated with "blocked" status?
  3. Is the dashboard loading the correct failure files?
---

## Goal

Restore the ability to track which packages are blocked by missing dependencies in the new per-recipe failure format.

## Context

The batch recipe generation system tracks package failures in `data/failures/`. When we switched from batch-oriented to per-recipe failure tracking (Feb 7-8), we lost the `blocked_by` field that identified dependency blockers.

**Old format** (Jan 31, `data/failures/homebrew.jsonl`):
```json
{
  "schema_version": 1,
  "ecosystem": "homebrew",
  "failures": [{
    "package_id": "homebrew:node",
    "category": "missing_dep",
    "blocked_by": ["ada-url"],
    "message": "...",
    "timestamp": "2026-01-31T18:00:30Z"
  }]
}
```

**New format** (Feb 8, `data/failures/homebrew-2026-02-08T*.jsonl`):
```json
{
  "schema_version": 1,
  "recipe": "procs",
  "platform": "linux-alpine-musl-x86_64",
  "exit_code": 6,
  "category": "deterministic",
  "timestamp": "2026-02-08T02:37:10Z"
}
```

The new format lacks `blocked_by`, so:
- Packages needing dependencies show as `status: "failed"` instead of `status: "blocked"` in the queue
- Dashboard blockers section shows stale January data
- Can't distinguish dependency-blocked packages from genuinely failed packages
- Can't prioritize fixing blocker packages that unblock many others

## Root Cause Analysis

There are **two separate failure tracking systems**:

1. **Go orchestrator** (`internal/batch/orchestrator.go`):
   - Runs during generation phase
   - Detects exit code 8 → missing_dep with blocked_by field
   - Writes via `WriteFailures()` with full FailureRecord struct
   - Sets queue status to "blocked"

2. **CI workflow** (`.github/workflows/batch-generate.yml` lines 722-731):
   - Runs during validation phase
   - Writes per-recipe/platform failures
   - Uses simpler format without blocked_by
   - Only tracks validation failures (after generation)

The problem is the per-recipe format from CI is missing the blocked_by context.
Since validation happens AFTER successful generation, the missing_dep failures
should come from the Go orchestrator, not the CI workflow.

Need to verify:
- Is the orchestrator's WriteFailures being called?
- Are those files being included in the artifact upload?
- Is the dashboard correctly reading both formats?

## Acceptance Criteria

- [ ] Per-recipe failure records include `blocked_by` field when `category: "missing_dep"`
- [ ] Batch generation updates queue package `status` to `"blocked"` when failure has blockers
- [ ] Dashboard blockers section reflects current data (not January data)
- [ ] Queue analytics correctly counts packages by status including blocked
- [ ] Verified via manual batch run showing blocked packages tracked correctly

## Dependencies

None
