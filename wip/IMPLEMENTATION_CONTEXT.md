---
summary:
  constraints:
    - Must trigger only on push to main when state files change (data/queues/**, data/metrics/**, data/failures/**)
    - Must use concurrency group 'update-dashboard' with cancel-in-progress: false
    - Must include retry logic with exponential backoff (3 attempts: 2s, 4s, 8s delays)
    - Must check for no-op case (don't commit if dashboard unchanged after generation)
    - Must use github-actions[bot] identity for commits
    - Must have permissions: contents: write to commit directly to main
    - Dashboard updates must happen post-merge, not in PRs (eliminates final conflict source)
  integration_points:
    - cmd/queue-analytics - CLI tool that aggregates state files and generates dashboard.json
    - .github/workflows/batch-generate.yml - Remove dashboard generation from merge job after this workflow is deployed
    - website/pipeline/dashboard.json - Output file that gets committed
    - data/queues/**, data/metrics/**, data/failures/** - Input files that trigger the workflow
    - Git operations - Commit and push with retry logic to handle concurrent updates
  risks:
    - If workflow fails, dashboard stays stale until next PR merges
    - Concurrent pushes from rapid successive merges could fail (mitigated by retry logic)
    - Dashboard shows stale data for ~60 seconds between PR merge and workflow completion (acceptable trade-off)
    - If queue-analytics has bugs, could generate incorrect dashboard (same risk as current design)
    - Workflow could be triggered unnecessarily if non-state files in trigger paths change
  approach_notes: |
    Create .github/workflows/update-dashboard.yml with:
    1. Trigger: on push to main, paths filter for state files
    2. Concurrency: update-dashboard group to serialize runs
    3. Steps: checkout, setup-go, build queue-analytics, run it
    4. Check if dashboard changed (git diff --cached --quiet)
    5. If changed: commit with message "chore(dashboard): update pipeline dashboard"
    6. Push with retry logic: for loop 1..3, pull --rebase before push, exponential backoff

    Then remove dashboard generation from batch-generate.yml merge job (find where it runs
    queue-analytics and delete those steps). This completes the migration from in-PR to
    post-merge dashboard updates, eliminating the final conflict source.
---

# Implementation Context: Issue #1507

**Source**: docs/designs/DESIGN-batch-pr-coordination.md (None)

## Design Excerpt

---
status: Planned
problem: |
  The batch-generate workflow runs hourly, creating PRs that modify shared state files
  (data/priority-queue.json, data/metrics/batch-runs.jsonl, data/failures/*.jsonl,
  website/pipeline/dashboard.json, batch-control.json). When multiple batch runs complete
  before earlier PRs merge, all subsequent PRs have merge conflicts. Currently 16+ batch
  PRs are open with conflicts despite green CI checks, requiring 3-4 hours/day manual
  resolution and delaying recipe availability by 24-72 hours. Branch protection cannot be
  enabled because workflows write directly to main and making batch-specific checks required
  would block unrelated PRs.
decision: |
  Combine five coordinated changes: (1) Add preflight check to skip batch runs when an open
  PR exists for the ecosystem (prevents new conflicts), (2) Refactor state files into
  per-ecosystem queues and timestamped metrics/failures (eliminates 4 of 5 conflict sources),
  (3) Enable branch protection requiring 3 always-run checks (enables auto-merge and security),
  (4) Add concurrency groups and retry logic for workflows writing to main (serializes
  conflicting operations), (5) Manual review of 16 conflicting PRs using cleanup script
  (resolves existing backlog safely).
rationale: |
  This combination addresses all conflict sources while maintaining hourly batch velocity.
  Preflight check is simple (5-10 lines) and preserves cron schedule. State refactoring
  surgically targets conflict sources: split queues allow per-ecosystem independence,
  timestamped files eliminate append conflicts, post-merge dashboard avoids non-deterministic
  recomputation. Branch protection enables GitHub auto-merge without breaking non-recipe PRs.
  Concurrency groups serialize remaining conflicts (batch-control.json) without requiring
  complex locks. Manual cleanup is safer than automation for one-time backlog (prevents data
  loss). Rejected alternatives either added complexity (databases, external storage),
  sacrificed velocity (daily batches, waiting for merge), or failed to solve root causes
  (time-slicing, queue files).
---

## Post-Merge Dashboard Workflow (Issue #1507)

From the design's Solution Architecture section (lines 797-856):

**Post-Merge Dashboard Workflow:**

New file `.github/workflows/update-dashboard.yml`:
```yaml
name: Update Dashboard

on:
  push:
    branches: [main]
    paths:
      - 'data/queues/**'
      - 'data/metrics/**'
      - 'data/failures/**'

permissions:
  contents: write

concurrency:
  group: update-dashboard
  cancel-in-progress: false

jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Generate dashboard
        run: |
          go build -o queue-analytics ./cmd/queue-analytics
          ./queue-analytics

      - name: Commit dashboard
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add website/pipeline/dashboard.json
          if git diff --cached --quiet; then
            echo "No dashboard changes"
            exit 0
          fi
          git commit -m "chore(dashboard): update pipeline dashboard"

          # Retry logic (in case of concurrent pushes)
          for attempt in {1..3}; do
            if git push; then
              break
            else
              echo "Push failed (attempt $attempt/3), retrying..."
              sleep $((2 ** attempt))
              git pull --rebase
            fi
          done
```

This runs after every push to main that modifies state files, ensuring dashboard reflects merged state within ~60 seconds.

## Key Design Context

**Problem**: Dashboard is the last remaining conflict source. Currently generated during PR creation, which means:
- Each PR computes dashboard based on speculative state (what if this PR merged)
- Multiple PRs have different dashboard versions
- When one PR merges, all other PR dashboards become stale and conflict

**Solution**: Move dashboard generation to post-merge workflow
- Dashboard always reflects actual merged state (not speculative)
- No dashboard in PRs = no conflicts
- Updates happen within 60 seconds of merge (acceptable latency)
- Sequential processing via concurrency group prevents race conditions

**Integration with Other Changes**:
- Depends on #1506 (queue-analytics must support timestamped file aggregation)
- Blocks #1508 (branch protection enabled after this is stable)
- Part of Decision 2 (State Architecture Refactoring) from the design

**Critical Requirements**:
1. Only trigger on state file changes (avoid unnecessary runs)
2. Handle no-op case gracefully (don't commit if dashboard unchanged)
3. Retry logic for concurrent pushes (multiple PRs merging quickly)
4. Use concurrency group to serialize runs
5. Remove dashboard generation from batch-generate.yml after deploying this
