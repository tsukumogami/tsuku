---
status: Proposed
problem: |
  Auto-updates produce no outcome telemetry. Successful auto-applies reuse the
  existing install flow which fires a generic update event with no outcome field.
  Failures write local notices only. Rollbacks are invisible. There is no way to
  measure update reliability, failure rates, or rollback frequency at scale.
decision: |
  TBD
rationale: |
  TBD
upstream: docs/prds/PRD-auto-update.md
---

# DESIGN: Update Outcome Telemetry

## Status

Proposed

## Context and Problem Statement

The auto-update system (Features 1-4 of the auto-update roadmap) can now check for
updates, apply them within pin boundaries, roll back on failure, and self-update the
tsuku binary. But the telemetry system hasn't kept pace. Today's `NewUpdateEvent`
fires only on successful updates and carries no outcome field -- it's structurally
identical whether the update was manual or automatic.

Failures and rollbacks are tracked locally via the notices system
(`$TSUKU_HOME/notices/`) but never leave the machine. This means:

- No visibility into what percentage of auto-updates succeed vs fail
- No data on which tools or versions cause failures
- No way to detect if a specific upstream release is breaking auto-updates across users
- No understanding of how often rollback is triggered

PRD requirement R22 calls for extending the existing telemetry system with
success/failure/rollback outcomes. The telemetry worker, stats API, and dashboard
all need updates to receive, store, and surface this data.

### Scope

**In scope:**
- CLI event struct and emission points for update outcomes
- Telemetry worker validation, dispatch, and blob layout
- Stats API endpoint for update outcome data
- Dashboard section for update reliability metrics
- Respect for existing opt-out mechanisms

**Out of scope:**
- Changing auto-apply behavior based on telemetry (feedback loops)
- Per-user or per-machine tracking
- Alerting on failure spikes
- Modifying the existing successful update event path

## Decision Drivers

- Follow established telemetry patterns (separate struct per event category, dedicated send method, action prefix dispatch)
- Analytics Engine constraint: 20 blobs max per data point
- Fire-and-forget: telemetry must never block or slow down updates, rollbacks, or any user-facing operation
- Schema version coordination between CLI and worker
- Full pipeline: CLI emission, backend processing/storage, dashboard consumption
- Privacy: no PII, no tool paths, no error messages that could contain filesystem details
