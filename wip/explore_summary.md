# Exploration Summary: Homebrew Deterministic Error Path

## Problem (Phase 1)
The batch orchestrator crashes with "no LLM providers available" when Homebrew deterministic generation fails, because the CLI has no way to suppress LLM fallback. The DeterministicOnly session option exists in the Go API but isn't exposed through the CLI.

## Decision Drivers (Phase 1)
- Batch pipeline runs without LLM API keys by design
- DeterministicFailedError type already exists with failure categories
- Orchestrator invokes CLI as subprocess (not programmatic API)
- Existing interactive flow must remain unchanged

## Research Findings (Phase 2)
- `DeterministicOnly` field exists in `SessionOptions` (builder.go:212)
- `classifyDeterministicFailure()` exists in homebrew.go:472
- CLI `create.go` constructs `SessionOptions` at line 266 without setting `DeterministicOnly`
- Batch orchestrator calls `tsuku create` at orchestrator.go:154, no `--deterministic-only` flag available
- `categoryFromExitCode` maps exit codes to failure categories but has no code for "deterministic insufficient"

## Options (Phase 3)
- Option A: Add `--deterministic-only` CLI flag to `tsuku create`
- Option B: Orchestrator detects no-LLM environment and parses output

## Decision (Phase 5)
**Problem:** The batch pipeline crashes when Homebrew deterministic generation fails because the CLI can't suppress LLM fallback, producing "no LLM providers available" instead of a structured failure record.
**Decision:** Add a `--deterministic-only` CLI flag to `tsuku create` that sets `SessionOptions.DeterministicOnly`, and add a dedicated exit code for deterministic failures so the batch orchestrator can categorize them without output parsing.
**Rationale:** The Go API already supports deterministic-only mode with structured errors. Exposing it through the CLI closes the gap between the builder API and the subprocess-based orchestrator. A dedicated exit code lets the orchestrator classify failures reliably.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-01-31
