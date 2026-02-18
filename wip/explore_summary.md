# Exploration Summary: unified-batch-pipeline

## Problem (Phase 1)
The batch pipeline runs hourly but processes zero packages because `selectCandidates()` filters by ecosystem prefix (`homebrew:`) while 261 re-routed packages now have non-homebrew sources (`github:`, `cargo:`, etc.). The pipeline, workflow, circuit breaker, and dashboard all assume single-ecosystem batches despite the unified queue being ecosystem-agnostic by design.

## Decision Drivers (Phase 1)
- Minimal change: remove the ecosystem filter without rebuilding the pipeline
- Preserve rate limiting: different APIs have different rate limits
- Preserve circuit breaker: failures should still trip breakers per-ecosystem
- Dashboard continuity: ecosystem remains useful metadata for display/filtering
- No new CI costs: same hourly budget, same concurrency

## Research Findings (Phase 2)
- Upstream DESIGN-pipeline-dashboard-2.md explicitly says "batch generation uses the source directly" and "multi-ecosystem coverage: unified queue naturally includes packages from all ecosystems"
- #1699 acceptance criteria says "use pkg.Source directly" but never mentions ecosystem filtering in selectCandidates
- The ecosystem prefix filter in selectCandidates() and the workflow's homebrew default are vestiges from pre-unified-queue era
- ecosystem is not stored in queue entries -- it's derived at runtime from Source prefix via QueueEntry.Ecosystem()
- Circuit breaker, rate limits, failure files, batch IDs, PR labels all use ecosystem
- Dashboard renders ecosystem as metadata for filtering and display

## Options (Phase 3)
- Option A: Remove ecosystem filter, derive per-entry (minimal orchestrator change)
- Option B: Remove ecosystem concept entirely (full refactor)
- Option C: Run 8 separate cron schedules (keep ecosystem filter, add more triggers)

## Decision (Phase 5)

**Problem:**
The batch pipeline generates zero recipes because the orchestrator filters queue entries by
ecosystem prefix, but 261 packages were re-routed to non-homebrew sources during Bootstrap
Phase B. The cron trigger defaults to homebrew, so re-routed packages are invisible. The
upstream design explicitly intended the unified queue to enable multi-ecosystem coverage in
a single batch run, but the ecosystem filter in selectCandidates() and the workflow's
-ecosystem flag were never cleaned up.

**Decision:**
Remove the ecosystem prefix filter from selectCandidates() so it processes all pending entries
regardless of source ecosystem. Derive ecosystem per-entry from QueueEntry.Ecosystem() for rate
limiting and circuit breaker checks. The workflow drops its -ecosystem flag for cron runs and
the orchestrator handles mixed-ecosystem batches natively. Dashboard and failure files continue
recording ecosystem as metadata derived from the source, not from a batch-level config.

**Rationale:**
This is the simplest fix that fulfills the upstream design's intent. The unified queue already
has pre-resolved sources; the only gap was that selectCandidates() still filtered by ecosystem
prefix. Keeping ecosystem as derived per-entry metadata preserves rate limiting, circuit breaker
safety, and dashboard filtering without requiring a full pipeline rebuild. The alternative of
running 8 separate cron jobs adds CI cost and doesn't solve the core problem of mixed-source
batches.

## Architecture and Security Review (Phase 8)

Key feedback incorporated:
- Added `PerEcosystem map[string]EcosystemResult` for per-ecosystem success/failure breakdown
- Added half-open breaker limiting (1 probe entry per half-open ecosystem)
- Added `FilterEcosystem` for manual dispatch debugging
- Added default rate limit for unknown ecosystems, `github` entry at 2s
- Orchestrator reads `batch-control.json` directly (no new CLI flag)
- Added ecosystem prefix validation for path-safety
- Added "dispatch per ecosystem" alternative with rejection
- Strengthened "optional config" rejection rationale
- Noted Phases 1+2 must ship together
- Specified `data/batch-results.json` output path
- Added failure file grouping pseudocode

## Current Status
**Phase:** 8 - Final review complete, ready for commit
**Last Updated:** 2026-02-17
