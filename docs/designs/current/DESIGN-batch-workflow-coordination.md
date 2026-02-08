---
status: Current
problem: |
  Batch recipe generation requires coordination between multiple concurrent workflows
  (homebrew, npm, pypi) without merge conflicts. The original single-file state design
  caused PRs to conflict when multiple ecosystems ran simultaneously, requiring manual
  intervention and slowing down recipe additions.
decision: |
  Implement per-ecosystem state files in batch-control.json and use GitHub App
  authentication to enable automated PR creation, validation, and merging. Each
  ecosystem updates only its own state section, eliminating conflicts. The system
  uses priority queues, circuit breakers, and SLI metrics to manage batch runs
  with automatic retry logic and race condition protection.
rationale: |
  Per-ecosystem state files provide natural isolation between concurrent batch runs.
  GitHub App tokens enable bypassing branch protection for automated updates while
  preserving human review requirements for manual PRs. The circuit breaker pattern
  prevents cascading failures, and priority queues ensure high-value packages are
  processed first. This design scales to multiple ecosystems running concurrently
  without coordination overhead or manual conflict resolution.
---

# Batch Recipe Workflow Coordination

**Status**: Current

## Context and Problem Statement

The tsuku package manager needs to continuously add new recipes from multiple package ecosystems (Homebrew, npm, PyPI, RubyGems, etc.). Manual recipe creation doesn't scale, requiring automation.

The batch workflow system generates recipes in batches, validates them across multiple platforms, and merges them automatically. Multiple ecosystems must run concurrently without conflicts.

**Previous issues:**
- Single-file state caused merge conflicts between concurrent PRs
- Manual conflict resolution slowed down recipe additions
- No automated coordination between ecosystem batch runs

## Decision Drivers

- **Zero manual intervention**: Batch runs should complete without human involvement
- **Concurrent execution**: Multiple ecosystems must run simultaneously
- **Quality gates**: All recipes must validate on target platforms before merge
- **Failure isolation**: One ecosystem's failures shouldn't block others
- **Auditability**: Track what was generated, when, and why decisions were made

## Decision Outcome

### Summary

The batch workflow system uses per-ecosystem state files, priority queues, circuit breakers, and GitHub App authentication to coordinate automated recipe generation and merging.

**Architecture layers:**
1. **Priority Queue** (`data/queues/priority-queue-{ecosystem}.json`): Tier-based package queue with status tracking
2. **Batch Generator** (`cmd/batch-generate`): Generates recipes from queue entries
3. **Platform Validation**: Parallel validation across Linux (x86_64, arm64) and macOS (Intel, Apple Silicon)
4. **Constraint Derivation**: Automatically writes platform constraints based on validation results
5. **Circuit Breaker** (`batch-control.json`): Per-ecosystem state prevents cascading failures
6. **Auto-merge**: GitHub App authentication enables automated PR merging after CI passes
7. **Dashboard Update**: Post-merge workflow updates dashboard.json with latest recipes

**Concurrency model:**
- Each ecosystem has its own state section in `batch-control.json`
- PRs only modify their ecosystem's state → no merge conflicts
- Workflows coordinate via race condition protection (rebase before PR creation)

**Quality gates:**
- Recipes must pass validation on at least one non-skipped platform
- All required CI checks must pass (lint, test, validate-recipes)
- Branch must be up-to-date with main (strict mode)

### Race Condition Scenarios and Mitigations

**Scenario 1: Same-ecosystem concurrent runs**
- Problem: Two homebrew batch workflows triggered within seconds
- Detection: Workflow checks for existing PRs with `batch:homebrew` label
- Mitigation: Second run exits early if PR already exists (prevents duplicate work)
- Fallback: If both pass the check, second fails at PR creation (GitHub enforces unique branch names)

**Scenario 2: Cross-ecosystem PR race**
- Problem: Homebrew PR merges to main during npm workflow's 6-minute generation phase
- Impact: npm workflow checked out main at T0, but main advanced at T3 (homebrew merge)
- Mitigation: Rebase with `origin/main` before creating PR branch (line 123 in workflow)
- Result: npm PR includes homebrew's merged state, no conflicts

**Scenario 3: State persistence race**
- Problem: Multiple workflows try to push circuit breaker state to main simultaneously
- Impact: Git push fails with "fetch first" error
- Mitigation: Retry logic with `git pull --rebase --autostash` (handles uncommitted failure logs)
- Result: Second push succeeds after incorporating first push's changes

**Why state persists before PR creation:**
Circuit breaker state must update even if no recipes were generated (all failed validation). This decouples failure tracking from PR lifecycle.

**Why rebase before PR creation:**
Without rebase, PR branch is based on stale main, causing auto-merge to fail with "branch out of date". Rebasing ensures clean merge with latest state from other ecosystems.

## Solution Architecture

### Component Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                          Seed Phase                              │
│                                                                  │
│  Priority Queue: data/queues/priority-queue-{ecosystem}.json    │
│  ├─ Tier 1: Critical (manually curated high-impact tools)       │
│  ├─ Tier 2: Popular (>10K weekly downloads)                     │
│  └─ Tier 3: Standard (all other packages)                       │
│                                                                  │
│  Status: pending → success|failed (updated by batch workflow)   │
└────────────────────────┬─────────────────────────────────────────┘
                         │ (Manual: add packages to queue)
                         ▼
┌──────────────────────────────────────────────────────────────────┐
│              Batch Generation Workflow (Automated)               │
│                   .github/workflows/batch-generate.yml           │
│                                                                  │
│  Trigger: Manual (workflow_dispatch) or Scheduled                │
│  Inputs: ecosystem, tier, batch_size                             │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Job 1: Generate                                          │   │
│  │ ├─ Check for open batch PRs (skip if exists)            │   │
│  │ ├─ Circuit breaker check (skip if open)                 │   │
│  │ ├─ Build tsuku binaries (Go)                            │   │
│  │ ├─ Run batch-generate (read queue, generate recipes)    │   │
│  │ ├─ Requeue unblocked packages (retry non-deterministic) │   │
│  │ └─ Upload recipes + binaries as artifacts               │   │
│  └─────────────────────────────────────────────────────────┘   │
│                         │                                        │
│                         ▼                                        │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Job 2-5: Validate (matrix, parallel)                    │   │
│  │ ├─ linux-x86_64 (Alpine musl, Ubuntu glibc)             │   │
│  │ ├─ linux-arm64 (Alpine musl, Ubuntu glibc)              │   │
│  │ ├─ darwin-x86_64 (macOS Intel)                          │   │
│  │ └─ darwin-arm64 (macOS Apple Silicon)                   │   │
│  │                                                          │   │
│  │ Each validation:                                         │   │
│  │ ├─ Download recipes + tsuku binary                      │   │
│  │ ├─ Run tsuku install --validate {recipe}                │   │
│  │ └─ Upload results (pass|fail|skipped per recipe)        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                         │                                        │
│                         ▼                                        │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Job 6: Merge                                             │   │
│  │ ├─ Download all validation results                      │   │
│  │ ├─ Aggregate (pass/fail/skipped per platform)           │   │
│  │ ├─ Derive platform constraints:                         │   │
│  │ │  • All pass → no constraints                          │   │
│  │ │  • Some fail → add supported_os/unsupported_platforms │   │
│  │ │  • All fail → exclude recipe                          │   │
│  │ ├─ Update circuit breaker (ecosystem-specific state)    │   │
│  │ ├─ Collect SLI metrics (batch-runs-{timestamp}.jsonl)   │   │
│  │ ├─ Persist state to main (retry with autostash)         │   │
│  │ ├─ If recipes changed:                                   │   │
│  │ │  ├─ Update queue statuses (success/failed)            │   │
│  │ │  ├─ Rebase with latest main (race condition fix)      │   │
│  │ │  ├─ Create PR branch                                  │   │
│  │ │  ├─ Commit recipes + constraints + queue              │   │
│  │ │  ├─ Create PR with batch:{ecosystem} label            │   │
│  │ │  └─ Enable auto-merge (squash)                        │   │
│  │ └─ If no recipes, only state committed to main          │   │
│  └─────────────────────────────────────────────────────────┘   │
└────────────────────────┬─────────────────────────────────────────┘
                         │ (if PR created)
                         ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Branch Protection + CI                        │
│                                                                  │
│  Required status checks:                                         │
│  ├─ lint (golangci-lint, go vet)                                │
│  ├─ test (Go unit tests)                                         │
│  └─ validate-recipes (schema validation)                         │
│                                                                  │
│  Strict mode: Branch must be up-to-date before merge            │
│                                                                  │
│  Auto-merge enabled by workflow:                                 │
│  └─ Merges automatically when all checks pass                    │
└────────────────────────┬─────────────────────────────────────────┘
                         │ (on merge to main)
                         ▼
┌──────────────────────────────────────────────────────────────────┐
│                Dashboard Update Workflow (Automated)             │
│                .github/workflows/update-dashboard.yml            │
│                                                                  │
│  Trigger: push to main (paths: recipes/**, data/dashboard.json) │
│                                                                  │
│  Steps:                                                          │
│  ├─ Generate GitHub App token (bypass branch protection)        │
│  ├─ Checkout main with app token                                │
│  ├─ Run scripts/update-dashboard.sh                             │
│  ├─ Commit changes to data/dashboard.json                       │
│  └─ Push to main (bypasses protection)                          │
└──────────────────────────────────────────────────────────────────┘
```

### Failure Flow

Recipes can fail validation on one or more platforms. The system categorizes failures and takes appropriate action:

```
Validation Job (per platform)
├─ Download recipes + tsuku binary
├─ Run: tsuku install --validate {recipe}
├─ Exit code 0 → pass
├─ Exit code 5 (network/timeout) → skipped, will requeue
└─ Exit code != 0,5 → fail

Merge Job - Aggregate Results
├─ For each recipe:
│  ├─ Count: pass, fail, skipped per platform
│  │
│  ├─ Decision tree:
│  │  ├─ All platforms pass|skip → include, no constraints
│  │  ├─ Some pass, some fail → include, add platform constraints
│  │  ├─ All fail (deterministic) → exclude, write failure log
│  │  └─ All fail|skip (transient) → requeue for next batch
│  │
│  ├─ Platform constraints:
│  │  • supported_os = ["linux", "darwin"]
│  │  • supported_libc = ["glibc"]
│  │  • unsupported_platforms = ["darwin-x86_64"]
│  │
│  └─ Write failure log:
│     data/failures/{ecosystem}-{timestamp}.jsonl
│     {recipe, platform, exit_code, category, timestamp}
│
├─ Update queue statuses:
│  ├─ Included recipes: pending → success
│  ├─ Excluded recipes: pending → failed
│  └─ Requeued: status unchanged, will retry
│
└─ Update circuit breaker:
   ├─ If any recipe succeeded → reset consecutive_failures
   └─ If zero recipes succeeded → increment consecutive_failures
```

**Failure categories:**
- `deterministic`: Recipe bug or incompatibility (exit codes: 1-4, 6+)
- `network`: Download failure, timeout (exit code: 5)
- `timeout`: Build exceeded time limit (exit code: 124, 137)

**Requeue logic:**
Non-deterministic failures (network, timeout) are automatically requeued if:
- Failure occurred <3 times for this package
- Not all platforms failed (at least one skipped)
- Exit code matches transient pattern (5, 124, 137)

### State Files

**Priority Queue** (`data/queues/priority-queue-{ecosystem}.json`):
```json
{
  "schema_version": 1,
  "updated_at": "2026-02-07T...",
  "tiers": {
    "1": "Critical - manually curated",
    "2": "Popular - >10K weekly downloads",
    "3": "Standard - all other packages"
  },
  "packages": [
    {
      "id": "homebrew:gh",
      "source": "homebrew",
      "name": "gh",
      "tier": 1,
      "status": "pending|success|failed",
      "added_at": "2026-02-07T..."
    }
  ]
}
```

**Circuit Breaker** (`batch-control.json`):
```json
{
  "schema_version": 1,
  "ecosystems": {
    "homebrew": {
      "last_batch_id": "2026-02-07-homebrew",
      "last_run": "2026-02-07T16:17:36Z",
      "consecutive_failures": 0,
      "circuit_state": "closed",
      "backoff_until": null
    }
  }
}
```

**Circuit Breaker Logic:**

Failure conditions (increment `consecutive_failures`):
1. Batch generation script exits with non-zero code
2. All generated recipes fail validation on all platforms
3. Zero recipes generated (empty batch)

Success condition (reset `consecutive_failures` to 0):
- Any batch run generates ≥1 recipe that passes validation

State transitions:
- `closed` → `open`: After 3 consecutive failures
- `open` → `closed`: After `backoff_until` timestamp passes and next run succeeds
- Backoff duration: Exponential (1h → 2h → 4h, max 24h)

**SLI Metrics** (`data/metrics/batch-runs-{timestamp}.jsonl`):
```json
{
  "batch_id": "2026-02-07-homebrew",
  "ecosystem": "homebrew",
  "total": 5,
  "generated": 5,
  "platforms": ["linux-x86_64", "darwin-arm64", ...],
  "merged": 3,
  "excluded": 2,
  "constrained": 1,
  "timestamp": "2026-02-07T16:17:36Z",
  "duration_seconds": 180
}
```

**Failure Logs** (`data/failures/{ecosystem}-{timestamp}.jsonl`):
```json
{
  "schema_version": 1,
  "recipe": "gh",
  "platform": "linux-debian-glibc-x86_64",
  "exit_code": 5,
  "category": "network|timeout|deterministic",
  "timestamp": "2026-02-07T..."
}
```

## Implementation Approach

### Maintainer Operations

#### Adding Packages to Queue

**Manual addition** (recommended for initial seeding):

```bash
# Add a single package
./scripts/add-to-queue.sh homebrew gh 1

# Add multiple packages
for pkg in ripgrep bat fd exa; do
  ./scripts/add-to-queue.sh homebrew "$pkg" 2
done
```

**Programmatic addition** (for bulk imports):

```bash
# Use jq to add packages
jq '.packages += [{
  "id": "homebrew:ripgrep",
  "source": "homebrew",
  "name": "ripgrep",
  "tier": 2,
  "status": "pending",
  "added_at": (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
}] | .updated_at = (now | strftime("%Y-%m-%dT%H:%M:%SZ"))' \
  data/queues/priority-queue-homebrew.json > tmp.json
mv tmp.json data/queues/priority-queue-homebrew.json

git add data/queues/priority-queue-homebrew.json
git commit -m "feat(batch): add ripgrep to homebrew queue"
git push
```

#### Triggering Batch Runs

**Manual trigger** (recommended for testing):

```bash
# Generate 5 tier-2 homebrew recipes
gh workflow run batch-generate.yml \
  -f ecosystem=homebrew \
  -f tier=2 \
  -f batch_size=5

# Monitor the run
gh run watch
```

**Scheduled trigger** (configured in workflow):

```yaml
on:
  schedule:
    - cron: '0 */6 * * *'  # Every 6 hours
  workflow_dispatch:       # Manual trigger
```

#### Monitoring Batch Runs

**Check workflow status**:

```bash
# List recent batch runs
gh run list --workflow=batch-generate.yml --limit 10

# View specific run
gh run view <run-id>

# Check for failures
gh run list --workflow=batch-generate.yml --status=failure --limit 5
```

**Check circuit breaker state**:

```bash
# View all ecosystem states
jq '.ecosystems' batch-control.json

# Check specific ecosystem
jq '.ecosystems.homebrew' batch-control.json
```

**Review SLI metrics**:

```bash
# View recent batch metrics
ls -lt data/metrics/batch-runs-*.jsonl | head -5

# Aggregate success rates
jq -s 'group_by(.ecosystem) | map({
  ecosystem: .[0].ecosystem,
  total_batches: length,
  avg_success_rate: (map(.merged / .total) | add / length)
})' data/metrics/batch-runs-*.jsonl
```

#### Troubleshooting

**Symptom: Batch workflow skipped (no PR created)**

Possible causes:
1. **Open batch PR already exists**
   ```bash
   gh pr list --label batch:homebrew
   ```
   Action: Wait for existing PR to merge, or close it if stale

2. **Circuit breaker is open**
   ```bash
   jq '.ecosystems.homebrew.circuit_state' batch-control.json
   ```
   Action: Check `backoff_until` timestamp, wait or manually reset:
   ```bash
   jq '.ecosystems.homebrew.circuit_state = "closed" |
       .ecosystems.homebrew.consecutive_failures = 0 |
       .ecosystems.homebrew.backoff_until = null' \
     batch-control.json > tmp.json
   mv tmp.json batch-control.json
   git add batch-control.json
   git commit -m "chore(batch): manually reset homebrew circuit breaker"
   git push
   ```

3. **No pending packages in queue**
   ```bash
   jq '.packages[] | select(.status == "pending")' \
     data/queues/priority-queue-homebrew.json
   ```
   Action: Add packages to queue (see "Adding Packages to Queue")

**Symptom: Recipes fail validation on all platforms**

Check failure logs:
```bash
# View recent failures for ecosystem
LATEST=$(ls -t data/failures/homebrew-*.jsonl | head -1)
jq -c 'select(.category == "deterministic")' "$LATEST"
```

Categories:
- `deterministic`: Recipe bug (fix recipe or exclude package)
- `network`: Transient download failure (will be requeued automatically)
- `timeout`: Build too slow (increase timeout or exclude)

Action for deterministic failures:
```bash
# Review failure details
jq 'select(.recipe == "problematic-tool")' "$LATEST"

# Option 1: Fix recipe and re-add to queue
# Option 2: Exclude package permanently
jq '.packages[] | select(.name == "problematic-tool").status = "failed"' \
  data/queues/priority-queue-homebrew.json > tmp.json
mv tmp.json data/queues/priority-queue-homebrew.json
```

**Symptom: PR created but not auto-merging**

Check PR status:
```bash
PR_NUM=$(gh pr list --label batch:homebrew --json number --jq '.[0].number')
gh pr view "$PR_NUM" --json statusCheckRollup
```

Common issues:
1. **CI checks failing**: Fix issues in subsequent commits to the PR branch
2. **Branch out of date**: Should auto-update, but can manually trigger:
   ```bash
   gh pr view "$PR_NUM" --json headRefName --jq '.headRefName' | \
     xargs -I {} git push origin main:{}
   ```
3. **Auto-merge not enabled**: Check workflow logs for "Enable auto-merge" step

**Symptom: State persistence failed with rebase error**

This should be rare (fixed with `--autostash` in retry logic). Check logs:
```bash
gh run view <run-id> --log | grep -A 20 "Persist circuit breaker"
```

If still occurring, check for uncommitted files:
```bash
# In workflow, this is now handled by --autostash
# If seeing this error, file an issue with workflow logs
```

**Symptom: Dashboard not updating after PR merge**

Check dashboard workflow:
```bash
gh run list --workflow=update-dashboard.yml --limit 5
```

Common issues:
1. **Workflow not triggered**: Verify PR changed files in `recipes/` or `data/`
2. **Branch protection blocking push**: Verify GitHub App token has admin permissions
3. **Script error**: Check workflow logs for `update-dashboard.sh` failures

#### Advanced Troubleshooting Scenarios

**Scenario: State file corruption (invalid JSON)**

Symptoms: Workflow fails to parse `batch-control.json` or queue files

Diagnosis:
```bash
# Validate all state files
for file in batch-control.json data/queues/priority-queue-*.json; do
  if ! jq empty "$file" 2>/dev/null; then
    echo "CORRUPT: $file"
  fi
done
```

Recovery:
```bash
# Find last known good commit
git log --all --full-history -- batch-control.json | head -1

# Review the diff
git show <commit>:batch-control.json | jq .

# Restore from last good state
git checkout <commit> -- batch-control.json
git add batch-control.json
git commit -m "fix: restore batch-control.json from <commit>"
git push
```

**Scenario: Queue state inconsistency**

Symptoms: Package marked `success` in queue but recipe doesn't exist

Diagnosis:
```bash
# Audit queue vs recipes
jq -r '.packages[] | select(.status == "success") | .name' \
  data/queues/priority-queue-homebrew.json | \
while read name; do
  if [ ! -f "recipes/${name:0:1}/${name}.toml" ]; then
    echo "INCONSISTENT: ${name} marked success but recipe missing"
  fi
done
```

Recovery:
```bash
# Option 1: Re-mark as pending to retry
jq '.packages[] | select(.name == "missing-recipe").status = "pending"' \
  data/queues/priority-queue-homebrew.json > tmp.json
mv tmp.json data/queues/priority-queue-homebrew.json

# Option 2: Mark as failed if recipe should not exist
jq '.packages[] | select(.name == "missing-recipe").status = "failed"' \
  data/queues/priority-queue-homebrew.json > tmp.json
mv tmp.json data/queues/priority-queue-homebrew.json
```

**Scenario: Cascading failures across ecosystems**

Symptoms: Multiple ecosystems fail simultaneously (infrastructure issue)

Diagnosis:
```bash
# Check if failures are correlated in time
jq -s 'group_by(.ecosystem) | map({
  ecosystem: .[0].ecosystem,
  recent_failures: map(.timestamp) | sort[-3:]
})' data/failures/*-*.jsonl

# Check GitHub Actions runner availability
gh api /repos/tsukumogami/tsuku/actions/runners

# Check API rate limits
gh api rate_limit
```

Recovery:
```bash
# Emergency pause: open circuit breakers for all ecosystems
jq '.ecosystems | with_entries(.value.circuit_state = "open" |
   .value.backoff_until = (now + 7200 | strftime("%Y-%m-%dT%H:%M:%SZ")))' \
  batch-control.json > tmp.json
mv tmp.json batch-control.json
git add batch-control.json
git commit -m "chore(batch): emergency circuit breaker activation due to infrastructure issue"
git push

# After infrastructure recovery, reset circuit breakers
jq '.ecosystems | with_entries(.value.circuit_state = "closed" |
   .value.consecutive_failures = 0 | .value.backoff_until = null)' \
  batch-control.json > tmp.json
mv tmp.json batch-control.json
```

**Scenario: Manual merge required (auto-merge stuck)**

Symptoms: PR has all checks passing but auto-merge not triggering

Procedure:
```bash
# Get PR details
PR_NUM=$(gh pr list --label batch:homebrew --json number --jq '.[0].number')

# Verify checks are actually green
gh pr checks "$PR_NUM"

# If checks are green but auto-merge stuck:
# 1. Verify branch is up-to-date
gh pr view "$PR_NUM" --json mergeable,mergeStateStatus

# 2. If mergeable=true, manually merge
gh pr merge "$PR_NUM" --squash --auto=false

# 3. Document reason in PR comment
gh pr comment "$PR_NUM" --body "Manually merged after auto-merge timeout. All checks passed."
```

### Security Considerations

**GitHub App Configuration**:
- **Permissions**: Contents (write), Pull Requests (write), Administration (write)
- **Secrets**: `TSUKU_BATCH_GENERATOR_APP_ID`, `TSUKU_BATCH_GENERATOR_APP_PRIVATE_KEY`
- **Usage**:
  - Batch workflow: Create PRs, enable auto-merge
  - Dashboard workflow: Bypass branch protection for automated updates
- **Branch protection**: Set "Do not allow bypassing" to OFF for the GitHub App

**Recipe Security Gates**:
- **run_command filter**: Recipes with `action = "run_command"` are automatically excluded
- **Platform validation**: All recipes must install successfully on at least one platform
- **Review**: Maintainers can review batch PRs before they auto-merge (disable auto-merge if needed)

**Secrets Exposure**:
- Failure logs never contain recipe content, only exit codes and categories
- SLI metrics track counts and durations, not package details
- Queue files are public (open-source project)

**Supply Chain Risks**:
- Recipes reference upstream URLs (Homebrew formulae, GitHub releases)
- Validation runs in isolated GitHub Actions runners
- Each recipe install is sandboxed via tsuku's standard installation

## Consequences

### Positive

- **Zero-touch operation**: Batch runs complete without maintainer intervention
- **Concurrent ecosystems**: Multiple package sources can run simultaneously without conflicts
- **Quality gates**: All recipes validated on target platforms before merge
- **Failure isolation**: Circuit breaker prevents cascading failures across ecosystems
- **Auditability**: SLI metrics and failure logs provide full visibility into batch operations
- **Scalability**: Per-ecosystem state design scales to N ecosystems without coordination overhead

### Negative

- **Complexity**: Multiple state files and workflows require understanding the full system
- **GitHub App dependency**: Requires GitHub App setup and secret management
- **Auto-merge risks**: Failed recipes could theoretically merge if validation has gaps
- **Storage growth**: Failure logs and SLI metrics accumulate over time (need cleanup strategy)

### Neutral

- **Manual queue management**: Packages must be added to queue manually (not auto-discovered)
- **Tier-based priority**: Requires classifying packages into tiers (subjective)
- **Platform constraints**: Some recipes may only work on subset of platforms (expected)

## Validation Results

End-to-end validation of the batch workflow system completed on 2026-02-08. Testing identified and fixed several bugs before declaring the system production-ready.

### Manual Batch E2E Test

**PR #1559**: First successful end-to-end batch generation

**Timeline:**
- 02:30:32Z: Batch triggered manually (batch_size=11, tier=1)
- 02:38:XX: Generation completed (4 recipes: procs, sd, tealdeer, xh)
- 02:40:35Z: PR auto-merged after all CI checks passed
- **Total time: 10 minutes from trigger to merge**

**What worked:**
1. ✅ Recipe generation from priority queue
2. ✅ Multi-platform validation (Linux x86/arm64, macOS x86/arm64)
3. ✅ PR creation with `batch:homebrew` label
4. ✅ Auto-merge enabled via GitHub App token
5. ✅ All CI checks passed (54 total checks)
6. ✅ PR merged without conflicts
7. ✅ Queue status updated correctly
8. ✅ Circuit breaker did not trigger (successes reset counter)

### Bugs Found and Fixed

#### 1. Seed Queue Path Mismatch (PR #1552)

**Problem**: `seed-queue.yml` workflow used wrong file paths
- Used: `data/priority-queue.json`
- Should be: `data/queues/priority-queue-{ecosystem}.json`

**Root Cause**: Workflow not updated after migration to per-ecosystem queues.

**Fix**: Updated all path references to per-ecosystem structure, added `--autostash` to retry logic.

#### 2. Seed Queue Branch Protection (PR #1553)

**Problem**: Workflow failed with `GH006: Protected branch update failed`

**Root Cause**: Standard GitHub Actions token cannot bypass branch protection rules. Seed queue pushes directly to main with `[skip ci]` but lacked proper authentication.

**Fix**: Added GitHub App token generation (same pattern as batch-generate and dashboard workflows).

#### 3. Batch Generate Rebase Without Autostash (PR #1555)

**Problem**: Merge job failed on "Create pull request" step with exit code 1.

**Root Cause**: Workflow updates queue file with success/failure statuses before rebasing with origin/main, creating unstaged changes. Without `--autostash`, git rebase fails with "You have unstaged changes."

**Fix**: Added `--autostash` flag to rebase command:
```yaml
git rebase --autostash origin/main
```

This allows rebase to proceed by stashing queue updates, rebasing, then restoring the stash.

#### 4. Dashboard Queue Loading

**Problem**: https://tsuku.dev/pipeline/ showed empty queue status after migration to per-ecosystem queues.

**Root Cause**: Dashboard tried to load single `data/priority-queue.json` file instead of aggregating from `data/queues/` directory.

**Fix**: Added `loadQueueFromPathOrDir` function to aggregate all `priority-queue-*.json` files. Function detects whether input is file or directory and handles both cases.

#### 5. Category Mismatch: recipe_not_found vs missing_dep (Issue #1557)

**Problem**: Packages with missing runtime dependencies counted as "failed" instead of "blocked", unnecessarily incrementing the circuit breaker failure counter.

**Root Cause**: CLI returns `"category": "recipe_not_found"` for missing dependencies, but orchestrator only treats `"category": "missing_dep"` as blocked in validation logic (`internal/batch/orchestrator.go:113-120`).

**Impact**: Packages that should be treated as "blocked on dependencies" trigger circuit breaker failures, potentially blocking all batch runs when the issue is just missing dependency recipes.

**Status**: Issue filed, needs code fix in orchestrator to treat both categories as blocked.

### Root Cause Analysis: Why Initial Batches Failed

Most packages in the priority queue have C library runtime dependencies that don't exist as recipes yet:

**Examples:**
- neovim: 8 dependencies (tree-sitter, gettext, libuv, lpeg, luajit, luv, unibilium, utf8proc)
- tmux: 3 dependencies (libevent, ncurses, utf8proc)
- vim: 6 dependencies (gettext, libsodium, lua, ncurses, python@3.14, ruby)
- wget: 4 dependencies (libidn2, openssl@3, gettext, libunistring)

**Solution**: Identified standalone Rust binaries with no dependencies (procs, sd, xh, tealdeer). Triggered batch with size=11 to reach these packages in queue. 7 failed (have dependencies), 4 succeeded (standalone).

**Category bug impact**: The 7 failed packages should have been marked "blocked" instead of "failed", which would have prevented circuit breaker failures. After fixing issue #1557, packages with missing dependencies will properly wait for those dependencies to be added rather than counting as failures.

### Automatic Batch Validation

**Status**: In progress (scheduled run at 03:00 UTC)

GitHub Actions scheduled workflows typically delay 10-26 minutes during peak load. Historical data shows:
- 02:00 UTC scheduled → started at 02:14:14Z (14 min delay)
- 23:00 UTC scheduled → started at 23:26:16Z (26 min delay)

Monitoring workflow runs to confirm scheduled trigger works correctly.

## Related Issues

- #1508: Batch PR coordination and state refactoring (this design)
- #1487: State refactoring implementation (merged)
- #1552: Fix seed queue paths and autostash (merged)
- #1553: Fix seed queue branch protection (merged)
- #1555: Fix batch generate rebase autostash (merged)
- #1557: Fix category mismatch for missing dependencies (open)
- #1559: First successful batch PR (merged - 4 recipes)
- PRs #1549, #1550: E2E validation runs
