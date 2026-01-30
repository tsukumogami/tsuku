# Exploration Summary: Homebrew Deterministic Mode

## Problem (Phase 1)
The Homebrew builder returns `RequiresLLM() = true` and fails when deterministic generation can't fall back to LLM, blocking the batch pipeline which must operate without API keys at $0 cost.

## Decision Drivers (Phase 1)
- Batch pipeline requires deterministic-only operation (no API keys, $0 cost)
- Structured failure records needed when deterministic generation fails (feeds gap analysis)
- Existing builder interface (SessionBuilder/BuildSession) must be preserved
- Bottle inspection already works; the issue is error handling and mode signaling
- Failure categories must match the failure-record.schema.json categories

## Research Findings (Phase 2)
- HomebrewBuilder already tries deterministic first, falls back to LLM (homebrew.go:412-434)
- RequiresLLM() unconditionally returns true (homebrew.go:211)
- ensureLLMProvider() fails with generic error when no API key (homebrew.go:459-486)
- No DeterministicFailedError type exists - failures aren't structured
- failure-record.schema.json defines categories: missing_dep, no_bottles, build_from_source, complex_archive, api_error, validation_failed
- Line 430 has potential nil dereference: s.provider.Name() before ensureLLMProvider()

## Options (Phase 3)
- Decision 1 (mode signaling): Session option flag vs separate builder type vs constructor option
- Decision 2 (failure reporting): New error type with category vs wrapped existing errors
- Decision 3 (RequiresLLM): Return false vs add CanRunDeterministic() vs keep true

## Decision (Phase 5)
**Problem:** The Homebrew builder declares RequiresLLM()=true and fails when deterministic generation can't fall back to LLM, blocking the batch pipeline which must operate without API keys at $0 cost.
**Decision:** Add a DeterministicOnly session option that prevents LLM fallback and returns structured DeterministicFailedError on failure, with failure categories matching the failure-record schema.
**Rationale:** The builder already tries deterministic first. The change is small (new option, new error type, conditional in Generate) and doesn't alter the existing interactive flow. Structured errors feed directly into the batch pipeline's failure analysis system.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-01-29
