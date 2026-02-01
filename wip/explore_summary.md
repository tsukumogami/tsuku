# Exploration Summary: Merge Job Completion

## Problem (Phase 1)
The merge job in batch-generate.yml has working constraint derivation and PR creation, but lacks structured commit messages with batch metadata (batch_id, success_rate), SLI metrics collection, circuit breaker state updates, and auto-merge gating. These are required by DESIGN-batch-recipe-generation.md (#1256) before downstream work (#1258 PR CI filtering) can proceed.

## Decision Drivers (Phase 1)
- Existing infrastructure: batch-control.json, internal/batch/, scripts/update_breaker.sh, telemetry schema
- Must integrate with Go orchestrator (cmd/batch-generate) which already generates batch_id
- Circuit breaker scripts exist but aren't wired into the GitHub Actions merge job
- data/metrics/ directory doesn't exist yet; SLI format specified in upstream design
- Auto-merge should be conservative: blocked by run_command, zero-pass recipes, or CI failures

## Decision (Phase 5)
Shell-based merge job additions with conservative auto-merge. Four new steps: batch_id generation, structured commit with trailers, SLI metrics collection, circuit breaker update. Auto-merge only for clean batches (zero exclusions). All shell/jq, no new Go code.

## Current Status
**Phase:** Complete
**Last Updated:** 2026-02-01
