# Design Summary: update-outcome-telemetry

## Input Context (Phase 0)
**Source:** Issue #2189 (feat(update): update outcome telemetry)
**Upstream:** PRD-auto-update.md (R22)
**Roadmap:** ROADMAP-auto-update.md (Feature 9)
**Problem:** Auto-updates have no outcome telemetry. Success, failure, and rollback events are invisible at scale. The full pipeline (CLI, worker, dashboard) needs updates.
**Constraints:** Follow existing telemetry patterns, Analytics Engine 20-blob limit, fire-and-forget, no PII

## Decisions (Phase 2)
1. **Event data model**: Separate UpdateOutcomeEvent struct (high confidence)
2. **Emission architecture**: Emit from MaybeAutoApply directly (high confidence)
3. **Consumption pipeline**: New /stats/updates endpoint (high confidence)

## Cross-Validation (Phase 3)
Passed. Minor tensions resolved: success events use new struct alongside existing Event; error taxonomy naming aligned.

## Security Review (Phase 5)
**Outcome:** No blocking concerns
**Summary:** Design introduces no new attack surface. classifyError() is the critical privacy boundary -- needs unit tests. Opt-out compliance needs verification test.

## Current Status
**Phase:** 5 - Security (complete)
**Last Updated:** 2026-04-01
