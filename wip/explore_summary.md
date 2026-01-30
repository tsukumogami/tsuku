# Exploration Summary: seed-queue-pipeline

## Problem (Phase 1)
The priority queue (`data/priority-queue.json`) doesn't exist in the repo yet, and the seed script only supports Homebrew. The batch generation pipeline has nothing to process without this data, and popularity data changes over time so one-time seeding isn't enough.

## Decision Drivers (Phase 1)
- Existing seed script pattern (seed-queue.sh) should be extended, not replaced
- Must validate output against priority-queue.schema.json
- Additive merging: don't remove packages already being processed
- Start with workflow_dispatch, add cron later
- Each ecosystem has a different popularity API
- Pipeline must handle API failures gracefully

## Research Findings (Phase 2)
- seed-queue.sh exists for Homebrew with retry logic, curated tier 1 list, download-count tier 2 threshold
- Schema supports source field for multi-ecosystem packages
- batch-operations.yml shows existing CI patterns: pre-flight checks, circuit breaker, control file
- No data/priority-queue.json exists yet
- Ecosystem APIs vary: some have download counts, Go has no public popularity API

## Options (Phase 3)
- Option A: Extend seed-queue.sh with multi-ecosystem support
- Option B: Separate scripts per ecosystem, orchestrated by workflow

## Decision (Phase 5)
**Problem:** The batch generation pipeline needs a populated priority queue with packages from multiple ecosystems, but only a Homebrew seed script exists and no workflow runs it.
**Decision:** Extend seed-queue.sh with pluggable ecosystem sources and create a GitHub Actions workflow to run it, merging results additively into priority-queue.json.
**Rationale:** Extending the existing script preserves the proven retry logic and tier assignment pattern. A single entry point with --source flags keeps the workflow simple while allowing ecosystem-specific fetch logic internally.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-01-30
