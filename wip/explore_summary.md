# Exploration Summary: requeue-on-recipe-merge

## Problem (Phase 1)
Blocked packages in the batch pipeline queue aren't unblocked when their dependency recipes are merged. The requeue logic only runs during scheduled batch runs (hourly), leaving a latency gap.

## Decision Drivers (Phase 1)
- The requeue script has a format mismatch with the unified queue
- `cmd/reorder-queue/` (merged via PR #1820) needs to run alongside requeue
- `update-queue-status.yml` already triggers on recipe merges and is the natural integration point
- Both operations share the same data sources (failure JSONL + unified queue)

## Research Findings (Phase 2)
- `requeue-unblocked.sh` reads per-ecosystem queue (`.packages[]`, `.id`), not unified queue (`.entries[]`, `.name`)
- `internal/reorder/` already has Go code to load failures and compute blocker maps from the same JSONL files
- `internal/blocker/` provides shared transitive blocker computation
- `batch.LoadUnifiedQueue` and `batch.SaveUnifiedQueue` handle the unified queue format
- The format mismatch is documented in DESIGN-registry-scale-strategy.md as a known gap

## Current Status
**Phase:** 3 - Options
**Last Updated:** 2026-02-21
