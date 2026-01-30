---
status: Current
problem: >
  The batch generation pipeline reads from data/priority-queue.json to decide which packages
  to generate recipes for, but that file doesn't exist yet. The seed script can create it from
  Homebrew analytics data, but there's no CI workflow to run it and no merge logic to prevent
  re-seeding from clobbering packages the pipeline has already processed.

  Beyond the initial bootstrapping gap, the queue needs a clear lifecycle model. Items move
  through statuses (pending, in_progress, success, failed) as they're consumed by the batch
  pipeline. The seed tool must understand this lifecycle so it only adds genuinely new
  packages and leaves existing entries untouched regardless of their processing state.

  Without automation, the queue becomes stale as Homebrew popularity rankings shift over time.
  A tool that's the 50th most downloaded formula this month might be the 20th next month, and
  new tools appear regularly. Manual seeding doesn't scale.
decision: >
  Build a Go tool (cmd/seed-queue) that fetches Homebrew popularity data, merges it additively
  into data/priority-queue.json, and validates the output against the schema. A GitHub Actions
  workflow runs this tool, starting with workflow_dispatch and graduating to a weekly cron
  schedule after meeting reliability criteria.

  The merge is additive: the tool reads the existing queue, fetches fresh data, and adds only
  packages whose IDs aren't already present. Existing entries keep their status, tier, and
  metadata regardless of what the Homebrew API returns. This ensures the batch pipeline's
  state is never disrupted by a re-seed.

  The queue lifecycle is explicitly modeled with clear ownership boundaries. The seed tool
  only creates pending entries. The batch pipeline owns all status transitions. Operators can
  manually re-queue failed items after fixes. Items are never removed from the file -- success
  and skipped entries serve as deduplication records.
rationale: >
  Go is the right language for this tool. The merge logic involves structured JSON manipulation
  (reading the queue, deduplicating by ID while preserving status fields, writing valid output)
  that's awkward in bash/jq but natural with Go structs. The existing seed-queue.sh is already
  185 lines; adding merge logic, error handling, and response size limits would push it past
  the point where bash is comfortable. A Go implementation gets type safety, proper error
  handling, unit-testable merge logic, and no runtime dependency on jq.

  Direct commits to main are acceptable because the output is a JSON data file validated
  against a strict schema. PR-based updates would block the automation path since cron-triggered
  runs can't wait for human review. The graduation criteria (3 successful manual runs, merge
  verified, batch pipeline tested) provide confidence before enabling automation.

  Scoping to Homebrew only keeps the initial implementation focused. The same source-flag
  pattern supports additional ecosystems when their fetch functions are added later.
---

# DESIGN: Seed Priority Queue Pipeline

## Status

Current

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

The existing `seed-queue.sh` is already 185 lines of bash with complex jq pipelines for JSON transformation. Adding merge logic (read existing file, deduplicate by ID, preserve statuses) and error handling (corrupt files, oversized API responses) would push it well past the point where bash is maintainable. The merge logic in particular involves structured data manipulation that's natural in Go but awkward in bash/jq.

### Why Now

M57 (Visibility Infrastructure Schemas) is complete. The schema, validation scripts, and Homebrew seed script all exist. The batch pipeline (#1189) is the next milestone and needs this data.

### Scope

**In scope:**
- Queue lifecycle model: status transitions, ownership, deduplication rules
- Go tool (`cmd/seed-queue`) with merge logic and Homebrew source support
- GitHub Actions workflow with `workflow_dispatch` trigger
- Schema validation as a workflow step
- Graduation criteria for enabling a cron schedule
- Initial seed run to create `data/priority-queue.json`

**Out of scope:**
- Additional ecosystem sources (Cargo, npm, PyPI, etc.) -- added later as new source functions in the same Go tool
- Backend storage migration (queue stays as a JSON file)
- Version-aware re-queuing (the queue tracks package names, not versions)
- Replacing `seed-queue.sh` (it stays for reference; the Go tool supersedes it for CI use)

## Decision Drivers

- **Don't break existing status**: Re-running the seed must not reset packages the batch pipeline has already processed.
- **Schema compliance**: Output must validate against `data/schemas/priority-queue.schema.json` before committing.
- **Observable before automated**: The workflow should run manually first so operators can verify the output before trusting a schedule.
- **Testable merge logic**: The merge/deduplication behavior must be unit-testable, not buried in jq pipelines.
- **Idempotent**: Running the tool twice with the same Homebrew data should produce the same output.
- **Extensible to more ecosystems**: Adding Cargo, npm, PyPI sources later should be a matter of adding a Go function, not a new script.

## Implementation Context

### Existing Patterns

**seed-queue.sh** (`scripts/seed-queue.sh`): Fetches Homebrew analytics (install-on-request, 30d), assigns tiers (1=curated list of ~35 tools, 2=>40K downloads/30d, 3=everything else), writes `data/priority-queue.json`. Uses `fetch_with_retry()` with exponential backoff. Accepts `--source homebrew` and `--limit N`.

**batch-operations.yml**: Shows the established CI pattern for automated data workflows. Uses pre-flight checks via `batch-control.json`, circuit breaker logic, retry-and-push for commits. The seed workflow can follow a simpler subset of this pattern.

**validate-queue.sh** (`scripts/validate-queue.sh`): Validates `data/priority-queue.json` against the JSON Schema. Already exists and can be called as a workflow step.

**cmd/benchmark**: An existing standalone Go tool in the repo. Shows the pattern for operational Go tools that aren't part of the tsuku CLI.

### What Needs to Happen for Full Automation

Getting from "script exists" to "queue stays fresh automatically" requires these pieces:

1. **Seed tool with merge logic** -- so re-running doesn't clobber existing package statuses
2. **Workflow with manual trigger** -- so operators can seed and verify
3. **Schema validation in the workflow** -- so bad data can't land
4. **Commit-and-push step** -- so the output persists without manual intervention
5. **Monitoring of workflow runs** -- so failures are noticed
6. **Cron schedule** -- so the queue refreshes without human action
7. **Graduation criteria** -- so the schedule isn't enabled before confidence is established

## Considered Options

### Option A: Extend seed-queue.sh with --merge

Add a `--merge` flag to the existing bash script. The script reads the existing queue, fetches new data, deduplicates by ID, and writes merged output.

**Pros:**
- Reuses existing retry logic, tier assignment, output formatting
- Single change to an existing script
- Workflow calls one script

**Cons:**
- Merge logic in bash/jq is complex and hard to test (no unit tests for jq pipelines)
- Script is already 185 lines; adding merge, error handling, and size limits pushes it past maintainable bash
- No type safety for the queue schema -- malformed JSON silently produces wrong output
- Adding more ecosystems later means more jq pipelines in the same script
- Runtime dependency on `jq` (available on GitHub Actions but an extra requirement)

### Option B: Workflow creates a PR instead of direct commit

Same as Option A, but the workflow opens a PR for the queue update instead of pushing directly.

**Pros:**
- Human review of every queue update
- PR status checks run (including schema validation)
- Audit trail via PR history

**Cons:**
- Adds friction that prevents true automation
- PRs pile up if not merged promptly
- Blocks the cron graduation path (automated runs need automated commits)
- Overkill for a data file validated against a schema

### Option C: Workflow orchestrates without modifying the script

Keep `seed-queue.sh` as-is (overwrite mode). The workflow handles merge logic externally: back up the existing file, run the script, merge the backup with the new output using jq in a workflow step.

**Pros:**
- No changes to the existing, tested script
- Merge logic visible in YAML

**Cons:**
- Merge logic in YAML workflow steps is harder to test than in any programming language
- Duplicates responsibilities between script and workflow
- Can't run merge locally without reproducing the workflow logic

### Option D: Go tool (cmd/seed-queue)

Build a standalone Go tool that replaces the bash script for CI use. The tool handles fetching, tier assignment, merging, and validation. The existing `seed-queue.sh` stays for reference but isn't used by the workflow.

**Pros:**
- Type-safe queue schema (Go structs match JSON schema exactly)
- Unit-testable merge logic, error handling, and tier assignment
- Proper error handling with structured errors instead of bash exit codes
- No runtime dependency on `jq`
- Natural extensibility: adding a new ecosystem means adding a Go function that implements a `Source` interface
- HTTP client with built-in retry, timeouts, and response size limits
- Consistent with the project's Go codebase

**Cons:**
- More upfront work than extending the bash script
- Introduces a new binary to build in CI (but `go build` is already part of CI)
- Existing seed-queue.sh logic needs porting (fetch, tier assignment, curated list)

### Evaluation Against Decision Drivers

| Driver | Option A (bash) | Option B (PR) | Option C (workflow) | Option D (Go) |
|--------|----------------|---------------|--------------------|----|
| Don't break status | jq merge logic | jq merge logic | jq merge in YAML | Go merge logic |
| Schema compliance | Validate after | PR checks | Validate after | Validate in-tool + after |
| Observable before automated | Manual trigger | Always manual | Manual trigger | Manual trigger |
| Testable merge logic | No unit tests | No unit tests | No unit tests | Unit tests |
| Idempotent | Depends on jq impl | Depends on jq impl | Depends on jq impl | Deterministic Go code |
| Extensible | More jq pipelines | More jq pipelines | More jq pipelines | Add Go function |

### Uncertainties

- **Homebrew API stability**: The analytics endpoint format could change. The tool should fail cleanly on unexpected responses rather than producing corrupt output.
- **Concurrent access**: If the batch pipeline modifies `priority-queue.json` while the seed workflow runs, they could conflict. This is unlikely with `workflow_dispatch` (manual) and low-risk with weekly cron since the batch pipeline runs at a different time.
- **API response size**: A malformed or unexpectedly large API response could cause problems. The Go HTTP client should enforce a response size limit.

## Decision Outcome

**Chosen option: D (Go tool)**

The merge logic is the core complexity of this design, and it deserves real tests. A Go tool gives type safety for the queue schema, unit-testable merge and deduplication, proper error handling, and a natural path to adding more ecosystems. The bash script's jq pipelines are already complex enough; adding merge logic would make them harder to maintain and impossible to unit test.

### Rationale

- The merge logic (read JSON, deduplicate by ID while preserving status fields, write valid output) is structured data manipulation. Go handles this naturally with typed structs. Bash/jq handles it with fragile pipelines that can silently produce wrong output.
- The project is a Go monorepo. A Go tool fits the codebase, builds with the same toolchain, and can reuse patterns from `cmd/benchmark`.
- Unit tests for merge behavior are critical. "Does merging preserve `in_progress` status?" is a question that should have a test, not rely on manual verification of jq output.
- Adding Cargo/npm/PyPI sources later means implementing a Go function per ecosystem, not adding more jq pipelines to an already-long bash script.

### Alternatives Rejected

- **Option A (bash --merge)**: Merge logic in jq is untestable. The script is already at the limit of comfortable bash complexity.
- **Option B (PR-based)**: Prevents full automation. A data file validated against a schema doesn't need human review on every update.
- **Option C (External merge)**: Splits merge responsibility into YAML, making it untestable and unrunnable locally.

### Trade-offs Accepted

- More upfront work than extending bash. But the merge logic needs to be correct (it protects batch pipeline state), and correctness comes from tests.
- The existing `seed-queue.sh` becomes a reference artifact rather than the production tool. It can be removed later if desired.
- Direct commits to main for a data file. If this becomes a concern, we can switch to PR-based updates later.

## Solution Architecture

### Overview

Two deliverables: (1) `cmd/seed-queue`, a Go tool that fetches, merges, and validates the priority queue, and (2) `.github/workflows/seed-queue.yml`, a workflow that runs the tool and commits the result. But these only make sense with a clear model of how the queue works as a queue -- who produces items, who consumes them, and what happens to items after they're processed.

### Queue Lifecycle

The queue file serves two roles: it's the input for the batch pipeline (what to process next) and the record of what's been processed (what already succeeded or failed). These roles are in tension -- a pure queue would remove consumed items, but we need history to avoid re-processing.

**Status transitions:**

```
                seed-queue tool
                     │
                     ▼
              ┌─── pending ◄──── (re-seed on failed after fix)
              │      │
              │      │  batch pipeline picks up
              │      ▼
              │  in_progress
              │      │
              ├──────┤
              │      │
              ▼      ▼
           failed  success
              │
              │  (manual fix or dep added)
              ▼
           pending  (re-queued)
```

- **pending**: Ready for the batch pipeline to pick up. The seed tool creates all new entries in this state.
- **in_progress**: The batch pipeline has claimed this item. Only the pipeline writes this status.
- **success**: Recipe was generated and merged. The item stays in the queue as a record, preventing the seed tool from re-adding it.
- **failed**: Generation or validation failed. Stays in the queue with failure details in the failure record files (`data/failures/`). Can be moved back to `pending` when the blocking issue is resolved.
- **skipped**: Deliberately excluded (e.g., not a CLI tool, duplicate of another source). Stays in the queue to prevent re-adding.

**Who writes what:**

| Actor | Creates entries? | Changes status? | To which statuses? |
|-------|-----------------|-----------------|-------------------|
| seed-queue tool | Yes (pending) | No | -- |
| batch pipeline | No | Yes | pending → in_progress → success/failed |
| operator (manual) | No | Yes | failed → pending (re-queue), any → skipped |

The seed tool never changes the status of an existing entry. It only adds new entries as `pending`. This separation is critical: the seed tool doesn't need to know about the batch pipeline's state machine.

**What "consumed" means:**

Items are never removed from the file. An item with `status: success` is "consumed" -- it stays as a record that prevents re-seeding and provides historical context. This is a deliberate choice: the file is small (hundreds to low thousands of entries), and keeping history avoids the need for a separate "processed items" store.

**How the seed tool avoids duplicates:**

The tool checks the `id` field of every existing entry regardless of status. If `homebrew:ripgrep` exists with any status (`pending`, `success`, `failed`, etc.), the tool doesn't add it again. This means:

- A `success` item won't be re-queued just because it's still popular on Homebrew
- A `failed` item won't be re-queued automatically by the seed tool (it needs manual intervention or a separate re-queue mechanism)
- A `skipped` item stays skipped

**When items should be re-queued:**

The seed tool doesn't handle re-queuing. Re-queuing happens through separate mechanisms:

1. **Failed items after a fix**: When a blocking dependency is added or a builder bug is fixed, an operator (or the gap analysis script) changes the status from `failed` back to `pending`. The batch pipeline picks it up on its next run.

2. **Upstream version changes**: This design does NOT address version-aware re-queuing. The queue tracks package names, not versions. Version-aware refresh is a separate concern for when the batch pipeline matures. For now, if a recipe needs updating because the upstream tool released a new version, that's handled by the existing `tsuku update` workflow, not the seed queue.

3. **New packages from the source**: The seed tool handles this naturally. When the Homebrew API returns a package that isn't in the queue, it gets added as `pending`. This is the primary growth mechanism.

**Queue growth and pruning:**

The queue grows monotonically (items are added, never removed). At Homebrew scale with a limit of 100, the file stays under 200 entries. Even at 500 entries across multiple ecosystems, the file is ~50 KB of JSON. Pruning isn't needed for the foreseeable future.

If the file does grow large enough to be a concern, pruning `success` entries older than N months is safe -- the batch pipeline already has the merged recipes. But this is future work, not part of this design.

### Go Tool Design

`cmd/seed-queue/main.go`:

```
seed-queue --source homebrew [--limit N] [--output PATH] [--merge]
```

**Flags:**
- `--source`: Ecosystem to fetch from (initially only `homebrew`)
- `--limit`: Maximum packages to fetch (default 100)
- `--output`: Path to queue file (default `data/priority-queue.json`)
- `--merge`: Enable additive merge with existing queue file

**Package structure:**

```
cmd/seed-queue/
  main.go              # CLI entry point, flag parsing
internal/seed/
  queue.go             # PriorityQueue type, Load/Save/Merge methods
  queue_test.go        # Unit tests for merge, dedup, status preservation
  homebrew.go          # Homebrew source: fetch analytics, assign tiers
  homebrew_test.go     # Tests with fixture data (no network)
  source.go            # Source interface for future ecosystems
```

**Key types:**

```go
// Package represents a queue entry.
type Package struct {
    ID       string            `json:"id"`
    Source   string            `json:"source"`
    Name     string            `json:"name"`
    Tier     int               `json:"tier"`
    Status   string            `json:"status"`
    AddedAt  string            `json:"added_at"`
    Metadata map[string]any    `json:"metadata,omitempty"`
}

// PriorityQueue is the top-level queue file.
type PriorityQueue struct {
    SchemaVersion int                `json:"schema_version"`
    UpdatedAt     string             `json:"updated_at"`
    Tiers         map[string]string  `json:"tiers,omitempty"`
    Packages      []Package          `json:"packages"`
}

// Source fetches packages from an ecosystem API.
type Source interface {
    Fetch(limit int) ([]Package, error)
}
```

**Merge logic (the critical path):**

```go
func (q *PriorityQueue) Merge(newPackages []Package) int {
    existing := make(map[string]bool, len(q.Packages))
    for _, p := range q.Packages {
        existing[p.ID] = true
    }
    added := 0
    for _, p := range newPackages {
        if !existing[p.ID] {
            q.Packages = append(q.Packages, p)
            existing[p.ID] = true
            added++
        }
    }
    return added
}
```

This is the code that needs tests. Key test cases:

- Merge into empty queue: all packages added
- Merge with existing `success` entry: entry preserved, not duplicated
- Merge with existing `in_progress` entry: entry preserved, not duplicated
- Merge with existing `failed` entry: entry preserved, not re-queued
- Merge with same data twice: no duplicates, idempotent
- Merge preserves all fields (status, tier, added_at, metadata)

### Error Handling

**Corrupt or missing queue file**: If `--merge` is set but the existing file is missing, start with an empty queue. If the file exists but fails JSON parsing, exit with an error. Unlike the bash approach, we don't silently ignore corrupt files -- that could mask a real problem.

**API response size**: The Go HTTP client enforces a 10 MB response body limit using `io.LimitReader`. This prevents a malformed API response from consuming unbounded memory.

**API failures**: The tool retries with exponential backoff (3 attempts, starting at 1s). On permanent failure, it exits with a non-zero code. The workflow step fails and no commit is made.

**Schema validation**: The tool validates its own output against the schema before writing. If validation fails (which would indicate a bug in the tool), it exits with an error rather than writing invalid data.

### Workflow Design

`.github/workflows/seed-queue.yml`:

**Triggers:**
- `workflow_dispatch` with inputs for limit (number, default 100)
- `schedule` with cron (added later, after graduation criteria met)

**Steps:**
1. Check out the repo with write permissions
2. Build the seed-queue tool: `go build -o seed-queue ./cmd/seed-queue`
3. Run: `./seed-queue --source homebrew --limit $LIMIT --merge`
4. Run `validate-queue.sh` to verify output (belt-and-suspenders with the in-tool validation)
5. If the file changed: commit and push with retry logic
6. If the file didn't change: exit cleanly

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

Since the workflow is simple (build tool, run, validate, commit), monitoring is lightweight:

- **Workflow failure notifications**: GitHub Actions sends email on failure by default. No additional setup needed.
- **Schema validation failure**: Both the tool's internal validation and `validate-queue.sh` catch invalid output.
- **Manual audit**: Operators can check `git log -- data/priority-queue.json` to see when the file was last updated and by whom.

When the cron schedule is active, the weekly cadence means a missed run is noticeable (file's `updated_at` will be >7 days old). The batch pipeline can check this and warn if the queue is stale.

### Data Flow

```
Homebrew Analytics API
        │
        ▼
  cmd/seed-queue (Go)
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
  Validate output
       │
       ▼
  Write to file
       │
       ▼
  validate-queue.sh (belt-and-suspenders)
       │
       ▼
  Commit + push
```

## Implementation Approach

### Step 1: Queue Package and Merge Logic

Create `internal/seed/queue.go` with `PriorityQueue` and `Package` types. Implement `Load`, `Save`, and `Merge` methods. Write thorough unit tests for merge behavior -- this is the critical path that protects batch pipeline state.

### Step 2: Homebrew Source

Create `internal/seed/homebrew.go` implementing the `Source` interface. Port the fetch logic and tier assignment from `seed-queue.sh` (analytics API, curated tier 1 list, download-count tier 2 threshold). Write tests using fixture data so tests don't hit the network.

### Step 3: CLI Entry Point

Create `cmd/seed-queue/main.go` with flag parsing and the fetch-merge-validate-write pipeline.

### Step 4: Create Workflow

Add `.github/workflows/seed-queue.yml` with `workflow_dispatch` trigger. Include the build, seed, validate, and commit steps.

### Step 5: Initial Seed Run

Run the workflow manually to create the first `data/priority-queue.json`. Verify the committed file is valid and contains the expected number of packages.

### Step 6: Graduate to Cron

After meeting the graduation criteria (3 successful runs, merge verified, batch pipeline tested), add the cron schedule to the workflow.

## Security Considerations

### Download Verification

**Not applicable.** The seed tool fetches popularity metadata (package names, download counts) from the Homebrew analytics API. It doesn't download any binaries or executable artifacts. The output is a JSON file listing package names and priority levels.

### Execution Isolation

The workflow runs in GitHub Actions with standard repository permissions. It needs write access to push commits (via `GITHUB_TOKEN`). No elevated permissions beyond what `GITHUB_TOKEN` provides.

**Risk**: The workflow commits directly to the default branch.

**Mitigation**: The committed file is a JSON data file validated against a strict schema -- both by the tool itself and by `validate-queue.sh`. It contains package names and metadata, not executable code. If either validation fails, nothing is committed.

### Supply Chain Risks

**Risk**: The Homebrew analytics API could be compromised to inject misleading package names (e.g., typosquatted names designed to trick the batch pipeline).

**Mitigation**: The seed tool only writes package names and popularity scores to the queue. The batch pipeline validates every generated recipe through its own gates (schema validation, sandbox testing) before merging any recipe. A bad entry in the queue wastes CI time but can't introduce malicious code into the registry without passing the pipeline's validation.

**Risk**: HTTPS interception of the Homebrew API request.

**Mitigation**: All API calls use HTTPS. The Go HTTP client verifies TLS certificates by default. GitHub Actions runners connect through GitHub's network. The risk level is equivalent to any CI job fetching external data.

### User Data Exposure

**Not applicable.** The tool reads from a public API (`formulae.brew.sh`) and writes to a file in the repository. No user data is accessed, collected, or transmitted. The workflow uses only the standard `GITHUB_TOKEN`, which is scoped to the repository.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Misleading package names | Batch pipeline validates all recipes independently | Wasted CI time on bad entries |
| API response interception | HTTPS + Go TLS verification | GitHub runner network compromise |
| Direct push without review | Dual validation (tool + script) before commit | Both validators have same bug |
| Oversized API response | 10 MB response body limit in HTTP client | Limit may need adjustment |
| Stale data after API outage | Tool fails cleanly, retries on next run | Queue may be outdated for a week |

## Consequences

### Positive

- `data/priority-queue.json` will exist with real Homebrew popularity data
- The batch pipeline (#1189) can start processing packages as soon as it's ready
- Queue stays fresh automatically once cron is enabled
- Merge logic is unit-tested, protecting batch pipeline state from corruption
- Adding more ecosystems means implementing a Go `Source` interface, not writing more jq

### Negative

- More upfront work than extending the bash script
- `seed-queue.sh` becomes a reference artifact (not actively used by CI)
- Direct commits to main bypass PR review (acceptable for schema-validated data files)
- Weekly cron means popularity data can be up to 7 days stale (fine for this use case)

### Neutral

- The Go tool replaces the bash script for CI use. The bash script can still be run manually for quick one-off seeding if needed, but the workflow uses the Go tool.
