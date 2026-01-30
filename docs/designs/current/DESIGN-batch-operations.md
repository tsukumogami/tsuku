---
status: Current
problem: The batch recipe generation pipeline can auto-merge recipes into the registry, but without operational controls, operators have no way to halt runaway generation, revert problematic recipes, or respond to incidents.
decision: Batch ID metadata + git revert for rollback; Circuit breaker + control file for emergency stop; Time-windowed budget + sampling for cost control; Per-ecosystem SLIs with severity alerting; Hybrid storage (repository-primary)
rationale: Combines automatic response (circuit breaker) with manual override (control file). Batch IDs solve commit identification. Per-ecosystem metrics enable targeted response. Repository-primary storage avoids external dependencies for critical control path.
---

# DESIGN: Batch Operations and Rollback Procedures

## Status

Current

## Upstream Design Reference

This design implements part of [DESIGN-registry-scale-strategy.md](DESIGN-registry-scale-strategy.md).

**Relevant sections:**
- Phase 0: Rollback scripts (tested against manually-created recipes)
- Phase 1b: Emergency stop, SLI/SLO definitions, circuit breaker
- Security: Post-merge monitoring, incident response

**This design must deliver:**
- Rollback procedure specification (required by #1189)
- Emergency stop mechanism (required by #1189)

## Context and Problem Statement

The registry scale strategy introduces automated batch generation that will auto-merge recipes into the registry. This pipeline operates at scale: hundreds of recipes generated per batch run, multiple ecosystems processing in parallel, and automatic PR merges without human review for deterministic recipes.

This creates significant operational risk:
- **Blast radius amplification**: A bug in the validation pipeline or a compromised upstream package could result in many problematic recipes being merged before detection
- **No defined recovery path**: If a batch run introduces broken recipes, there's no documented way to identify and revert them
- **Cost runaway**: macOS CI minutes cost 10x Linux; uncontrolled batch runs could exceed monthly budgets within days
- **Incident response vacuum**: Operators have no playbook for common failure scenarios

### Scale Assumptions

These assumptions inform option evaluation:

| Metric | Assumption | Implication |
|--------|------------|-------------|
| Batch size | 50-100 recipes per CI run | ~100 recipes worst-case before detection |
| Run frequency | Daily (nightly schedule) | Detection latency up to 24 hours |
| Detection latency | 1-6 hours (health check cycle) | Emergency stop must work within minutes |
| Merge rate | 10-20 recipes/hour when active | Rollback may need to revert 100+ recipes |

### Why Now

The batch pipeline (DESIGN-batch-recipe-generation.md) depends on this design. Operations readiness is explicitly elevated to "required" status in the upstream design because auto-merge amplifies the blast radius of any failure. Without runbooks and emergency procedures, operators will improvise during incidents.

### Scope

**In scope:**
- Rollback procedure for reverting merged recipes
- Emergency stop mechanism for halting batch processing
- Cost control mechanisms (budget caps, sampling)
- SLI/SLO definitions for batch health
- Runbook templates for common scenarios
- Post-merge monitoring for security (checksum drift)
- Access control for operational interventions

**Out of scope:**
- Batch pipeline implementation (covered by DESIGN-batch-recipe-generation.md)
- Failure analysis backend (covered by DESIGN-batch-failure-analysis.md)
- Priority queue implementation (covered by DESIGN-priority-queue.md)
- User notification of recalled recipes (future work)
- Rollback of library recipes with dependency analysis (this design covers tool recipes only)

## Decision Drivers

- **Auto-merge amplifies blast radius**: Automated merges can introduce many problematic recipes before detection; operational controls must enable rapid response
- **Deterministic failures are valuable data**: Failures reveal capability gaps; rollback must preserve failure records
- **Cost control is critical**: macOS CI minutes are 10x Linux ($40/month vs $400/month at scale); budget caps must prevent overruns
- **Operations readiness before scale**: Must be tested before the first batch recipe merges
- **Cloudflare-based infrastructure**: Must integrate with existing telemetry worker architecture
- **Incident response must be fast**: Operators need to stop generation and revert changes within minutes

## Considered Options

### Decision 1: Rollback Mechanism

How should we revert recipes introduced by a problematic batch run?

#### Option 1A: Git Revert Commit

Create a revert commit that removes the problematic recipe files.

**Pros:**
- Simple implementation using standard git operations
- Full audit trail in git history (original commit, revert, and any re-application all visible)
- Works with existing PR review workflow
- No additional infrastructure required

**Cons:**
- Requires identifying which commits to revert (may span multiple PRs)
- Revert PR still needs CI to pass (could be blocked by unrelated failures)
- Doesn't automatically restore previous version if one existed
- Merge conflicts possible with subsequent PRs in flight during high-frequency merging

#### Option 1B: Recipe Deletion PR

Generate a PR that deletes the recipe files, bypassing normal validation since the goal is removal.

**Pros:**
- Faster than revert (no dependency analysis)
- Can batch multiple recipe removals
- Clear intent: "removing problematic recipes"

**Cons:**
- Loses git history connection to why recipe was removed
- Still requires PR merge (even if auto-approved)
- No automatic way to restore a previous version

#### Option 1C: Soft Delete via Deprecation

Mark recipes as deprecated/invalid rather than deleting, making them uninstallable but preserving history.

**Pros:**
- Preserves full history for post-incident analysis
- Faster than git operations (metadata change only)
- Can be undone easily if rollback was unnecessary
- Can be automated in CLI (check deprecation at install time and warn users)

**Cons:**
- Requires recipe schema extension for deprecation flag
- Users still see the recipe in listings (confusing)
- Accumulates cruft if many recipes are deprecated
- Doesn't help users who already installed before deprecation

#### Option 1D: Batch ID Metadata

Add a `batch_id` field to recipes during generation. Rollback becomes a query + bulk operation.

```bash
# Find all recipes from batch 2026-01-28-001
git log --all --name-only --grep="batch_id: 2026-01-28-001"
# Generate revert commit for those files
```

**Pros:**
- Surgical rollback of exactly what a batch introduced
- No dependency analysis needed (batch already captures the cohort)
- Works with any rollback mechanism (revert, deletion, soft delete)
- Solves the "identifying which commits" problem directly

**Cons:**
- Requires adding metadata to recipe format
- Batch ID must be recorded during generation
- Commit messages must follow consistent format for grep to work

### Decision 2: Emergency Stop Mechanism

How should operators halt batch processing during an incident?

#### Option 2A: Nuclear Stop via Workflow Disable

Disable the batch workflow in GitHub repository settings. This is an escalation path, not a primary mechanism.

**Pros:**
- Immediate effect (no code deployment needed)
- GitHub native, no custom tooling
- Clear audit trail in repository settings history

**Cons:**
- Requires repository admin access (high privilege)
- All-or-nothing (disables entire workflow, not per-ecosystem)
- Manual step to re-enable after incident
- Affects ALL runs of that workflow, not just new triggers
- Cannot target a specific in-progress run
- No machine-readable state (relies on GitHub UI)

#### Option 2B: Control File in Repository

Check for a `batch-control.json` file that specifies enabled/disabled ecosystems.

```json
{
  "enabled": false,
  "disabled_ecosystems": ["homebrew"],
  "reason": "Investigating homebrew validation failures",
  "incident_url": "https://github.com/tsukumogami/tsuku/issues/1234",
  "disabled_by": "operator@example.com",
  "disabled_at": "2026-01-28T10:00:00Z",
  "expected_resume": "2026-01-28T14:00:00Z"
}
```

**Pros:**
- Fine-grained control (per-ecosystem pause)
- Self-documenting (reason, incident URL, expected resume time all recorded)
- Version-controlled change history
- Can be modified via PR or direct push
- Works in any execution environment (not just GitHub-hosted)

**Cons:**
- Requires workflow to check file at start
- Push access required (not just workflow dispatch)
- Control file must be checked early before any processing

#### Option 2C: Workflow Dispatch Input

Add workflow_dispatch inputs for emergency controls.

```yaml
on:
  workflow_dispatch:
    inputs:
      emergency_stop:
        description: 'Set true to stop after current batch completes'
        type: boolean
        default: false
      ecosystems:
        description: 'Comma-separated list of ecosystems to process (empty = all)'
        type: string
```

**Pros:**
- No code changes needed for each intervention
- Fine-grained control via inputs
- Immediate effect on next run
- Audit trail in workflow run history

**Cons:**
- Doesn't stop currently running workflow
- Requires write access to dispatch workflows
- Inputs reset to defaults on next scheduled run

#### Option 2D: Circuit Breaker Auto-Stop

Automatic pause when success rate drops below threshold. Required by upstream design (Phase 1b: "auto-pause at <50% success").

**Configuration:**
```yaml
circuit_breaker:
  threshold: 50%  # Success rate below this triggers pause
  window: 10      # Consecutive attempts to evaluate
  recovery: 60m   # Wait before retry after trip
  half_open_requests: 1  # Requests to test in half-open state
  close_on_success: 1    # Successes needed to close from half-open
```

**State transitions:**
- CLOSED → OPEN: When failures reach threshold
- OPEN → HALF-OPEN: After recovery timeout expires
- HALF-OPEN → CLOSED: Single success closes circuit
- HALF-OPEN → OPEN: Single failure reopens with fresh timeout

**Pros:**
- Automatic response (no human intervention needed)
- Proportional response (triggers on actual failure rate)
- Self-documenting (logs show why it tripped)
- Enables per-ecosystem circuit breakers when combined with Option 4B
- Already proven in codebase (`internal/llm/breaker.go`)

**Cons:**
- Requires tuning threshold per ecosystem
- May trip on transient infrastructure issues (false positives)
- Doesn't replace manual stop for security incidents

### Decision 3: Cost Control Mechanism

How should we prevent CI cost overruns?

#### Option 3A: Hard Budget Caps via Workflow Logic

Track accumulated minutes in workflow state; abort when threshold reached.

**Pros:**
- Precise control over spending
- Can adjust thresholds per ecosystem/environment
- Immediate enforcement within workflow
- Can implement gradual throttling (reduce batch size at 80%, stop at 90%)

**Cons:**
- Requires tracking state across workflow runs (non-trivial in GitHub Actions)
- Complex calculation (macOS minutes vs Linux minutes)
- May abort mid-batch leaving partial state
- State persistence requires artifacts, repository files, or external storage

#### Option 3B: GitHub Spending Limits

Use GitHub's built-in spending limits for Actions.

**Pros:**
- Native GitHub feature, no custom logic
- Applies across all workflows
- Account-level visibility and alerts

**Cons:**
- Organization-wide, not per-workflow
- Affects all CI, not just batch processing
- No per-ecosystem granularity

#### Option 3C: Sampling Strategy

Validate only a sample of recipes on expensive environments (macOS).

**Pros:**
- Deterministic cost reduction (e.g., 10% sample = 10x cost reduction)
- Still catches most environment-specific issues
- Can increase sampling for high-priority recipes
- Can be adaptive (100% initially, reduce as confidence grows)

**Cons:**
- May miss environment-specific failures
- Requires careful sample selection (not purely random)
- "Validated" claim is weaker for sampled environments
- Creates second-class validation (users on skipped environments may experience failures CI "approved")

#### Option 3D: Time-Windowed Budget

Use rolling budget windows with gradual throttling rather than hard stops.

```yaml
cost_limits:
  macos_minutes_per_week: 1000
  linux_minutes_per_week: 5000
  sampling_when_above: 80%  # Start sampling at 80% of budget
```

**Pros:**
- Allows bursting early in the week while ensuring budget compliance
- Gradual degradation instead of hard stop
- Matches existing pattern (see `.github/workflows/r2-cost-monitoring.yml` which alerts at 80%)
- More predictable cost behavior

**Cons:**
- More complex to implement than hard caps
- Requires weekly budget tracking
- May encourage front-loading work at week start

### Decision 4: SLI/SLO Approach

What metrics should we track and what thresholds trigger alerts?

#### Option 4A: Success Rate Per Batch

Track the percentage of recipes that pass validation per batch run.

**SLI:** `success_rate = passed_validations / total_attempts`
**SLO:** `success_rate >= 85%` (matches expected Homebrew deterministic rate)

**Pros:**
- Simple to calculate and understand
- Single metric captures overall batch health
- Aligns with upstream design expectations

**Cons:**
- Doesn't distinguish transient from structural failures
- Ecosystem-specific issues masked by aggregate
- 85% may be too low for some ecosystems (Cargo should be near 100%)

#### Option 4B: Per-Ecosystem Success Rates

Track success rate separately per ecosystem with ecosystem-specific thresholds.

**SLIs:**
- `cargo_success_rate`, `npm_success_rate`, etc.

**SLOs:**
- Cargo, NPM, PyPI, RubyGems, Go, CPAN: >= 98%
- Homebrew: >= 85%
- Cask: >= 95%

**Pros:**
- Catches ecosystem-specific regressions
- Thresholds match expected behavior per ecosystem
- Enables per-ecosystem circuit breakers (pause Homebrew only while Cargo continues)
- Can identify ecosystem-specific infrastructure issues

**Cons:**
- More metrics to track and alert on
- Requires understanding of each ecosystem's expected rate
- Alert fatigue risk with many thresholds

#### Option 4C: Multi-Metric Dashboard

Track multiple SLIs with severity-based alerting:

- **Critical**: Any ecosystem drops below 50% (indicates systemic failure)
- **Warning**: Ecosystem below expected threshold but above 50%
- **Info**: Validation latency exceeds 2 minutes per recipe

**Pros:**
- Severity levels prevent alert fatigue
- Multiple signals for better diagnosis
- Latency metric catches infrastructure issues

**Cons:**
- More complex alerting infrastructure
- Requires baseline for "expected" rates
- More operator training needed

### Decision 5: Operational Data Storage

Where should we store operational state (control files, metrics history)?

#### Option 5A: Repository Files

Store control files and metrics in the repository itself.

**Pros:**
- Version controlled with full history
- No additional infrastructure
- PRs provide change review

**Cons:**
- Frequent commits pollute history
- Race conditions with concurrent workflows
- Not queryable for historical analysis

#### Option 5B: D1 Database (Cloudflare)

Store operational data in D1, extending the failure analysis backend.

**Pros:**
- Queryable for historical trends
- Matches existing Cloudflare architecture
- Supports concurrent access
- Can power dashboards

**Cons:**
- Adds dependency on external service
- Schema migrations needed over time
- Network latency for control file checks

#### Option 5C: Hybrid

Control files in repository, metrics in D1.

**Pros:**
- Control files are version-controlled (audit trail)
- Metrics are queryable (trending, dashboards)
- Separation of concerns
- Control files work without network dependency

**Cons:**
- Two systems to maintain
- Split-brain risk if control file says "resume" but D1 says "paused" (needs reconciliation logic)

## Assumptions

These assumptions are explicit design constraints:

1. **Single operator model**: This design assumes single-operator incidents. Multi-operator coordination is out of scope for v1.

2. **GitHub Actions execution**: Batch processing executes exclusively in GitHub Actions. Self-hosted runners or local execution not supported.

3. **Tool recipes only**: Rollback of library recipes with dependencies requires additional analysis. This design covers tool recipes that can be reverted independently.

4. **Detection before harm**: Emergency stop prevents new harm but doesn't remediate existing harm. Users who installed before detection keep the problematic version.

## Uncertainties

- **Git revert complexity**: We haven't validated how to efficiently identify commits from a specific batch run for rollback (Option 1D addresses this)
- **Sampling effectiveness**: Unknown what percentage of environment-specific failures would be caught by 10% sampling
- **GitHub spending limit behavior**: Unclear how GitHub handles limit reached mid-workflow (may leave partial state, blocking gate for Option 3B)
- **D1 query latency**: Unknown if D1 latency is acceptable for pre-workflow control checks

## Options Summary

| Decision | Options |
|----------|---------|
| 1. Rollback Mechanism | Git revert, Deletion PR, Soft delete, **Batch ID metadata** |
| 2. Emergency Stop | Nuclear (workflow disable), Control file, Dispatch inputs, **Circuit breaker** |
| 3. Cost Control | Workflow logic, GitHub limits, Sampling, **Time-windowed budget** |
| 4. SLI/SLO | Per-batch rate, Per-ecosystem rates, Multi-metric |
| 5. Data Storage | Repository, D1, Hybrid |

## Decision Outcome

### Decision 1: Rollback Mechanism

**Chosen:** Option 1D (Batch ID Metadata) + Option 1A (Git Revert)

**Rationale:**
- Batch ID metadata solves the core problem of "identifying which commits to revert" that all other options struggle with
- Git revert provides the actual rollback mechanism with full audit trail
- Together they enable surgical rollback: query by batch ID, then revert those specific files
- No schema changes needed (batch ID goes in commit message, not recipe format)

**Implementation:**
- Commit messages include `batch_id: YYYY-MM-DD-NNN` format
- Rollback script: `git log --grep="batch_id: X" --name-only | xargs git rm`
- Revert PR generated automatically with validation bypassed for removals

### Decision 2: Emergency Stop Mechanism

**Chosen:** Option 2D (Circuit Breaker) + Option 2B (Control File)

**Rationale:**
- Circuit breaker provides automatic response (required by upstream design Phase 1b)
- Control file provides manual override for security incidents or circuit breaker tuning
- Workflow disable (Option 2A) remains available as nuclear escalation but isn't primary
- Dispatch inputs (Option 2C) don't persist across scheduled runs, making them unsuitable

**Implementation:**
- Circuit breaker: 50% threshold, 10-attempt window, per-ecosystem
- Control file: `batch-control.json` at repo root, checked before each ecosystem
- Circuit breaker state persisted in control file for visibility

### Decision 3: Cost Control Mechanism

**Chosen:** Option 3D (Time-Windowed Budget) + Option 3C (Sampling)

**Rationale:**
- Time-windowed budget matches existing pattern (R2 cost monitoring uses 80% threshold alerts)
- Sampling provides graceful degradation when approaching budget limits
- GitHub spending limits (Option 3B) affect all CI, not just batch processing
- Hard caps (Option 3A) require complex state management

**Implementation:**
- Weekly budget: 1000 macOS minutes, 5000 Linux minutes
- At 80% usage: reduce batch size and start sampling macOS validation
- At 95% usage: pause new batches until next week
- Budget tracked via GitHub Actions artifacts (simple, no external dependency)

### Decision 4: SLI/SLO Approach

**Chosen:** Option 4B (Per-Ecosystem Success Rates) + severity levels from 4C

**Rationale:**
- Per-ecosystem rates enable targeted response (pause Homebrew while Cargo continues)
- Severity levels prevent alert fatigue while ensuring critical issues surface
- Aggregate rate (Option 4A) masks ecosystem-specific problems
- Thresholds derived from upstream design expectations (85% Homebrew, 98% others)

**Implementation:**
- SLIs per ecosystem: `{ecosystem}_success_rate`
- SLO thresholds: Homebrew >= 85%, Cask >= 95%, others >= 98%
- Alerting: Critical at <50% (systemic failure), Warning at <SLO, Info for latency

### Decision 5: Operational Data Storage

**Chosen:** Option 5C (Hybrid) with repository-primary design

**Rationale:**
- Control file in repository: version-controlled, works without network, audit trail
- Metrics in D1: queryable for trending, powers dashboards
- Repository-primary: if D1 unavailable, control file is authoritative
- Avoids split-brain by making repository the source of truth

**Implementation:**
- `batch-control.json`: emergency stop, circuit breaker state, disabled ecosystems
- D1 database: batch run metrics, per-ecosystem success rates, cost tracking
- Reconciliation: workflow reads control file first, D1 metrics are advisory

## Solution Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Batch Pipeline Workflow                       │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────────────┐ │
│  │ Pre-flight   │──▶│ Batch        │──▶│ Post-batch           │ │
│  │ Checks       │   │ Processing   │   │ Reporting            │ │
│  └──────────────┘   └──────────────┘   └──────────────────────┘ │
│         │                  │                      │              │
│         ▼                  ▼                      ▼              │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────────────┐ │
│  │ Control File │   │ Circuit      │   │ D1 Metrics           │ │
│  │ Check        │   │ Breaker      │   │ Upload               │ │
│  └──────────────┘   └──────────────┘   └──────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Control Plane (Repository)                    │
├─────────────────────────────────────────────────────────────────┤
│  batch-control.json                                              │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ {                                                            ││
│  │   "enabled": true,                                           ││
│  │   "disabled_ecosystems": [],                                 ││
│  │   "circuit_breaker": {                                       ││
│  │     "homebrew": { "state": "closed", "failures": 0 },        ││
│  │     "cargo": { "state": "closed", "failures": 0 }            ││
│  │   },                                                         ││
│  │   "budget": { "macos_minutes_used": 450, "week_start": "..." }│
│  │ }                                                            ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

### Control File Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "enabled": {
      "type": "boolean",
      "description": "Master switch for batch processing"
    },
    "disabled_ecosystems": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Ecosystems to skip (manual pause)"
    },
    "reason": {
      "type": "string",
      "description": "Why batch is paused (for incident tracking)"
    },
    "incident_url": {
      "type": "string",
      "format": "uri",
      "description": "Link to incident issue"
    },
    "disabled_by": {
      "type": "string",
      "description": "Operator who paused"
    },
    "disabled_at": {
      "type": "string",
      "format": "date-time"
    },
    "expected_resume": {
      "type": "string",
      "format": "date-time"
    },
    "circuit_breaker": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "properties": {
          "state": { "enum": ["closed", "open", "half-open"] },
          "failures": { "type": "integer" },
          "last_failure": { "type": "string", "format": "date-time" },
          "opens_at": { "type": "string", "format": "date-time" }
        }
      }
    },
    "budget": {
      "type": "object",
      "properties": {
        "macos_minutes_used": { "type": "integer" },
        "linux_minutes_used": { "type": "integer" },
        "week_start": { "type": "string", "format": "date-time" },
        "sampling_active": { "type": "boolean", "description": "True when budget-triggered sampling is in effect" }
      }
    }
  }
}
```

### Workflow Integration

#### Pre-flight Checks

```yaml
jobs:
  pre-flight:
    runs-on: ubuntu-latest
    outputs:
      can_proceed: ${{ steps.check.outputs.can_proceed }}
      ecosystems: ${{ steps.check.outputs.ecosystems }}
    steps:
      - uses: actions/checkout@v4

      - name: Check control file
        id: check
        run: |
          if [ -f batch-control.json ]; then
            enabled=$(jq -r '.enabled // true' batch-control.json)
            if [ "$enabled" = "false" ]; then
              echo "can_proceed=false" >> $GITHUB_OUTPUT
              echo "::warning::Batch processing disabled: $(jq -r '.reason' batch-control.json)"
              exit 0
            fi

            # Filter out disabled ecosystems
            disabled=$(jq -r '.disabled_ecosystems // [] | join(",")' batch-control.json)
            echo "disabled_ecosystems=$disabled" >> $GITHUB_OUTPUT
          fi

          echo "can_proceed=true" >> $GITHUB_OUTPUT
```

#### Circuit Breaker Logic

```yaml
- name: Check circuit breaker
  id: breaker
  run: |
    ecosystem="${{ matrix.ecosystem }}"
    state=$(jq -r ".circuit_breaker.${ecosystem}.state // \"closed\"" batch-control.json)

    if [ "$state" = "open" ]; then
      opens_at=$(jq -r ".circuit_breaker.${ecosystem}.opens_at" batch-control.json)
      if [ "$(date -u +%s)" -lt "$(date -d "$opens_at" +%s)" ]; then
        echo "skip=true" >> $GITHUB_OUTPUT
        echo "::warning::Circuit breaker open for $ecosystem until $opens_at"
        exit 0
      fi
      # Try half-open
      echo "state=half-open" >> $GITHUB_OUTPUT
    fi

    echo "skip=false" >> $GITHUB_OUTPUT
```

#### Post-Batch Update

```yaml
- name: Update circuit breaker state
  if: always()
  run: |
    ecosystem="${{ matrix.ecosystem }}"
    success="${{ steps.validate.outcome == 'success' }}"

    if [ "$success" = "true" ]; then
      # Reset failures on success
      jq ".circuit_breaker.${ecosystem}.failures = 0 |
          .circuit_breaker.${ecosystem}.state = \"closed\"" \
          batch-control.json > tmp.json && mv tmp.json batch-control.json
    else
      # Increment failures
      failures=$(jq ".circuit_breaker.${ecosystem}.failures + 1" batch-control.json)
      if [ "$failures" -ge 5 ]; then
        # Trip circuit breaker
        opens_at=$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)
        jq ".circuit_breaker.${ecosystem}.state = \"open\" |
            .circuit_breaker.${ecosystem}.opens_at = \"$opens_at\" |
            .circuit_breaker.${ecosystem}.failures = $failures" \
            batch-control.json > tmp.json && mv tmp.json batch-control.json
      else
        jq ".circuit_breaker.${ecosystem}.failures = $failures" \
            batch-control.json > tmp.json && mv tmp.json batch-control.json
      fi
    fi

    git add batch-control.json
    git commit -m "chore: update circuit breaker state [skip ci]" || true

    # Push with retry for concurrent updates (self-healing: occasional loss is acceptable)
    for i in 1 2 3; do
      git pull --rebase origin main && git push && break
      sleep $((i * 2))
    done || echo "::warning::Control file update failed, will self-heal on next run"
```

### Rollback Script

```bash
#!/bin/bash
# scripts/rollback-batch.sh
# Usage: ./rollback-batch.sh 2026-01-28-001

set -euo pipefail

BATCH_ID="$1"

if [ -z "$BATCH_ID" ]; then
  echo "Usage: $0 <batch_id>"
  echo "Example: $0 2026-01-28-001"
  exit 1
fi

# Find all recipes from this batch
echo "Finding recipes from batch $BATCH_ID..."
files=$(git log --all --name-only --grep="batch_id: $BATCH_ID" --format="" |
        grep '^recipes/' | sort -u)

if [ -z "$files" ]; then
  echo "No recipes found for batch $BATCH_ID"
  exit 1
fi

echo "Found $(echo "$files" | wc -l) recipes to rollback:"
echo "$files"

# Create rollback branch
branch="rollback-batch-$BATCH_ID"
git checkout -b "$branch"

# Remove the files
echo "$files" | xargs git rm

# Commit
git commit -m "chore: rollback batch $BATCH_ID

batch_id: $BATCH_ID
rollback_reason: manual rollback request
"

echo ""
echo "Rollback branch created: $branch"
echo "Review changes with: git diff main...$branch"
echo "Create PR with: gh pr create --title 'Rollback batch $BATCH_ID'"
```

### D1 Schema (Metrics)

```sql
-- Batch run metrics
CREATE TABLE batch_runs (
  id TEXT PRIMARY KEY,
  batch_id TEXT NOT NULL,
  ecosystem TEXT NOT NULL,
  started_at TEXT NOT NULL,
  completed_at TEXT,
  total_recipes INTEGER,
  passed INTEGER,
  failed INTEGER,
  skipped INTEGER,
  success_rate REAL,
  macos_minutes INTEGER,
  linux_minutes INTEGER
);

CREATE INDEX idx_batch_runs_batch_id ON batch_runs(batch_id);
CREATE INDEX idx_batch_runs_ecosystem ON batch_runs(ecosystem);
CREATE INDEX idx_batch_runs_started_at ON batch_runs(started_at);

-- Per-recipe results for failure analysis
CREATE TABLE recipe_results (
  id TEXT PRIMARY KEY,
  batch_run_id TEXT REFERENCES batch_runs(id),
  recipe_name TEXT NOT NULL,
  ecosystem TEXT NOT NULL,
  result TEXT NOT NULL, -- 'passed', 'failed', 'skipped'
  error_category TEXT,  -- 'validation', 'network', 'timeout', etc.
  error_message TEXT,
  duration_seconds INTEGER
);

CREATE INDEX idx_recipe_results_batch ON recipe_results(batch_run_id);
CREATE INDEX idx_recipe_results_result ON recipe_results(result);
```

### Runbook Requirements

Each operational procedure must follow the R2 runbook template:

1. **Decision Tree**: When to investigate vs when to wait
2. **Step-by-Step Procedure**: Commands with expected output
3. **Diagnostics**: How to determine root cause
4. **Escalation**: When to involve additional operators

Example runbook structure for "Batch Success Rate Drop":

```markdown
## Symptom: Batch success rate dropped below SLO

### Decision Tree
- If single ecosystem affected → Check ecosystem-specific issues
- If all ecosystems affected → Check infrastructure
- If during deployment window → Likely deployment issue

### Investigation Steps
1. Check which ecosystem(s) dropped:
   \`\`\`
   jq '.circuit_breaker | to_entries | map(select(.value.failures > 0))' batch-control.json
   \`\`\`

2. Review recent failures in D1:
   \`\`\`
   SELECT ecosystem, error_category, COUNT(*)
   FROM recipe_results
   WHERE result = 'failed'
   AND batch_run_id = (SELECT id FROM batch_runs ORDER BY started_at DESC LIMIT 1)
   GROUP BY ecosystem, error_category
   \`\`\`

3. If network errors dominate → check upstream API status
4. If validation errors dominate → check recent CI changes

### Resolution
- Transient: Wait for circuit breaker recovery
- Persistent: Pause ecosystem via control file, investigate

### Escalation
- If >50% recipes failing across ecosystems: Page on-call
- If security-related failure: Escalate immediately
```

## Implementation Approach

### Phase 1: Control Plane (Required before batch pipeline)

1. Create `batch-control.json` with initial schema
2. Add pre-flight check step to batch workflow template
3. Implement circuit breaker state transitions
4. Create `scripts/rollback-batch.sh`

### Phase 2: Metrics Infrastructure

1. Deploy D1 schema for batch_runs and recipe_results tables
2. Add post-batch metrics upload step to workflow
3. Configure alerting thresholds per ecosystem

### Phase 3: Integration Testing

1. Test rollback script against synthetic batch
2. Validate circuit breaker trips correctly at 50% threshold
3. Verify budget tracking resets weekly
4. Dry-run emergency stop procedures

### Dependencies

- Batch pipeline workflow (DESIGN-batch-recipe-generation.md) must integrate control file checks
- D1 database requires Cloudflare account setup (existing telemetry account can be extended)
- Runbook documentation follows R2 runbook template

## Security Considerations

### Access Control

| Action | Required Access | Justification |
|--------|-----------------|---------------|
| View batch-control.json | Read (public repo) | Transparency for contributors |
| Modify batch-control.json | Write (push) | Operators need quick response |
| Trigger rollback script | Write (push) | Rollback creates commits |
| Disable workflow | Admin | Nuclear option, high privilege |
| Query D1 metrics | API token | Metrics are operational data |

**Principle of least privilege:**
- Operators should have write access, not admin
- Admin access reserved for workflow disable (escalation only)
- D1 query access separate from control file modification

### Attack Vectors

#### 1. Malicious Control File Modification

**Threat:** Attacker with write access disables batch processing or re-enables a paused ecosystem prematurely.

**Mitigations:**
- Control file changes create git history (audit trail)
- Require PR for control file changes in non-emergency situations
- GitHub branch protection can require review for `batch-control.json`
- Alert on control file changes via GitHub notification

#### 2. Circuit Breaker Bypass

**Threat:** Attacker modifies circuit breaker state to force processing despite failures.

**Mitigations:**
- Circuit breaker state in version-controlled file
- Workflow validates state transitions (can't go from "open" to "closed" without half-open)
- Anomaly detection: alert if state changes without corresponding batch run

#### 3. Batch ID Spoofing

**Threat:** Attacker creates fake batch ID to include malicious recipes in rollback.

**Mitigations:**
- Batch IDs generated by CI, not user input
- Rollback script validates batch ID format (YYYY-MM-DD-NNN)
- Rollback script shows files before deletion (operator review)
- PR required for rollback merge

#### 4. D1 Data Tampering

**Threat:** Attacker modifies metrics in D1 to hide failures or trigger false alerts.

**Mitigations:**
- D1 write access restricted to CI workflow
- Metrics are advisory only; control decisions use repository file
- Anomaly detection on metric patterns (sudden jumps, impossible values)

#### 5. Denial of Service via Budget Exhaustion

**Threat:** Attacker triggers excessive batch runs to exhaust CI budget.

**Mitigations:**
- Scheduled runs only (no workflow_dispatch for full batches)
- Time-windowed budget prevents single-week exhaustion
- Alert at 80% budget before hard stop

### Post-Merge Monitoring (Security)

Per upstream design, monitor for checksum drift indicating compromised upstream:

```yaml
- name: Verify checksums haven't drifted
  run: |
    # For each recipe merged today, re-fetch and compare
    for recipe in $(git diff --name-only HEAD~1 | grep '^recipes/'); do
      current_checksum=$(tomlq -r '.install.checksum' "$recipe")
      tool=$(basename "$recipe" .toml)
      new_checksum=$(tsuku fetch-checksum "$tool")

      if [ "$current_checksum" != "$new_checksum" ]; then
        echo "::error::Checksum drift detected for $tool"
        echo "Expected: $current_checksum"
        echo "Got: $new_checksum"
        # Create security incident issue
        gh issue create --title "Security: Checksum drift for $tool" \
          --body "Merged recipe checksum differs from current upstream." \
          --label "security,needs-triage"
      fi
    done
```

### Incident Classification

| Severity | Criteria | Response Time | Example |
|----------|----------|---------------|---------|
| Critical | Security compromise suspected | < 1 hour | Checksum drift, supply chain |
| High | Batch affecting >50% recipes | < 4 hours | Validation pipeline bug |
| Medium | Single ecosystem degraded | < 24 hours | Homebrew API down |
| Low | Transient failures | Next batch | Network timeout |

### Audit Trail Requirements

All operational actions must be auditable:

1. **Control file changes**: Git history with author, timestamp, message
2. **Rollback execution**: PR with batch ID reference
3. **Circuit breaker trips**: Logged in control file with timestamp
4. **Budget consumption**: Tracked in control file and D1

Retention: Git history (permanent), D1 metrics (90 days minimum)

## Consequences

### Positive

- **Rapid incident response**: Circuit breaker provides automatic protection; control file enables manual override within minutes
- **Surgical rollback**: Batch IDs enable reverting exactly what a batch introduced without affecting other recipes
- **Cost predictability**: Time-windowed budgets with graceful degradation prevent surprise overruns
- **Observability**: Per-ecosystem SLIs reveal targeted issues rather than masking them in aggregates
- **Audit trail**: All operational actions are traceable through git history and D1 metrics

### Negative

- **Operational complexity**: Two storage systems (repo + D1) require understanding both
- **Eventual consistency**: Circuit breaker state may occasionally be stale due to concurrent updates (self-healing)
- **Learning curve**: Operators must understand circuit breaker states, control file format, and runbook procedures

### Neutral

- **Repository-primary design**: Prioritizes availability over features (no dashboards without D1, but control always works)
- **Batch ID requirement**: All batch commits must follow format for rollback to work; creates convention dependency
