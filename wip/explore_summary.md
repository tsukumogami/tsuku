# Exploration Summary: Pipeline Dashboard

## Problem (Phase 1)

The batch recipe generation pipeline collects data about failures and queue status, but there's no way to visualize pipeline health without reading raw JSON files or waiting for the full failure analysis backend (#1190).

## Decision Drivers (Phase 1)

- No server or database required - data lives in repo
- Must work with existing data formats (priority-queue.json, homebrew.jsonl, batch-runs.jsonl)
- Immediate value - should be usable before #1190 backend exists
- Simple to maintain - minimal dependencies

## Research Findings (Phase 2)

**Available Data Sources:**
1. `data/priority-queue.json` - 1000+ packages with status (pending/failed/blocked/success), tier, timestamps
2. `data/failures/homebrew.jsonl` - JSONL with failure records (category, blocked_by, message)
3. `data/metrics/batch-runs.jsonl` - Per-run SLI data (PR #1422, not yet merged)

**Existing Patterns:**
- Shell scripts use: bash + jq + awk + standard Unix tools
- Website uses: static HTML + vanilla JS, no build step, no framework
- Data generation: Python scripts (generate-registry.py) produce JSON from TOML
- Stats page (`website/stats/index.html`) has CSS bar charts, card layouts, responsive design

**Constraints from Upstream:**
- Issue #1190 owns the full backend - this design should complement, not duplicate
- Gap analysis script exists (`scripts/gap-analysis.sh`) for CLI usage
- Batch metrics script exists (`scripts/batch-metrics.sh`) for run summaries

## Options (Phase 3)

1. **Visualization**: Static HTML dashboard + CLI scripts (chosen) vs CLI-only vs real-time
2. **Processing**: Shell/jq (chosen) vs Python vs Go
3. **Updates**: CI-generated on pipeline run (chosen) vs manual vs cron
4. **Content**: Four-panel layout (queue, blockers, categories, runs) vs minimal vs comprehensive

## Decision (Phase 5)

**Problem:**
The batch recipe generation pipeline collects structured data about failures, queue status, and run metrics, but there's no way to visualize this data without manually reading JSON files. Operators need quick visibility into pipeline health - which packages are failing, why they're failing, and whether the pipeline is making progress over time.

Issue #1190 designs a full backend with Cloudflare D1 for failure analysis, but that requires significant infrastructure. Meanwhile, all the necessary data already exists in the repository. An intermediate solution using only repo-resident data would provide immediate value without infrastructure dependencies.

**Decision:**
Implement a static HTML dashboard at `website/pipeline/` that displays queue status with tier breakdown, top blocking dependencies, failure categories, and recent batch runs. A Go tool (`internal/dashboard/`) processes the existing JSON/JSONL data files and outputs a `dashboard.json` file that the HTML page fetches.

The dashboard is regenerated automatically during each batch pipeline run, ensuring data freshness without manual intervention. The implementation follows existing patterns: Go for data processing (like `internal/seed/`), vanilla JavaScript for the frontend, no framework or build step.

**Rationale:**
Go tooling aligns with established internal patterns (seed, queue packages). The two JSONL record formats benefit from type-safe parsing with proper error handling. Static HTML matches the website's existing architecture. CI-triggered generation ensures data is always current after batch runs without operator intervention.

This is an intermediate solution that provides immediate value while #1190 (full failure analysis backend) is developed. The simple architecture means low maintenance burden and fast iteration on what visualizations are most useful.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-03
