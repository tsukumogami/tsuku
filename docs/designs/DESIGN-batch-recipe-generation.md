---
status: Proposed
problem: No CI pipeline exists to orchestrate batch recipe generation across deterministic ecosystem builders, validate recipes across target environments, record failures with structured metadata, and merge passing recipes at scale.
decision: A manually-triggered GitHub Actions workflow that reads from the priority queue, runs per-ecosystem generation jobs with progressive validation (Linux first, macOS on pass), records failures to JSONL artifacts, and creates one PR per batch with auto-merge gated on validation and run_command absence.
rationale: Manual trigger gives operators control over batch timing and size without requiring external orchestration. Per-ecosystem jobs isolate failures and enable ecosystem-specific rate limiting. Progressive validation reduces macOS CI cost by 80%+ since most failures surface on Linux. JSONL artifacts in the repo keep failure data versioned and auditable without requiring backend infrastructure in this phase.
---

# DESIGN: Batch Recipe Generation CI Pipeline

## Status

Proposed

## Upstream Design Reference

This design implements part of [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md).

**Relevant sections:**
- Decision 1: Fully deterministic batch generation
- Prioritization Strategy (2A): Popularity-based queue
- Target Environment Validation Matrix
- Rate Limiting (RATE-1) and Circuit Breaker requirements
- Batch Operations Control Plane

**This design must deliver:**
- CI workflow that generates failure records (required by #1190)

**Dependent designs:**
- [DESIGN-batch-operations.md](current/DESIGN-batch-operations.md): Control plane, rollback, emergency stop
- [DESIGN-homebrew-deterministic-mode.md](current/DESIGN-homebrew-deterministic-mode.md): Homebrew DeterministicOnly session option
- [DESIGN-seed-queue-pipeline.md](current/DESIGN-seed-queue-pipeline.md): Queue population and lifecycle model

## Context and Problem Statement

The registry scale strategy calls for automated generation of recipes from 8 deterministic ecosystem builders (Cargo, NPM, PyPI, RubyGems, Go, CPAN, Homebrew Cask, and Homebrew). The priority queue contains packages ranked by popularity, and the failure record schema defines how to capture generation failures. The batch operations control plane provides emergency stop, circuit breaker state, and budget tracking.

What's missing is the CI pipeline that ties these together: reading from the queue, invoking builders, validating across environments, recording failures, and merging passing recipes. The target is 200+ recipes/week across all ecosystems, limited primarily by macOS CI budget and per-ecosystem rate limits.

Three challenges make this non-trivial:

1. **Validation cost asymmetry.** macOS runners cost 10x Linux runners. Validating every recipe on all 5 target environments is expensive. A 100-recipe batch validated on all platforms uses ~1100 CI minutes; the weekly macOS budget is 1000 minutes.

2. **Ecosystem isolation.** Different ecosystems have different failure modes, rate limits, and success rates. A Homebrew API outage shouldn't pause Cargo generation. The circuit breaker must operate per-ecosystem.

3. **Merge safety.** Recipes with `run_command` actions execute arbitrary shell at install time. Auto-merging these without human review is a security risk. The pipeline must distinguish safe-to-merge recipes from those requiring review.

### Scope

**In scope:**
- GitHub Actions workflow for batch recipe generation
- Per-ecosystem generation jobs with rate limiting
- Progressive validation (Linux first, macOS on pass)
- Failure recording to JSONL matching failure-record.schema.json
- Auto-merge with security gates (no `run_command`)
- Circuit breaker integration via batch-control.json
- SLI metrics collection (success rate, validation pass rate)
- Batch ID tracking for rollback support

**Constraints:**
- Single concurrent run enforced via GitHub Actions `concurrency` group. Overlapping dispatches are queued, not parallel. This prevents `batch-control.json` write races and JSONL merge conflicts.

**Prerequisites:**
- `tsuku create` CLI command with `--deterministic` flag (invokes builder with `DeterministicOnly: true`). If this CLI surface doesn't exist, it must be added as a prerequisite issue.
- Sandbox validation via `tsuku install --sandbox` (existing infrastructure).
- Priority queue populated at `data/priority-queue.json` via the seed-queue workflow (#1241, completed). The `cmd/seed-queue` Go tool fetches from ecosystem APIs and merges additively into the queue file. See [DESIGN-seed-queue-pipeline.md](current/DESIGN-seed-queue-pipeline.md).

**Out of scope:**
- Failure analysis backend (downstream #1190)
- Builder improvements (separate issues per ecosystem)
- D1/R2 backend integration (Phase 2 of scale strategy)
- Re-queue mechanism (requires failure analysis backend)
- Queue seeding (handled by #1241, completed)

## Decision Drivers

- macOS CI budget: 1000 minutes/week (10x cost of Linux)
- Must produce structured failure records for downstream gap analysis
- Circuit breaker must operate per-ecosystem, not globally
- Recipes with `run_command` must not auto-merge
- Batch ID metadata required in commits for surgical rollback
- Rate limits vary by ecosystem (GitHub API: 5000/hr authenticated)
- Pipeline must work without LLM API keys ($0 per recipe)
- Partial platform coverage is acceptable for merge (>=1 environment)

## Considered Options

### Decision 1: Workflow Trigger Model

How does the batch pipeline get triggered?

#### Option 1A: Manual Dispatch with Parameters

`workflow_dispatch` with inputs for ecosystem, batch size, and queue tier. Operators trigger runs on demand.

**Pros:**
- Full operator control over timing, size, and scope
- No surprise costs from automated runs
- Easy to pause: just don't trigger
- Can target specific ecosystems or tiers

**Cons:**
- Requires operator attention to run batches
- No automatic cadence without a separate scheduler
- Could fall behind if operators forget

#### Option 1B: Scheduled with Override

`schedule` cron (daily 3 AM UTC) plus `workflow_dispatch` for ad-hoc runs. Batch size and ecosystem configured in batch-control.json.

**Pros:**
- Automatic cadence ensures steady progress
- Still supports manual override
- Configuration in version-controlled file

**Cons:**
- Scheduled runs can surprise with costs during inactive periods
- Harder to scale up temporarily (need to change config file)
- Schedule changes require commits

#### Option 1C: Event-Driven (Queue Watcher)

External service (Cloudflare Worker) monitors queue and triggers workflow via GitHub API when packages are pending.

**Pros:**
- Reactive: processes packages as they enter the queue
- Optimal throughput

**Cons:**
- Requires external infrastructure (Worker + API token management)
- Higher operational complexity (debugging distributed triggers across Worker and GitHub Actions)
- Harder to audit trigger history

### Decision 2: Validation Strategy

How do we validate recipes across the target environment matrix?

#### Option 2A: Progressive Validation (Linux First)

Validate on Linux first. Only promote to macOS validation if Linux passes. Record per-environment results.

**Pros:**
- Saves ~80% macOS minutes (most failures surface on Linux)
- Fast feedback: Linux runners start in seconds vs minutes for macOS
- Matches cost reality (macOS is 10x)

**Cons:**
- macOS-only failures discovered later
- Linux pass doesn't guarantee macOS pass
- Slightly more complex workflow logic

#### Option 2B: Full Matrix

Validate every recipe on all 5 environments in parallel.

**Pros:**
- Complete coverage from the start
- Simpler workflow: one matrix job

**Cons:**
- Consumes macOS budget 5x faster than progressive
- Most failed recipes waste macOS minutes on a recipe that would have failed on Linux
- Budget exhaustion leads to pipeline stalls

#### Option 2C: Linux-Only with Periodic macOS Sweeps

Validate only on Linux in normal batches. Run weekly macOS validation sweep on all merged recipes.

**Pros:**
- Near-zero macOS cost for normal operations
- Periodic sweep catches macOS-specific issues

**Cons:**
- macOS failures discovered days late
- Recipes merged without macOS validation may break users
- Sweep creates large batch of fixes

### Decision 3: Merge Strategy

How do validated recipes get merged?

#### Option 3A: One PR Per Batch, Auto-Merge with Gates

Create a single PR per batch run containing all passing recipes. Auto-merge if: all recipes pass validation, none contain `run_command`, and CI checks pass.

**Pros:**
- Single PR for review if auto-merge gates fail
- Batch ID in commit message enables rollback
- Atomic: all recipes from a batch are one commit

**Cons:**
- One failing recipe blocks the entire batch
- Large PRs are harder to review when gates fail

#### Option 3B: One PR Per Recipe

Each validated recipe gets its own PR with auto-merge. This is the pattern Dependabot and similar automation tools use at scale.

**Pros:**
- Failing recipes don't block others
- Easy to review individual recipes
- Proven pattern (Dependabot, Renovate)
- GitHub's auto-merge handles high PR volume

**Cons:**
- High PR volume increases notification noise for maintainers
- Each PR triggers separate CI runs, multiplying CI cost
- Batch provenance requires external tracking (no single commit for rollback)
- GitHub Actions concurrency limits may queue PRs

#### Option 3C: Batch PR with Selective Exclusion

One PR per batch, but automatically exclude recipes that fail validation. Only passing recipes go in the PR.

**Pros:**
- Batch stays atomic (one commit per run)
- Failures don't block passing recipes
- Batch ID covers exactly what merged

**Cons:**
- Non-trivial PR assembly: collecting partial results across ecosystem jobs, tracking exclusions, and maintaining batch ID coherence requires careful workflow orchestration
- Excluded recipes need separate tracking

### Decision 4: Failure Recording

How are structured failure records stored?

#### Option 4A: JSONL Files in Repository

Append failures to `data/failures/<ecosystem>.jsonl` per run. Version-controlled, auditable.

**Pros:**
- No external infrastructure needed
- Full git history of failures
- Easy to query with jq
- Matches existing schema files in `data/schemas/`

**Cons:**
- Files grow over time (need periodic cleanup)
- Merge conflicts if multiple runs overlap
- Git repo size increases

#### Option 4B: GitHub Actions Artifacts

Upload failure records as workflow artifacts. Query via GitHub API.

**Pros:**
- No repo pollution
- Automatic retention policies (90 days)

**Cons:**
- Artifacts disappear after retention period
- Harder to query historically
- Failure analysis backend (#1190) needs API access

#### Option 4C: Direct D1 Database Write

Write failures to Cloudflare D1 via Worker API during batch run.

**Pros:**
- Queryable immediately by failure analysis backend
- No repo size impact

**Cons:**
- Requires Worker infrastructure (Phase 2 dependency)
- Network dependency during CI runs
- Couples CI to external service

### Uncertainties

- Actual macOS runner availability and queue times at scale are untested. Wait times could exceed estimates.
- Per-ecosystem rate limits for some registries (CPAN, Go proxy) aren't well documented. Conservative defaults may throttle unnecessarily.
- The 85-90% Homebrew deterministic success rate is estimated. First batch run will validate this.

## Decision Outcome

**Chosen: 1A + 2A + 3C + 4A**

### Summary

A manually-triggered GitHub Actions workflow reads from the priority queue and runs per-ecosystem generation jobs. Recipes are validated progressively (Linux first, macOS on pass). Failures are recorded to JSONL files in the repository. One PR per batch is created containing only passing recipes, with auto-merge gated on `run_command` absence and CI checks.

### Rationale

Option 1A (manual dispatch) gives operators explicit control over timing and cost without requiring external infrastructure. The existing `batch-control.json` pattern handles configuration. A scheduler can be added later as a thin cron job.

Option 2A (progressive validation) is the only strategy compatible with the macOS budget. At 10x cost, full-matrix validation of 100 recipes would consume the entire weekly macOS budget in one run. Progressive saves ~80% by catching most failures on Linux first.

Option 3C (selective exclusion) balances atomicity with throughput. One recipe failure shouldn't block 99 passing recipes in the same batch. The batch ID in the commit covers exactly what merged.

Option 4A (JSONL in repo) matches the existing `data/schemas/` pattern and avoids infrastructure dependencies. The failure analysis backend (#1190) can read these files directly. D1 integration (4C) is Phase 2 work.

### Trade-offs Accepted

- Manual triggering means no automatic cadence. This is acceptable because operators should control spend until the pipeline proves reliable. A cron trigger can be added once success rates are validated.
- JSONL files grow the repo. Acceptable at expected scale (hundreds of failures per run = kilobytes). A cleanup script can archive old records.
- macOS-specific failures are discovered after Linux validation. Acceptable because most failures are platform-independent, and the 80% cost savings outweigh the delay.

## Solution Architecture

### Overview

The pipeline is a GitHub Actions workflow with three job tiers: queue reading, per-ecosystem generation with Linux validation, and macOS validation for Linux-passing recipes.

### Workflow Structure

```yaml
# .github/workflows/batch-generate.yml
name: Batch Recipe Generation
on:
  workflow_dispatch:
    inputs:
      ecosystem:
        description: 'Ecosystem to process (or "all")'
        required: true
        default: 'all'
        type: choice
        options: [all, cargo, npm, pypi, rubygems, go, cpan, cask, homebrew]
      batch_size:
        description: 'Max recipes per ecosystem'
        required: true
        default: '25'
        type: number
      tier:
        description: 'Queue tier (1=critical, 2=popular, 3=all)'
        required: true
        default: '2'
        type: choice
        options: ['1', '2', '3']
      skip_macos:
        description: 'Skip macOS validation (Linux only)'
        required: false
        default: false
        type: boolean

concurrency:
  group: batch-generate
  cancel-in-progress: false

permissions:
  contents: write
  pull-requests: write
```

### Job Architecture

```
┌─────────────────────────────────────────────────────────┐
│ Job 1: preflight                                         │
│ - Read batch-control.json (circuit breaker, budget)      │
│ - Read priority queue for selected ecosystem/tier        │
│ - Output: package list per ecosystem, batch_id           │
├─────────────────────────────────────────────────────────┤
│ Job 2: generate-<ecosystem> (matrix, per ecosystem)      │
│ - Build tsuku binary                                     │
│ - For each package in ecosystem slice:                   │
│   - Invoke builder with DeterministicOnly=true           │
│   - On success: validate on Linux (sandbox)              │
│   - On failure: record DeterministicFailedError          │
│ - Output: passing recipes, failure records               │
├─────────────────────────────────────────────────────────┤
│ Job 3: validate-macos (conditional, if !skip_macos)      │
│ - For each Linux-passing recipe:                         │
│   - Validate on macOS (darwin-arm64, darwin-x86_64)      │
│   - Update platform coverage metadata                    │
│ - Output: per-recipe platform results                    │
├─────────────────────────────────────────────────────────┤
│ Job 4: merge                                             │
│ - Collect passing recipes from all ecosystems            │
│ - Exclude recipes with run_command actions               │
│ - Create PR with batch_id in commit message              │
│ - Record failures to data/failures/<ecosystem>.jsonl     │
│ - Update batch-control.json metrics                      │
│ - Auto-merge if gates pass                               │
└─────────────────────────────────────────────────────────┘
```

### Generation Flow (Per Package)

```go
// Pseudocode for batch generation of one package
func generatePackage(ctx context.Context, ecosystem, pkg string) (*Result, error) {
    builder := registry.Get(ecosystem)
    if builder == nil {
        return nil, fmt.Errorf("unknown ecosystem: %s", ecosystem)
    }

    req := BuildRequest{Package: pkg}
    canBuild, err := builder.CanBuild(ctx, req)
    if !canBuild || err != nil {
        return recordFailure(pkg, "api_error", err)
    }

    session, err := builder.NewSession(ctx, req, &SessionOptions{
        DeterministicOnly: true,
    })
    if err != nil {
        return recordFailure(pkg, classifyError(err))
    }
    defer session.Close()

    result, err := session.Generate(ctx)
    if err != nil {
        var detErr *DeterministicFailedError
        if errors.As(err, &detErr) {
            return recordFailure(pkg, string(detErr.Category), detErr)
        }
        return recordFailure(pkg, "api_error", err)
    }

    return &Result{Recipe: result.Recipe, Status: "generated"}, nil
}
```

### Validation Flow

```
Recipe generated
    │
    ├─ Linux validation (ubuntu-latest)
    │   ├─ Schema validation (tsuku validate --strict)
    │   ├─ Plan generation (tsuku eval <recipe>)
    │   └─ Sandbox install (tsuku install --plan --sandbox)
    │       ├─ PASS → promote to macOS validation
    │       └─ FAIL → record failure (validation_failed)
    │
    └─ macOS validation (if Linux passed && !skip_macos)
        ├─ darwin-arm64 (macos-14)
        │   └─ Sandbox install
        └─ darwin-x86_64 (macos-13)
            └─ Sandbox install
```

### Failure Record Format

```jsonl
{"schema_version":1,"ecosystem":"homebrew","environment":"linux-glibc-x86_64","updated_at":"2026-01-29T10:00:00Z","failures":[{"package_id":"homebrew:imagemagick","category":"missing_dep","blocked_by":["libpng","libjpeg"],"message":"formula imagemagick requires dependencies without tsuku recipes","timestamp":"2026-01-29T10:01:23Z"}]}
```

Per-ecosystem files at `data/failures/<ecosystem>.jsonl`. Each line is a complete failure record for one batch run's failures in one environment.

### Batch ID and Commit Format

```
feat(recipes): add batch 2026-01-29-001 cargo recipes

Batch generation of 25 Cargo packages from tier 2 queue.
- 23 passed validation (Linux + macOS)
- 2 failed (recorded to data/failures/cargo.jsonl)

batch_id: 2026-01-29-001
ecosystem: cargo
batch_size: 25
success_rate: 0.92
```

### Circuit Breaker Integration

The preflight job reads `batch-control.json` and skips ecosystems with open circuit breakers:

```json
{
  "circuit_breaker": {
    "homebrew": {"state": "closed", "consecutive_failures": 0},
    "cargo": {"state": "open", "opened_at": "2026-01-28T15:00:00Z",
              "reason": "5 consecutive failures"}
  }
}
```

After generation, the merge job updates circuit breaker state:
- Track consecutive failures per ecosystem
- Open breaker at 10 consecutive failures (>50% over window)
- Half-open after 60 minutes: test with single package
- Close on success; reopen on failure

### Rate Limiting

Per-ecosystem rate limiting in the generation script:

| Ecosystem | Rate Limit | Mechanism |
|-----------|-----------|-----------|
| Homebrew | 1 req/sec to GHCR + Homebrew API | Sleep between packages |
| Cargo | 1 req/sec to crates.io | Sleep between packages |
| NPM | 1 req/sec to registry.npmjs.org | Sleep between packages |
| PyPI | 1 req/sec to pypi.org | Sleep between packages |
| Go | 1 req/sec to proxy.golang.org | Sleep between packages |
| RubyGems | 10 req/min to rubygems.org | Sleep + counter |
| CPAN | 1 req/sec to metacpan.org | Sleep between packages |

### SLI Collection

Each generation job outputs metrics to a JSONL summary:

```jsonl
{"batch_id":"2026-01-29-001","ecosystem":"cargo","total":25,"generated":23,"failed":2,"validated_linux":23,"validated_macos":21,"merged":21,"success_rate":0.84,"duration_seconds":450,"timestamp":"2026-01-29T10:30:00Z"}
```

Metrics are appended to `data/metrics/batch-runs.jsonl` and optionally uploaded to the telemetry endpoint.

### Key Interfaces (Unchanged)

The pipeline uses existing interfaces:
- `SessionBuilder.NewSession()` with `DeterministicOnly: true`
- `BuildSession.Generate()` returning `BuildResult` or `DeterministicFailedError`
- `sandbox.Executor.Run()` for container validation
- `tsuku validate --strict` for schema validation

No new Go interfaces are needed. The pipeline is a workflow + shell scripts invoking the existing CLI.

### Artifact Passing Between Jobs

Jobs communicate via GitHub Actions artifacts (`actions/upload-artifact` and `actions/download-artifact`):

- **preflight → generate**: Package lists as JSON files (one per ecosystem)
- **generate → validate-macos**: Recipe TOML files that passed Linux validation
- **generate → merge**: Recipe TOML files, failure JSONL records
- **validate-macos → merge**: Per-recipe platform validation results

Each artifact is named with the batch ID and ecosystem for traceability. Artifacts expire after 1 day (they're committed to the repo by the merge job).

### Queue Consumption

The preflight job reads `data/priority-queue.json` and selects packages with `status: "pending"` matching the requested ecosystem and tier. The queue uses a status-based lifecycle (see [DESIGN-seed-queue-pipeline.md](current/DESIGN-seed-queue-pipeline.md)):

- **Before generation**: The batch pipeline sets selected packages to `in_progress`.
- **On success**: The merge job sets packages to `success` after the recipe PR merges.
- **On failure**: The merge job sets packages to `failed` and records the failure in JSONL.

The seed tool only creates `pending` entries and never modifies existing statuses. Items are never removed from the queue -- `success` and `skipped` entries serve as deduplication records. An operator can manually change `failed` back to `pending` to re-queue after fixes.

### Data Flow

```
Priority Queue (data/priority-queue.json)
    │
    ├─ Preflight reads queue, filters by ecosystem/tier
    │
    ├─ Generate jobs invoke:  tsuku create --from <eco>:<pkg> --deterministic
    │   ├─ Success → recipe TOML file
    │   └─ Failure → failure record (JSONL)
    │
    ├─ Validate jobs invoke:  tsuku install --plan <recipe> --sandbox
    │   ├─ Pass → recipe stays in PR
    │   └─ Fail → recipe excluded, failure recorded
    │
    ├─ Merge job creates PR with passing recipes
    │   ├─ run_command check → manual review if present
    │   └─ auto-merge if clean
    │
    └─ Output artifacts:
        ├─ data/failures/<ecosystem>.jsonl (failure records)
        ├─ data/metrics/batch-runs.jsonl (SLI metrics)
        └─ batch-control.json (circuit breaker state)
```

## Implementation Approach

### Phase 1: End-to-End Pipeline (Generation + Linux Validation + Merge)

Create the complete workflow: manual dispatch, preflight, per-ecosystem generation with Linux sandbox validation, and merge job with `run_command` gate. This delivers a working pipeline from the first phase. Generation invokes `tsuku create --from <eco>:<pkg> --deterministic` (shell script wrapping the CLI). Failures are recorded to JSONL.

**Files:** `.github/workflows/batch-generate.yml`, `scripts/batch-generate.sh`, `scripts/batch-validate.sh`, `scripts/batch-merge.sh`, `data/failures/`

### Phase 2: macOS Progressive Validation

Add conditional macOS validation job for Linux-passing recipes. Recipes pass through artifact upload from the generate job to the macOS runner. Update platform metadata in recipes.

**Files:** `.github/workflows/batch-generate.yml` (macOS job), `scripts/batch-validate-macos.sh`

### Phase 3: Metrics and Circuit Breaker

Add SLI metrics collection to `data/metrics/batch-runs.jsonl`. Integrate circuit breaker state updates in `batch-control.json` after each run.

**Files:** `data/metrics/`, `scripts/batch-merge.sh` (metrics update)

## Security Considerations

### Download Verification

The batch pipeline downloads artifacts from ecosystem registries (crates.io, npmjs.com, GHCR, etc.). Each ecosystem builder already verifies downloads:
- Homebrew: SHA256 from GHCR manifest annotations
- Cargo/NPM/PyPI/etc.: Checksums from registry APIs
- All downloads use HTTPS

The pipeline reuses existing builder download paths, so checksum verification is inherited. However, the batch pipeline changes the trust model: interactive installs have a human who chose the package, while batch generation removes that decision. The attack surface (number of packages processed without human review) is larger. This is mitigated by the `run_command` gate and sandbox validation, but the changed risk profile should be acknowledged.

### Execution Isolation

Generated recipes are validated in sandbox containers with:
- `--network=none`: No network access during validation
- Resource limits: 2GB RAM, 2 CPU, 5-minute timeout
- Ephemeral containers destroyed after each run
- Read-only mounts for source artifacts

Deterministic-mode recipes produce binary-download-only recipes (no `run_command`, `npm_install`, or `cargo_build` actions). These don't require network during validation. Any recipe that requires network access during sandbox validation (i.e., `RequiresNetwork=true`) fails the auto-merge gate and requires human review.

The batch pipeline itself runs in GitHub Actions runners, which are ephemeral VMs. The workflow declares minimal permissions (`contents: write`, `pull-requests: write`) to limit blast radius if a job is compromised.

### Supply Chain Risks

The pipeline generates recipes from upstream registries. Supply chain risks include:

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Compromised upstream package | Checksums verified against registry; sandbox catches malicious behavior | Zero-day compromise before registry detection |
| Typosquatting in queue | Priority queue populated from official registry APIs, not user input | Malicious package with legitimate-looking name |
| `run_command` injection | Auto-merge gate blocks recipes with `run_command`; human review required | Reviewer oversight |
| Batch poisoning (many bad recipes) | Rollback via batch_id; circuit breaker detects high failure rates (reliability control, not security) | Low-volume poisoning (1-2 recipes/batch) stays under circuit breaker threshold |

### User Data Exposure

The batch pipeline does not access user data. It operates on:
- Public registry APIs (anonymous or authenticated with CI tokens)
- Priority queue data (package names and popularity scores)
- Generated recipe files (TOML definitions)

No user-identifying information is transmitted. CI tokens are GitHub-provided GITHUB_TOKEN with minimal permissions.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| macOS budget exhaustion | Progressive validation skips macOS for Linux failures; budget tracking in batch-control.json | Unexpected macOS runner costs from long-running validations |
| Rate limit exceeded | Per-ecosystem sleep between requests; conservative defaults | Transient API errors causing false circuit breaker trips |
| Merge of bad recipe | Sandbox validation + schema validation + run_command gate | Recipe works in sandbox but fails in real environment |
| Failure data loss | JSONL committed to repo with full git history | Merge conflicts during concurrent runs (mitigated by per-ecosystem files) |

## Consequences

### Positive

- Automated recipe generation at scale across 8 ecosystems
- Structured failure data enables gap analysis (downstream #1190)
- Progressive validation keeps macOS costs manageable
- Batch ID enables surgical rollback of any batch
- Circuit breaker prevents runaway failures

### Negative

- Manual trigger requires operator attention (no automatic cadence)
- JSONL files in repo increase repo size over time
- macOS-specific failures discovered after Linux validation

### Mitigations

- Add cron trigger once pipeline reliability is proven (1A → 1B transition)
- Archive old failure records periodically (keep last 90 days in repo)
- Weekly macOS sweep validates all recently merged recipes
