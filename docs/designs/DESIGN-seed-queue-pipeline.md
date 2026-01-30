---
status: Proposed
problem: The batch generation pipeline has no data to process because data/priority-queue.json doesn't exist and no CI workflow runs the seed script.
decision: Create a GitHub Actions workflow that runs seed-queue.sh and commits the output, starting with workflow_dispatch and graduating to a cron schedule after reliability is proven.
rationale: The seed script already works for Homebrew. Wrapping it in a workflow and adding merge logic is the smallest step that produces a working automated pipeline. Multi-ecosystem support can follow the same pattern later.
---

# DESIGN: Seed Priority Queue Pipeline

## Status

Proposed

## Upstream Design Reference

This design implements part of [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md) (Phase 0 visibility infrastructure).

**Relevant sections:**
- Phase 0 deliverables: "Scripts to populate queue from Homebrew API (popularity data)"
- Decision 2: Popularity-based prioritization (Option 2A)
- M-BatchPipeline milestone, issue #1241

## Context and Problem Statement

The batch generation pipeline (#1189) reads from `data/priority-queue.json` to decide which packages to generate recipes for. Three things prevent this from working:

1. **The file doesn't exist.** `scripts/seed-queue.sh` can create it from Homebrew data, but nobody has run it and committed the output.

2. **No automation.** Running the script manually doesn't scale. Homebrew's popularity rankings shift as new tools gain traction and others fall off. The queue needs periodic refreshing.

3. **No merge logic.** The current script always overwrites the output file. If the batch pipeline has marked some packages as `in_progress` or `success`, re-running the seed script would reset them to `pending`.

### Why Now

M57 (Visibility Infrastructure Schemas) is complete. The schema, validation scripts, and Homebrew seed script all exist. The batch pipeline (#1189) is the next milestone and needs this data.

### Scope

**In scope:**
- `--merge` flag for seed-queue.sh (additive updates)
- GitHub Actions workflow with `workflow_dispatch` trigger
- Schema validation as a workflow step
- Graduation criteria for enabling a cron schedule
- Initial seed run to create `data/priority-queue.json`

**Out of scope:**
- Additional ecosystem sources (Cargo, npm, PyPI, etc.) -- these follow the same pattern and will be added later
- Backend storage migration (queue stays as a JSON file)
- Changes to the seed script's tier assignment logic

## Decision Drivers

- **Don't break existing status**: Re-running the seed must not reset packages the batch pipeline has already processed.
- **Schema compliance**: Output must validate against `data/schemas/priority-queue.schema.json` before committing.
- **Observable before automated**: The workflow should run manually first so operators can verify the output before trusting a schedule.
- **Minimal moving parts**: The workflow should need only `curl`, `jq`, and the existing seed script. No new tooling.
- **Idempotent**: Running the workflow twice with the same Homebrew data should produce the same output.

## Implementation Context

### Existing Patterns

**seed-queue.sh** (`scripts/seed-queue.sh`): Fetches Homebrew analytics (install-on-request, 30d), assigns tiers (1=curated list of ~35 tools, 2=>40K downloads/30d, 3=everything else), writes `data/priority-queue.json`. Uses `fetch_with_retry()` with exponential backoff. Accepts `--source homebrew` and `--limit N`.

**batch-operations.yml**: Shows the established CI pattern for automated data workflows. Uses pre-flight checks via `batch-control.json`, circuit breaker logic, retry-and-push for commits. The seed workflow can follow a simpler subset of this pattern.

**validate-queue.sh** (`scripts/validate-queue.sh`): Validates `data/priority-queue.json` against the JSON Schema. Already exists and can be called as a workflow step.

### What Needs to Happen for Full Automation

Getting from "script exists" to "queue stays fresh automatically" requires these pieces:

1. **Merge logic in the script** -- so re-running doesn't clobber existing package statuses
2. **Workflow with manual trigger** -- so operators can seed and verify
3. **Schema validation in the workflow** -- so bad data can't land
4. **Commit-and-push step** -- so the output persists without manual intervention
5. **Monitoring of workflow runs** -- so failures are noticed
6. **Cron schedule** -- so the queue refreshes without human action
7. **Graduation criteria** -- so the schedule isn't enabled before confidence is established

## Considered Options

### Option A: Add --merge to Existing Script + Workflow

Extend `seed-queue.sh` with a `--merge` flag that reads the existing queue file, adds new packages, and preserves statuses. Create a workflow that calls the script and commits the result.

**Pros:**
- Reuses all existing code (retry logic, tier assignment, output formatting)
- Single change to the script (merge logic)
- Workflow is straightforward: run script, validate, commit
- Idempotent by design (merge skips existing IDs)

**Cons:**
- Merge logic in bash/jq is somewhat complex
- The script grows by ~30-40 lines
- Direct commits to main bypass PR review

### Option B: Workflow Creates a PR Instead of Direct Commit

Same as Option A, but the workflow opens a PR for the queue update instead of pushing directly.

**Pros:**
- Human review of every queue update
- PR status checks run (including schema validation)
- Audit trail via PR history

**Cons:**
- Adds friction that prevents true automation
- PRs pile up if not merged promptly
- Blocks the cron graduation path (automated runs need automated commits)
- Overkill for a data file that's validated against a schema

### Option C: Use the Workflow to Orchestrate Without Modifying the Script

Keep `seed-queue.sh` as-is (overwrite mode). The workflow handles merge logic externally: back up the existing file, run the script, merge the backup with the new output using jq.

**Pros:**
- No changes to the existing, tested script
- Merge logic lives in the workflow (visible in YAML)

**Cons:**
- Merge logic in YAML workflow steps is harder to test
- Duplicates responsibilities between script and workflow
- Can't run merge locally without reproducing the workflow logic

### Evaluation Against Decision Drivers

| Driver | Option A | Option B | Option C |
|--------|----------|----------|----------|
| Don't break status | Script handles merge | Script handles merge | Workflow handles merge |
| Schema compliance | Validate after merge | PR checks validate | Validate after merge |
| Observable before automated | Manual trigger first | Always manual (PR) | Manual trigger first |
| Minimal moving parts | Script + workflow | Script + workflow + PR | Script + workflow + external merge |
| Idempotent | Yes (skip existing IDs) | Yes | Depends on merge impl |

### Uncertainties

- **Homebrew API stability**: The analytics endpoint format could change. The existing script handles this by failing on non-200 responses, which is sufficient.
- **Concurrent access**: If the batch pipeline modifies `priority-queue.json` while the seed workflow runs, they could conflict. This is unlikely with `workflow_dispatch` (manual) and low-risk with weekly cron since the batch pipeline runs at a different time.
- **API response size**: A malformed or unexpectedly large API response could produce an oversized queue file. The script should reject responses over a reasonable size limit.

## Decision Outcome

**Chosen option: A (Add --merge to existing script + workflow)**

The merge logic belongs in the script, not the workflow. This keeps the script self-contained and testable. Direct commits are acceptable for a schema-validated data file. PR-based updates (Option B) block the automation path.

### Rationale

- The batch pipeline needs fresh data without human intervention. Option B makes every update manual.
- Merge logic in the script (Option A) can be tested locally. Merge logic in the workflow (Option C) can't.
- The script already has the structure for flag handling (`--source`, `--limit`). Adding `--merge` is natural.

### Alternatives Rejected

- **Option B (PR-based)**: Prevents full automation. A data file validated against a schema doesn't need human review on every update.
- **Option C (External merge)**: Splits merge responsibility between script and workflow, making both harder to test.

### Trade-offs Accepted

- Direct commits to main for a data file. If this becomes a concern, we can switch to PR-based updates later.
- Concurrent access is handled by timing (manual trigger or cron at a different time than batch pipeline), not locking.

## Solution Architecture

### Overview

Two changes: (1) add `--merge` to `seed-queue.sh`, and (2) create `.github/workflows/seed-queue.yml`.

### Script Changes: --merge Flag

When `--merge` is passed, the script:

1. Reads `data/priority-queue.json` if it exists
2. Fetches new data from the Homebrew API (existing logic)
3. Merges: for each new package, adds it only if its `id` doesn't already exist in the queue
4. Preserves all fields of existing entries (status, tier, added_at, metadata)
5. Updates the top-level `updated_at` timestamp
6. Writes the merged result

Without `--merge`, the script behaves exactly as it does today (overwrite mode). This preserves backward compatibility.

**Merge implementation in jq:**

```
# Pseudocode for merge:
existing_ids = [existing.packages[].id]
new_packages = [generated packages where id NOT IN existing_ids]
merged = existing.packages + new_packages
output = {schema_version: 1, updated_at: now, tiers: {...}, packages: merged}
```

The merge is additive only. It doesn't update tiers for existing packages, because the batch pipeline or operators may have manually adjusted them.

### Error Handling

**Corrupt or missing queue file**: If `--merge` is set but the existing file is missing or fails JSON parsing, the script should treat it as an empty queue and proceed (effectively a fresh seed). This prevents a single corrupted commit from permanently breaking the workflow.

**API response size**: The script should reject Homebrew API responses larger than 10 MB. This prevents a malformed API response from producing an unreasonably large queue.

**Partial failures**: If the Homebrew API fails after retries, the script exits with code 2. The workflow step fails, and no commit is made. The existing queue file is unchanged.

### Workflow Design

`.github/workflows/seed-queue.yml`:

**Triggers:**
- `workflow_dispatch` with inputs for limit (number, default 100)
- `schedule` with cron (added later, after graduation criteria met)

**Steps:**
1. Check out the repo with write permissions
2. Run `seed-queue.sh --source homebrew --limit $LIMIT --merge`
3. Run `validate-queue.sh` to verify output against schema
4. If the file changed: commit and push with retry logic (to handle concurrent pushes)
5. If the file didn't change: exit cleanly

**Commit-and-push with retry** (pattern from `batch-operations.yml`):

```bash
git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git add data/priority-queue.json
git diff --cached --quiet && echo "No changes" && exit 0
git commit -m "chore(data): seed priority queue (homebrew)"
for i in 1 2 3; do
  git pull --rebase origin main && git push && break
  sleep $((i * 2))
done
```

### Graduation to Cron

The workflow starts with `workflow_dispatch` only. Adding a cron schedule requires meeting these criteria:

1. **3 successful manual runs**: workflow completes green, output file validates against schema, package count is within expected range (50-500 for Homebrew with default limit)
2. **Merge verified**: at least one run with `--merge` against an existing queue file, confirming existing statuses are preserved
3. **No data corruption**: committed queue file is valid JSON, passes schema validation, and has reasonable `updated_at` timestamps
4. **Batch pipeline tested** (#1189): the pipeline has successfully read and processed at least one package from the seeded queue

Once criteria are met, add the cron trigger:

```yaml
on:
  schedule:
    - cron: '0 1 * * 1'  # Weekly, Monday 1 AM UTC
  workflow_dispatch:
    # ... existing inputs ...
```

Weekly is sufficient because Homebrew popularity rankings don't shift dramatically day-to-day. The batch pipeline processes packages over days/weeks, not minutes.

### Monitoring

Since the workflow is simple (one script + validation + commit), monitoring is lightweight:

- **Workflow failure notifications**: GitHub Actions sends email on failure by default. No additional setup needed.
- **Schema validation failure**: The `validate-queue.sh` step fails the workflow if the output is invalid, preventing bad data from being committed.
- **Manual audit**: Operators can check `git log -- data/priority-queue.json` to see when the file was last updated and by whom.

When the cron schedule is active, the weekly cadence means a missed run is noticeable (file's `updated_at` will be >7 days old). The batch pipeline can check this and warn if the queue is stale.

### Data Flow

```
Homebrew Analytics API
        │
        ▼
  seed-queue.sh --merge
        │
  ┌─────┴─────┐
  │            │
  ▼            ▼
Read existing  Fetch new
queue (if any) packages
  │            │
  └────┬───────┘
       │
       ▼
  Merge (additive)
       │
       ▼
  Write merged output
       │
       ▼
  validate-queue.sh
       │
       ▼
  Commit + push
```

## Implementation Approach

### Step 1: Add --merge Flag

Modify `seed-queue.sh` to accept `--merge`. When set, read the existing queue file and combine it with newly fetched data. Existing entries (matched by `id`) are preserved unchanged. New entries are appended with `status: "pending"`.

Test locally: run with `--merge` on an empty directory (should behave like overwrite), then run again (should add no duplicates).

### Step 2: Create Workflow

Add `.github/workflows/seed-queue.yml` with `workflow_dispatch` trigger. Include the seed, validate, and commit steps.

### Step 3: Initial Seed Run

Run the workflow manually to create the first `data/priority-queue.json`. Verify the committed file is valid and contains the expected number of packages.

### Step 4: Validate with Batch Pipeline

Once the batch pipeline (#1189) is ready, verify it can read and process the seeded queue. This is a dependency check, not a deliverable of this design.

### Step 5: Graduate to Cron

After meeting the graduation criteria (3 successful runs, no corruption, batch pipeline tested), add the cron schedule to the workflow.

## Security Considerations

### Download Verification

**Not applicable.** The seed script fetches popularity metadata (package names, download counts) from the Homebrew analytics API. It doesn't download any binaries or executable artifacts. The output is a JSON file listing package names and priority levels.

### Execution Isolation

The workflow runs in GitHub Actions with standard repository permissions. It needs write access to push commits (via `GITHUB_TOKEN`). No elevated permissions beyond what `GITHUB_TOKEN` provides.

**Risk**: The workflow commits directly to the default branch.

**Mitigation**: The committed file is a JSON data file validated against a strict schema. It contains package names and metadata, not executable code. Schema validation runs before the commit step; if validation fails, nothing is committed.

### Supply Chain Risks

**Risk**: The Homebrew analytics API could be compromised to inject misleading package names (e.g., typosquatted names designed to trick the batch pipeline).

**Mitigation**: The seed script only writes package names and popularity scores to the queue. The batch pipeline validates every generated recipe through its own gates (schema validation, sandbox testing) before merging any recipe. A bad entry in the queue wastes CI time but can't introduce malicious code into the registry without passing the pipeline's validation.

**Risk**: HTTPS interception of the Homebrew API request.

**Mitigation**: All API calls use HTTPS. GitHub Actions runners connect through GitHub's network. The risk level is equivalent to any CI job fetching external data.

### User Data Exposure

**Not applicable.** The script reads from a public API (`formulae.brew.sh`) and writes to a file in the repository. No user data is accessed, collected, or transmitted. The workflow uses only the standard `GITHUB_TOKEN`, which is scoped to the repository.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Misleading package names | Batch pipeline validates all recipes independently | Wasted CI time on bad entries |
| API response interception | HTTPS for all requests | GitHub runner network compromise |
| Direct push without review | Schema validation before commit | Schema bypass via script bug |
| Stale data after API outage | Workflow fails and retries on next run | Queue may be outdated for a week |

## Consequences

### Positive

- `data/priority-queue.json` will exist with real Homebrew popularity data
- The batch pipeline (#1189) can start processing packages as soon as it's ready
- Queue stays fresh automatically once cron is enabled
- Adding more ecosystems later follows the same pattern (add a `--source` handler)

### Negative

- Direct commits to main bypass PR review (acceptable for schema-validated data files)
- Weekly cron means popularity data can be up to 7 days stale (fine for this use case)

### Neutral

- The `--merge` flag changes the script's interface but not its default behavior. Calling without `--merge` works exactly as before.
