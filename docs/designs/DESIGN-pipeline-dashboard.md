---
status: Proposed
problem: |
  The batch pipeline runs hourly but generates zero new recipes because all remaining
  Homebrew packages fail deterministic generation. The dashboard at tsuku.dev/pipeline/
  displays stale data (last successful batch: Feb 6) and lacks debugging visibility.
  Users can't see why packages fail, circuit breaker status, or ecosystem coverage.
  Meanwhile, disambiguation exists in the CLI but isn't integrated into batch generation,
  and only Homebrew runs despite the design supporting 8 ecosystems.
decision: |
  Augment the existing pipeline with four changes: (1) expand the dashboard with
  drill-down navigation where every panel links to a list page and every list item
  links to a detail page showing full information without requiring JSON file inspection;
  (2) integrate disambiguation into batch generation so packages route to the correct
  ecosystem before recipe creation; (3) enable multi-ecosystem scheduled runs so all
  8 supported ecosystems process autonomously; (4) add failure subcategories for
  precise debugging. This builds on DESIGN-registry-scale-strategy rather than replacing it.
rationale: |
  The current infrastructure works but has visibility and coverage gaps. Rebuilding
  would duplicate effort already invested in the batch workflow, circuit breaker,
  and validation matrix. The dashboard enhancement is the highest-leverage change
  since it enables debugging without requiring pipeline changes. Multi-ecosystem
  support and disambiguation integration address the coverage gap while reusing
  existing builder code.
---

# DESIGN: Pipeline Dashboard Enhancement

## Status

Proposed

## Upstream Design Reference

This design augments [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md).

**Relevant sections:**
- Failure Analysis System: structured failure tracking
- Phase 2: Failure Analysis Backend + macOS Platform (dashboard infrastructure planned but partially implemented)
- DESIGN-operator-dashboard.md: mentioned as recommended but not created

## Context and Problem Statement

The batch recipe generation pipeline and its dashboard (tsuku.dev/pipeline/) are operational but have gaps that prevent autonomous multi-ecosystem coverage.

**Current state:**
- Pipeline runs hourly via `batch-generate.yml`
- Validation runs across 11 platform environments (5 Linux x86_64 families, 4 Linux arm64 families, 2 macOS architectures)
- Dashboard shows queue status, blockers, failure categories, and recent runs
- Circuit breaker pattern prevents runaway failures

**Root cause hypothesis:**

Popular tools (bat, fd, rg, etc.) are mapped to `github:` or `cargo:` sources in `data/disambiguations/curated.jsonl`, but batch generation hardcodes `--from homebrew:<name>`. These packages don't have Homebrew bottles (they're Rust crates distributed via GitHub releases). All 10 packages selected each hour fail deterministic generation because the pipeline tries to extract bottles that don't exist.

**Observed problems:**

1. **Zero recipe throughput** (symptom): The pipeline has run successfully since Feb 9 but generates 0 new recipes per run. The dashboard shows "last run: Feb 6" because that was the last run that actually merged recipes.

2. **Wrong ecosystem routing** (root cause): Batch generation ignores disambiguation. Packages that should route to `cargo:ripgrep` or `github:sharkdp/bat` fail when processed as `homebrew:ripgrep` or `homebrew:bat`.

3. **No failure debugging** (observability gap): The dashboard shows failure counts but not why packages fail. The `validation_failed` category covers too many distinct problems (missing bottles, bottle extraction errors, verify pattern mismatches).

4. **Single-ecosystem operation** (coverage gap): Despite supporting 8 ecosystems, only Homebrew runs on schedule. Other ecosystems have zero queue entries.

5. **Circuit breaker invisible** (observability gap): The circuit breaker state exists in `batch-control.json` but isn't shown on the dashboard.

### Scope

**In scope:**
- Dashboard enhancements for failure visibility and debugging
- Integration of disambiguation into batch generation
- Multi-ecosystem scheduling
- Failure category refinement

**Out of scope:**
- LLM-based generation (excluded by design in DESIGN-registry-scale-strategy)
- New ecosystems beyond the 8 already supported
- Backend service changes (failure analysis backend is Phase 2, not this design)
- Dashboard styling or UX redesign

## Decision Drivers

1. **Autonomous operation**: The pipeline should run without manual intervention
2. **Debug-first**: Operators need to understand failures before fixing them
3. **Incremental enhancement**: Build on existing infrastructure, don't rebuild
4. **Multi-ecosystem fairness**: All ecosystems should progress, not just Homebrew
5. **Disambiguation early**: Route packages to correct ecosystem before generation
6. **Transparency**: Users should see pipeline health at a glance

## Implementation Context

### Existing Patterns

**Dashboard data flow:**
- `cmd/queue-analytics/` generates `website/pipeline/dashboard.json`
- Workflow `update-dashboard.yml` triggers on `data/` changes
- Frontend JavaScript fetches and renders the JSON

**Batch generation flow:**
- `cmd/batch-generate/` orchestrates via `internal/batch/`
- Selects pending packages from ecosystem-specific queue
- Invokes `tsuku create --from <ecosystem>:<name> --deterministic-only`
- Records failures to `data/failures/<ecosystem>-<timestamp>.jsonl`

**Disambiguation implementation:**
- `internal/disambiguation/` contains ecosystem routing logic
- `data/disambiguations/curated.jsonl` stores manual overrides
- CLI uses disambiguation in `install` command but not in `create`

### Queue State (as of Feb 15)

```
Total: 5,144 packages
- Pending: 4,988 (97%)
- Success: 138 (2.7%)
- Failed: 14 (0.3%)
- Blocked: 4 (0.1%)
```

All packages are in the homebrew queue. No other ecosystem queues exist.

## Considered Options

### Decision 1: Dashboard Failure Visibility

The dashboard currently shows failure categories but lacks detail for debugging. When all 10 packages fail hourly with `validation_failed`, operators can't determine if it's bottle availability, verify pattern issues, or something else. The failure JSONL files contain this data but aren't exposed.

#### Chosen: Drill-Down Dashboard with Full Detail Pages

Every dashboard panel links to a dedicated page, and every list item links to a detail page. No JSON file inspection required.

**Main dashboard (index.html)** shows summary panels:
- Each panel displays a preview (e.g., last 5 failures, recent 3 runs)
- Each panel header is clickable → navigates to full list page
- "Pipeline Health" panel shows:
  - **Pipeline Status**: "Running" / "Stalled" (based on last_run timestamp)
  - **Last Run**: "1 hour ago (0/10 succeeded)" - shows pipeline is alive even with failures
  - **Last Success**: "9 days ago (2 recipes)" - when recipes were last merged
  - **Runs Since Success**: "156 runs" - quantifies the drought
  - **Circuit Breaker**: per-ecosystem state (closed/open/half-open)

**List pages** show complete data:
- `failures.html`: All failures with filtering by category, ecosystem, date range
- `runs.html`: All batch runs with success/fail counts (existing, enhanced)
- `blocked.html`: All blocked packages with dependency info (existing, enhanced)
- `pending.html`: All pending packages by ecosystem (existing)

**Detail pages** show single-item deep dive:
- `failure.html?id=<failure-id>`: Full failure record including:
  - Package ID and ecosystem
  - Category and subcategory
  - Full error message (not truncated)
  - Stack trace or CLI output if available
  - Timestamp and batch ID
  - Platform where failure occurred
  - Link to related workflow run (if available)
- `run.html?id=<batch-id>`: Full batch run details including:
  - All packages processed
  - Per-platform results
  - Recipes generated
  - Failures encountered

**Navigation pattern:**
```
Dashboard Panel → List Page → Detail Page
     ↓                ↓            ↓
  "Failures (42)"  → failures.html → failure.html?id=xyz
  "Recent Runs"    → runs.html     → run.html?id=2026-02-15-homebrew
  "Blocked (4)"    → blocked.html  → (package detail in queue)
```

This reuses existing data in `data/failures/` and `batch-control.json`. The `queue-analytics` command aggregates everything into `dashboard.json` with enough detail for all pages.

#### Alternatives Considered

**Grafana/external dashboarding**: Build metrics pipeline to external service.
Rejected because it adds operational complexity: another service to deploy, another set of credentials to manage, another monitoring target. The dashboard.json is 788KB and serves 5K packages. Grafana's value comes from alerting and historical trends; we need debugging visibility, not time-series analysis.

**Log aggregation**: Point operators to GitHub Actions logs.
Rejected because logs are ephemeral (90 days retention) and require navigating through workflow runs. Finding why `neovim` failed means searching across 20+ workflow runs. A persistent dashboard with recent failures is more accessible.

**Structured JSON in PR comments**: Enhance batch PR bodies with failure details.
Rejected because PR bodies have size limits (65K characters) and aren't queryable for aggregation. The batch workflow already creates PRs with validation summaries; enhancing these would help individual PR review but not overall pipeline debugging.

### Decision 2: Failure Category Refinement

The current `categoryFromExitCode` maps exit codes to categories but lumps too much under `validation_failed`. Exit code 6 covers both "verify pattern mismatch" and "binary not found". Exit code 7 covers "recipe schema invalid" and "install failed". This makes debugging impossible.

#### Chosen: Structured Failure Subcategories

Extend the failure record schema to include a `subcategory` field. The CLI already outputs JSON with `--json`; parse it for additional detail.

New category structure:
```
deterministic_insufficient
  → no_bottle (no bottle available for platform)
  → archive_extraction_failed (bottle exists but extraction fails)
  → binary_discovery_failed (no executables found in archive)

validation_failed
  → verify_pattern_mismatch (version output doesn't match pattern)
  → verify_timeout (verify command didn't complete)
  → install_failed (install action failed)
  → schema_invalid (recipe TOML doesn't validate)

missing_dep
  → (already has blocked_by field)

api_error
  → rate_limited (429 from ecosystem API)
  → upstream_unavailable (5xx from ecosystem API)
  → timeout (network timeout)
```

The `parseInstallJSON` function in `orchestrator.go` already extracts some of this. Extend it to populate subcategory from CLI output.

#### Alternatives Considered

**Exit code explosion**: Add new exit codes for each failure type.
Rejected because exit codes are limited (0-255) and the CLI already uses structured JSON output. Parsing JSON is cleaner than inventing new exit codes.

**Separate log files**: Write different failure types to different files.
Rejected because it fragments the data and makes aggregation harder. A single JSONL with structured fields is easier to query.

### Decision 3: Multi-Ecosystem Scheduling

The current workflow uses `inputs.ecosystem || 'homebrew'` so scheduled runs always process Homebrew. Other ecosystems require manual dispatch. The queue only has Homebrew packages because no one seeded other ecosystems.

#### Chosen: Ecosystem Rotation with Per-Ecosystem Queues

Modify the scheduled workflow to rotate through ecosystems. Each hour, process a different ecosystem in round-robin:

1. Create ecosystem-specific queues: `data/queues/priority-queue-<ecosystem>.json`
2. Add a seeding script per ecosystem (similar to `seed-homebrew.sh` but for cargo/npm/etc.)
3. Modify the hourly cron job to cycle through ecosystems:
   - Hour 0, 8, 16: homebrew
   - Hour 1, 9, 17: cargo
   - Hour 2, 10, 18: npm
   - Hour 3, 11, 19: pypi
   - Hour 4, 12, 20: rubygems
   - Hour 5, 13, 21: go
   - Hour 6, 14, 22: cpan
   - Hour 7, 15, 23: cask

This spreads load across ecosystems while keeping CI costs predictable (same hourly budget).

#### Alternatives Considered

**Parallel ecosystem processing**: Run all 8 ecosystems concurrently each hour.
Rejected because it multiplies CI costs by 8x. The validation matrix already runs 11 platform environments per batch; 8 concurrent ecosystems would mean 88 parallel jobs. This also creates rate-limiting pressure on ecosystem APIs that have different limits (RubyGems: 300 requests/5min, npm: 60 requests/min for search).

**Demand-weighted rotation**: Weight ecosystem hours by queue depth or user demand.
Rejected for initial implementation because we don't have reliable demand signals yet. Telemetry data is sparse, and queue depth reflects seeding strategy more than user need. However, this remains a valid future iteration once we have per-ecosystem success rates and can weight by "likely to succeed" rather than raw counts.

**Priority-weighted by pending count**: Process ecosystems proportional to pending package count.
Rejected because this effectively means only homebrew runs. With 97% of packages in homebrew's queue, other ecosystems would get ~1 hour/week. The point of multi-ecosystem support is to demonstrate that tsuku handles more than homebrew; fair rotation achieves that goal.

### Decision 4: Disambiguation Integration

Batch generation currently assumes all packages in the homebrew queue should use `--from homebrew:<name>`. But some packages (like `rg`) should route to `github:BurntSushi/ripgrep` or `cargo:ripgrep` instead. The disambiguation system knows this but isn't consulted.

#### Chosen: Pre-Generation Disambiguation Check

Before invoking `tsuku create`, check if the package has a disambiguation record. If yes, use the selected ecosystem instead of the queue's ecosystem.

Flow:
1. Load `data/disambiguations/curated.jsonl`
2. For each package in batch:
   - Check if package name has a curated disambiguation
   - If yes, override `--from` to use the curated source
   - If no, use the queue's ecosystem as before
3. Record which source was used in the batch metrics

This doesn't auto-discover disambiguation; it uses existing curated records. Auto-discovery remains a CLI feature for interactive use.

#### Alternatives Considered

**Auto-discover in batch**: Run disambiguation lookup for every package.
Rejected because disambiguation queries multiple ecosystems (rate-limit risk) and involves heuristics that may be wrong for batch automation. Curated records are explicit and safe.

**Queue per source**: Instead of `homebrew:jq`, use `github:jqlang/jq` as the queue entry.
Rejected because it requires re-seeding all queues with full source URLs. Using curated overrides is a smaller change.

### Assumptions

1. **Curated.jsonl is authoritative for batch**: When a tool is in curated.jsonl, the batch pipeline should use that source. Gap: some curated entries map to `github:` sources, which require LLM-based generation (excluded from batch). Mitigation: the orchestrator should skip packages that route to unsupported sources.

2. **Failure categories map to actionable remediation**: Operators know what to do when they see "verify_pattern_mismatch" vs "binary_discovery_failed". If this doesn't hold, we'll need to add resolution guidance to the dashboard.

3. **Each ecosystem has a viable seeding strategy**: We assume we can populate queues for cargo, npm, etc. using public APIs. This is untested for some ecosystems.

4. **Dashboard data refresh is sufficient**: The dashboard regenerates when data changes. If all batches fail, no data changes, so the dashboard can become stale. Mitigation: include "generated_at" timestamp prominently.

### Uncertainties

- The root cause hypothesis (disambiguation not integrated) hasn't been validated. Running `tsuku create --from github:sharkdp/bat` manually would confirm if this unblocks generation.
- Ecosystem-specific seeders don't exist yet for 7 of 8 ecosystems.
- The disambiguation curated list has ~30 entries; many of the 5K queued packages may not have records.

### Success Metrics

- **Primary**: Recipe throughput increases from 0/week to >10/week within 2 weeks of deployment
- **Secondary**: Time to diagnose a failure decreases from "check workflow logs" (~5 minutes) to "check dashboard" (~30 seconds)
- **Coverage**: All 8 ecosystems have at least 1 pending package in queue
- **Health visibility**: Operators can determine pipeline health status in <10 seconds via dashboard

## Decision Outcome

**Chosen: All four enhancements (Dashboard visibility + Category refinement + Multi-ecosystem rotation + Disambiguation integration)**

### Summary

We're making the pipeline autonomous by addressing its three gaps: visibility, coverage, and routing.

For visibility, the dashboard gets two new panels. "Recent Failures" shows the last 20 failures with package name, ecosystem, category, subcategory, one-line message, and timestamp. "Pipeline Health" shows circuit breaker state per ecosystem, last successful batch with recipe count, and time since last recipe merged. Both panels draw from existing data files (`data/failures/*.jsonl` and `batch-control.json`), aggregated by the `queue-analytics` command into `dashboard.json`.

For coverage, the hourly cron job rotates through all 8 ecosystems in round-robin. Each ecosystem gets 3 hours per day. This requires creating per-ecosystem queues (`data/queues/priority-queue-<ecosystem>.json`) and seeding scripts. The rotation schedule is deterministic based on UTC hour modulo 8.

For routing, batch generation checks `data/disambiguations/curated.jsonl` before invoking `tsuku create`. If a package has a curated disambiguation record, it uses the selected source instead of assuming the queue's ecosystem. This handles cases like `rg` routing to `github:BurntSushi/ripgrep` instead of `homebrew:ripgrep`.

Failure categories get refined subcategories to distinguish "no bottle available" from "verify pattern mismatch" from "binary not found". The existing `parseInstallJSON` function is extended to extract subcategory from CLI output.

### Rationale

These changes work together because visibility enables debugging, which informs routing and coverage decisions. Without seeing why packages fail, we can't know if disambiguation would help or if the queue needs filtering.

The incremental approach fits because the existing infrastructure (workflow, circuit breaker, validation matrix) works correctly. The problem isn't execution but configuration (single ecosystem) and observability (no debugging info). Adding capabilities is lower risk than rebuilding.

Multi-ecosystem rotation trades throughput for fairness. A pipeline stuck on homebrew makes no progress on other ecosystems even if they'd succeed. Spreading load ensures all ecosystems advance, even if homebrew's backlog takes longer to clear.

## Solution Architecture

### Components

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Dashboard (website/pipeline/)                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  PAGES (all panels link to list pages, all list items link to       │
│         detail pages)                                               │
│                                                                     │
│  index.html (main dashboard)                                        │
│  ├── Queue Status panel → pending.html                              │
│  ├── Top Blockers panel → blocked.html                              │
│  ├── Failure Categories panel → failures.html                       │
│  ├── Recent Runs panel → runs.html                                  │
│  ├── [NEW] Recent Failures panel → failures.html                    │
│  ├── [NEW] Pipeline Health panel (breaker state, last success)      │
│  └── Disambiguation panel → disambiguations.html                    │
│                                                                     │
│  [NEW] failures.html (list all failures, filterable)                │
│  ├── Table: package, ecosystem, category, subcategory, timestamp    │
│  ├── Filters: by category, ecosystem, date range                    │
│  └── Each row → failure.html?id=<failure-id>                        │
│                                                                     │
│  [NEW] failure.html?id=<id> (single failure detail)                 │
│  ├── Full error message (not truncated)                             │
│  ├── CLI output / stack trace                                       │
│  ├── Platform, batch ID, timestamp                                  │
│  └── Link to workflow run (if available)                            │
│                                                                     │
│  runs.html (existing, enhanced)                                     │
│  ├── Each row → run.html?id=<batch-id>                              │
│                                                                     │
│  [NEW] run.html?id=<id> (single run detail)                         │
│  ├── All packages processed                                         │
│  ├── Per-platform results                                           │
│  └── Recipes generated, failures encountered                        │
│                                                                     │
│  pending.html, blocked.html, success.html (existing, enhanced)      │
│  └── Each row → package detail or disambiguation page               │
│                                                                     │
│  dashboard.json                                                     │
│  ├── queue: { total, by_status, packages }                         │
│  ├── blockers: [...]                                                │
│  ├── runs: [...]                                                    │
│  ├── disambiguations: { total, by_reason, need_review }             │
│  ├── [NEW] failures: [{ id, package, category, subcategory, ... }]  │
│  ├── [NEW] health: { per_ecosystem_breaker, last_success, ... }     │
│  └── generated_at                                                   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                      Batch Generation Pipeline                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  batch-generate.yml                                                 │
│  ├── [MODIFY] schedule: ecosystem rotation based on hour            │
│  ├── [MODIFY] env.ECOSYSTEM: computed from $(date +%H) % 8          │
│  └── (rest unchanged)                                               │
│                                                                     │
│  cmd/batch-generate/main.go                                         │
│  └── (unchanged, ecosystem passed via flag)                         │
│                                                                     │
│  internal/batch/orchestrator.go                                     │
│  ├── [MODIFY] generate(): check disambiguation before --from        │
│  ├── [MODIFY] parseInstallJSON(): extract subcategory               │
│  └── [MODIFY] FailureRecord: add Subcategory field                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                            Data Files                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  data/queues/                                                       │
│  ├── priority-queue-homebrew.json (existing)                        │
│  ├── [NEW] priority-queue-cargo.json                                │
│  ├── [NEW] priority-queue-npm.json                                  │
│  ├── [NEW] priority-queue-pypi.json                                 │
│  ├── [NEW] priority-queue-rubygems.json                             │
│  ├── [NEW] priority-queue-go.json                                   │
│  ├── [NEW] priority-queue-cpan.json                                 │
│  └── [NEW] priority-queue-cask.json                                 │
│                                                                     │
│  data/failures/*.jsonl                                              │
│  └── [MODIFY] records now include subcategory field                 │
│                                                                     │
│  data/disambiguations/curated.jsonl                                 │
│  └── (existing, read by orchestrator)                               │
│                                                                     │
│  batch-control.json                                                 │
│  └── (existing, read by queue-analytics for health display)         │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Structures

**Extended FailureRecord** (internal/batch/failure.go):
```go
type FailureRecord struct {
    PackageID   string   `json:"package_id"`
    Category    string   `json:"category"`
    Subcategory string   `json:"subcategory,omitempty"` // NEW
    BlockedBy   []string `json:"blocked_by,omitempty"`
    Message     string   `json:"message"`
    Timestamp   time.Time `json:"timestamp"`
}
```

**Dashboard health section** (website/pipeline/dashboard.json):
```json
{
  "health": {
    "ecosystems": {
      "homebrew": {
        "breaker_state": "closed",
        "last_failure": "2026-02-08T09:31:11Z",
        "consecutive_failures": 0
      },
      "cargo": {
        "breaker_state": "closed",
        "last_failure": null,
        "consecutive_failures": 0
      }
    },
    "last_run": {
      "batch_id": "2026-02-15-homebrew",
      "ecosystem": "homebrew",
      "timestamp": "2026-02-15T13:45:21Z",
      "succeeded": 0,
      "failed": 10,
      "total": 10
    },
    "last_successful_run": {
      "batch_id": "2026-02-06-homebrew",
      "recipes_merged": 2,
      "timestamp": "2026-02-06T20:45:08Z"
    },
    "runs_since_last_success": 156,
    "hours_since_last_run": 1,
    "hours_since_recipe_merged": 213
  }
}
```

This distinguishes:
- **last_run**: Most recent batch execution (shows pipeline is alive even with 0 recipes)
- **last_successful_run**: Most recent batch that produced recipes
- **runs_since_last_success**: How many batches have run without merging recipes (156 runs = ~6.5 days at hourly)

**Failures section** (all failures, not just recent):
```json
{
  "failures": [
    {
      "id": "homebrew-2026-02-15T13-45-21Z-neovim",
      "package": "neovim",
      "ecosystem": "homebrew",
      "category": "validation_failed",
      "subcategory": "verify_pattern_mismatch",
      "message": "expected version pattern 'v0.10.0' but got 'NVIM v0.10.0'",
      "full_output": "tsuku install: verification failed\nexpected: v0.10.0\ngot: NVIM v0.10.0\nexit code: 6",
      "platform": "linux-x86_64-debian",
      "batch_id": "2026-02-15-homebrew",
      "timestamp": "2026-02-15T13:45:21Z",
      "workflow_run_url": "https://github.com/tsukumogami/tsuku/actions/runs/22036696489"
    }
  ],
  "failures_summary": {
    "total": 42,
    "by_category": {
      "validation_failed": 30,
      "deterministic_insufficient": 8,
      "api_error": 4
    },
    "by_ecosystem": {
      "homebrew": 42
    }
  }
}
```

**Runs section** (full detail per run):
```json
{
  "runs": [
    {
      "batch_id": "2026-02-15-homebrew",
      "ecosystem": "homebrew",
      "timestamp": "2026-02-15T13:45:21Z",
      "total": 10,
      "succeeded": 0,
      "failed": 10,
      "blocked": 0,
      "packages_processed": ["neovim", "bat", "fd", "..."],
      "recipes_generated": [],
      "workflow_run_url": "https://github.com/tsukumogami/tsuku/actions/runs/22036696489",
      "platform_results": {
        "linux-x86_64-debian": {"tested": 10, "passed": 0, "failed": 10},
        "darwin-arm64": {"tested": 10, "passed": 0, "failed": 10}
      }
    }
  ]
}
```

### Ecosystem Rotation Logic

In `batch-generate.yml`:
```yaml
env:
  # Rotate through ecosystems based on UTC hour
  ECOSYSTEM_INDEX: ${{ github.event_name == 'schedule' && (github.run_number % 8) || (inputs.ecosystem_index || 0) }}
  ECOSYSTEMS: '["homebrew","cargo","npm","pypi","rubygems","go","cpan","cask"]'
  ECOSYSTEM: ${{ fromJSON(env.ECOSYSTEMS)[fromJSON(env.ECOSYSTEM_INDEX)] }}
```

Actually, `run_number` increments globally so this won't give fair rotation. Use a simpler approach:

```bash
HOUR=$(date -u +%H)
INDEX=$((HOUR % 8))
ECOSYSTEMS=("homebrew" "cargo" "npm" "pypi" "rubygems" "go" "cpan" "cask")
echo "ECOSYSTEM=${ECOSYSTEMS[$INDEX]}" >> "$GITHUB_ENV"
```

### Disambiguation Check

In `orchestrator.go`, before calling `generate()`:

```go
func (o *Orchestrator) resolveSource(pkg seed.Package) string {
    // Check if disambiguation has an override
    if override := o.disambiguations.LookupSource(pkg.Name); override != "" {
        return override
    }
    // Fall back to queue ecosystem
    return pkg.ID
}
```

Load disambiguations at orchestrator construction:
```go
func NewOrchestrator(cfg Config, queue *seed.PriorityQueue) *Orchestrator {
    disambiguations, _ := loadDisambiguations("data/disambiguations/curated.jsonl")
    return &Orchestrator{cfg: cfg, queue: queue, disambiguations: disambiguations}
}
```

## Implementation Approach

### Phase 1: Dashboard Visibility with Drill-Down Navigation

Add failure debugging and full navigation to the dashboard.

1. Extend `cmd/queue-analytics/` to aggregate:
   - All failures from `data/failures/*.jsonl` (not just recent)
   - Full failure details including `full_output`, `platform`, `workflow_run_url`
   - Circuit breaker state from `batch-control.json`
   - Time since last successful batch (derived from `data/metrics/batch-runs.jsonl`)
   - Full run details including per-platform results

2. Create drill-down page structure:
   - `failures.html`: List all failures with filters (category, ecosystem, date)
   - `failure.html`: Single failure detail page (query param `?id=<failure-id>`)
   - `run.html`: Single run detail page (query param `?id=<batch-id>`)
   - Enhance existing `runs.html` to link to `run.html?id=`

3. Update `website/pipeline/index.html`:
   - Add "Recent Failures" panel linking to `failures.html`
   - Add "Pipeline Health" panel with breaker state
   - Make all panel headers clickable → link to respective list pages
   - Make all table rows clickable → link to detail pages

4. Update `website/pipeline/dashboard.json` schema:
   - Add `failures` array with full failure records
   - Add `failures_summary` for category/ecosystem breakdown
   - Extend `runs` array with full detail per run
   - Add `health` section

**Deliverables:**
- Modified `cmd/queue-analytics/`
- Modified `website/pipeline/index.html`
- New `website/pipeline/failures.html`
- New `website/pipeline/failure.html`
- New `website/pipeline/run.html`
- Modified `website/pipeline/runs.html`
- Updated dashboard.json schema

**Validation:**
- All dashboard panels are clickable and navigate to list pages
- All list items are clickable and navigate to detail pages
- Failure detail page shows full error output (not truncated)
- No need to inspect JSON files for any information
- Pipeline Health clearly distinguishes "last run" from "last successful run"
- A pipeline running hourly with 0 successes shows as "Running" not stalled
- "Runs since success" counter shows how many batches have failed to produce recipes

### Phase 2: Disambiguation Integration

Route packages through disambiguation before generation. This comes before multi-ecosystem queues because `curated.jsonl` already exists and can immediately improve homebrew batch success.

1. Add disambiguation loading to orchestrator constructor.

2. Modify `generate()` to use `resolveSource()` instead of raw `pkg.ID`.

3. Handle unsupported sources gracefully: if curated source is `github:` (requires LLM), skip the package with `blocked` status and a clear reason.

4. Add metrics for "used_disambiguation" in batch results.

5. Update dashboard to show disambiguation usage in recent runs.

**Deliverables:**
- Modified `internal/batch/orchestrator.go`
- Modified batch result schema
- Modified dashboard

**Validation:**
- Packages with curated disambiguations use the curated source
- Packages routed to `github:` are skipped (not failed)
- Batch metrics show how many packages used disambiguation

### Phase 3: Failure Subcategories

Refine failure categories for better debugging.

1. Add `Subcategory` field to `FailureRecord` struct.

2. Verify `tsuku install --json` output includes subcategory information. If not, add it to CLI first.

3. Extend `parseInstallJSON` in `orchestrator.go` to extract subcategory from CLI JSON output.

4. Update `categoryFromExitCode` to return `(category, subcategory)` tuple.

5. Modify failure JSONL writing to include subcategory.

6. Update `queue-analytics` to aggregate by subcategory.

**Deliverables:**
- Modified `internal/batch/orchestrator.go`
- Modified `internal/batch/failure.go`
- Modified `cmd/queue-analytics/`
- Possibly modified CLI (`tsuku install --json` output schema)

**Validation:**
- Failure records include meaningful subcategories
- Dashboard shows subcategory breakdown

### Phase 4: Multi-Ecosystem Queues

Create queues for all ecosystems and modify scheduling.

1. Create seeding scripts for each ecosystem:
   - `scripts/seed-cargo.sh`
   - `scripts/seed-npm.sh`
   - etc.

2. Create initial queue files (can be empty or seeded with popular packages):
   - `data/queues/priority-queue-cargo.json`
   - etc.

3. Modify `batch-generate.yml` to compute ecosystem from UTC hour.

4. Run seeding workflow to populate queues.

**Deliverables:**
- 7 new seeding scripts
- 7 new queue files
- Modified `batch-generate.yml`
- Seeding workflow or manual seed run

**Validation:**
- Workflow runs different ecosystems at different hours
- Each ecosystem queue has packages
- No errors when processing non-homebrew ecosystems

## Security Considerations

### Download Verification

**No change.** Recipe generation and installation continue to use existing checksum verification. Downloaded artifacts are validated against checksums from ecosystem APIs (Homebrew bottle checksums, npm integrity hashes, etc.).

### Execution Isolation

**No change.** Batch generation runs in GitHub Actions CI with ephemeral runners. Generated recipes are validated in Docker containers (Linux) or sandboxed macOS environments. No new execution surface is added.

### Supply Chain Risks

**Disambiguation lookup is read-only.** The curated disambiguations file is checked into the repository and reviewed via PR. Batch generation reads this file but can't modify it during execution.

**New ecosystem queues follow existing patterns.** Cargo, npm, and other ecosystems use the same seeding approach as Homebrew: query public APIs for popular packages, store in static JSON files, generate recipes deterministically from official ecosystem sources.

**No new external dependencies.** All ecosystem APIs are already used by tsuku's builders. Multi-ecosystem rotation doesn't introduce new API integrations.

### User Data Exposure

**Minimal operational data only.** The dashboard displays pipeline operational data: failure counts, batch metrics, package names processed, and platform information. No personally identifiable information (PII) is collected or displayed. Error messages in failure records could contain path information from CI environments, but these are ephemeral runner paths with no user association.

Note: The failure records reveal ecosystem trends (which packages are being processed) and platform distribution (OS, architecture). This is acceptable for a public package manager where all recipes are public anyway.

## Consequences

### Positive

- **Debugging enabled**: Operators can now see why packages fail without reading workflow logs
- **Multi-ecosystem progress**: All 8 ecosystems advance instead of only Homebrew
- **Correct routing**: Packages with known better sources (github:, cargo:) use them
- **Health visibility**: Circuit breaker state visible at a glance
- **Incremental**: Builds on existing infrastructure without rebuilding

### Negative

- **Slower Homebrew progress**: Homebrew gets 3 hours/day instead of 24
- **More files**: 7 new queue files and seeding scripts to maintain
- **Disambiguation dependency**: Packages without curated records still use wrong ecosystem

### Neutral

- **CI cost unchanged**: Same hourly budget, just spread across ecosystems
- **No new external services**: Still static JSON, no Grafana/Prometheus/etc.
