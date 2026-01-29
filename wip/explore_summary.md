# Exploration Summary: Batch Operations

## Problem (Phase 1)

The registry scale strategy introduces automated batch generation that will auto-merge recipes into the registry. Without operational controls, rollback procedures, and emergency stop mechanisms, operators have no way to respond to incidents when they occur. A compromised recipe, runaway generation, or validation bug could affect the entire registry with no defined recovery path.

## Decision Drivers (Phase 1)

- **Auto-merge amplifies blast radius**: Unlike manual PRs, automated merges can introduce many problematic recipes before anyone notices
- **Deterministic failures are valuable data**: Failures reveal capability gaps; operational systems must preserve failure records during rollback
- **Cost control is critical**: macOS CI minutes are 10x Linux; uncontrolled batch runs can exceed budgets
- **Operations readiness before scale**: The upstream design explicitly requires operations designs before the batch pipeline can safely operate
- **Cloudflare-based infrastructure**: Must integrate with existing telemetry worker architecture (Worker + D1/R2)
- **Incident response must be fast**: Operators need to stop generation and revert changes within minutes, not hours

## Research Findings (Phase 2)

**Upstream Design Constraints (from DESIGN-registry-scale-strategy.md):**
- Phase 0: Rollback scripts tested against manually-created recipes
- Phase 1b: SLIs, circuit breaker (auto-pause at <50% success), rate limiting
- Phase 2: Cloudflare Worker + D1 for failure storage, cost caps, observability
- Phase 4: Post-merge monitoring for checksum drift detection

**Existing Codebase Patterns:**
- **Telemetry Worker** (`telemetry/src/index.ts`): Cloudflare Analytics Engine with blob-based schema versioning
- **Circuit Breaker** (`internal/llm/breaker.go`): 3 failures threshold, 60s recovery timeout, telemetry callback
- **State Locking** (`internal/install/state.go`): File-level exclusive locking for atomic updates
- **Error Classification** (`internal/version/errors.go`): Transient vs structural error types
- **R2 Health Monitoring**: Pre-flight checks, degraded state handling, issue automation

## Decisions (Phase 5)

| Decision | Chosen Option | Rationale |
|----------|---------------|-----------|
| 1. Rollback | Batch ID metadata + Git revert | Batch IDs solve commit identification; git revert provides audit trail |
| 2. Emergency Stop | Circuit breaker + Control file | Auto + manual response; required by upstream |
| 3. Cost Control | Time-windowed budget + Sampling | Matches existing pattern; graceful degradation |
| 4. SLI/SLO | Per-ecosystem rates + severity levels | Targeted response; prevents alert fatigue |
| 5. Data Storage | Hybrid (repository-primary) | Version-controlled control plane; queryable metrics |

## Architecture (Phase 6)

Key components:
- **batch-control.json**: Repository file for emergency stop, circuit breaker state, budget tracking
- **D1 database**: Metrics storage for batch runs, per-recipe results
- **Rollback script**: `scripts/rollback-batch.sh` using batch ID grep + git rm
- **Workflow integration**: Pre-flight checks, circuit breaker logic, post-batch updates

## Security (Phase 7)

Key considerations:
- Access control: Write access for operators, admin for nuclear stop
- Attack vectors: Control file tampering, circuit breaker bypass, batch ID spoofing
- Post-merge monitoring: Checksum drift detection for supply chain security
- Audit trail: Git history + D1 metrics with 90-day retention

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-01-27
