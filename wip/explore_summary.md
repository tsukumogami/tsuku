# Exploration Summary: Error UX and Verbose Mode

## Problem (Phase 1)

The discovery resolver has multiple failure modes across its three stages (registry, ecosystem, LLM), but currently produces only generic error messages. Users get a single `NotFoundError` message regardless of why discovery failed. They also have no visibility into which resolver stages ran during discovery, making debugging difficult.

## Decision Drivers (Phase 1)

- **Actionable error messages**: Each failure scenario needs a message that tells users what to do next
- **Consistent verbose output**: Show resolver chain progress using existing `-v/--verbose` flag
- **No internal terminology leakage**: Messages shouldn't reference "ChainResolver", "EcosystemProbe", etc.
- **Reuse existing infrastructure**: Use the existing `internal/log/` package and verbosity flags
- **Coordination with #1321**: Disambiguation handles `AmbiguousMatchError`; this issue handles remaining errors

## Research Findings (Phase 2)

### Upstream Context
- Parent design: `docs/designs/DESIGN-discovery-resolver.md` Phase 6 specifies 8 error scenarios
- Related design: `docs/designs/DESIGN-disambiguation.md` handles ambiguity-specific errors (#1321)

### Existing Infrastructure
- Verbose flag exists: `--verbose/-v` maps to INFO level
- Debug flag exists: `--debug` maps to DEBUG level with timestamps/source
- Logging package: `internal/log/` with Logger interface wrapping slog
- Error types: `NotFoundError`, `RateLimitError`, `ErrBudgetExceeded` exist

### Current Gaps
1. `NotFoundError` has single hardcoded message for all "not found" scenarios
2. No distinction between "LLM not configured" vs "LLM rate limited" vs "all stages failed"
3. No `--deterministic-only` guard errors implemented
4. No INFO-level verbose output for resolver chain progress
5. Soft errors logged at WARN but not visible to users in normal mode

## Current Status

**Phase:** 2 - Research (complete)
**Last Updated:** 2026-02-11
