# Exploration Summary: Batch Recipe Generation CI Pipeline

## Problem (Phase 1)
The registry scale strategy requires automated batch generation of recipes across 8 deterministic ecosystems, but no CI pipeline exists to orchestrate generation, validation, failure recording, and merge at scale.

## Decision Drivers (Phase 1)
- Must operate without LLM API keys ($0 cost per recipe)
- Failures must produce structured records matching failure-record.schema.json
- Validation across 5 target environments (partial coverage OK to merge)
- Circuit breaker auto-pause at <50% success rate per ecosystem
- Rate limiting per ecosystem (1 req/sec GitHub API minimum)
- Cost control: 1000 macOS min/week, 5000 Linux min/week
- Batch ID metadata for surgical rollback
- No auto-merge for recipes with run_command actions
- Re-queue mechanism when blocking dependencies are resolved

## Research Findings (Phase 2)
- 37 existing GitHub Actions workflows; batch-operations.yml already exists as placeholder
- Builder invocation: Registry -> Builder -> Session -> Orchestrator -> Sandbox
- DeterministicSession wraps ecosystem builders; Homebrew now has DeterministicOnly mode
- Sandbox validation: Docker/Podman containers with resource limits, network isolation
- Priority queue schema and failure record schema exist in data/schemas/
- batch-control.json pattern already established for circuit breaker state

## Current Status
**Phase:** 2 - Research complete
**Last Updated:** 2026-01-29
